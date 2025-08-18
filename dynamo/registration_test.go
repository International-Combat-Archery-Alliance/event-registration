package dynamo

import (
	"context"
	"testing"

	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateRegistration(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully create an individual registration", func(t *testing.T) {
		resetTable(ctx)
		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    uuid.New(),
			HomeCity:   "Test City",
			Paid:       true,
			Email:      "test@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "John", LastName: "Doe"},
			Experience: registration.NOVICE,
		}

		require.NoError(t, db.CreateRegistration(ctx, reg))

		// Verify by getting the registration
		fetchedReg, err := db.GetRegistration(ctx, reg.EventID, reg.ID)
		require.NoError(t, err)
		assert.Equal(t, reg, fetchedReg)
	})

	t.Run("successfully create a team registration", func(t *testing.T) {
		resetTable(ctx)
		reg := registration.TeamRegistration{
			ID:           uuid.New(),
			EventID:      uuid.New(),
			HomeCity:     "Team City",
			Paid:         false,
			TeamName:     "Test Team",
			CaptainEmail: "captain@example.com",
			Players:      []registration.PlayerInfo{{FirstName: "Jane", LastName: "Doe"}},
		}

		require.NoError(t, db.CreateRegistration(ctx, reg))

		// Verify by getting the registration
		fetchedReg, err := db.GetRegistration(ctx, reg.EventID, reg.ID)
		require.NoError(t, err)
		assert.Equal(t, reg, fetchedReg)
	})

	t.Run("fail to create a registration that already exists", func(t *testing.T) {
		resetTable(ctx)
		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    uuid.New(),
			HomeCity:   "Test City",
			Paid:       true,
			Email:      "test@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "John", LastName: "Doe"},
			Experience: registration.NOVICE,
		}

		require.NoError(t, db.CreateRegistration(ctx, reg))

		err := db.CreateRegistration(ctx, reg)
		require.Error(t, err)
		var regError *registration.RegistrationError
		require.ErrorAs(t, err, &regError)
		assert.Equal(t, registration.REASON_REGISTRATION_ALREADY_EXISTS, regError.Reason)
	})
}

func TestGetRegistration(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully get an individual registration", func(t *testing.T) {
		resetTable(ctx)
		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    uuid.New(),
			HomeCity:   "Test City",
			Paid:       true,
			Email:      "test@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "John", LastName: "Doe"},
			Experience: registration.NOVICE,
		}
		require.NoError(t, db.CreateRegistration(ctx, reg))

		fetchedReg, err := db.GetRegistration(ctx, reg.EventID, reg.ID)
		require.NoError(t, err)
		assert.Equal(t, reg, fetchedReg)
	})

	t.Run("successfully get a team registration", func(t *testing.T) {
		resetTable(ctx)
		reg := registration.TeamRegistration{
			ID:           uuid.New(),
			EventID:      uuid.New(),
			HomeCity:     "Team City",
			Paid:         false,
			TeamName:     "Test Team",
			CaptainEmail: "captain@example.com",
			Players:      []registration.PlayerInfo{{FirstName: "Jane", LastName: "Doe"}},
		}
		require.NoError(t, db.CreateRegistration(ctx, reg))

		fetchedReg, err := db.GetRegistration(ctx, reg.EventID, reg.ID)
		require.NoError(t, err)
		assert.Equal(t, reg, fetchedReg)
	})

	t.Run("fail to get a registration that does not exist", func(t *testing.T) {
		resetTable(ctx)

		_, err := db.GetRegistration(ctx, uuid.New(), uuid.New())
		require.Error(t, err)
		var regError *registration.RegistrationError
		require.ErrorAs(t, err, &regError)
		assert.Equal(t, registration.REASON_REGISTRATION_DOES_NOT_EXIST, regError.Reason)
	})
}

