package api

import (
	"context"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/Rhymond/go-money"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostEventsDefaultsStatusOpened(t *testing.T) {
	now := time.Now()
	reqBody := PostEventsV1JSONRequestBody{
		Name:                  "Status Default Event",
		StartTime:             now,
		EndTime:               now.Add(time.Hour),
		RegistrationCloseTime: now,
		RegistrationOptions:   []EventRegistrationOption{{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}}},
	}
	var captured events.Event
	mock := &mockDB{
		CreateEventFunc: func(ctx context.Context, event events.Event) error {
			captured = event
			return nil
		},
	}
	api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

	resp, err := api.PostEventsV1(ctxWithLogger(context.Background(), noopLogger), PostEventsV1RequestObject{Body: &reqBody})
	require.NoError(t, err)

	switch r := resp.(type) {
	case PostEventsV1200JSONResponse:
		require.NotNil(t, r.Status)
		assert.Equal(t, OPENED, *r.Status)
	default:
		t.Fatalf("unexpected response type: %T", resp)
	}
	assert.Equal(t, events.EventStatusOpened, captured.Status)
}

func TestPostEventsRespectsExplicitStatus(t *testing.T) {
	now := time.Now()
	inProgress := INPROGRESS
	reqBody := PostEventsV1JSONRequestBody{
		Name:                  "Explicit Status Event",
		StartTime:             now,
		EndTime:               now.Add(time.Hour),
		RegistrationCloseTime: now,
		RegistrationOptions:   []EventRegistrationOption{{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}}},
		Status:                &inProgress,
	}
	mock := &mockDB{
		CreateEventFunc: func(ctx context.Context, event events.Event) error {
			assert.Equal(t, events.EventStatusInProgress, event.Status)
			return nil
		},
	}
	api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

	resp, err := api.PostEventsV1(ctxWithLogger(context.Background(), noopLogger), PostEventsV1RequestObject{Body: &reqBody})
	require.NoError(t, err)
	switch r := resp.(type) {
	case PostEventsV1200JSONResponse:
		require.NotNil(t, r.Status)
		assert.Equal(t, INPROGRESS, *r.Status)
	default:
		t.Fatalf("unexpected response type: %T", resp)
	}
}

func TestPatchEventsStatusTransitions(t *testing.T) {
	newEventBody := func(status *EventStatus) Event {
		now := time.Now()
		return Event{
			Name:                  "E",
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
			RegistrationCloseTime: now,
			RegistrationOptions:   []EventRegistrationOption{{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}}},
			Status:                status,
			SignUpStats:           &SignUpStats{},
		}
	}

	t.Run("nil status preserves existing", func(t *testing.T) {
		eventID := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{ID: eventID, Version: 1, Name: "E", Status: events.EventStatusInProgress, TimeZone: time.UTC}, nil
			},
			UpdateEventFunc: func(ctx context.Context, event events.Event) error { return nil },
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		body := newEventBody(nil)
		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), PatchEventsV1IdRequestObject{Id: eventID, Body: &body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PatchEventsV1Id200JSONResponse:
			require.NotNil(t, r.Event.Status)
			assert.Equal(t, INPROGRESS, *r.Event.Status)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("forward transition succeeds", func(t *testing.T) {
		eventID := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{ID: eventID, Version: 1, Name: "E", Status: events.EventStatusOpened, TimeZone: time.UTC}, nil
			},
			UpdateEventFunc: func(ctx context.Context, event events.Event) error { return nil },
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		body := newEventBody(ptrStatus(INPROGRESS))
		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), PatchEventsV1IdRequestObject{Id: eventID, Body: &body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PatchEventsV1Id200JSONResponse:
			assert.Equal(t, INPROGRESS, *r.Event.Status)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("backward transition returns 400", func(t *testing.T) {
		eventID := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{ID: eventID, Version: 1, Name: "E", Status: events.EventStatusFinalized, TimeZone: time.UTC}, nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		body := newEventBody(ptrStatus(OPENED))
		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), PatchEventsV1IdRequestObject{Id: eventID, Body: &body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PatchEventsV1Id400JSONResponse:
			assert.Equal(t, InvalidBody, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("invalid status value returns 400", func(t *testing.T) {
		eventID := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{ID: eventID, Version: 1, Name: "E", Status: events.EventStatusOpened, TimeZone: time.UTC}, nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		bogus := EventStatus("BOGUS")
		body := newEventBody(&bogus)
		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), PatchEventsV1IdRequestObject{Id: eventID, Body: &body})
		require.NoError(t, err)
		switch r := resp.(type) {
		case PatchEventsV1Id400JSONResponse:
			assert.Equal(t, InvalidBody, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func ptrStatus(s EventStatus) *EventStatus {
	return &s
}

func TestEventStatusConverters(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		for _, tc := range []struct {
			domain events.EventStatus
			api    EventStatus
		}{
			{events.EventStatusOpened, OPENED},
			{events.EventStatusInProgress, INPROGRESS},
			{events.EventStatusFinalized, FINALIZED},
		} {
			got, err := eventStatusToApiStatus(tc.domain)
			require.NoError(t, err)
			assert.Equal(t, tc.api, got)

			back, err := apiStatusToEventStatus(tc.api)
			require.NoError(t, err)
			assert.Equal(t, tc.domain, back)
		}
	})

	t.Run("unknown values error", func(t *testing.T) {
		_, err := eventStatusToApiStatus(events.EventStatus("BOGUS"))
		assert.Error(t, err)
		_, err = apiStatusToEventStatus(EventStatus("BOGUS"))
		assert.Error(t, err)
	})

	t.Run("event round trip preserves status", func(t *testing.T) {
		now := time.Now()
		domain := events.Event{
			ID:                    uuid.New(),
			Version:               1,
			Name:                  "E",
			TimeZone:              time.UTC,
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
			RegistrationCloseTime: now,
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_TEAM, Price: money.New(1000, "USD")}},
			Status:                events.EventStatusInProgress,
		}
		apiEvent, err := eventToApiEvent(domain)
		require.NoError(t, err)
		require.NotNil(t, apiEvent.Status)
		assert.Equal(t, INPROGRESS, *apiEvent.Status)
	})
}

func TestPostEventsRejectsInvalidStatus(t *testing.T) {
	now := time.Now()
	bogus := EventStatus("BOGUS")
	reqBody := PostEventsV1JSONRequestBody{
		Name:                  "Bogus Status Event",
		StartTime:             now,
		EndTime:               now.Add(time.Hour),
		RegistrationCloseTime: now,
		RegistrationOptions:   []EventRegistrationOption{{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}}},
		Status:                &bogus,
	}
	mock := &mockDB{
		CreateEventFunc: func(ctx context.Context, event events.Event) error {
			t.Fatal("CreateEvent should not be called for invalid status")
			return nil
		},
	}
	api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

	resp, err := api.PostEventsV1(ctxWithLogger(context.Background(), noopLogger), PostEventsV1RequestObject{Body: &reqBody})
	require.NoError(t, err)
	switch r := resp.(type) {
	case PostEventsV1400JSONResponse:
		assert.Equal(t, InvalidBody, r.Code)
	default:
		t.Fatalf("unexpected response type: %T", resp)
	}
}
