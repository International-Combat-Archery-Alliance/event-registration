package api

import (
	"context"
	"errors"
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

func ptrInt(i int) *int {
	return &i
}

func confirmedParticipations(eventID uuid.UUID, ids []uuid.UUID) []teams.Participation {
	out := make([]teams.Participation, len(ids))
	for i, id := range ids {
		out[i] = teams.Participation{EventID: eventID, TeamID: id, Status: teams.ParticipationStatusConfirmed, Version: 1}
	}
	return out
}

func scheduledGame(eventID uuid.UUID, gameNo, round, seq int, a, b uuid.UUID) games.Game {
	return games.Game{
		ID:      uuid.New(),
		EventID: eventID,
		GameNo:  gameNo,
		Phase:   games.GamePhaseQualifying,
		Round:   round,
		Seq:     seq,
		Status:  games.GameStatusScheduled,
		SideA:   games.Side{TeamID: &a},
		SideB:   games.Side{TeamID: &b},
		Version: 1,
	}
}

func scheduledRound(eventID uuid.UUID, round, startNo int, ids []uuid.UUID) []games.Game {
	var out []games.Game
	seq := 1
	for i := 0; i+1 < len(ids); i += 2 {
		out = append(out, scheduledGame(eventID, startNo+seq-1, round, seq, ids[i], ids[i+1]))
		seq++
	}
	return out
}

func testTeamNames() func(context.Context, uuid.UUID) (teams.Team, error) {
	return func(ctx context.Context, id uuid.UUID) (teams.Team, error) {
		return teams.Team{ID: id, Name: "T-" + id.String()[:4], Status: teams.TeamStatusActive}, nil
	}
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

	t.Run("exposes the minted game number", func(t *testing.T) {
		eventID := uuid.New()
		a, b := uuid.New(), uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusOpened, -time.Hour), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return []games.Game{scheduledGame(eventID, 42, 1, 1, a, b)}, nil
			},
			GetTeamFunc: testTeamNames(),
		}

		resp, err := gamesTestAPI(mock).GetEventsV1EventIdGames(gamesCtx(), GetEventsV1EventIdGamesRequestObject{EventId: eventID})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1EventIdGames200JSONResponse:
			require.Len(t, r.Games, 1)
			require.NotNil(t, r.Games[0].GameNo)
			assert.Equal(t, 42, *r.Games[0].GameNo)
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

	t.Run("missing team names degrade instead of failing", func(t *testing.T) {
		eventID := uuid.New()
		a, b := uuid.New(), uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusOpened, -time.Hour), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return []games.Game{scheduledGame(eventID, 1, 1, 1, a, b)}, nil
			},
			GetTeamFunc: func(ctx context.Context, id uuid.UUID) (teams.Team, error) {
				return teams.Team{}, errors.New("team directory unavailable")
			},
		}

		resp, err := gamesTestAPI(mock).GetEventsV1EventIdGames(gamesCtx(), GetEventsV1EventIdGamesRequestObject{EventId: eventID})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1EventIdGames200JSONResponse:
			require.Len(t, r.Games, 1)
			assert.Nil(t, r.Games[0].SideA.TeamName)
			assert.Nil(t, r.Games[0].SideB.TeamName)
		default:
			t.Fatalf("a cosmetic name join must not fail the read: %T", resp)
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
		confirmed := confirmedParticipations(eventID, []uuid.UUID{uuid.New()})
		a, b := uuid.New(), uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return closedInProgress(eventID), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return confirmed, nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return []games.Game{scheduledGame(eventID, 1, 1, 1, a, b)}, nil
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
	t.Run("success creates full schedule with minted numbers", func(t *testing.T) {
		eventID := uuid.New()
		teamIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
		var created []games.Game
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return confirmedParticipations(eventID, teamIDs), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return nil, nil
			},
			CreateGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameList []games.Game) error {
				created = gameList
				return nil
			},
			GetTeamFunc: testTeamNames(),
		}
		format := ROUNDROBIN
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format}})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate200JSONResponse:
			assert.Len(t, r.Games, 15)
			assert.Len(t, created, 15)
			for i, g := range created {
				assert.Equal(t, i+1, g.GameNo)
			}
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("replace grants after validation, then reserves, deletes, and writes", func(t *testing.T) {
		eventID := uuid.New()
		teamIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
		a, b := uuid.New(), uuid.New()
		var calls []string
		var deleted []int
		var reserved int
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return confirmedParticipations(eventID, teamIDs), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return []games.Game{scheduledGame(eventID, 7, 1, 1, a, b)}, nil
			},
			BeginScheduleWriteFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int) (int, error) {
				calls = append(calls, "grant")
				return expectedVersion + 1, nil
			},
			DeleteGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameNos []int) error {
				calls = append(calls, "delete")
				deleted = gameNos
				return nil
			},
			ReserveGameNumbersFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int, lastGameNo int) (int, error) {
				calls = append(calls, "reserve")
				reserved = lastGameNo
				return expectedVersion + 1, nil
			},
			CreateGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameList []games.Game) error {
				calls = append(calls, "create")
				return nil
			},
			GetTeamFunc: testTeamNames(),
		}
		format, replace := ROUNDROBIN, true
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Replace: &replace}})
		require.NoError(t, err)
		switch resp.(type) {
		case PostEventsV1EventIdGamesGenerate200JSONResponse:
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Equal(t, []string{"grant", "reserve", "delete", "create"}, calls)
		assert.Equal(t, []int{7}, deleted)
		assert.Equal(t, 22, reserved, "the high-water mark reserves every minted number (7 + 15)")
	})

	t.Run("generation failure deletes nothing", func(t *testing.T) {
		eventID := uuid.New()
		a, b := uuid.New(), uuid.New()
		var deleted, granted bool
		format, replace := ROUNDROBIN, true
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				// A one-team field fails pure generation.
				return confirmedParticipations(eventID, []uuid.UUID{uuid.New()}), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return []games.Game{scheduledGame(eventID, 1, 1, 1, a, b)}, nil
			},
			BeginScheduleWriteFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int) (int, error) {
				granted = true
				return expectedVersion + 1, nil
			},
			DeleteScheduledGameFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int, game games.Game) error {
				deleted = true
				return nil
			},
			GetTeamFunc: testTeamNames(),
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Replace: &replace}})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate400JSONResponse:
			assert.Contains(t, r.Message, "field must have between")
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.False(t, granted, "a failed build must not take the epoch grant")
		assert.False(t, deleted, "a generation failure must not delete the old schedule")
	})

	t.Run("mid-flight result aborts the replace instead of destroying it", func(t *testing.T) {
		eventID := uuid.New()
		teamIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
		a, b := uuid.New(), uuid.New()
		var created bool
		format, replace := ROUNDROBIN, true
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return confirmedParticipations(eventID, teamIDs), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return []games.Game{scheduledGame(eventID, 1, 1, 1, a, b)}, nil
			},
			DeleteGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameNos []int) error {
				// A result lands between the read and the conditional delete.
				return games.NewVersionConflictError("no longer SCHEDULED", nil)
			},
			CreateGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameList []games.Game) error {
				created = true
				return nil
			},
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Replace: &replace}})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.False(t, created, "the new schedule must not be written after a refused delete")
	})

	t.Run("replace refuses a result that conflicts with the regenerated pairing", func(t *testing.T) {
		eventID := uuid.New()
		teamIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
		a, b := uuid.New(), uuid.New()
		completed := scheduledGame(eventID, 1, 1, 1, a, b)
		completed.Status = games.GameStatusCompleted
		completed.ScoreA, completed.ScoreB = ptrInt(10), ptrInt(5)
		format, replace := ROUNDROBIN, true
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return confirmedParticipations(eventID, teamIDs), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return []games.Game{completed}, nil
			},
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Replace: &replace}})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestGenerateRoundRobinReplaceScope(t *testing.T) {
	eventID := uuid.New()
	teamIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	a, b := uuid.New(), uuid.New()

	qualifying := scheduledGame(eventID, 1, 1, 1, a, b)
	playoff := scheduledGame(eventID, 2, 1, 1, a, b)
	playoff.Phase = games.GamePhasePlayoff

	var deleted []int
	format, replace := ROUNDROBIN, true
	mock := &mockDB{
		GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
			return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
		},
		ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
			return confirmedParticipations(eventID, teamIDs), nil
		},
		ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
			return []games.Game{qualifying, playoff}, nil
		},
		DeleteGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameNos []int) error {
			deleted = gameNos
			return nil
		},
		CreateGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameList []games.Game) error {
			return nil
		},
		GetTeamFunc: testTeamNames(),
	}
	resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
		PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Replace: &replace}})
	require.NoError(t, err)
	switch resp.(type) {
	case PostEventsV1EventIdGamesGenerate200JSONResponse:
	default:
		t.Fatalf("unexpected response type: %T", resp)
	}
	assert.Equal(t, []int{1}, deleted, "only QUALIFYING SCHEDULED rows are replaced")
}

func TestGenerateSwiss(t *testing.T) {
	sixTeams := func() []uuid.UUID {
		return []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	}

	t.Run("replace of a non-last round succeeds", func(t *testing.T) {
		eventID := uuid.New()
		ids := sixTeams()
		existing := append(scheduledRound(eventID, 1, 1, ids), scheduledRound(eventID, 2, 4, ids)...)
		existing = append(existing, scheduledRound(eventID, 3, 7, ids)...)

		var deleted []int
		format, replace, round := SWISS, true, 2
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return confirmedParticipations(eventID, ids), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return existing, nil
			},
			DeleteGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameNos []int) error {
				deleted = gameNos
				return nil
			},
			CreateGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameList []games.Game) error {
				return nil
			},
			GetTeamFunc: testTeamNames(),
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Replace: &replace, Round: &round}})
		require.NoError(t, err)
		switch resp.(type) {
		case PostEventsV1EventIdGamesGenerate200JSONResponse:
		default:
			t.Fatalf("replacing a non-last round must now succeed: %T", resp)
		}
		assert.Equal(t, []int{4, 5, 6}, deleted, "only the target round's SCHEDULED rows")
	})

	t.Run("replace preserves a result whose slot still holds the same pairing", func(t *testing.T) {
		eventID := uuid.New()
		ids := sixTeams()
		names := map[string]string{}
		for _, id := range ids {
			team, err := testTeamNames()(gamesCtx(), id)
			require.NoError(t, err)
			names[id.String()] = team.Name
		}
		roundOne := scheduledRound(eventID, 1, 1, ids)
		roundTwo, err := games.GenerateSwissRound(gamesCtx(), eventID, ids, names, roundOne, 2, nil)
		require.NoError(t, err)
		for i := range roundTwo {
			roundTwo[i].GameNo = 4 + i
		}
		roundTwo[0].Status = games.GameStatusCompleted
		roundTwo[0].ScoreA, roundTwo[0].ScoreB = ptrInt(10), ptrInt(5)
		roundTwo[0].Version = 2

		var deleted []int
		var created []games.Game
		format, replace, round := SWISS, true, 2
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return confirmedParticipations(eventID, ids), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return append(append([]games.Game{}, roundOne...), roundTwo...), nil
			},
			DeleteGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameNos []int) error {
				deleted = gameNos
				return nil
			},
			CreateGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameList []games.Game) error {
				created = gameList
				return nil
			},
			GetTeamFunc: testTeamNames(),
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Replace: &replace, Round: &round}})
		require.NoError(t, err)
		var gamesResp PostEventsV1EventIdGamesGenerate200JSONResponse
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate200JSONResponse:
			gamesResp = r
		default:
			t.Fatalf("preserving a matching result must succeed: %T", resp)
		}
		assert.NotContains(t, deleted, 4, "the result row is never in the delete set")
		assert.Len(t, created, 2, "the result's slot is not regenerated")

		// The response is the round's whole schedule, not only the rows this
		// request wrote: the preserved result is part of it, in schedule order.
		require.Len(t, gamesResp.Games, 3, "the preserved result row is returned too")
		for i, g := range gamesResp.Games {
			assert.Equal(t, i+1, g.Seq, "games come back in round/seq order")
		}
		require.NotNil(t, gamesResp.Games[0].GameNo)
		assert.Equal(t, 4, *gamesResp.Games[0].GameNo, "the preserved result keeps its identity")
		assert.Equal(t, COMPLETED, gamesResp.Games[0].Status)
		require.NotNil(t, gamesResp.Games[0].ScoreA)
		assert.Equal(t, 10, *gamesResp.Games[0].ScoreA)
		require.NotNil(t, gamesResp.Games[1].GameNo)
		assert.Equal(t, 7, *gamesResp.Games[1].GameNo, "the minted rows follow the result")
	})

	t.Run("replace refuses a result whose slot holds another pairing", func(t *testing.T) {
		eventID := uuid.New()
		ids := sixTeams()
		roundTwo := scheduledRound(eventID, 2, 4, ids)
		unknownA, unknownB := uuid.New(), uuid.New()
		roundTwo[0].Status = games.GameStatusCompleted
		roundTwo[0].SideA = games.Side{TeamID: &unknownA}
		roundTwo[0].SideB = games.Side{TeamID: &unknownB}
		roundTwo[0].ScoreA, roundTwo[0].ScoreB = ptrInt(10), ptrInt(5)

		var deleted bool
		format, replace, round := SWISS, true, 2
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return confirmedParticipations(eventID, ids), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return append(scheduledRound(eventID, 1, 1, ids), roundTwo...), nil
			},
			DeleteScheduledGameFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int, game games.Game) error {
				deleted = true
				return nil
			},
			GetTeamFunc: testTeamNames(),
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Replace: &replace, Round: &round}})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
			assert.Contains(t, r.Message, "recorded result occupies")
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.False(t, deleted, "the conflict check must run before any delete")
	})

	t.Run("round gaps are rejected", func(t *testing.T) {
		eventID := uuid.New()
		ids := sixTeams()
		format, round := SWISS, 3
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return confirmedParticipations(eventID, ids), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return scheduledRound(eventID, 1, 1, ids), nil
			},
			GetTeamFunc: testTeamNames(),
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Round: &round}})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("next round appends", func(t *testing.T) {
		eventID := uuid.New()
		ids := sixTeams()
		format, round := SWISS, 2
		var created []games.Game
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return confirmedParticipations(eventID, ids), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return scheduledRound(eventID, 1, 1, ids), nil
			},
			CreateGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameList []games.Game) error {
				created = gameList
				return nil
			},
			GetTeamFunc: testTeamNames(),
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Round: &round}})
		require.NoError(t, err)
		switch resp.(type) {
		case PostEventsV1EventIdGamesGenerate200JSONResponse:
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		require.Len(t, created, 3)
		firstNo := 4
		for i, g := range created {
			assert.Equal(t, firstNo+i, g.GameNo, "numbers continue after the existing max")
		}
	})
}

func TestGenerateCrossFormatRejected(t *testing.T) {
	eventID := uuid.New()
	newMock := func() *mockDB {
		return &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return confirmedParticipations(eventID, []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return nil, nil
			},
		}
	}

	t.Run("round robin rejects explicit round", func(t *testing.T) {
		format, round := ROUNDROBIN, 2
		resp, err := gamesTestAPI(newMock()).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Round: &round}})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate400JSONResponse:
			assert.Equal(t, InvalidBody, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("swiss rejects mirror", func(t *testing.T) {
		format, mirror := SWISS, true
		resp, err := gamesTestAPI(newMock()).PostEventsV1EventIdGamesGenerate(gamesCtx(),
			PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Mirror: &mirror}})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGamesGenerate400JSONResponse:
			assert.Equal(t, InvalidBody, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestGenerateInfraFailureIs500(t *testing.T) {
	eventID := uuid.New()
	mock := &mockDB{
		GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
			return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
		},
		ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
			return confirmedParticipations(eventID, []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}), nil
		},
		ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
			return nil, nil
		},
		CreateGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameList []games.Game) error {
			return games.NewTimeoutError("write timed out")
		},
	}
	format := ROUNDROBIN
	resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
		PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format}})
	require.NoError(t, err)
	switch r := resp.(type) {
	case PostEventsV1EventIdGamesGenerate500JSONResponse:
		assert.Equal(t, InternalError, r.Code)
	default:
		t.Fatalf("unexpected response type: %T", resp)
	}
}

func TestManualCreateGame(t *testing.T) {
	newMock := func(eventID uuid.UUID, existing []games.Game) (*mockDB, *[]games.Game, *[]string) {
		created := &[]games.Game{}
		calls := &[]string{}
		return &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return existing, nil
			},
			BeginScheduleWriteFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int) (int, error) {
				*calls = append(*calls, "separate-guard")
				return expectedVersion + 1, nil
			},
			CreateScheduledGameFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int, game games.Game) error {
				*calls = append(*calls, "create")
				assert.Equal(t, 1, expectedVersion, "the handler passes the event version it read")
				*created = append(*created, game)
				return nil
			},
			GetTeamFunc: testTeamNames(),
		}, created, calls
	}

	t.Run("mints the next game number and folds the guard into the write", func(t *testing.T) {
		eventID := uuid.New()
		a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		existing := []games.Game{scheduledGame(eventID, 4, 1, 1, a, b)}
		mock, created, calls := newMock(eventID, existing)

		notes := "court 2"
		body := &CreateGameRequest{
			Round: 2,
			Seq:   1,
			SideA: GameSideInput{TeamId: &c},
			SideB: GameSideInput{TeamId: &d},
			Notes: &notes,
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGames(gamesCtx(),
			PostEventsV1EventIdGamesRequestObject{EventId: eventID, Body: body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGames200JSONResponse:
			require.NotNil(t, r.Game.GameNo)
			assert.Equal(t, 5, *r.Game.GameNo)
			assert.Equal(t, SCHEDULED, r.Game.Status)
			assert.Equal(t, QUALIFYING, r.Game.Phase)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		require.Len(t, *created, 1)
		game := (*created)[0]
		assert.Equal(t, 5, game.GameNo)
		assert.Equal(t, games.GameStatusScheduled, game.Status)
		assert.Equal(t, games.GamePhaseQualifying, game.Phase)
		assert.Equal(t, notes, *game.Notes)
		assert.Equal(t, []string{"create"}, *calls, "guard and row write are one transaction")
	})

	t.Run("mints above the LastGameNo high-water mark", func(t *testing.T) {
		eventID := uuid.New()
		a, b := uuid.New(), uuid.New()
		created := &[]games.Game{}
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				event := gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour)
				event.LastGameNo = 9
				return event, nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return nil, nil
			},
			CreateScheduledGameFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int, game games.Game) error {
				*created = append(*created, game)
				return nil
			},
			GetTeamFunc: testTeamNames(),
		}
		body := &CreateGameRequest{
			Round: 1, Seq: 1,
			SideA: GameSideInput{TeamId: &a},
			SideB: GameSideInput{TeamId: &b},
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGames(gamesCtx(),
			PostEventsV1EventIdGamesRequestObject{EventId: eventID, Body: body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGames200JSONResponse:
			assert.Equal(t, 10, *r.Game.GameNo)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		require.Len(t, *created, 1)
		assert.Equal(t, 10, (*created)[0].GameNo, "a deleted number is never recycled")
	})

	t.Run("refuses an occupied round and position", func(t *testing.T) {
		eventID := uuid.New()
		a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		existing := []games.Game{scheduledGame(eventID, 1, 1, 1, a, b)}
		mock, created, _ := newMock(eventID, existing)

		body := &CreateGameRequest{
			Round: 1, Seq: 1,
			SideA: GameSideInput{TeamId: &c},
			SideB: GameSideInput{TeamId: &d},
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGames(gamesCtx(),
			PostEventsV1EventIdGamesRequestObject{EventId: eventID, Body: body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGames409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Empty(t, *created)
	})

	t.Run("refuses a qualifying game with a null side", func(t *testing.T) {
		eventID := uuid.New()
		b := uuid.New()
		mock, created, _ := newMock(eventID, nil)
		body := &CreateGameRequest{
			Round: 1, Seq: 1,
			SideA: GameSideInput{},
			SideB: GameSideInput{TeamId: &b},
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGames(gamesCtx(),
			PostEventsV1EventIdGamesRequestObject{EventId: eventID, Body: body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGames400JSONResponse:
			assert.Equal(t, InvalidBody, r.Code)
			assert.Contains(t, r.Message, "qualifying game requires two real sides")
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Empty(t, *created)
	})

	t.Run("refuses a FINALIZED event", func(t *testing.T) {
		eventID := uuid.New()
		a, b := uuid.New(), uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusFinalized, -time.Hour), nil
			},
		}
		body := &CreateGameRequest{
			Round: 1, Seq: 1,
			SideA: GameSideInput{TeamId: &a},
			SideB: GameSideInput{TeamId: &b},
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGames(gamesCtx(),
			PostEventsV1EventIdGamesRequestObject{EventId: eventID, Body: body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGames409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("invalid game is rejected", func(t *testing.T) {
		eventID := uuid.New()
		a, b := uuid.New(), uuid.New()
		mock, created, _ := newMock(eventID, nil)
		body := &CreateGameRequest{
			Round: 0, Seq: 1,
			SideA: GameSideInput{TeamId: &a},
			SideB: GameSideInput{TeamId: &b},
		}
		resp, err := gamesTestAPI(mock).PostEventsV1EventIdGames(gamesCtx(),
			PostEventsV1EventIdGamesRequestObject{EventId: eventID, Body: body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PostEventsV1EventIdGames400JSONResponse:
			assert.Equal(t, InvalidBody, r.Code)
			assert.Contains(t, r.Message, "round and seq")
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Empty(t, *created)
	})
}

func TestScheduleEdit(t *testing.T) {
	eventID := uuid.New()
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	newMock := func(stored games.Game) (*mockDB, *games.Game) {
		written := &games.Game{}
		return &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			GetGameByNumberFunc: func(ctx context.Context, eventID uuid.UUID, gameNo int) (games.Game, error) {
				return stored, nil
			},
			UpdateGameScheduleCheckedFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int, storedGame, game games.Game) error {
				*written = game
				return nil
			},
			GetTeamFunc: testTeamNames(),
		}, written
	}

	t.Run("rebuilds schedule fields and preserves identity", func(t *testing.T) {
		stored := scheduledGame(eventID, 3, 1, 1, a, b)
		mock, written := newMock(stored)

		start := time.Now().UTC().Truncate(time.Second)
		notes := "moved"
		body := &GameScheduleRequest{
			Round: 2, Seq: 4,
			SideA:     GameSideInput{TeamId: &c},
			SideB:     GameSideInput{TeamId: &d},
			StartTime: &start,
			Notes:     &notes,
			Version:   1,
		}
		resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameNo(gamesCtx(),
			PutEventsV1EventIdGamesGameNoRequestObject{EventId: eventID, GameNo: 3, Body: body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PutEventsV1EventIdGamesGameNo200JSONResponse:
			assert.Equal(t, 3, *r.Game.GameNo)
			assert.Equal(t, 2, r.Game.Round)
			assert.Equal(t, 4, r.Game.Seq)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Equal(t, stored.ID, written.ID)
		assert.Equal(t, stored.GameNo, written.GameNo)
		assert.Equal(t, stored.Phase, written.Phase)
		assert.Equal(t, games.GameStatusScheduled, written.Status)
		assert.Equal(t, c, *written.SideA.TeamID)
		assert.Equal(t, d, *written.SideB.TeamID)
		assert.True(t, start.Equal(*written.StartTime))
		assert.Equal(t, notes, *written.Notes)
		assert.Equal(t, 2, written.Version)
		require.NoError(t, written.Validate())
	})

	t.Run("only SCHEDULED games can be edited", func(t *testing.T) {
		stored := scheduledGame(eventID, 3, 1, 1, a, b)
		stored.Status = games.GameStatusCompleted
		stored.ScoreA, stored.ScoreB = ptrInt(10), ptrInt(5)
		mock, written := newMock(stored)

		body := &GameScheduleRequest{
			Round: 2, Seq: 4,
			SideA:   GameSideInput{TeamId: &c},
			SideB:   GameSideInput{TeamId: &d},
			Version: 1,
		}
		resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameNo(gamesCtx(),
			PutEventsV1EventIdGamesGameNoRequestObject{EventId: eventID, GameNo: 3, Body: body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PutEventsV1EventIdGamesGameNo409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Empty(t, *written, "a result row must never be edited by the schedule path")
	})

	t.Run("stale version is a conflict", func(t *testing.T) {
		stored := scheduledGame(eventID, 3, 1, 1, a, b)
		mock, written := newMock(stored)
		body := &GameScheduleRequest{
			Round: 2, Seq: 4,
			SideA:   GameSideInput{TeamId: &c},
			SideB:   GameSideInput{TeamId: &d},
			Version: 0,
		}
		resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameNo(gamesCtx(),
			PutEventsV1EventIdGamesGameNoRequestObject{EventId: eventID, GameNo: 3, Body: body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PutEventsV1EventIdGamesGameNo409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Empty(t, *written)
	})

	t.Run("finalize and version conflicts are distinguished", func(t *testing.T) {
		for _, tc := range []struct {
			name    string
			err     error
			message string
		}{
			{"finalized", games.NewEventFinalizedError("finalized", nil), "FINALIZED"},
			{"version", games.NewVersionConflictError("conflict", nil), "Concurrent schedule change"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				stored := scheduledGame(eventID, 3, 1, 1, a, b)
				mock, _ := newMock(stored)
				mock.UpdateGameScheduleCheckedFunc = func(ctx context.Context, eventID uuid.UUID, expectedVersion int, storedGame, game games.Game) error {
					return tc.err
				}
				body := &GameScheduleRequest{
					Round: 2, Seq: 4,
					SideA:   GameSideInput{TeamId: &c},
					SideB:   GameSideInput{TeamId: &d},
					Version: 1,
				}
				resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameNo(gamesCtx(),
					PutEventsV1EventIdGamesGameNoRequestObject{EventId: eventID, GameNo: 3, Body: body})
				require.NoError(t, err)
				switch r := resp.(type) {
				case PutEventsV1EventIdGamesGameNo409JSONResponse:
					assert.Equal(t, Conflict, r.Code)
					assert.Contains(t, r.Message, tc.message)
				default:
					t.Fatalf("unexpected response type: %T", resp)
				}
			})
		}
	})
}

func TestDeleteGame(t *testing.T) {
	eventID := uuid.New()
	a, b := uuid.New(), uuid.New()

	t.Run("deletes a SCHEDULED game", func(t *testing.T) {
		stored := scheduledGame(eventID, 9, 1, 1, a, b)
		var deleted []int
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			GetGameByNumberFunc: func(ctx context.Context, eventID uuid.UUID, gameNo int) (games.Game, error) {
				return stored, nil
			},
			DeleteScheduledGameFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int, game games.Game) error {
				deleted = []int{game.GameNo}
				return nil
			},
		}
		resp, err := gamesTestAPI(mock).DeleteEventsV1EventIdGamesGameNo(gamesCtx(),
			DeleteEventsV1EventIdGamesGameNoRequestObject{EventId: eventID, GameNo: 9})
		require.NoError(t, err)
		switch resp.(type) {
		case DeleteEventsV1EventIdGamesGameNo204Response:
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Equal(t, []int{9}, deleted)
	})

	t.Run("refuses a game holding a result", func(t *testing.T) {
		stored := scheduledGame(eventID, 9, 1, 1, a, b)
		stored.Status = games.GameStatusCompleted
		stored.ScoreA, stored.ScoreB = ptrInt(10), ptrInt(5)
		var deleted bool
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			GetGameByNumberFunc: func(ctx context.Context, eventID uuid.UUID, gameNo int) (games.Game, error) {
				return stored, nil
			},
			DeleteScheduledGameFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int, game games.Game) error {
				deleted = true
				return nil
			},
			GetTeamFunc: testTeamNames(),
		}
		resp, err := gamesTestAPI(mock).DeleteEventsV1EventIdGamesGameNo(gamesCtx(),
			DeleteEventsV1EventIdGamesGameNoRequestObject{EventId: eventID, GameNo: 9})
		require.NoError(t, err)
		switch r := resp.(type) {
		case DeleteEventsV1EventIdGamesGameNo409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.False(t, deleted)
	})

	t.Run("refuses a FINALIZED event", func(t *testing.T) {
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusFinalized, -time.Hour), nil
			},
		}
		resp, err := gamesTestAPI(mock).DeleteEventsV1EventIdGamesGameNo(gamesCtx(),
			DeleteEventsV1EventIdGamesGameNoRequestObject{EventId: eventID, GameNo: 9})
		require.NoError(t, err)
		switch r := resp.(type) {
		case DeleteEventsV1EventIdGamesGameNo409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("missing game is 404", func(t *testing.T) {
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			GetGameByNumberFunc: func(ctx context.Context, eventID uuid.UUID, gameNo int) (games.Game, error) {
				return games.Game{}, games.NewGameDoesNotExistError("missing", nil)
			},
		}
		resp, err := gamesTestAPI(mock).DeleteEventsV1EventIdGamesGameNo(gamesCtx(),
			DeleteEventsV1EventIdGamesGameNoRequestObject{EventId: eventID, GameNo: 9})
		require.NoError(t, err)
		switch r := resp.(type) {
		case DeleteEventsV1EventIdGamesGameNo404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestGameResultEndpoint(t *testing.T) {
	eventID := uuid.New()
	a, b := uuid.New(), uuid.New()

	newMock := func(stored games.Game) (*mockDB, *games.Game) {
		written := &games.Game{}
		return &mockDB{
			GetGameByNumberFunc: func(ctx context.Context, eventID uuid.UUID, gameNo int) (games.Game, error) {
				return stored, nil
			},
			UpdateGameCheckedFunc: func(ctx context.Context, eventID uuid.UUID, game games.Game) error {
				*written = game
				return nil
			},
			GetTeamFunc: testTeamNames(),
		}, written
	}

	t.Run("full shape replaces the whole result", func(t *testing.T) {
		stored := scheduledGame(eventID, 2, 1, 1, a, b)
		stored.Status = games.GameStatusCompleted
		stored.ScoreA, stored.ScoreB = ptrInt(10), ptrInt(5)
		mock, written := newMock(stored)

		// Sending SCHEDULED with no scores retracts the result: omitted
		// result fields clear rather than being preserved.
		body := &GameResultRequest{Status: SCHEDULED, Version: 1}
		resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameNoResult(gamesCtx(),
			PutEventsV1EventIdGamesGameNoResultRequestObject{EventId: eventID, GameNo: 2, Body: body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PutEventsV1EventIdGamesGameNoResult200JSONResponse:
			assert.Equal(t, SCHEDULED, r.Game.Status)
			assert.Nil(t, r.Game.ScoreA)
			assert.Nil(t, r.Game.ScoreB)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Equal(t, games.GameStatusScheduled, written.Status)
		assert.Nil(t, written.ScoreA)
		assert.Nil(t, written.ScoreB)
		assert.Nil(t, written.ForfeitSide)
		assert.Equal(t, 2, written.Version)
	})

	t.Run("forfeit side is cleared when omitted", func(t *testing.T) {
		stored := scheduledGame(eventID, 2, 1, 1, a, b)
		stored.Status = games.GameStatusForfeit
		slot := games.GameSlotA
		stored.ForfeitSide = &slot
		mock, written := newMock(stored)

		scoreA, scoreB := 4, 6
		body := &GameResultRequest{Status: COMPLETED, ScoreA: &scoreA, ScoreB: &scoreB, Version: 1}
		resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameNoResult(gamesCtx(),
			PutEventsV1EventIdGamesGameNoResultRequestObject{EventId: eventID, GameNo: 2, Body: body})
		require.NoError(t, err)
		switch resp.(type) {
		case PutEventsV1EventIdGamesGameNoResult200JSONResponse:
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Equal(t, games.GameStatusCompleted, written.Status)
		assert.Nil(t, written.ForfeitSide)
		assert.Equal(t, 4, *written.ScoreA)
		assert.Equal(t, 6, *written.ScoreB)
	})

	t.Run("version conflict does not write", func(t *testing.T) {
		stored := scheduledGame(eventID, 2, 1, 1, a, b)
		mock, written := newMock(stored)
		body := &GameResultRequest{Status: SCHEDULED, Version: 2}
		resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameNoResult(gamesCtx(),
			PutEventsV1EventIdGamesGameNoResultRequestObject{EventId: eventID, GameNo: 2, Body: body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PutEventsV1EventIdGamesGameNoResult409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Empty(t, *written)
	})

	t.Run("invalid result shape is rejected", func(t *testing.T) {
		stored := scheduledGame(eventID, 2, 1, 1, a, b)
		mock, written := newMock(stored)
		body := &GameResultRequest{Status: COMPLETED, Version: 1}
		resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameNoResult(gamesCtx(),
			PutEventsV1EventIdGamesGameNoResultRequestObject{EventId: eventID, GameNo: 2, Body: body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PutEventsV1EventIdGamesGameNoResult400JSONResponse:
			assert.Equal(t, InvalidBody, r.Code)
			assert.Contains(t, r.Message, "both scores")
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.Empty(t, *written)
	})

	t.Run("finalized event is a conflict", func(t *testing.T) {
		stored := scheduledGame(eventID, 2, 1, 1, a, b)
		mock, _ := newMock(stored)
		mock.UpdateGameCheckedFunc = func(ctx context.Context, eventID uuid.UUID, game games.Game) error {
			return games.NewEventFinalizedError("finalized", nil)
		}
		body := &GameResultRequest{Status: SCHEDULED, Version: 1}
		resp, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameNoResult(gamesCtx(),
			PutEventsV1EventIdGamesGameNoResultRequestObject{EventId: eventID, GameNo: 2, Body: body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PutEventsV1EventIdGamesGameNoResult409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
			assert.Contains(t, r.Message, "FINALIZED")
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestGetStandingsHandler(t *testing.T) {
	t.Run("derived with names, draws, and DNS", func(t *testing.T) {
		eventID := uuid.New()
		a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		scoreW, scoreL, scoreDraw := 10, 5, 7
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return []teams.Participation{
					{EventID: eventID, TeamID: a, Status: teams.ParticipationStatusConfirmed, Version: 1},
					{EventID: eventID, TeamID: b, Status: teams.ParticipationStatusConfirmed, Version: 1},
					{EventID: eventID, TeamID: c, Status: teams.ParticipationStatusRegistered, Version: 1},
					{EventID: eventID, TeamID: d, Status: teams.ParticipationStatusConfirmed, Version: 1},
				}, nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return []games.Game{
					{
						ID: uuid.New(), EventID: eventID, GameNo: 1, Phase: games.GamePhaseQualifying,
						Round: 1, Seq: 1, Status: games.GameStatusCompleted,
						SideA: games.Side{TeamID: &a}, SideB: games.Side{TeamID: &b},
						ScoreA: &scoreW, ScoreB: &scoreL, Version: 1,
					},
					{
						ID: uuid.New(), EventID: eventID, GameNo: 2, Phase: games.GamePhaseQualifying,
						Round: 1, Seq: 2, Status: games.GameStatusCompleted,
						SideA: games.Side{TeamID: &a}, SideB: games.Side{TeamID: &d},
						ScoreA: &scoreDraw, ScoreB: &scoreDraw, Version: 1,
					},
				}, nil
			},
			GetTeamFunc: testTeamNames(),
		}

		resp, err := gamesTestAPI(mock).GetEventsV1EventIdStandings(gamesCtx(), GetEventsV1EventIdStandingsRequestObject{EventId: eventID})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1EventIdStandings200JSONResponse:
			require.Len(t, r.Standings, 4)
			assert.Equal(t, a, r.Standings[0].TeamId)
			assert.Equal(t, 1, r.Standings[0].QualifyingRank)
			assert.Equal(t, 1, r.Standings[0].Draws)
			assert.Equal(t, 1, r.Standings[0].Wins)

			// The REGISTERED team never played a counted game: DNS/unranked.
			var registered Standing
			for _, s := range r.Standings {
				if s.TeamId == c {
					registered = s
				}
			}
			assert.True(t, registered.Dns)
			assert.Equal(t, 0, registered.QualifyingRank)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}
