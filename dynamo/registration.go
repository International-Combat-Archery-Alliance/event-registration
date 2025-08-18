package dynamo

import (
	"context"
	"errors"
	"fmt"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
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
	ID       uuid.UUID
	EventID  uuid.UUID
	HomeCity string
	Paid     bool

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
			PK:         registrationPK(indivReg.EventID),
			SK:         registrationSK(indivReg.ID),
			Type:       indivReg.Type(),
			ID:         indivReg.ID,
			EventID:    indivReg.EventID,
			HomeCity:   indivReg.HomeCity,
			Paid:       indivReg.Paid,
			Email:      indivReg.Email,
			PlayerInfo: indivReg.PlayerInfo,
			Experience: indivReg.Experience,
		}
	case events.BY_TEAM:
		teamReg := reg.(registration.TeamRegistration)
		return registrationDynamo{
			PK:           registrationPK(teamReg.EventID),
			SK:           registrationSK(teamReg.ID),
			Type:         teamReg.Type(),
			ID:           teamReg.ID,
			EventID:      teamReg.EventID,
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
			ID:         dynReg.ID,
			EventID:    dynReg.EventID,
			HomeCity:   dynReg.HomeCity,
			Paid:       dynReg.Paid,
			Email:      dynReg.Email,
			PlayerInfo: dynReg.PlayerInfo,
			Experience: dynReg.Experience,
		}
	case events.BY_TEAM:
		return registration.TeamRegistration{
			ID:           dynReg.ID,
			EventID:      dynReg.EventID,
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

func (d *DB) CreateRegistration(ctx context.Context, reg registration.Registration) error {
	dynamoReg := registrationToDynamo(reg)

	item, err := attributevalue.MarshalMap(dynamoReg)
	if err != nil {
		return registration.NewFailedToTranslateToDBModelError("Failed to translate registration to dynamo model", err)
	}

	_, err = d.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(d.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(SK)"),
	})
	if err != nil {
		var condCheckFailedErr *types.ConditionalCheckFailedException
		if errors.As(err, &condCheckFailedErr) {
			return registration.NewRegistrationAlreadyExistsError(fmt.Sprintf("Registration with ID %q already exists", dynamoReg.ID), err)
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
