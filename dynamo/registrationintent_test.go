package dynamo

import (
	"context"
	"testing"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRegistrationIntent(t *testing.T) {
	ctx := context.Background()
	a := assert.New(t)

	t.Run("successfully get registration intent", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Intent City",
			Paid:       false,
			Email:      "intent@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Intent", LastName: "User"},
			Experience: registration.NOVICE,
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "stripe_session_intent",
			Email:            "intent@example.com",
		}

		// Create registration with payment intent
		event2 := events.Event{ID: eventID, Version: 2}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, event2)
		a.NoError(err)

		// Get registration intent
		retrieved, err := db.GetRegistrationIntent(ctx, eventID, "intent@example.com")
		a.NoError(err)
		a.Equal(regIntent, retrieved)
	})

	t.Run("registration intent does not exist", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		_, err := db.GetRegistrationIntent(ctx, eventID, "nonexistent@example.com")
		a.Error(err)
		var regError *registration.Error
		require.ErrorAs(t, err, &regError)
		a.Equal(registration.REASON_REGISTRATION_DOES_NOT_EXIST, regError.Reason)
	})
}
