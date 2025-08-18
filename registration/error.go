package registration

import "fmt"

type ErrorReason string

const (
	REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL ErrorReason = "FAILED_TO_TRANSLATE_TO_DB_MODEL"
	REASON_FAILED_TO_WRITE                 ErrorReason = "FAILED_TO_WRITE"
	REASON_REGISTRATION_DOES_NOT_EXIST     ErrorReason = "REGISTRATION_DOES_NOT_EXIST"
	REASON_REGISTRATION_ALREADY_EXISTS     ErrorReason = "REGISTRATION_ALREADY_EXISTS"
	REASON_FAILED_TO_FETCH                 ErrorReason = "FAILED_TO_FETCH"
	REASON_INVALID_CURSOR                  ErrorReason = "INVALID_CURSOR"
	REASON_ASSOCIATED_EVENT_DOES_NOT_EXIST ErrorReason = "ASSOCIATED_EVENT_DOES_NOT_EXIST"
	REASON_UNKNOWN_REGISTRATION_TYPE       ErrorReason = "UNKNOWN_REGISTRATION_TYPE"
	REASON_TEAM_SIZE_NOT_ALLOWED           ErrorReason = "TEAM_SIZE_NOT_ALLOWED"
)

type Error struct {
	Reason  ErrorReason
	Message string
	Cause   error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s. Cause: %s", e.Reason, e.Message, e.Cause)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func newRegistrationError(reason ErrorReason, message string, cause error) *Error {
	return &Error{
		Reason:  reason,
		Message: message,
		Cause:   cause,
	}
}

func NewFailedToWriteError(message string, cause error) *Error {
	return newRegistrationError(REASON_FAILED_TO_WRITE, message, cause)
}

func NewFailedToTranslateToDBModelError(message string, cause error) *Error {
	return newRegistrationError(REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL, message, cause)
}

func NewRegistrationAlreadyExistsError(message string, cause error) *Error {
	return newRegistrationError(REASON_REGISTRATION_ALREADY_EXISTS, message, cause)
}

func NewRegistrationDoesNotExistsError(message string, cause error) *Error {
	return newRegistrationError(REASON_REGISTRATION_DOES_NOT_EXIST, message, cause)
}

func NewFailedToFetchError(message string, cause error) *Error {
	return newRegistrationError(REASON_FAILED_TO_FETCH, message, cause)
}

func NewInvalidCursorError(message string, cause error) *Error {
	return newRegistrationError(REASON_INVALID_CURSOR, message, cause)
}

func NewAssociatedEventDoesNotExistError(message string, cause error) *Error {
	return newRegistrationError(REASON_ASSOCIATED_EVENT_DOES_NOT_EXIST, message, cause)
}

func NewUnknownRegistrationTypeError(message string) *Error {
	return newRegistrationError(REASON_UNKNOWN_REGISTRATION_TYPE, message, nil)
}

func NewTeamSizeNotAllowedError(teamSize, minSize, maxSize int) *Error {
	return newRegistrationError(REASON_TEAM_SIZE_NOT_ALLOWED, fmt.Sprintf("Team size must be within %d and %d. Size is %d", minSize, maxSize, teamSize), nil)
}
