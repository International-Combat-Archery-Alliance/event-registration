package games

import "fmt"

type ErrorReason string

const (
	REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL ErrorReason = "FAILED_TO_TRANSLATE_TO_DB_MODEL"
	REASON_FAILED_TO_WRITE                 ErrorReason = "FAILED_TO_WRITE"
	REASON_GAME_DOES_NOT_EXIST             ErrorReason = "GAME_DOES_NOT_EXIST"
	REASON_GAME_ALREADY_EXISTS             ErrorReason = "GAME_ALREADY_EXISTS"
	REASON_FAILED_TO_FETCH                 ErrorReason = "FAILED_TO_FETCH"
	REASON_INVALID_CURSOR                  ErrorReason = "INVALID_CURSOR"
	REASON_TIMEOUT                         ErrorReason = "TIMEOUT"
	REASON_CANNOT_MODIFY_COMPLETED_GAME    ErrorReason = "CANNOT_MODIFY_COMPLETED_GAME"
	REASON_CANNOT_DELETE_COMPLETED_GAME    ErrorReason = "CANNOT_DELETE_COMPLETED_GAME"
	REASON_GAME_ALREADY_HAS_RESULT         ErrorReason = "GAME_ALREADY_HAS_RESULT"
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

func NewInvalidCursorError(message string, cause error) *Error {
	return newGameError(REASON_INVALID_CURSOR, message, cause)
}

func NewTimeoutError(message string) *Error {
	return newGameError(REASON_TIMEOUT, message, nil)
}

func NewCannotModifyCompletedGameError(message string) *Error {
	return newGameError(REASON_CANNOT_MODIFY_COMPLETED_GAME, message, nil)
}

func NewCannotDeleteCompletedGameError(message string) *Error {
	return newGameError(REASON_CANNOT_DELETE_COMPLETED_GAME, message, nil)
}

func NewGameAlreadyHasResultError(message string) *Error {
	return newGameError(REASON_GAME_ALREADY_HAS_RESULT, message, nil)
}
