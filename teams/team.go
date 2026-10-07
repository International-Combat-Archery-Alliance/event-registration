package teams

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// TeamStatus is the lifecycle state of a team. Only ACTIVE teams can enter
// events; ARCHIVED teams keep their history but are excluded.
type TeamStatus string

const (
	TeamStatusActive   TeamStatus = "ACTIVE"
	TeamStatusArchived TeamStatus = "ARCHIVED"
)

// Valid reports whether s is a known status.
func (s TeamStatus) Valid() bool {
	switch s {
	case TeamStatusActive, TeamStatusArchived:
		return true
	default:
		return false
	}
}

// Team is a persistent cross-event identity (RFC-0001 §3, teams-lite). The
// roster lives elsewhere: rosters stay empty until player-profiles lands,
// so there are no member rows and no roster validation in this cut.
// CaptainPlayerID is nullable in the MVP (strict fail-closed validation
// returns with RFC-0005, ADR-0002).
type Team struct {
	ID              uuid.UUID
	Version         int
	Name            string
	HomeCity        string
	Status          TeamStatus
	CaptainPlayerID *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// NormalizeTeamName folds a team name for case-insensitive uniqueness.
func NormalizeTeamName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// Validate checks the team's structural invariants. Lengths apply to the
// normalized (trimmed) values, counted in characters to match the API
// schema, so blank or whitespace-only names cannot reserve an empty key.
func (t Team) Validate() error {
	if utf8.RuneCountInString(NormalizeTeamName(t.Name)) < 3 || utf8.RuneCountInString(t.Name) > 100 {
		return NewInvalidTeamError("team name must be 3..100 characters", nil)
	}
	trimmedCity := strings.TrimSpace(t.HomeCity)
	if utf8.RuneCountInString(trimmedCity) < 3 || utf8.RuneCountInString(t.HomeCity) > 100 {
		return NewInvalidTeamError("home city must be 3..100 characters", nil)
	}
	if !t.Status.Valid() {
		return NewInvalidTeamError("invalid team status", nil)
	}
	return nil
}

// Repository is the persistence port for teams.
type Repository interface {
	CreateTeam(ctx context.Context, team Team) error
	GetTeam(ctx context.Context, id uuid.UUID) (Team, error)
}
