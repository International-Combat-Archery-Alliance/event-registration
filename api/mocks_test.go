package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/International-Combat-Archery-Alliance/auth"
	"github.com/International-Combat-Archery-Alliance/captcha"
	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/International-Combat-Archery-Alliance/middleware"
	"github.com/International-Combat-Archery-Alliance/payments"
	"github.com/google/uuid"
)

var noopLogger = slog.New(slog.DiscardHandler)

type mockAuthValidator struct{}

type mockAuthToken struct{}

func (m *mockAuthToken) ExpiresAt() time.Time  { return time.Now().Add(time.Hour) }
func (m *mockAuthToken) ProfilePicURL() string { return "" }
func (m *mockAuthToken) IsAdmin() bool         { return false }
func (m *mockAuthToken) UserEmail() string     { return "test@example.com" }

func (m *mockAuthValidator) Validate(ctx context.Context, token string, clientID string) (auth.AuthToken, error) {
	return &mockAuthToken{}, nil
}

type mockCaptchaValidator struct {
	ValidateFunc func(ctx context.Context, token string, remoteIP string) (captcha.ValidatedData, error)
}

type mockCaptchaValidatedData struct{}

func (m *mockCaptchaValidatedData) Hostname() string       { return "icaa.world" }
func (m *mockCaptchaValidatedData) Action() string         { return "" }
func (m *mockCaptchaValidatedData) ChallengeTS() time.Time { return time.Now() }

func (m *mockCaptchaValidator) Validate(ctx context.Context, token string, remoteIP string) (captcha.ValidatedData, error) {
	if m.ValidateFunc != nil {
		return m.ValidateFunc(ctx, token, remoteIP)
	}
	return &mockCaptchaValidatedData{}, nil
}

type mockEmailSender struct{}

func (m *mockEmailSender) SendEmail(ctx context.Context, e email.Email) error {
	return nil
}

type mockCheckoutManager struct {
	CreateCheckoutFunc  func(ctx context.Context, params payments.CheckoutParams) (payments.CheckoutInfo, error)
	ConfirmCheckoutFunc func(ctx context.Context, payload []byte, signature string) (map[string]string, error)
}

func (m *mockCheckoutManager) CreateCheckout(ctx context.Context, params payments.CheckoutParams) (payments.CheckoutInfo, error) {
	if m.CreateCheckoutFunc != nil {
		return m.CreateCheckoutFunc(ctx, params)
	}
	return payments.CheckoutInfo{}, nil
}

func (m *mockCheckoutManager) ConfirmCheckout(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
	if m.ConfirmCheckoutFunc != nil {
		return m.ConfirmCheckoutFunc(ctx, payload, signature)
	}
	return map[string]string{}, nil
}

func ctxWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return middleware.CtxWithLogger(ctx, logger)
}

var _ DB = &mockDB{}

type mockDB struct {
	GetEventsFunc                     func(ctx context.Context, limit int32, cursor *string) (events.GetEventsResponse, error)
	CreateEventFunc                   func(ctx context.Context, event events.Event) error
	GetEventFunc                      func(ctx context.Context, id uuid.UUID) (events.Event, error)
	UpdateEventFunc                   func(ctx context.Context, event events.Event) error
	CreateRegistrationFunc            func(ctx context.Context, registration registration.Registration, event events.Event) error
	GetAllRegistrationsForEventFunc   func(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (registration.GetAllRegistrationsResponse, error)
	CreateRegistrationWithPaymentFunc func(ctx context.Context, reg registration.Registration, intent registration.RegistrationIntent, event events.Event) error
	GetRegistrationFunc               func(ctx context.Context, eventId uuid.UUID, email string) (registration.Registration, error)
	UpdateRegistrationToPaidFunc      func(ctx context.Context, reg registration.Registration) error
	DeleteExpiredRegistrationFunc     func(ctx context.Context, registration registration.Registration, intent registration.RegistrationIntent, event events.Event) error
	GetRegistrationIntentFunc         func(ctx context.Context, eventId uuid.UUID, email string) (registration.RegistrationIntent, error)
}

func (m *mockDB) DeleteExpiredRegistration(ctx context.Context, registration registration.Registration, intent registration.RegistrationIntent, event events.Event) error {
	return m.DeleteExpiredRegistrationFunc(ctx, registration, intent, event)
}

func (m *mockDB) GetRegistrationIntent(ctx context.Context, eventId uuid.UUID, email string) (registration.RegistrationIntent, error) {
	return m.GetRegistrationIntentFunc(ctx, eventId, email)
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

func (m *mockDB) CreateRegistrationWithPayment(ctx context.Context, reg registration.Registration, intent registration.RegistrationIntent, event events.Event) error {
	if m.CreateRegistrationWithPaymentFunc != nil {
		return m.CreateRegistrationWithPaymentFunc(ctx, reg, intent, event)
	}
	return nil
}

func (m *mockDB) GetRegistration(ctx context.Context, eventId uuid.UUID, email string) (registration.Registration, error) {
	if m.GetRegistrationFunc != nil {
		return m.GetRegistrationFunc(ctx, eventId, email)
	}
	return nil, nil
}

func (m *mockDB) UpdateRegistrationToPaid(ctx context.Context, reg registration.Registration) error {
	if m.UpdateRegistrationToPaidFunc != nil {
		return m.UpdateRegistrationToPaidFunc(ctx, reg)
	}
	return nil
}
