package events

import "fmt"

type ErrorReason string

const (
	REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL ErrorReason = "FAILED_TO_TRANSLATE_TO_DB_MODEL"
	REASON_FAILED_TO_WRITE                 ErrorReason = "FAILED_TO_WRITE"
	REASON_EVENT_DOES_NOT_EXIST            ErrorReason = "EVENT_DOES_NOT_EXIST"
	REASON_EVENT_ALREADY_EXISTS            ErrorReason = "EVENT_ALREADY_EXISTS"
	REASON_FAILED_TO_FETCH                 ErrorReason = "FAILED_TO_FETCH"
	REASON_INVALID_CURSOR                  ErrorReason = "INVALID_CURSOR"
	REASON_TIMEOUT                         ErrorReason = "TIMEOUT"
	REASON_INVALID_EVENT_STATUS            ErrorReason = "INVALID_EVENT_STATUS"
	REASON_INVALID_STATUS_TRANSITION       ErrorReason = "INVALID_STATUS_TRANSITION"
	REASON_VERSION_CONFLICT                ErrorReason = "VERSION_CONFLICT"
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

func newEventError(reason ErrorReason, message string, cause error) *Error {
	return &Error{
		Reason:  reason,
		Message: message,
		Cause:   cause,
	}
}

func NewFailedToWriteError(message string, cause error) *Error {
	return newEventError(REASON_FAILED_TO_WRITE, message, cause)
}

func NewFailedToTranslateToDBModelError(message string, cause error) *Error {
	return newEventError(REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL, message, cause)
}

func NewEventAlreadyExistsError(message string, cause error) *Error {
	return newEventError(REASON_EVENT_ALREADY_EXISTS, message, cause)
}

func NewEventDoesNotExistsError(message string, cause error) *Error {
	return newEventError(REASON_EVENT_DOES_NOT_EXIST, message, cause)
}

func NewFailedToFetchError(message string, cause error) *Error {
	return newEventError(REASON_FAILED_TO_FETCH, message, cause)
}

func NewInvalidCursorError(message string, cause error) *Error {
	return newEventError(REASON_INVALID_CURSOR, message, cause)
}

func NewTimeoutError(message string) *Error {
	return newEventError(REASON_TIMEOUT, message, nil)
}

func NewInvalidEventStatusError(message string, cause error) *Error {
	return newEventError(REASON_INVALID_EVENT_STATUS, message, cause)
}

func NewInvalidStatusTransitionError(message string, cause error) *Error {
	return newEventError(REASON_INVALID_STATUS_TRANSITION, message, cause)
}

func NewVersionConflictError(message string, cause error) *Error {
	return newEventError(REASON_VERSION_CONFLICT, message, cause)
}
