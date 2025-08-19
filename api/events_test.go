package api

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

var noopLogger = slog.New(slog.DiscardHandler)

type mockDB struct {
	GetEventsFunc                   func(ctx context.Context, limit int32, cursor *string) (events.GetEventsResponse, error)
	CreateEventFunc                 func(ctx context.Context, event events.Event) error
	GetEventFunc                    func(ctx context.Context, id uuid.UUID) (events.Event, error)
	UpdateEventFunc                 func(ctx context.Context, event events.Event) error
	CreateRegistrationFunc          func(ctx context.Context, registration registration.Registration, event events.Event) error
	GetAllRegistrationsForEventFunc func(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (registration.GetAllRegistrationsResponse, error)
}

func (m *mockDB) GetEvents(ctx context.Context, limit int32, cursor *string) (events.GetEventsResponse, error) {
	return m.GetEventsFunc(ctx, limit, cursor)
}

func (m *mockDB) CreateEvent(ctx context.Context, event events.Event) error {
	return m.CreateEventFunc(ctx, event)
}

func (m *mockDB) GetEvent(ctx context.Context, id uuid.UUID) (events.Event, error) {
	return m.GetEventFunc(ctx, id)
}

func (m *mockDB) UpdateEvent(ctx context.Context, event events.Event) error {
	return m.UpdateEventFunc(ctx, event)
}

func (m *mockDB) CreateRegistration(ctx context.Context, reg registration.Registration, event events.Event) error {
	return m.CreateRegistrationFunc(ctx, reg, event)
}

func (m *mockDB) GetAllRegistrationsForEvent(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (registration.GetAllRegistrationsResponse, error) {
	return m.GetAllRegistrationsForEventFunc(ctx, eventID, limit, cursor)
}

func TestGetEvents(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		id := uuid.New()
		now := time.Now()
		expectedEvents := []events.Event{
			{
				ID:                    id,
				Name:                  "Test Event",
				StartTime:             now,
				EndTime:               now.Add(time.Hour),
				RegistrationCloseTime: now,
				RegistrationTypes:     []events.RegistrationType{events.BY_INDIVIDUAL},
			},
		}
		mock := &mockDB{
			GetEventsFunc: func(ctx context.Context, limit int32, cursor *string) (events.GetEventsResponse, error) {
				return events.GetEventsResponse{
					Data:        expectedEvents,
					HasNextPage: false,
				}, nil
			},
		}
		api := NewAPI(mock, noopLogger)

		req := GetEventsRequestObject{
			Params: GetEventsParams{
				Limit: ptr.Int(10),
			},
		}

		resp, err := api.GetEvents(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEvents200JSONResponse:
			assert.Equal(t, len(expectedEvents), len(r.Data))
			assert.Equal(t, &expectedEvents[0].ID, r.Data[0].Id)
			assert.Equal(t, expectedEvents[0].Name, r.Data[0].Name)
			assert.Equal(t, []RegistrationType{ByIndividual}, r.Data[0].RegistrationTypes)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestPostEvents(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		now := time.Now()
		reqBody := PostEventsJSONRequestBody{
			Name:                  "Test Event",
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
			RegistrationCloseTime: now,
			RegistrationTypes:     []RegistrationType{ByIndividual},
		}
		mock := &mockDB{
			CreateEventFunc: func(ctx context.Context, event events.Event) error {
				return nil
			},
		}
		api := NewAPI(mock, noopLogger)

		req := PostEventsRequestObject{
			Body: &reqBody,
		}

		resp, err := api.PostEvents(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEvents200JSONResponse:
			assert.NotNil(t, r.Id)
			assert.Equal(t, reqBody.Name, r.Name)
			assert.Equal(t, reqBody.RegistrationTypes, r.RegistrationTypes)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestGetEventsId(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		id := uuid.New()
		now := time.Now()
		expectedEvent := events.Event{
			ID:                    id,
			Name:                  "Test Event",
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
			RegistrationCloseTime: now,
			RegistrationTypes:     []events.RegistrationType{events.BY_INDIVIDUAL},
		}
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, eventId uuid.UUID) (events.Event, error) {
				assert.Equal(t, id, eventId)
				return expectedEvent, nil
			},
		}
		api := NewAPI(mock, noopLogger)

		req := GetEventsIdRequestObject{
			Id: id,
		}

		resp, err := api.GetEventsId(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsId200JSONResponse:
			assert.Equal(t, &expectedEvent.ID, r.Event.Id)
			assert.Equal(t, expectedEvent.Name, r.Event.Name)
			assert.Equal(t, []RegistrationType{ByIndividual}, r.Event.RegistrationTypes)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("not found", func(t *testing.T) {
		id := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, eventId uuid.UUID) (events.Event, error) {
				return events.Event{}, &events.Error{Reason: events.REASON_EVENT_DOES_NOT_EXIST}
			},
		}
		api := NewAPI(mock, noopLogger)

		req := GetEventsIdRequestObject{
			Id: id,
		}

		resp, err := api.GetEventsId(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsId404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("internal server error", func(t *testing.T) {
		id := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, eventId uuid.UUID) (events.Event, error) {
				return events.Event{}, errors.New("some error")
			},
		}
		api := NewAPI(mock, noopLogger)

		req := GetEventsIdRequestObject{
			Id: id,
		}

		resp, err := api.GetEventsId(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsId500JSONResponse:
			assert.Equal(t, InternalError, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}
