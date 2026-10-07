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

// participationDynamo and teamHistoryDynamo carry no GSI attributes: only
// EVENT and TEAM masters do (RFC-0001/0002 GSI rule).
type participationDynamo struct {
	PK                string
	SK                string
	EventID           string
	TeamID            string
	Status            string
	RosterSnapshot    []string
	RosterSizeAtEvent int
	Version           int
}

type teamHistoryDynamo struct {
	PK        string
	SK        string
	TeamID    string
	EventID   string
	EventName string
	EventDate time.Time
	Status    string
	Result    *teamHistoryResultDynamo
	Version   int
}

type teamHistoryResultDynamo struct {
	Wins      int
	Losses    int
	PF        int
	PA        int
	Placement int
}

func participationPK(eventID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", eventEntityName, eventID)
}

func participationSK(teamID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", teamEntityName, teamID)
}

func teamHistoryPK(teamID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", teamEntityName, teamID)
}

func teamHistorySK(eventID uuid.UUID) string {
	return fmt.Sprintf("%s#%s", eventEntityName, eventID)
}

func newParticipationDynamo(p teams.Participation) participationDynamo {
	snapshot := make([]string, 0, len(p.RosterSnapshot))
	for _, id := range p.RosterSnapshot {
		snapshot = append(snapshot, id.String())
	}
	return participationDynamo{
		PK:                participationPK(p.EventID),
		SK:                participationSK(p.TeamID),
		EventID:           p.EventID.String(),
		TeamID:            p.TeamID.String(),
		Status:            string(p.Status),
		RosterSnapshot:    snapshot,
		RosterSizeAtEvent: p.RosterSizeAtEvent,
		Version:           p.Version,
	}
}

func participationFromDynamo(item participationDynamo) (teams.Participation, error) {
	eventID, err := uuid.Parse(item.EventID)
	if err != nil {
		return teams.Participation{}, err
	}
	teamID, err := uuid.Parse(item.TeamID)
	if err != nil {
		return teams.Participation{}, err
	}
	snapshot := make([]uuid.UUID, 0, len(item.RosterSnapshot))
	for _, s := range item.RosterSnapshot {
		id, err := uuid.Parse(s)
		if err != nil {
			return teams.Participation{}, err
		}
		snapshot = append(snapshot, id)
	}
	return teams.Participation{
		EventID:           eventID,
		TeamID:            teamID,
		Status:            teams.ParticipationStatus(item.Status),
		RosterSnapshot:    snapshot,
		RosterSizeAtEvent: item.RosterSizeAtEvent,
		Version:           item.Version,
	}, nil
}

func newTeamHistoryDynamo(h teams.TeamHistory) teamHistoryDynamo {
	var result *teamHistoryResultDynamo
	if h.Result != nil {
		result = &teamHistoryResultDynamo{
			Wins:      h.Result.Wins,
			Losses:    h.Result.Losses,
			PF:        h.Result.PF,
			PA:        h.Result.PA,
			Placement: h.Result.Placement,
		}
	}
	return teamHistoryDynamo{
		PK:        teamHistoryPK(h.TeamID),
		SK:        teamHistorySK(h.EventID),
		TeamID:    h.TeamID.String(),
		EventID:   h.EventID.String(),
		EventName: h.EventName,
		EventDate: h.EventDate.UTC(),
		Status:    string(h.Status),
		Result:    result,
		Version:   h.Version,
	}
}

func (d *DB) SeedParticipation(ctx context.Context, participation teams.Participation, history teams.TeamHistory) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	if err := participation.Validate(); err != nil {
		return err
	}
	if participation.EventID != history.EventID || participation.TeamID != history.TeamID {
		return teams.NewInvalidTeamError("participation and history must reference the same event and team", nil)
	}

	participationItem, err := attributevalue.MarshalMap(newParticipationDynamo(participation))
	if err != nil {
		return teams.NewFailedToTranslateToDBModelError("Failed to convert Participation to dynamo model", err)
	}
	participationExpr := exprMustBuild(expression.NewBuilder().
		WithCondition(expression.Name("PK").AttributeNotExists()))

	historyItem, err := attributevalue.MarshalMap(newTeamHistoryDynamo(history))
	if err != nil {
		return teams.NewFailedToTranslateToDBModelError("Failed to convert TeamHistory to dynamo model", err)
	}
	historyExpr := exprMustBuild(expression.NewBuilder().
		WithCondition(expression.Name("PK").AttributeNotExists()))

	_, err = d.dynamoClient.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:                 aws.String(d.tableName),
					Item:                      participationItem,
					ConditionExpression:       participationExpr.Condition(),
					ExpressionAttributeNames:  participationExpr.Names(),
					ExpressionAttributeValues: participationExpr.Values(),
				},
			},
			{
				Put: &types.Put{
					TableName:                 aws.String(d.tableName),
					Item:                      historyItem,
					ConditionExpression:       historyExpr.Condition(),
					ExpressionAttributeNames:  historyExpr.Names(),
					ExpressionAttributeValues: historyExpr.Values(),
				},
			},
		},
	})
	if err != nil {
		var transactionFailedErr *types.TransactionCanceledException
		if errors.As(err, &transactionFailedErr) {
			for _, reason := range transactionFailedErr.CancellationReasons {
				if reason.Code != nil && *reason.Code == "ConditionalCheckFailed" {
					return teams.NewParticipationAlreadyExistsError(
						fmt.Sprintf("Team %q already participates in event %q", participation.TeamID, participation.EventID), err)
				}
			}
			return teams.NewFailedToWriteError("Seed participation transaction failed", err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return teams.NewTimeoutError("SeedParticipation timed out")
		} else {
			return teams.NewFailedToWriteError("Failed TransactWriteItems call", err)
		}
	}

	return nil
}

func (d *DB) GetParticipation(ctx context.Context, eventID, teamID uuid.UUID) (teams.Participation, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	resp, err := d.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: participationPK(eventID)},
			"SK": &types.AttributeValueMemberS{Value: participationSK(teamID)},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return teams.Participation{}, teams.NewTimeoutError("GetParticipation timed out")
		}
		return teams.Participation{}, teams.NewFailedToFetchError("Failed to fetch participation", err)
	}

	if len(resp.Item) == 0 {
		return teams.Participation{}, teams.NewParticipationDoesNotExistError("Participation not found", nil)
	}

	var item participationDynamo
	if err := attributevalue.UnmarshalMap(resp.Item, &item); err != nil {
		return teams.Participation{}, teams.NewFailedToFetchError("Failed to parse participation from DB", err)
	}
	participation, err := participationFromDynamo(item)
	if err != nil {
		return teams.Participation{}, teams.NewFailedToFetchError("Failed to parse participation from DB", err)
	}
	if err := participation.Validate(); err != nil {
		return teams.Participation{}, teams.NewFailedToFetchError("Stored participation is invalid", err)
	}
	return participation, nil
}

func (d *DB) ListParticipationsForEvent(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	expr, err := expression.NewBuilder().
		WithKeyCondition(
			expression.Key("PK").Equal(expression.Value(participationPK(eventID))).
				And(expression.Key("SK").BeginsWith(teamEntityName + "#")),
		).Build()
	if err != nil {
		return nil, teams.NewFailedToFetchError("Failed to build participation query", err)
	}

	var result []teams.Participation
	var startKey map[string]types.AttributeValue
	for {
		out, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
			TableName:                 aws.String(d.tableName),
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			ExclusiveStartKey:         startKey,
		})
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, teams.NewTimeoutError("ListParticipationsForEvent timed out")
			}
			return nil, teams.NewFailedToFetchError("Failed to fetch participations", err)
		}

		var page []participationDynamo
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &page); err != nil {
			return nil, teams.NewFailedToFetchError("Failed to parse participations from DB", err)
		}
		for _, item := range page {
			participation, err := participationFromDynamo(item)
			if err != nil {
				return nil, teams.NewFailedToFetchError("Failed to parse participations from DB", err)
			}
			if err := participation.Validate(); err != nil {
				return nil, teams.NewFailedToFetchError("Stored participation is invalid", err)
			}
			result = append(result, participation)
		}

		if len(out.LastEvaluatedKey) == 0 {
			return result, nil
		}
		startKey = out.LastEvaluatedKey
	}
}
