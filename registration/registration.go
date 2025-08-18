package registration

import (
	"context"
	"fmt"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/google/uuid"
)

type Repository interface {
	GetRegistration(ctx context.Context, eventId uuid.UUID, id uuid.UUID) (Registration, error)
	CreateRegistration(ctx context.Context, registration Registration) error
}

type Registration interface {
	Type() events.RegistrationType
}

type IndividualRegistration struct {
	ID         uuid.UUID
	EventID    uuid.UUID
	HomeCity   string
	Paid       bool
	Email      string
	PlayerInfo PlayerInfo
	Experience ExperienceLevel
}

func (r IndividualRegistration) Type() events.RegistrationType {
	return events.BY_INDIVIDUAL
}

type TeamRegistration struct {
	ID           uuid.UUID
	EventID      uuid.UUID
	HomeCity     string
	Paid         bool
	TeamName     string
	CaptainEmail string
	Players      []PlayerInfo
}

func (r TeamRegistration) Type() events.RegistrationType {
	return events.BY_TEAM
}

type ErrorReason string

const (
	REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL ErrorReason = "FAILED_TO_TRANSLATE_TO_DB_MODEL"
	REASON_FAILED_TO_WRITE                 ErrorReason = "FAILED_TO_WRITE"
	REASON_REGISTRATION_DOES_NOT_EXIST     ErrorReason = "EVENT_DOES_NOT_EXIST"
	REASON_REGISTRATION_ALREADY_EXISTS     ErrorReason = "EVENT_ALREADY_EXISTS"
	REASON_FAILED_TO_FETCH                 ErrorReason = "FAILED_TO_FETCH"
	REASON_INVALID_CURSOR                  ErrorReason = "INVALID_CURSOR"
)

type RegistrationError struct {
	Reason  ErrorReason
	Message string
	Cause   error
}

func (e *RegistrationError) Error() string {
	return fmt.Sprintf("%s: %s. Reason: %s", e.Reason, e.Message, e.Cause)
}

func (e *RegistrationError) Unwrap() error {
	return e.Cause
}

func newRegistrationError(reason ErrorReason, message string, cause error) *RegistrationError {
	return &RegistrationError{
		Reason:  reason,
		Message: message,
		Cause:   cause,
	}
}

func NewFailedToWriteError(message string, cause error) *RegistrationError {
	return newRegistrationError(REASON_FAILED_TO_WRITE, message, cause)
}

func NewFailedToTranslateToDBModelError(message string, cause error) *RegistrationError {
	return newRegistrationError(REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL, message, cause)
}

func NewRegistrationAlreadyExistsError(message string, cause error) *RegistrationError {
	return newRegistrationError(REASON_REGISTRATION_ALREADY_EXISTS, message, cause)
}

func NewRegistrationDoesNotExistsError(message string, cause error) *RegistrationError {
	return newRegistrationError(REASON_REGISTRATION_DOES_NOT_EXIST, message, cause)
}

func NewFailedToFetchError(message string, cause error) *RegistrationError {
	return newRegistrationError(REASON_FAILED_TO_FETCH, message, cause)
}

func NewInvalidCursorError(message string, cause error) *RegistrationError {
	return newRegistrationError(REASON_INVALID_CURSOR, message, cause)
}
