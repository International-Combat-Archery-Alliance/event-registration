package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
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
	ExpiresAt        time.Time
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
		ExpiresAt:        regIntent.ExpiresAt,
	}
}

func dynamoRegIntentToRegIntent(regIntent registrationIntentDynamo) registration.RegistrationIntent {
	return registration.RegistrationIntent{
		Version:          regIntent.Version,
		EventId:          regIntent.EventId,
		PaymentSessionId: regIntent.PaymentSessionID,
		Email:            regIntent.Email,
		ExpiresAt:        regIntent.ExpiresAt,
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
