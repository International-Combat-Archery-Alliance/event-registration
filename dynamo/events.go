package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/slices"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

var _ events.Repository = &DB{}

type eventDynamo struct {
	PK                    string
	SK                    string
	GSI1PK                string
	GSI1SK                string
	ID                    string
	Version               int
	Name                  string
	EventLocation         events.Location
	StartTime             time.Time
	EndTime               time.Time
	RegistrationCloseTime time.Time
	RegistrationTypes     []events.RegistrationType
	AllowedTeamSizeRange  events.Range
	NumTeams              int
	NumRosteredPlayers    int
	NumTotalPlayers       int
}

const (
	eventEntityName = "EVENT"
)

func eventPK(id uuid.UUID) string {
	return fmt.Sprintf("%s#%s", eventEntityName, id)
}

func eventSK(id uuid.UUID) string {
	return fmt.Sprintf("%s#%s", eventEntityName, id)
}

func newEventDynamo(event events.Event) eventDynamo {
	return eventDynamo{
		PK:                    eventPK(event.ID),
		SK:                    eventSK(event.ID),
		GSI1PK:                eventEntityName,
		GSI1SK:                fmt.Sprintf("%s#%s#%s", eventEntityName, event.StartTime, event.ID),
		ID:                    event.ID.String(),
		Version:               event.Version,
		Name:                  event.Name,
		EventLocation:         event.EventLocation,
		StartTime:             event.StartTime,
		EndTime:               event.EndTime,
		RegistrationCloseTime: event.RegistrationCloseTime,
		RegistrationTypes:     event.RegistrationTypes,
		AllowedTeamSizeRange:  event.AllowedTeamSizeRange,
		NumTotalPlayers:       event.NumTotalPlayers,
		NumRosteredPlayers:    event.NumRosteredPlayers,
		NumTeams:              event.NumTeams,
	}
}

func eventFromEventDynamo(event eventDynamo) events.Event {
	return events.Event{
		ID:                    uuid.MustParse(event.ID),
		Version:               event.Version,
		Name:                  event.Name,
		EventLocation:         event.EventLocation,
		StartTime:             event.StartTime,
		EndTime:               event.EndTime,
		RegistrationCloseTime: event.RegistrationCloseTime,
		RegistrationTypes:     event.RegistrationTypes,
		AllowedTeamSizeRange:  event.AllowedTeamSizeRange,
		NumTeams:              event.NumTeams,
		NumRosteredPlayers:    event.NumRosteredPlayers,
		NumTotalPlayers:       event.NumTotalPlayers,
	}
}

func (d *DB) GetEvent(ctx context.Context, id uuid.UUID) (events.Event, error) {
	resp, err := d.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: eventPK(id)},
			"SK": &types.AttributeValueMemberS{Value: eventSK(id)},
		},
	})
	if err != nil {
		return events.Event{}, events.NewFailedToFetchError(fmt.Sprintf("Failed to fetch event with ID %q", id), err)
	}

	if len(resp.Item) == 0 {
		return events.Event{}, events.NewEventDoesNotExistsError(fmt.Sprintf("Event with ID %q not found", id), nil)
	}

	var event eventDynamo
	err = attributevalue.UnmarshalMap(resp.Item, &event)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal event from DB: %s", err))
	}
	return eventFromEventDynamo(event), nil
}

func (d *DB) CreateEvent(ctx context.Context, event events.Event) error {
	dynamoItem := newEventDynamo(event)

	item, err := attributevalue.MarshalMap(dynamoItem)
	if err != nil {
		return events.NewFailedToTranslateToDBModelError("Failed to convert Event to eventDynamo", err)
	}

	expr := exprMustBuild(expression.NewBuilder().
		WithCondition(newEntityVersionConditional(dynamoItem.Version)))

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
			return events.NewEventAlreadyExistsError(fmt.Sprintf("Event with ID %q already exists", event.ID), err)
		} else {
			return events.NewFailedToWriteError("Failed PutItem call", err)
		}
	}

	return nil
}

func (d *DB) GetEvents(ctx context.Context, limit int32, cursor *string) (events.GetEventsResponse, error) {
	keyCond := expression.Key("GSI1PK").Equal(expression.Value(eventEntityName)).
		And(expression.Key("GSI1SK").BeginsWith(eventEntityName))

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build dynamo key expression: %s", err))
	}

	var startKey map[string]types.AttributeValue
	if cursor != nil {
		startKey, err = cursorToLastEval(*cursor)
		if err != nil {
			return events.GetEventsResponse{}, events.NewInvalidCursorError("Invalid cursor", err)
		}
	}

	result, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
		IndexName:                 aws.String(gsi1),
		TableName:                 aws.String(d.tableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		// Fetch 1 more than limit to check if there is another page or not
		Limit:             aws.Int32(limit + 1),
		ExclusiveStartKey: startKey,
	})
	if err != nil {
		return events.GetEventsResponse{}, events.NewFailedToFetchError("Failed to fetch events from dynamo", err)
	}

	var dynamoItems []eventDynamo
	err = attributevalue.UnmarshalListOfMaps(result.Items, &dynamoItems)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal dynamo events: %s", err))
	}

	hasNextPage := len(dynamoItems) > int(limit)

	var newCursor *string
	if hasNextPage && len(result.LastEvaluatedKey) > 0 {
		// Can't use LastEvalKey directly because we grabbed an extra item to check for next page
		lastItemGivenToUser := result.Items[len(result.Items)-2]
		lastItemKey := getKeyFromItem(result.LastEvaluatedKey, lastItemGivenToUser)
		c, err := lastEvalKeyToCursor(lastItemKey)
		if err != nil {
			panic(fmt.Sprintf("failed to make cursor from lastEvalKey: %s", err))
		}
		newCursor = &c
	}

	return events.GetEventsResponse{
		Data: slices.Map(dynamoItems, func(v eventDynamo) events.Event {
			return eventFromEventDynamo(v)
		})[:min(int(limit), len(dynamoItems))],
		Cursor:      newCursor,
		HasNextPage: hasNextPage,
	}, nil
}

func (d *DB) UpdateEvent(ctx context.Context, event events.Event) error {
	dynamoItem := newEventDynamo(event)

	item, err := attributevalue.MarshalMap(dynamoItem)
	if err != nil {
		return events.NewFailedToTranslateToDBModelError("Failed to convert Event to eventDynamo", err)
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
			return events.NewEventDoesNotExistsError(fmt.Sprintf("Event with ID %q does not exists", event.ID), err)
		} else {
			return events.NewFailedToWriteError("Failed PutItem call", err)
		}
	}

	return nil
}
