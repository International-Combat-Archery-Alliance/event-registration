package dynamo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/International-Combat-Archery-Alliance/event-registration/slices"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

var _ registration.Repository = &DB{}

type registrationDynamo struct {
	PK string
	SK string

	Type events.RegistrationType

	// Both type attributes
	ID           uuid.UUID
	Version      int
	EventID      uuid.UUID
	RegisteredAt time.Time
	HomeCity     string
	Paid         bool

	// Individual attributes
	Email      string
	PlayerInfo registration.PlayerInfo
	Experience registration.ExperienceLevel

	// Team attributes
	TeamName     string
	CaptainEmail string
	Players      []registration.PlayerInfo
}

const (
	registrationEntityName = "REGISTRATION"
)

func registrationPK(eventId uuid.UUID) string {
	return eventPK(eventId)
}

func registrationSK(id uuid.UUID) string {
	return fmt.Sprintf("%s#%s", registrationEntityName, id)
}

func registrationToDynamo(reg registration.Registration) registrationDynamo {
	switch reg.Type() {
	case events.BY_INDIVIDUAL:
		indivReg := reg.(registration.IndividualRegistration)
		return registrationDynamo{
			PK:           registrationPK(indivReg.EventID),
			SK:           registrationSK(indivReg.ID),
			Type:         indivReg.Type(),
			ID:           indivReg.ID,
			Version:      indivReg.Version,
			EventID:      indivReg.EventID,
			RegisteredAt: indivReg.RegisteredAt,
			HomeCity:     indivReg.HomeCity,
			Paid:         indivReg.Paid,
			Email:        indivReg.Email,
			PlayerInfo:   indivReg.PlayerInfo,
			Experience:   indivReg.Experience,
		}
	case events.BY_TEAM:
		teamReg := reg.(registration.TeamRegistration)
		return registrationDynamo{
			PK:           registrationPK(teamReg.EventID),
			SK:           registrationSK(teamReg.ID),
			Type:         teamReg.Type(),
			ID:           teamReg.ID,
			Version:      teamReg.Version,
			EventID:      teamReg.EventID,
			RegisteredAt: teamReg.RegisteredAt,
			HomeCity:     teamReg.HomeCity,
			Paid:         teamReg.Paid,
			TeamName:     teamReg.TeamName,
			CaptainEmail: teamReg.CaptainEmail,
			Players:      teamReg.Players,
		}
	default:
		panic("unknown registration type")
	}
}

func dynamoToRegistration(dynReg registrationDynamo) registration.Registration {
	switch dynReg.Type {
	case events.BY_INDIVIDUAL:
		return registration.IndividualRegistration{
			ID:           dynReg.ID,
			Version:      dynReg.Version,
			EventID:      dynReg.EventID,
			RegisteredAt: dynReg.RegisteredAt,
			HomeCity:     dynReg.HomeCity,
			Paid:         dynReg.Paid,
			Email:        dynReg.Email,
			PlayerInfo:   dynReg.PlayerInfo,
			Experience:   dynReg.Experience,
		}
	case events.BY_TEAM:
		return registration.TeamRegistration{
			ID:           dynReg.ID,
			Version:      dynReg.Version,
			EventID:      dynReg.EventID,
			RegisteredAt: dynReg.RegisteredAt,
			HomeCity:     dynReg.HomeCity,
			Paid:         dynReg.Paid,
			TeamName:     dynReg.TeamName,
			CaptainEmail: dynReg.CaptainEmail,
			Players:      dynReg.Players,
		}
	default:
		panic("unknown registration type")
	}
}

func (d *DB) CreateRegistration(ctx context.Context, reg registration.Registration, event events.Event) error {
	dynamoReg := registrationToDynamo(reg)

	regItem, err := attributevalue.MarshalMap(dynamoReg)
	if err != nil {
		return registration.NewFailedToTranslateToDBModelError("Failed to translate registration to dynamo model", err)
	}
	regExpr := exprMustBuild(expression.NewBuilder().
		WithCondition(newEntityVersionConditional(dynamoReg.Version)))

	dynamoEvent := newEventDynamo(event)

	eventItem, err := attributevalue.MarshalMap(dynamoEvent)
	if err != nil {
		return registration.NewFailedToTranslateToDBModelError("Failed to translate event to dynamo model", err)
	}
	eventExpr := exprMustBuild(expression.NewBuilder().
		WithCondition(existingEntityVersionConditional(event.Version)))

	_, err = d.dynamoClient.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:                 aws.String(d.tableName),
					Item:                      regItem,
					ConditionExpression:       regExpr.Condition(),
					ExpressionAttributeNames:  regExpr.Names(),
					ExpressionAttributeValues: regExpr.Values(),
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
			if transactionFailedErr.CancellationReasons[0].Code != nil {
				return registration.NewRegistrationAlreadyExistsError(fmt.Sprintf("Registration with ID %q already exists", dynamoReg.ID), err)
			}
			return registration.NewFailedToWriteError("Version conflict error", err)
		} else {
			return registration.NewFailedToWriteError("Failed PutItem call", err)
		}
	}

	return nil
}

func (d *DB) GetRegistration(ctx context.Context, eventId uuid.UUID, id uuid.UUID) (registration.Registration, error) {
	resp, err := d.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: registrationPK(eventId)},
			"SK": &types.AttributeValueMemberS{Value: registrationSK(id)},
		},
	})
	if err != nil {
		return nil, registration.NewFailedToFetchError(fmt.Sprintf("Failed to fetch registration with event id %q and id %q", eventId, id), err)
	}

	if len(resp.Item) == 0 {
		return nil, registration.NewRegistrationDoesNotExistsError(fmt.Sprintf("Registration with event id %q and id %q not found", eventId, id), err)
	}

	var dynReg registrationDynamo
	err = attributevalue.UnmarshalMap(resp.Item, &dynReg)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal registration from dynamo: %s", err))
	}

	return dynamoToRegistration(dynReg), nil
}

func (d *DB) GetAllRegistrationsForEvent(ctx context.Context, eventId uuid.UUID, cursor *string, limit int32) (registration.GetAllRegistrationsResponse, error) {
	keyCond := expression.Key("PK").Equal(expression.Value(registrationPK(eventId))).
		And(expression.Key("SK").BeginsWith(registrationEntityName))

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build dynamo key expression: %s", err))
	}

	var startKey map[string]types.AttributeValue
	if cursor != nil {
		startKey, err = cursorToLastEval(*cursor)
		if err != nil {
			return registration.GetAllRegistrationsResponse{}, registration.NewInvalidCursorError("Invalid cursor", err)
		}
	}

	result, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
		TableName:                 aws.String(d.tableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		// Fetch 1 more than limit to check if there is another page or not
		Limit:             aws.Int32(limit + 1),
		ExclusiveStartKey: startKey,
	})
	if err != nil {
		return registration.GetAllRegistrationsResponse{}, registration.NewFailedToFetchError("Failed to fetch registrations from dynamo", err)
	}

	var dynamoItems []registrationDynamo
	err = attributevalue.UnmarshalListOfMaps(result.Items, &dynamoItems)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal dynamo registrations: %s", err))
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

	return registration.GetAllRegistrationsResponse{
		Data: slices.Map(dynamoItems, func(v registrationDynamo) registration.Registration {
			return dynamoToRegistration(v)
		})[:min(int(limit), len(dynamoItems))],
		Cursor:      newCursor,
		HasNextPage: hasNextPage,
	}, nil
}
