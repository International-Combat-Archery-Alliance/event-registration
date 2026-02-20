package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/slices"
	"github.com/International-Combat-Archery-Alliance/event-registration/standings"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"
)

var _ standings.Repository = &DB{}

type standingDynamo struct {
	PK            string
	SK            string
	EventID       string
	TeamID        string
	TeamName      string
	Wins          int
	Losses        int
	PointsFor     int
	PointsAgainst int
	GamesPlayed   int
	WinPercentage float64
	UpdatedAt     time.Time
}

const (
	standingEntityName = "STANDING"
)

func standingPK(eventID uuid.UUID) string {
	return eventPK(eventID)
}

func standingSK(teamID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", standingEntityName, teamID)
}

func newStandingDynamo(standing standings.Standing) standingDynamo {
	return standingDynamo{
		PK:            standingPK(standing.EventID),
		SK:            standingSK(standing.TeamID),
		EventID:       standing.EventID.String(),
		TeamID:        standing.TeamID.String(),
		TeamName:      standing.TeamName,
		Wins:          standing.Wins,
		Losses:        standing.Losses,
		PointsFor:     standing.PointsFor,
		PointsAgainst: standing.PointsAgainst,
		GamesPlayed:   standing.GamesPlayed,
		WinPercentage: standing.WinPercentage,
		UpdatedAt:     time.Now().UTC(),
	}
}

func standingFromDynamo(d standingDynamo) standings.Standing {
	return standings.Standing{
		EventID:       uuid.MustParse(d.EventID),
		TeamID:        uuid.MustParse(d.TeamID),
		TeamName:      d.TeamName,
		Wins:          d.Wins,
		Losses:        d.Losses,
		PointsFor:     d.PointsFor,
		PointsAgainst: d.PointsAgainst,
		GamesPlayed:   d.GamesPlayed,
		WinPercentage: d.WinPercentage,
	}
}

func (d *DB) GetStandingsForEvent(ctx context.Context, eventID uuid.UUID) (standings.GetStandingsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	keyCond := expression.Key("PK").Equal(expression.Value(standingPK(eventID))).
		And(expression.Key("SK").BeginsWith(standingEntityName))

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build dynamo key expression: %s", err))
	}

	result, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.tableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return standings.GetStandingsResponse{}, standings.NewTimeoutError("GetStandingsForEvent timed out")
		}
		return standings.GetStandingsResponse{}, standings.NewFailedToFetchError("Failed to fetch standings from dynamo", err)
	}

	var dynamoItems []standingDynamo
	err = attributevalue.UnmarshalListOfMaps(result.Items, &dynamoItems)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal dynamo standings: %s", err))
	}

	return standings.GetStandingsResponse{
		Data: slices.Map(dynamoItems, func(v standingDynamo) standings.Standing {
			return standingFromDynamo(v)
		}),
	}, nil
}

func (d *DB) UpdateStandings(ctx context.Context, eventID uuid.UUID, teamID uuid.UUID, standing standings.Standing) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoStanding := newStandingDynamo(standing)

	item, err := attributevalue.MarshalMap(dynamoStanding)
	if err != nil {
		return standings.NewFailedToTranslateToDBModelError("Failed to convert Standing to standingDynamo", err)
	}

	// Use PutItem with no condition to upsert the standing
	_, err = d.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(d.tableName),
		Item:      item,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return standings.NewTimeoutError("UpdateStandings timed out")
		}
		return standings.NewFailedToWriteError("Failed PutItem call", err)
	}

	return nil
}

func (d *DB) RecalculateStandings(ctx context.Context, eventID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	// Get all games for the event
	gamesResp, err := d.GetGamesForEvent(ctx, eventID, 1000, nil)
	if err != nil {
		return err
	}

	// Get all teams for the event to get team names
	teamsResp, err := d.GetTeamsForEvent(ctx, eventID, 1000, nil)
	if err != nil {
		return err
	}

	// Create a map of team ID to team name
	teamNames := make(map[uuid.UUID]string)
	for _, team := range teamsResp.Data {
		teamNames[team.ID] = team.Name
	}

	// Convert games to GameInfo
	gameInfos := make([]standings.GameInfo, 0, len(gamesResp.Data))
	for _, game := range gamesResp.Data {
		gameInfos = append(gameInfos, standings.GameInfo{
			Team1ID:    game.Team1ID,
			Team1Name:  teamNames[game.Team1ID],
			Team2ID:    game.Team2ID,
			Team2Name:  teamNames[game.Team2ID],
			Status:     fmt.Sprintf("%d", game.Status),
			Team1Score: game.Team1Score,
			Team2Score: game.Team2Score,
			WinnerID:   game.WinnerID,
		})
	}

	// Calculate standings
	calculatedStandings := standings.CalculateStandings(gameInfos)

	// Store each standing
	for _, standing := range calculatedStandings.Data {
		// Set EventID from the parameter
		standing.EventID = eventID
		err := d.UpdateStandings(ctx, eventID, standing.TeamID, standing)
		if err != nil {
			return err
		}
	}

	return nil
}
