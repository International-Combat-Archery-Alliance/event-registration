package api

import (
	"context"
	"errors"
	"testing"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/google/uuid"
	"github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostEventsEventIdRegister(t *testing.T) {
	t.Run("no body", func(t *testing.T) {
		api := NewAPI(&mockDB{}, noopLogger)
		req := PostEventsEventIdRegisterRequestObject{
			EventId: uuid.New(),
		}

		resp, err := api.PostEventsEventIdRegister(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsEventIdRegister400JSONResponse:
			assert.Equal(t, EmptyBody, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		api := NewAPI(&mockDB{}, noopLogger)
		reg := Registration{}
		// Set a field that will cause the discriminator to fail
		reg.FromIndividualRegistration(IndividualRegistration{})
		reg.FromTeamRegistration(TeamRegistration{})

		req := PostEventsEventIdRegisterRequestObject{
			EventId: uuid.New(),
			Body:    &reg,
		}

		resp, err := api.PostEventsEventIdRegister(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsEventIdRegister400JSONResponse:
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
		api := NewAPI(mock, noopLogger)
		reg := &Registration{}
		indivReg := IndividualRegistration{
			HomeCity:   "test city",
			Email:      types.Email("test@test.com"),
			PlayerInfo: PlayerInfo{FirstName: "first", LastName: "last"},
			Experience: Novice,
		}
		require.NoError(t, reg.FromIndividualRegistration(indivReg))

		req := PostEventsEventIdRegisterRequestObject{
			EventId: uuid.New(),
			Body:    reg,
		}

		resp, err := api.PostEventsEventIdRegister(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsEventIdRegister404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("registration already exists", func(t *testing.T) {
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{RegistrationTypes: []events.RegistrationType{events.BY_INDIVIDUAL}}, nil
			},
			CreateRegistrationFunc: func(ctx context.Context, reg registration.Registration, event events.Event) error {
				return &registration.Error{Reason: registration.REASON_REGISTRATION_ALREADY_EXISTS}
			},
		}
		api := NewAPI(mock, noopLogger)
		reg := Registration{}
		indivReg := IndividualRegistration{
			HomeCity:   "test city",
			Email:      types.Email("test@test.com"),
			PlayerInfo: PlayerInfo{FirstName: "first", LastName: "last"},
			Experience: Novice,
		}
		reg.FromIndividualRegistration(indivReg)

		req := PostEventsEventIdRegisterRequestObject{
			EventId: uuid.New(),
			Body:    &reg,
		}

		resp, err := api.PostEventsEventIdRegister(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsEventIdRegister409JSONResponse:
			assert.Equal(t, AlreadyExists, r.Code)
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
		api := NewAPI(mock, noopLogger)
		reg := Registration{}
		indivReg := IndividualRegistration{
			HomeCity:   "test city",
			Email:      types.Email("test@test.com"),
			PlayerInfo: PlayerInfo{FirstName: "first", LastName: "last"},
			Experience: Novice,
		}
		reg.FromIndividualRegistration(indivReg)

		req := PostEventsEventIdRegisterRequestObject{
			EventId: uuid.New(),
			Body:    &reg,
		}

		resp, err := api.PostEventsEventIdRegister(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsEventIdRegister500JSONResponse:
			assert.Equal(t, InternalError, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestGetEventsEventIdRegistrations(t *testing.T) {
	t.Run("limit out of bounds", func(t *testing.T) {
		api := NewAPI(&mockDB{}, noopLogger)
		limit := 0
		req := GetEventsEventIdRegistrationsRequestObject{
			EventId: uuid.New(),
			Params: GetEventsEventIdRegistrationsParams{
				Limit: &limit,
			},
		}

		resp, err := api.GetEventsEventIdRegistrations(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsEventIdRegistrations400JSONResponse:
			assert.Equal(t, LimitOutOfBounds, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("internal server error", func(t *testing.T) {
		mock := &mockDB{
			GetAllRegistrationsForEventFunc: func(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (registration.GetAllRegistrationsResponse, error) {
				return registration.GetAllRegistrationsResponse{}, errors.New("some error")
			},
		}
		api := NewAPI(mock, noopLogger)
		req := GetEventsEventIdRegistrationsRequestObject{
			EventId: uuid.New(),
		}

		resp, err := api.GetEventsEventIdRegistrations(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsEventIdRegistrations500JSONResponse:
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
		api := NewAPI(mock, noopLogger)
		req := GetEventsEventIdRegistrationsRequestObject{
			EventId: uuid.New(),
		}

		resp, err := api.GetEventsEventIdRegistrations(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsEventIdRegistrations400JSONResponse:
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
						&mockRegistration{TypeFunc: func() events.RegistrationType { return 99 }},
					},
				}, nil
			},
		}
		api := NewAPI(mock, noopLogger)
		req := GetEventsEventIdRegistrationsRequestObject{
			EventId: uuid.New(),
		}

		resp, err := api.GetEventsEventIdRegistrations(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsEventIdRegistrations500JSONResponse:
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
						registration.IndividualRegistration{
							Email:      "test@test.com",
							Experience: registration.NOVICE,
						},
					},
				}, nil
			},
		}
		api := NewAPI(mock, noopLogger)
		req := GetEventsEventIdRegistrationsRequestObject{
			EventId: uuid.New(),
		}

		resp, err := api.GetEventsEventIdRegistrations(context.Background(), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsEventIdRegistrations200JSONResponse:
			assert.Len(t, r.Data, 1)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

type mockRegistration struct {
	GetEventIDFunc func() uuid.UUID
	TypeFunc       func() events.RegistrationType
}

func (m *mockRegistration) GetEventID() uuid.UUID {
	return m.GetEventIDFunc()
}

func (m *mockRegistration) Type() events.RegistrationType {
	return m.TypeFunc()
}