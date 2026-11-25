package dynamo

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
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
	GameNo       int
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
		GameNo:       game.GameNo,
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
		GameNo:       item.GameNo,
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

func (d *DB) GetGameByNumber(ctx context.Context, eventID uuid.UUID, gameNo int) (games.Game, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	items, err := d.queryGamesForEvent(ctx, eventID)
	if err != nil {
		return games.Game{}, err
	}

	for _, item := range items {
		if item.GameNo == gameNo {
			game, err := gameFromGameDynamo(item)
			if err != nil {
				return games.Game{}, games.NewFailedToFetchError(fmt.Sprintf("Failed to parse game %d", gameNo), err)
			}
			return game, nil
		}
	}

	return games.Game{}, games.NewGameDoesNotExistError(fmt.Sprintf("Game %d not found for event %q", gameNo, eventID), nil)
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

// gameWriteChunk is the number of game actions per transaction. A
// TransactWriteItems call carries at most 100 actions; each chunk also carries
// one ConditionCheck on the EVENT item, so 99 rows per chunk.
const gameWriteChunk = 99

// eventScheduleCondition is the structural write barrier shared by every
// schedule mutation: the caller's event Version must still be current and the
// event must not be FINALIZED. Finalize bumps Version, so the condition also
// rejects a finalize that raced the caller.
func eventScheduleCondition(expectedVersion int) expression.ConditionBuilder {
	return expression.Name("Version").Equal(expression.Value(expectedVersion)).
		And(expression.Name("Status").AttributeNotExists().
			Or(expression.Name("Status").NotEqual(expression.Value(string(events.EventStatusFinalized)))))
}

func (d *DB) conditionCheckEventSchedule(eventID uuid.UUID, expectedVersion int) types.TransactWriteItem {
	cond := exprMustBuild(expression.NewBuilder().WithCondition(eventScheduleCondition(expectedVersion)))
	return types.TransactWriteItem{
		ConditionCheck: &types.ConditionCheck{
			TableName: aws.String(d.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: eventPK(eventID)},
				"SK": &types.AttributeValueMemberS{Value: eventSK(eventID)},
			},
			ConditionExpression:       cond.Condition(),
			ExpressionAttributeNames:  cond.Names(),
			ExpressionAttributeValues: cond.Values(),
		},
	}
}

func (d *DB) eventScheduleUpdate(eventID uuid.UUID, condition expression.ConditionBuilder, update string, values map[string]types.AttributeValue) types.TransactWriteItem {
	cond := exprMustBuild(expression.NewBuilder().WithCondition(condition))
	for k, v := range cond.Values() {
		values[k] = v
	}
	return types.TransactWriteItem{
		Update: &types.Update{
			TableName: aws.String(d.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: eventPK(eventID)},
				"SK": &types.AttributeValueMemberS{Value: eventSK(eventID)},
			},
			UpdateExpression:          aws.String(update),
			ConditionExpression:       cond.Condition(),
			ExpressionAttributeNames:  cond.Names(),
			ExpressionAttributeValues: values,
		},
	}
}

func deletableGameCondition(version int) expression.ConditionBuilder {
	deletable := expression.Name("Status").Equal(expression.Value(string(games.GameStatusScheduled))).
		Or(expression.Name("Status").Equal(expression.Value(string(games.GameStatusBye)))).
		Or(expression.Name("Status").Equal(expression.Value(string(games.GameStatusCancelled))))
	return expression.Name("Version").Equal(expression.Value(version)).And(deletable)
}

func cancellationFailed(reasons []types.CancellationReason, index int) bool {
	return index < len(reasons) && reasons[index].Code != nil && *reasons[index].Code == "ConditionalCheckFailed"
}

func (d *DB) writeGameTransaction(ctx context.Context, items []types.TransactWriteItem) error {
	_, err := d.dynamoClient.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return games.NewTimeoutError("game transaction timed out")
	}
	return games.NewFailedToWriteError("Failed TransactWriteItems call", err)
}

// CreateGames writes a generated schedule in chunked transactions. Puts are
// conditional on (attribute_not_exists(Status) OR Status = SCHEDULED), so a
// preserved result row blocks its slot instead of being overwritten, while a
// leftover unplayed row from an earlier partial replace can be replaced. Each
// chunk also ConditionChecks the EVENT item at epochVersion, so a concurrent
// schedule writer that bumps the event Version between chunks stops the batch
// there. Chunks are atomic individually, not as a group: a failure stops at
// that chunk and earlier chunks stay written. The caller aborts; a later
// replace sweeps the unplayed leftovers and preserves results.
func (d *DB) CreateGames(ctx context.Context, eventID uuid.UUID, epochVersion int, gameList []games.Game) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if len(gameList) == 0 {
		return nil
	}

	// Validate the whole schedule before the first transaction: a malformed
	// row must never leave earlier chunks written.
	for _, game := range gameList {
		if err := game.Validate(); err != nil {
			return err
		}
	}

	putCondition := exprMustBuild(expression.NewBuilder().WithCondition(
		expression.Name("Status").AttributeNotExists().
			Or(expression.Name("Status").Equal(expression.Value(string(games.GameStatusScheduled)))),
	))

	pending := make([]types.TransactWriteItem, 0, gameWriteChunk)
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		items := make([]types.TransactWriteItem, 0, len(pending)+1)
		items = append(items, d.conditionCheckEventSchedule(eventID, epochVersion))
		items = append(items, pending...)
		if err := d.writeGameTransaction(ctx, items); err != nil {
			var canceled *types.TransactionCanceledException
			if errors.As(err, &canceled) {
				reasons := canceled.CancellationReasons
				if cancellationFailed(reasons, 0) {
					return games.NewVersionConflictError(fmt.Sprintf("Event %q schedule changed concurrently", eventID), err)
				}
				return games.NewSlotOccupiedError("A game slot is held by a recorded result or a concurrent game", err)
			}
			return err
		}
		pending = pending[:0]
		return nil
	}

	for _, game := range gameList {
		item, err := attributevalue.MarshalMap(newGameDynamo(game))
		if err != nil {
			return games.NewFailedToTranslateToDBModelError("Failed to convert Game to gameDynamo", err)
		}
		pending = append(pending, types.TransactWriteItem{
			Put: &types.Put{
				TableName:                 aws.String(d.tableName),
				Item:                      item,
				ConditionExpression:       putCondition.Condition(),
				ExpressionAttributeNames:  putCondition.Names(),
				ExpressionAttributeValues: putCondition.Values(),
			},
		})
		if len(pending) == gameWriteChunk {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// DeleteGames deletes the named unplayed games in chunked transactions. The
// rows are resolved from a pre-delete read and every delete is conditional on
// that row's Version and on it still being SCHEDULED/BYE/CANCELLED, so a
// result recorded after the read fails the transaction instead of being
// destroyed. A target row that already holds a result at pre-read time is
// refused before any chunk runs. Chunks are atomic individually, not as a
// group: a failure stops at that chunk and earlier chunks stay deleted. That
// partial state is recoverable: the surviving rows are all still listed and a
// later replace sweeps the unplayed leftovers (a result is never part of a
// delete set).
func (d *DB) DeleteGames(ctx context.Context, eventID uuid.UUID, epochVersion int, gameNos []int) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if len(gameNos) == 0 {
		return nil
	}

	items, err := d.queryGamesForEvent(ctx, eventID)
	if err != nil {
		return err
	}

	target := make(map[int]struct{}, len(gameNos))
	for _, gameNo := range gameNos {
		target[gameNo] = struct{}{}
	}

	for _, item := range items {
		if _, ok := target[item.GameNo]; !ok {
			continue
		}
		if !games.GameStatus(item.Status).Deletable() {
			return games.NewVersionConflictError(fmt.Sprintf("Game %d holds a result and cannot be deleted", item.GameNo), nil)
		}
	}

	pending := make([]types.TransactWriteItem, 0, gameWriteChunk)
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		items := make([]types.TransactWriteItem, 0, len(pending)+1)
		items = append(items, d.conditionCheckEventSchedule(eventID, epochVersion))
		items = append(items, pending...)
		if err := d.writeGameTransaction(ctx, items); err != nil {
			var canceled *types.TransactionCanceledException
			if errors.As(err, &canceled) {
				reasons := canceled.CancellationReasons
				if cancellationFailed(reasons, 0) {
					return games.NewVersionConflictError(fmt.Sprintf("Event %q schedule changed concurrently", eventID), err)
				}
				return games.NewVersionConflictError("A game in the delete set changed concurrently (a result may have landed)", err)
			}
			return err
		}
		pending = pending[:0]
		return nil
	}

	for _, item := range items {
		if _, ok := target[item.GameNo]; !ok {
			continue
		}
		cond := exprMustBuild(expression.NewBuilder().WithCondition(deletableGameCondition(item.Version)))
		pending = append(pending, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(d.tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: item.PK},
					"SK": &types.AttributeValueMemberS{Value: item.SK},
				},
				ConditionExpression:       cond.Condition(),
				ExpressionAttributeNames:  cond.Names(),
				ExpressionAttributeValues: cond.Values(),
			},
		})
		if len(pending) == gameWriteChunk {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// BeginScheduleWrite is the epoch grant for a multi-step schedule write. The
// successful event update is the single authorization to proceed: another
// schedule writer can only pass by bumping Version again, which invalidates
// every later epoch-conditioned step of this write. It writes no games.
func (d *DB) BeginScheduleWrite(ctx context.Context, eventID uuid.UUID, expectedVersion int) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	update := "SET ScheduleGeneration = if_not_exists(ScheduleGeneration, :zero) + :one, Version = Version + :one"
	item := d.eventScheduleUpdate(eventID, eventScheduleCondition(expectedVersion), update, map[string]types.AttributeValue{
		":zero": &types.AttributeValueMemberN{Value: "0"},
		":one":  &types.AttributeValueMemberN{Value: "1"},
	})

	if err := d.writeGameTransaction(ctx, []types.TransactWriteItem{item}); err != nil {
		var canceled *types.TransactionCanceledException
		if errors.As(err, &canceled) {
			return 0, games.NewVersionConflictError(fmt.Sprintf("Event %q schedule changed concurrently", eventID), err)
		}
		return 0, err
	}
	return expectedVersion + 1, nil
}

// ReserveGameNumbers advances the event's LastGameNo high-water mark and bumps
// Version in one transaction, conditional on the expected Version. The
// high-water mark never moves backwards, so deleted game numbers are never
// reissued. It returns the new event Version.
func (d *DB) ReserveGameNumbers(ctx context.Context, eventID uuid.UUID, expectedVersion int, lastGameNo int) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	condition := eventScheduleCondition(expectedVersion).
		And(expression.Name("LastGameNo").AttributeNotExists().
			Or(expression.Name("LastGameNo").LessThanEqual(expression.Value(lastGameNo))))
	update := "SET LastGameNo = :lastGameNo, Version = Version + :one"
	item := d.eventScheduleUpdate(eventID, condition, update, map[string]types.AttributeValue{
		":one":        &types.AttributeValueMemberN{Value: "1"},
		":lastGameNo": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", lastGameNo)},
	})

	if err := d.writeGameTransaction(ctx, []types.TransactWriteItem{item}); err != nil {
		var canceled *types.TransactionCanceledException
		if errors.As(err, &canceled) {
			return 0, games.NewVersionConflictError(fmt.Sprintf("Event %q schedule changed concurrently", eventID), err)
		}
		return 0, err
	}
	return expectedVersion + 1, nil
}

// CreateScheduledGame folds the schedule guard and the row write into one
// transaction: the event update (Version CAS, FINALIZED barrier,
// ScheduleGeneration +1, LastGameNo = the minted number) and the conditional
// put happen together or not at all. A concurrent schedule writer that
// committed first leaves the Version stale, so the whole transaction fails
// rather than minting a duplicate gameNo. The put requires an empty slot.
func (d *DB) CreateScheduledGame(ctx context.Context, eventID uuid.UUID, expectedVersion int, game games.Game) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	if err := game.Validate(); err != nil {
		return err
	}
	item, err := attributevalue.MarshalMap(newGameDynamo(game))
	if err != nil {
		return games.NewFailedToTranslateToDBModelError("Failed to convert Game to gameDynamo", err)
	}

	condition := eventScheduleCondition(expectedVersion).
		And(expression.Name("LastGameNo").AttributeNotExists().
			Or(expression.Name("LastGameNo").LessThan(expression.Value(game.GameNo))))
	update := "SET ScheduleGeneration = if_not_exists(ScheduleGeneration, :zero) + :one, Version = Version + :one, LastGameNo = :gameNo"
	eventItem := d.eventScheduleUpdate(eventID, condition, update, map[string]types.AttributeValue{
		":zero":   &types.AttributeValueMemberN{Value: "0"},
		":one":    &types.AttributeValueMemberN{Value: "1"},
		":gameNo": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", game.GameNo)},
	})

	putCondition := exprMustBuild(expression.NewBuilder().WithCondition(
		expression.Name("PK").AttributeNotExists(),
	))
	gameItem := types.TransactWriteItem{
		Put: &types.Put{
			TableName:                 aws.String(d.tableName),
			Item:                      item,
			ConditionExpression:       putCondition.Condition(),
			ExpressionAttributeNames:  putCondition.Names(),
			ExpressionAttributeValues: putCondition.Values(),
		},
	}

	if err := d.writeGameTransaction(ctx, []types.TransactWriteItem{eventItem, gameItem}); err != nil {
		var canceled *types.TransactionCanceledException
		if errors.As(err, &canceled) {
			reasons := canceled.CancellationReasons
			if cancellationFailed(reasons, 0) {
				return games.NewVersionConflictError(fmt.Sprintf("Event %q schedule changed concurrently", eventID), err)
			}
			return games.NewSlotOccupiedError(fmt.Sprintf("Game slot round %d seq %d is already occupied", game.Round, game.Seq), err)
		}
		return err
	}
	return nil
}

// DeleteScheduledGame folds the schedule guard and the row delete into one
// transaction. The delete is conditional on the row's stored Version and on
// it still being SCHEDULED/BYE/CANCELLED, so a result recorded after the read
// makes the whole transaction fail.
func (d *DB) DeleteScheduledGame(ctx context.Context, eventID uuid.UUID, expectedVersion int, game games.Game) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	if !game.Status.Deletable() {
		return games.NewVersionConflictError(fmt.Sprintf("Game %d holds a result and cannot be deleted", game.GameNo), nil)
	}

	update := "SET ScheduleGeneration = if_not_exists(ScheduleGeneration, :zero) + :one, Version = Version + :one"
	eventItem := d.eventScheduleUpdate(eventID, eventScheduleCondition(expectedVersion), update, map[string]types.AttributeValue{
		":zero": &types.AttributeValueMemberN{Value: "0"},
		":one":  &types.AttributeValueMemberN{Value: "1"},
	})

	cond := exprMustBuild(expression.NewBuilder().WithCondition(deletableGameCondition(game.Version)))
	gameItem := types.TransactWriteItem{
		Delete: &types.Delete{
			TableName: aws.String(d.tableName),
			Key: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: gamePK(game.EventID)},
				"SK": &types.AttributeValueMemberS{Value: gameSK(game.Phase, game.Round, game.Seq)},
			},
			ConditionExpression:       cond.Condition(),
			ExpressionAttributeNames:  cond.Names(),
			ExpressionAttributeValues: cond.Values(),
		},
	}

	if err := d.writeGameTransaction(ctx, []types.TransactWriteItem{eventItem, gameItem}); err != nil {
		var canceled *types.TransactionCanceledException
		if errors.As(err, &canceled) {
			reasons := canceled.CancellationReasons
			if cancellationFailed(reasons, 0) {
				return games.NewVersionConflictError(fmt.Sprintf("Event %q schedule changed concurrently", eventID), err)
			}
			return games.NewVersionConflictError(fmt.Sprintf("Game %d changed concurrently (a result may have landed)", game.GameNo), err)
		}
		return err
	}
	return nil
}

// UpdateGameScheduleChecked folds the schedule guard and the schedule edit
// into one transaction: the event update (Version CAS, FINALIZED barrier,
// ScheduleGeneration +1) plus the row put, plus the old-row delete when the
// edit moves the (phase, round, seq) key. A destination occupied by another
// row fails the put, reported as a slot conflict.
func (d *DB) UpdateGameScheduleChecked(ctx context.Context, eventID uuid.UUID, expectedVersion int, stored, updated games.Game) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	if err := updated.Validate(); err != nil {
		return err
	}
	item, err := attributevalue.MarshalMap(newGameDynamo(updated))
	if err != nil {
		return games.NewFailedToTranslateToDBModelError("Failed to convert Game to gameDynamo", err)
	}

	update := "SET ScheduleGeneration = if_not_exists(ScheduleGeneration, :zero) + :one, Version = Version + :one"
	eventItem := d.eventScheduleUpdate(eventID, eventScheduleCondition(expectedVersion), update, map[string]types.AttributeValue{
		":zero": &types.AttributeValueMemberN{Value: "0"},
		":one":  &types.AttributeValueMemberN{Value: "1"},
	})

	moving := gameSK(stored.Phase, stored.Round, stored.Seq) != gameSK(updated.Phase, updated.Round, updated.Seq)

	transactItems := []types.TransactWriteItem{eventItem}
	putIndex := 1
	if moving {
		moveFrom := exprMustBuild(expression.NewBuilder().WithCondition(
			expression.Name("Version").Equal(expression.Value(stored.Version))))
		transactItems = append(transactItems, types.TransactWriteItem{
			Delete: &types.Delete{
				TableName: aws.String(d.tableName),
				Key: map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: gamePK(stored.EventID)},
					"SK": &types.AttributeValueMemberS{Value: gameSK(stored.Phase, stored.Round, stored.Seq)},
				},
				ConditionExpression:       moveFrom.Condition(),
				ExpressionAttributeNames:  moveFrom.Names(),
				ExpressionAttributeValues: moveFrom.Values(),
			},
		})
		putIndex = 2
	}

	putCondition := exprMustBuild(expression.NewBuilder().
		WithCondition(existingEntityVersionConditional(updated.Version)))
	if moving {
		// The destination must be free; the old row is removed in the same
		// transaction, so it cannot block its own replacement.
		putCondition = exprMustBuild(expression.NewBuilder().
			WithCondition(expression.Name("PK").AttributeNotExists()))
	}
	transactItems = append(transactItems, types.TransactWriteItem{
		Put: &types.Put{
			TableName:                 aws.String(d.tableName),
			Item:                      item,
			ConditionExpression:       putCondition.Condition(),
			ExpressionAttributeNames:  putCondition.Names(),
			ExpressionAttributeValues: putCondition.Values(),
		},
	})

	if err := d.writeGameTransaction(ctx, transactItems); err != nil {
		var canceled *types.TransactionCanceledException
		if errors.As(err, &canceled) {
			reasons := canceled.CancellationReasons
			if cancellationFailed(reasons, 0) {
				return games.NewVersionConflictError(fmt.Sprintf("Event %q schedule changed concurrently", eventID), err)
			}
			if cancellationFailed(reasons, putIndex) {
				if moving {
					return games.NewSlotOccupiedError(fmt.Sprintf("Round %d seq %d is already occupied", updated.Round, updated.Seq), err)
				}
				return games.NewVersionConflictError(fmt.Sprintf("Game %d changed concurrently", updated.GameNo), err)
			}
			return games.NewVersionConflictError(fmt.Sprintf("Game %d changed concurrently", updated.GameNo), err)
		}
		return err
	}
	return nil
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
			Or(expression.Name("Status").NotEqual(expression.Value(string(events.EventStatusFinalized)))),
	))

	// A result never changes the row's (phase, round, seq) key, so this is a
	// plain conditional put. The event precondition keeps the FINALIZED
	// barrier structural; the row Version condition rejects a concurrent
	// result edit; and the GameNo equality rejects a row regenerated at the
	// same slot, which is a different game that happens to carry the same
	// Version. Schedule edits (which may move the key) go through
	// UpdateGameScheduleChecked.
	putExpr := exprMustBuild(expression.NewBuilder().
		WithCondition(existingEntityVersionConditional(dynamoItem.Version).
			And(expression.Name("GameNo").Equal(expression.Value(dynamoItem.GameNo)))))
	transactItems := []types.TransactWriteItem{
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
	}

	_, err = d.dynamoClient.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: transactItems,
	})
	if err != nil {
		var transactionFailedErr *types.TransactionCanceledException
		if errors.As(err, &transactionFailedErr) {
			reasons := transactionFailedErr.CancellationReasons
			// Index 0 is the event precondition (FINALIZED); index 1 is the
			// game version condition.
			if cancellationFailed(reasons, 0) {
				return games.NewEventFinalizedError(fmt.Sprintf("Event %q is FINALIZED", eventID), err)
			}
			if cancellationFailed(reasons, 1) {
				return games.NewVersionConflictError(fmt.Sprintf("Game %d changed concurrently", game.GameNo), err)
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
