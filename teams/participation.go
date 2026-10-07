package teams

import (
	"time"

	"github.com/google/uuid"
)

// ParticipationStatus is the lifecycle state of a team's entry in an event
// (RFC-0001 §3).
type ParticipationStatus string

const (
	ParticipationStatusRegistered ParticipationStatus = "REGISTERED"
	ParticipationStatusConfirmed  ParticipationStatus = "CONFIRMED"
	ParticipationStatusWithdrawn  ParticipationStatus = "WITHDRAWN"
	ParticipationStatusDNS        ParticipationStatus = "DNS"
)

// Valid reports whether s is a known status.
func (s ParticipationStatus) Valid() bool {
	switch s {
	case ParticipationStatusRegistered, ParticipationStatusConfirmed,
		ParticipationStatusWithdrawn, ParticipationStatusDNS:
		return true
	default:
		return false
	}
}

// Participation is one team's entry in one event (RFC-0001 §3): at most one
// row per event, enforced by create-conditional writes (second entry is a
// 409). The MVP seeds CONFIRMED rows directly with an empty roster snapshot
// (INT-51); the Stage C payment lifecycle replaces this path.
type Participation struct {
	EventID           uuid.UUID
	TeamID            uuid.UUID
	Status            ParticipationStatus
	RosterSnapshot    []uuid.UUID
	RosterSizeAtEvent int
	Version           int
}

// Validate checks the participation's structural invariants.
func (p Participation) Validate() error {
	if !p.Status.Valid() {
		return NewInvalidTeamError("invalid participation status", nil)
	}
	if p.RosterSizeAtEvent != len(p.RosterSnapshot) {
		return NewInvalidTeamError("roster size must match snapshot length", nil)
	}
	return nil
}

// TeamHistory is the team-side adjacency row for an event (RFC-0001 §3,
// ADR-0009): finalize stamps the result here, and player history reads
// derive through it. Written in the same transaction as participation.
type TeamHistory struct {
	TeamID    uuid.UUID
	EventID   uuid.UUID
	EventName string
	EventDate time.Time
	Status    ParticipationStatus
	// Result is stamped at finalize (nil until then).
	Result  *TeamResult
	Version int
}

// TeamResult is the stamped final outcome consumed by circuits and player
// history (ADR-0009).
type TeamResult struct {
	Wins      int
	Losses    int
	PF        int
	PA        int
	Placement int
}
