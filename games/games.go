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
type Game struct {
	ID           uuid.UUID
	EventID      uuid.UUID
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
type Repository interface {
	GetGame(ctx context.Context, eventID, gameID uuid.UUID) (Game, error)
	ListGamesForEvent(ctx context.Context, eventID uuid.UUID) ([]Game, error)
	CreateGames(ctx context.Context, games []Game) error
	UpdateGame(ctx context.Context, game Game) error
	DeleteGames(ctx context.Context, eventID uuid.UUID, gameIDs []uuid.UUID) error
	// UpdateGameChecked writes the game only when its event is not
	// FINALIZED, atomically: the transaction carries a ConditionCheck on
	// the EVENT item, so post-finalize edits are structurally impossible.
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
	case GameStatusCompleted:
		if err := requireContestedSides(g); err != nil {
			return err
		}
		if g.ScoreA == nil || g.ScoreB == nil {
			return NewInvalidGameResultError("completed game requires both scores", nil)
		}
		if *g.ScoreA == *g.ScoreB {
			return NewInvalidGameResultError("draws are not allowed in v1; every completed game has a winner", nil)
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
