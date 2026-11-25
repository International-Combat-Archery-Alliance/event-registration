package games

import (
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
)

// Tiebreaker names one key in the ordered standings/Swiss comparator
// (RFC-0002 §5, D34).
type Tiebreaker string

const (
	TiebreakerNet   Tiebreaker = "NET"
	TiebreakerWins  Tiebreaker = "WINS"
	TiebreakerDraws Tiebreaker = "DRAWS"
	TiebreakerPF    Tiebreaker = "PF"
	TiebreakerName  Tiebreaker = "NAME"
)

// TiebreakOrder is an ordered list of tiebreak keys shared by standings and
// Swiss pairing so the two can never disagree (D34). A nil order means
// DefaultTiebreakOrder.
type TiebreakOrder []Tiebreaker

// DefaultTiebreakOrder is the D34 default: net → wins → PF → name.
func DefaultTiebreakOrder() TiebreakOrder {
	return TiebreakOrder{TiebreakerNet, TiebreakerWins, TiebreakerPF, TiebreakerName}
}

// WinsFirstTiebreakOrder is the pre-D34 order, still selectable per event.
func WinsFirstTiebreakOrder() TiebreakOrder {
	return TiebreakOrder{TiebreakerWins, TiebreakerNet, TiebreakerPF, TiebreakerName}
}

// Validate rejects keys outside the D34 enum. An empty order is valid: it
// means the default. The API rejects invalid values at the request boundary,
// so a stored order only reaches the comparator after this check.
func (o TiebreakOrder) Validate() error {
	for _, tb := range o {
		switch tb {
		case TiebreakerNet, TiebreakerWins, TiebreakerDraws, TiebreakerPF, TiebreakerName:
		default:
			return NewInvalidTiebreakerError(fmt.Sprintf("unknown tiebreaker %q", string(tb)), nil)
		}
	}
	return nil
}

// resolveTiebreakOrder returns the single comparator order for this call.
// There is exactly one order per call (not a fallback chain), so an empty
// order means the D34 default; callers validate non-empty orders with
// TiebreakOrder.Validate.
func resolveTiebreakOrder(order TiebreakOrder) TiebreakOrder {
	if len(order) == 0 {
		return DefaultTiebreakOrder()
	}
	return order
}

// TeamRecord is the sortable scoring record shared by standings and Swiss
// pairing (RFC-0002 §4/§5). Both sort paths share lessRecord so they can
// never disagree.
type TeamRecord struct {
	TeamID   uuid.UUID
	TeamName string
	Wins     int
	Draws    int
	Net      int
	PF       int
}

// SortTeamRecords orders records by the given comparator, defaulting to the
// D34 order when order is empty. Names fall back to the team ID string when
// unknown so sorting never crashes on a missing directory entry.
func SortTeamRecords(records []TeamRecord, order TiebreakOrder) {
	tb := resolveTiebreakOrder(order)
	sort.SliceStable(records, func(i, j int) bool {
		return lessRecord(records[i], records[j], tb)
	})
}

func lessRecord(a, b TeamRecord, order TiebreakOrder) bool {
	for _, tb := range order {
		switch tb {
		case TiebreakerNet:
			if a.Net != b.Net {
				return a.Net > b.Net
			}
		case TiebreakerWins:
			if a.Wins != b.Wins {
				return a.Wins > b.Wins
			}
		case TiebreakerDraws:
			if a.Draws != b.Draws {
				return a.Draws > b.Draws
			}
		case TiebreakerPF:
			if a.PF != b.PF {
				return a.PF > b.PF
			}
		case TiebreakerName:
			if displayName(a) != displayName(b) {
				return displayName(a) < displayName(b)
			}
		}
	}
	return a.TeamID.String() < b.TeamID.String()
}

func displayName(r TeamRecord) string {
	if r.TeamName != "" {
		return r.TeamName
	}
	return r.TeamID.String()
}

// StandingTeam is one participant fed to standings derivation: identity plus
// participation status, so including a non-CONFIRMED entry is a deliberate
// choice rather than an accident.
type StandingTeam struct {
	TeamID uuid.UUID
	Status teams.ParticipationStatus
}

// TeamStanding is one row of derived standings (RFC-0002 §5).
type TeamStanding struct {
	TeamRecord
	GP     int
	Losses int
	PA     int
	// QualifyingRank is the 1-based qualifying order among placed teams;
	// zero when DNS (unranked). It is display-only: the finalize-stamped
	// placement (bracket-adjusted) lands with INT-61.
	QualifyingRank int
	// DNS marks a participant with no counted games: excluded from
	// placement and from circuit points (D5).
	DNS bool
}

// ComputeStandings derives per-team standings at read time; nothing is
// stored (RFC-0002 §5). Only COMPLETED, FORFEIT, and DOUBLE_FORFEIT
// QUALIFYING games count: BYE, CANCELLED, and PLAYOFF games never do.
// COMPLETED ties are draws (GP+1, Draws+1, no W/L); a FORFEIT is won by the
// non-forfeiting side and its recorded scores are decoration (never fed to
// pf/pa); a DOUBLE_FORFEIT is a loss for both sides with no pf/pa. A game
// counts only when both sides are CONFIRMED participants. Anyone listed
// without a counted game is DNS and unranked. names supplies display names
// for the name tiebreak; order selects the single comparator (empty means
// D34 net-first; callers validate it first).
func ComputeStandings(games []Game, participants []StandingTeam, names map[string]string, order TiebreakOrder) []TeamStanding {
	tb := resolveTiebreakOrder(order)
	rows := make(map[uuid.UUID]*TeamStanding, len(participants))
	confirmed := make(map[uuid.UUID]bool, len(participants))
	for _, p := range participants {
		rows[p.TeamID] = &TeamStanding{TeamRecord: TeamRecord{TeamID: p.TeamID, TeamName: names[p.TeamID.String()]}}
		confirmed[p.TeamID] = p.Status == teams.ParticipationStatusConfirmed
	}

	for _, g := range games {
		if g.Phase != GamePhaseQualifying {
			continue
		}
		a, b := g.SideA.TeamID, g.SideB.TeamID
		if a == nil || b == nil || !confirmed[*a] || !confirmed[*b] {
			continue
		}
		rowA, rowB := rows[*a], rows[*b]

		switch g.Status {
		case GameStatusCompleted:
			if g.ScoreA == nil || g.ScoreB == nil {
				continue
			}
			scoreA, scoreB := *g.ScoreA, *g.ScoreB
			rowA.GP++
			rowB.GP++
			rowA.PF += scoreA
			rowA.PA += scoreB
			rowB.PF += scoreB
			rowB.PA += scoreA
			switch {
			case scoreA > scoreB:
				rowA.Wins++
				rowB.Losses++
			case scoreB > scoreA:
				rowB.Wins++
				rowA.Losses++
			default:
				rowA.Draws++
				rowB.Draws++
			}
		case GameStatusForfeit:
			if g.ForfeitSide == nil || !g.ForfeitSide.Valid() {
				continue
			}
			rowA.GP++
			rowB.GP++
			// Recorded forfeit scores are decoration (D2): they never
			// feed pf/pa.
			if *g.ForfeitSide == GameSlotA {
				rowA.Losses++
				rowB.Wins++
			} else {
				rowA.Wins++
				rowB.Losses++
			}
		case GameStatusDoubleForfeit:
			rowA.GP++
			rowB.GP++
			rowA.Losses++
			rowB.Losses++
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
		return lessRecord(standings[i].TeamRecord, standings[j].TeamRecord, tb)
	})
	rank := 0
	for i := range standings {
		if !standings[i].DNS {
			rank++
			standings[i].QualifyingRank = rank
		}
	}
	return standings
}
