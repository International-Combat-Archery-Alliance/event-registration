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

var _ events.EventRepository = &DB{}

type eventDynamo struct {
	PK            string
	SK            string
	GSI1PK        string
	GSI1SK        string
	ID            uuid.UUID
	Name          string
	EventDateTime time.Time
}

const (
	eventPKPrefix = "EVENT"
)

func newEventDynamo(event events.Event) eventDynamo {
	return eventDynamo{
		PK:            fmt.Sprintf("%s#%s", eventPKPrefix, event.ID),
		SK:            fmt.Sprintf("%s#%s", eventPKPrefix, event.ID),
		GSI1PK:        eventPKPrefix,
		GSI1SK:        fmt.Sprintf("%s#%s#%s", eventPKPrefix, event.EventDateTime, event.ID),
		ID:            event.ID,
		Name:          event.Name,
		EventDateTime: event.EventDateTime,
	}
}

func eventFromEventDynamo(event eventDynamo) events.Event {
	return events.Event{
		ID:            event.ID,
		Name:          event.Name,
		EventDateTime: event.EventDateTime,
	}
}

func (d *DB) CreateEvent(ctx context.Context, event events.Event) error {
	dynamoItem := newEventDynamo(event)

	item, err := attributevalue.MarshalMap(dynamoItem)
	if err != nil {
		return events.NewFailedToTranslateToDBModelError("Failed to convert Event to eventDynamo", err)
	}

	_, err = d.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(d.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
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
	keyCond := expression.Key("GSI1PK").Equal(expression.Value(eventPKPrefix)).
		And(expression.Key("GSI1SK").BeginsWith(eventPKPrefix))

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

	_, err = d.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(d.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_exists(PK)"),
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
