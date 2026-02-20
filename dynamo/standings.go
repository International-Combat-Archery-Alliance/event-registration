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
	"github.com/google/uuid"
)

var _ games.Repository = &DB{}

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

func newStandingDynamo(standing games.Standing) standingDynamo {
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

func standingFromDynamo(d standingDynamo) games.Standing {
	return games.Standing{
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

func (d *DB) GetStandingsForEvent(ctx context.Context, eventID uuid.UUID) (games.GetStandingsResponse, error) {
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
			return games.GetStandingsResponse{}, games.NewTimeoutError("GetStandingsForEvent timed out")
		}
		return games.GetStandingsResponse{}, games.NewFailedToFetchError("Failed to fetch standings from dynamo", err)
	}

	var dynamoItems []standingDynamo
	err = attributevalue.UnmarshalListOfMaps(result.Items, &dynamoItems)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal dynamo standings: %s", err))
	}

	return games.GetStandingsResponse{
		Data: slices.Map(dynamoItems, func(v standingDynamo) games.Standing {
			return standingFromDynamo(v)
		}),
	}, nil
}
