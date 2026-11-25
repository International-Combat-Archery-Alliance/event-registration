package games

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func swissTeams(names ...string) ([]uuid.UUID, map[string]string) {
	ids := make([]uuid.UUID, len(names))
	byName := map[string]string{}
	for i, name := range names {
		ids[i] = uuid.New()
		byName[ids[i].String()] = name
	}
	return ids, byName
}

func swissResult(eventID, a, b uuid.UUID, scoreA, scoreB, round, seq int) Game {
	return Game{
		ID:      uuid.New(),
		EventID: eventID,
		Phase:   GamePhaseQualifying,
		Round:   round,
		Seq:     seq,
		Status:  GameStatusCompleted,
		SideA:   Side{TeamID: &a},
		SideB:   Side{TeamID: &b},
		ScoreA:  &scoreA,
		ScoreB:  &scoreB,
		Version: 1,
	}
}

func pairTeams(g Game) (uuid.UUID, uuid.UUID) {
	return *g.SideA.TeamID, *g.SideB.TeamID
}

func TestSwissRoundOne(t *testing.T) {
	eventID := uuid.New()

	t.Run("even field pairs top-down by name", func(t *testing.T) {
		ids, names := swissTeams("A", "B", "C", "D", "E", "F")
		got, err := GenerateSwissRound(context.Background(), eventID, ids, names, nil, 1, nil)
		require.NoError(t, err)
		require.Len(t, got, 3)

		for _, g := range got {
			validateDraft(t, g)
			assert.Equal(t, GameStatusScheduled, g.Status)
			assert.Equal(t, 1, g.Round)
		}
		a1, b1 := pairTeams(got[0])
		assert.Equal(t, names[a1.String()], "A")
		assert.Equal(t, names[b1.String()], "B")
	})

	t.Run("odd field grants bye to weakest", func(t *testing.T) {
		ids, names := swissTeams("A", "B", "C", "D", "E")
		got, err := GenerateSwissRound(context.Background(), eventID, ids, names, nil, 1, nil)
		require.NoError(t, err)
		require.Len(t, got, 3)

		var bye *Game
		for i := range got {
			validateDraft(t, got[i])
			if got[i].Status == GameStatusBye {
				bye = &got[i]
			}
		}
		require.NotNil(t, bye, "odd field emits one BYE row")
		assert.Equal(t, "E", names[bye.SideA.TeamID.String()])
	})
}

func TestSwissWinnersMeet(t *testing.T) {
	eventID := uuid.New()
	ids, names := swissTeams("A", "B", "C", "D", "E", "F")
	a, b, c, d, e, f := ids[0], ids[1], ids[2], ids[3], ids[4], ids[5]

	priors := []Game{
		swissResult(eventID, a, b, 10, 5, 1, 1),
		swissResult(eventID, c, d, 10, 5, 1, 2),
		swissResult(eventID, e, f, 10, 5, 1, 3),
	}

	got, err := GenerateSwissRound(context.Background(), eventID, ids, names, priors, 2, nil)
	require.NoError(t, err)
	require.Len(t, got, 3)

	a1, b1 := pairTeams(got[0])
	assert.Equal(t, "A", names[a1.String()], "1-0 leaders meet top-down")
	assert.Equal(t, "C", names[b1.String()])
}

func TestSwissDownPairRelaxation(t *testing.T) {
	eventID := uuid.New()
	ids, names := swissTeams("A", "B", "C", "D", "E")
	a, b, c, d, e := ids[0], ids[1], ids[2], ids[3], ids[4]

	// A sole leader at 2-0 having already met the top of the 1-1 group
	// (B and C): down-pairs past both to D.
	priors := []Game{
		swissResult(eventID, a, b, 10, 5, 1, 1),
		swissResult(eventID, c, d, 10, 5, 1, 2),
		swissResult(eventID, a, c, 10, 5, 2, 1),
		swissResult(eventID, b, e, 10, 5, 2, 2),
		swissResult(eventID, d, e, 10, 5, 2, 3),
	}

	got, err := GenerateSwissRound(context.Background(), eventID, ids, names, priors, 3, nil)
	require.NoError(t, err)
	require.Len(t, got, 3)

	a1, b1 := pairTeams(got[0])
	assert.Equal(t, "A", names[a1.String()])
	assert.Equal(t, "D", names[b1.String()], "down-pairs past blocked B and C")

	var bye *Game
	for i := range got {
		validateDraft(t, got[i])
		if got[i].Status == GameStatusBye {
			bye = &got[i]
		}
	}
	require.NotNil(t, bye)
	assert.Equal(t, "E", names[bye.SideA.TeamID.String()], "winless E takes the bye")
}

func TestSwissNeverRematch(t *testing.T) {
	eventID := uuid.New()
	ids, names := swissTeams("A", "B", "C", "D", "E", "F")
	a, b, c, d, e, f := ids[0], ids[1], ids[2], ids[3], ids[4], ids[5]

	priors := []Game{
		swissResult(eventID, a, b, 10, 5, 1, 1),
		swissResult(eventID, a, c, 10, 0, 1, 2),
		swissResult(eventID, d, e, 10, 5, 1, 3),
		swissResult(eventID, d, f, 10, 5, 1, 4),
	}

	got, err := GenerateSwissRound(context.Background(), eventID, ids, names, priors, 2, nil)
	require.NoError(t, err)
	require.Len(t, got, 3)

	played := map[string]bool{}
	for _, g := range priors {
		a1, b1 := pairTeams(g)
		played[pairKeyOf(a1, b1)] = true
	}
	for _, g := range got {
		if g.Status == GameStatusBye {
			continue
		}
		a1, b1 := pairTeams(g)
		assert.False(t, played[pairKeyOf(a1, b1)], "never rematch")
	}
}

func TestSwissByeNotRepeated(t *testing.T) {
	eventID := uuid.New()
	ids, names := swissTeams("A", "B", "C", "D", "E")
	a, b, c, d, e := ids[0], ids[1], ids[2], ids[3], ids[4]

	byeRound := []Game{
		swissResult(eventID, a, b, 10, 5, 1, 1),
		swissResult(eventID, c, d, 10, 5, 1, 2),
		{ID: uuid.New(), EventID: eventID, Phase: GamePhaseQualifying, Round: 1, Seq: 3, Status: GameStatusBye, SideA: Side{TeamID: &e}, SideB: Side{IsBye: true}, Version: 1},
	}

	got, err := GenerateSwissRound(context.Background(), eventID, ids, names, byeRound, 2, nil)
	require.NoError(t, err)

	for _, g := range got {
		if g.Status == GameStatusBye {
			assert.NotEqual(t, e, *g.SideA.TeamID, "bye not repeated while others have none")
		}
	}
}

func TestSwissGuards(t *testing.T) {
	eventID := uuid.New()

	t.Run("tiny fields force round robin", func(t *testing.T) {
		allIDs, allNames := swissTeams("A", "B", "C", "D")
		for _, n := range []int{2, 3, 4} {
			_, err := GenerateSwissRound(context.Background(), eventID, allIDs[:n], allNames, nil, 1, nil)
			require.Error(t, err, "n=%d", n)
			var gameErr *Error
			require.ErrorAs(t, err, &gameErr)
			assert.Equal(t, REASON_INVALID_SCHEDULE, gameErr.Reason)
		}
	})

	t.Run("bad round rejected", func(t *testing.T) {
		ids, names := swissTeams("A", "B", "C", "D", "E", "F")
		_, err := GenerateSwissRound(context.Background(), eventID, ids, names, nil, 0, nil)
		require.Error(t, err)
	})

	t.Run("backtracks out of a greedy dead end", func(t *testing.T) {
		ids, names := swissTeams("A", "B", "C", "D", "E", "F")
		a, b, c, d, e, f := ids[0], ids[1], ids[2], ids[3], ids[4], ids[5]
		priors := []Game{
			swissResult(eventID, a, b, 10, 5, 1, 1),
			swissResult(eventID, c, d, 10, 5, 1, 2),
			swissResult(eventID, e, f, 10, 5, 1, 3),
			swissResult(eventID, a, c, 10, 5, 2, 1),
			swissResult(eventID, b, e, 10, 5, 2, 2),
			swissResult(eventID, d, f, 10, 5, 2, 3),
		}
		got, err := GenerateSwissRound(context.Background(), eventID, ids, names, priors, 3, nil)
		require.NoError(t, err, "a complete matching exists (e.g. A-D, B-F, C-E)")

		played := playedPairs(priors)
		require.Len(t, got, 3)
		for _, g := range got {
			validateDraft(t, g)
			require.Equal(t, GameStatusScheduled, g.Status)
			a1, b1 := pairTeams(g)
			assert.False(t, played[pairKeyOf(a1, b1)], "never rematch")
		}
	})
}

func TestSwissEmittedValidate(t *testing.T) {
	eventID := uuid.New()
	ids, names := swissTeams("A", "B", "C", "D", "E", "F", "G")
	a, b, c, d, e, f := ids[0], ids[1], ids[2], ids[3], ids[4], ids[5]

	priors := []Game{
		swissResult(eventID, a, b, 10, 5, 1, 1),
		swissResult(eventID, c, d, 10, 5, 1, 2),
		swissResult(eventID, e, f, 10, 5, 1, 3),
	}

	got, err := GenerateSwissRound(context.Background(), eventID, ids, names, priors, 2, nil)
	require.NoError(t, err)
	for _, game := range got {
		validateDraft(t, game)
		assert.Equal(t, 2, game.Round)
	}
}

// TestSwissEightTeamFavourites is the review's regression shape: 8 teams,
// the favourite (earlier name) wins every game, four rounds. A valid pairing
// exists in every round; the old first-available greedy failed this 300/300.
func TestSwissEightTeamFavourites(t *testing.T) {
	eventID := uuid.New()
	ids, names := swissTeams("A", "B", "C", "D", "E", "F", "G", "H")

	var prior []Game
	for round := 1; round <= 4; round++ {
		generated, err := GenerateSwissRound(context.Background(), eventID, ids, names, prior, round, nil)
		require.NoError(t, err, "round %d", round)
		require.Len(t, generated, 4)

		for _, g := range generated {
			validateDraft(t, g)
			prior = append(prior, favouriteResult(g, round))
		}
	}
}

func favouriteResult(g Game, round int) Game {
	scoreA, scoreB := 10, 5
	winnerSide := *g.SideA.TeamID
	loserSide := *g.SideB.TeamID
	if loserSide.String() < winnerSide.String() {
		scoreA, scoreB = 5, 10
	}
	result := g
	result.Round = round
	result.Status = GameStatusCompleted
	result.ScoreA, result.ScoreB = &scoreA, &scoreB
	return result
}

// TestSwissRandomFields is a multi-round property test: if any complete
// matching exists, GenerateSwissRound must find one; an error is only legal
// when brute force confirms the field is unmatchable.
func TestSwissRandomFields(t *testing.T) {
	for seed := int64(0); seed < 20; seed++ {
		t.Run(fmt.Sprintf("seed-%02d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			n := 6 + rng.Intn(7) // 6..12
			nameList := make([]string, n)
			for i := range nameList {
				nameList[i] = fmt.Sprintf("T%02d", i)
			}
			ids, names := swissTeams(nameList...)
			rounds := 3 + rng.Intn(3) // 3..5
			eventID := uuid.New()

			var prior []Game
			played := map[string]bool{}
			for round := 1; round <= rounds; round++ {
				feasible := swissRoundFeasible(ids, played)
				generated, err := GenerateSwissRound(context.Background(), eventID, ids, names, prior, round, nil)
				if !feasible {
					require.Error(t, err)
					var gameErr *Error
					require.ErrorAs(t, err, &gameErr)
					assert.Equal(t, REASON_NO_VALID_PAIRING, gameErr.Reason)
					return
				}
				require.NoError(t, err, "round %d", round)

				requireRoundShape(t, ids, generated, played, round)
				for _, g := range generated {
					prior = append(prior, randomResult(rng, g, round))
				}
				for _, g := range generated {
					if g.Status == GameStatusScheduled {
						played[pairKeyOf(*g.SideA.TeamID, *g.SideB.TeamID)] = true
					}
				}
			}
		})
	}
}

func requireRoundShape(t *testing.T, ids []uuid.UUID, generated []Game, played map[string]bool, round int) {
	t.Helper()

	seen := map[uuid.UUID]bool{}
	byeCount := 0
	for _, g := range generated {
		validateDraft(t, g)
		assert.Equal(t, round, g.Round)
		if g.Status == GameStatusBye {
			byeCount++
			assert.False(t, seen[*g.SideA.TeamID], "team appears twice in a round")
			seen[*g.SideA.TeamID] = true
			continue
		}
		require.Equal(t, GameStatusScheduled, g.Status)
		a, b := pairTeams(g)
		assert.False(t, seen[a], "team appears twice in a round")
		assert.False(t, seen[b], "team appears twice in a round")
		seen[a], seen[b] = true, true
		assert.False(t, played[pairKeyOf(a, b)], "rematch generated")
	}
	assert.Len(t, seen, len(ids), "every team must appear exactly once")
	assert.Equal(t, len(ids)%2, byeCount, "odd fields emit exactly one bye")
}

func randomResult(rng *rand.Rand, g Game, round int) Game {
	result := g
	result.Round = round
	if g.Status == GameStatusBye {
		return result
	}
	switch roll := rng.Intn(10); {
	case roll == 0:
		result.Status = GameStatusDoubleForfeit
	case roll == 1:
		result.Status = GameStatusForfeit
		side := GameSlotA
		if rng.Intn(2) == 0 {
			side = GameSlotB
		}
		result.ForfeitSide = &side
	default:
		scoreA := rng.Intn(11)
		scoreB := scoreA
		if rng.Intn(4) != 0 {
			scoreB = rng.Intn(10)
			if scoreB >= scoreA {
				scoreB++
			}
		}
		result.Status = GameStatusCompleted
		result.ScoreA, result.ScoreB = &scoreA, &scoreB
	}
	return result
}

func swissRoundFeasible(ids []uuid.UUID, played map[string]bool) bool {
	if len(ids)%2 == 0 {
		return hasPerfectMatching(ids, played)
	}
	for i := range ids {
		rest := make([]uuid.UUID, 0, len(ids)-1)
		rest = append(rest, ids[:i]...)
		rest = append(rest, ids[i+1:]...)
		if hasPerfectMatching(rest, played) {
			return true
		}
	}
	return false
}

func hasPerfectMatching(ids []uuid.UUID, played map[string]bool) bool {
	paired := make([]bool, len(ids))
	var solve func() bool
	solve = func() bool {
		next := -1
		for i := range ids {
			if !paired[i] {
				next = i
				break
			}
		}
		if next == -1 {
			return true
		}
		for j := next + 1; j < len(ids); j++ {
			if paired[j] || played[pairKeyOf(ids[next], ids[j])] {
				continue
			}
			paired[next], paired[j] = true, true
			if solve() {
				return true
			}
			paired[next], paired[j] = false, false
		}
		return false
	}
	return solve()
}

// TestSwissStrandedTeamIsNoValidPairing is the review's exponential shape:
// the last team has a hand-created SCHEDULED game against every other team,
// so no complete matching exists, and a naive DFS enumerates every matching
// of the remaining 15 teams before discovering it. The O(n^2) stranded-team
// precheck must recognize the provable dead end immediately and report
// NO_VALID_PAIRING instead of burning the budget on an answer it would then
// mislabel as "too complex".
func TestSwissStrandedTeamIsNoValidPairing(t *testing.T) {
	eventID := uuid.New()
	nameList := make([]string, 16)
	for i := range nameList {
		nameList[i] = fmt.Sprintf("T%02d", i)
	}
	ids, names := swissTeams(nameList...)
	stranded := ids[len(ids)-1]

	var priors []Game
	for i, opponent := range ids[:len(ids)-1] {
		priors = append(priors, Game{
			ID:      uuid.New(),
			EventID: eventID,
			Phase:   GamePhaseQualifying,
			Round:   1,
			Seq:     i + 1,
			Status:  GameStatusScheduled,
			SideA:   Side{TeamID: &stranded},
			SideB:   Side{TeamID: &opponent},
			Version: 1,
		})
	}

	start := time.Now()
	_, err := GenerateSwissRound(context.Background(), eventID, ids, names, priors, 2, nil)
	elapsed := time.Since(start)

	require.Error(t, err)
	var gameErr *Error
	require.ErrorAs(t, err, &gameErr)
	assert.Equal(t, REASON_NO_VALID_PAIRING, gameErr.Reason)
	assert.Less(t, elapsed, 2*time.Second, "the stranded team is caught before the search")
}

// TestSwissHundredTeamFieldPairs is the coverage side of the budget: an
// ordinary large field must still pair for several rounds.
func TestSwissHundredTeamFieldPairs(t *testing.T) {
	eventID := uuid.New()
	nameList := make([]string, 100)
	for i := range nameList {
		nameList[i] = fmt.Sprintf("T%03d", i)
	}
	ids, names := swissTeams(nameList...)

	var prior []Game
	for round := 1; round <= 5; round++ {
		generated, err := GenerateSwissRound(context.Background(), eventID, ids, names, prior, round, nil)
		require.NoError(t, err, "round %d must pair inside the budget", round)
		require.Len(t, generated, 50)
		for _, g := range generated {
			validateDraft(t, g)
			prior = append(prior, favouriteResult(g, round))
		}
	}
}

// TestSwissByeRecipientIsTrueWeakest is the review's net-first fixture: the
// 0-win group is not contiguous in net-first order, so a tail scan misses the
// true weakest team. C is the weakest 0-win team and must take the bye; the
// old tail scan saw only the last row (B, one win) and gave it to B.
func TestSwissByeRecipientIsTrueWeakest(t *testing.T) {
	eventID := uuid.New()
	ids, names := swissTeams("A", "B", "C", "D", "E")
	a, b, c, d, e := ids[0], ids[1], ids[2], ids[3], ids[4]

	priors := []Game{
		swissResult(eventID, a, c, 12, 0, 1, 1),
		swissResult(eventID, a, d, 8, 0, 1, 2),
		swissResult(eventID, b, e, 5, 0, 1, 3),
		swissResult(eventID, a, b, 20, 0, 1, 4),
	}

	got, err := GenerateSwissRound(context.Background(), eventID, ids, names, priors, 2, nil)
	require.NoError(t, err)

	var bye *Game
	for i := range got {
		if got[i].Status == GameStatusBye {
			bye = &got[i]
		}
	}
	require.NotNil(t, bye)
	assert.Equal(t, "C", names[bye.SideA.TeamID.String()], "the true weakest 0-win team takes the bye")
}

func TestSwissPairingHonoursCancelledContext(t *testing.T) {
	eventID := uuid.New()
	nameList := make([]string, 16)
	for i := range nameList {
		nameList[i] = fmt.Sprintf("T%02d", i)
	}
	ids, names := swissTeams(nameList...)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := GenerateSwissRound(ctx, eventID, ids, names, nil, 1, nil)
	require.Error(t, err)
	var gameErr *Error
	require.ErrorAs(t, err, &gameErr)
	assert.Equal(t, REASON_TIMEOUT, gameErr.Reason)
}
