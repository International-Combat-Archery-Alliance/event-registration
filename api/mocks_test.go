package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/International-Combat-Archery-Alliance/auth"
	"github.com/International-Combat-Archery-Alliance/captcha"
	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/games"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
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
	// Events
	GetEventsFunc   func(ctx context.Context, limit int32, cursor *string) (events.GetEventsResponse, error)
	CreateEventFunc func(ctx context.Context, event events.Event) error
	GetEventFunc    func(ctx context.Context, id uuid.UUID) (events.Event, error)
	UpdateEventFunc func(ctx context.Context, event events.Event) error

	// Registration
	CreateRegistrationFunc            func(ctx context.Context, registration registration.Registration, event events.Event) error
	GetAllRegistrationsForEventFunc   func(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (registration.GetAllRegistrationsResponse, error)
	CreateRegistrationWithPaymentFunc func(ctx context.Context, reg registration.Registration, intent registration.RegistrationIntent, event events.Event) error
	GetRegistrationFunc               func(ctx context.Context, eventId uuid.UUID, email string) (registration.Registration, error)
	UpdateRegistrationToPaidFunc      func(ctx context.Context, reg registration.Registration) error
	DeleteExpiredRegistrationFunc     func(ctx context.Context, registration registration.Registration, intent registration.RegistrationIntent, event events.Event) error
	GetRegistrationIntentFunc         func(ctx context.Context, eventId uuid.UUID, email string) (registration.RegistrationIntent, error)

	// Global Teams (TeamRepository)
	GetTeamFunc    func(ctx context.Context, teamID uuid.UUID) (teams.Team, error)
	GetTeamsFunc   func(ctx context.Context, limit int32, cursor *string) (teams.GetTeamsResponse, error)
	CreateTeamFunc func(ctx context.Context, team teams.Team) error
	UpdateTeamFunc func(ctx context.Context, team teams.Team) error
	DeleteTeamFunc func(ctx context.Context, teamID uuid.UUID) error

	// Event Teams (EventTeamRepository)
	GetEventTeamFunc              func(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID) (teams.EventTeam, error)
	GetEventTeamsForEventFunc     func(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (teams.GetEventTeamsResponse, error)
	GetEventTeamsByTeamFunc       func(ctx context.Context, teamID uuid.UUID, limit int32, cursor *string) (teams.GetEventTeamsResponse, error)
	CreateEventTeamFunc           func(ctx context.Context, eventTeam teams.EventTeam) error
	UpdateEventTeamFunc           func(ctx context.Context, eventTeam teams.EventTeam) error
	DeleteEventTeamFunc           func(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID) error
	AddPlayerToEventTeamFunc      func(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID, player teams.TeamPlayer) error
	RemovePlayerFromEventTeamFunc func(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID, registrationID uuid.UUID) error
	HasGamesFunc                  func(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID) (bool, error)
	IsIndividualAssignedFunc      func(ctx context.Context, eventID uuid.UUID, registrationID uuid.UUID) (bool, error)

	// Games
	GetGameFunc          func(ctx context.Context, eventID uuid.UUID, gameID uuid.UUID) (games.Game, error)
	GetGamesForEventFunc func(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (games.GetGamesResponse, error)
	CreateGameFunc       func(ctx context.Context, game games.Game) error
	UpdateGameFunc       func(ctx context.Context, game games.Game) error
	DeleteGameFunc       func(ctx context.Context, eventID uuid.UUID, gameID uuid.UUID) error
	RecordResultFunc     func(ctx context.Context, game games.Game, team1Standing games.Standing, team2Standing games.Standing) error

	// Standings
	GetStandingsForEventFunc func(ctx context.Context, eventID uuid.UUID) (games.GetStandingsResponse, error)
}

// ==================== Events ====================

func (m *mockDB) GetEvents(ctx context.Context, limit int32, cursor *string) (events.GetEventsResponse, error) {
	if m.GetEventsFunc != nil {
		return m.GetEventsFunc(ctx, limit, cursor)
	}
	return events.GetEventsResponse{}, nil
}

func (m *mockDB) CreateEvent(ctx context.Context, event events.Event) error {
	if m.CreateEventFunc != nil {
		return m.CreateEventFunc(ctx, event)
	}
	return nil
}

func (m *mockDB) GetEvent(ctx context.Context, id uuid.UUID) (events.Event, error) {
	if m.GetEventFunc != nil {
		return m.GetEventFunc(ctx, id)
	}
	return events.Event{}, nil
}

func (m *mockDB) UpdateEvent(ctx context.Context, event events.Event) error {
	if m.UpdateEventFunc != nil {
		return m.UpdateEventFunc(ctx, event)
	}
	return nil
}

// ==================== Registration ====================

func (m *mockDB) CreateRegistration(ctx context.Context, reg registration.Registration, event events.Event) error {
	if m.CreateRegistrationFunc != nil {
		return m.CreateRegistrationFunc(ctx, reg, event)
	}
	return nil
}

func (m *mockDB) GetAllRegistrationsForEvent(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (registration.GetAllRegistrationsResponse, error) {
	if m.GetAllRegistrationsForEventFunc != nil {
		return m.GetAllRegistrationsForEventFunc(ctx, eventID, limit, cursor)
	}
	return registration.GetAllRegistrationsResponse{}, nil
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

func (m *mockDB) DeleteExpiredRegistration(ctx context.Context, reg registration.Registration, intent registration.RegistrationIntent, event events.Event) error {
	if m.DeleteExpiredRegistrationFunc != nil {
		return m.DeleteExpiredRegistrationFunc(ctx, reg, intent, event)
	}
	return nil
}

func (m *mockDB) GetRegistrationIntent(ctx context.Context, eventId uuid.UUID, email string) (registration.RegistrationIntent, error) {
	if m.GetRegistrationIntentFunc != nil {
		return m.GetRegistrationIntentFunc(ctx, eventId, email)
	}
	return registration.RegistrationIntent{}, nil
}

// ==================== Global Teams (TeamRepository) ====================

func (m *mockDB) GetTeam(ctx context.Context, teamID uuid.UUID) (teams.Team, error) {
	if m.GetTeamFunc != nil {
		return m.GetTeamFunc(ctx, teamID)
	}
	return teams.Team{}, nil
}

func (m *mockDB) GetTeams(ctx context.Context, limit int32, cursor *string) (teams.GetTeamsResponse, error) {
	if m.GetTeamsFunc != nil {
		return m.GetTeamsFunc(ctx, limit, cursor)
	}
	return teams.GetTeamsResponse{}, nil
}

func (m *mockDB) CreateTeam(ctx context.Context, team teams.Team) error {
	if m.CreateTeamFunc != nil {
		return m.CreateTeamFunc(ctx, team)
	}
	return nil
}

func (m *mockDB) UpdateTeam(ctx context.Context, team teams.Team) error {
	if m.UpdateTeamFunc != nil {
		return m.UpdateTeamFunc(ctx, team)
	}
	return nil
}

func (m *mockDB) DeleteTeam(ctx context.Context, teamID uuid.UUID) error {
	if m.DeleteTeamFunc != nil {
		return m.DeleteTeamFunc(ctx, teamID)
	}
	return nil
}

// ==================== Event Teams (EventTeamRepository) ====================

func (m *mockDB) GetEventTeam(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID) (teams.EventTeam, error) {
	if m.GetEventTeamFunc != nil {
		return m.GetEventTeamFunc(ctx, eventID, eventTeamID)
	}
	return teams.EventTeam{}, nil
}

func (m *mockDB) GetEventTeamsForEvent(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (teams.GetEventTeamsResponse, error) {
	if m.GetEventTeamsForEventFunc != nil {
		return m.GetEventTeamsForEventFunc(ctx, eventID, limit, cursor)
	}
	return teams.GetEventTeamsResponse{}, nil
}

func (m *mockDB) GetEventTeamsByTeam(ctx context.Context, teamID uuid.UUID, limit int32, cursor *string) (teams.GetEventTeamsResponse, error) {
	if m.GetEventTeamsByTeamFunc != nil {
		return m.GetEventTeamsByTeamFunc(ctx, teamID, limit, cursor)
	}
	return teams.GetEventTeamsResponse{}, nil
}

func (m *mockDB) CreateEventTeam(ctx context.Context, eventTeam teams.EventTeam) error {
	if m.CreateEventTeamFunc != nil {
		return m.CreateEventTeamFunc(ctx, eventTeam)
	}
	return nil
}

func (m *mockDB) UpdateEventTeam(ctx context.Context, eventTeam teams.EventTeam) error {
	if m.UpdateEventTeamFunc != nil {
		return m.UpdateEventTeamFunc(ctx, eventTeam)
	}
	return nil
}

func (m *mockDB) DeleteEventTeam(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID) error {
	if m.DeleteEventTeamFunc != nil {
		return m.DeleteEventTeamFunc(ctx, eventID, eventTeamID)
	}
	return nil
}

func (m *mockDB) AddPlayerToEventTeam(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID, player teams.TeamPlayer) error {
	if m.AddPlayerToEventTeamFunc != nil {
		return m.AddPlayerToEventTeamFunc(ctx, eventID, eventTeamID, player)
	}
	return nil
}

func (m *mockDB) RemovePlayerFromEventTeam(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID, registrationID uuid.UUID) error {
	if m.RemovePlayerFromEventTeamFunc != nil {
		return m.RemovePlayerFromEventTeamFunc(ctx, eventID, eventTeamID, registrationID)
	}
	return nil
}

func (m *mockDB) HasGames(ctx context.Context, eventID uuid.UUID, eventTeamID uuid.UUID) (bool, error) {
	if m.HasGamesFunc != nil {
		return m.HasGamesFunc(ctx, eventID, eventTeamID)
	}
	return false, nil
}

func (m *mockDB) IsIndividualAssigned(ctx context.Context, eventID uuid.UUID, registrationID uuid.UUID) (bool, error) {
	if m.IsIndividualAssignedFunc != nil {
		return m.IsIndividualAssignedFunc(ctx, eventID, registrationID)
	}
	return false, nil
}

// ==================== Games ====================

func (m *mockDB) GetGame(ctx context.Context, eventID uuid.UUID, gameID uuid.UUID) (games.Game, error) {
	if m.GetGameFunc != nil {
		return m.GetGameFunc(ctx, eventID, gameID)
	}
	return games.Game{}, nil
}

func (m *mockDB) GetGamesForEvent(ctx context.Context, eventID uuid.UUID, limit int32, cursor *string) (games.GetGamesResponse, error) {
	if m.GetGamesForEventFunc != nil {
		return m.GetGamesForEventFunc(ctx, eventID, limit, cursor)
	}
	return games.GetGamesResponse{}, nil
}

func (m *mockDB) CreateGame(ctx context.Context, game games.Game) error {
	if m.CreateGameFunc != nil {
		return m.CreateGameFunc(ctx, game)
	}
	return nil
}

func (m *mockDB) UpdateGame(ctx context.Context, game games.Game) error {
	if m.UpdateGameFunc != nil {
		return m.UpdateGameFunc(ctx, game)
	}
	return nil
}

func (m *mockDB) DeleteGame(ctx context.Context, eventID uuid.UUID, gameID uuid.UUID) error {
	if m.DeleteGameFunc != nil {
		return m.DeleteGameFunc(ctx, eventID, gameID)
	}
	return nil
}

func (m *mockDB) RecordResult(ctx context.Context, game games.Game, team1Standing games.Standing, team2Standing games.Standing) error {
	if m.RecordResultFunc != nil {
		return m.RecordResultFunc(ctx, game, team1Standing, team2Standing)
	}
	return nil
}

// ==================== Standings ====================

func (m *mockDB) GetStandingsForEvent(ctx context.Context, eventID uuid.UUID) (games.GetStandingsResponse, error) {
	if m.GetStandingsForEventFunc != nil {
		return m.GetStandingsForEventFunc(ctx, eventID)
	}
	return games.GetStandingsResponse{}, nil
}
