package games

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// GamePhase is the tournament phase a game belongs to (RFC-0002 §3).
type GamePhase string

const (
	GamePhaseQualifying GamePhase = "QUALIFYING"
	GamePhasePlayoff    GamePhase = "PLAYOFF"
)

// Valid reports whether p is a known phase.
func (p GamePhase) Valid() bool {
	switch p {
	case GamePhaseQualifying, GamePhasePlayoff:
		return true
	default:
		return false
	}
}

// GameStatus is the lifecycle state of a game (RFC-0002 §3/§5).
type GameStatus string

const (
	GameStatusScheduled     GameStatus = "SCHEDULED"
	GameStatusCompleted     GameStatus = "COMPLETED"
	GameStatusForfeit       GameStatus = "FORFEIT"
	GameStatusDoubleForfeit GameStatus = "DOUBLE_FORFEIT"
	GameStatusCancelled     GameStatus = "CANCELLED"
	GameStatusBye           GameStatus = "BYE"
)

// Valid reports whether s is a known status.
func (s GameStatus) Valid() bool {
	switch s {
	case GameStatusScheduled, GameStatusCompleted, GameStatusForfeit,
		GameStatusDoubleForfeit, GameStatusCancelled, GameStatusBye:
		return true
	default:
		return false
	}
}

// Resolved reports whether the game counts as settled for the finalize
// precondition: everything except SCHEDULED.
func (s GameStatus) Resolved() bool {
	return s.Valid() && s != GameStatusScheduled
}

// Deletable reports whether a schedule write may remove the row. SCHEDULED,
// BYE, and CANCELLED rows carry no result (BYE and CANCELLED are not
// results), so replace and the delete endpoint may sweep them. COMPLETED,
// FORFEIT, and DOUBLE_FORFEIT are records and are never deletable.
func (s GameStatus) Deletable() bool {
	switch s {
	case GameStatusScheduled, GameStatusBye, GameStatusCancelled:
		return true
	default:
		return false
	}
}

// GameSlot identifies one side of a game.
type GameSlot string

const (
	GameSlotA GameSlot = "A"
	GameSlotB GameSlot = "B"
)

// Valid reports whether s names a side.
func (s GameSlot) Valid() bool {
	return s == GameSlotA || s == GameSlotB
}

// Side is one participant in a game. A bye side carries no team; a playoff
// side may be undetermined (no team, not a bye) until its feeder games
// resolve.
type Side struct {
	TeamID *uuid.UUID
	IsBye  bool
}

// Real reports whether the side is an actual participating team.
func (s Side) Real() bool {
	return !s.IsBye && s.TeamID != nil
}

// Game is a single scheduled contest within an event (RFC-0002 §3).
// GameNo is the per-event, admin-facing identifier: minted from the event's
// LastGameNo high-water mark at creation and never renumbered or recycled.
// ID stays the internal item identity.
type Game struct {
	ID           uuid.UUID
	EventID      uuid.UUID
	GameNo       int
	Phase        GamePhase
	Round        int
	Seq          int
	Status       GameStatus
	SideA        Side
	SideB        Side
	ScoreA       *int
	ScoreB       *int
	ForfeitSide  *GameSlot
	NextGameID   *uuid.UUID
	NextGameSlot *GameSlot
	PrevAID      *uuid.UUID
	PrevBID      *uuid.UUID
	Notes        *string
	StartTime    *time.Time
	Version      int
}

// Repository is the persistence port for games. ListGamesForEvent returns
// QUALIFYING games before PLAYOFF games: 'P' < 'Q' lexically, so the adapter
// must order phases explicitly rather than relying on key order.
//
// Concurrent schedule writes are serialized by the EVENT item's Version:
// every method that changes the schedule takes the event version it read and
// flips it inside the same transaction as the row write (or, for a generated
// batch, before the batch). A writer whose read is stale fails the condition
// instead of minting over it.
type Repository interface {
	GetGameByNumber(ctx context.Context, eventID uuid.UUID, gameNo int) (Game, error)
	ListGamesForEvent(ctx context.Context, eventID uuid.UUID) ([]Game, error)

	// BeginScheduleWrite is the epoch grant for a multi-step schedule write
	// (generate): it increments the EVENT item's ScheduleGeneration and
	// Version only when the event still carries expectedVersion and is not
	// FINALIZED. It returns the new event Version, which later steps of the
	// same write pass as their epoch. It writes no games, so a read taken
	// after a successful grant cannot be raced by another schedule writer
	// without that writer invalidating this epoch.
	BeginScheduleWrite(ctx context.Context, eventID uuid.UUID, expectedVersion int) (int, error)

	// ReserveGameNumbers advances the EVENT item's LastGameNo high-water mark
	// to lastGameNo (never backwards) and increments Version when
	// expectedVersion still matches. Mints are exact and never recycled after
	// a delete. Returns the new event Version.
	ReserveGameNumbers(ctx context.Context, eventID uuid.UUID, expectedVersion int, lastGameNo int) (int, error)

	// CreateGames writes a generated schedule in transactions of at most 100
	// actions. Each transaction carries a ConditionCheck on the EVENT item at
	// epochVersion (and not FINALIZED) and every put is conditional on
	// (attribute_not_exists(Status) OR Status = SCHEDULED): a preserved
	// result row blocks its slot instead of being overwritten. Chunks are
	// atomic individually, not as a group, so a mid-flight result or a
	// concurrent epoch change can leave earlier chunks written; the caller
	// aborts and a later replace sweeps the unplayed leftovers.
	CreateGames(ctx context.Context, eventID uuid.UUID, epochVersion int, gameList []Game) error

	// DeleteGames deletes the named unplayed games in transactions of at most
	// 100 actions. Rows are resolved from a pre-delete read; every delete is
	// conditional on that row's Version and on the row still being deletable
	// (SCHEDULED/BYE/CANCELLED), and every transaction checks the EVENT item
	// at epochVersion and not FINALIZED. A result recorded after the
	// pre-delete read bumps the row's Version, so the first chunk touching it
	// fails; that chunk is atomic, but earlier chunks may already have
	// committed. Results themselves are never deleted.
	DeleteGames(ctx context.Context, eventID uuid.UUID, epochVersion int, gameNos []int) error

	// CreateScheduledGame folds the schedule guard and the row write into ONE
	// transaction: the event update (Version CAS, FINALIZED barrier,
	// ScheduleGeneration +1, LastGameNo = game.GameNo) and the conditional
	// put happen together or not at all. The caller mints game.GameNo from
	// the high-water mark it read; a concurrent schedule writer that
	// committed first leaves that Version stale, so the whole transaction
	// fails rather than minting a duplicate. The put requires an empty slot.
	CreateScheduledGame(ctx context.Context, eventID uuid.UUID, expectedVersion int, game Game) error

	// DeleteScheduledGame bumps the schedule generation and deletes a
	// SCHEDULED/BYE/CANCELLED row in one transaction, conditional on the
	// row's Version and the event's expectedVersion. Results are refused.
	DeleteScheduledGame(ctx context.Context, eventID uuid.UUID, expectedVersion int, game Game) error

	// UpdateGameScheduleChecked bumps the schedule generation and rewrites a
	// SCHEDULED row in one transaction, conditional on the event's
	// expectedVersion and the row's stored Version. When the edit moves the
	// row's (round, seq) key, the old row is deleted in the same transaction
	// and the destination must be empty.
	UpdateGameScheduleChecked(ctx context.Context, eventID uuid.UUID, expectedVersion int, stored, updated Game) error

	// UpdateGameChecked writes a game result. It does not change the
	// schedule, so it does not bump the event Version; it carries a
	// ConditionCheck on the EVENT item so a post-finalize edit is
	// structurally impossible, plus Version and GameNo conditions on the
	// row: the GameNo makes a stale result fail against a row regenerated at
	// the same (phase, round, seq) slot, which is a different game.
	UpdateGameChecked(ctx context.Context, eventID uuid.UUID, game Game) error
}

// Validate checks the game's structural invariants and status-specific
// result rules (RFC-0002 §3/§5).
func (g Game) Validate() error {
	if !g.Phase.Valid() {
		return NewInvalidGamePhaseError(fmt.Sprintf("invalid game phase %q", string(g.Phase)), nil)
	}
	if !g.Status.Valid() {
		return NewInvalidGameStatusError(fmt.Sprintf("invalid game status %q", string(g.Status)), nil)
	}
	if g.GameNo < 1 || g.GameNo > 9999 {
		return NewInvalidGameResultError(fmt.Sprintf("game number must be in 1..9999, got %d", g.GameNo), nil)
	}
	// Round/seq are two-digit schedule positions: game keys encode them as
	// %02d, so values above 99 would break schedule order.
	if g.Round < 1 || g.Round > 99 || g.Seq < 1 || g.Seq > 99 {
		return NewInvalidGameResultError(fmt.Sprintf("round and seq must be in 1..99, got round=%d seq=%d", g.Round, g.Seq), nil)
	}
	for _, side := range []Side{g.SideA, g.SideB} {
		if side.IsBye && side.TeamID != nil {
			return NewInvalidGameResultError("bye side must not carry a team", nil)
		}
	}
	if g.ForfeitSide != nil && !g.ForfeitSide.Valid() {
		return NewInvalidGameResultError(fmt.Sprintf("invalid forfeit side %q", string(*g.ForfeitSide)), nil)
	}
	for _, score := range []*int{g.ScoreA, g.ScoreB} {
		if score != nil && *score < 0 {
			return NewInvalidGameResultError("scores must not be negative", nil)
		}
	}

	switch g.Status {
	case GameStatusScheduled:
		if g.ScoreA != nil || g.ScoreB != nil {
			return NewInvalidGameResultError("scheduled game must not carry scores", nil)
		}
		if g.ForfeitSide != nil {
			return NewInvalidGameResultError("scheduled game must not carry a forfeit side", nil)
		}
		// A bye is a resolved row: it can never be turned back into an
		// unresolved game, so a SCHEDULED game has no bye side.
		if g.SideA.IsBye || g.SideB.IsBye {
			return NewInvalidGameResultError("scheduled game must not carry a bye side", nil)
		}
		// A QUALIFYING row is a real contest, so both sides must name a
		// team. PLAYOFF rows legitimately keep undetermined (TBD) sides
		// until their feeder games resolve.
		if g.Phase == GamePhaseQualifying && (!g.SideA.Real() || !g.SideB.Real()) {
			return NewInvalidGameResultError("qualifying game requires two real sides", nil)
		}
		if g.SideA.Real() && g.SideB.Real() && *g.SideA.TeamID == *g.SideB.TeamID {
			return NewInvalidGameResultError("game sides must be distinct teams", nil)
		}
	case GameStatusCompleted:
		if err := requireContestedSides(g); err != nil {
			return err
		}
		if g.ScoreA == nil || g.ScoreB == nil {
			return NewInvalidGameResultError("completed game requires both scores", nil)
		}
		if g.ForfeitSide != nil {
			return NewInvalidGameResultError("completed game must not carry a forfeit side", nil)
		}
	case GameStatusForfeit:
		if err := requireContestedSides(g); err != nil {
			return err
		}
		if g.ForfeitSide == nil {
			return NewInvalidGameResultError("forfeit game requires a forfeit side", nil)
		}
		// Forfeit scores are historical decoration recorded by the admin, not
		// the outcome (D2): unlike COMPLETED, no winner is required.
	case GameStatusDoubleForfeit:
		if err := requireContestedSides(g); err != nil {
			return err
		}
		if g.ForfeitSide != nil {
			return NewInvalidGameResultError("double forfeit must not single out a forfeit side", nil)
		}
		// Both sides record a loss with pf/pa 0: scores are both absent or
		// both zero, never split.
		if (g.ScoreA == nil) != (g.ScoreB == nil) {
			return NewInvalidGameResultError("double forfeit scores must both be absent or both be zero", nil)
		}
		if (g.ScoreA != nil && *g.ScoreA != 0) || (g.ScoreB != nil && *g.ScoreB != 0) {
			return NewInvalidGameResultError("double forfeit scores must be 0 or absent", nil)
		}
	case GameStatusCancelled:
		if g.ForfeitSide != nil {
			return NewInvalidGameResultError("cancelled game must not carry a forfeit side", nil)
		}
		if g.ScoreA != nil || g.ScoreB != nil {
			return NewInvalidGameResultError("cancelled game must not carry scores", nil)
		}
	case GameStatusBye:
		if g.ForfeitSide != nil {
			return NewInvalidGameResultError("bye game must not carry a forfeit side", nil)
		}
		if g.ScoreA != nil || g.ScoreB != nil {
			return NewInvalidGameResultError("bye game must not carry scores", nil)
		}
		// A bye is granted to someone: exactly one bye side, one real side.
		byeCount := 0
		for _, side := range []Side{g.SideA, g.SideB} {
			if side.IsBye {
				byeCount++
			} else if !side.Real() {
				return NewInvalidGameResultError("bye game sides must be one bye and one real team", nil)
			}
		}
		if byeCount != 1 {
			return NewInvalidGameResultError("bye game sides must be one bye and one real team", nil)
		}
	}

	return nil
}

func requireContestedSides(g Game) error {
	if !g.SideA.Real() || !g.SideB.Real() {
		return NewInvalidGameResultError("result game requires two real sides", nil)
	}
	if *g.SideA.TeamID == *g.SideB.TeamID {
		return NewInvalidGameResultError("game sides must be distinct teams", nil)
	}
	return nil
}
