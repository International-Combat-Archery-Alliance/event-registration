package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPaidIndividualReg(eventID uuid.UUID, email string) *registration.IndividualRegistration {
	return &registration.IndividualRegistration{
		ID:           uuid.New(),
		Version:      2,
		EventID:      eventID,
		Email:        email,
		Paid:         true,
		HomeCity:     "test city",
		Experience:   registration.NOVICE,
		PlayerInfo:   registration.PlayerInfo{FirstName: "first", LastName: "last"},
		RegisteredAt: time.Now(),
	}
}

func TestPostEventsV1AdminRegistrationsEventIdEmailNotify(t *testing.T) {
	t.Run("paid registration is notified", func(t *testing.T) {
		eventID := uuid.New()
		emailSent := false
		subscribed := false
		mock := &mockDB{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (registration.Registration, error) {
				assert.Equal(t, "test@test.com", regEmail)
				return newPaidIndividualReg(eventID, "test@test.com"), nil
			},
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{ID: eventID, Name: "Test Event", MailingListGroupID: strPtr("group-id")}, nil
			},
		}
		emailSender := &mockEmailSender{
			SendEmailFunc: func(ctx context.Context, e email.Email) error {
				emailSent = true
				return nil
			},
		}
		subscriberManager := &mockSubscriberManager{
			AddSubscriberToGroupFunc: func(ctx context.Context, email, name, groupID string) error {
				subscribed = true
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, emailSender, subscriberManager, &mockCheckoutManagerReg{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1AdminRegistrationsEventIdEmailNotify(ctxWithLogger(context.Background(), noopLogger), PostEventsV1AdminRegistrationsEventIdEmailNotifyRequestObject{
			EventId: eventID,
			Email:   "TEST@test.com",
		})
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1AdminRegistrationsEventIdEmailNotify200JSONResponse:
			indivReg, err := r.Registration.AsIndividualRegistration()
			require.NoError(t, err)
			require.NotNil(t, indivReg.Paid)
			assert.True(t, *indivReg.Paid)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.True(t, emailSent)
		assert.True(t, subscribed)
	})

	t.Run("unpaid registration is rejected without notifying", func(t *testing.T) {
		eventID := uuid.New()
		emailSent := false
		mock := &mockDB{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (registration.Registration, error) {
				reg := newPaidIndividualReg(eventID, "test@test.com")
				reg.Paid = false
				return reg, nil
			},
		}
		emailSender := &mockEmailSender{
			SendEmailFunc: func(ctx context.Context, e email.Email) error {
				emailSent = true
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, emailSender, &mockSubscriberManager{}, &mockCheckoutManagerReg{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1AdminRegistrationsEventIdEmailNotify(ctxWithLogger(context.Background(), noopLogger), PostEventsV1AdminRegistrationsEventIdEmailNotifyRequestObject{
			EventId: eventID,
			Email:   "test@test.com",
		})
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1AdminRegistrationsEventIdEmailNotify409JSONResponse:
			assert.Equal(t, RegistrationUnpaid, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.False(t, emailSent)
	})

	t.Run("registration not found", func(t *testing.T) {
		mock := &mockDB{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (registration.Registration, error) {
				return nil, registration.NewRegistrationDoesNotExistsError("not found", nil)
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManagerReg{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1AdminRegistrationsEventIdEmailNotify(ctxWithLogger(context.Background(), noopLogger), PostEventsV1AdminRegistrationsEventIdEmailNotifyRequestObject{
			EventId: uuid.New(),
			Email:   "missing@test.com",
		})
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1AdminRegistrationsEventIdEmailNotify404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("event fetch failure returns 500", func(t *testing.T) {
		eventID := uuid.New()
		emailSent := false
		mock := &mockDB{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (registration.Registration, error) {
				return newPaidIndividualReg(eventID, "test@test.com"), nil
			},
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{}, errors.New("some error")
			},
		}
		emailSender := &mockEmailSender{
			SendEmailFunc: func(ctx context.Context, e email.Email) error {
				emailSent = true
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, emailSender, &mockSubscriberManager{}, &mockCheckoutManagerReg{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1AdminRegistrationsEventIdEmailNotify(ctxWithLogger(context.Background(), noopLogger), PostEventsV1AdminRegistrationsEventIdEmailNotifyRequestObject{
			EventId: eventID,
			Email:   "test@test.com",
		})
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1AdminRegistrationsEventIdEmailNotify500JSONResponse:
			assert.Equal(t, InternalError, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.False(t, emailSent)
	})

	t.Run("email failure returns 500 but still subscribes", func(t *testing.T) {
		eventID := uuid.New()
		subscribed := false
		mock := &mockDB{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (registration.Registration, error) {
				return newPaidIndividualReg(eventID, "test@test.com"), nil
			},
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{ID: eventID, Name: "Test Event", MailingListGroupID: strPtr("group-id")}, nil
			},
		}
		emailSender := &mockEmailSender{
			SendEmailFunc: func(ctx context.Context, e email.Email) error {
				return errors.New("smtp down")
			},
		}
		subscriberManager := &mockSubscriberManager{
			AddSubscriberToGroupFunc: func(ctx context.Context, email, name, groupID string) error {
				subscribed = true
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, emailSender, subscriberManager, &mockCheckoutManagerReg{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1AdminRegistrationsEventIdEmailNotify(ctxWithLogger(context.Background(), noopLogger), PostEventsV1AdminRegistrationsEventIdEmailNotifyRequestObject{
			EventId: eventID,
			Email:   "test@test.com",
		})
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1AdminRegistrationsEventIdEmailNotify500JSONResponse:
			assert.Equal(t, InternalError, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.True(t, subscribed)
	})
}

func strPtr(s string) *string {
	return &s
}
