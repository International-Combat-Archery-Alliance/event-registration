package games

import (
	"fmt"
	"sort"

	"github.com/google/uuid"
)

// GenerateRoundRobin builds a single round robin schedule with the circle
// method (RFC-0002 §4): N(N-1)/2 real games, N-1 rounds for even fields and
// N rounds for odd ones (one team resting per round), floor(N/2) games per
// round. Byes are virtual — resting teams simply have no game, so no
// BYE-status rows are emitted. With mirror, every pairing meets twice with
// swapped sides.
//
// Input order does not matter: teams are sorted by ID, so the same field
// always yields the same schedule. Emitted games are SCHEDULED,
// QUALIFYING, version 1.
func GenerateRoundRobin(eventID uuid.UUID, teamIDs []uuid.UUID, mirror bool) ([]Game, error) {
	// Rounds are two-digit schedule positions (domain 1..99 cap), so a
	// mirrored schedule (twice the rounds) halves the maximum field.
	maxTeams := maxRoundRobinTeams
	if mirror {
		maxTeams = maxMirroredRoundRobinTeams
	}
	if err := checkField(teamIDs, maxTeams); err != nil {
		return nil, err
	}

	ordered := sortedCopy(teamIDs)

	// Odd fields get a dummy slot; whoever meets it rests that round.
	dummy := uuid.New()
	odd := len(ordered)%2 == 1

	type pairing struct {
		round int
		seq   int
		pos   int
		a, b  uuid.UUID
		sideA uuid.UUID
		sideB uuid.UUID
	}
	emit := func(p pairing, round int, sideA, sideB uuid.UUID) Game {
		return Game{
			ID:      uuid.New(),
			EventID: eventID,
			Phase:   GamePhaseQualifying,
			Round:   round,
			Seq:     p.seq,
			Status:  GameStatusScheduled,
			SideA:   Side{TeamID: &sideA},
			SideB:   Side{TeamID: &sideB},
			Version: 1,
		}
	}
	pairings := func() []pairing {
		slots := append([]uuid.UUID{}, ordered...)
		if odd {
			slots = append(slots, dummy)
		}
		var out []pairing
		for round := 1; round <= len(slots)-1; round++ {
			seq := 1
			for i := 0; i < len(slots)/2; i++ {
				a, b := slots[i], slots[len(slots)-1-i]
				if a == dummy || b == dummy {
					continue
				}
				out = append(out, pairing{round: round, seq: seq, pos: i, a: a, b: b})
				seq++
			}
			slots = rotateCircle(slots)
		}
		return out
	}

	// Sides alternate for fairness (green/yellow balance): an initial
	// round-and-position parity assignment followed by a hill-climb that
	// flips any game whose SideA team holds two or more extra assignments
	// over its opponent. Every flip strictly reduces the squared deviation,
	// so it terminates with every team within one of an even split. The
	// mirror leg replays the same pairings with swapped sides, keeping the
	// split exact.
	first := pairings()
	aCount := map[uuid.UUID]int{}
	for i := range first {
		if (first[i].round+first[i].pos)%2 == 0 {
			first[i].sideA, first[i].sideB = first[i].a, first[i].b
			aCount[first[i].a]++
		} else {
			first[i].sideA, first[i].sideB = first[i].b, first[i].a
			aCount[first[i].b]++
		}
	}
	for {
		flipped := false
		for i := range first {
			a, b := first[i].sideA, first[i].sideB
			if aCount[a]-aCount[b] >= 2 {
				first[i].sideA, first[i].sideB = b, a
				aCount[a]--
				aCount[b]++
				flipped = true
			}
		}
		if !flipped {
			break
		}
	}
	var result []Game
	for i := range first {
		sideA, sideB := first[i].sideA, first[i].sideB
		result = append(result, emit(first[i], first[i].round, sideA, sideB))
	}
	if mirror {
		roundsPerLeg := first[len(first)-1].round
		for _, p := range first {
			result = append(result, emit(p, p.round+roundsPerLeg, p.sideB, p.sideA))
		}
	}

	return result, nil
}

const (
	maxRoundRobinTeams         = 100
	maxMirroredRoundRobinTeams = 50
)

func checkField(teamIDs []uuid.UUID, maxTeams int) error {
	if len(teamIDs) < 2 || len(teamIDs) > maxTeams {
		return NewInvalidScheduleError(fmt.Sprintf("field must have between 2 and %d teams", maxTeams), nil)
	}
	seen := make(map[uuid.UUID]struct{}, len(teamIDs))
	for _, id := range teamIDs {
		if _, ok := seen[id]; ok {
			return NewInvalidScheduleError("duplicate team in field", nil)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func sortedCopy(ids []uuid.UUID) []uuid.UUID {
	out := append([]uuid.UUID{}, ids...)
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

func rotateCircle(slots []uuid.UUID) []uuid.UUID {
	out := append([]uuid.UUID{}, slots...)
	last := out[len(out)-1]
	copy(out[2:], out[1:len(out)-1])
	out[1] = last
	return out
}
