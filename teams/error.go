package teams

import "fmt"

type ErrorReason string

const (
	REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL ErrorReason = "FAILED_TO_TRANSLATE_TO_DB_MODEL"
	REASON_FAILED_TO_WRITE                 ErrorReason = "FAILED_TO_WRITE"
	REASON_TEAM_DOES_NOT_EXIST             ErrorReason = "TEAM_DOES_NOT_EXIST"
	REASON_TEAM_NAME_TAKEN                 ErrorReason = "TEAM_NAME_TAKEN"
	REASON_FAILED_TO_FETCH                 ErrorReason = "FAILED_TO_FETCH"
	REASON_INVALID_TEAM                    ErrorReason = "INVALID_TEAM"
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

func newTeamError(reason ErrorReason, message string, cause error) *Error {
	return &Error{
		Reason:  reason,
		Message: message,
		Cause:   cause,
	}
}

func NewFailedToWriteError(message string, cause error) *Error {
	return newTeamError(REASON_FAILED_TO_WRITE, message, cause)
}

func NewFailedToTranslateToDBModelError(message string, cause error) *Error {
	return newTeamError(REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL, message, cause)
}

func NewTeamDoesNotExistError(message string, cause error) *Error {
	return newTeamError(REASON_TEAM_DOES_NOT_EXIST, message, cause)
}

func NewTeamNameTakenError(message string, cause error) *Error {
	return newTeamError(REASON_TEAM_NAME_TAKEN, message, cause)
}

func NewFailedToFetchError(message string, cause error) *Error {
	return newTeamError(REASON_FAILED_TO_FETCH, message, cause)
}

func NewInvalidTeamError(message string, cause error) *Error {
	return newTeamError(REASON_INVALID_TEAM, message, cause)
}

func NewTimeoutError(message string) *Error {
	return newTeamError(REASON_TIMEOUT, message, nil)
}
