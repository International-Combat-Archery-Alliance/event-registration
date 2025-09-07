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

func TestCreateRegistration(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully create an individual registration", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := &events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, *event))

		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Test City",
			Paid:       true,
			Email:      "test@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "John", LastName: "Doe"},
			Experience: registration.NOVICE,
		}

		event.Version++
		require.NoError(t, db.CreateRegistration(ctx, &reg, *event))

	})

	t.Run("successfully create a team registration", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := &events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, *event))

		reg := registration.TeamRegistration{

			ID:           uuid.New(),
			EventID:      eventID,
			Version:      1,
			HomeCity:     "Team City",
			Paid:         false,
			TeamName:     "Test Team",
			CaptainEmail: "captain@example.com",
			Players:      []registration.PlayerInfo{{FirstName: "Jane", LastName: "Doe"}},
		}

		event.Version++
		require.NoError(t, db.CreateRegistration(ctx, &reg, *event))

	})

	t.Run("fail to create a registration that already exists", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := &events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, *event))

		reg := &registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Test City",
			Paid:       true,
			Email:      "test@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "John", LastName: "Doe"},
			Experience: registration.NOVICE,
		}

		event.Version++
		require.NoError(t, db.CreateRegistration(ctx, reg, *event))

		event.Version++
		reg.Version++
		err := db.CreateRegistration(ctx, reg, *event)
		require.Error(t, err)
		var regError *registration.Error
		require.ErrorAs(t, err, &regError)
		assert.Equal(t, registration.REASON_REGISTRATION_ALREADY_EXISTS, regError.Reason)
	})
}

// getRegistrationID is a helper function to extract the ID from a Registration interface.
func getRegistrationID(reg registration.Registration) uuid.UUID {
	switch r := reg.(type) {
	case *registration.IndividualRegistration:
		return r.ID
	case *registration.TeamRegistration:
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

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		reg1 := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "City A",
			Paid:       true,
			Email:      "a@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Alice", LastName: "Smith"},
			Experience: registration.NOVICE,
		}
		reg2 := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "City B",
			Paid:       false,
			Email:      "b@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Bob", LastName: "Johnson"},
			Experience: registration.INTERMEDIATE,
		}

		event1 := events.Event{ID: reg1.EventID, Version: 2}
		require.NoError(t, db.CreateRegistration(ctx, &reg1, event1))
		event2 := events.Event{ID: reg2.EventID, Version: 3}
		require.NoError(t, db.CreateRegistration(ctx, &reg2, event2))

		resp, err := db.GetAllRegistrationsForEvent(ctx, eventID, 100, nil)
		a.NoError(err)
		a.Len(resp.Data, 2)
		a.False(resp.HasNextPage)
		a.Nil(resp.Cursor)

		// Check if both registrations are present (order might not be guaranteed)
		foundReg1 := false
		foundReg2 := false
		for _, r := range resp.Data {
			if getRegistrationID(r) == reg1.ID {
				a.Equal(reg1, *r.(*registration.IndividualRegistration))
				foundReg1 = true
			} else if getRegistrationID(r) == reg2.ID {
				a.Equal(reg2, *r.(*registration.IndividualRegistration))
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

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		teamReg1 := registration.TeamRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			Version:      1,
			HomeCity:     "Team City 1",
			Paid:         true,
			TeamName:     "Team Alpha",
			CaptainEmail: "alpha@example.com",
			Players:      []registration.PlayerInfo{{FirstName: "Charlie", LastName: "Brown"}},
		}
		teamReg2 := registration.TeamRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			Version:      1,
			HomeCity:     "Team City 2",
			Paid:         false,
			TeamName:     "Team Beta",
			CaptainEmail: "beta@example.com",
			Players:      []registration.PlayerInfo{{FirstName: "Diana", LastName: "Prince"}},
		}

		eventTeam1 := events.Event{ID: teamReg1.EventID, Version: 2}
		require.NoError(t, db.CreateRegistration(ctx, &teamReg1, eventTeam1))
		eventTeam2 := events.Event{ID: teamReg2.EventID, Version: 3}
		require.NoError(t, db.CreateRegistration(ctx, &teamReg2, eventTeam2))

		resp, err := db.GetAllRegistrationsForEvent(ctx, eventID, 100, nil)
		a.NoError(err)
		a.Len(resp.Data, 2)
		a.False(resp.HasNextPage)
		a.Nil(resp.Cursor)

		foundTeamReg1 := false
		foundTeamReg2 := false
		for _, r := range resp.Data {
			if getRegistrationID(r) == teamReg1.ID {
				a.Equal(teamReg1, *r.(*registration.TeamRegistration))
				foundTeamReg1 = true
			} else if getRegistrationID(r) == teamReg2.ID {
				a.Equal(teamReg2, *r.(*registration.TeamRegistration))
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

		resp, err := db.GetAllRegistrationsForEvent(ctx, eventID, 100, nil)
		a.NoError(err)
		a.Empty(resp.Data)
		a.False(resp.HasNextPage)
		a.Nil(resp.Cursor)
	})

	// Test case 4: Mixed individual and team registrations
	t.Run("mixed individual and team registrations", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		regIndiv := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Mixed City",
			Paid:       true,
			Email:      "mixed@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Mixed", LastName: "User"},
			Experience: registration.NOVICE,
		}
		regTeam := registration.TeamRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			Version:      1,
			HomeCity:     "Mixed Team City",
			Paid:         false,
			TeamName:     "Mixed Team",
			CaptainEmail: "mixedteam@example.com",
			Players:      []registration.PlayerInfo{{FirstName: "Mixed", LastName: "Team Player"}},
		}

		eventIndiv := events.Event{ID: regIndiv.EventID, Version: 2}
		require.NoError(t, db.CreateRegistration(ctx, &regIndiv, eventIndiv))
		eventTeam := events.Event{ID: regTeam.EventID, Version: 3}
		require.NoError(t, db.CreateRegistration(ctx, &regTeam, eventTeam))

		resp, err := db.GetAllRegistrationsForEvent(ctx, eventID, 100, nil)
		a.NoError(err)
		a.Len(resp.Data, 2)
		a.False(resp.HasNextPage)
		a.Nil(resp.Cursor)

		foundIndiv := false
		foundTeam := false
		for _, r := range resp.Data {
			if getRegistrationID(r) == regIndiv.ID {
				a.Equal(regIndiv, *r.(*registration.IndividualRegistration))
				foundIndiv = true
			} else if getRegistrationID(r) == regTeam.ID {
				a.Equal(regTeam, *r.(*registration.TeamRegistration))
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

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		// Create 3 registrations
		reg1 := registration.IndividualRegistration{ID: uuid.New(), EventID: eventID, Version: 1, Email: "email1@email.com", PlayerInfo: registration.PlayerInfo{FirstName: "P1"}}
		reg2 := registration.IndividualRegistration{ID: uuid.New(), EventID: eventID, Version: 1, Email: "email2@email.com", PlayerInfo: registration.PlayerInfo{FirstName: "P2"}}
		reg3 := registration.IndividualRegistration{ID: uuid.New(), EventID: eventID, Version: 1, Email: "email3@email.com", PlayerInfo: registration.PlayerInfo{FirstName: "P3"}}

		event1 := events.Event{ID: reg1.EventID, Version: 2}
		require.NoError(t, db.CreateRegistration(ctx, &reg1, event1))
		event2 := events.Event{ID: reg2.EventID, Version: 3}
		require.NoError(t, db.CreateRegistration(ctx, &reg2, event2))
		event3 := events.Event{ID: reg3.EventID, Version: 4}
		require.NoError(t, db.CreateRegistration(ctx, &reg3, event3))

		// Fetch with limit 2
		resp, err := db.GetAllRegistrationsForEvent(ctx, eventID, 2, nil)
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

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		// Create 3 registrations
		reg1 := registration.IndividualRegistration{ID: uuid.New(), EventID: eventID, Version: 1, Email: "email1@email.com", PlayerInfo: registration.PlayerInfo{FirstName: "P1"}}
		reg2 := registration.IndividualRegistration{ID: uuid.New(), EventID: eventID, Version: 1, Email: "email2@email.com", PlayerInfo: registration.PlayerInfo{FirstName: "P2"}}
		reg3 := registration.IndividualRegistration{ID: uuid.New(), EventID: eventID, Version: 1, Email: "email3@email.com", PlayerInfo: registration.PlayerInfo{FirstName: "P3"}}

		event1 := events.Event{ID: reg1.EventID, Version: 2}
		require.NoError(t, db.CreateRegistration(ctx, &reg1, event1))
		event2 := events.Event{ID: reg2.EventID, Version: 3}
		require.NoError(t, db.CreateRegistration(ctx, &reg2, event2))
		event3 := events.Event{ID: reg3.EventID, Version: 4}
		require.NoError(t, db.CreateRegistration(ctx, &reg3, event3))

		// Fetch first page to get cursor
		resp1, err := db.GetAllRegistrationsForEvent(ctx, eventID, 2, nil)
		a.NoError(err)
		a.True(resp1.HasNextPage)
		a.NotNil(resp1.Cursor)

		// Fetch second page using the cursor
		resp2, err := db.GetAllRegistrationsForEvent(ctx, eventID, 2, resp1.Cursor)
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
