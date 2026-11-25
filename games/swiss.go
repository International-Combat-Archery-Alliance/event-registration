package games

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
)

// pairingBudgetPerTeamSquared sizes the backtracking search at 100*n^2
// candidate examinations. A normal field needs O(n^2) examinations to walk a
// single complete matching, so 100x covers ordinary Swiss fields (including
// the extra bye-recipient attempts of an odd field) comfortably. The budget
// only matters at unusual round counts, where rematch constraints accumulate
// until the search needs exponential backtracking: measured across result
// distributions, the first exhaustion is around round 15-20 for n=32 and
// round 17-32 for n=100, well past the 5-9 rounds a real event runs. A
// pathological hand-edited schedule stops quickly instead of exhausting the
// request's time budget.
const pairingBudgetPerTeamSquared = 100

var (
	// errNoPerfectMatching is the internal "no matching exists" result. The
	// stranded-team precheck and a completed search both report it; budget
	// exhaustion is distinct because it proves nothing about the field.
	errNoPerfectMatching = errors.New("no perfect matching without rematch")
	// errPairingBudgetExhausted reports that the search stopped early with
	// the outcome undetermined: the field may be feasible, but settling it
	// would exceed the bounded search.
	errPairingBudgetExhausted = errors.New("swiss pairing budget exhausted")
)

// GenerateSwissRound pairs one Swiss round from prior results (RFC-0002
// §4): order by the shared standings comparator (order; empty means the D34
// net-first default); pair top-down, preferring the same score group (wins)
// and down-pairing when necessary; never rematch an already-played pair.
// Pairing is a deterministic backtracking search bounded by a step budget and
// by ctx. A team with no unplayed opponent is detected by an O(n^2) precheck
// and reported as NO_VALID_PAIRING; a completed search with no matching is
// also NO_VALID_PAIRING; a search that had to stop early is
// PAIRING_BUDGET_EXCEEDED, which means "feasible but too complex at this
// round count", not a proven dead end. An odd field grants one virtual bye to
// the lowest-score group's team with the fewest byes within that group
// (recorded as a BYE row: no scores, no win credit, resolved); that rule is a
// preference, not a guarantee against a second bye, and only an unmatchable
// remainder makes the search try other teams weakest-first.
//
// priorGames are the event's earlier QUALIFYING games: results feed the
// score groups, and every non-cancelled two-sided game blocks a rematch
// (cancelled games were never played). Fields of four or fewer must use
// round robin instead (D7). Emitted games are SCHEDULED QUALIFYING (plus
// the BYE row), version 1, with dense per-round seq numbers.
func GenerateSwissRound(ctx context.Context, eventID uuid.UUID, teamIDs []uuid.UUID, names map[string]string, priorGames []Game, round int, order TiebreakOrder) ([]Game, error) {
	if err := checkField(teamIDs, 100); err != nil {
		return nil, err
	}
	if len(teamIDs) <= 4 {
		return nil, NewInvalidScheduleError("fields of four or fewer must use round robin", nil)
	}
	if round < 1 || round > 99 {
		return nil, NewInvalidScheduleError("swiss round must be in 1..99", nil)
	}
	if err := order.Validate(); err != nil {
		return nil, err
	}

	// The field is the CONFIRMED roster the handler filtered for scheduling,
	// so every scheduled team participates at full standing here.
	participants := make([]StandingTeam, len(teamIDs))
	for i, id := range teamIDs {
		participants[i] = StandingTeam{TeamID: id, Status: teams.ParticipationStatusConfirmed}
	}

	standings := ComputeStandings(qualifyingGames(priorGames), participants, names, order)
	played := playedPairs(priorGames)
	byeCount := countByes(priorGames)

	pairs, byeRecipient, err := pairSwissRound(ctx, standings, played, byeCount)
	switch {
	case err == nil:
	case errors.Is(err, errNoPerfectMatching):
		return nil, NewNoValidPairingError("no valid pairing without rematch", err)
	case errors.Is(err, errPairingBudgetExhausted):
		return nil, NewPairingBudgetExceededError("pairing search exceeded its budget; the schedule leaves too many rematch constraints. Correct scores or regenerate", err)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return nil, NewTimeoutError("swiss pairing was cancelled")
	default:
		return nil, err
	}

	result := make([]Game, 0, len(pairs)+1)
	for i, pair := range pairs {
		x, y := pair[0], pair[1]
		result = append(result, Game{
			ID:      uuid.New(),
			EventID: eventID,
			Phase:   GamePhaseQualifying,
			Round:   round,
			Seq:     i + 1,
			Status:  GameStatusScheduled,
			SideA:   Side{TeamID: &x},
			SideB:   Side{TeamID: &y},
			Version: 1,
		})
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

// pairingBudget is shared across every bye-recipient attempt of one round, so
// an odd field cannot multiply the bound by its field size.
type pairingBudget struct {
	remaining int
}

func newPairingBudget(n int) *pairingBudget {
	steps := pairingBudgetPerTeamSquared * n * n
	if steps < 5000 {
		steps = 5000
	}
	return &pairingBudget{remaining: steps}
}

// spend charges one candidate examination and periodically checks ctx.
func (b *pairingBudget) spend(ctx context.Context) error {
	if b.remaining <= 0 {
		return errPairingBudgetExhausted
	}
	b.remaining--
	if b.remaining%1024 == 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

// pairSwissRound returns a perfect matching over ordered, or a bye recipient
// plus a perfect matching over the rest when the field is odd.
func pairSwissRound(ctx context.Context, ordered []TeamStanding, played map[string]bool, byeCount map[uuid.UUID]int) ([][2]uuid.UUID, *uuid.UUID, error) {
	budget := newPairingBudget(len(ordered))
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if len(ordered)%2 == 0 {
		pairs, err := matchTeams(ctx, ordered, played, budget)
		return pairs, nil, err
	}
	for _, recipient := range byeCandidates(ordered, byeCount) {
		pairs, err := matchTeams(ctx, removeTeam(ordered, recipient), played, budget)
		switch {
		case err == nil:
			bye := recipient
			return pairs, &bye, nil
		case errors.Is(err, errNoPerfectMatching):
			continue
		default:
			return nil, nil, err
		}
	}
	return nil, nil, errNoPerfectMatching
}

// strandedTeam reports whether some team in the pool has no opponent it has
// not already played. Such a team can never be paired, so no perfect matching
// exists: the O(n^2) scan proves the dead end without entering the search.
func strandedTeam(ordered []TeamStanding, played map[string]bool) bool {
	for i := range ordered {
		hasCandidate := false
		for j := range ordered {
			if i == j {
				continue
			}
			if !played[pairKeyOf(ordered[i].TeamID, ordered[j].TeamID)] {
				hasCandidate = true
				break
			}
		}
		if !hasCandidate {
			return true
		}
	}
	return false
}

// matchTeams finds a perfect matching that never reuses a played pair.
// Candidates are tried in comparator order, same score group first, then
// down-paired; backtracking means a dead end only occurs when no matching
// exists, not merely when the greedy choice was wrong. A team with no unplayed
// opponent at the root is a provable dead end, so it is caught by a cheap
// O(n^2) precheck instead of being discovered by the search. The search is
// deterministic (no map iteration or randomness) and stops at the shared
// budget or a cancelled ctx.
func matchTeams(ctx context.Context, ordered []TeamStanding, played map[string]bool, budget *pairingBudget) ([][2]uuid.UUID, error) {
	if strandedTeam(ordered, played) {
		return nil, errNoPerfectMatching
	}

	paired := make([]bool, len(ordered))
	pairs := make([][2]uuid.UUID, 0, len(ordered)/2)

	var solve func() (bool, error)
	solve = func() (bool, error) {
		next := -1
		for i := range ordered {
			if !paired[i] {
				next = i
				break
			}
		}
		if next == -1 {
			return true, nil
		}
		x := ordered[next].TeamID
		for _, sameScoreGroup := range []bool{true, false} {
			for j := next + 1; j < len(ordered); j++ {
				if err := budget.spend(ctx); err != nil {
					return false, err
				}
				if paired[j] {
					continue
				}
				if sameScoreGroup && ordered[j].Wins != ordered[next].Wins {
					continue
				}
				y := ordered[j].TeamID
				if played[pairKeyOf(x, y)] {
					continue
				}
				paired[next], paired[j] = true, true
				pairs = append(pairs, [2]uuid.UUID{x, y})
				ok, err := solve()
				if err != nil {
					return false, err
				}
				if ok {
					return true, nil
				}
				pairs = pairs[:len(pairs)-1]
				paired[next], paired[j] = false, false
			}
		}
		return false, nil
	}

	ok, err := solve()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errNoPerfectMatching
	}
	return pairs, nil
}

// byeCandidates returns the policy recipient first — D6's lowest score group,
// fewest byes within that group — then every other team weakest-first. The
// fallback exists only for a remainder the policy choice cannot match; it does
// not second-guess the fewest-byes preference.
func byeCandidates(ordered []TeamStanding, byeCount map[uuid.UUID]int) []uuid.UUID {
	policy := pickByeRecipient(ordered, byeCount)
	candidates := make([]uuid.UUID, 0, len(ordered))
	candidates = append(candidates, policy)
	for i := len(ordered) - 1; i >= 0; i-- {
		if ordered[i].TeamID != policy {
			candidates = append(candidates, ordered[i].TeamID)
		}
	}
	return candidates
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
		if g.Phase != GamePhaseQualifying || g.Status != GameStatusBye {
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

// pickByeRecipient grants the bye to a team in the lowest score group (fewest
// wins), preferring fewer prior byes within that group, then the weakest
// record. This is D6 as implemented: the fewest-byes rule compares only teams
// in the lowest-wins group, so a higher-score team with fewer byes never
// displaces the policy recipient. The comparator order is not guaranteed to
// put every team of the same score group together (net-first can place a
// 0-win team above a 1-win team), so the minimum is computed over the whole
// slice rather than a contiguous tail. Scanning weakest-first and replacing
// only on a strict improvement keeps "weakest within the group".
func pickByeRecipient(ordered []TeamStanding, byeCount map[uuid.UUID]int) uuid.UUID {
	best := ordered[len(ordered)-1]
	for i := len(ordered) - 1; i >= 0; i-- {
		candidate := ordered[i]
		if candidate.Wins < best.Wins {
			best = candidate
			continue
		}
		if candidate.Wins == best.Wins && byeCount[candidate.TeamID] < byeCount[best.TeamID] {
			best = candidate
		}
	}
	return best.TeamID
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
