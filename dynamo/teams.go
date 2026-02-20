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

var _ teams.Repository = &DB{}

type teamDynamo struct {
	PK             string
	SK             string
	GSI1PK         string
	GSI1SK         string
	ID             string
	Version        int
	EventID        string
	Name           string
	SourceType     teams.TeamSourceType
	RegistrationID *string
	Players        []teamPlayerDynamo
	CreatedAt      time.Time
}

type teamPlayerDynamo struct {
	FirstName      string
	LastName       string
	Email          *string
	SourceType     teams.PlayerSourceType
	RegistrationID string
	AssignedAt     time.Time
}

const (
	teamEntityName = "TEAM"
)

func teamPK(eventID uuid.UUID) string {
	return eventPK(eventID)
}

func teamSK(teamID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", teamEntityName, teamID)
}

func teamGSI1PK() string {
	return teamEntityName
}

func teamGSI1SK(eventID uuid.UUID, name string) string {
	return fmt.Sprintf("%s#%s#%s", teamEntityName, eventID, name)
}

func newTeamDynamo(team teams.Team) teamDynamo {
	var regID *string
	if team.RegistrationID != nil {
		rid := team.RegistrationID.String()
		regID = &rid
	}

	return teamDynamo{
		PK:             teamPK(team.EventID),
		SK:             teamSK(team.ID),
		GSI1PK:         teamGSI1PK(),
		GSI1SK:         teamGSI1SK(team.EventID, team.Name),
		ID:             team.ID.String(),
		Version:        team.Version,
		EventID:        team.EventID.String(),
		Name:           team.Name,
		SourceType:     team.SourceType,
		RegistrationID: regID,
		Players: slices.Map(team.Players, func(p teams.TeamPlayer) teamPlayerDynamo {
			return teamPlayerDynamo{
				FirstName:      p.PlayerInfo.FirstName,
				LastName:       p.PlayerInfo.LastName,
				Email:          p.PlayerInfo.Email,
				SourceType:     p.SourceType,
				RegistrationID: p.RegistrationID.String(),
				AssignedAt:     p.AssignedAt,
			}
		}),
		CreatedAt: team.CreatedAt,
	}
}

func teamFromDynamo(d teamDynamo) teams.Team {
	var regID *uuid.UUID
	if d.RegistrationID != nil {
		rid := uuid.MustParse(*d.RegistrationID)
		regID = &rid
	}

	return teams.Team{
		ID:             uuid.MustParse(d.ID),
		Version:        d.Version,
		EventID:        uuid.MustParse(d.EventID),
		Name:           d.Name,
		SourceType:     d.SourceType,
		RegistrationID: regID,
		Players: slices.Map(d.Players, func(p teamPlayerDynamo) teams.TeamPlayer {
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

func (d *DB) GetTeam(ctx context.Context, eventID uuid.UUID, teamID uuid.UUID) (teams.Team, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	resp, err := d.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: teamPK(eventID)},
			"SK": &types.AttributeValueMemberS{Value: teamSK(teamID)},
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

	var team teamDynamo
	err = attributevalue.UnmarshalMap(resp.Item, &team)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal team from DB: %s", err))
	}
	return teamFromDynamo(team), nil
}

func (d *DB) GetTeamsForEvent(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (teams.GetTeamsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	keyCond := expression.Key("PK").Equal(expression.Value(teamPK(eventID))).
		And(expression.Key("SK").BeginsWith(teamEntityName))

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
			return teams.GetTeamsResponse{}, teams.NewTimeoutError("GetTeamsForEvent timed out")
		}
		return teams.GetTeamsResponse{}, teams.NewFailedToFetchError("Failed to fetch teams from dynamo", err)
	}

	var dynamoItems []teamDynamo
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
		Data: slices.Map(dynamoItems, func(v teamDynamo) teams.Team {
			return teamFromDynamo(v)
		})[:min(int(limit), len(dynamoItems))],
		Cursor:      newCursor,
		HasNextPage: hasNextPage,
	}, nil
}

func (d *DB) CreateTeam(ctx context.Context, team teams.Team) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoTeam := newTeamDynamo(team)

	item, err := attributevalue.MarshalMap(dynamoTeam)
	if err != nil {
		return teams.NewFailedToTranslateToDBModelError("Failed to convert Team to teamDynamo", err)
	}

	expr := exprMustBuild(expression.NewBuilder().
		WithCondition(newEntityVersionConditional(dynamoTeam.Version)))

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
			return teams.NewTeamAlreadyExistsError(fmt.Sprintf("Team with ID %q already exists", team.ID), err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return teams.NewTimeoutError("CreateTeam timed out")
		} else {
			return teams.NewFailedToWriteError("Failed PutItem call", err)
		}
	}

	return nil
}

func (d *DB) UpdateTeam(ctx context.Context, team teams.Team) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoTeam := newTeamDynamo(team)

	item, err := attributevalue.MarshalMap(dynamoTeam)
	if err != nil {
		return teams.NewFailedToTranslateToDBModelError("Failed to convert Team to teamDynamo", err)
	}

	expr := exprMustBuild(expression.NewBuilder().
		WithCondition(existingEntityVersionConditional(dynamoTeam.Version)))

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
			return teams.NewTeamDoesNotExistError(fmt.Sprintf("Team with ID %q does not exist", team.ID), err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return teams.NewTimeoutError("UpdateTeam timed out")
		} else {
			return teams.NewFailedToWriteError("Failed PutItem call", err)
		}
	}

	return nil
}

func (d *DB) DeleteTeam(ctx context.Context, eventID uuid.UUID, teamID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	_, err := d.dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: teamPK(eventID)},
			"SK": &types.AttributeValueMemberS{Value: teamSK(teamID)},
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

func (d *DB) AddPlayerToTeam(ctx context.Context, eventID uuid.UUID, teamID uuid.UUID, player teams.TeamPlayer) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Get current team
	team, err := d.GetTeam(ctx, eventID, teamID)
	if err != nil {
		return err
	}

	// Add player
	team.Players = append(team.Players, player)
	team.Version++

	// Save updated team
	return d.UpdateTeam(ctx, team)
}

func (d *DB) RemovePlayerFromTeam(ctx context.Context, eventID uuid.UUID, teamID uuid.UUID, registrationID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Get current team
	team, err := d.GetTeam(ctx, eventID, teamID)
	if err != nil {
		return err
	}

	// Remove player
	for i, player := range team.Players {
		if player.RegistrationID == registrationID {
			team.Players = append(team.Players[:i], team.Players[i+1:]...)
			break
		}
	}
	team.Version++

	// Save updated team
	return d.UpdateTeam(ctx, team)
}

func (d *DB) HasGames(ctx context.Context, eventID uuid.UUID, teamID uuid.UUID) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Query games where this team is either team1 or team2
	// We'll use a scan with filter since we need to check both Team1ID and Team2ID
	// In production, you might want to add GSIs for this

	// For now, let's query all games for the event and filter
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

	teamIDStr := teamID.String()
	for _, game := range games {
		if game.Team1ID == teamIDStr || game.Team2ID == teamIDStr {
			return true, nil
		}
	}

	return false, nil
}

func (d *DB) IsIndividualAssigned(ctx context.Context, eventID uuid.UUID, registrationID uuid.UUID) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Get all teams for the event
	teams, err := d.GetTeamsForEvent(ctx, eventID, 1000, nil)
	if err != nil {
		return false, err
	}

	regIDStr := registrationID.String()
	for _, team := range teams.Data {
		for _, player := range team.Players {
			if player.RegistrationID.String() == regIDStr {
				return true, nil
			}
		}
	}

	return false, nil
}
