package events

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	ID                    uuid.UUID
	Name                  string
	EventLocation         Location
	StartTime             time.Time
	EndTime               time.Time
	RegistrationCloseTime time.Time
	RegistrationTypes     []RegistrationType
}

type GetEventsResponse struct {
	Data        []Event
	Cursor      *string
	HasNextPage bool
}

type Repository interface {
	GetEvent(ctx context.Context, id uuid.UUID) (Event, error)
	GetEvents(ctx context.Context, limit int32, cursor *string) (GetEventsResponse, error)
	CreateEvent(ctx context.Context, event Event) error
	UpdateEvent(ctx context.Context, event Event) error
}

type ErrorReason string

const (
	REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL ErrorReason = "FAILED_TO_TRANSLATE_TO_DB_MODEL"
	REASON_FAILED_TO_WRITE                 ErrorReason = "FAILED_TO_WRITE"
	REASON_EVENT_DOES_NOT_EXIST            ErrorReason = "EVENT_DOES_NOT_EXIST"
	REASON_EVENT_ALREADY_EXISTS            ErrorReason = "EVENT_ALREADY_EXISTS"
	REASON_FAILED_TO_FETCH                 ErrorReason = "FAILED_TO_FETCH"
	REASON_INVALID_CURSOR                  ErrorReason = "INVALID_CURSOR"
)

type EventError struct {
	Reason  ErrorReason
	Message string
	Cause   error
}

func (e *EventError) Error() string {
	return fmt.Sprintf("%s: %s. Reason: %s", e.Reason, e.Message, e.Cause)
}

func (e *EventError) Unwrap() error {
	return e.Cause
}

func newEventError(reason ErrorReason, message string, cause error) *EventError {
	return &EventError{
		Reason:  reason,
		Message: message,
		Cause:   cause,
	}
}

func NewFailedToWriteError(message string, cause error) *EventError {
	return newEventError(REASON_FAILED_TO_WRITE, message, cause)
}

func NewFailedToTranslateToDBModelError(message string, cause error) *EventError {
	return newEventError(REASON_FAILED_TO_TRANSLATE_TO_DB_MODEL, message, cause)
}

func NewEventAlreadyExistsError(message string, cause error) *EventError {
	return newEventError(REASON_EVENT_ALREADY_EXISTS, message, cause)
}

func NewEventDoesNotExistsError(message string, cause error) *EventError {
	return newEventError(REASON_EVENT_DOES_NOT_EXIST, message, cause)
}

func NewFailedToFetchError(message string, cause error) *EventError {
	return newEventError(REASON_FAILED_TO_FETCH, message, cause)
}

func NewInvalidCursorError(message string, cause error) *EventError {
	return newEventError(REASON_INVALID_CURSOR, message, cause)
}
