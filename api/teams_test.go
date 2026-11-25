package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
			require.NotNil(t, r.Status)
			assert.Equal(t, ACTIVE, *r.Status)
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
		archived := ARCHIVED
		reqBody := PostEventsV1TeamsJSONRequestBody{
			Id:       &zeroID,
			Name:     "Boston Renegades",
			HomeCity: "Boston, USA",
			Status:   &archived,
		}

		resp, err := api.PostEventsV1Teams(ctxWithLogger(context.Background(), noopLogger), PostEventsV1TeamsRequestObject{Body: &reqBody})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1Teams200JSONResponse:
			assert.NotEqual(t, &zeroID, r.Id)
			require.NotNil(t, r.Status)
			assert.Equal(t, ACTIVE, *r.Status)
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

// newFullMuxHandler builds the complete generated route table on a real
// http.ServeMux, mirroring the routing half of API.ListenAndServe. Wrapping
// the served handler in the shipped OpenAPI validator is the only middleware
// that changes requests; the rest (CORS, logging, tracing, BaseNamePrefix)
// neither registers nor reshapes mux patterns.
func newFullMuxHandler(t *testing.T, api *API) http.Handler {
	t.Helper()
	swagger, err := GetSwagger()
	require.NoError(t, err)
	swagger.Servers = nil

	mux := http.NewServeMux()
	HandlerFromMux(NewStrictHandler(api, nil), mux)
	return api.openapiValidateMiddleware(swagger)(mux)
}

// TestFullGeneratedMuxConstruction registers the complete generated route
// table on a real *http.ServeMux, the same way API.ListenAndServe does. This
// is the guard for the class of bug that forced the team detail path to grow a
// "by-id" segment: /events/v1/teams/{teamId} and /events/v1/{eventId}/games
// both match /events/v1/teams/games and neither was more specific, so
// net/http's ServeMux panicked at registration. Any new ambiguous pair in the
// spec now fails this test, and the panic output names the conflicting routes.
func TestFullGeneratedMuxConstruction(t *testing.T) {
	teamID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	var gotTeamID uuid.UUID
	mock := &mockDB{
		GetTeamFunc: func(_ context.Context, id uuid.UUID) (teams.Team, error) {
			gotTeamID = id
			return teams.Team{
				ID:        id,
				Version:   1,
				Name:      "Alpha Archers",
				HomeCity:  "Anytown, USA",
				Status:    teams.TeamStatusActive,
				CreatedAt: timeFixture(),
				UpdatedAt: timeFixture(),
			}, nil
		},
	}
	api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

	var handler http.Handler
	require.NotPanics(t, func() {
		handler = newFullMuxHandler(t, api)
	}, "registering the full generated route table on net/http.ServeMux must not panic")

	t.Run("team detail request reaches the handler", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/events/v1/teams/by-id/"+teamID.String(), nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, teamID, gotTeamID)

		var body struct {
			Team Team `json:"team"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.NotNil(t, body.Team.Id)
		assert.Equal(t, teamID, *body.Team.Id)
	})

	t.Run("former ambiguous path resolves to the games operation", func(t *testing.T) {
		teamHandlerCalled := false
		otherMock := &mockDB{
			GetTeamFunc: func(_ context.Context, id uuid.UUID) (teams.Team, error) {
				teamHandlerCalled = true
				return teams.Team{}, nil
			},
		}
		otherAPI := NewAPI(otherMock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		rawMux := http.NewServeMux()
		HandlerFromMux(NewStrictHandler(otherAPI, nil), rawMux)

		req := httptest.NewRequest(http.MethodGet, "/events/v1/teams/games", nil)
		rec := httptest.NewRecorder()
		rawMux.ServeHTTP(rec, req)

		// "teams" is not a UUID event ID, so the games wrapper rejects it with
		// 400; either way the request must not fall through to a team lookup.
		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.False(t, teamHandlerCalled)
	})
}

// TestPostTeamsThroughShippedValidator exercises the real OpenAPI request
// validator. Team.status is readOnly, so the documented create body (name +
// homeCity only) must pass; sending status explicitly is rejected.
func TestPostTeamsThroughShippedValidator(t *testing.T) {
	newHandler := func(t *testing.T, mock *mockDB) http.Handler {
		t.Helper()
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })
		return newFullMuxHandler(t, api)
	}

	post := func(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/events/v1/teams", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+generateTestToken("admin@example.com", true))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	t.Run("documented body without status passes", func(t *testing.T) {
		var created teams.Team
		mock := &mockDB{
			CreateTeamFunc: func(ctx context.Context, team teams.Team) error {
				created = team
				return nil
			},
		}
		rec := post(t, newHandler(t, mock), `{"name":"Boston Renegades","homeCity":"Boston, USA"}`)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "Boston Renegades", created.Name)
		assert.Equal(t, teams.TeamStatusActive, created.Status)
	})

	t.Run("explicit readOnly status is rejected", func(t *testing.T) {
		mock := &mockDB{
			CreateTeamFunc: func(ctx context.Context, team teams.Team) error {
				t.Fatal("a body carrying a readOnly property must not reach the handler")
				return nil
			},
		}
		rec := post(t, newHandler(t, mock), `{"name":"Boston Renegades","homeCity":"Boston, USA","status":"ARCHIVED"}`)
		require.Equal(t, http.StatusBadRequest, rec.Code)

		var e Error
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &e))
		assert.Equal(t, InputValidationError, e.Code)
		assert.Contains(t, e.Message, "readOnly")
	})
}

func TestGetTeams(t *testing.T) {
	t.Run("returns a page with cursor", func(t *testing.T) {
		cursor := "next-page"
		now := timeFixture()
		mock := &mockDB{
			ListTeamsFunc: func(ctx context.Context, limit int32, gotCursor *string) (teams.GetTeamsResponse, error) {
				assert.Equal(t, int32(2), limit)
				if gotCursor != nil {
					assert.Equal(t, "start", *gotCursor)
				}
				return teams.GetTeamsResponse{
					Data: []teams.Team{{
						ID: uuid.New(), Version: 1, Name: "Alpha Archers", HomeCity: "Anytown, USA",
						Status: teams.TeamStatusActive, CreatedAt: now, UpdatedAt: now,
					}},
					Cursor:      &cursor,
					HasNextPage: true,
				}, nil
			},
		}
		start := "start"
		limit := 2
		resp, err := gamesTestAPI(mock).GetEventsV1Teams(gamesCtx(), GetEventsV1TeamsRequestObject{
			Params: GetEventsV1TeamsParams{Limit: &limit, Cursor: &start},
		})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1Teams200JSONResponse:
			require.Len(t, r.Teams, 1)
			assert.Equal(t, "Alpha Archers", r.Teams[0].Name)
			require.NotNil(t, r.Cursor)
			assert.Equal(t, cursor, *r.Cursor)
			assert.True(t, r.HasNextPage)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("invalid cursor is a 400", func(t *testing.T) {
		mock := &mockDB{
			ListTeamsFunc: func(ctx context.Context, limit int32, gotCursor *string) (teams.GetTeamsResponse, error) {
				return teams.GetTeamsResponse{}, teams.NewInvalidCursorError("bad cursor", nil)
			},
		}
		resp, err := gamesTestAPI(mock).GetEventsV1Teams(gamesCtx(), GetEventsV1TeamsRequestObject{})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1Teams400JSONResponse:
			assert.Equal(t, InvalidCursor, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("storage failure is a 500", func(t *testing.T) {
		mock := &mockDB{
			ListTeamsFunc: func(ctx context.Context, limit int32, gotCursor *string) (teams.GetTeamsResponse, error) {
				return teams.GetTeamsResponse{}, errors.New("boom")
			},
		}
		resp, err := gamesTestAPI(mock).GetEventsV1Teams(gamesCtx(), GetEventsV1TeamsRequestObject{})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1Teams500JSONResponse:
			assert.Equal(t, InternalError, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func timeFixture() time.Time {
	return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
}
