package games

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTiebreakOrderValidate(t *testing.T) {
	t.Run("empty means default and is valid", func(t *testing.T) {
		require.NoError(t, TiebreakOrder(nil).Validate())
		require.NoError(t, TiebreakOrder{}.Validate())
	})

	t.Run("every D34 key is valid", func(t *testing.T) {
		require.NoError(t, DefaultTiebreakOrder().Validate())
		require.NoError(t, WinsFirstTiebreakOrder().Validate())
		require.NoError(t, TiebreakOrder{TiebreakerDraws}.Validate())
	})

	t.Run("unknown key is rejected", func(t *testing.T) {
		err := TiebreakOrder{TiebreakerNet, "BOGUS"}.Validate()
		require.Error(t, err)
		var gameErr *Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, REASON_INVALID_TIEBREAKER, gameErr.Reason)
	})
}

// TestComputeStandingsHonoursOrder shows the D34 per-event choice has a real
// effect: net-first ranks the big-margin one-win team first, wins-first ranks
// the two-win team first.
func TestComputeStandingsHonoursOrder(t *testing.T) {
	eventID := uuid.New()
	ids, names := standingTeams(4)
	a, b, c, d := ids[0], ids[1], ids[2], ids[3]

	results := []Game{
		completedGame(eventID, a, b, 6, 5, GamePhaseQualifying),
		completedGame(eventID, a, c, 6, 5, GamePhaseQualifying),
		completedGame(eventID, d, b, 30, 0, GamePhaseQualifying),
	}

	netFirst := ComputeStandings(results, confirmedParticipants(ids), names, nil)
	winsFirst := ComputeStandings(results, confirmedParticipants(ids), names, WinsFirstTiebreakOrder())
	defaultOrder := ComputeStandings(results, confirmedParticipants(ids), names, DefaultTiebreakOrder())

	require.Len(t, netFirst, 4)
	require.Len(t, winsFirst, 4)
	assert.Equal(t, d, netFirst[0].TeamID, "net-first: the +30 team leads")
	assert.Equal(t, a, winsFirst[0].TeamID, "wins-first: the 2-0 team leads")
	assert.Equal(t, netFirst[0].TeamID, defaultOrder[0].TeamID, "an absent order is the D34 default")
}

func TestSortTeamRecordsEmptyOrderUsesDefault(t *testing.T) {
	records := []TeamRecord{
		{TeamID: uuid.New(), TeamName: "A", Wins: 2, Net: 2},
		{TeamID: uuid.New(), TeamName: "B", Wins: 1, Net: 30},
	}
	SortTeamRecords(records, nil)
	assert.Equal(t, "B", records[0].TeamName, "net-first default")
}

// TestGenerateSwissRoundHonoursOrder proves the order is threaded into
// pairing, not just standings: net-first pairs the two 1-win teams, wins-first
// down-pairs the unbeaten leader.
func TestGenerateSwissRoundHonoursOrder(t *testing.T) {
	eventID := uuid.New()
	ids, names := swissTeams("A", "B", "C", "D", "E", "F")
	a, b, c, d, e, f := ids[0], ids[1], ids[2], ids[3], ids[4], ids[5]

	priors := []Game{
		completedGame(eventID, a, b, 6, 5, GamePhaseQualifying),
		completedGame(eventID, a, c, 6, 5, GamePhaseQualifying),
		completedGame(eventID, d, b, 30, 0, GamePhaseQualifying),
		completedGame(eventID, e, f, 10, 0, GamePhaseQualifying),
	}

	firstPairNames := func(order TiebreakOrder) [2]string {
		generated, err := GenerateSwissRound(context.Background(), eventID, ids, names, priors, 2, order)
		require.NoError(t, err)
		require.NotEmpty(t, generated)
		return [2]string{names[generated[0].SideA.TeamID.String()], names[generated[0].SideB.TeamID.String()]}
	}

	netFirst := firstPairNames(nil)
	winsFirst := firstPairNames(WinsFirstTiebreakOrder())

	assert.ElementsMatch(t, []string{"D", "E"}, netFirst[:], "net-first pairs the 1-1 teams")
	assert.ElementsMatch(t, []string{"A", "D"}, winsFirst[:], "wins-first down-pairs the 2-0 leader")
	assert.NotEqual(t, netFirst, winsFirst)
}
