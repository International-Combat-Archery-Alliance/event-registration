package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/games"
	"github.com/International-Combat-Archery-Alliance/event-registration/slices"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

var _ games.Repository = &DB{}

type gameDynamo struct {
	PK            string
	SK            string
	GSI1PK        string
	GSI1SK        string
	ID            string
	Version       int
	EventID       string
	Team1ID       string
	Team2ID       string
	ScheduledTime time.Time
	Location      string
	Status        games.GameStatus

	// Results
	Team1Score   *int
	Team2Score   *int
	WinnerID     *string
	RoundResults []roundResultDynamo
	RecordedAt   *time.Time
	RecordedBy   *string
}

type roundResultDynamo struct {
	RoundNumber  int
	WinnerTeamID string
}

const (
	gameEntityName = "GAME"
)

func gamePK(eventID uuid.UUID) string {
	return eventPK(eventID)
}

func gameSK(gameID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", gameEntityName, gameID)
}

func gameGSI1PK() string {
	return gameEntityName
}

func gameGSI1SK(eventID uuid.UUID, scheduledTime time.Time) string {
	return fmt.Sprintf("%s#%s#%s", gameEntityName, eventID, scheduledTime.Format(time.RFC3339))
}

func newGameDynamo(game games.Game) gameDynamo {
	var winnerID *string
	if game.WinnerID != nil {
		wid := game.WinnerID.String()
		winnerID = &wid
	}

	return gameDynamo{
		PK:            gamePK(game.EventID),
		SK:            gameSK(game.ID),
		GSI1PK:        gameGSI1PK(),
		GSI1SK:        gameGSI1SK(game.EventID, game.ScheduledTime),
		ID:            game.ID.String(),
		Version:       game.Version,
		EventID:       game.EventID.String(),
		Team1ID:       game.Team1ID.String(),
		Team2ID:       game.Team2ID.String(),
		ScheduledTime: game.ScheduledTime,
		Location:      game.Location,
		Status:        game.Status,
		Team1Score:    game.Team1Score,
		Team2Score:    game.Team2Score,
		WinnerID:      winnerID,
		RoundResults: slices.Map(game.RoundResults, func(r games.RoundResult) roundResultDynamo {
			return roundResultDynamo{
				RoundNumber:  r.RoundNumber,
				WinnerTeamID: r.WinnerTeamID.String(),
			}
		}),
		RecordedAt: game.RecordedAt,
		RecordedBy: game.RecordedBy,
	}
}

func gameFromDynamo(d gameDynamo) games.Game {
	var winnerID *uuid.UUID
	if d.WinnerID != nil {
		wid := uuid.MustParse(*d.WinnerID)
		winnerID = &wid
	}

	return games.Game{
		ID:            uuid.MustParse(d.ID),
		Version:       d.Version,
		EventID:       uuid.MustParse(d.EventID),
		Team1ID:       uuid.MustParse(d.Team1ID),
		Team2ID:       uuid.MustParse(d.Team2ID),
		ScheduledTime: d.ScheduledTime,
		Location:      d.Location,
		Status:        d.Status,
		Team1Score:    d.Team1Score,
		Team2Score:    d.Team2Score,
		WinnerID:      winnerID,
		RoundResults: slices.Map(d.RoundResults, func(r roundResultDynamo) games.RoundResult {
			return games.RoundResult{
				RoundNumber:  r.RoundNumber,
				WinnerTeamID: uuid.MustParse(r.WinnerTeamID),
			}
		}),
		RecordedAt: d.RecordedAt,
		RecordedBy: d.RecordedBy,
	}
}

func (d *DB) GetGame(ctx context.Context, eventID uuid.UUID, gameID uuid.UUID) (games.Game, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	resp, err := d.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: gamePK(eventID)},
			"SK": &types.AttributeValueMemberS{Value: gameSK(gameID)},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return games.Game{}, games.NewTimeoutError("GetGame timed out")
		}
		return games.Game{}, games.NewFailedToFetchError(fmt.Sprintf("Failed to fetch game with ID %q", gameID), err)
	}

	if len(resp.Item) == 0 {
		return games.Game{}, games.NewGameDoesNotExistError(fmt.Sprintf("Game with ID %q not found", gameID), nil)
	}

	var game gameDynamo
	err = attributevalue.UnmarshalMap(resp.Item, &game)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal game from DB: %s", err))
	}
	return gameFromDynamo(game), nil
}

func (d *DB) GetGamesForEvent(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (games.GetGamesResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	keyCond := expression.Key("PK").Equal(expression.Value(gamePK(eventID))).
		And(expression.Key("SK").BeginsWith(gameEntityName))

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build dynamo key expression: %s", err))
	}

	var startKey map[string]types.AttributeValue
	if cursor != nil {
		startKey, err = cursorToLastEval(*cursor)
		if err != nil {
			return games.GetGamesResponse{}, games.NewInvalidCursorError("Invalid cursor", err)
		}
	}

	result, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.tableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     aws.Int32(limit + 1),
		ExclusiveStartKey:         startKey,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return games.GetGamesResponse{}, games.NewTimeoutError("GetGamesForEvent timed out")
		}
		return games.GetGamesResponse{}, games.NewFailedToFetchError("Failed to fetch games from dynamo", err)
	}

	var dynamoItems []gameDynamo
	err = attributevalue.UnmarshalListOfMaps(result.Items, &dynamoItems)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal dynamo games: %s", err))
	}

	hasNextPage := len(dynamoItems) > int(limit)

	var newCursor *string
	if hasNextPage && len(result.LastEvaluatedKey) > 0 {
		lastItemGivenToUser := result.Items[len(result.Items)-2]
		lastItemKey := getKeyFromItem(result.LastEvaluatedKey, lastItemGivenToUser)
		c, err := lastEvalKeyToCursor(lastItemKey)
		if err != nil {
			panic(fmt.Sprintf("failed to make cursor from lastEvalKey: %s", err))
		}
		newCursor = &c
	}

	return games.GetGamesResponse{
		Data: slices.Map(dynamoItems, func(v gameDynamo) games.Game {
			return gameFromDynamo(v)
		})[:min(int(limit), len(dynamoItems))],
		Cursor:      newCursor,
		HasNextPage: hasNextPage,
	}, nil
}

func (d *DB) CreateGame(ctx context.Context, game games.Game) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoGame := newGameDynamo(game)

	item, err := attributevalue.MarshalMap(dynamoGame)
	if err != nil {
		return games.NewFailedToTranslateToDBModelError("Failed to convert Game to gameDynamo", err)
	}

	expr := exprMustBuild(expression.NewBuilder().
		WithCondition(newEntityVersionConditional(dynamoGame.Version)))

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
			return games.NewGameAlreadyExistsError(fmt.Sprintf("Game with ID %q already exists", game.ID), err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return games.NewTimeoutError("CreateGame timed out")
		} else {
			return games.NewFailedToWriteError("Failed PutItem call", err)
		}
	}

	return nil
}

func (d *DB) UpdateGame(ctx context.Context, game games.Game) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoGame := newGameDynamo(game)

	item, err := attributevalue.MarshalMap(dynamoGame)
	if err != nil {
		return games.NewFailedToTranslateToDBModelError("Failed to convert Game to gameDynamo", err)
	}

	expr := exprMustBuild(expression.NewBuilder().
		WithCondition(existingEntityVersionConditional(dynamoGame.Version)))

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

func (d *DB) DeleteGame(ctx context.Context, eventID uuid.UUID, gameID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	_, err := d.dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: gamePK(eventID)},
			"SK": &types.AttributeValueMemberS{Value: gameSK(gameID)},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return games.NewTimeoutError("DeleteGame timed out")
		}
		return games.NewFailedToWriteError("Failed DeleteItem call", err)
	}

	return nil
}

func (d *DB) RecordResult(ctx context.Context, eventID uuid.UUID, gameID uuid.UUID, result games.GameResult, recordedBy string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Get current game
	game, err := d.GetGame(ctx, eventID, gameID)
	if err != nil {
		return err
	}

	// Update game with result
	game.Version++
	game.Status = games.STATUS_COMPLETED
	game.Team1Score = &result.Team1Score
	game.Team2Score = &result.Team2Score
	game.WinnerID = &result.WinnerID
	game.RoundResults = result.RoundResults
	now := time.Now().UTC()
	game.RecordedAt = &now
	game.RecordedBy = &recordedBy

	// Save updated game
	return d.UpdateGame(ctx, game)
}
