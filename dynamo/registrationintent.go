package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

type registrationIntentDynamo struct {
	PK               string
	SK               string
	Version          int
	EventId          uuid.UUID
	PaymentSessionID string
	Email            string
}

const (
	registrationIntentEntityName = "REG_INTENT"
)

func registrationIntentPK(eventId uuid.UUID) string {
	return eventPK(eventId)
}

func registrationIntentSK(email string) string {
	return fmt.Sprintf("%s#%s", registrationIntentEntityName, email)
}

func regIntentToDynamo(regIntent registration.RegistrationIntent) registrationIntentDynamo {
	return registrationIntentDynamo{
		PK:               registrationPK(regIntent.EventId),
		SK:               registrationIntentSK(regIntent.Email),
		Version:          regIntent.Version,
		Email:            regIntent.Email,
		EventId:          regIntent.EventId,
		PaymentSessionID: regIntent.PaymentSessionId,
	}
}

func dynamoRegIntentToRegIntent(regIntent registrationIntentDynamo) registration.RegistrationIntent {
	return registration.RegistrationIntent{
		Version:          regIntent.Version,
		EventId:          regIntent.EventId,
		PaymentSessionId: regIntent.PaymentSessionID,
		Email:            regIntent.Email,
	}
}

func (d *DB) GetRegistrationIntent(ctx context.Context, eventId uuid.UUID, email string) (registration.RegistrationIntent, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	resp, err := d.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: registrationIntentPK(eventId)},
			"SK": &types.AttributeValueMemberS{Value: registrationIntentSK(email)},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return registration.RegistrationIntent{}, registration.NewTimeoutError("GetRegistrationIntent timed out")
		}
		return registration.RegistrationIntent{}, registration.NewFailedToFetchError(fmt.Sprintf("Failed to fetch registration intent for event ID %q and email %s", eventId, email), err)
	}

	if len(resp.Item) == 0 {
		return registration.RegistrationIntent{}, registration.NewRegistrationDoesNotExistsError(fmt.Sprintf("RegistrationIntent for event ID %q and email %s not found", eventId, email), nil)
	}

	var reg registrationIntentDynamo
	err = attributevalue.UnmarshalMap(resp.Item, &reg)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal registrationIntent from DB: %s", err))
	}
	return dynamoRegIntentToRegIntent(reg), nil
}

func (d *DB) DeleteExpiredRegistration(ctx context.Context, reg registration.Registration, regIntent registration.RegistrationIntent, event events.Event) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoReg := registrationToDynamo(reg)
	regExpr := exprMustBuild(expression.NewBuilder().
		WithCondition(existingEntityVersionConditional(dynamoReg.Version)))

	dynamoRegIntent := regIntentToDynamo(regIntent)
	regIntentExpr := exprMustBuild(expression.NewBuilder().
		WithCondition(existingEntityVersionConditional(dynamoRegIntent.Version)))

	dynamoEvent := newEventDynamo(event)
	eventItem, err := attributevalue.MarshalMap(dynamoEvent)
	if err != nil {
		return registration.NewFailedToTranslateToDBModelError("Failed to translate event to dynamo model", err)
	}
	eventExpr := exprMustBuild(expression.NewBuilder().
		WithCondition(existingEntityVersionConditional(event.Version)))

	_, err = d.dynamoClient.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			// Delete the reg and reg intent, update the event to have the updated stats
			{
				Delete: &types.Delete{
					TableName: aws.String(d.tableName),
					Key: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: dynamoReg.PK},
						"SK": &types.AttributeValueMemberS{Value: dynamoReg.SK},
					},
					ConditionExpression:       regExpr.Condition(),
					ExpressionAttributeNames:  regExpr.Names(),
					ExpressionAttributeValues: regExpr.Values(),
				},
			},
			{
				Delete: &types.Delete{
					TableName: aws.String(d.tableName),
					Key: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: dynamoRegIntent.PK},
						"SK": &types.AttributeValueMemberS{Value: dynamoRegIntent.SK},
					},
					ConditionExpression:       regIntentExpr.Condition(),
					ExpressionAttributeNames:  regIntentExpr.Names(),
					ExpressionAttributeValues: regIntentExpr.Values(),
				},
			},
			{
				Put: &types.Put{
					TableName:                 aws.String(d.tableName),
					Item:                      eventItem,
					ConditionExpression:       eventExpr.Condition(),
					ExpressionAttributeNames:  eventExpr.Names(),
					ExpressionAttributeValues: eventExpr.Values(),
				},
			},
		},
	})
	if err != nil {
		var transactionFailedErr *types.TransactionCanceledException
		if errors.As(err, &transactionFailedErr) {
			return registration.NewFailedToWriteError("Version conflict error", err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return registration.NewTimeoutError("DeleteExpiredRegistration timed out")
		} else {
			return registration.NewFailedToWriteError("Failed PutItem call", err)
		}
	}

	return nil

}
