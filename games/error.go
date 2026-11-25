package games

import "fmt"

type ErrorReason string

const (
	REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL ErrorReason = "FAILED_TO_TRANSLATE_TO_DB_MODEL"
	REASON_FAILED_TO_WRITE                 ErrorReason = "FAILED_TO_WRITE"
	REASON_GAME_DOES_NOT_EXIST             ErrorReason = "GAME_DOES_NOT_EXIST"
	REASON_GAME_ALREADY_EXISTS             ErrorReason = "GAME_ALREADY_EXISTS"
	REASON_FAILED_TO_FETCH                 ErrorReason = "FAILED_TO_FETCH"
	REASON_INVALID_GAME_PHASE              ErrorReason = "INVALID_GAME_PHASE"
	REASON_INVALID_GAME_STATUS             ErrorReason = "INVALID_GAME_STATUS"
	REASON_INVALID_GAME_RESULT             ErrorReason = "INVALID_GAME_RESULT"
	REASON_INVALID_SCHEDULE                ErrorReason = "INVALID_SCHEDULE"
	REASON_INVALID_TIEBREAKER              ErrorReason = "INVALID_TIEBREAKER"
	REASON_NO_VALID_PAIRING                ErrorReason = "NO_VALID_PAIRING"
	REASON_PAIRING_BUDGET_EXCEEDED         ErrorReason = "PAIRING_BUDGET_EXCEEDED"
	REASON_SLOT_OCCUPIED                   ErrorReason = "SLOT_OCCUPIED"
	REASON_EVENT_FINALIZED                 ErrorReason = "EVENT_FINALIZED"
	REASON_VERSION_CONFLICT                ErrorReason = "VERSION_CONFLICT"
	REASON_TIMEOUT                         ErrorReason = "TIMEOUT"
)

type Error struct {
	Reason  ErrorReason
	Message string
	Cause   error
}

func (e *Error) Error() string {
	s := fmt.Sprintf("%s: %s.", e.Reason, e.Message)
	if e.Cause != nil {
		s += fmt.Sprintf(" Cause: %s", e.Cause)
	}
	return s
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func newGameError(reason ErrorReason, message string, cause error) *Error {
	return &Error{
		Reason:  reason,
		Message: message,
		Cause:   cause,
	}
}

func NewFailedToWriteError(message string, cause error) *Error {
	return newGameError(REASON_FAILED_TO_WRITE, message, cause)
}

func NewFailedToTranslateToDBModelError(message string, cause error) *Error {
	return newGameError(REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL, message, cause)
}

func NewGameAlreadyExistsError(message string, cause error) *Error {
	return newGameError(REASON_GAME_ALREADY_EXISTS, message, cause)
}

func NewGameDoesNotExistError(message string, cause error) *Error {
	return newGameError(REASON_GAME_DOES_NOT_EXIST, message, cause)
}

func NewFailedToFetchError(message string, cause error) *Error {
	return newGameError(REASON_FAILED_TO_FETCH, message, cause)
}

func NewInvalidGamePhaseError(message string, cause error) *Error {
	return newGameError(REASON_INVALID_GAME_PHASE, message, cause)
}

func NewInvalidGameStatusError(message string, cause error) *Error {
	return newGameError(REASON_INVALID_GAME_STATUS, message, cause)
}

func NewInvalidGameResultError(message string, cause error) *Error {
	return newGameError(REASON_INVALID_GAME_RESULT, message, cause)
}

func NewTimeoutError(message string) *Error {
	return newGameError(REASON_TIMEOUT, message, nil)
}

func NewInvalidScheduleError(message string, cause error) *Error {
	return newGameError(REASON_INVALID_SCHEDULE, message, cause)
}

func NewNoValidPairingError(message string, cause error) *Error {
	return newGameError(REASON_NO_VALID_PAIRING, message, cause)
}

// NewPairingBudgetExceededError reports that the Swiss pairing search stopped
// at its step budget. It is deliberately distinct from NO_VALID_PAIRING: the
// search did not prove that no matching exists, so the admin must change the
// inputs rather than accept an impossible field.
func NewPairingBudgetExceededError(message string, cause error) *Error {
	return newGameError(REASON_PAIRING_BUDGET_EXCEEDED, message, cause)
}

func NewInvalidTiebreakerError(message string, cause error) *Error {
	return newGameError(REASON_INVALID_TIEBREAKER, message, cause)
}

// NewSlotOccupiedError reports that a game slot is held by a recorded result
// or another game that the write must not overwrite.
func NewSlotOccupiedError(message string, cause error) *Error {
	return newGameError(REASON_SLOT_OCCUPIED, message, cause)
}

func NewEventFinalizedError(message string, cause error) *Error {
	return newGameError(REASON_EVENT_FINALIZED, message, cause)
}

func NewVersionConflictError(message string, cause error) *Error {
	return newGameError(REASON_VERSION_CONFLICT, message, cause)
}
