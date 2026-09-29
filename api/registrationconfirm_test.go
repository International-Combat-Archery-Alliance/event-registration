package api

import (
	"context"
	"errors"
	"testing"

	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/google/uuid"
	"github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"time"
)

func TestPostEventsV1AdminRegistrationsEventIdEmailConfirm(t *testing.T) {
	t.Run("unpaid registration is confirmed and email sent", func(t *testing.T) {
		eventID := uuid.New()
		reg := &registration.IndividualRegistration{
			ID:           uuid.New(),
			Version:      1,
			EventID:      eventID,
			Email:        "test@test.com",
			Paid:         false,
			HomeCity:     "test city",
			Experience:   registration.NOVICE,
			PlayerInfo:   registration.PlayerInfo{FirstName: "first", LastName: "last"},
			RegisteredAt: time.Now(),
		}
		updateCalled := false
		emailSent := false
		mock := &mockDB{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (registration.Registration, error) {
				return reg, nil
			},
			UpdateRegistrationToPaidFunc: func(ctx context.Context, r registration.Registration) error {
				updateCalled = true
				return nil
			},
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{ID: eventID, Name: "Test Event"}, nil
			},
		}
		emailSender := &mockEmailSender{
			SendEmailFunc: func(ctx context.Context, e email.Email) error {
				emailSent = true
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, emailSender, &mockSubscriberManager{}, &mockCheckoutManagerReg{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1AdminRegistrationsEventIdEmailConfirm(ctxWithLogger(context.Background(), noopLogger), PostEventsV1AdminRegistrationsEventIdEmailConfirmRequestObject{
			EventId: eventID,
			Email:   "test@test.com",
		})
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1AdminRegistrationsEventIdEmailConfirm200JSONResponse:
			assert.False(t, r.AlreadyPaid)
			indivReg, err := r.Registration.AsIndividualRegistration()
			require.NoError(t, err)
			require.NotNil(t, indivReg.Paid)
			assert.True(t, *indivReg.Paid)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.True(t, updateCalled)
		assert.True(t, emailSent)
	})

	t.Run("already paid registration is idempotent without email", func(t *testing.T) {
		eventID := uuid.New()
		reg := &registration.IndividualRegistration{
			ID:           uuid.New(),
			Version:      2,
			EventID:      eventID,
			Email:        "paid@test.com",
			Paid:         true,
			HomeCity:     "test city",
			Experience:   registration.NOVICE,
			PlayerInfo:   registration.PlayerInfo{FirstName: "first", LastName: "last"},
			RegisteredAt: time.Now(),
		}
		updateCalled := false
		emailSent := false
		mock := &mockDB{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (registration.Registration, error) {
				return reg, nil
			},
			UpdateRegistrationToPaidFunc: func(ctx context.Context, r registration.Registration) error {
				updateCalled = true
				return nil
			},
		}
		emailSender := &mockEmailSender{
			SendEmailFunc: func(ctx context.Context, e email.Email) error {
				emailSent = true
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, emailSender, &mockSubscriberManager{}, &mockCheckoutManagerReg{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1AdminRegistrationsEventIdEmailConfirm(ctxWithLogger(context.Background(), noopLogger), PostEventsV1AdminRegistrationsEventIdEmailConfirmRequestObject{
			EventId: eventID,
			Email:   "paid@test.com",
		})
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1AdminRegistrationsEventIdEmailConfirm200JSONResponse:
			assert.True(t, r.AlreadyPaid)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
		assert.False(t, updateCalled)
		assert.False(t, emailSent)
	})

	t.Run("email is lowercased before lookup", func(t *testing.T) {
		eventID := uuid.New()
		var lookedUpEmail string
		mock := &mockDB{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (registration.Registration, error) {
				lookedUpEmail = regEmail
				return nil, registration.NewRegistrationDoesNotExistsError("not found", nil)
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManagerReg{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1AdminRegistrationsEventIdEmailConfirm(ctxWithLogger(context.Background(), noopLogger), PostEventsV1AdminRegistrationsEventIdEmailConfirmRequestObject{
			EventId: eventID,
			Email:   types.Email("TeSt@ExAmple.COM"),
		})
		assert.NoError(t, err)
		assert.Equal(t, "test@example.com", lookedUpEmail)

		switch r := resp.(type) {
		case PostEventsV1AdminRegistrationsEventIdEmailConfirm404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("registration not found", func(t *testing.T) {
		mock := &mockDB{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (registration.Registration, error) {
				return nil, registration.NewRegistrationDoesNotExistsError("not found", nil)
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManagerReg{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1AdminRegistrationsEventIdEmailConfirm(ctxWithLogger(context.Background(), noopLogger), PostEventsV1AdminRegistrationsEventIdEmailConfirmRequestObject{
			EventId: uuid.New(),
			Email:   "missing@test.com",
		})
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1AdminRegistrationsEventIdEmailConfirm404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("internal server error on update failure", func(t *testing.T) {
		eventID := uuid.New()
		reg := &registration.IndividualRegistration{
			ID:         uuid.New(),
			Version:    1,
			EventID:    eventID,
			Email:      "test@test.com",
			Paid:       false,
			Experience: registration.NOVICE,
		}
		mock := &mockDB{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (registration.Registration, error) {
				return reg, nil
			},
			UpdateRegistrationToPaidFunc: func(ctx context.Context, r registration.Registration) error {
				return errors.New("some error")
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManagerReg{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1AdminRegistrationsEventIdEmailConfirm(ctxWithLogger(context.Background(), noopLogger), PostEventsV1AdminRegistrationsEventIdEmailConfirmRequestObject{
			EventId: eventID,
			Email:   "test@test.com",
		})
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1AdminRegistrationsEventIdEmailConfirm500JSONResponse:
			assert.Equal(t, InternalError, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("event fetch failure still returns paid registration", func(t *testing.T) {
		eventID := uuid.New()
		reg := &registration.IndividualRegistration{
			ID:           uuid.New(),
			Version:      1,
			EventID:      eventID,
			Email:        "test@test.com",
			Paid:         false,
			HomeCity:     "test city",
			Experience:   registration.NOVICE,
			PlayerInfo:   registration.PlayerInfo{FirstName: "first", LastName: "last"},
			RegisteredAt: time.Now(),
		}
		mock := &mockDB{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (registration.Registration, error) {
				return reg, nil
			},
			UpdateRegistrationToPaidFunc: func(ctx context.Context, r registration.Registration) error {
				return nil
			},
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{}, errors.New("some error")
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManagerReg{}, func(context.Context) error { return nil })

		resp, err := api.PostEventsV1AdminRegistrationsEventIdEmailConfirm(ctxWithLogger(context.Background(), noopLogger), PostEventsV1AdminRegistrationsEventIdEmailConfirmRequestObject{
			EventId: eventID,
			Email:   "test@test.com",
		})
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1AdminRegistrationsEventIdEmailConfirm200JSONResponse:
			assert.False(t, r.AlreadyPaid)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}
