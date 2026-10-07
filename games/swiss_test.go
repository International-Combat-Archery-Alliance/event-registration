package games

import (
	"testing"

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
		got, err := GenerateSwissRound(eventID, ids, names, nil, 1)
		require.NoError(t, err)
		require.Len(t, got, 3)

		for _, g := range got {
			require.NoError(t, g.Validate())
			assert.Equal(t, GameStatusScheduled, g.Status)
			assert.Equal(t, 1, g.Round)
		}
		a1, b1 := pairTeams(got[0])
		assert.Equal(t, names[a1.String()], "A")
		assert.Equal(t, names[b1.String()], "B")
	})

	t.Run("odd field grants bye to weakest", func(t *testing.T) {
		ids, names := swissTeams("A", "B", "C", "D", "E")
		got, err := GenerateSwissRound(eventID, ids, names, nil, 1)
		require.NoError(t, err)
		require.Len(t, got, 3)

		var bye *Game
		for i := range got {
			require.NoError(t, got[i].Validate())
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

	got, err := GenerateSwissRound(eventID, ids, names, priors, 2)
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

	got, err := GenerateSwissRound(eventID, ids, names, priors, 3)
	require.NoError(t, err)
	require.Len(t, got, 3)

	a1, b1 := pairTeams(got[0])
	assert.Equal(t, "A", names[a1.String()])
	assert.Equal(t, "D", names[b1.String()], "down-pairs past blocked B and C")

	var bye *Game
	for i := range got {
		require.NoError(t, got[i].Validate())
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

	got, err := GenerateSwissRound(eventID, ids, names, priors, 2)
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

	got, err := GenerateSwissRound(eventID, ids, names, byeRound, 2)
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
			_, err := GenerateSwissRound(eventID, allIDs[:n], allNames, nil, 1)
			require.Error(t, err, "n=%d", n)
			var gameErr *Error
			require.ErrorAs(t, err, &gameErr)
			assert.Equal(t, REASON_INVALID_SCHEDULE, gameErr.Reason)
		}
	})

	t.Run("bad round rejected", func(t *testing.T) {
		ids, names := swissTeams("A", "B", "C", "D", "E", "F")
		_, err := GenerateSwissRound(eventID, ids, names, nil, 0)
		require.Error(t, err)
	})

	t.Run("dead end errors without rematch", func(t *testing.T) {
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
		_, err := GenerateSwissRound(eventID, ids, names, priors, 3)
		require.Error(t, err, "E has only met F among the remainder")
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

	got, err := GenerateSwissRound(eventID, ids, names, priors, 2)
	require.NoError(t, err)
	for _, game := range got {
		require.NoError(t, game.Validate())
		assert.Equal(t, 2, game.Round)
	}
}
