package games

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func field(n int) []uuid.UUID {
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = uuid.New()
	}
	return ids
}

func pairKey(a, b uuid.UUID) string {
	if a.String() < b.String() {
		return a.String() + "|" + b.String()
	}
	return b.String() + "|" + a.String()
}

func TestRoundRobinCounts(t *testing.T) {
	for _, n := range []int{2, 3, 4, 5, 6, 7, 8, 12, 16} {
		got, err := GenerateRoundRobin(uuid.New(), field(n), false)
		require.NoError(t, err, "n=%d", n)
		assert.Len(t, got, n*(n-1)/2, "n=%d game count", n)

		rounds := map[int]int{}
		for _, g := range got {
			rounds[g.Round]++
			require.NoError(t, g.Validate(), "n=%d emitted game must validate", n)
			assert.Equal(t, GamePhaseQualifying, g.Phase)
			assert.Equal(t, GameStatusScheduled, g.Status)
		}

		wantRounds := n - 1
		if n%2 == 1 {
			wantRounds = n
		}
		assert.Len(t, rounds, wantRounds, "n=%d round count", n)
		for r, count := range rounds {
			assert.Equal(t, n/2, count, "n=%d round %d size", n, r)
		}
	}
}

func TestRoundRobinEveryPairOnce(t *testing.T) {
	for n := 3; n <= 16; n++ {
		ids := field(n)
		got, err := GenerateRoundRobin(uuid.New(), ids, false)
		require.NoError(t, err, "n=%d", n)

		seen := map[string]int{}
		for _, g := range got {
			seen[pairKey(*g.SideA.TeamID, *g.SideB.TeamID)]++
		}
		assert.Len(t, seen, n*(n-1)/2, "n=%d distinct pairs", n)
		for pair, count := range seen {
			assert.Equal(t, 1, count, "n=%d pair %s", n, pair)
		}
	}
}

func TestRoundRobinOddFieldRests(t *testing.T) {
	for _, n := range []int{3, 5, 7} {
		ids := field(n)
		eventID := uuid.New()
		got, err := GenerateRoundRobin(eventID, ids, false)
		require.NoError(t, err, "n=%d", n)

		played := map[uuid.UUID]map[int]bool{}
		for _, id := range ids {
			played[id] = map[int]bool{}
		}
		for _, g := range got {
			played[*g.SideA.TeamID][g.Round] = true
			played[*g.SideB.TeamID][g.Round] = true
		}
		for id, rounds := range played {
			assert.Len(t, rounds, n-1, "n=%d team %s plays N-1 rounds", n, id)
		}
	}
}

func TestRoundRobinSideBalance(t *testing.T) {
	for n := 2; n <= 16; n++ {
		got, err := GenerateRoundRobin(uuid.New(), field(n), false)
		require.NoError(t, err, "n=%d", n)

		counts := map[uuid.UUID][2]int{}
		for _, g := range got {
			a := counts[*g.SideA.TeamID]
			a[0]++
			counts[*g.SideA.TeamID] = a
			b := counts[*g.SideB.TeamID]
			b[1]++
			counts[*g.SideB.TeamID] = b
		}
		for id, c := range counts {
			diff := c[0] - c[1]
			if diff < 0 {
				diff = -diff
			}
			assert.LessOrEqual(t, diff, 1, "n=%d team %s side split %v", n, id, c)
		}
	}
}

func TestRoundRobinMirror(t *testing.T) {
	for _, n := range []int{3, 4, 5, 6} {
		ids := field(n)
		got, err := GenerateRoundRobin(uuid.New(), ids, true)
		require.NoError(t, err, "n=%d", n)
		assert.Len(t, got, n*(n-1), "n=%d mirrored game count", n)

		byPair := map[string][]Game{}
		for _, g := range got {
			require.NoError(t, g.Validate(), "n=%d mirrored game must validate", n)
			byPair[pairKey(*g.SideA.TeamID, *g.SideB.TeamID)] = append(
				byPair[pairKey(*g.SideA.TeamID, *g.SideB.TeamID)], g)
		}
		for pair, pairGames := range byPair {
			require.Len(t, pairGames, 2, "n=%d pair %s meets twice", n, pair)
			assert.Equal(t, *pairGames[0].SideA.TeamID, *pairGames[1].SideB.TeamID,
				"n=%d pair %s swaps sides", n, pair)
			assert.Equal(t, *pairGames[0].SideB.TeamID, *pairGames[1].SideA.TeamID,
				"n=%d pair %s swaps sides", n, pair)
		}
	}
}

func TestRoundRobinScheduleOrder(t *testing.T) {
	for _, n := range []int{5, 6, 7} {
		got, err := GenerateRoundRobin(uuid.New(), field(n), false)
		require.NoError(t, err, "n=%d", n)

		for i := 1; i < len(got); i++ {
			prev, cur := got[i-1], got[i]
			prevBefore := prev.Round < cur.Round ||
				(prev.Round == cur.Round && prev.Seq < cur.Seq)
			assert.True(t, prevBefore, "n=%d games emitted in schedule order", n)
		}
		seq := map[int]int{}
		for _, g := range got {
			seq[g.Round]++
			assert.Equal(t, seq[g.Round], g.Seq, "n=%d seq numbers dense per round", n)
		}
	}
}

func TestRoundRobinDeterministic(t *testing.T) {
	eventID := uuid.New()
	ids := field(8)

	first, err := GenerateRoundRobin(eventID, ids, false)
	require.NoError(t, err)

	reversed := append([]uuid.UUID{}, ids...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	second, err := GenerateRoundRobin(eventID, reversed, false)
	require.NoError(t, err)

	require.Len(t, second, len(first))
	for i := range first {
		assert.Equal(t, *first[i].SideA.TeamID, *second[i].SideA.TeamID)
		assert.Equal(t, *first[i].SideB.TeamID, *second[i].SideB.TeamID)
		assert.Equal(t, first[i].Round, second[i].Round)
		assert.Equal(t, first[i].Seq, second[i].Seq)
	}
}

func TestRoundRobinInputErrors(t *testing.T) {
	eventID := uuid.New()

	for _, tc := range []struct {
		name string
		ids  []uuid.UUID
	}{
		{"empty", nil},
		{"single", field(1)},
	} {
		_, err := GenerateRoundRobin(eventID, tc.ids, false)
		require.Error(t, err, tc.name)
		var gameErr *Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, REASON_INVALID_SCHEDULE, gameErr.Reason)
	}

	t.Run("duplicate teams rejected", func(t *testing.T) {
		dup := uuid.New()
		_, err := GenerateRoundRobin(eventID, []uuid.UUID{dup, dup, uuid.New()}, false)
		require.Error(t, err)
		var gameErr *Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, REASON_INVALID_SCHEDULE, gameErr.Reason)
	})

	t.Run("oversized fields rejected", func(t *testing.T) {
		_, err := GenerateRoundRobin(eventID, field(101), false)
		require.Error(t, err)
		_, err = GenerateRoundRobin(eventID, field(51), true)
		require.Error(t, err)
	})
}
