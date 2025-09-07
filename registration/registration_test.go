package registration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
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

type mockRegistrationRepository struct {
	CreateRegistrationFunc            func(ctx context.Context, registration Registration, event events.Event) error
	GetAllRegistrationsForEventFunc   func(ctx context.Context, eventId uuid.UUID, limit int32, cursor *string) (GetAllRegistrationsResponse, error)
	CreateRegistrationWithPaymentFunc func(ctx context.Context, registration Registration, intent RegistrationIntent, event events.Event) error
	GetRegistrationFunc               func(ctx context.Context, eventId uuid.UUID, email string) (Registration, error)
	UpdateRegistrationToPaidFunc      func(ctx context.Context, registration Registration) error
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
