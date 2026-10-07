package api

import (
	"context"
	"errors"
	"testing"

	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostTeams(t *testing.T) {
	t.Run("success defaults server-owned fields", func(t *testing.T) {
		var captured teams.Team
		mock := &mockDB{
			CreateTeamFunc: func(ctx context.Context, team teams.Team) error {
				captured = team
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		reqBody := PostEventsV1TeamsJSONRequestBody{
			Name:     "Boston Renegades",
			HomeCity: "Boston, USA",
		}

		resp, err := api.PostEventsV1Teams(ctxWithLogger(context.Background(), noopLogger), PostEventsV1TeamsRequestObject{Body: &reqBody})
		require.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1Teams200JSONResponse:
			require.NotNil(t, r.Id)
			assert.Equal(t, "Boston Renegades", r.Name)
			assert.Equal(t, ACTIVE, r.Status)
			require.NotNil(t, r.Version)
			assert.Equal(t, 1, *r.Version)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Equal(t, teams.TeamStatusActive, captured.Status)
		assert.Equal(t, 1, captured.Version)
		assert.False(t, captured.CreatedAt.IsZero())
	})

	t.Run("client-supplied identity is ignored", func(t *testing.T) {
		mock := &mockDB{
			CreateTeamFunc: func(ctx context.Context, team teams.Team) error {
				assert.NotEqual(t, uuid.MustParse("00000000-0000-0000-0000-000000000000"), team.ID)
				assert.Equal(t, teams.TeamStatusActive, team.Status)
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		zeroID := uuid.MustParse("00000000-0000-0000-0000-000000000000")
		reqBody := PostEventsV1TeamsJSONRequestBody{
			Id:       &zeroID,
			Name:     "Boston Renegades",
			HomeCity: "Boston, USA",
			Status:   ARCHIVED,
		}

		resp, err := api.PostEventsV1Teams(ctxWithLogger(context.Background(), noopLogger), PostEventsV1TeamsRequestObject{Body: &reqBody})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1Teams200JSONResponse:
			assert.NotEqual(t, &zeroID, r.Id)
			assert.Equal(t, ACTIVE, r.Status)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("invalid body returns 400", func(t *testing.T) {
		mock := &mockDB{}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		for _, name := range []string{"AB", "   "} {
			reqBody := PostEventsV1TeamsJSONRequestBody{
				Name:     name,
				HomeCity: "Boston, USA",
			}

			resp, err := api.PostEventsV1Teams(ctxWithLogger(context.Background(), noopLogger), PostEventsV1TeamsRequestObject{Body: &reqBody})
			require.NoError(t, err)
			switch r := resp.(type) {
			case PostEventsV1Teams400JSONResponse:
				assert.Equal(t, InvalidBody, r.Code)
			default:
				t.Fatalf("unexpected response type for %q: %T", name, resp)
			}
		}
	})

	t.Run("taken name returns 409", func(t *testing.T) {
		mock := &mockDB{
			CreateTeamFunc: func(ctx context.Context, team teams.Team) error {
				return teams.NewTeamNameTakenError("taken", nil)
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		reqBody := PostEventsV1TeamsJSONRequestBody{
			Name:     "Boston Renegades",
			HomeCity: "Boston, USA",
		}

		resp, err := api.PostEventsV1Teams(ctxWithLogger(context.Background(), noopLogger), PostEventsV1TeamsRequestObject{Body: &reqBody})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1Teams409JSONResponse:
			assert.Equal(t, AlreadyExists, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestGetTeam(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		id := uuid.New()
		expected := teams.Team{
			ID:       id,
			Version:  1,
			Name:     "Boston Renegades",
			HomeCity: "Boston, USA",
			Status:   teams.TeamStatusActive,
		}
		mock := &mockDB{
			GetTeamFunc: func(ctx context.Context, teamID uuid.UUID) (teams.Team, error) {
				assert.Equal(t, id, teamID)
				return expected, nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		resp, err := api.GetEventsV1TeamsTeamId(ctxWithLogger(context.Background(), noopLogger), GetEventsV1TeamsTeamIdRequestObject{TeamId: id})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1TeamsTeamId200JSONResponse:
			assert.Equal(t, &id, r.Team.Id)
			assert.Equal(t, "Boston Renegades", r.Team.Name)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("not found", func(t *testing.T) {
		mock := &mockDB{
			GetTeamFunc: func(ctx context.Context, teamID uuid.UUID) (teams.Team, error) {
				return teams.Team{}, &teams.Error{Reason: teams.REASON_TEAM_DOES_NOT_EXIST}
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		resp, err := api.GetEventsV1TeamsTeamId(ctxWithLogger(context.Background(), noopLogger), GetEventsV1TeamsTeamIdRequestObject{TeamId: uuid.New()})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1TeamsTeamId404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("internal error", func(t *testing.T) {
		mock := &mockDB{
			GetTeamFunc: func(ctx context.Context, teamID uuid.UUID) (teams.Team, error) {
				return teams.Team{}, errors.New("boom")
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		resp, err := api.GetEventsV1TeamsTeamId(ctxWithLogger(context.Background(), noopLogger), GetEventsV1TeamsTeamIdRequestObject{TeamId: uuid.New()})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1TeamsTeamId500JSONResponse:
			assert.Equal(t, InternalError, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestTeamConverters(t *testing.T) {
	t.Run("round trip preserves nullable captain", func(t *testing.T) {
		captain := uuid.New()
		domain := teams.Team{
			ID:              uuid.New(),
			Version:         1,
			Name:            "Boston Renegades",
			HomeCity:        "Boston, USA",
			Status:          teams.TeamStatusActive,
			CaptainPlayerID: &captain,
		}
		apiTeam, err := teamToApiTeam(domain)
		require.NoError(t, err)
		require.NotNil(t, apiTeam.CaptainPlayerId)
		assert.Equal(t, captain, *apiTeam.CaptainPlayerId)
	})

	t.Run("nil captain round trips", func(t *testing.T) {
		domain := teams.Team{
			ID:       uuid.New(),
			Version:  1,
			Name:     "Rosterless Club",
			HomeCity: "Nowhere, USA",
			Status:   teams.TeamStatusActive,
		}
		apiTeam, err := teamToApiTeam(domain)
		require.NoError(t, err)
		assert.Nil(t, apiTeam.CaptainPlayerId)
	})
}
