package games

import (
	"github.com/google/uuid"
)

// GenerateSwissRound pairs one Swiss round from prior results (RFC-0002
// §4): sort by score group, then net, then PF, then name (the shared
// standings order); pair top-down within groups; never rematch an
// already-played pair; when within-group pairing is impossible, down-pair
// to the next score group. An odd field grants one virtual bye to the
// lowest-score group's team with the fewest byes (recorded as a BYE row:
// no scores, no win credit, resolved).
//
// priorGames are the event's earlier QUALIFYING games: results feed the
// score groups, and every non-cancelled two-sided game blocks a rematch
// (cancelled games were never played). Fields of four or fewer must use
// round robin instead (D7). Emitted games are SCHEDULED QUALIFYING (plus
// the BYE row), version 1, with dense per-round seq numbers.
func GenerateSwissRound(eventID uuid.UUID, teamIDs []uuid.UUID, names map[string]string, priorGames []Game, round int) ([]Game, error) {
	if err := checkField(teamIDs, 100); err != nil {
		return nil, err
	}
	if len(teamIDs) <= 4 {
		return nil, NewInvalidScheduleError("fields of four or fewer must use round robin", nil)
	}
	if round < 1 || round > 99 {
		return nil, NewInvalidScheduleError("swiss round must be in 1..99", nil)
	}

	standings := ComputeStandings(qualifyingGames(priorGames), teamIDs, names)
	played := playedPairs(priorGames)
	byeCount := countByes(priorGames)

	ordered := standings

	var result []Game
	var byeRecipient *uuid.UUID
	if len(ordered)%2 == 1 {
		recipient := pickByeRecipient(ordered, byeCount)
		ordered = removeTeam(ordered, recipient)
		byeTeam := recipient
		byeRecipient = &byeTeam
	}

	seq := 1
	for len(ordered) > 0 {
		x := ordered[0].TeamID
		partner := -1
		for i := 1; i < len(ordered); i++ {
			if ordered[i].Wins == ordered[0].Wins && !played[pairKeyOf(x, ordered[i].TeamID)] {
				partner = i
				break
			}
		}
		if partner == -1 {
			for i := 1; i < len(ordered); i++ {
				if !played[pairKeyOf(x, ordered[i].TeamID)] {
					partner = i
					break
				}
			}
		}
		if partner == -1 {
			// Greedy pairing has no backtracking: a strand with only
			// rematches left errors instead of emitting an illegal
			// schedule. The admin resolves it (score corrections change
			// the order) and regenerates.
			return nil, NewInvalidScheduleError("no valid pairing without rematch", nil)
		}
		y := ordered[partner].TeamID
		result = append(result, Game{
			ID:      uuid.New(),
			EventID: eventID,
			Phase:   GamePhaseQualifying,
			Round:   round,
			Seq:     seq,
			Status:  GameStatusScheduled,
			SideA:   Side{TeamID: &x},
			SideB:   Side{TeamID: &y},
			Version: 1,
		})
		seq++
		ordered = append(ordered[:partner], ordered[partner+1:]...)
		ordered = ordered[1:]
	}

	if byeRecipient != nil {
		byeTeam := *byeRecipient
		result = append(result, Game{
			ID:      uuid.New(),
			EventID: eventID,
			Phase:   GamePhaseQualifying,
			Round:   round,
			Seq:     len(result) + 1,
			Status:  GameStatusBye,
			SideA:   Side{TeamID: &byeTeam},
			SideB:   Side{IsBye: true},
			Version: 1,
		})
	}
	return result, nil
}

func qualifyingGames(priorGames []Game) []Game {
	out := make([]Game, 0, len(priorGames))
	for _, g := range priorGames {
		if g.Phase == GamePhaseQualifying {
			out = append(out, g)
		}
	}
	return out
}

func playedPairs(priorGames []Game) map[string]bool {
	played := map[string]bool{}
	for _, g := range priorGames {
		if g.Phase != GamePhaseQualifying {
			continue
		}
		if g.Status == GameStatusCancelled {
			continue
		}
		if g.SideA.Real() && g.SideB.Real() {
			played[pairKeyOf(*g.SideA.TeamID, *g.SideB.TeamID)] = true
		}
	}
	return played
}

func pairKeyOf(a, b uuid.UUID) string {
	if a.String() < b.String() {
		return a.String() + "|" + b.String()
	}
	return b.String() + "|" + a.String()
}

func countByes(priorGames []Game) map[uuid.UUID]int {
	counts := map[uuid.UUID]int{}
	for _, g := range priorGames {
		if g.Status != GameStatusBye {
			continue
		}
		if g.SideA.Real() {
			counts[*g.SideA.TeamID]++
		}
		if g.SideB.Real() {
			counts[*g.SideB.TeamID]++
		}
	}
	return counts
}

// pickByeRecipient grants the bye to the lowest score group, fewest byes
// first, then the weakest record. ordered runs best-first, so the weakest
// matching candidate wins.
func pickByeRecipient(ordered []TeamStanding, byeCount map[uuid.UUID]int) uuid.UUID {
	minWins := ordered[len(ordered)-1].Wins
	var recipient uuid.UUID
	found := false
	for i := len(ordered) - 1; i >= 0; i-- {
		if ordered[i].Wins != minWins {
			break
		}
		if !found || byeCount[ordered[i].TeamID] < byeCount[recipient] {
			recipient = ordered[i].TeamID
			found = true
		}
	}
	return recipient
}

func removeTeam(ordered []TeamStanding, id uuid.UUID) []TeamStanding {
	out := make([]TeamStanding, 0, len(ordered))
	for _, s := range ordered {
		if s.TeamID != id {
			out = append(out, s)
		}
	}
	return out
}
