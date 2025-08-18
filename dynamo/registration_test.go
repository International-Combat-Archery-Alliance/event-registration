package dynamo

import (
	"context"
	"testing"
	"time"

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
			ID:           uuid.New(),
			EventID:      uuid.New(),
			RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
			HomeCity:     "Test City",
			Paid:         true,
			Email:        "test@example.com",
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
			RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
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
			ID:           uuid.New(),
			EventID:      uuid.New(),
			RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
			HomeCity:     "Test City",
			Paid:         true,
			Email:        "test@example.com",
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
			RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
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

// getRegistrationID is a helper function to extract the ID from a Registration interface.
func getRegistrationID(reg registration.Registration) uuid.UUID {
	switch r := reg.(type) {
	case registration.IndividualRegistration:
		return r.ID
	case registration.TeamRegistration:
		return r.ID
	default:
		panic("unknown registration type")
	}
}

func TestGetAllRegistrationsForEvent(t *testing.T) {
	ctx := context.Background()
	a := assert.New(t)

	// Test case 1: Successfully retrieve multiple individual registrations for an event
	t.Run("successfully retrieve multiple individual registrations for an event", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		reg1 := registration.IndividualRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
			HomeCity:     "City A",
			Paid:         true,
			Email:        "a@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Alice", LastName: "Smith"},
			Experience: registration.NOVICE,
		}
		reg2 := registration.IndividualRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
			HomeCity:     "City B",
			Paid:         false,
			Email:        "b@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Bob", LastName: "Johnson"},
			Experience: registration.INTERMEDIATE,
		}

		require.NoError(t, db.CreateRegistration(ctx, reg1))
		require.NoError(t, db.CreateRegistration(ctx, reg2))

		resp, err := db.GetAllRegistrationsForEvent(ctx, eventID, nil, 100)
		a.NoError(err)
		a.Len(resp.Data, 2)
		a.False(resp.HasNextPage)
		a.Nil(resp.Cursor)

		// Check if both registrations are present (order might not be guaranteed)
		foundReg1 := false
		foundReg2 := false
		for _, r := range resp.Data {
			if getRegistrationID(r) == reg1.ID {
				a.Equal(reg1, r)
				foundReg1 = true
			} else if getRegistrationID(r) == reg2.ID {
				a.Equal(reg2, r)
				foundReg2 = true
			}
		}
		a.True(foundReg1, "reg1 not found in results")
		a.True(foundReg2, "reg2 not found in results")
	})

	// Test case 2: Successfully retrieve multiple team registrations for an event
	t.Run("successfully retrieve multiple team registrations for an event", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		teamReg1 := registration.TeamRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
			HomeCity:     "Team City 1",
			Paid:         true,
			TeamName:     "Team Alpha",
			CaptainEmail: "alpha@example.com",
			Players:      []registration.PlayerInfo{{FirstName: "Charlie", LastName: "Brown"}},
		}
		teamReg2 := registration.TeamRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
			HomeCity:     "Team City 2",
			Paid:         false,
			TeamName:     "Team Beta",
			CaptainEmail: "beta@example.com",
			Players:      []registration.PlayerInfo{{FirstName: "Diana", LastName: "Prince"}},
		}

		require.NoError(t, db.CreateRegistration(ctx, teamReg1))
		require.NoError(t, db.CreateRegistration(ctx, teamReg2))

		resp, err := db.GetAllRegistrationsForEvent(ctx, eventID, nil, 100)
		a.NoError(err)
		a.Len(resp.Data, 2)
		a.False(resp.HasNextPage)
		a.Nil(resp.Cursor)

		foundTeamReg1 := false
		foundTeamReg2 := false
		for _, r := range resp.Data {
			if getRegistrationID(r) == teamReg1.ID {
				a.Equal(teamReg1, r)
				foundTeamReg1 = true
			} else if getRegistrationID(r) == teamReg2.ID {
				a.Equal(teamReg2, r)
				foundTeamReg2 = true
			}
		}
		a.True(foundTeamReg1, "teamReg1 not found in results")
		a.True(foundTeamReg2, "teamReg2 not found in results")
	})

	// Test case 3: No registrations found for an event
	t.Run("no registrations found for an event", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New() // Use a new event ID to ensure no existing registrations

		resp, err := db.GetAllRegistrationsForEvent(ctx, eventID, nil, 100)
		a.NoError(err)
		a.Empty(resp.Data)
		a.False(resp.HasNextPage)
		a.Nil(resp.Cursor)
	})

	// Test case 4: Mixed individual and team registrations
	t.Run("mixed individual and team registrations", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		regIndiv := registration.IndividualRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
			HomeCity:     "Mixed City",
			Paid:         true,
			Email:        "mixed@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Mixed", LastName: "User"},
			Experience: registration.NOVICE,
		}
		regTeam := registration.TeamRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
			HomeCity:     "Mixed Team City",
			Paid:         false,
			TeamName:     "Mixed Team",
			CaptainEmail: "mixedteam@example.com",
			Players:      []registration.PlayerInfo{{FirstName: "Mixed", LastName: "Team Player"}},
		}

		require.NoError(t, db.CreateRegistration(ctx, regIndiv))
		require.NoError(t, db.CreateRegistration(ctx, regTeam))

		resp, err := db.GetAllRegistrationsForEvent(ctx, eventID, nil, 100)
		a.NoError(err)
		a.Len(resp.Data, 2)
		a.False(resp.HasNextPage)
		a.Nil(resp.Cursor)

		foundIndiv := false
		foundTeam := false
		for _, r := range resp.Data {
			if getRegistrationID(r) == regIndiv.ID {
				a.Equal(regIndiv, r)
				foundIndiv = true
			} else if getRegistrationID(r) == regTeam.ID {
				a.Equal(regTeam, r)
				foundTeam = true
			}
		}
		a.True(foundIndiv, "individual registration not found")
		a.True(foundTeam, "team registration not found")
	})

	// Test case 5: Pagination - first page
	t.Run("pagination - first page", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		// Create 3 registrations
		reg1 := registration.IndividualRegistration{ID: uuid.New(), EventID: eventID, RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC), PlayerInfo: registration.PlayerInfo{FirstName: "P1"}}
		reg2 := registration.IndividualRegistration{ID: uuid.New(), EventID: eventID, RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC), PlayerInfo: registration.PlayerInfo{FirstName: "P2"}}
		reg3 := registration.IndividualRegistration{ID: uuid.New(), EventID: eventID, RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC), PlayerInfo: registration.PlayerInfo{FirstName: "P3"}}

		require.NoError(t, db.CreateRegistration(ctx, reg1))
		require.NoError(t, db.CreateRegistration(ctx, reg2))
		require.NoError(t, db.CreateRegistration(ctx, reg3))

		// Fetch with limit 2
		resp, err := db.GetAllRegistrationsForEvent(ctx, eventID, nil, 2)
		a.NoError(err)
		a.Len(resp.Data, 2)
		a.True(resp.HasNextPage)
		a.NotNil(resp.Cursor)

		// Verify that the first two registrations are returned (order might vary)
		returnedIDs := make(map[uuid.UUID]bool)
		for _, r := range resp.Data {
			returnedIDs[getRegistrationID(r)] = true
		}
		a.True(returnedIDs[reg1.ID] || returnedIDs[reg2.ID] || returnedIDs[reg3.ID])
	})

	// Test case 6: Pagination - second page
	t.Run("pagination - second page", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		// Create 3 registrations
		reg1 := registration.IndividualRegistration{ID: uuid.New(), EventID: eventID, RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC), PlayerInfo: registration.PlayerInfo{FirstName: "P1"}}
		reg2 := registration.IndividualRegistration{ID: uuid.New(), EventID: eventID, RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC), PlayerInfo: registration.PlayerInfo{FirstName: "P2"}}
		reg3 := registration.IndividualRegistration{ID: uuid.New(), EventID: eventID, RegisteredAt: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC), PlayerInfo: registration.PlayerInfo{FirstName: "P3"}}

		require.NoError(t, db.CreateRegistration(ctx, reg1))
		require.NoError(t, db.CreateRegistration(ctx, reg2))
		require.NoError(t, db.CreateRegistration(ctx, reg3))

		// Fetch first page to get cursor
		resp1, err := db.GetAllRegistrationsForEvent(ctx, eventID, nil, 2)
		a.NoError(err)
		a.True(resp1.HasNextPage)
		a.NotNil(resp1.Cursor)

		// Fetch second page using the cursor
		resp2, err := db.GetAllRegistrationsForEvent(ctx, eventID, resp1.Cursor, 2)
		a.NoError(err)
		a.Len(resp2.Data, 1) // Only one remaining
		a.False(resp2.HasNextPage)
		a.Nil(resp2.Cursor)

		// Verify that the remaining registration is returned
		returnedIDs := make(map[uuid.UUID]bool)
		for _, r := range resp1.Data {
			returnedIDs[getRegistrationID(r)] = true
		}
		for _, r := range resp2.Data {
			returnedIDs[getRegistrationID(r)] = true
		}
		a.Len(returnedIDs, 3) // All three should be found across both pages
	})
}

