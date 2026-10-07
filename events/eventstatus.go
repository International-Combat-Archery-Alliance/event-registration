package events

// EventStatus is the lifecycle state of an event (RFC-0002 §3).
//
// OPENED is the default for new events. IN_PROGRESS means the event is
// running (schedules can be generated). FINALIZED means results are locked
// (game edits must go through unfinalize → edit → finalize).
//
// Registration close is solely time-based via registrationCloseTime (D8).
type EventStatus string

const (
	EventStatusOpened     EventStatus = "OPENED"
	EventStatusInProgress EventStatus = "IN_PROGRESS"
	EventStatusFinalized  EventStatus = "FINALIZED"
)

// Valid reports whether s is a known status. Empty means "unspecified"
// (old clients / pre-status rows) and is not valid on its own.
func (s EventStatus) Valid() bool {
	switch s {
	case EventStatusOpened, EventStatusInProgress, EventStatusFinalized:
		return true
	default:
		return false
	}
}

// NormalizeDefault returns s, or OPENED when s is empty (server-side default
// for creates and pre-status rows).
func (s EventStatus) NormalizeDefault() EventStatus {
	if s == "" {
		return EventStatusOpened
	}
	return s
}

// AllowedTransition reports whether moving from existing to next is permitted
// via PATCH /events/v1/{id} (RFC-0002 §7).
//
// Forward-only: OPENED → IN_PROGRESS → FINALIZED (OPENED → FINALIZED is
// allowed for backfill/migration). Same → same is a no-op. Leaving FINALIZED
// via PATCH is rejected — score corrections go through the audited
// unfinalize → edit → finalize path (later PR).
func AllowedTransition(existing, next EventStatus) bool {
	existing = existing.NormalizeDefault()
	next = next.NormalizeDefault()
	if !existing.Valid() || !next.Valid() {
		return false
	}
	if existing == next {
		return true
	}
	switch existing {
	case EventStatusOpened:
		return next == EventStatusInProgress || next == EventStatusFinalized
	case EventStatusInProgress:
		return next == EventStatusFinalized
	case EventStatusFinalized:
		return false
	default:
		return false
	}
}
