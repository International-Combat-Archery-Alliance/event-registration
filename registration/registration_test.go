package registration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/payments"
	"github.com/Rhymond/go-money"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

type mockEventRepository struct {
	events.Repository
	GetEventFunc func(ctx context.Context, id uuid.UUID) (events.Event, error)
}

func (m *mockEventRepository) GetEvent(ctx context.Context, id uuid.UUID) (events.Event, error) {
	return m.GetEventFunc(ctx, id)
}

var _ Repository = &mockRegistrationRepository{}

type mockRegistrationRepository struct {
	CreateRegistrationFunc            func(ctx context.Context, registration Registration, event events.Event) error
	GetAllRegistrationsForEventFunc   func(ctx context.Context, eventId uuid.UUID, limit int32, cursor *string) (GetAllRegistrationsResponse, error)
	CreateRegistrationWithPaymentFunc func(ctx context.Context, registration Registration, intent RegistrationIntent, event events.Event) error
	GetRegistrationFunc               func(ctx context.Context, eventId uuid.UUID, email string) (Registration, error)
	UpdateRegistrationToPaidFunc      func(ctx context.Context, registration Registration) error
	DeleteExpiredRegistrationFunc     func(ctx context.Context, registration Registration, intent RegistrationIntent, event events.Event) error
	GetRegistrationIntentFunc         func(ctx context.Context, eventId uuid.UUID, email string) (RegistrationIntent, error)
}

func (m *mockRegistrationRepository) DeleteExpiredRegistration(ctx context.Context, registration Registration, intent RegistrationIntent, event events.Event) error {
	return m.DeleteExpiredRegistrationFunc(ctx, registration, intent, event)
}

func (m *mockRegistrationRepository) GetRegistrationIntent(ctx context.Context, eventId uuid.UUID, email string) (RegistrationIntent, error) {
	return m.GetRegistrationIntentFunc(ctx, eventId, email)
}

func (m *mockRegistrationRepository) CreateRegistration(ctx context.Context, registration Registration, event events.Event) error {
	return m.CreateRegistrationFunc(ctx, registration, event)
}

func (m *mockRegistrationRepository) GetAllRegistrationsForEvent(ctx context.Context, eventId uuid.UUID, limit int32, cursor *string) (GetAllRegistrationsResponse, error) {
	return m.GetAllRegistrationsForEventFunc(ctx, eventId, limit, cursor)
}

func (m *mockRegistrationRepository) CreateRegistrationWithPayment(ctx context.Context, registration Registration, intent RegistrationIntent, event events.Event) error {
	if m.CreateRegistrationWithPaymentFunc != nil {
		return m.CreateRegistrationWithPaymentFunc(ctx, registration, intent, event)
	}
	return nil
}

func (m *mockRegistrationRepository) GetRegistration(ctx context.Context, eventId uuid.UUID, email string) (Registration, error) {
	if m.GetRegistrationFunc != nil {
		return m.GetRegistrationFunc(ctx, eventId, email)
	}
	return nil, nil
}

func (m *mockRegistrationRepository) UpdateRegistrationToPaid(ctx context.Context, registration Registration) error {
	if m.UpdateRegistrationToPaidFunc != nil {
		return m.UpdateRegistrationToPaidFunc(ctx, registration)
	}
	return nil
}

func TestAttemptRegistration(t *testing.T) {
	t.Run("event does not exist", func(t *testing.T) {
		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{}, &events.Error{Reason: events.REASON_EVENT_DOES_NOT_EXIST}
			},
		}
		registrationRepo := &mockRegistrationRepository{}
		registrationRequest := &IndividualRegistration{
			EventID: uuid.New(),
		}

		_, _, err := AttemptRegistration(context.Background(), registrationRequest, eventRepo, registrationRepo)
		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_ASSOCIATED_EVENT_DOES_NOT_EXIST, registrationErr.Reason)
	})

	t.Run("failed to fetch event", func(t *testing.T) {
		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{}, errors.New("some error")
			},
		}
		registrationRepo := &mockRegistrationRepository{}
		registrationRequest := &IndividualRegistration{
			EventID: uuid.New(),
		}

		_, _, err := AttemptRegistration(context.Background(), registrationRequest, eventRepo, registrationRepo)
		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_FAILED_TO_FETCH, registrationErr.Reason)
	})

	t.Run("individual registration success", func(t *testing.T) {
		eventID := uuid.New()
		event := events.Event{
			ID:                  eventID,
			Version:             1,
			RegistrationOptions: []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(5000, "USD")}},
		}
		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return event, nil
			},
		}
		registrationRepo := &mockRegistrationRepository{
			CreateRegistrationFunc: func(ctx context.Context, registration Registration, evt events.Event) error {
				assert.Equal(t, event.Version+1, evt.Version)
				return nil
			},
		}
		registrationRequest := &IndividualRegistration{
			EventID: eventID,
		}

		_, _, err := AttemptRegistration(context.Background(), registrationRequest, eventRepo, registrationRepo)
		assert.NoError(t, err)
	})

	t.Run("team registration success", func(t *testing.T) {
		eventID := uuid.New()
		event := events.Event{
			ID:                   eventID,
			Version:              1,
			RegistrationOptions:  []events.EventRegistrationOption{{RegType: events.BY_TEAM, Price: money.New(5000, "USD")}},
			AllowedTeamSizeRange: events.Range{Min: 1, Max: 5},
		}
		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return event, nil
			},
		}
		registrationRepo := &mockRegistrationRepository{
			CreateRegistrationFunc: func(ctx context.Context, registration Registration, evt events.Event) error {
				assert.Equal(t, event.Version+1, evt.Version)
				return nil
			},
		}
		registrationRequest := &TeamRegistration{
			EventID: eventID,
			Players: []PlayerInfo{{}},
		}

		_, _, err := AttemptRegistration(context.Background(), registrationRequest, eventRepo, registrationRepo)
		assert.NoError(t, err)
	})

	t.Run("individual registration not allowed", func(t *testing.T) {
		eventID := uuid.New()
		event := events.Event{
			ID:                  eventID,
			RegistrationOptions: []events.EventRegistrationOption{{RegType: events.BY_TEAM}},
		}
		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return event, nil
			},
		}
		registrationRepo := &mockRegistrationRepository{}
		registrationRequest := &IndividualRegistration{
			EventID: eventID,
		}

		_, _, err := AttemptRegistration(context.Background(), registrationRequest, eventRepo, registrationRepo)
		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_NOT_ALLOWED_TO_SIGN_UP_AS_TYPE, registrationErr.Reason)
	})

	t.Run("team registration not allowed", func(t *testing.T) {
		eventID := uuid.New()
		event := events.Event{
			ID:                  eventID,
			RegistrationOptions: []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL}},
		}
		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return event, nil
			},
		}
		registrationRepo := &mockRegistrationRepository{}
		registrationRequest := &TeamRegistration{
			EventID: eventID,
		}

		_, _, err := AttemptRegistration(context.Background(), registrationRequest, eventRepo, registrationRepo)
		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_NOT_ALLOWED_TO_SIGN_UP_AS_TYPE, registrationErr.Reason)
	})

	t.Run("team size not allowed", func(t *testing.T) {
		eventID := uuid.New()
		event := events.Event{
			ID:                   eventID,
			RegistrationOptions:  []events.EventRegistrationOption{{RegType: events.BY_TEAM}},
			AllowedTeamSizeRange: events.Range{Min: 2, Max: 5},
		}
		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return event, nil
			},
		}
		registrationRepo := &mockRegistrationRepository{}
		registrationRequest := &TeamRegistration{
			EventID: eventID,
			Players: []PlayerInfo{{}},
		}

		_, _, err := AttemptRegistration(context.Background(), registrationRequest, eventRepo, registrationRepo)
		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_TEAM_SIZE_NOT_ALLOWED, registrationErr.Reason)
	})

	t.Run("unknown registration type", func(t *testing.T) {
		eventID := uuid.New()
		event := events.Event{
			ID: eventID,
		}
		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return event, nil
			},
		}
		registrationRepo := &mockRegistrationRepository{}
		registrationRequest := &mockRegistration{
			GetEventIDFunc: func() uuid.UUID {
				return eventID
			},
			GetEmailFunc: func() string {
				return "test@example.com"
			},
			TypeFunc: func() events.RegistrationType {
				return 99
			},
		}

		_, _, err := AttemptRegistration(context.Background(), registrationRequest, eventRepo, registrationRepo)
		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_UNKNOWN_REGISTRATION_TYPE, registrationErr.Reason)
	})
}

type mockRegistration struct {
	GetEventIDFunc  func() uuid.UUID
	GetEmailFunc    func() string
	TypeFunc        func() events.RegistrationType
	SetToPaidFunc   func()
	BumpVersionFunc func()
}

func (m *mockRegistration) GetEventID() uuid.UUID {
	return m.GetEventIDFunc()
}

func (m *mockRegistration) GetEmail() string {
	return m.GetEmailFunc()
}

func (m *mockRegistration) Type() events.RegistrationType {
	return m.TypeFunc()
}

func (m *mockRegistration) SetToPaid() {
	if m.SetToPaidFunc != nil {
		m.SetToPaidFunc()
	}
}

func (m *mockRegistration) BumpVersion() {
	if m.BumpVersionFunc != nil {
		m.BumpVersionFunc()
	}
}

func TestRegisterIndividualAsFreeAgent(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		event := &events.Event{
			RegistrationOptions: []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL}},
		}
		reg := &IndividualRegistration{}

		err := registerIndividualAsFreeAgent(event, reg)
		assert.NoError(t, err)
		assert.Equal(t, 1, event.NumTotalPlayers)
	})

	t.Run("not allowed", func(t *testing.T) {
		event := &events.Event{
			RegistrationOptions: []events.EventRegistrationOption{{RegType: events.BY_TEAM}},
		}
		reg := &IndividualRegistration{}

		err := registerIndividualAsFreeAgent(event, reg)
		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_NOT_ALLOWED_TO_SIGN_UP_AS_TYPE, registrationErr.Reason)
	})

	t.Run("registration closed", func(t *testing.T) {
		event := &events.Event{
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL}},
			RegistrationCloseTime: time.Now().Add(-time.Hour),
		}
		reg := &IndividualRegistration{
			RegisteredAt: time.Now(),
		}

		err := registerIndividualAsFreeAgent(event, reg)
		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_REGISTRATION_IS_CLOSED, registrationErr.Reason)
	})
}

func TestRegisterTeam(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		event := &events.Event{
			RegistrationOptions:  []events.EventRegistrationOption{{RegType: events.BY_TEAM}},
			AllowedTeamSizeRange: events.Range{Min: 1, Max: 5},
		}
		reg := &TeamRegistration{
			Players: []PlayerInfo{{}},
		}

		err := registerTeam(event, reg)
		assert.NoError(t, err)
		assert.Equal(t, 1, event.NumTeams)
		assert.Equal(t, 1, event.NumTotalPlayers)
		assert.Equal(t, 1, event.NumRosteredPlayers)
	})

	t.Run("not allowed", func(t *testing.T) {
		event := &events.Event{
			RegistrationOptions: []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL}},
		}
		reg := &TeamRegistration{}

		err := registerTeam(event, reg)
		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_NOT_ALLOWED_TO_SIGN_UP_AS_TYPE, registrationErr.Reason)
	})

	t.Run("team size too small", func(t *testing.T) {
		event := &events.Event{
			RegistrationOptions:  []events.EventRegistrationOption{{RegType: events.BY_TEAM}},
			AllowedTeamSizeRange: events.Range{Min: 2, Max: 5},
		}
		reg := &TeamRegistration{
			Players: []PlayerInfo{{}},
		}

		err := registerTeam(event, reg)
		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_TEAM_SIZE_NOT_ALLOWED, registrationErr.Reason)
	})

	t.Run("team size too large", func(t *testing.T) {
		event := &events.Event{
			RegistrationOptions:  []events.EventRegistrationOption{{RegType: events.BY_TEAM}},
			AllowedTeamSizeRange: events.Range{Min: 1, Max: 1},
		}
		reg := &TeamRegistration{
			Players: []PlayerInfo{{}, {}},
		}

		err := registerTeam(event, reg)
		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_TEAM_SIZE_NOT_ALLOWED, registrationErr.Reason)
	})

	t.Run("registration closed", func(t *testing.T) {
		event := &events.Event{
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_TEAM}},
			AllowedTeamSizeRange:  events.Range{Min: 1, Max: 5},
			RegistrationCloseTime: time.Now().Add(-time.Hour),
		}
		reg := &TeamRegistration{
			RegisteredAt: time.Now(),
			Players:      []PlayerInfo{{}},
		}

		err := registerTeam(event, reg)
		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_REGISTRATION_IS_CLOSED, registrationErr.Reason)
	})
}

type mockCheckoutManager struct {
	CreateCheckoutFunc  func(ctx context.Context, params payments.CheckoutParams) (payments.CheckoutInfo, error)
	ConfirmCheckoutFunc func(ctx context.Context, payload []byte, signature string) (map[string]string, error)
}

func (m *mockCheckoutManager) CreateCheckout(ctx context.Context, params payments.CheckoutParams) (payments.CheckoutInfo, error) {
	if m.CreateCheckoutFunc != nil {
		return m.CreateCheckoutFunc(ctx, params)
	}
	return payments.CheckoutInfo{
		SessionId:    "test_session_id",
		ClientSecret: "test_client_secret",
	}, nil
}

func (m *mockCheckoutManager) ConfirmCheckout(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
	if m.ConfirmCheckoutFunc != nil {
		return m.ConfirmCheckoutFunc(ctx, payload, signature)
	}
	return map[string]string{
		"EMAIL":    "test@example.com",
		"EVENT_ID": "123e4567-e89b-12d3-a456-426614174000",
	}, nil
}

func TestRegisterWithPayment(t *testing.T) {
	t.Run("successful individual registration with payment", func(t *testing.T) {
		eventID := uuid.New()
		event := events.Event{
			ID:      eventID,
			Name:    "Test Event",
			Version: 1,
			RegistrationOptions: []events.EventRegistrationOption{{
				RegType: events.BY_INDIVIDUAL,
				Price:   money.New(5000, "USD"),
			}},
		}
		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return event, nil
			},
		}
		registrationRepo := &mockRegistrationRepository{
			CreateRegistrationWithPaymentFunc: func(ctx context.Context, registration Registration, intent RegistrationIntent, evt events.Event) error {
				assert.Equal(t, event.Version+1, evt.Version)
				assert.Equal(t, event.ID, intent.EventId)
				assert.Equal(t, "test_session_id", intent.PaymentSessionId)
				return nil
			},
		}
		checkoutManager := &mockCheckoutManager{}
		registrationRequest := &IndividualRegistration{
			EventID: eventID,
			Email:   "test@example.com",
		}

		before := time.Now()
		reg, regIntent, clientSecret, evt, err := RegisterWithPayment(context.Background(), registrationRequest, eventRepo, registrationRepo, checkoutManager, "https://return.url")
		after := time.Now()

		assert.NoError(t, err)
		assert.Equal(t, registrationRequest, reg)
		assert.Equal(t, "test_client_secret", clientSecret)
		assert.Equal(t, event.Version+1, evt.Version)

		// Verify RegistrationIntent fields
		assert.Equal(t, eventID, regIntent.EventId)
		assert.Equal(t, "test_session_id", regIntent.PaymentSessionId)
		assert.Equal(t, "test@example.com", regIntent.Email)
		assert.Equal(t, 1, regIntent.Version)

		// Verify ExpiresAt is set to 30 minutes from now
		expectedExpiration := before.Add(30 * time.Minute)
		actualExpiration := regIntent.ExpiresAt
		assert.True(t, actualExpiration.After(expectedExpiration.Add(-1*time.Second)), "ExpiresAt should be approximately 30 minutes from now")
		assert.True(t, actualExpiration.Before(after.Add(30*time.Minute).Add(1*time.Second)), "ExpiresAt should be approximately 30 minutes from now")
	})

	t.Run("successful team registration with payment", func(t *testing.T) {
		eventID := uuid.New()
		event := events.Event{
			ID:      eventID,
			Name:    "Test Team Event",
			Version: 2,
			RegistrationOptions: []events.EventRegistrationOption{{
				RegType: events.BY_TEAM,
				Price:   money.New(15000, "USD"),
			}},
			AllowedTeamSizeRange: events.Range{Min: 1, Max: 5},
		}
		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return event, nil
			},
		}
		registrationRepo := &mockRegistrationRepository{
			CreateRegistrationWithPaymentFunc: func(ctx context.Context, registration Registration, intent RegistrationIntent, evt events.Event) error {
				assert.Equal(t, event.ID, intent.EventId)
				assert.Equal(t, event.Version+1, evt.Version)
				return nil
			},
		}
		checkoutManager := &mockCheckoutManager{}
		registrationRequest := &TeamRegistration{
			EventID:      eventID,
			CaptainEmail: "captain@example.com",
			Players:      []PlayerInfo{{}},
		}

		before := time.Now()
		reg, regIntent, clientSecret, evt, err := RegisterWithPayment(context.Background(), registrationRequest, eventRepo, registrationRepo, checkoutManager, "https://return.url")
		after := time.Now()

		assert.NoError(t, err)
		assert.Equal(t, registrationRequest, reg)
		assert.Equal(t, "test_client_secret", clientSecret)
		assert.Equal(t, event.Version+1, evt.Version)

		// Verify RegistrationIntent fields
		assert.Equal(t, eventID, regIntent.EventId)
		assert.Equal(t, "test_session_id", regIntent.PaymentSessionId)
		assert.Equal(t, "captain@example.com", regIntent.Email)
		assert.Equal(t, 1, regIntent.Version)

		// Verify ExpiresAt is set to 30 minutes from now
		expectedExpiration := before.Add(30 * time.Minute)
		actualExpiration := regIntent.ExpiresAt
		assert.True(t, actualExpiration.After(expectedExpiration.Add(-1*time.Second)), "ExpiresAt should be approximately 30 minutes from now")
		assert.True(t, actualExpiration.Before(after.Add(30*time.Minute).Add(1*time.Second)), "ExpiresAt should be approximately 30 minutes from now")
	})

	t.Run("event does not exist", func(t *testing.T) {
		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{}, &events.Error{Reason: events.REASON_EVENT_DOES_NOT_EXIST}
			},
		}
		registrationRepo := &mockRegistrationRepository{}
		checkoutManager := &mockCheckoutManager{}
		registrationRequest := &IndividualRegistration{
			EventID: uuid.New(),
		}

		_, _, _, _, err := RegisterWithPayment(context.Background(), registrationRequest, eventRepo, registrationRepo, checkoutManager, "https://return.url")

		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_ASSOCIATED_EVENT_DOES_NOT_EXIST, registrationErr.Reason)
	})

	t.Run("checkout creation fails", func(t *testing.T) {
		eventID := uuid.New()
		event := events.Event{
			ID:      eventID,
			Version: 1,
			RegistrationOptions: []events.EventRegistrationOption{{
				RegType: events.BY_INDIVIDUAL,
				Price:   money.New(5000, "USD"),
			}},
		}
		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return event, nil
			},
		}
		registrationRepo := &mockRegistrationRepository{}
		checkoutManager := &mockCheckoutManager{
			CreateCheckoutFunc: func(ctx context.Context, params payments.CheckoutParams) (payments.CheckoutInfo, error) {
				return payments.CheckoutInfo{}, errors.New("checkout creation failed")
			},
		}
		registrationRequest := &IndividualRegistration{
			EventID: eventID,
			Email:   "test@example.com",
		}

		_, _, _, _, err := RegisterWithPayment(context.Background(), registrationRequest, eventRepo, registrationRepo, checkoutManager, "https://return.url")

		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_FAILED_TO_CREATE_CHECKOUT, registrationErr.Reason)
	})

	t.Run("unknown registration type", func(t *testing.T) {
		eventID := uuid.New()
		event := events.Event{
			ID: eventID,
		}
		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return event, nil
			},
		}
		registrationRepo := &mockRegistrationRepository{}
		checkoutManager := &mockCheckoutManager{}
		registrationRequest := &mockRegistration{
			GetEventIDFunc: func() uuid.UUID {
				return eventID
			},
			GetEmailFunc: func() string {
				return "test@example.com"
			},
			TypeFunc: func() events.RegistrationType {
				return 99
			},
		}

		_, _, _, _, err := RegisterWithPayment(context.Background(), registrationRequest, eventRepo, registrationRepo, checkoutManager, "https://return.url")

		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_UNKNOWN_REGISTRATION_TYPE, registrationErr.Reason)
	})
}

func TestConfirmRegistrationPayment(t *testing.T) {
	t.Run("successful payment confirmation", func(t *testing.T) {
		eventID := uuid.New()
		email := "test@example.com"
		reg := &IndividualRegistration{
			ID:      uuid.New(),
			EventID: eventID,
			Email:   email,
			Version: 1,
			Paid:    false,
		}

		eventRepo := &mockEventRepository{}
		registrationRepo := &mockRegistrationRepository{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (Registration, error) {
				assert.Equal(t, eventID, eventId)
				assert.Equal(t, email, regEmail)
				return reg, nil
			},
			UpdateRegistrationToPaidFunc: func(ctx context.Context, registration Registration) error {
				assert.Equal(t, 2, registration.(*IndividualRegistration).Version) // Should be bumped
				assert.True(t, registration.(*IndividualRegistration).Paid)        // Should be set to paid
				return nil
			},
		}
		checkoutManager := &mockCheckoutManager{
			ConfirmCheckoutFunc: func(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
				assert.Equal(t, []byte("test_payload"), payload)
				assert.Equal(t, "test_signature", signature)
				return map[string]string{
					"EMAIL":    email,
					"EVENT_ID": eventID.String(),
				}, nil
			},
		}

		result, err := ConfirmRegistrationPayment(context.Background(), []byte("test_payload"), "test_signature", registrationRepo, eventRepo, checkoutManager)

		assert.NoError(t, err)
		assert.Equal(t, reg, result)
		assert.Equal(t, 2, result.(*IndividualRegistration).Version)
		assert.True(t, result.(*IndividualRegistration).Paid)
	})

	t.Run("missing email in metadata", func(t *testing.T) {
		eventRepo := &mockEventRepository{}
		registrationRepo := &mockRegistrationRepository{}
		checkoutManager := &mockCheckoutManager{
			ConfirmCheckoutFunc: func(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
				return map[string]string{
					"EVENT_ID": uuid.New().String(),
				}, nil
			},
		}

		_, err := ConfirmRegistrationPayment(context.Background(), []byte("test_payload"), "test_signature", registrationRepo, eventRepo, checkoutManager)

		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_PAYMENT_MISSING_METADATA, registrationErr.Reason)
		assert.Contains(t, registrationErr.Message, "EMAIL")
	})

	t.Run("invalid event ID in metadata", func(t *testing.T) {
		eventRepo := &mockEventRepository{}
		registrationRepo := &mockRegistrationRepository{}
		checkoutManager := &mockCheckoutManager{
			ConfirmCheckoutFunc: func(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
				return map[string]string{
					"EMAIL":    "test@example.com",
					"EVENT_ID": "invalid-uuid",
				}, nil
			},
		}

		_, err := ConfirmRegistrationPayment(context.Background(), []byte("test_payload"), "test_signature", registrationRepo, eventRepo, checkoutManager)

		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_INVALID_PAYMENT_METADATA, registrationErr.Reason)
	})

	t.Run("expired checkout - individual registration", func(t *testing.T) {
		eventID := uuid.New()
		email := "expired@example.com"
		reg := &IndividualRegistration{
			ID:      uuid.New(),
			EventID: eventID,
			Email:   email,
			Version: 1,
			Paid:    false,
		}
		regIntent := RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "session_123",
			Email:            email,
			ExpiresAt:        time.Now().Add(30 * time.Minute),
		}
		event := events.Event{
			ID:                 eventID,
			Version:            1,
			NumTotalPlayers:    1,
			NumTeams:           0,
			NumRosteredPlayers: 0,
		}

		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return event, nil
			},
		}
		registrationRepo := &mockRegistrationRepository{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (Registration, error) {
				return reg, nil
			},
			GetRegistrationIntentFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (RegistrationIntent, error) {
				return regIntent, nil
			},
			DeleteExpiredRegistrationFunc: func(ctx context.Context, registration Registration, intent RegistrationIntent, evt events.Event) error {
				assert.Equal(t, reg, registration)
				assert.Equal(t, regIntent, intent)
				assert.Equal(t, event.Version+1, evt.Version)
				assert.Equal(t, 0, evt.NumTotalPlayers) // Should be decremented
				return nil
			},
		}
		checkoutManager := &mockCheckoutManager{
			ConfirmCheckoutFunc: func(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
				return map[string]string{
					"EMAIL":    email,
					"EVENT_ID": eventID.String(),
				}, &payments.Error{Reason: payments.ErrorReasonCheckoutExpired}
			},
		}

		result, err := ConfirmRegistrationPayment(context.Background(), []byte("test_payload"), "test_signature", registrationRepo, eventRepo, checkoutManager)

		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_REGISTRATION_EXPIRED, registrationErr.Reason)
		assert.Equal(t, reg, result)
	})

	t.Run("expired checkout - team registration", func(t *testing.T) {
		eventID := uuid.New()
		email := "team@example.com"
		reg := &TeamRegistration{
			ID:           uuid.New(),
			EventID:      eventID,
			CaptainEmail: email,
			Version:      1,
			Paid:         false,
			Players:      []PlayerInfo{{}, {}, {}}, // 3 players
		}
		regIntent := RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "session_456",
			Email:            email,
			ExpiresAt:        time.Now().Add(30 * time.Minute),
		}
		event := events.Event{
			ID:                 eventID,
			Version:            2,
			NumTotalPlayers:    3,
			NumTeams:           1,
			NumRosteredPlayers: 3,
		}

		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return event, nil
			},
		}
		registrationRepo := &mockRegistrationRepository{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (Registration, error) {
				return reg, nil
			},
			GetRegistrationIntentFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (RegistrationIntent, error) {
				return regIntent, nil
			},
			DeleteExpiredRegistrationFunc: func(ctx context.Context, registration Registration, intent RegistrationIntent, evt events.Event) error {
				assert.Equal(t, reg, registration)
				assert.Equal(t, regIntent, intent)
				assert.Equal(t, event.Version+1, evt.Version)
				assert.Equal(t, 0, evt.NumTotalPlayers)    // Should be decremented by 3
				assert.Equal(t, 0, evt.NumTeams)           // Should be decremented by 1
				assert.Equal(t, 0, evt.NumRosteredPlayers) // Should be decremented by 3
				return nil
			},
		}
		checkoutManager := &mockCheckoutManager{
			ConfirmCheckoutFunc: func(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
				return map[string]string{
					"EMAIL":    email,
					"EVENT_ID": eventID.String(),
				}, &payments.Error{Reason: payments.ErrorReasonCheckoutExpired}
			},
		}

		result, err := ConfirmRegistrationPayment(context.Background(), []byte("test_payload"), "test_signature", registrationRepo, eventRepo, checkoutManager)

		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_REGISTRATION_EXPIRED, registrationErr.Reason)
		assert.Equal(t, reg, result)
	})

	t.Run("expired checkout - registration already deleted", func(t *testing.T) {
		eventID := uuid.New()
		email := "deleted@example.com"

		eventRepo := &mockEventRepository{}
		registrationRepo := &mockRegistrationRepository{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (Registration, error) {
				return nil, &Error{Reason: REASON_REGISTRATION_DOES_NOT_EXIST}
			},
			GetRegistrationIntentFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (RegistrationIntent, error) {
				return RegistrationIntent{}, &Error{Reason: REASON_REGISTRATION_DOES_NOT_EXIST}
			},
		}
		checkoutManager := &mockCheckoutManager{
			ConfirmCheckoutFunc: func(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
				return map[string]string{
					"EMAIL":    email,
					"EVENT_ID": eventID.String(),
				}, &payments.Error{Reason: payments.ErrorReasonCheckoutExpired}
			},
		}

		result, err := ConfirmRegistrationPayment(context.Background(), []byte("test_payload"), "test_signature", registrationRepo, eventRepo, checkoutManager)

		assert.Error(t, err)
		var registrationErr *Error
		assert.True(t, errors.As(err, &registrationErr))
		assert.Equal(t, REASON_REGISTRATION_EXPIRED, registrationErr.Reason)
		assert.Nil(t, result) // Should return nil since registration was already deleted
	})

	t.Run("expired checkout - failed to get event", func(t *testing.T) {
		eventID := uuid.New()
		email := "event-error@example.com"
		reg := &IndividualRegistration{
			ID:      uuid.New(),
			EventID: eventID,
			Email:   email,
		}
		regIntent := RegistrationIntent{
			Version:          1,
			EventId:          eventID,
			PaymentSessionId: "session_789",
			Email:            email,
			ExpiresAt:        time.Now().Add(30 * time.Minute),
		}

		eventRepo := &mockEventRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{}, errors.New("failed to get event")
			},
		}
		registrationRepo := &mockRegistrationRepository{
			GetRegistrationFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (Registration, error) {
				return reg, nil
			},
			GetRegistrationIntentFunc: func(ctx context.Context, eventId uuid.UUID, regEmail string) (RegistrationIntent, error) {
				return regIntent, nil
			},
		}
		checkoutManager := &mockCheckoutManager{
			ConfirmCheckoutFunc: func(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
				return map[string]string{
					"EMAIL":    email,
					"EVENT_ID": eventID.String(),
				}, &payments.Error{Reason: payments.ErrorReasonCheckoutExpired}
			},
		}

		result, err := ConfirmRegistrationPayment(context.Background(), []byte("test_payload"), "test_signature", registrationRepo, eventRepo, checkoutManager)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get event")
		assert.Nil(t, result)
	})
}
