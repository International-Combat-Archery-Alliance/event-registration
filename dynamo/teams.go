package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/International-Combat-Archery-Alliance/event-registration/slices"
	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

var _ teams.TeamRepository = &DB{}
var _ teams.EventTeamRepository = &DB{}

// Global Team DynamoDB model
type globalTeamDynamo struct {
	PK        string
	SK        string
	ID        string
	Name      string
	CreatedAt time.Time
}

const (
	globalTeamEntityName = "TEAM"
)

func globalTeamPK(teamID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", globalTeamEntityName, teamID)
}

func globalTeamSK(teamID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", globalTeamEntityName, teamID)
}

func newGlobalTeamDynamo(team teams.Team) globalTeamDynamo {
	return globalTeamDynamo{
		PK:        globalTeamPK(team.ID),
		SK:        globalTeamSK(team.ID),
		ID:        team.ID.String(),
		Name:      team.Name,
		CreatedAt: team.CreatedAt,
	}
}

func globalTeamFromDynamo(d globalTeamDynamo) teams.Team {
	return teams.Team{
		ID:        uuid.MustParse(d.ID),
		Name:      d.Name,
		CreatedAt: d.CreatedAt,
	}
}

// Event Team DynamoDB model
type eventTeamDynamo struct {
	PK             string
	SK             string
	GSI1PK         string
	GSI1SK         string
	ID             string
	Version        int
	EventID        string
	TeamID         string
	Name           string
	SourceType     teams.TeamSourceType
	RegistrationID *string
	Players        []eventTeamPlayerDynamo
	CreatedAt      time.Time
}

type eventTeamPlayerDynamo struct {
	FirstName      string
	LastName       string
	Email          *string
	SourceType     teams.PlayerSourceType
	RegistrationID string
	AssignedAt     time.Time
}

const (
	eventTeamEntityName = "EVENTTEAM"
)

func eventTeamPK(eventID uuid.UUID) string {
	return eventPK(eventID)
}

func eventTeamSK(eventTeamID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", eventTeamEntityName, eventTeamID)
}

func eventTeamGSI1PK(teamID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", globalTeamEntityName, teamID)
}

func eventTeamGSI1SK(eventID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", eventEntityName, eventID)
}

func newEventTeamDynamo(eventTeam teams.EventTeam) eventTeamDynamo {
	var regID *string
	if eventTeam.RegistrationID != nil {
		rid := eventTeam.RegistrationID.String()
		regID = &rid
	}

	return eventTeamDynamo{
		PK:             eventTeamPK(eventTeam.EventID),
		SK:             eventTeamSK(eventTeam.ID),
		GSI1PK:         eventTeamGSI1PK(eventTeam.TeamID),
		GSI1SK:         eventTeamGSI1SK(eventTeam.EventID),
		ID:             eventTeam.ID.String(),
		Version:        eventTeam.Version,
		EventID:        eventTeam.EventID.String(),
		TeamID:         eventTeam.TeamID.String(),
		Name:           eventTeam.Name,
		SourceType:     eventTeam.SourceType,
		RegistrationID: regID,
		Players: slices.Map(eventTeam.Players, func(p teams.TeamPlayer) eventTeamPlayerDynamo {
			return eventTeamPlayerDynamo{
				FirstName:      p.PlayerInfo.FirstName,
				LastName:       p.PlayerInfo.LastName,
				Email:          p.PlayerInfo.Email,
				SourceType:     p.SourceType,
				RegistrationID: p.RegistrationID.String(),
				AssignedAt:     p.AssignedAt,
			}
		}),
		CreatedAt: eventTeam.CreatedAt,
	}
}

func eventTeamFromDynamo(d eventTeamDynamo) teams.EventTeam {
	var regID *uuid.UUID
	if d.RegistrationID != nil {
		rid := uuid.MustParse(*d.RegistrationID)
		regID = &rid
	}

	return teams.EventTeam{
		ID:             uuid.MustParse(d.ID),
		Version:        d.Version,
		EventID:        uuid.MustParse(d.EventID),
		TeamID:         uuid.MustParse(d.TeamID),
		Name:           d.Name,
		SourceType:     d.SourceType,
		RegistrationID: regID,
		Players: slices.Map(d.Players, func(p eventTeamPlayerDynamo) teams.TeamPlayer {
			return teams.TeamPlayer{
				PlayerInfo: registration.PlayerInfo{
					FirstName: p.FirstName,
					LastName:  p.LastName,
					Email:     p.Email,
				},
				SourceType:     p.SourceType,
				RegistrationID: uuid.MustParse(p.RegistrationID),
				AssignedAt:     p.AssignedAt,
			}
		}),
		CreatedAt: d.CreatedAt,
	}
}

// ==================== Global Team Repository Implementation ====================

func (d *DB) GetTeam(ctx context.Context, teamID uuid.UUID) (teams.Team, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	resp, err := d.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: globalTeamPK(teamID)},
			"SK": &types.AttributeValueMemberS{Value: globalTeamSK(teamID)},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return teams.Team{}, teams.NewTimeoutError("GetTeam timed out")
		}
		return teams.Team{}, teams.NewFailedToFetchError(fmt.Sprintf("Failed to fetch team with ID %q", teamID), err)
	}

	if len(resp.Item) == 0 {
		return teams.Team{}, teams.NewTeamDoesNotExistError(fmt.Sprintf("Team with ID %q not found", teamID), nil)
	}

	var team globalTeamDynamo
	err = attributevalue.UnmarshalMap(resp.Item, &team)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal team from DB: %s", err))
	}
	return globalTeamFromDynamo(team), nil
}

func (d *DB) GetTeams(ctx context.Context, limit int32, cursor *string) (teams.GetTeamsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	keyCond := expression.Key("PK").BeginsWith(globalTeamEntityName)

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build dynamo key expression: %s", err))
	}

	var startKey map[string]types.AttributeValue
	if cursor != nil {
		startKey, err = cursorToLastEval(*cursor)
		if err != nil {
			return teams.GetTeamsResponse{}, teams.NewInvalidCursorError("Invalid cursor", err)
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
			return teams.GetTeamsResponse{}, teams.NewTimeoutError("GetTeams timed out")
		}
		return teams.GetTeamsResponse{}, teams.NewFailedToFetchError("Failed to fetch teams from dynamo", err)
	}

	var dynamoItems []globalTeamDynamo
	err = attributevalue.UnmarshalListOfMaps(result.Items, &dynamoItems)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal dynamo teams: %s", err))
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

	return teams.GetTeamsResponse{
		Data: slices.Map(dynamoItems, func(v globalTeamDynamo) teams.Team {
			return globalTeamFromDynamo(v)
		})[:min(int(limit), len(dynamoItems))],
		Cursor:      newCursor,
		HasNextPage: hasNextPage,
	}, nil
}

func (d *DB) CreateTeam(ctx context.Context, team teams.Team) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoTeam := newGlobalTeamDynamo(team)

	item, err := attributevalue.MarshalMap(dynamoTeam)
	if err != nil {
		return teams.NewFailedToTranslateToDBModelError("Failed to convert Team to globalTeamDynamo", err)
	}

	_, err = d.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(d.tableName),
		Item:      item,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return teams.NewTimeoutError("CreateTeam timed out")
		}
		return teams.NewFailedToWriteError("Failed PutItem call", err)
	}

	return nil
}

func (d *DB) UpdateTeam(ctx context.Context, team teams.Team) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoTeam := newGlobalTeamDynamo(team)

	item, err := attributevalue.MarshalMap(dynamoTeam)
	if err != nil {
		return teams.NewFailedToTranslateToDBModelError("Failed to convert Team to globalTeamDynamo", err)
	}

	_, err = d.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(d.tableName),
		Item:      item,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return teams.NewTimeoutError("UpdateTeam timed out")
		}
		return teams.NewFailedToWriteError("Failed PutItem call", err)
	}

	return nil
}

func (d *DB) DeleteTeam(ctx context.Context, teamID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	_, err := d.dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: globalTeamPK(teamID)},
			"SK": &types.AttributeValueMemberS{Value: globalTeamSK(teamID)},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return teams.NewTimeoutError("DeleteTeam timed out")
		}
		return teams.NewFailedToWriteError("Failed DeleteItem call", err)
	}

	return nil
}

// ==================== Event Team Repository Implementation ====================

func (d *DB) GetEventTeam(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID) (teams.EventTeam, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	resp, err := d.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: eventTeamPK(eventID)},
			"SK": &types.AttributeValueMemberS{Value: eventTeamSK(eventTeamID)},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return teams.EventTeam{}, teams.NewTimeoutError("GetEventTeam timed out")
		}
		return teams.EventTeam{}, teams.NewFailedToFetchError(fmt.Sprintf("Failed to fetch event team with ID %q", eventTeamID), err)
	}

	if len(resp.Item) == 0 {
		return teams.EventTeam{}, teams.NewTeamDoesNotExistError(fmt.Sprintf("Event team with ID %q not found", eventTeamID), nil)
	}

	var eventTeam eventTeamDynamo
	err = attributevalue.UnmarshalMap(resp.Item, &eventTeam)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal event team from DB: %s", err))
	}
	return eventTeamFromDynamo(eventTeam), nil
}

func (d *DB) GetEventTeamsForEvent(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (teams.GetEventTeamsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	keyCond := expression.Key("PK").Equal(expression.Value(eventTeamPK(eventID))).
		And(expression.Key("SK").BeginsWith(eventTeamEntityName))

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build dynamo key expression: %s", err))
	}

	var startKey map[string]types.AttributeValue
	if cursor != nil {
		startKey, err = cursorToLastEval(*cursor)
		if err != nil {
			return teams.GetEventTeamsResponse{}, teams.NewInvalidCursorError("Invalid cursor", err)
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
			return teams.GetEventTeamsResponse{}, teams.NewTimeoutError("GetEventTeamsForEvent timed out")
		}
		return teams.GetEventTeamsResponse{}, teams.NewFailedToFetchError("Failed to fetch event teams from dynamo", err)
	}

	var dynamoItems []eventTeamDynamo
	err = attributevalue.UnmarshalListOfMaps(result.Items, &dynamoItems)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal dynamo event teams: %s", err))
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

	return teams.GetEventTeamsResponse{
		Data: slices.Map(dynamoItems, func(v eventTeamDynamo) teams.EventTeam {
			return eventTeamFromDynamo(v)
		})[:min(int(limit), len(dynamoItems))],
		Cursor:      newCursor,
		HasNextPage: hasNextPage,
	}, nil
}

func (d *DB) GetEventTeamsByTeam(ctx context.Context, teamID uuid.UUID, limit int32, cursor *string) (teams.GetEventTeamsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	keyCond := expression.Key("GSI1PK").Equal(expression.Value(eventTeamGSI1PK(teamID)))

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build dynamo key expression: %s", err))
	}

	var startKey map[string]types.AttributeValue
	if cursor != nil {
		startKey, err = cursorToLastEval(*cursor)
		if err != nil {
			return teams.GetEventTeamsResponse{}, teams.NewInvalidCursorError("Invalid cursor", err)
		}
	}

	result, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
		IndexName:                 aws.String(gsi1),
		TableName:                 aws.String(d.tableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		Limit:                     aws.Int32(limit + 1),
		ExclusiveStartKey:         startKey,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return teams.GetEventTeamsResponse{}, teams.NewTimeoutError("GetEventTeamsByTeam timed out")
		}
		return teams.GetEventTeamsResponse{}, teams.NewFailedToFetchError("Failed to fetch event teams from dynamo", err)
	}

	var dynamoItems []eventTeamDynamo
	err = attributevalue.UnmarshalListOfMaps(result.Items, &dynamoItems)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal dynamo event teams: %s", err))
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

	return teams.GetEventTeamsResponse{
		Data: slices.Map(dynamoItems, func(v eventTeamDynamo) teams.EventTeam {
			return eventTeamFromDynamo(v)
		})[:min(int(limit), len(dynamoItems))],
		Cursor:      newCursor,
		HasNextPage: hasNextPage,
	}, nil
}

func (d *DB) CreateEventTeam(ctx context.Context, eventTeam teams.EventTeam) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoEventTeam := newEventTeamDynamo(eventTeam)

	item, err := attributevalue.MarshalMap(dynamoEventTeam)
	if err != nil {
		return teams.NewFailedToTranslateToDBModelError("Failed to convert EventTeam to eventTeamDynamo", err)
	}

	expr := exprMustBuild(expression.NewBuilder().
		WithCondition(newEntityVersionConditional(dynamoEventTeam.Version)))

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
			return teams.NewTeamAlreadyExistsError(fmt.Sprintf("Event team with ID %q already exists", eventTeam.ID), err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return teams.NewTimeoutError("CreateEventTeam timed out")
		} else {
			return teams.NewFailedToWriteError("Failed PutItem call", err)
		}
	}

	return nil
}

func (d *DB) UpdateEventTeam(ctx context.Context, eventTeam teams.EventTeam) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoEventTeam := newEventTeamDynamo(eventTeam)

	item, err := attributevalue.MarshalMap(dynamoEventTeam)
	if err != nil {
		return teams.NewFailedToTranslateToDBModelError("Failed to convert EventTeam to eventTeamDynamo", err)
	}

	expr := exprMustBuild(expression.NewBuilder().
		WithCondition(existingEntityVersionConditional(dynamoEventTeam.Version)))

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
			return teams.NewTeamDoesNotExistError(fmt.Sprintf("Event team with ID %q does not exist", eventTeam.ID), err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return teams.NewTimeoutError("UpdateEventTeam timed out")
		} else {
			return teams.NewFailedToWriteError("Failed PutItem call", err)
		}
	}

	return nil
}

func (d *DB) DeleteEventTeam(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	_, err := d.dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: eventTeamPK(eventID)},
			"SK": &types.AttributeValueMemberS{Value: eventTeamSK(eventTeamID)},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return teams.NewTimeoutError("DeleteEventTeam timed out")
		}
		return teams.NewFailedToWriteError("Failed DeleteItem call", err)
	}

	return nil
}

func (d *DB) AddPlayerToEventTeam(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID, player teams.TeamPlayer) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Get current event team
	eventTeam, err := d.GetEventTeam(ctx, eventID, eventTeamID)
	if err != nil {
		return err
	}

	// Add player
	eventTeam.Players = append(eventTeam.Players, player)
	eventTeam.Version++

	// Save updated event team
	return d.UpdateEventTeam(ctx, eventTeam)
}

func (d *DB) RemovePlayerFromEventTeam(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID, registrationID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Get current event team
	eventTeam, err := d.GetEventTeam(ctx, eventID, eventTeamID)
	if err != nil {
		return err
	}

	// Remove player
	for i, player := range eventTeam.Players {
		if player.RegistrationID == registrationID {
			eventTeam.Players = append(eventTeam.Players[:i], eventTeam.Players[i+1:]...)
			break
		}
	}
	eventTeam.Version++

	// Save updated event team
	return d.UpdateEventTeam(ctx, eventTeam)
}

func (d *DB) HasGames(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Query games where this event team is either team1 or team2
	// We'll use a scan with filter since we need to check both Team1ID and Team2ID

	keyCond := expression.Key("PK").Equal(expression.Value(eventPK(eventID))).
		And(expression.Key("SK").BeginsWith(gameEntityName))

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return false, err
	}

	result, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.tableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return false, teams.NewTimeoutError("HasGames timed out")
		}
		return false, teams.NewFailedToFetchError("Failed to fetch games from dynamo", err)
	}

	var games []gameDynamo
	err = attributevalue.UnmarshalListOfMaps(result.Items, &games)
	if err != nil {
		return false, err
	}

	eventTeamIDStr := eventTeamID.String()
	for _, game := range games {
		if game.Team1ID == eventTeamIDStr || game.Team2ID == eventTeamIDStr {
			return true, nil
		}
	}

	return false, nil
}

func (d *DB) IsIndividualAssigned(ctx context.Context, eventID uuid.UUID, registrationID uuid.UUID) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Get all event teams for the event
	eventTeams, err := d.GetEventTeamsForEvent(ctx, eventID, 1000, nil)
	if err != nil {
		return false, err
	}

	regIDStr := registrationID.String()
	for _, eventTeam := range eventTeams.Data {
		for _, player := range eventTeam.Players {
			if player.RegistrationID.String() == regIDStr {
				return true, nil
			}
		}
	}

	return false, nil
}
