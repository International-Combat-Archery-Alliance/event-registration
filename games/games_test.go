package games

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTeams() (uuid.UUID, uuid.UUID) {
	return uuid.New(), uuid.New()
}

func realSide(id uuid.UUID) Side {
	return Side{TeamID: &id}
}

func intPtr(i int) *int {
	return &i
}

func slotPtr(s GameSlot) *GameSlot {
	return &s
}

func validScheduledGame() Game {
	a, b := testTeams()
	return Game{
		ID:      uuid.New(),
		EventID: uuid.New(),
		Phase:   GamePhaseQualifying,
		Round:   1,
		Seq:     1,
		Status:  GameStatusScheduled,
		SideA:   realSide(a),
		SideB:   realSide(b),
		Version: 1,
	}
}

func TestGameValidateStructure(t *testing.T) {
	t.Run("valid scheduled game passes", func(t *testing.T) {
		require.NoError(t, validScheduledGame().Validate())
	})

	t.Run("invalid phase rejected", func(t *testing.T) {
		g := validScheduledGame()
		g.Phase = GamePhase("BOGUS")
		err := g.Validate()
		require.Error(t, err)
		var gameErr *Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, REASON_INVALID_GAME_PHASE, gameErr.Reason)
	})

	t.Run("invalid status rejected", func(t *testing.T) {
		g := validScheduledGame()
		g.Status = GameStatus("BOGUS")
		err := g.Validate()
		require.Error(t, err)
		var gameErr *Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, REASON_INVALID_GAME_STATUS, gameErr.Reason)
	})

	t.Run("round and seq bounds enforced", func(t *testing.T) {
		for _, tc := range []struct {
			round, seq int
		}{{0, 1}, {1, 0}, {100, 1}, {1, 100}, {-1, 1}} {
			g := validScheduledGame()
			g.Round, g.Seq = tc.round, tc.seq
			err := g.Validate()
			require.Error(t, err, "round=%d seq=%d", tc.round, tc.seq)
			var gameErr *Error
			require.ErrorAs(t, err, &gameErr)
			assert.Equal(t, REASON_INVALID_GAME_RESULT, gameErr.Reason)
		}
	})

	t.Run("round and seq upper bound passes", func(t *testing.T) {
		g := validScheduledGame()
		g.Round, g.Seq = 99, 99
		require.NoError(t, g.Validate())
	})

	t.Run("bye side must not carry a team", func(t *testing.T) {
		g := validScheduledGame()
		team := uuid.New()
		g.SideA = Side{TeamID: &team, IsBye: true}
		err := g.Validate()
		require.Error(t, err)
		var gameErr *Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, REASON_INVALID_GAME_RESULT, gameErr.Reason)
	})

	t.Run("invalid forfeit side rejected", func(t *testing.T) {
		a, b := testTeams()
		g := validScheduledGame()
		g.Status = GameStatusForfeit
		g.SideA, g.SideB = realSide(a), realSide(b)
		bogus := GameSlot("C")
		g.ForfeitSide = &bogus
		err := g.Validate()
		require.Error(t, err)
		var gameErr *Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, REASON_INVALID_GAME_RESULT, gameErr.Reason)
	})

	t.Run("playoff sides may be undetermined until feeders resolve", func(t *testing.T) {
		g := validScheduledGame()
		g.Phase = GamePhasePlayoff
		g.SideA, g.SideB = Side{}, Side{}
		require.NoError(t, g.Validate(), "TBD sides allowed on scheduled playoff games")
	})

	t.Run("undetermined sides cannot contest a result", func(t *testing.T) {
		g := validScheduledGame()
		g.Phase = GamePhasePlayoff
		g.Status = GameStatusCompleted
		g.SideA, g.SideB = Side{}, Side{}
		g.ScoreA, g.ScoreB = intPtr(5), intPtr(3)
		require.Error(t, g.Validate(), "TBD sides cannot contest a result")
	})

	t.Run("negative scores rejected", func(t *testing.T) {
		a, b := testTeams()
		g := validScheduledGame()
		g.Status = GameStatusCompleted
		g.SideA, g.SideB = realSide(a), realSide(b)
		g.ScoreA, g.ScoreB = intPtr(-1), intPtr(5)
		require.Error(t, g.Validate())
	})
}

func TestGameValidateResults(t *testing.T) {
	t.Run("completed requires scores and a winner", func(t *testing.T) {
		a, b := testTeams()

		g := validScheduledGame()
		g.Status = GameStatusCompleted
		g.SideA, g.SideB = realSide(a), realSide(b)
		require.Error(t, g.Validate(), "missing scores")

		g.ScoreA, g.ScoreB = intPtr(10), intPtr(10)
		err := g.Validate()
		require.Error(t, err, "draws disallowed")
		var gameErr *Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, REASON_INVALID_GAME_RESULT, gameErr.Reason)

		g.ScoreA, g.ScoreB = intPtr(10), intPtr(8)
		require.NoError(t, g.Validate())
	})

	t.Run("completed requires two distinct real sides", func(t *testing.T) {
		a := uuid.New()

		g := validScheduledGame()
		g.Status = GameStatusCompleted
		g.SideA, g.SideB = realSide(a), realSide(a)
		g.ScoreA, g.ScoreB = intPtr(3), intPtr(1)
		require.Error(t, g.Validate(), "same team both sides")

		g.SideB = Side{IsBye: true}
		require.Error(t, g.Validate(), "bye side cannot contest a result")
	})

	t.Run("completed must not carry a forfeit side", func(t *testing.T) {
		a, b := testTeams()
		g := validScheduledGame()
		g.Status = GameStatusCompleted
		g.SideA, g.SideB = realSide(a), realSide(b)
		g.ScoreA, g.ScoreB = intPtr(5), intPtr(3)
		g.ForfeitSide = slotPtr(GameSlotA)
		require.Error(t, g.Validate())
	})

	t.Run("forfeit requires a forfeit side", func(t *testing.T) {
		a, b := testTeams()
		g := validScheduledGame()
		g.Status = GameStatusForfeit
		g.SideA, g.SideB = realSide(a), realSide(b)
		require.Error(t, g.Validate(), "missing forfeit side")

		g.ForfeitSide = slotPtr(GameSlotB)
		require.NoError(t, g.Validate(), "admin-recorded scores optional on forfeit")
	})

	t.Run("double forfeit records two losses", func(t *testing.T) {
		a, b := testTeams()
		g := validScheduledGame()
		g.Status = GameStatusDoubleForfeit
		g.SideA, g.SideB = realSide(a), realSide(b)
		require.NoError(t, g.Validate(), "scores absent")

		g.ScoreA, g.ScoreB = intPtr(0), intPtr(0)
		require.NoError(t, g.Validate(), "explicit 0-0 allowed")

		g.ScoreA = intPtr(3)
		require.Error(t, g.Validate(), "non-zero scores rejected")

		g.ScoreA, g.ScoreB = intPtr(0), nil
		require.Error(t, g.Validate(), "split absent/zero scores rejected")

		g.ScoreA = nil
		g.ForfeitSide = slotPtr(GameSlotA)
		require.Error(t, g.Validate(), "must not single out a side")
	})

	t.Run("scheduled cancelled and bye carry no result", func(t *testing.T) {
		g := validScheduledGame()
		g.ScoreA = intPtr(1)
		require.Error(t, g.Validate(), "scheduled with scores")

		g = validScheduledGame()
		g.Status = GameStatusCancelled
		require.NoError(t, g.Validate())
		g.ScoreA = intPtr(1)
		require.Error(t, g.Validate(), "cancelled with scores")

		g = validScheduledGame()
		g.Status = GameStatusBye
		g.SideB = Side{IsBye: true}
		require.NoError(t, g.Validate())
		g.ScoreA = intPtr(1)
		require.Error(t, g.Validate(), "bye with scores")
	})

	t.Run("bye goes to exactly one team", func(t *testing.T) {
		a := uuid.New()

		g := validScheduledGame()
		g.Status = GameStatusBye
		g.SideA, g.SideB = realSide(a), Side{IsBye: true}
		require.NoError(t, g.Validate())

		g.SideB = realSide(uuid.New())
		require.Error(t, g.Validate(), "two real sides is a game, not a bye")

		g.SideA, g.SideB = Side{IsBye: true}, Side{IsBye: true}
		require.Error(t, g.Validate(), "two byes grant nothing")

		g.SideA, g.SideB = realSide(a), Side{}
		require.Error(t, g.Validate(), "undetermined side cannot receive a bye")
	})
}

func TestGameStatusResolved(t *testing.T) {
	assert.False(t, GameStatusScheduled.Resolved())
	for _, s := range []GameStatus{
		GameStatusCompleted, GameStatusForfeit, GameStatusDoubleForfeit,
		GameStatusCancelled, GameStatusBye,
	} {
		assert.True(t, s.Resolved(), string(s))
	}
	assert.False(t, GameStatus("BOGUS").Resolved())
}
