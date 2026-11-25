package api

import (
	"context"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/games"
	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateConcurrentScheduleChange(t *testing.T) {
	eventID := uuid.New()
	teamIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	a, b := uuid.New(), uuid.New()

	var deleted, created bool
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
		BeginScheduleWriteFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int) (int, error) {
			return 0, games.NewVersionConflictError("another writer got there first", nil)
		},
		DeleteGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameNos []int) error {
			deleted = true
			return nil
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
		assert.Contains(t, r.Message, "Concurrent schedule change")
	default:
		t.Fatalf("unexpected response type: %T", resp)
	}
	assert.False(t, deleted, "a failed epoch grant must run before any delete")
	assert.False(t, created)
}

// TestGenerateFailureDoesNotTakeTheEpochGrant pins the ordering: validation
// (cheap guards and buildSchedule) runs before BeginScheduleWrite, so a
// refused generate does not bump the event Version or ScheduleGeneration.
// Only the writes need exclusivity.
func TestGenerateFailureDoesNotTakeTheEpochGrant(t *testing.T) {
	eventID := uuid.New()
	sixTeams := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	a, b := uuid.New(), uuid.New()

	cases := []struct {
		name           string
		event          events.Event
		participations []teams.Participation
		existing       []games.Game
		body           GenerateRequest
	}{
		{
			name:           "registration still open",
			event:          gamesTestEvent(eventID, events.EventStatusInProgress, time.Hour),
			participations: confirmedParticipations(eventID, sixTeams),
			body:           GenerateRequest{Format: ROUNDROBIN},
		},
		{
			name:           "event not IN_PROGRESS",
			event:          gamesTestEvent(eventID, events.EventStatusOpened, -time.Hour),
			participations: confirmedParticipations(eventID, sixTeams),
			body:           GenerateRequest{Format: ROUNDROBIN},
		},
		{
			name:  "no CONFIRMED teams",
			event: gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour),
			participations: []teams.Participation{{
				EventID: eventID, TeamID: uuid.New(),
				Status: teams.ParticipationStatusRegistered, Version: 1,
			}},
			body: GenerateRequest{Format: ROUNDROBIN},
		},
		{
			name:           "existing games without replace",
			event:          gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour),
			participations: confirmedParticipations(eventID, sixTeams),
			existing:       []games.Game{scheduledGame(eventID, 1, 1, 1, a, b)},
			body:           GenerateRequest{Format: ROUNDROBIN},
		},
		{
			name:           "unknown format",
			event:          gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour),
			participations: confirmedParticipations(eventID, sixTeams),
			body:           GenerateRequest{Format: "NOPE"},
		},
		{
			name:           "field too small for the format",
			event:          gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour),
			participations: confirmedParticipations(eventID, []uuid.UUID{uuid.New()}),
			body:           GenerateRequest{Format: ROUNDROBIN},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var granted, deleted, created bool
			mock := &mockDB{
				GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
					return tc.event, nil
				},
				ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
					return tc.participations, nil
				},
				ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
					return tc.existing, nil
				},
				BeginScheduleWriteFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int) (int, error) {
					granted = true
					return expectedVersion + 1, nil
				},
				DeleteGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameNos []int) error {
					deleted = true
					return nil
				},
				CreateGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameList []games.Game) error {
					created = true
					return nil
				},
				GetTeamFunc: testTeamNames(),
			}
			resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
				PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &tc.body})
			require.NoError(t, err)
			switch resp.(type) {
			case PostEventsV1EventIdGamesGenerate400JSONResponse, PostEventsV1EventIdGamesGenerate409JSONResponse:
			default:
				t.Fatalf("a refused generate must not succeed: %T", resp)
			}
			assert.False(t, granted, "a refused generate must not take the epoch grant")
			assert.False(t, deleted, "a refused generate must not delete")
			assert.False(t, created, "a refused generate must not write")
		})
	}
}

// TestSingleRowScheduleWritesFoldTheGuard pins the guard protocol for
// single-row writes: there is no separate bump call that could be followed by
// a stale read; the transaction that carries the row write carries the event
// Version condition too.
func TestSingleRowScheduleWritesFoldTheGuard(t *testing.T) {
	eventID := uuid.New()
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	stored := scheduledGame(eventID, 5, 1, 1, a, b)

	newMock := func() (*mockDB, *bool) {
		separateGuardCalled := new(bool)
		return &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
			},
			GetGameByNumberFunc: func(ctx context.Context, eventID uuid.UUID, gameNo int) (games.Game, error) {
				return stored, nil
			},
			BeginScheduleWriteFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int) (int, error) {
				*separateGuardCalled = true
				return expectedVersion + 1, nil
			},
			GetTeamFunc: testTeamNames(),
		}, separateGuardCalled
	}

	t.Run("delete passes the read version into the folded delete", func(t *testing.T) {
		mock, separateGuardCalled := newMock()
		var gotVersion int
		mock.DeleteScheduledGameFunc = func(ctx context.Context, eventID uuid.UUID, expectedVersion int, game games.Game) error {
			gotVersion = expectedVersion
			assert.Equal(t, stored.Version, game.Version)
			return nil
		}
		_, err := gamesTestAPI(mock).DeleteEventsV1EventIdGamesGameNo(gamesCtx(),
			DeleteEventsV1EventIdGamesGameNoRequestObject{EventId: eventID, GameNo: 5})
		require.NoError(t, err)
		assert.Equal(t, 1, gotVersion)
		assert.False(t, *separateGuardCalled, "delete must not run a separate guard transaction")
	})

	t.Run("edit passes the read version into the folded write", func(t *testing.T) {
		mock, separateGuardCalled := newMock()
		var gotVersion int
		mock.UpdateGameScheduleCheckedFunc = func(ctx context.Context, eventID uuid.UUID, expectedVersion int, storedGame, updated games.Game) error {
			gotVersion = expectedVersion
			assert.Equal(t, stored.ID, storedGame.ID)
			return nil
		}
		body := &GameScheduleRequest{
			Round: 1, Seq: 2,
			SideA:   GameSideInput{TeamId: &c},
			SideB:   GameSideInput{TeamId: &d},
			Version: 1,
		}
		_, err := gamesTestAPI(mock).PutEventsV1EventIdGamesGameNo(gamesCtx(),
			PutEventsV1EventIdGamesGameNoRequestObject{EventId: eventID, GameNo: 5, Body: body})
		require.NoError(t, err)
		assert.Equal(t, 1, gotVersion)
		assert.False(t, *separateGuardCalled, "edit must not run a separate guard transaction")
	})

	t.Run("create passes the read version into the folded create", func(t *testing.T) {
		mock, separateGuardCalled := newMock()
		var gotVersion int
		mock.ListGamesForEventFunc = func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
			return nil, nil
		}
		mock.CreateScheduledGameFunc = func(ctx context.Context, eventID uuid.UUID, expectedVersion int, game games.Game) error {
			gotVersion = expectedVersion
			return nil
		}
		body := &CreateGameRequest{
			Round: 1, Seq: 1,
			SideA: GameSideInput{TeamId: &c},
			SideB: GameSideInput{TeamId: &d},
		}
		_, err := gamesTestAPI(mock).PostEventsV1EventIdGames(gamesCtx(),
			PostEventsV1EventIdGamesRequestObject{EventId: eventID, Body: body})
		require.NoError(t, err)
		assert.Equal(t, 1, gotVersion)
		assert.False(t, *separateGuardCalled, "create must not run a separate guard transaction")
	})
}

// TestDuplicateMintedGameNoIsRejected simulates the review's interleaving:
// two manual creates both read event version 1 (and therefore mint gameNo 1).
// The first folded transaction wins; the second fails its Version condition,
// so exactly one game with that number exists.
func TestDuplicateMintedGameNoIsRejected(t *testing.T) {
	eventID := uuid.New()
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	eventVersion := 1
	lastGameNo := 0
	stored := map[int]games.Game{}
	mock := &mockDB{
		GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
			event := gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour)
			event.Version = eventVersion
			event.LastGameNo = lastGameNo
			return event, nil
		},
		ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
			out := make([]games.Game, 0, len(stored))
			for _, g := range stored {
				out = append(out, g)
			}
			return out, nil
		},
		CreateScheduledGameFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int, game games.Game) error {
			// The transaction's event Version CAS plus the last-writer check.
			if expectedVersion != eventVersion {
				return games.NewVersionConflictError("stale event version", nil)
			}
			if _, exists := stored[game.GameNo]; exists {
				return games.NewSlotOccupiedError("slot occupied", nil)
			}
			stored[game.GameNo] = game
			eventVersion++
			lastGameNo = game.GameNo
			return nil
		},
		GetTeamFunc: testTeamNames(),
	}

	// Both requests read the same event snapshot.
	newBody := func(x, y uuid.UUID) *CreateGameRequest {
		return &CreateGameRequest{
			Round: 1, Seq: 1,
			SideA: GameSideInput{TeamId: &x},
			SideB: GameSideInput{TeamId: &y},
		}
	}

	firstResp, err := gamesTestAPI(mock).PostEventsV1EventIdGames(gamesCtx(),
		PostEventsV1EventIdGamesRequestObject{EventId: eventID, Body: newBody(a, b)})
	require.NoError(t, err)
	switch r := firstResp.(type) {
	case PostEventsV1EventIdGames200JSONResponse:
		assert.Equal(t, 1, *r.Game.GameNo)
	default:
		t.Fatalf("unexpected response type: %T", firstResp)
	}

	secondResp, err := gamesTestAPI(mock).PostEventsV1EventIdGames(gamesCtx(),
		PostEventsV1EventIdGamesRequestObject{EventId: eventID, Body: newBody(c, d)})
	require.NoError(t, err)
	switch r := secondResp.(type) {
	case PostEventsV1EventIdGames409JSONResponse:
		assert.Equal(t, Conflict, r.Code)
	default:
		t.Fatalf("the second writer must not mint the same gameNo: %T", secondResp)
	}
	assert.Len(t, stored, 1)
	assert.Equal(t, 1, lastGameNo)
}
