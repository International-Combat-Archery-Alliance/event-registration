package dynamo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/games"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

var _ games.Repository = &DB{}

// gameDynamo carries no GSI1PK/GSI1SK: only EVENT and TEAM masters carry GSI
// attributes (RFC-0001/0002 GSI rule). The struct shape makes poisoning the
// GSI1 events-list query impossible.
type gameDynamo struct {
	PK           string
	SK           string
	ID           string
	EventID      string
	Phase        string
	Round        int
	Seq          int
	Status       string
	SideATeamID  *string
	SideAIsBye   bool
	SideBTeamID  *string
	SideBIsBye   bool
	ScoreA       *int
	ScoreB       *int
	ForfeitSide  *string
	NextGameID   *string
	NextGameSlot *string
	PrevAID      *string
	PrevBID      *string
	Notes        *string
	StartTime    *time.Time
	Version      int
}

const gameEntityName = "GAME"

func gamePK(eventID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", gameEntityName, eventID)
}

func gameSK(phase games.GamePhase, round, seq int) string {
	return fmt.Sprintf("%s#%s#%02d#%02d", gameEntityName, phase, round, seq)
}

func newGameDynamo(game games.Game) gameDynamo {
	return gameDynamo{
		PK:           gamePK(game.EventID),
		SK:           gameSK(game.Phase, game.Round, game.Seq),
		ID:           game.ID.String(),
		EventID:      game.EventID.String(),
		Phase:        string(game.Phase),
		Round:        game.Round,
		Seq:          game.Seq,
		Status:       string(game.Status),
		SideATeamID:  uuidPtrToStringPtr(game.SideA.TeamID),
		SideAIsBye:   game.SideA.IsBye,
		SideBTeamID:  uuidPtrToStringPtr(game.SideB.TeamID),
		SideBIsBye:   game.SideB.IsBye,
		ScoreA:       game.ScoreA,
		ScoreB:       game.ScoreB,
		ForfeitSide:  slotPtrToStringPtr(game.ForfeitSide),
		NextGameID:   uuidPtrToStringPtr(game.NextGameID),
		NextGameSlot: slotPtrToStringPtr(game.NextGameSlot),
		PrevAID:      uuidPtrToStringPtr(game.PrevAID),
		PrevBID:      uuidPtrToStringPtr(game.PrevBID),
		Notes:        game.Notes,
		StartTime:    game.StartTime,
		Version:      game.Version,
	}
}

func uuidPtrToStringPtr(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

func slotPtrToStringPtr(s *games.GameSlot) *string {
	if s == nil {
		return nil
	}
	v := string(*s)
	return &v
}

func stringPtrToUUIDPtr(s *string) (*uuid.UUID, error) {
	if s == nil {
		return nil, nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func stringPtrToSlotPtr(s *string) *games.GameSlot {
	if s == nil {
		return nil
	}
	slot := games.GameSlot(*s)
	return &slot
}

func gameFromGameDynamo(item gameDynamo) (games.Game, error) {
	id, err := uuid.Parse(item.ID)
	if err != nil {
		return games.Game{}, err
	}
	eventID, err := uuid.Parse(item.EventID)
	if err != nil {
		return games.Game{}, err
	}
	sideATeamID, err := stringPtrToUUIDPtr(item.SideATeamID)
	if err != nil {
		return games.Game{}, err
	}
	sideBTeamID, err := stringPtrToUUIDPtr(item.SideBTeamID)
	if err != nil {
		return games.Game{}, err
	}
	nextGameID, err := stringPtrToUUIDPtr(item.NextGameID)
	if err != nil {
		return games.Game{}, err
	}
	prevAID, err := stringPtrToUUIDPtr(item.PrevAID)
	if err != nil {
		return games.Game{}, err
	}
	prevBID, err := stringPtrToUUIDPtr(item.PrevBID)
	if err != nil {
		return games.Game{}, err
	}

	return games.Game{
		ID:           id,
		EventID:      eventID,
		Phase:        games.GamePhase(item.Phase),
		Round:        item.Round,
		Seq:          item.Seq,
		Status:       games.GameStatus(item.Status),
		SideA:        games.Side{TeamID: sideATeamID, IsBye: item.SideAIsBye},
		SideB:        games.Side{TeamID: sideBTeamID, IsBye: item.SideBIsBye},
		ScoreA:       item.ScoreA,
		ScoreB:       item.ScoreB,
		ForfeitSide:  stringPtrToSlotPtr(item.ForfeitSide),
		NextGameID:   nextGameID,
		NextGameSlot: stringPtrToSlotPtr(item.NextGameSlot),
		PrevAID:      prevAID,
		PrevBID:      prevBID,
		Notes:        item.Notes,
		StartTime:    item.StartTime,
		Version:      item.Version,
	}, nil
}

// queryGamesForEvent reads the whole GAME# partition for an event. Game SKs
// carry phase/round/seq, not the game UUID, so ID lookups filter in code;
// partitions stay small (a 16-team round robin is 120 games).
func (d *DB) queryGamesForEvent(ctx context.Context, eventID uuid.UUID) ([]gameDynamo, error) {
	var items []gameDynamo
	var startKey map[string]types.AttributeValue

	for {
		out, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(d.tableName),
			KeyConditionExpression: aws.String("PK = :pk"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":pk": &types.AttributeValueMemberS{Value: gamePK(eventID)},
			},
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, games.NewTimeoutError("ListGamesForEvent timed out")
			}
			return nil, games.NewFailedToFetchError(fmt.Sprintf("Failed to fetch games for event %q", eventID), err)
		}

		var page []gameDynamo
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &page); err != nil {
			return nil, games.NewFailedToFetchError("Failed to unmarshal games from DB", err)
		}
		items = append(items, page...)

		if len(out.LastEvaluatedKey) == 0 {
			return items, nil
		}
		startKey = out.LastEvaluatedKey
	}
}

func orderGamesQualifyingFirst(items []gameDynamo) {
	phaseRank := func(phase string) int {
		if phase == string(games.GamePhaseQualifying) {
			return 0
		}
		return 1
	}
	sort.SliceStable(items, func(i, j int) bool {
		if phaseRank(items[i].Phase) != phaseRank(items[j].Phase) {
			return phaseRank(items[i].Phase) < phaseRank(items[j].Phase)
		}
		if items[i].Round != items[j].Round {
			return items[i].Round < items[j].Round
		}
		return items[i].Seq < items[j].Seq
	})
}

func (d *DB) GetGame(ctx context.Context, eventID, gameID uuid.UUID) (games.Game, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	items, err := d.queryGamesForEvent(ctx, eventID)
	if err != nil {
		return games.Game{}, err
	}

	for _, item := range items {
		if item.ID == gameID.String() {
			game, err := gameFromGameDynamo(item)
			if err != nil {
				return games.Game{}, games.NewFailedToFetchError(fmt.Sprintf("Failed to parse game %q", gameID), err)
			}
			return game, nil
		}
	}

	return games.Game{}, games.NewGameDoesNotExistError(fmt.Sprintf("Game with ID %q not found", gameID), nil)
}

func (d *DB) ListGamesForEvent(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	items, err := d.queryGamesForEvent(ctx, eventID)
	if err != nil {
		return nil, err
	}

	// 'P' < 'Q' lexically, so PLAYOFF sort keys precede QUALIFYING ones;
	// the schedule always reads QUALIFYING first.
	orderGamesQualifyingFirst(items)

	result := make([]games.Game, 0, len(items))
	for _, item := range items {
		game, err := gameFromGameDynamo(item)
		if err != nil {
			return nil, games.NewFailedToFetchError("Failed to parse games from DB", err)
		}
		result = append(result, game)
	}
	return result, nil
}

// batchWriteGameRequests issues puts/deletes in chunks of 25 with an
// UnprocessedItems retry loop. Generate writes whole schedules (a 16-team
// round robin is 120 games), far beyond a single request. Each chunk gets
// its own retry budget so a throttled chunk cannot starve later chunks.
func (d *DB) batchWriteGameRequests(ctx context.Context, requests []types.WriteRequest) error {
	for len(requests) > 0 {
		batch := requests
		if len(batch) > 25 {
			batch = batch[:25]
		}
		rest := requests[len(batch):]

		unprocessed, err := d.writeGameBatch(ctx, batch)
		if err != nil {
			return err
		}
		requests = append(rest, unprocessed...)
	}
	return nil
}

func (d *DB) writeGameBatch(ctx context.Context, batch []types.WriteRequest) ([]types.WriteRequest, error) {
	pending := batch
	for attempt := 0; ; attempt++ {
		out, err := d.dynamoClient.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				d.tableName: pending,
			},
		})
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, games.NewTimeoutError("BatchWrite games timed out")
			}
			return nil, games.NewFailedToWriteError("Failed BatchWriteItem call", err)
		}

		pending = out.UnprocessedItems[d.tableName]
		if len(pending) == 0 {
			return nil, nil
		}
		if attempt >= 5 {
			return nil, games.NewFailedToWriteError(fmt.Sprintf("Failed to write %d games after retries", len(pending)), nil)
		}
	}
}

func (d *DB) CreateGames(ctx context.Context, gameList []games.Game) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Puts are unconditional (BatchWriteItem carries no conditions): callers
	// must ensure the partition is empty first - generate refuses when games
	// exist, and the replace path deletes unplayed games before recreating.
	requests := make([]types.WriteRequest, 0, len(gameList))
	for _, game := range gameList {
		item, err := attributevalue.MarshalMap(newGameDynamo(game))
		if err != nil {
			return games.NewFailedToTranslateToDBModelError("Failed to convert Game to gameDynamo", err)
		}
		requests = append(requests, types.WriteRequest{
			PutRequest: &types.PutRequest{Item: item},
		})
	}

	return d.batchWriteGameRequests(ctx, requests)
}

func (d *DB) UpdateGame(ctx context.Context, game games.Game) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoItem := newGameDynamo(game)

	item, err := attributevalue.MarshalMap(dynamoItem)
	if err != nil {
		return games.NewFailedToTranslateToDBModelError("Failed to convert Game to gameDynamo", err)
	}

	expr := exprMustBuild(expression.NewBuilder().
		WithCondition(existingEntityVersionConditional(dynamoItem.Version)))

	_, err = d.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:                 aws.String(d.tableName),
		Item:                      item,
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		var condCheckFailedErr *types.ConditionalCheckFailedException
		if errors.As(err, &condCheckFailedErr) {
			return games.NewGameDoesNotExistError(fmt.Sprintf("Game with ID %q does not exist", game.ID), err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return games.NewTimeoutError("UpdateGame timed out")
		} else {
			return games.NewFailedToWriteError("Failed PutItem call", err)
		}
	}

	return nil
}

func (d *DB) DeleteGames(ctx context.Context, eventID uuid.UUID, gameIDs []uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Deletes resolve UUIDs to keys via a prior read and carry no conditions,
	// so callers must serialize scoring against regenerate/delete; unknown
	// IDs are silently ignored.
	items, err := d.queryGamesForEvent(ctx, eventID)
	if err != nil {
		return err
	}

	target := make(map[string]struct{}, len(gameIDs))
	for _, id := range gameIDs {
		target[id.String()] = struct{}{}
	}

	var requests []types.WriteRequest
	for _, item := range items {
		if _, ok := target[item.ID]; !ok {
			continue
		}
		requests = append(requests, types.WriteRequest{
			DeleteRequest: &types.DeleteRequest{
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: item.PK},
					"SK": &types.AttributeValueMemberS{Value: item.SK},
				},
			},
		})
	}

	return d.batchWriteGameRequests(ctx, requests)
}

func (d *DB) UpdateGameChecked(ctx context.Context, eventID uuid.UUID, game games.Game) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoItem := newGameDynamo(game)
	item, err := attributevalue.MarshalMap(dynamoItem)
	if err != nil {
		return games.NewFailedToTranslateToDBModelError("Failed to convert Game to gameDynamo", err)
	}

	checkExpr := exprMustBuild(expression.NewBuilder().WithCondition(
		expression.Name("Status").AttributeNotExists().
			Or(expression.Name("Status").NotEqual(expression.Value("FINALIZED"))),
	))
	putExpr := exprMustBuild(expression.NewBuilder().
		WithCondition(existingEntityVersionConditional(dynamoItem.Version)))

	_, err = d.dynamoClient.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				ConditionCheck: &types.ConditionCheck{
					TableName: aws.String(d.tableName),
					Key: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: eventPK(eventID)},
						"SK": &types.AttributeValueMemberS{Value: eventSK(eventID)},
					},
					ConditionExpression:       checkExpr.Condition(),
					ExpressionAttributeNames:  checkExpr.Names(),
					ExpressionAttributeValues: checkExpr.Values(),
				},
			},
			{
				Put: &types.Put{
					TableName:                 aws.String(d.tableName),
					Item:                      item,
					ConditionExpression:       putExpr.Condition(),
					ExpressionAttributeNames:  putExpr.Names(),
					ExpressionAttributeValues: putExpr.Values(),
				},
			},
		},
	})
	if err != nil {
		var transactionFailedErr *types.TransactionCanceledException
		if errors.As(err, &transactionFailedErr) {
			reasons := transactionFailedErr.CancellationReasons
			if len(reasons) > 0 && reasons[0].Code != nil && *reasons[0].Code == "ConditionalCheckFailed" {
				return games.NewEventFinalizedError(fmt.Sprintf("Event %q is FINALIZED", eventID), err)
			}
			if len(reasons) > 1 && reasons[1].Code != nil && *reasons[1].Code == "ConditionalCheckFailed" {
				return games.NewGameDoesNotExistError(fmt.Sprintf("Game with ID %q does not exist", game.ID), err)
			}
			return games.NewFailedToWriteError("Game checked-update transaction failed", err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return games.NewTimeoutError("UpdateGameChecked timed out")
		} else {
			return games.NewFailedToWriteError("Failed TransactWriteItems call", err)
		}
	}

	return nil
}
