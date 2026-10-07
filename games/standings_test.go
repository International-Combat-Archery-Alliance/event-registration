package games

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func standingTeams(n int) ([]uuid.UUID, map[string]string) {
	ids := field(n)
	names := map[string]string{}
	for i, id := range ids {
		names[id.String()] = string(rune('A' + i))
	}
	return ids, names
}

func completedGame(eventID, a, b uuid.UUID, scoreA, scoreB int, phase GamePhase) Game {
	return Game{
		ID:      uuid.New(),
		EventID: eventID,
		Phase:   phase,
		Round:   1,
		Seq:     1,
		Status:  GameStatusCompleted,
		SideA:   Side{TeamID: &a},
		SideB:   Side{TeamID: &b},
		ScoreA:  &scoreA,
		ScoreB:  &scoreB,
		Version: 1,
	}
}

func TestComputeStandingsBasic(t *testing.T) {
	eventID := uuid.New()
	ids, names := standingTeams(4)
	a, b, c, d := ids[0], ids[1], ids[2], ids[3]

	games := []Game{
		completedGame(eventID, a, b, 10, 5, GamePhaseQualifying),
		completedGame(eventID, c, d, 8, 3, GamePhaseQualifying),
		completedGame(eventID, a, c, 7, 6, GamePhaseQualifying),
	}

	got := ComputeStandings(games, ids, names)
	require.Len(t, got, 4)

	assert.Equal(t, a, got[0].TeamID)
	assert.Equal(t, 2, got[0].GP)
	assert.Equal(t, 2, got[0].Wins)
	assert.Equal(t, 0, got[0].Losses)
	assert.Equal(t, 17, got[0].PF)
	assert.Equal(t, 11, got[0].PA)
	assert.Equal(t, 6, got[0].Net)
	assert.Equal(t, 1, got[0].Rank)
	assert.False(t, got[0].DNS)

	assert.Equal(t, c, got[1].TeamID)
	assert.Equal(t, 1, got[1].Wins)
	assert.Equal(t, 1, got[1].Losses)
	assert.Equal(t, 2, got[1].Rank)

	assert.Equal(t, b, got[2].TeamID, "winless tie broken by PF")
	assert.Equal(t, 0, got[2].Wins)
	assert.Equal(t, 3, got[2].Rank)

	assert.Equal(t, d, got[3].TeamID)
	assert.Equal(t, 4, got[3].Rank)
}

func TestComputeStandingsTiebreaks(t *testing.T) {
	eventID := uuid.New()
	ids, names := standingTeams(3)
	a, b, c := ids[0], ids[1], ids[2]

	t.Run("net breaks win ties", func(t *testing.T) {
		games := []Game{
			completedGame(eventID, a, c, 10, 0, GamePhaseQualifying),
			completedGame(eventID, b, c, 5, 4, GamePhaseQualifying),
		}
		got := ComputeStandings(games, ids, names)
		assert.Equal(t, a, got[0].TeamID)
		assert.Equal(t, b, got[1].TeamID)
	})

	t.Run("pf breaks net ties", func(t *testing.T) {
		games := []Game{
			completedGame(eventID, a, c, 10, 5, GamePhaseQualifying),
			completedGame(eventID, b, c, 8, 3, GamePhaseQualifying),
		}
		got := ComputeStandings(games, ids, names)
		assert.Equal(t, a, got[0].TeamID)
		assert.Equal(t, 10, got[0].PF)
		assert.Equal(t, b, got[1].TeamID)
	})

	t.Run("name breaks full ties", func(t *testing.T) {
		games := []Game{
			completedGame(eventID, a, c, 10, 5, GamePhaseQualifying),
			completedGame(eventID, b, c, 10, 5, GamePhaseQualifying),
		}
		got := ComputeStandings(games, ids, names)
		assert.Equal(t, a, got[0].TeamID)
		assert.Equal(t, b, got[1].TeamID)
	})
}

func TestComputeStandingsForfeit(t *testing.T) {
	eventID := uuid.New()
	ids, names := standingTeams(2)
	a, b := ids[0], ids[1]

	side := GameSlotA
	scoreA, scoreB := 0, 7
	g := Game{
		ID:          uuid.New(),
		EventID:     eventID,
		Phase:       GamePhaseQualifying,
		Round:       1,
		Seq:         1,
		Status:      GameStatusForfeit,
		SideA:       Side{TeamID: &a},
		SideB:       Side{TeamID: &b},
		ScoreA:      &scoreA,
		ScoreB:      &scoreB,
		ForfeitSide: &side,
		Version:     1,
	}

	got := ComputeStandings([]Game{g}, ids, names)
	require.Len(t, got, 2)
	assert.Equal(t, b, got[0].TeamID, "non-forfeiting side wins")
	assert.Equal(t, 1, got[0].Wins)
	assert.Equal(t, 7, got[0].PF)
	assert.Equal(t, a, got[1].TeamID)
	assert.Equal(t, 1, got[1].Losses)
}

func TestComputeStandingsExclusions(t *testing.T) {
	eventID := uuid.New()
	ids, names := standingTeams(3)
	a, b := ids[0], ids[1]

	doubleScore := 0
	excluded := []Game{
		{ID: uuid.New(), EventID: eventID, Phase: GamePhaseQualifying, Round: 1, Seq: 1, Status: GameStatusBye, SideA: Side{TeamID: &a}, SideB: Side{IsBye: true}, Version: 1},
		{ID: uuid.New(), EventID: eventID, Phase: GamePhaseQualifying, Round: 1, Seq: 2, Status: GameStatusCancelled, SideA: Side{TeamID: &a}, SideB: Side{TeamID: &b}, Version: 1},
		{ID: uuid.New(), EventID: eventID, Phase: GamePhaseQualifying, Round: 1, Seq: 3, Status: GameStatusDoubleForfeit, SideA: Side{TeamID: &a}, SideB: Side{TeamID: &b}, ScoreA: &doubleScore, ScoreB: &doubleScore, Version: 1},
		completedGame(eventID, a, b, 10, 5, GamePhasePlayoff),
	}

	got := ComputeStandings(excluded, ids, names)
	for _, s := range got {
		assert.Equal(t, 0, s.GP, "excluded games never count")
		assert.True(t, s.DNS)
		assert.Equal(t, 0, s.Rank)
	}
}

func TestComputeStandingsDNS(t *testing.T) {
	eventID := uuid.New()
	ids, names := standingTeams(3)
	a, b, c := ids[0], ids[1], ids[2]

	games := []Game{
		completedGame(eventID, a, b, 10, 5, GamePhaseQualifying),
	}

	got := ComputeStandings(games, ids, names)
	require.Len(t, got, 3)
	assert.Equal(t, a, got[0].TeamID)
	assert.Equal(t, 1, got[0].Rank)
	assert.Equal(t, b, got[1].TeamID)
	assert.Equal(t, 2, got[1].Rank)

	assert.Equal(t, c, got[2].TeamID, "DNS sinks below placed teams")
	assert.True(t, got[2].DNS)
	assert.Equal(t, 0, got[2].Rank)
	assert.Equal(t, 0, got[2].GP)
}

func TestComputeStandingsIgnoresOutsiders(t *testing.T) {
	eventID := uuid.New()
	ids, names := standingTeams(2)
	a, b := ids[0], ids[1]
	outsider := uuid.New()

	games := []Game{
		completedGame(eventID, a, outsider, 10, 5, GamePhaseQualifying),
		completedGame(eventID, a, b, 10, 5, GamePhaseQualifying),
	}

	got := ComputeStandings(games, ids, names)
	require.Len(t, got, 2)
	assert.Equal(t, 1, got[0].GP, "games vs non-participants are ignored")
	assert.Equal(t, 1, got[0].Wins)
}

func TestSortTeamRecordsShared(t *testing.T) {
	records := []TeamRecord{
		{TeamID: uuid.New(), TeamName: "B", Wins: 1, Net: 0, PF: 5},
		{TeamID: uuid.New(), TeamName: "A", Wins: 1, Net: 0, PF: 5},
		{TeamID: uuid.New(), TeamName: "C", Wins: 2, Net: -10, PF: 0},
	}
	SortTeamRecords(records)
	assert.Equal(t, "C", records[0].TeamName)
	assert.Equal(t, "A", records[1].TeamName)
	assert.Equal(t, "B", records[2].TeamName)
}
