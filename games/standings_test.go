package games

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
)

func standingTeams(n int) ([]uuid.UUID, map[string]string) {
	ids := field(n)
	names := map[string]string{}
	for i, id := range ids {
		names[id.String()] = string(rune('A' + i))
	}
	return ids, names
}

func confirmedParticipants(ids []uuid.UUID) []StandingTeam {
	out := make([]StandingTeam, len(ids))
	for i, id := range ids {
		out[i] = StandingTeam{TeamID: id, Status: teams.ParticipationStatusConfirmed}
	}
	return out
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

	got := ComputeStandings(games, confirmedParticipants(ids), names, nil)
	require.Len(t, got, 4)

	assert.Equal(t, a, got[0].TeamID)
	assert.Equal(t, 2, got[0].GP)
	assert.Equal(t, 2, got[0].Wins)
	assert.Equal(t, 0, got[0].Draws)
	assert.Equal(t, 0, got[0].Losses)
	assert.Equal(t, 17, got[0].PF)
	assert.Equal(t, 11, got[0].PA)
	assert.Equal(t, 6, got[0].Net)
	assert.Equal(t, 1, got[0].QualifyingRank)
	assert.False(t, got[0].DNS)

	assert.Equal(t, c, got[1].TeamID)
	assert.Equal(t, 1, got[1].Wins)
	assert.Equal(t, 1, got[1].Losses)
	assert.Equal(t, 2, got[1].QualifyingRank)

	assert.Equal(t, b, got[2].TeamID, "winless tie broken by PF")
	assert.Equal(t, 0, got[2].Wins)
	assert.Equal(t, 3, got[2].QualifyingRank)

	assert.Equal(t, d, got[3].TeamID)
	assert.Equal(t, 4, got[3].QualifyingRank)
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
		got := ComputeStandings(games, confirmedParticipants(ids), names, nil)
		assert.Equal(t, a, got[0].TeamID)
		assert.Equal(t, b, got[1].TeamID)
	})

	t.Run("pf breaks net ties", func(t *testing.T) {
		games := []Game{
			completedGame(eventID, a, c, 10, 5, GamePhaseQualifying),
			completedGame(eventID, b, c, 8, 3, GamePhaseQualifying),
		}
		got := ComputeStandings(games, confirmedParticipants(ids), names, nil)
		assert.Equal(t, a, got[0].TeamID)
		assert.Equal(t, 10, got[0].PF)
		assert.Equal(t, b, got[1].TeamID)
	})

	t.Run("name breaks full ties", func(t *testing.T) {
		games := []Game{
			completedGame(eventID, a, c, 10, 5, GamePhaseQualifying),
			completedGame(eventID, b, c, 10, 5, GamePhaseQualifying),
		}
		got := ComputeStandings(games, confirmedParticipants(ids), names, nil)
		assert.Equal(t, a, got[0].TeamID)
		assert.Equal(t, b, got[1].TeamID)
	})
}

func TestComputeStandingsTiebreakOrders(t *testing.T) {
	eventID := uuid.New()
	ids, names := standingTeams(4)
	a, b, c, d := ids[0], ids[1], ids[2], ids[3]

	// A is 2-0 with a +2 net; B is 1-1 with a +49 net. Net-first puts B
	// ahead, wins-first puts A ahead.
	games := []Game{
		completedGame(eventID, a, c, 1, 0, GamePhaseQualifying),
		completedGame(eventID, a, d, 1, 0, GamePhaseQualifying),
		completedGame(eventID, b, c, 50, 0, GamePhaseQualifying),
		completedGame(eventID, d, b, 1, 0, GamePhaseQualifying),
	}

	t.Run("default is net first", func(t *testing.T) {
		got := ComputeStandings(games, confirmedParticipants(ids), names, nil)
		assert.Equal(t, b, got[0].TeamID, "net-first ranks the big net first")
		assert.Equal(t, a, got[1].TeamID)
	})

	t.Run("wins-first remains selectable", func(t *testing.T) {
		got := ComputeStandings(games, confirmedParticipants(ids), names, WinsFirstTiebreakOrder())
		assert.Equal(t, a, got[0].TeamID, "wins-first ranks the 2-0 team first")
		assert.Equal(t, b, got[1].TeamID)
	})
}

func TestComputeStandingsDraws(t *testing.T) {
	eventID := uuid.New()
	ids, names := standingTeams(2)
	a, b := ids[0], ids[1]

	got := ComputeStandings([]Game{
		completedGame(eventID, a, b, 5, 5, GamePhaseQualifying),
	}, confirmedParticipants(ids), names, nil)
	require.Len(t, got, 2)

	for _, s := range got {
		assert.Equal(t, 1, s.GP)
		assert.Equal(t, 1, s.Draws)
		assert.Equal(t, 0, s.Wins, "draws are never half-wins")
		assert.Equal(t, 0, s.Losses)
		assert.Equal(t, 5, s.PF)
		assert.Equal(t, 5, s.PA)
		assert.Equal(t, 0, s.Net)
		assert.False(t, s.DNS)
	}
	assert.ElementsMatch(t, []int{1, 2}, []int{got[0].QualifyingRank, got[1].QualifyingRank})
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

	got := ComputeStandings([]Game{g}, confirmedParticipants(ids), names, nil)
	require.Len(t, got, 2)
	assert.Equal(t, b, got[0].TeamID, "non-forfeiting side wins")
	assert.Equal(t, 1, got[0].Wins)
	assert.Equal(t, 1, got[0].GP)
	assert.Equal(t, 0, got[0].PF, "forfeit scores are decoration")
	assert.Equal(t, 0, got[0].PA)
	assert.Equal(t, 0, got[0].Net)
	assert.Equal(t, a, got[1].TeamID)
	assert.Equal(t, 1, got[1].Losses)
	assert.Equal(t, 0, got[1].PF)
	assert.Equal(t, 0, got[1].PA)
}

func TestComputeStandingsDoubleForfeit(t *testing.T) {
	eventID := uuid.New()
	ids, names := standingTeams(2)
	a, b := ids[0], ids[1]

	zero := 0
	g := Game{
		ID:      uuid.New(),
		EventID: eventID,
		Phase:   GamePhaseQualifying,
		Round:   1,
		Seq:     1,
		Status:  GameStatusDoubleForfeit,
		SideA:   Side{TeamID: &a},
		SideB:   Side{TeamID: &b},
		ScoreA:  &zero,
		ScoreB:  &zero,
		Version: 1,
	}

	got := ComputeStandings([]Game{g}, confirmedParticipants(ids), names, nil)
	require.Len(t, got, 2)
	for _, s := range got {
		assert.Equal(t, 1, s.GP)
		assert.Equal(t, 1, s.Losses, "double forfeit is a loss for both")
		assert.Equal(t, 0, s.Wins)
		assert.Equal(t, 0, s.Draws)
		assert.Equal(t, 0, s.PF)
		assert.Equal(t, 0, s.PA)
		assert.False(t, s.DNS)
	}
}

func TestComputeStandingsExclusions(t *testing.T) {
	eventID := uuid.New()
	ids, names := standingTeams(3)
	a, b := ids[0], ids[1]

	excluded := []Game{
		{ID: uuid.New(), EventID: eventID, Phase: GamePhaseQualifying, Round: 1, Seq: 1, Status: GameStatusBye, SideA: Side{TeamID: &a}, SideB: Side{IsBye: true}, Version: 1},
		{ID: uuid.New(), EventID: eventID, Phase: GamePhaseQualifying, Round: 1, Seq: 2, Status: GameStatusCancelled, SideA: Side{TeamID: &a}, SideB: Side{TeamID: &b}, Version: 1},
		{ID: uuid.New(), EventID: eventID, Phase: GamePhaseQualifying, Round: 1, Seq: 3, Status: GameStatusScheduled, SideA: Side{TeamID: &a}, SideB: Side{TeamID: &b}, Version: 1},
		completedGame(eventID, a, b, 10, 5, GamePhasePlayoff),
	}

	got := ComputeStandings(excluded, confirmedParticipants(ids), names, nil)
	for _, s := range got {
		assert.Equal(t, 0, s.GP, "excluded games never count")
		assert.True(t, s.DNS)
		assert.Equal(t, 0, s.QualifyingRank)
	}
}

func TestComputeStandingsNonConfirmedExcluded(t *testing.T) {
	eventID := uuid.New()
	ids, names := standingTeams(2)
	a, b := ids[0], ids[1]

	participants := []StandingTeam{
		{TeamID: a, Status: teams.ParticipationStatusConfirmed},
		{TeamID: b, Status: teams.ParticipationStatusRegistered},
	}

	got := ComputeStandings([]Game{
		completedGame(eventID, a, b, 10, 5, GamePhaseQualifying),
	}, participants, names, nil)
	require.Len(t, got, 2)
	for _, s := range got {
		assert.True(t, s.DNS, "a non-CONFIRMED entry earns no counted games")
		assert.Equal(t, 0, s.GP)
	}
}

func TestComputeStandingsDNS(t *testing.T) {
	eventID := uuid.New()
	ids, names := standingTeams(3)
	a, b, c := ids[0], ids[1], ids[2]

	games := []Game{
		completedGame(eventID, a, b, 10, 5, GamePhaseQualifying),
	}

	got := ComputeStandings(games, confirmedParticipants(ids), names, nil)
	require.Len(t, got, 3)
	assert.Equal(t, a, got[0].TeamID)
	assert.Equal(t, 1, got[0].QualifyingRank)
	assert.Equal(t, b, got[1].TeamID)
	assert.Equal(t, 2, got[1].QualifyingRank)

	assert.Equal(t, c, got[2].TeamID, "DNS sinks below placed teams")
	assert.True(t, got[2].DNS)
	assert.Equal(t, 0, got[2].QualifyingRank)
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

	got := ComputeStandings(games, confirmedParticipants(ids), names, nil)
	require.Len(t, got, 2)
	assert.Equal(t, 1, got[0].GP, "games vs non-participants are ignored")
	assert.Equal(t, 1, got[0].Wins)
}

func TestSortTeamRecordsDrawsTiebreak(t *testing.T) {
	records := []TeamRecord{
		{TeamID: uuid.New(), TeamName: "B", Wins: 1, Draws: 0, Net: 5},
		{TeamID: uuid.New(), TeamName: "A", Wins: 1, Draws: 2, Net: 1},
	}
	SortTeamRecords(records, TiebreakOrder{TiebreakerWins, TiebreakerDraws})
	assert.Equal(t, "A", records[0].TeamName, "draws is a usable ordered tiebreak")
}

func TestSortTeamRecordsShared(t *testing.T) {
	records := []TeamRecord{
		{TeamID: uuid.New(), TeamName: "B", Wins: 1, Net: 0, PF: 5},
		{TeamID: uuid.New(), TeamName: "A", Wins: 1, Net: 0, PF: 5},
		{TeamID: uuid.New(), TeamName: "C", Wins: 2, Net: -10, PF: 0},
	}

	t.Run("default net-first", func(t *testing.T) {
		sorted := append([]TeamRecord(nil), records...)
		SortTeamRecords(sorted, nil)
		assert.Equal(t, "A", sorted[0].TeamName)
		assert.Equal(t, "B", sorted[1].TeamName)
		assert.Equal(t, "C", sorted[2].TeamName)
	})

	t.Run("wins-first", func(t *testing.T) {
		sorted := append([]TeamRecord(nil), records...)
		SortTeamRecords(sorted, WinsFirstTiebreakOrder())
		assert.Equal(t, "C", sorted[0].TeamName)
		assert.Equal(t, "A", sorted[1].TeamName)
		assert.Equal(t, "B", sorted[2].TeamName)
	})
}
