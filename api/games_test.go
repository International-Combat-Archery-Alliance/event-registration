package api

import (
	"context"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/games"
	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/Rhymond/go-money"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gamesTestEvent(id uuid.UUID, status events.EventStatus, closeOffset time.Duration) events.Event {
	now := time.Now()
	return events.Event{
		ID:                    id,
		Version:               1,
		Name:                  "Games Cup",
		TimeZone:              time.UTC,
		StartTime:             now,
		EndTime:               now.Add(time.Hour),
		RegistrationCloseTime: now.Add(closeOffset),
		RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_TEAM, Price: money.New(1000, "USD")}},
		Status:                status,
	}
}

func gamesTestAPI(mock *mockDB) *API {
	return NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })
}

func gamesCtx() context.Context {
	return ctxWithLogger(context.Background(), noopLogger)
}

func TestGetGames(t *testing.T) {
	t.Run("empty until generated", func(t *testing.T) {
		eventID := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusOpened, -time.Hour), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return nil, nil
			},
		}

		resp, err := gamesTestAPI(mock).GetEventsV1EventIdGames(gamesCtx(), GetEventsV1EventIdGamesRequestObject{EventId: eventID})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1EventIdGames200JSONResponse:
			assert.Empty(t, r.Games)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("event not found", func(t *testing.T) {
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{}, &events.Error{Reason: events.REASON_EVENT_DOES_NOT_EXIST}
			},
		}

		resp, err := gamesTestAPI(mock).GetEventsV1EventIdGames(gamesCtx(), GetEventsV1EventIdGamesRequestObject{EventId: uuid.New()})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1EventIdGames404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestGenerateGuards(t *testing.T) {
	closedInProgress := func(eventID uuid.UUID) events.Event {
		return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour)
	}

	t.Run("registration open refused", func(t *testing.T) {
		eventID := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, time.Hour), nil
			},
		}
		format := ROUNDROBIN
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format}})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("event must be IN_PROGRESS", func(t *testing.T) {
		eventID := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusOpened, -time.Hour), nil
			},
		}
		format := ROUNDROBIN
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format}})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("no CONFIRMED teams refused", func(t *testing.T) {
		eventID := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return closedInProgress(eventID), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return []teams.Participation{{
					EventID: eventID, TeamID: uuid.New(),
					Status: teams.ParticipationStatusRegistered, Version: 1,
				}}, nil
			},
		}
		format := ROUNDROBIN
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format}})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("existing games refused without replace", func(t *testing.T) {
		eventID := uuid.New()
		confirmed := []teams.Participation{{EventID: eventID, TeamID: uuid.New(), Status: teams.ParticipationStatusConfirmed, Version: 1}}
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return closedInProgress(eventID), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return confirmed, nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				a, b := uuid.New(), uuid.New()
				return []games.Game{{
					ID: eventID, EventID: eventID, Phase: games.GamePhaseQualifying,
					Round: 1, Seq: 1, Status: games.GameStatusScheduled,
					SideA: games.Side{TeamID: &a}, SideB: games.Side{TeamID: &b}, Version: 1,
				}}, nil
			},
		}
		format := ROUNDROBIN
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format}})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestGenerateRoundRobinHandler(t *testing.T) {
	t.Run("success creates full schedule", func(t *testing.T) {
		eventID := uuid.New()
		teamIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
		confirmed := make([]teams.Participation, len(teamIDs))
		for i, id := range teamIDs {
			confirmed[i] = teams.Participation{EventID: eventID, TeamID: id, Status: teams.ParticipationStatusConfirmed, Version: 1}
		}
		var created []games.Game
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return confirmed, nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return nil, nil
			},
			CreateGamesFunc: func(ctx context.Context, gameList []games.Game) error {
				created = gameList
				return nil
			},
			GetTeamFunc: func(ctx context.Context, id uuid.UUID) (teams.Team, error) {
				return teams.Team{ID: id, Name: "T-" + id.String()[:4], Status: teams.TeamStatusActive}, nil
			},
		}
		format := ROUNDROBIN
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format}})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate200JSONResponse:
			assert.Len(t, r.Games, 15)
			assert.Len(t, created, 15)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestPutGame(t *testing.T) {
	t.Run("success records score with version bump", func(t *testing.T) {
		eventID, gameID := uuid.New(), uuid.New()
		a, b := uuid.New(), uuid.New()
		var written games.Game
		mock := &mockDB{
			GetGameFunc: func(ctx context.Context, eID, gID uuid.UUID) (games.Game, error) {
				return games.Game{
					ID: eventID, EventID: eventID, Phase: games.GamePhaseQualifying,
					Round: 1, Seq: 1, Status: games.GameStatusScheduled,
					SideA: games.Side{TeamID: &a}, SideB: games.Side{TeamID: &b}, Version: 1,
				}, nil
			},
			UpdateGameCheckedFunc: func(ctx context.Context, eID uuid.UUID, game games.Game) error {
				written = game
				return nil
			},
			GetTeamFunc: func(ctx context.Context, id uuid.UUID) (teams.Team, error) {
				return teams.Team{ID: id, Name: "T", Status: teams.TeamStatusActive}, nil
			},
		}
		_ = gameID
		scoreA, scoreB := 10, 5
		version := 1
		completed := COMPLETED
		resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameId(gamesCtx(),
			PutEventsV1EventIdGamesGameIdRequestObject{
				EventId: eventID, GameId: eventID,
				Body: &PutGameRequest{Status: completed, ScoreA: &scoreA, ScoreB: &scoreB, Version: version},
			})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PutEventsV1EventIdGamesGameId200JSONResponse:
			assert.Equal(t, COMPLETED, r.Game.Status)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Equal(t, 2, written.Version)
		assert.Equal(t, games.GameStatusCompleted, written.Status)
	})

	t.Run("finalized event refused", func(t *testing.T) {
		eventID := uuid.New()
		a, b := uuid.New(), uuid.New()
		mock := &mockDB{
			GetGameFunc: func(ctx context.Context, eID, gID uuid.UUID) (games.Game, error) {
				return games.Game{
					ID: eID, EventID: eID, Phase: games.GamePhaseQualifying,
					Round: 1, Seq: 1, Status: games.GameStatusScheduled,
					SideA: games.Side{TeamID: &a}, SideB: games.Side{TeamID: &b}, Version: 1,
				}, nil
			},
			UpdateGameCheckedFunc: func(ctx context.Context, eID uuid.UUID, game games.Game) error {
				return games.NewEventFinalizedError("finalized", nil)
			},
		}
		scoreA, scoreB := 10, 5
		completed := COMPLETED
		resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameId(gamesCtx(),
			PutEventsV1EventIdGamesGameIdRequestObject{
				EventId: eventID, GameId: uuid.New(),
				Body: &PutGameRequest{Status: completed, ScoreA: &scoreA, ScoreB: &scoreB, Version: 1},
			})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PutEventsV1EventIdGamesGameId409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("invalid score rejected", func(t *testing.T) {
		eventID := uuid.New()
		a, b := uuid.New(), uuid.New()
		mock := &mockDB{
			GetGameFunc: func(ctx context.Context, eID, gID uuid.UUID) (games.Game, error) {
				return games.Game{
					ID: eID, EventID: eID, Phase: games.GamePhaseQualifying,
					Round: 1, Seq: 1, Status: games.GameStatusScheduled,
					SideA: games.Side{TeamID: &a}, SideB: games.Side{TeamID: &b}, Version: 1,
				}, nil
			},
		}
		scoreA, scoreB := 5, 5
		completed := COMPLETED
		resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameId(gamesCtx(),
			PutEventsV1EventIdGamesGameIdRequestObject{
				EventId: eventID, GameId: uuid.New(),
				Body: &PutGameRequest{Status: completed, ScoreA: &scoreA, ScoreB: &scoreB, Version: 1},
			})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PutEventsV1EventIdGamesGameId400JSONResponse:
			assert.Equal(t, InvalidBody, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("stale version returns 409", func(t *testing.T) {
		eventID := uuid.New()
		a, b := uuid.New(), uuid.New()
		var writeAttempted bool
		mock := &mockDB{
			GetGameFunc: func(ctx context.Context, eID, gID uuid.UUID) (games.Game, error) {
				return games.Game{
					ID: eID, EventID: eID, Phase: games.GamePhaseQualifying,
					Round: 1, Seq: 1, Status: games.GameStatusScheduled,
					SideA: games.Side{TeamID: &a}, SideB: games.Side{TeamID: &b}, Version: 2,
				}, nil
			},
			UpdateGameCheckedFunc: func(ctx context.Context, eID uuid.UUID, game games.Game) error {
				writeAttempted = true
				return nil
			},
		}
		scoreA, scoreB := 10, 5
		completed := COMPLETED
		resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameId(gamesCtx(),
			PutEventsV1EventIdGamesGameIdRequestObject{
				EventId: eventID, GameId: uuid.New(),
				Body: &PutGameRequest{Status: completed, ScoreA: &scoreA, ScoreB: &scoreB, Version: 1},
			})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PutEventsV1EventIdGamesGameId409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.False(t, writeAttempted, "stale write must not reach storage")
	})

	t.Run("game from another event is 404", func(t *testing.T) {
		eventID := uuid.New()
		a, b := uuid.New(), uuid.New()
		mock := &mockDB{
			GetGameFunc: func(ctx context.Context, eID, gID uuid.UUID) (games.Game, error) {
				return games.Game{
					ID: gID, EventID: uuid.New(), Phase: games.GamePhaseQualifying,
					Round: 1, Seq: 1, Status: games.GameStatusScheduled,
					SideA: games.Side{TeamID: &a}, SideB: games.Side{TeamID: &b}, Version: 1,
				}, nil
			},
		}
		completed := COMPLETED
		scoreA, scoreB := 10, 5
		resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameId(gamesCtx(),
			PutEventsV1EventIdGamesGameIdRequestObject{
				EventId: eventID, GameId: uuid.New(),
				Body: &PutGameRequest{Status: completed, ScoreA: &scoreA, ScoreB: &scoreB, Version: 1},
			})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PutEventsV1EventIdGamesGameId404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestGetStandingsHandler(t *testing.T) {
	t.Run("derived with names and DNS", func(t *testing.T) {
		eventID := uuid.New()
		a, b, c := uuid.New(), uuid.New(), uuid.New()
		scoreW, scoreL := 10, 5
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return []teams.Participation{
					{EventID: eventID, TeamID: a, Status: teams.ParticipationStatusConfirmed, Version: 1},
					{EventID: eventID, TeamID: b, Status: teams.ParticipationStatusConfirmed, Version: 1},
					{EventID: eventID, TeamID: c, Status: teams.ParticipationStatusConfirmed, Version: 1},
				}, nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return []games.Game{{
					ID: uuid.New(), EventID: eventID, Phase: games.GamePhaseQualifying,
					Round: 1, Seq: 1, Status: games.GameStatusCompleted,
					SideA: games.Side{TeamID: &a}, SideB: games.Side{TeamID: &b},
					ScoreA: &scoreW, ScoreB: &scoreL, Version: 1,
				}}, nil
			},
			GetTeamFunc: func(ctx context.Context, id uuid.UUID) (teams.Team, error) {
				return teams.Team{ID: id, Name: "N-" + id.String()[:4], Status: teams.TeamStatusActive}, nil
			},
		}

		resp, err := gamesTestAPI(mock).GetEventsV1EventIdStandings(gamesCtx(), GetEventsV1EventIdStandingsRequestObject{EventId: eventID})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1EventIdStandings200JSONResponse:
			require.Len(t, r.Standings, 3)
			assert.Equal(t, a, r.Standings[0].TeamId)
			assert.Equal(t, 1, r.Standings[0].Rank)
			assert.True(t, r.Standings[2].Dns)
			assert.Equal(t, 0, r.Standings[2].Rank)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}
