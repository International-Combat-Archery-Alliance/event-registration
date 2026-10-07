package games

import (
	"sort"

	"github.com/google/uuid"
)

// TeamRecord is the sortable scoring record shared by standings and Swiss
// pairing (RFC-0002 §4/§5): score group (wins), then net, then PF, then
// name. Both sort paths share the lessRecord comparator so they can never
// disagree.
type TeamRecord struct {
	TeamID   uuid.UUID
	TeamName string
	Wins     int
	Net      int
	PF       int
}

// SortTeamRecords orders records by wins ↓, net ↓, PF ↓, name ↑. Names fall
// back to the team ID string when unknown so sorting never crashes on a
// missing directory entry.
func SortTeamRecords(records []TeamRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		return lessRecord(records[i], records[j])
	})
}

func lessRecord(a, b TeamRecord) bool {
	if a.Wins != b.Wins {
		return a.Wins > b.Wins
	}
	if a.Net != b.Net {
		return a.Net > b.Net
	}
	if a.PF != b.PF {
		return a.PF > b.PF
	}
	if displayName(a) != displayName(b) {
		return displayName(a) < displayName(b)
	}
	return a.TeamID.String() < b.TeamID.String()
}

func displayName(r TeamRecord) string {
	if r.TeamName != "" {
		return r.TeamName
	}
	return r.TeamID.String()
}

// TeamStanding is one row of derived standings (RFC-0002 §5).
type TeamStanding struct {
	TeamRecord
	GP     int
	Losses int
	PA     int
	// Rank is the 1-based qualifying order among placed teams; zero when
	// DNS (unranked).
	Rank int
	// DNS marks a participant with no counted games: excluded from
	// placement and from circuit points (D5).
	DNS bool
}

// ComputeStandings derives per-team standings at read time; nothing is
// stored (RFC-0002 §5). Only COMPLETED and FORFEIT QUALIFYING games count:
// BYE, CANCELLED, DOUBLE_FORFEIT, and PLAYOFF games never do. A FORFEIT is
// won by the non-forfeiting side with the recorded scores. teams lists the
// CONFIRMED participants; anyone in it with no counted game is DNS and
// unranked. names supplies display names for the name tiebreak.
func ComputeStandings(games []Game, teams []uuid.UUID, names map[string]string) []TeamStanding {
	rows := make(map[uuid.UUID]*TeamStanding, len(teams))
	for _, id := range teams {
		rows[id] = &TeamStanding{TeamRecord: TeamRecord{TeamID: id, TeamName: names[id.String()]}}
	}

	for _, g := range games {
		if g.Phase != GamePhaseQualifying {
			continue
		}
		var winner, loser *uuid.UUID
		winnerIsB := false
		switch g.Status {
		case GameStatusCompleted:
			if g.ScoreA == nil || g.ScoreB == nil || *g.ScoreA == *g.ScoreB {
				continue
			}
			if *g.ScoreA > *g.ScoreB {
				winner, loser = g.SideA.TeamID, g.SideB.TeamID
			} else {
				winner, loser = g.SideB.TeamID, g.SideA.TeamID
				winnerIsB = true
			}
		case GameStatusForfeit:
			if g.ForfeitSide == nil || (*g.ForfeitSide != GameSlotA && *g.ForfeitSide != GameSlotB) {
				continue
			}
			if *g.ForfeitSide == GameSlotA {
				winner, loser = g.SideB.TeamID, g.SideA.TeamID
				winnerIsB = true
			} else {
				winner, loser = g.SideA.TeamID, g.SideB.TeamID
			}
		default:
			continue
		}
		if winner == nil || loser == nil {
			continue
		}
		w, okW := rows[*winner]
		l, okL := rows[*loser]
		if !okW || !okL {
			continue
		}
		w.GP++
		l.GP++
		w.Wins++
		l.Losses++
		if g.ScoreA != nil && g.ScoreB != nil {
			pfW, paW := *g.ScoreA, *g.ScoreB
			if winnerIsB {
				pfW, paW = *g.ScoreB, *g.ScoreA
			}
			w.PF += pfW
			w.PA += paW
			l.PF += paW
			l.PA += pfW
		}
	}

	standings := make([]TeamStanding, 0, len(rows))
	for _, r := range rows {
		r.Net = r.PF - r.PA
		r.DNS = r.GP == 0
		standings = append(standings, *r)
	}

	sort.SliceStable(standings, func(i, j int) bool {
		if standings[i].DNS != standings[j].DNS {
			return !standings[i].DNS
		}
		if standings[i].DNS {
			return displayName(standings[i].TeamRecord) < displayName(standings[j].TeamRecord)
		}
		return lessRecord(standings[i].TeamRecord, standings[j].TeamRecord)
	})
	rank := 0
	for i := range standings {
		if !standings[i].DNS {
			rank++
			standings[i].Rank = rank
		}
	}
	return standings
}
