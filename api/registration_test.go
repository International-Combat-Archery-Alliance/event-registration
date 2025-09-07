package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/captcha"
	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/International-Combat-Archery-Alliance/payments"
	"github.com/Rhymond/go-money"
	"github.com/google/uuid"
	"github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCheckoutManagerReg struct{}

func (m *mockCheckoutManagerReg) CreateCheckout(ctx context.Context, params payments.CheckoutParams) (payments.CheckoutInfo, error) {
	return payments.CheckoutInfo{}, nil
}

func (m *mockCheckoutManagerReg) ConfirmCheckout(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
	return map[string]string{}, nil
}

func TestPostEventsEventIdRegister(t *testing.T) {
	t.Run("invalid captcha", func(t *testing.T) {
		mockCaptcha := &mockCaptchaValidator{
			ValidateFunc: func(ctx context.Context, token string, remoteIP string) (captcha.ValidatedData, error) {
				return nil, errors.New("invalid captcha")
			},
		}
		api := NewAPI(&mockDB{}, noopLogger, LOCAL, &mockAuthValidator{}, mockCaptcha, &mockEmailSender{}, &mockCheckoutManagerReg{})
		reg := Registration{}
		indivReg := IndividualRegistration{
			HomeCity:   "test city",
			Email:      types.Email("test@test.com"),
			PlayerInfo: PlayerInfo{FirstName: "first", LastName: "last"},
			Experience: Novice,
		}
		reg.FromIndividualRegistration(indivReg)

		req := PostEventsV1EventIdRegisterRequestObject{
			EventId: uuid.New(),
			Body:    &reg,
			Params: PostEventsV1EventIdRegisterParams{
				CfTurnstileResponse: "invalid-token",
			},
		}

		resp, err := api.PostEventsV1EventIdRegister(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1EventIdRegister400JSONResponse:
			assert.Equal(t, CaptchaInvalid, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		api := NewAPI(&mockDB{}, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManagerReg{})
		reg := Registration{}
		// Set a field that will cause the discriminator to fail
		reg.FromIndividualRegistration(IndividualRegistration{})
		reg.FromTeamRegistration(TeamRegistration{})

		req := PostEventsV1EventIdRegisterRequestObject{
			EventId: uuid.New(),
			Body:    &reg,
		}

		resp, err := api.PostEventsV1EventIdRegister(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1EventIdRegister400JSONResponse:
			assert.Equal(t, InvalidBody, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("event not found", func(t *testing.T) {
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{}, &events.Error{Reason: events.REASON_EVENT_DOES_NOT_EXIST}
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManagerReg{})
		reg := &Registration{}
		indivReg := IndividualRegistration{
			HomeCity:   "test city",
			Email:      types.Email("test@test.com"),
			PlayerInfo: PlayerInfo{FirstName: "first", LastName: "last"},
			Experience: Novice,
		}
		require.NoError(t, reg.FromIndividualRegistration(indivReg))

		req := PostEventsV1EventIdRegisterRequestObject{
			EventId: uuid.New(),
			Body:    reg,
		}

		resp, err := api.PostEventsV1EventIdRegister(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1EventIdRegister404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("registration already exists", func(t *testing.T) {
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{RegistrationOptions: []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(10000, "USD")}}, RegistrationCloseTime: time.Now().Add(time.Hour * 1000)}, nil
			},
			CreateRegistrationFunc: func(ctx context.Context, reg registration.Registration, event events.Event) error {
				return &registration.Error{Reason: registration.REASON_REGISTRATION_ALREADY_EXISTS}
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManagerReg{})
		reg := Registration{}
		indivReg := IndividualRegistration{
			HomeCity:   "test city",
			Email:      types.Email("test@test.com"),
			PlayerInfo: PlayerInfo{FirstName: "first", LastName: "last"},
			Experience: Novice,
		}
		reg.FromIndividualRegistration(indivReg)

		req := PostEventsV1EventIdRegisterRequestObject{
			EventId: uuid.New(),
			Body:    &reg,
		}

		resp, err := api.PostEventsV1EventIdRegister(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1EventIdRegister409JSONResponse:
			assert.Equal(t, AlreadyExists, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("registration is closed", func(t *testing.T) {
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{RegistrationOptions: []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(5500, "USD")}}}, nil
			},
			CreateRegistrationFunc: func(ctx context.Context, reg registration.Registration, event events.Event) error {
				return &registration.Error{Reason: registration.REASON_REGISTRATION_IS_CLOSED}
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManagerReg{})
		reg := Registration{}
		indivReg := IndividualRegistration{
			HomeCity:   "test city",
			Email:      types.Email("test@test.com"),
			PlayerInfo: PlayerInfo{FirstName: "first", LastName: "last"},
			Experience: Novice,
		}
		reg.FromIndividualRegistration(indivReg)

		req := PostEventsV1EventIdRegisterRequestObject{
			EventId: uuid.New(),
			Body:    &reg,
		}

		resp, err := api.PostEventsV1EventIdRegister(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1EventIdRegister403JSONResponse:
			assert.Equal(t, RegistrationClosed, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("internal server error on attempt registration", func(t *testing.T) {
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{}, errors.New("some error")
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManagerReg{})
		reg := Registration{}
		indivReg := IndividualRegistration{
			HomeCity:   "test city",
			Email:      types.Email("test@test.com"),
			PlayerInfo: PlayerInfo{FirstName: "first", LastName: "last"},
			Experience: Novice,
		}
		reg.FromIndividualRegistration(indivReg)

		req := PostEventsV1EventIdRegisterRequestObject{
			EventId: uuid.New(),
			Body:    &reg,
		}

		resp, err := api.PostEventsV1EventIdRegister(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1EventIdRegister500JSONResponse:
			assert.Equal(t, InternalError, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestGetEventsEventIdRegistrations(t *testing.T) {
	t.Run("internal server error", func(t *testing.T) {
		mock := &mockDB{
			GetAllRegistrationsForEventFunc: func(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (registration.GetAllRegistrationsResponse, error) {
				return registration.GetAllRegistrationsResponse{}, errors.New("some error")
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManagerReg{})
		req := GetEventsV1EventIdRegistrationsRequestObject{
			EventId: uuid.New(),
			Params: GetEventsV1EventIdRegistrationsParams{
				Limit: ptr.Int(10),
			},
		}

		resp, err := api.GetEventsV1EventIdRegistrations(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsV1EventIdRegistrations500JSONResponse:
			assert.Equal(t, InternalError, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("invalid cursor", func(t *testing.T) {
		mock := &mockDB{
			GetAllRegistrationsForEventFunc: func(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (registration.GetAllRegistrationsResponse, error) {
				return registration.GetAllRegistrationsResponse{}, &registration.Error{Reason: registration.REASON_INVALID_CURSOR}
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManagerReg{})
		req := GetEventsV1EventIdRegistrationsRequestObject{
			EventId: uuid.New(),
			Params: GetEventsV1EventIdRegistrationsParams{
				Limit: ptr.Int(10),
			},
		}

		resp, err := api.GetEventsV1EventIdRegistrations(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsV1EventIdRegistrations400JSONResponse:
			assert.Equal(t, InvalidCursor, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("failed to convert registration", func(t *testing.T) {
		mock := &mockDB{
			GetAllRegistrationsForEventFunc: func(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (registration.GetAllRegistrationsResponse, error) {
				return registration.GetAllRegistrationsResponse{
					Data: []registration.Registration{
						&mockRegistration{
							TypeFunc:     func() events.RegistrationType { return 99 },
							GetEmailFunc: func() string { return "test@example.com" },
						},
					},
				}, nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManagerReg{})
		req := GetEventsV1EventIdRegistrationsRequestObject{
			EventId: uuid.New(),
			Params: GetEventsV1EventIdRegistrationsParams{
				Limit: ptr.Int(10),
			},
		}

		resp, err := api.GetEventsV1EventIdRegistrations(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsV1EventIdRegistrations500JSONResponse:
			assert.Equal(t, InternalError, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("success", func(t *testing.T) {
		mock := &mockDB{
			GetAllRegistrationsForEventFunc: func(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (registration.GetAllRegistrationsResponse, error) {
				return registration.GetAllRegistrationsResponse{
					Data: []registration.Registration{
						&registration.IndividualRegistration{
							Email:      "test@test.com",
							Experience: registration.NOVICE,
						},
					},
				}, nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManagerReg{})
		req := GetEventsV1EventIdRegistrationsRequestObject{
			EventId: uuid.New(),
			Params: GetEventsV1EventIdRegistrationsParams{
				Limit: ptr.Int(10),
			},
		}

		resp, err := api.GetEventsV1EventIdRegistrations(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsV1EventIdRegistrations200JSONResponse:
			assert.Len(t, r.Data, 1)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
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
