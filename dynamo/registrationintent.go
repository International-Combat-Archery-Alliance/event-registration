package dynamo

import (
	"fmt"

	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
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
