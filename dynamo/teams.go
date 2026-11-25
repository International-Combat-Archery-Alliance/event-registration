package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

var _ teams.Repository = &DB{}

// teamDynamo carries GSI1PK/SK: TEAM masters are (with EVENT masters) the
// only items allowed GSI attributes (RFC-0001/0002 GSI rule). The
// TEAM_NAME# reservation rows below carry none.
type teamDynamo struct {
	PK              string
	SK              string
	GSI1PK          string
	GSI1SK          string
	ID              string
	Version         int
	Name            string
	NormalizedName  string
	HomeCity        string
	Status          string
	CaptainPlayerID *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type teamNameDynamo struct {
	PK        string
	SK        string
	TeamID    string
	CreatedAt time.Time
}

const (
	teamEntityName            = "TEAM"
	teamNameReservationEntity = "TEAM_NAME"
)

func teamPK(id uuid.UUID) string {
	return fmt.Sprintf("%s#%s", teamEntityName, id)
}

func teamNamePK(normalized string) string {
	return fmt.Sprintf("%s#%s", teamNameReservationEntity, normalized)
}

func newTeamDynamo(team teams.Team) teamDynamo {
	normalized := teams.NormalizeTeamName(team.Name)
	var captainID *string
	if team.CaptainPlayerID != nil {
		s := team.CaptainPlayerID.String()
		captainID = &s
	}
	return teamDynamo{
		PK:              teamPK(team.ID),
		SK:              teamPK(team.ID),
		GSI1PK:          teamEntityName,
		GSI1SK:          fmt.Sprintf("NAME#%s#%s", normalized, team.ID),
		ID:              team.ID.String(),
		Version:         team.Version,
		Name:            team.Name,
		NormalizedName:  normalized,
		HomeCity:        team.HomeCity,
		Status:          string(team.Status),
		CaptainPlayerID: captainID,
		CreatedAt:       team.CreatedAt.UTC(),
		UpdatedAt:       team.UpdatedAt.UTC(),
	}
}

func teamFromTeamDynamo(item teamDynamo) (teams.Team, error) {
	id, err := uuid.Parse(item.ID)
	if err != nil {
		return teams.Team{}, err
	}
	var captainID *uuid.UUID
	if item.CaptainPlayerID != nil {
		parsed, err := uuid.Parse(*item.CaptainPlayerID)
		if err != nil {
			return teams.Team{}, err
		}
		captainID = &parsed
	}
	return teams.Team{
		ID:              id,
		Version:         item.Version,
		Name:            item.Name,
		HomeCity:        item.HomeCity,
		Status:          teams.TeamStatus(item.Status),
		CaptainPlayerID: captainID,
		CreatedAt:       item.CreatedAt,
		UpdatedAt:       item.UpdatedAt,
	}, nil
}

func (d *DB) CreateTeam(ctx context.Context, team teams.Team) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoTeam := newTeamDynamo(team)
	teamItem, err := attributevalue.MarshalMap(dynamoTeam)
	if err != nil {
		return teams.NewFailedToTranslateToDBModelError("Failed to convert Team to teamDynamo", err)
	}
	teamExpr := exprMustBuild(expression.NewBuilder().
		WithCondition(newEntityVersionConditional(dynamoTeam.Version)))

	reservation := teamNameDynamo{
		PK:        teamNamePK(dynamoTeam.NormalizedName),
		SK:        teamNamePK(dynamoTeam.NormalizedName),
		TeamID:    dynamoTeam.ID,
		CreatedAt: dynamoTeam.CreatedAt,
	}
	reservationItem, err := attributevalue.MarshalMap(reservation)
	if err != nil {
		return teams.NewFailedToTranslateToDBModelError("Failed to convert team name reservation to dynamo model", err)
	}
	reservationExpr := exprMustBuild(expression.NewBuilder().
		WithCondition(expression.Name("PK").AttributeNotExists()))

	_, err = d.dynamoClient.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:                 aws.String(d.tableName),
					Item:                      teamItem,
					ConditionExpression:       teamExpr.Condition(),
					ExpressionAttributeNames:  teamExpr.Names(),
					ExpressionAttributeValues: teamExpr.Values(),
				},
			},
			{
				Put: &types.Put{
					TableName:                 aws.String(d.tableName),
					Item:                      reservationItem,
					ConditionExpression:       reservationExpr.Condition(),
					ExpressionAttributeNames:  reservationExpr.Names(),
					ExpressionAttributeValues: reservationExpr.Values(),
				},
			},
		},
	})
	if err != nil {
		var transactionFailedErr *types.TransactionCanceledException
		if errors.As(err, &transactionFailedErr) {
			if len(transactionFailedErr.CancellationReasons) > 1 &&
				transactionFailedErr.CancellationReasons[1].Code != nil &&
				*transactionFailedErr.CancellationReasons[1].Code == "ConditionalCheckFailed" {
				return teams.NewTeamNameTakenError(fmt.Sprintf("Team name %q is already taken", team.Name), err)
			}
			return teams.NewFailedToWriteError("Team create transaction failed", err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return teams.NewTimeoutError("CreateTeam timed out")
		} else {
			return teams.NewFailedToWriteError("Failed TransactWriteItems call", err)
		}
	}

	return nil
}

func (d *DB) GetTeam(ctx context.Context, id uuid.UUID) (teams.Team, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	resp, err := d.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: teamPK(id)},
			"SK": &types.AttributeValueMemberS{Value: teamPK(id)},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return teams.Team{}, teams.NewTimeoutError("GetTeam timed out")
		}
		return teams.Team{}, teams.NewFailedToFetchError(fmt.Sprintf("Failed to fetch team with ID %q", id), err)
	}

	if len(resp.Item) == 0 {
		return teams.Team{}, teams.NewTeamDoesNotExistError(fmt.Sprintf("Team with ID %q not found", id), nil)
	}

	var item teamDynamo
	if err := attributevalue.UnmarshalMap(resp.Item, &item); err != nil {
		return teams.Team{}, teams.NewFailedToFetchError("Failed to parse team from DB", err)
	}
	team, err := teamFromTeamDynamo(item)
	if err != nil {
		return teams.Team{}, teams.NewFailedToFetchError("Failed to parse team from DB", err)
	}
	if !team.Status.Valid() {
		return teams.Team{}, teams.NewFailedToFetchError("Stored team has invalid status", nil)
	}
	return team, nil
}

// ListTeams pages the TEAM GSI name-first: GSI1SK is NAME#<normalized>#<id>,
// so ascending order is the directory's alphabetical order.
func (d *DB) ListTeams(ctx context.Context, limit int32, cursor *string) (teams.GetTeamsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	keyCond := expression.Key("GSI1PK").Equal(expression.Value(teamEntityName)).
		And(expression.Key("GSI1SK").BeginsWith("NAME#"))

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		return teams.GetTeamsResponse{}, teams.NewFailedToFetchError("Failed to build team list query", err)
	}

	var startKey map[string]types.AttributeValue
	if cursor != nil {
		startKey, err = cursorToLastEval(*cursor)
		if err != nil {
			return teams.GetTeamsResponse{}, teams.NewInvalidCursorError("Invalid cursor", err)
		}
	}

	result, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
		IndexName:                 aws.String(gsi1),
		TableName:                 aws.String(d.tableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ScanIndexForward:          aws.Bool(true),
		// Fetch 1 more than limit to check if there is another page or not
		Limit:             aws.Int32(limit + 1),
		ExclusiveStartKey: startKey,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return teams.GetTeamsResponse{}, teams.NewTimeoutError("ListTeams timed out")
		}
		return teams.GetTeamsResponse{}, teams.NewFailedToFetchError("Failed to fetch teams from dynamo", err)
	}

	var items []teamDynamo
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &items); err != nil {
		return teams.GetTeamsResponse{}, teams.NewFailedToFetchError("Failed to parse teams from DB", err)
	}

	hasNextPage := len(items) > int(limit)

	var newCursor *string
	if hasNextPage && len(result.LastEvaluatedKey) > 0 {
		// The extra item fetched to detect the next page must not be handed
		// out; the cursor points at the last item this page returns.
		lastItemGivenToUser := result.Items[len(result.Items)-2]
		lastItemKey := getKeyFromItem(result.LastEvaluatedKey, lastItemGivenToUser)
		c, err := lastEvalKeyToCursor(lastItemKey)
		if err != nil {
			return teams.GetTeamsResponse{}, teams.NewFailedToFetchError("Failed to make cursor", err)
		}
		newCursor = &c
	}

	page := items[:min(int(limit), len(items))]
	teamsPage := make([]teams.Team, 0, len(page))
	for _, item := range page {
		team, err := teamFromTeamDynamo(item)
		if err != nil {
			return teams.GetTeamsResponse{}, teams.NewFailedToFetchError("Failed to parse team from DB", err)
		}
		if !team.Status.Valid() {
			return teams.GetTeamsResponse{}, teams.NewFailedToFetchError("Stored team has invalid status", nil)
		}
		teamsPage = append(teamsPage, team)
	}

	return teams.GetTeamsResponse{
		Data:        teamsPage,
		Cursor:      newCursor,
		HasNextPage: hasNextPage,
	}, nil
}
