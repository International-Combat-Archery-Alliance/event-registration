package events

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestUpdateEventStatus(t *testing.T) {
	eventID := uuid.New()

	t.Run("empty status preserves existing", func(t *testing.T) {
		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				return Event{ID: eventID, Version: 1, Name: "E", Status: EventStatusInProgress}, nil
			},
			UpdateEventFunc: func(ctx context.Context, event Event) error {
				assert.Equal(t, EventStatusInProgress, event.Status)
				return nil
			},
		}

		result, err := UpdateEvent(context.Background(), repo, eventID, Event{Name: "E"})
		assert.NoError(t, err)
		assert.Equal(t, EventStatusInProgress, result.Status)
	})

	t.Run("empty existing defaults to OPENED", func(t *testing.T) {
		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				// Pre-status row: no Status attribute.
				return Event{ID: eventID, Version: 1, Name: "E"}, nil
			},
			UpdateEventFunc: func(ctx context.Context, event Event) error {
				assert.Equal(t, EventStatusOpened, event.Status)
				return nil
			},
		}

		result, err := UpdateEvent(context.Background(), repo, eventID, Event{Name: "E"})
		assert.NoError(t, err)
		assert.Equal(t, EventStatusOpened, result.Status)
	})

	t.Run("forward transitions allowed", func(t *testing.T) {
		for _, tc := range []struct {
			from EventStatus
			to   EventStatus
		}{
			{EventStatusOpened, EventStatusInProgress},
		} {
			repo := &mockRepository{
				GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
					return Event{ID: eventID, Version: 1, Name: "E", Status: tc.from}, nil
				},
				UpdateEventFunc: func(ctx context.Context, event Event) error {
					assert.Equal(t, tc.to, event.Status)
					return nil
				},
			}

			result, err := UpdateEvent(context.Background(), repo, eventID, Event{Name: "E", Status: tc.to})
			assert.NoError(t, err)
			assert.Equal(t, tc.to, result.Status)
		}
	})

	t.Run("backward transitions rejected", func(t *testing.T) {
		for _, tc := range []struct {
			from EventStatus
			to   EventStatus
		}{
			{EventStatusInProgress, EventStatusOpened},
			{EventStatusFinalized, EventStatusOpened},
			{EventStatusFinalized, EventStatusInProgress},
		} {
			repo := &mockRepository{
				GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
					return Event{ID: eventID, Version: 1, Name: "E", Status: tc.from}, nil
				},
				UpdateEventFunc: func(ctx context.Context, event Event) error {
					t.Fatalf("UpdateEvent should not be called for %s -> %s", tc.from, tc.to)
					return nil
				},
			}

			_, err := UpdateEvent(context.Background(), repo, eventID, Event{Name: "E", Status: tc.to})
			assert.Error(t, err)
			var eventErr *Error
			assert.ErrorAs(t, err, &eventErr)
			assert.Equal(t, REASON_INVALID_STATUS_TRANSITION, eventErr.Reason)
		}
	})

	t.Run("invalid status value rejected", func(t *testing.T) {
		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				return Event{ID: eventID, Version: 1, Name: "E", Status: EventStatusOpened}, nil
			},
			UpdateEventFunc: func(ctx context.Context, event Event) error {
				t.Fatal("UpdateEvent should not be called for invalid status")
				return nil
			},
		}

		_, err := UpdateEvent(context.Background(), repo, eventID, Event{Name: "E", Status: EventStatus("BOGUS")})
		assert.Error(t, err)
		var eventErr *Error
		assert.ErrorAs(t, err, &eventErr)
		assert.Equal(t, REASON_INVALID_EVENT_STATUS, eventErr.Reason)
	})

	t.Run("corrupt existing status rejected", func(t *testing.T) {
		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				return Event{ID: eventID, Version: 1, Name: "E", Status: EventStatus("BOGUS")}, nil
			},
			UpdateEventFunc: func(ctx context.Context, event Event) error {
				t.Fatal("UpdateEvent should not be called for corrupt existing status")
				return nil
			},
		}

		_, err := UpdateEvent(context.Background(), repo, eventID, Event{Name: "E"})
		assert.Error(t, err)
		var eventErr *Error
		assert.ErrorAs(t, err, &eventErr)
		assert.Equal(t, REASON_INVALID_EVENT_STATUS, eventErr.Reason)
	})
}

func TestUpdateEventFinalizeDeferred(t *testing.T) {
	eventID := uuid.New()

	for _, from := range []EventStatus{EventStatusOpened, EventStatusInProgress} {
		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				return Event{ID: eventID, Version: 1, Name: "E", Status: from}, nil
			},
			UpdateEventFunc: func(ctx context.Context, event Event) error {
				t.Fatal("UpdateEvent should not be called for deferred finalize")
				return nil
			},
		}

		_, err := UpdateEvent(context.Background(), repo, eventID, Event{Name: "E", Status: EventStatusFinalized})
		assert.Error(t, err)
		var eventErr *Error
		assert.ErrorAs(t, err, &eventErr)
		assert.Equal(t, REASON_INVALID_STATUS_TRANSITION, eventErr.Reason)
	}
}

func TestUpdateEventVersionConflict(t *testing.T) {
	eventID := uuid.New()

	repo := &mockRepository{
		GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
			return Event{ID: eventID, Version: 1, Name: "E", Status: EventStatusOpened}, nil
		},
		UpdateEventFunc: func(ctx context.Context, event Event) error {
			return &Error{Reason: REASON_EVENT_DOES_NOT_EXIST}
		},
	}

	_, err := UpdateEvent(context.Background(), repo, eventID, Event{Name: "E"})
	assert.Error(t, err)
	var eventErr *Error
	assert.ErrorAs(t, err, &eventErr)
	assert.Equal(t, REASON_VERSION_CONFLICT, eventErr.Reason)
}
