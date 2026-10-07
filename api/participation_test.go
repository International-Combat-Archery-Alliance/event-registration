package api

import (
	"context"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/Rhymond/go-money"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedEvent(id uuid.UUID) events.Event {
	return events.Event{
		ID:                    id,
		Version:               1,
		Name:                  "Seed Cup",
		TimeZone:              time.UTC,
		StartTime:             time.Now(),
		EndTime:               time.Now().Add(time.Hour),
		RegistrationCloseTime: time.Now(),
		RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_TEAM, Price: money.New(1000, "USD")}},
		Status:                events.EventStatusOpened,
	}
}

func seedTeam(id uuid.UUID) teams.Team {
	return teams.Team{
		ID:       id,
		Version:  1,
		Name:     "Seed Club",
		HomeCity: "Boston, USA",
		Status:   teams.TeamStatusActive,
	}
}

func TestSeedParticipation(t *testing.T) {
	t.Run("success seeds CONFIRMED with empty snapshot", func(t *testing.T) {
		eventID, teamID := uuid.New(), uuid.New()
		var capturedParticipation teams.Participation
		var capturedHistory teams.TeamHistory
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return seedEvent(eventID), nil
			},
			GetTeamFunc: func(ctx context.Context, id uuid.UUID) (teams.Team, error) {
				return seedTeam(teamID), nil
			},
			SeedParticipationFunc: func(ctx context.Context, participation teams.Participation, history teams.TeamHistory) error {
				capturedParticipation = participation
				capturedHistory = history
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1EventIdTeamsTeamIdSeed(
			ctxWithLogger(context.Background(), noopLogger),
			PostEventsV1EventIdTeamsTeamIdSeedRequestObject{EventId: eventID, TeamId: teamID})
		require.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1EventIdTeamsTeamIdSeed200JSONResponse:
			assert.Equal(t, CONFIRMED, r.Participation.Status)
			assert.Empty(t, r.Participation.RosterSnapshot)
			assert.Equal(t, 0, r.Participation.RosterSizeAtEvent)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Equal(t, teams.ParticipationStatusConfirmed, capturedParticipation.Status)
		assert.Empty(t, capturedParticipation.RosterSnapshot)
		assert.Equal(t, "Seed Cup", capturedHistory.EventName)
		assert.Nil(t, capturedHistory.Result, "no result stamped at seed")
	})

	t.Run("missing event returns 404", func(t *testing.T) {
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{}, &events.Error{Reason: events.REASON_EVENT_DOES_NOT_EXIST}
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1EventIdTeamsTeamIdSeed(
			ctxWithLogger(context.Background(), noopLogger),
			PostEventsV1EventIdTeamsTeamIdSeedRequestObject{EventId: uuid.New(), TeamId: uuid.New()})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdTeamsTeamIdSeed404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("missing team returns 404", func(t *testing.T) {
		eventID := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return seedEvent(eventID), nil
			},
			GetTeamFunc: func(ctx context.Context, id uuid.UUID) (teams.Team, error) {
				return teams.Team{}, &teams.Error{Reason: teams.REASON_TEAM_DOES_NOT_EXIST}
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1EventIdTeamsTeamIdSeed(
			ctxWithLogger(context.Background(), noopLogger),
			PostEventsV1EventIdTeamsTeamIdSeedRequestObject{EventId: eventID, TeamId: uuid.New()})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdTeamsTeamIdSeed404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("repeat seed returns 409", func(t *testing.T) {
		eventID, teamID := uuid.New(), uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return seedEvent(eventID), nil
			},
			GetTeamFunc: func(ctx context.Context, id uuid.UUID) (teams.Team, error) {
				return seedTeam(teamID), nil
			},
			SeedParticipationFunc: func(ctx context.Context, participation teams.Participation, history teams.TeamHistory) error {
				return teams.NewParticipationAlreadyExistsError("exists", nil)
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1EventIdTeamsTeamIdSeed(
			ctxWithLogger(context.Background(), noopLogger),
			PostEventsV1EventIdTeamsTeamIdSeedRequestObject{EventId: eventID, TeamId: teamID})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdTeamsTeamIdSeed409JSONResponse:
			assert.Equal(t, AlreadyExists, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}
