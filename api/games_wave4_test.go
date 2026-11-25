package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/games"
	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
)

func byeGame(eventID uuid.UUID, gameNo, round, seq int, team uuid.UUID) games.Game {
	return games.Game{
		ID:      uuid.New(),
		EventID: eventID,
		GameNo:  gameNo,
		Phase:   games.GamePhaseQualifying,
		Round:   round,
		Seq:     seq,
		Status:  games.GameStatusBye,
		SideA:   games.Side{TeamID: &team},
		SideB:   games.Side{IsBye: true},
		Version: 1,
	}
}

// TestDeleteAllowsUnplayedRowsAndRefusesResults pins fix 4: BYE and CANCELLED
// are not results, so the delete endpoint removes them; recorded results stay
// undeletable everywhere.
func TestDeleteAllowsUnplayedRowsAndRefusesResults(t *testing.T) {
	eventID := uuid.New()
	a, b := uuid.New(), uuid.New()

	unplayed := []struct {
		name  string
		game  games.Game
		setup func(*games.Game)
	}{
		{"scheduled", scheduledGame(eventID, 9, 1, 1, a, b), nil},
		{"bye", byeGame(eventID, 9, 1, 1, a), nil},
		{"cancelled", scheduledGame(eventID, 9, 1, 1, a, b), func(g *games.Game) {
			g.Status = games.GameStatusCancelled
		}},
	}
	for _, tc := range unplayed {
		t.Run("deletes "+tc.name, func(t *testing.T) {
			stored := tc.game
			if tc.setup != nil {
				tc.setup(&stored)
			}
			deleted := false
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
			}
			resp, err := gamesTestAPI(mock).DeleteEventsV1EventIdGamesGameNo(gamesCtx(),
				DeleteEventsV1EventIdGamesGameNoRequestObject{EventId: eventID, GameNo: 9})
			require.NoError(t, err)
			switch resp.(type) {
			case DeleteEventsV1EventIdGamesGameNo204Response:
			default:
				t.Fatalf("unexpected response type: %T", resp)
			}
			assert.True(t, deleted)
		})
	}

	results := []struct {
		name  string
		game  games.Game
		setup func(*games.Game)
	}{
		{"completed", scheduledGame(eventID, 9, 1, 1, a, b), func(g *games.Game) {
			g.Status = games.GameStatusCompleted
			g.ScoreA, g.ScoreB = ptrInt(10), ptrInt(5)
		}},
		{"forfeit", scheduledGame(eventID, 9, 1, 1, a, b), func(g *games.Game) {
			g.Status = games.GameStatusForfeit
			slot := games.GameSlotA
			g.ForfeitSide = &slot
		}},
		{"double forfeit", scheduledGame(eventID, 9, 1, 1, a, b), func(g *games.Game) {
			g.Status = games.GameStatusDoubleForfeit
		}},
	}
	for _, tc := range results {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			stored := tc.game
			tc.setup(&stored)
			deleted := false
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
			assert.False(t, deleted, "a result row is never deleted")
		})
	}
}

func TestRoundRobinReplaceSweepsByeAndCancelledRows(t *testing.T) {
	eventID := uuid.New()
	teamIDs := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}

	existing := []games.Game{
		byeGame(eventID, 2, 1, 1, teamIDs[2]),
		scheduledGame(eventID, 3, 1, 2, teamIDs[3], teamIDs[4]),
		scheduledGame(eventID, 4, 1, 3, teamIDs[5], teamIDs[0]),
	}
	existing[2].Status = games.GameStatusCancelled

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
		PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Replace: &replace}})
	require.NoError(t, err)
	switch resp.(type) {
	case PostEventsV1EventIdGamesGenerate200JSONResponse:
	default:
		t.Fatalf("unexpected response type: %T", resp)
	}
	assert.ElementsMatch(t, []int{2, 3, 4}, deleted, "BYE and CANCELLED rows are swept with SCHEDULED rows")
}

// TestGenerateStrandedFieldIsHonest409 pins the handler mapping for the
// provable dead end: a team that has played everyone left cannot be paired, so
// the 409 says no valid pairing exists rather than overstating it as
// "too complex".
func TestGenerateStrandedFieldIsHonest409(t *testing.T) {
	eventID := uuid.New()
	teamIDs := make([]uuid.UUID, 16)
	teamNamesByID := map[uuid.UUID]string{}
	for i := range teamIDs {
		teamIDs[i] = uuid.New()
		teamNamesByID[teamIDs[i]] = fmt.Sprintf("T%02d", i)
	}
	stranded := teamIDs[len(teamIDs)-1]
	var priors []games.Game
	for i, opponent := range teamIDs[:len(teamIDs)-1] {
		g := scheduledGame(eventID, i+1, 1, i+1, stranded, opponent)
		priors = append(priors, g)
	}

	var created, deleted bool
	format, round := SWISS, 2
	mock := &mockDB{
		GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
			return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
		},
		ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
			return confirmedParticipations(eventID, teamIDs), nil
		},
		ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
			return priors, nil
		},
		DeleteGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameNos []int) error {
			deleted = true
			return nil
		},
		CreateGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameList []games.Game) error {
			created = true
			return nil
		},
		// Standings order does not matter to the dead end: the precheck scans
		// the whole remaining pool.
		GetTeamFunc: func(ctx context.Context, id uuid.UUID) (teams.Team, error) {
			return teams.Team{ID: id, Name: teamNamesByID[id], Status: teams.TeamStatusActive}, nil
		},
	}
	resp, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
		PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Round: &round}})
	require.NoError(t, err)
	switch r := resp.(type) {
	case PostEventsV1EventIdGamesGenerate409JSONResponse:
		assert.Equal(t, Conflict, r.Code)
		assert.Contains(t, r.Message, "No valid pairing")
	default:
		t.Fatalf("unexpected response type: %T", resp)
	}
	assert.False(t, deleted)
	assert.False(t, created)
}

// TestGenerateBudgetExceededMapsToActionable409 keeps the budget message
// covered even though no realistic fixture reaches it: exhaustion means
// "feasible but too complex at this round count", and the 409 points the
// admin at the inputs rather than claiming the field is impossible.
func TestGenerateBudgetExceededMapsToActionable409(t *testing.T) {
	span := trace.SpanFromContext(gamesCtx())
	resp := gamesTestAPI(&mockDB{}).generateErrorResponse(span, noopLogger,
		games.NewPairingBudgetExceededError("budget exhausted", nil))
	switch r := resp.(type) {
	case PostEventsV1EventIdGamesGenerate409JSONResponse:
		assert.Equal(t, Conflict, r.Code)
		assert.Contains(t, r.Message, "too complex")
	default:
		t.Fatalf("unexpected response type: %T", resp)
	}
}

func TestScheduleEditSlotOccupiedMessage(t *testing.T) {
	eventID := uuid.New()
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	stored := scheduledGame(eventID, 3, 1, 1, a, b)
	mock := &mockDB{
		GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
			return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
		},
		GetGameByNumberFunc: func(ctx context.Context, eventID uuid.UUID, gameNo int) (games.Game, error) {
			return stored, nil
		},
		UpdateGameScheduleCheckedFunc: func(ctx context.Context, eventID uuid.UUID, expectedVersion int, storedGame, updated games.Game) error {
			return games.NewSlotOccupiedError("occupied", nil)
		},
		GetTeamFunc: testTeamNames(),
	}
	body := &GameScheduleRequest{
		Round: 1, Seq: 2,
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
		assert.Contains(t, r.Message, "slot occupied; pick another round/seq")
	default:
		t.Fatalf("unexpected response type: %T", resp)
	}
}

// TestMidFlightResultReplaceIsAbortedAndRecoverable is the fix-2 regression at
// two-chunk scale: 120 games means the delete and create phases each span two
// transactions. A result landing mid-flight aborts the replace; the recorded
// result is still there and a retry sweeps every unplayed leftover and keeps
// the result.
func TestMidFlightResultReplaceIsAbortedAndRecoverable(t *testing.T) {
	eventID := uuid.New()
	teamIDs := make([]uuid.UUID, 16)
	for i := range teamIDs {
		teamIDs[i] = uuid.New()
	}

	generated, err := games.GenerateRoundRobin(eventID, teamIDs, false)
	require.NoError(t, err)
	require.Len(t, generated, 120)

	for i := range generated {
		generated[i].GameNo = i + 1
	}
	existing := append([]games.Game(nil), generated...)

	var deleted []int
	var created []games.Game
	deleteCalls := 0
	mock := &mockDB{
		GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
			return gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour), nil
		},
		ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
			return confirmedParticipations(eventID, teamIDs), nil
		},
		ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
			return existing, nil
		},
		DeleteGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameNos []int) error {
			deleteCalls++
			if deleteCalls == 1 {
				// The second chunk hits a row whose Version moved.
				return games.NewVersionConflictError("a result landed mid-flight", nil)
			}
			deleted = gameNos
			return nil
		},
		CreateGamesFunc: func(ctx context.Context, eventID uuid.UUID, epochVersion int, gameList []games.Game) error {
			created = gameList
			return nil
		},
		GetTeamFunc: testTeamNames(),
	}

	format, replace := ROUNDROBIN, true
	first, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
		PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Replace: &replace}})
	require.NoError(t, err)
	switch r := first.(type) {
	case PostEventsV1EventIdGamesGenerate409JSONResponse:
		assert.Equal(t, Conflict, r.Code)
	default:
		t.Fatalf("unexpected response type: %T", first)
	}
	assert.Empty(t, created, "the new schedule is not written after a refused delete")

	// The mid-flight result survives; the retry sees it as a recorded result.
	result := existing[60]
	result.Status = games.GameStatusCompleted
	result.ScoreA, result.ScoreB = ptrInt(10), ptrInt(5)
	result.Version = 2
	existing = append(append([]games.Game{}, existing[:60]...), append([]games.Game{result}, existing[61:]...)...)

	second, err := gamesTestAPI(mock).PostEventsV1EventIdGamesGenerate(gamesCtx(),
		PostEventsV1EventIdGamesGenerateRequestObject{EventId: eventID, Body: &GenerateRequest{Format: format, Replace: &replace}})
	require.NoError(t, err)
	switch second.(type) {
	case PostEventsV1EventIdGamesGenerate200JSONResponse:
	default:
		t.Fatalf("the retry must recover the partial state: %T", second)
	}
	assert.NotContains(t, deleted, result.GameNo, "the recorded result is not in the delete set")
	assert.Len(t, deleted, 119, "every unplayed leftover is swept in two chunks")
	assert.Len(t, created, 119, "the result's slot is not regenerated")
}

func TestStandingsTiebreakersPerEvent(t *testing.T) {
	eventID := uuid.New()
	a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	results := []games.Game{
		{
			ID: uuid.New(), EventID: eventID, GameNo: 1, Phase: games.GamePhaseQualifying,
			Round: 1, Seq: 1, Status: games.GameStatusCompleted,
			SideA: games.Side{TeamID: &a}, SideB: games.Side{TeamID: &b},
			ScoreA: ptrInt(6), ScoreB: ptrInt(5), Version: 1,
		},
		{
			ID: uuid.New(), EventID: eventID, GameNo: 2, Phase: games.GamePhaseQualifying,
			Round: 1, Seq: 2, Status: games.GameStatusCompleted,
			SideA: games.Side{TeamID: &a}, SideB: games.Side{TeamID: &c},
			ScoreA: ptrInt(6), ScoreB: ptrInt(5), Version: 1,
		},
		{
			ID: uuid.New(), EventID: eventID, GameNo: 3, Phase: games.GamePhaseQualifying,
			Round: 1, Seq: 3, Status: games.GameStatusCompleted,
			SideA: games.Side{TeamID: &d}, SideB: games.Side{TeamID: &b},
			ScoreA: ptrInt(30), ScoreB: ptrInt(0), Version: 1,
		},
	}
	participants := confirmedParticipations(eventID, []uuid.UUID{a, b, c, d})

	newMock := func(order games.TiebreakOrder) *mockDB {
		return &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				event := gamesTestEvent(eventID, events.EventStatusInProgress, -time.Hour)
				event.Tiebreakers = order
				return event, nil
			},
			ListParticipationsForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]teams.Participation, error) {
				return participants, nil
			},
			ListGamesForEventFunc: func(ctx context.Context, eventID uuid.UUID) ([]games.Game, error) {
				return results, nil
			},
			GetTeamFunc: testTeamNames(),
		}
	}

	t.Run("absent order uses net-first", func(t *testing.T) {
		resp, err := gamesTestAPI(newMock(nil)).GetEventsV1EventIdStandings(gamesCtx(),
			GetEventsV1EventIdStandingsRequestObject{EventId: eventID})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1EventIdStandings200JSONResponse:
			require.NotEmpty(t, r.Standings)
			assert.Equal(t, d, r.Standings[0].TeamId, "net-first ranks the +30 team first")
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("wins-first event ranks the unbeaten team first", func(t *testing.T) {
		resp, err := gamesTestAPI(newMock(games.WinsFirstTiebreakOrder())).GetEventsV1EventIdStandings(gamesCtx(),
			GetEventsV1EventIdStandingsRequestObject{EventId: eventID})
		require.NoError(t, err)
		switch r := resp.(type) {
		case GetEventsV1EventIdStandings200JSONResponse:
			require.NotEmpty(t, r.Standings)
			assert.Equal(t, a, r.Standings[0].TeamId, "wins-first ranks the 2-0 team first")
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("invalid stored order is an internal error, not a silent default", func(t *testing.T) {
		resp, err := gamesTestAPI(newMock(games.TiebreakOrder{"BOGUS"})).GetEventsV1EventIdStandings(gamesCtx(),
			GetEventsV1EventIdStandingsRequestObject{EventId: eventID})
		require.NoError(t, err)
		switch resp.(type) {
		case GetEventsV1EventIdStandings500JSONResponse:
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestTiebreakerConversionRejectsUnknown(t *testing.T) {
	order, err := apiTiebreakersToDomain(nil)
	require.NoError(t, err)
	assert.Nil(t, order, "absent means the default at use time")

	valid := []Tiebreaker{WINS}
	order, err = apiTiebreakersToDomain(&valid)
	require.NoError(t, err)
	assert.Equal(t, games.TiebreakOrder{games.TiebreakerWins}, order)

	invalid := []Tiebreaker{"BOGUS"}
	_, err = apiTiebreakersToDomain(&invalid)
	require.Error(t, err)
	var gameErr *games.Error
	require.ErrorAs(t, err, &gameErr)
	assert.Equal(t, games.REASON_INVALID_TIEBREAKER, gameErr.Reason)
}
