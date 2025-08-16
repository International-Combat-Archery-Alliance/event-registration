package api

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

var noopLogger = slog.New(slog.DiscardHandler)

type mockDB struct {
	events.EventRepository
	GetEventsFunc   func(ctx context.Context, limit int32, cursor *string) (events.GetEventsResponse, error)
	CreateEventFunc func(ctx context.Context, event events.Event) error
}

func (m *mockDB) GetEvents(ctx context.Context, limit int32, cursor *string) (events.GetEventsResponse, error) {
	return m.GetEventsFunc(ctx, limit, cursor)
}

func (m *mockDB) CreateEvent(ctx context.Context, event events.Event) error {
	return m.CreateEventFunc(ctx, event)
}

func TestGetEvents(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		id := uuid.New()
		now := time.Now()
		expectedEvents := []events.Event{
			{
				ID:                    id,
				Name:                  "Test Event",
				EventDateTime:         now,
				RegistrationCloseTime: now,
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
			Params: GetEventsParams{},
		}

		resp, err := api.GetEvents(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEvents200JSONResponse:
			assert.Equal(t, len(expectedEvents), len(r.Data))
			assert.Equal(t, &expectedEvents[0].ID, r.Data[0].Id)
			assert.Equal(t, expectedEvents[0].Name, r.Data[0].Name)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		limit := int(100)
		api := NewAPI(&mockDB{}, noopLogger)
		req := GetEventsRequestObject{
			Params: GetEventsParams{
				Limit: &limit,
			},
		}

		resp, err := api.GetEvents(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEvents400JSONResponse:
			assert.Equal(t, LimitOutOfBounds, r.Code)
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
			EventDateTime:         now,
			RegistrationCloseTime: now,
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
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}
