package dynamo

import (
	"context"
	"testing"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
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

func TestGetRegistration(t *testing.T) {
	ctx := context.Background()
	a := assert.New(t)

	t.Run("successfully get individual registration", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Test City",
			Paid:       true,
			Email:      "test@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "John", LastName: "Doe"},
			Experience: registration.ADVANCED,
		}

		event2 := events.Event{ID: eventID, Version: 2}
		require.NoError(t, db.CreateRegistration(ctx, &reg, event2))

		retrieved, err := db.GetRegistration(ctx, eventID, "test@example.com")
		a.NoError(err)
		a.Equal(reg, *retrieved.(*registration.IndividualRegistration))
	})

	t.Run("successfully get team registration", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		reg := registration.TeamRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			Version:      1,
			HomeCity:     "Team City",
			Paid:         false,
			TeamName:     "Test Team",
			CaptainEmail: "captain@example.com",
			Players:      []registration.PlayerInfo{{FirstName: "Jane", LastName: "Smith"}},
		}

		event2 := events.Event{ID: eventID, Version: 2}
		require.NoError(t, db.CreateRegistration(ctx, &reg, event2))

		retrieved, err := db.GetRegistration(ctx, eventID, "captain@example.com")
		a.NoError(err)
		a.Equal(reg, *retrieved.(*registration.TeamRegistration))
	})

	t.Run("registration does not exist", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		_, err := db.GetRegistration(ctx, eventID, "nonexistent@example.com")
		a.Error(err)
		var regError *registration.Error
		require.ErrorAs(t, err, &regError)
		a.Equal(registration.REASON_REGISTRATION_DOES_NOT_EXIST, regError.Reason)
	})
}

func TestCreateRegistrationWithPayment(t *testing.T) {
	ctx := context.Background()
	a := assert.New(t)

	t.Run("successfully create individual registration with payment intent", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Payment City",
			Paid:       false, // Should be false initially
			Email:      "payment@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Payment", LastName: "User"},
			Experience: registration.NOVICE,
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "stripe_session_123",
			Email:            "payment@example.com",
		}

		event2 := events.Event{ID: eventID, Version: 2}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, event2)
		a.NoError(err)

		// Verify registration was created
		retrieved, err := db.GetRegistration(ctx, eventID, "payment@example.com")
		a.NoError(err)
		a.Equal(reg, *retrieved.(*registration.IndividualRegistration))
		a.False(retrieved.(*registration.IndividualRegistration).Paid) // Should still be unpaid
	})

	t.Run("successfully create team registration with payment intent", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		reg := registration.TeamRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			Version:      1,
			HomeCity:     "Payment Team City",
			Paid:         false, // Should be false initially
			TeamName:     "Payment Team",
			CaptainEmail: "team-payment@example.com",
			Players:      []registration.PlayerInfo{{FirstName: "Team", LastName: "Player"}},
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "stripe_session_456",
			Email:            "team-payment@example.com",
		}

		event2 := events.Event{ID: eventID, Version: 2}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, event2)
		a.NoError(err)

		// Verify registration was created
		retrieved, err := db.GetRegistration(ctx, eventID, "team-payment@example.com")
		a.NoError(err)
		a.Equal(reg, *retrieved.(*registration.TeamRegistration))
		a.False(retrieved.(*registration.TeamRegistration).Paid) // Should still be unpaid
	})

	t.Run("successfully create individual registration with payment intent and player email", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Payment City",
			Paid:       false, // Should be false initially
			Email:      "payment-with-player-email@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Payment", LastName: "User", Email: ptr.String("player.payment@example.com")},
			Experience: registration.NOVICE,
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "stripe_session_player_email",
			Email:            "payment-with-player-email@example.com",
		}

		event2 := events.Event{ID: eventID, Version: 2}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, event2)
		a.NoError(err)

		// Verify registration was created with player email preserved
		retrieved, err := db.GetRegistration(ctx, eventID, "payment-with-player-email@example.com")
		a.NoError(err)
		indivReg := retrieved.(*registration.IndividualRegistration)
		a.Equal(reg, *indivReg)
		a.False(indivReg.Paid) // Should still be unpaid
		a.Equal("player.payment@example.com", *indivReg.PlayerInfo.Email)
	})

	t.Run("successfully create individual registration with payment intent but no player email", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Payment City No Email",
			Paid:       false,
			Email:      "payment-no-player-email@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Payment", LastName: "NoEmail", Email: nil},
			Experience: registration.INTERMEDIATE,
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "stripe_session_no_player_email",
			Email:            "payment-no-player-email@example.com",
		}

		event2 := events.Event{ID: eventID, Version: 2}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, event2)
		a.NoError(err)

		// Verify registration was created with nil player email preserved
		retrieved, err := db.GetRegistration(ctx, eventID, "payment-no-player-email@example.com")
		a.NoError(err)
		indivReg := retrieved.(*registration.IndividualRegistration)
		a.Equal(reg, *indivReg)
		a.False(indivReg.Paid)
		a.Nil(indivReg.PlayerInfo.Email)
	})

	t.Run("successfully create team registration with payment intent and mixed player emails", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		reg := registration.TeamRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			Version:      1,
			HomeCity:     "Payment Team City Mixed",
			Paid:         false,
			TeamName:     "Payment Team Mixed Emails",
			CaptainEmail: "team-payment-mixed@example.com",
			Players: []registration.PlayerInfo{
				{FirstName: "TeamPlayer1", LastName: "WithEmail", Email: ptr.String("teamplayer1@example.com")},
				{FirstName: "TeamPlayer2", LastName: "NoEmail", Email: nil},
				{FirstName: "TeamPlayer3", LastName: "EmptyEmail", Email: ptr.String("")},
				{FirstName: "TeamPlayer4", LastName: "AlsoWithEmail", Email: ptr.String("teamplayer4@example.com")},
			},
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "stripe_session_team_mixed",
			Email:            "team-payment-mixed@example.com",
		}

		event2 := events.Event{ID: eventID, Version: 2}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, event2)
		a.NoError(err)

		// Verify registration was created with all player email variations preserved
		retrieved, err := db.GetRegistration(ctx, eventID, "team-payment-mixed@example.com")
		a.NoError(err)
		teamReg := retrieved.(*registration.TeamRegistration)
		a.Equal(reg, *teamReg)
		a.False(teamReg.Paid)

		require.Len(t, teamReg.Players, 4)
		// Player 1 - has email
		a.Equal("TeamPlayer1", teamReg.Players[0].FirstName)
		a.NotNil(teamReg.Players[0].Email)
		a.Equal("teamplayer1@example.com", *teamReg.Players[0].Email)

		// Player 2 - no email (nil)
		a.Equal("TeamPlayer2", teamReg.Players[1].FirstName)
		a.Nil(teamReg.Players[1].Email)

		// Player 3 - empty email string
		a.Equal("TeamPlayer3", teamReg.Players[2].FirstName)
		a.NotNil(teamReg.Players[2].Email)
		a.Equal("", *teamReg.Players[2].Email)

		// Player 4 - has email
		a.Equal("TeamPlayer4", teamReg.Players[3].FirstName)
		a.NotNil(teamReg.Players[3].Email)
		a.Equal("teamplayer4@example.com", *teamReg.Players[3].Email)
	})

	t.Run("fail when registration already exists", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Duplicate City",
			Paid:       false,
			Email:      "duplicate@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Duplicate", LastName: "User"},
			Experience: registration.NOVICE,
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "stripe_session_789",
			Email:            "duplicate@example.com",
		}

		// First registration should succeed
		event2 := events.Event{ID: eventID, Version: 2}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, event2)
		a.NoError(err)

		// Second registration should fail
		reg.ID = uuid.New()   // Different ID but same email
		regIntent.Version = 1 // Reset version
		event3 := events.Event{ID: eventID, Version: 3}
		err = db.CreateRegistrationWithPayment(ctx, &reg, regIntent, event3)
		a.Error(err)
		var regError *registration.Error
		require.ErrorAs(t, err, &regError)
		a.Equal(registration.REASON_REGISTRATION_ALREADY_EXISTS, regError.Reason)
	})
}

func TestUpdateRegistrationToPaid(t *testing.T) {
	ctx := context.Background()
	a := assert.New(t)

	t.Run("successfully update individual registration to paid", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Update City",
			Paid:       false, // Start unpaid
			Email:      "update@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Update", LastName: "User"},
			Experience: registration.INTERMEDIATE,
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "stripe_session_update",
			Email:            "update@example.com",
		}

		// Create registration with payment intent
		event2 := events.Event{ID: eventID, Version: 2}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, event2)
		a.NoError(err)

		// Update to paid
		reg.Paid = true
		reg.Version = 2
		err = db.UpdateRegistrationToPaid(ctx, &reg)
		a.NoError(err)

		// Verify registration is now paid
		retrieved, err := db.GetRegistration(ctx, eventID, "update@example.com")
		a.NoError(err)
		a.True(retrieved.(*registration.IndividualRegistration).Paid)
		a.Equal(2, retrieved.(*registration.IndividualRegistration).Version)
	})

	t.Run("successfully update team registration to paid", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		reg := registration.TeamRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			Version:      1,
			HomeCity:     "Update Team City",
			Paid:         false, // Start unpaid
			TeamName:     "Update Team",
			CaptainEmail: "team-update@example.com",
			Players:      []registration.PlayerInfo{{FirstName: "Team", LastName: "Update"}},
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "stripe_session_team_update",
			Email:            "team-update@example.com",
		}

		// Create registration with payment intent
		event2 := events.Event{ID: eventID, Version: 2}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, event2)
		a.NoError(err)

		// Update to paid
		reg.Paid = true
		reg.Version = 2
		err = db.UpdateRegistrationToPaid(ctx, &reg)
		a.NoError(err)

		// Verify registration is now paid
		retrieved, err := db.GetRegistration(ctx, eventID, "team-update@example.com")
		a.NoError(err)
		a.True(retrieved.(*registration.TeamRegistration).Paid)
		a.Equal(2, retrieved.(*registration.TeamRegistration).Version)
	})

	t.Run("fail when registration does not exist", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Nonexistent City",
			Paid:       true,
			Email:      "nonexistent@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Nonexistent", LastName: "User"},
			Experience: registration.NOVICE,
		}

		err := db.UpdateRegistrationToPaid(ctx, &reg)
		a.Error(err)
		var regError *registration.Error
		require.ErrorAs(t, err, &regError)
		a.Equal(registration.REASON_FAILED_TO_WRITE, regError.Reason)
	})

	t.Run("fail when version conflict occurs", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Version City",
			Paid:       false,
			Email:      "version@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Version", LastName: "User"},
			Experience: registration.ADVANCED,
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "stripe_session_version",
			Email:            "version@example.com",
		}

		// Create registration with payment intent
		event2 := events.Event{ID: eventID, Version: 2}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, event2)
		a.NoError(err)

		// Try to update with wrong version (should be 2, but we're using 3 to simulate stale data)
		reg.Paid = true
		reg.Version = 3 // Wrong version - too high
		err = db.UpdateRegistrationToPaid(ctx, &reg)
		a.Error(err)
		var regError *registration.Error
		require.ErrorAs(t, err, &regError)
		// The actual error depends on DynamoDB's condition check - could be either
		a.Equal(registration.REASON_FAILED_TO_WRITE, regError.Reason)
	})
}

func TestDeleteExpiredRegistration(t *testing.T) {
	ctx := context.Background()
	a := assert.New(t)

	t.Run("successfully delete expired individual registration", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		// Create initial event
		event := events.Event{
			ID:              eventID,
			Version:         1,
			NumTotalPlayers: 0,
		}
		require.NoError(t, db.CreateEvent(ctx, event))

		// Create individual registration with payment intent
		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Expired City",
			Paid:       false,
			Email:      "expired@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Expired", LastName: "User"},
			Experience: registration.INTERMEDIATE,
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "expired_stripe_session",
			Email:            "expired@example.com",
		}

		// Create registration with payment intent and updated event counters
		eventWithRegistration := events.Event{
			ID:              eventID,
			Version:         2,
			NumTotalPlayers: 1, // Should be incremented when registration is created
		}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, eventWithRegistration)
		a.NoError(err)

		// Now delete the expired registration - event counters should be decremented
		eventForDeletion := events.Event{
			ID:              eventID,
			Version:         3, // Version should be incremented for the delete operation
			NumTotalPlayers: 0, // Should be decremented back to 0
		}

		err = db.DeleteExpiredRegistration(ctx, &reg, regIntent, eventForDeletion)
		a.NoError(err)

		// Verify registration is deleted
		_, err = db.GetRegistration(ctx, eventID, "expired@example.com")
		a.Error(err)
		var regError *registration.Error
		require.ErrorAs(t, err, &regError)
		a.Equal(registration.REASON_REGISTRATION_DOES_NOT_EXIST, regError.Reason)

		// Verify registration intent is deleted
		_, err = db.GetRegistrationIntent(ctx, eventID, "expired@example.com")
		a.Error(err)
		require.ErrorAs(t, err, &regError)
		a.Equal(registration.REASON_REGISTRATION_DOES_NOT_EXIST, regError.Reason)

		// Verify event was updated with new version and decremented counters
		updatedEvent, err := db.GetEvent(ctx, eventID)
		a.NoError(err)
		a.Equal(3, updatedEvent.Version)
		a.Equal(0, updatedEvent.NumTotalPlayers)
	})

	t.Run("successfully delete expired team registration", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		// Create initial event
		event := events.Event{
			ID:                 eventID,
			Version:            1,
			NumTotalPlayers:    0,
			NumTeams:           0,
			NumRosteredPlayers: 0,
		}
		require.NoError(t, db.CreateEvent(ctx, event))

		// Create team registration with payment intent
		reg := registration.TeamRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			Version:      1,
			HomeCity:     "Expired Team City",
			Paid:         false,
			TeamName:     "Expired Team",
			CaptainEmail: "expired-team@example.com",
			Players:      []registration.PlayerInfo{{FirstName: "Player1", LastName: "Team"}, {FirstName: "Player2", LastName: "Team"}}, // 2 players
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "expired_team_stripe_session",
			Email:            "expired-team@example.com",
		}

		// Create registration with payment intent and updated event counters
		eventWithRegistration := events.Event{
			ID:                 eventID,
			Version:            2,
			NumTotalPlayers:    2, // Should be incremented by team size
			NumTeams:           1, // Should be incremented by 1
			NumRosteredPlayers: 2, // Should be incremented by team size
		}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, eventWithRegistration)
		a.NoError(err)

		// Now delete the expired registration - event counters should be decremented
		eventForDeletion := events.Event{
			ID:                 eventID,
			Version:            3, // Version should be incremented for the delete operation
			NumTotalPlayers:    0, // Should be decremented back to 0
			NumTeams:           0, // Should be decremented back to 0
			NumRosteredPlayers: 0, // Should be decremented back to 0
		}

		err = db.DeleteExpiredRegistration(ctx, &reg, regIntent, eventForDeletion)
		a.NoError(err)

		// Verify registration is deleted
		_, err = db.GetRegistration(ctx, eventID, "expired-team@example.com")
		a.Error(err)
		var regError *registration.Error
		require.ErrorAs(t, err, &regError)
		a.Equal(registration.REASON_REGISTRATION_DOES_NOT_EXIST, regError.Reason)

		// Verify registration intent is deleted
		_, err = db.GetRegistrationIntent(ctx, eventID, "expired-team@example.com")
		a.Error(err)
		require.ErrorAs(t, err, &regError)
		a.Equal(registration.REASON_REGISTRATION_DOES_NOT_EXIST, regError.Reason)

		// Verify event was updated with new version and decremented counters
		updatedEvent, err := db.GetEvent(ctx, eventID)
		a.NoError(err)
		a.Equal(3, updatedEvent.Version)
		a.Equal(0, updatedEvent.NumTotalPlayers)
		a.Equal(0, updatedEvent.NumTeams)
		a.Equal(0, updatedEvent.NumRosteredPlayers)
	})

	t.Run("fail when registration does not exist", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		// Create initial event
		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		// Try to delete non-existent registration
		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			Email:      "nonexistent@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Nonexistent", LastName: "User"},
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "nonexistent_session",
			Email:            "nonexistent@example.com",
		}

		eventForDeletion := events.Event{
			ID:      eventID,
			Version: 2,
		}

		err := db.DeleteExpiredRegistration(ctx, &reg, regIntent, eventForDeletion)
		a.Error(err)
		var regError *registration.Error
		require.ErrorAs(t, err, &regError)
		a.Equal(registration.REASON_FAILED_TO_WRITE, regError.Reason)
	})

	t.Run("fail when registration intent does not exist", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		// Create initial event
		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		// Create only registration without intent
		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "No Intent City",
			Paid:       true, // Already paid, so no intent should exist
			Email:      "nointent@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "NoIntent", LastName: "User"},
			Experience: registration.NOVICE,
		}

		event2 := events.Event{ID: eventID, Version: 2}
		err := db.CreateRegistration(ctx, &reg, event2)
		a.NoError(err)

		// Try to delete with a non-existent intent
		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "nonexistent_session",
			Email:            "nointent@example.com",
		}

		eventForDeletion := events.Event{
			ID:      eventID,
			Version: 3,
		}

		err = db.DeleteExpiredRegistration(ctx, &reg, regIntent, eventForDeletion)
		a.Error(err)
		var regError *registration.Error
		require.ErrorAs(t, err, &regError)
		a.Equal(registration.REASON_FAILED_TO_WRITE, regError.Reason)
	})

	t.Run("fail when event version conflict occurs", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		// Create initial event
		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		// Create registration with payment intent
		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Version Conflict City",
			Paid:       false,
			Email:      "version@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "Version", LastName: "User"},
			Experience: registration.INTERMEDIATE,
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "version_conflict_session",
			Email:            "version@example.com",
		}

		eventWithRegistration := events.Event{
			ID:              eventID,
			Version:         2,
			NumTotalPlayers: 1,
		}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, eventWithRegistration)
		a.NoError(err)

		// Try to delete with wrong event version (simulate stale data)
		eventForDeletion := events.Event{
			ID:              eventID,
			Version:         5, // Wrong version - too high
			NumTotalPlayers: 0,
		}

		err = db.DeleteExpiredRegistration(ctx, &reg, regIntent, eventForDeletion)
		a.Error(err)
		var regError *registration.Error
		require.ErrorAs(t, err, &regError)
		a.Equal(registration.REASON_FAILED_TO_WRITE, regError.Reason)

		// Verify registration still exists (not deleted due to version conflict)
		retrievedReg, err := db.GetRegistration(ctx, eventID, "version@example.com")
		a.NoError(err)
		a.NotNil(retrievedReg)
	})

	t.Run("fail when registration version conflict occurs", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()

		// Create initial event
		event := events.Event{ID: eventID, Version: 1}
		require.NoError(t, db.CreateEvent(ctx, event))

		// Create registration with payment intent
		reg := registration.IndividualRegistration{
			ID:         uuid.New(),
			EventID:    eventID,
			Version:    1,
			HomeCity:   "Reg Version Conflict City",
			Paid:       false,
			Email:      "regversion@example.com",
			PlayerInfo: registration.PlayerInfo{FirstName: "RegVersion", LastName: "User"},
			Experience: registration.ADVANCED,
		}

		regIntent := registration.RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "reg_version_conflict_session",
			Email:            "regversion@example.com",
		}

		eventWithRegistration := events.Event{
			ID:              eventID,
			Version:         2,
			NumTotalPlayers: 1,
		}
		err := db.CreateRegistrationWithPayment(ctx, &reg, regIntent, eventWithRegistration)
		a.NoError(err)

		// Try to delete with wrong registration version
		regWithWrongVersion := reg
		regWithWrongVersion.Version = 5 // Wrong version - too high

		eventForDeletion := events.Event{
			ID:              eventID,
			Version:         3,
			NumTotalPlayers: 0,
		}

		err = db.DeleteExpiredRegistration(ctx, &regWithWrongVersion, regIntent, eventForDeletion)
		a.Error(err)
		var regError *registration.Error
		require.ErrorAs(t, err, &regError)
		a.Equal(registration.REASON_FAILED_TO_WRITE, regError.Reason)

		// Verify registration still exists (not deleted due to version conflict)
		retrievedReg, err := db.GetRegistration(ctx, eventID, "regversion@example.com")
		a.NoError(err)
		a.NotNil(retrievedReg)
	})
}
