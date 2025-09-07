package api

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/auth"
	"github.com/International-Combat-Archery-Alliance/captcha"
	"github.com/International-Combat-Archery-Alliance/email"
	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/International-Combat-Archery-Alliance/event-registration/registration"
	"github.com/International-Combat-Archery-Alliance/middleware"
	"github.com/International-Combat-Archery-Alliance/payments"
	"github.com/Rhymond/go-money"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
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

type mockCheckoutManager struct{}

func (m *mockCheckoutManager) CreateCheckout(ctx context.Context, params payments.CheckoutParams) (payments.CheckoutInfo, error) {
	return payments.CheckoutInfo{}, nil
}

func (m *mockCheckoutManager) ConfirmCheckout(ctx context.Context, payload []byte, signature string) (map[string]string, error) {
	return map[string]string{}, nil
}

func ctxWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return middleware.CtxWithLogger(ctx, logger)
}

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

func TestGetEvents(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		id := uuid.New()
		now := time.Now()
		tz, _ := time.LoadLocation("America/New_York")
		expectedEvents := []events.Event{
			{
				ID:                    id,
				Name:                  "Test Event",
				TimeZone:              tz,
				StartTime:             now,
				EndTime:               now.Add(time.Hour),
				RegistrationCloseTime: now,
				RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(5000, "USD")}},
				RulesDocLink:          ptr.String("https://example.com/rules"),
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
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		req := GetEventsV1RequestObject{
			Params: GetEventsV1Params{
				Limit: ptr.Int(10),
			},
		}

		resp, err := api.GetEventsV1(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsV1200JSONResponse:
			assert.Equal(t, len(expectedEvents), len(r.Data))
			assert.Equal(t, &expectedEvents[0].ID, r.Data[0].Id)
			assert.Equal(t, expectedEvents[0].Name, r.Data[0].Name)
			assert.Equal(t, ptr.String("America/New_York"), r.Data[0].TimeZone)
			assert.Equal(t, []EventRegistrationOption{{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}}}, r.Data[0].RegistrationOptions)
			assert.Equal(t, expectedEvents[0].RulesDocLink, r.Data[0].RulesDocLink)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestPostEvents(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		now := time.Now()
		reqBody := PostEventsV1JSONRequestBody{
			Name:                  "Test Event",
			TimeZone:              ptr.String("America/New_York"),
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
			RegistrationCloseTime: now,
			RegistrationOptions:   []EventRegistrationOption{{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}}},
			RulesDocLink:          ptr.String("https://example.com/rules"),
		}
		mock := &mockDB{
			CreateEventFunc: func(ctx context.Context, event events.Event) error {
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		req := PostEventsV1RequestObject{
			Body: &reqBody,
		}

		resp, err := api.PostEventsV1(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1200JSONResponse:
			assert.NotNil(t, r.Id)
			assert.Equal(t, reqBody.Name, r.Name)
			assert.Equal(t, reqBody.TimeZone, r.TimeZone)
			assert.Equal(t, reqBody.RegistrationOptions, r.RegistrationOptions)
			assert.Equal(t, reqBody.RulesDocLink, r.RulesDocLink)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestGetEventsId(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		id := uuid.New()
		now := time.Now()
		tz, _ := time.LoadLocation("Europe/London")
		expectedEvent := events.Event{
			ID:                    id,
			Name:                  "Test Event",
			TimeZone:              tz,
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
			RegistrationCloseTime: now,
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(5000, "USD")}},
			RulesDocLink:          ptr.String("https://example.com/rules"),
		}
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, eventId uuid.UUID) (events.Event, error) {
				assert.Equal(t, id, eventId)
				return expectedEvent, nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		req := GetEventsV1IdRequestObject{
			Id: id,
		}

		resp, err := api.GetEventsV1Id(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsV1Id200JSONResponse:
			assert.Equal(t, &expectedEvent.ID, r.Event.Id)
			assert.Equal(t, expectedEvent.Name, r.Event.Name)
			assert.Equal(t, ptr.String("Europe/London"), r.Event.TimeZone)
			assert.Equal(t, []EventRegistrationOption{{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}}}, r.Event.RegistrationOptions)
			assert.Equal(t, expectedEvent.RulesDocLink, r.Event.RulesDocLink)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("not found", func(t *testing.T) {
		id := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, eventId uuid.UUID) (events.Event, error) {
				return events.Event{}, &events.Error{Reason: events.REASON_EVENT_DOES_NOT_EXIST}
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		req := GetEventsV1IdRequestObject{
			Id: id,
		}

		resp, err := api.GetEventsV1Id(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsV1Id404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("internal server error", func(t *testing.T) {
		id := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, eventId uuid.UUID) (events.Event, error) {
				return events.Event{}, errors.New("some error")
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		req := GetEventsV1IdRequestObject{
			Id: id,
		}

		resp, err := api.GetEventsV1Id(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case GetEventsV1Id500JSONResponse:
			assert.Equal(t, InternalError, r.Code)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestPatchEventsV1Id(t *testing.T) {
	t.Run("successful update", func(t *testing.T) {
		eventID := uuid.New()
		now := time.Now()

		existingEvent := events.Event{
			ID:                 eventID,
			Version:            1,
			Name:               "Original Event",
			NumTeams:           5,
			NumRosteredPlayers: 25,
			NumTotalPlayers:    30,
		}

		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				assert.Equal(t, eventID, id)
				return existingEvent, nil
			},
			UpdateEventFunc: func(ctx context.Context, event events.Event) error {
				assert.Equal(t, eventID, event.ID)
				assert.Equal(t, "Updated Event Name", event.Name)
				return nil
			},
		}

		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		reqBody := Event{
			Name:                  "Updated Event Name",
			TimeZone:              ptr.String("America/Los_Angeles"),
			StartTime:             now,
			EndTime:               now.Add(2 * time.Hour),
			RegistrationCloseTime: now.Add(-time.Hour),
			Location: Location{
				Name: "New Venue",
				Address: Address{
					Street:     "456 New St",
					City:       "New City",
					State:      "NS",
					PostalCode: "54321",
					Country:    "USA",
				},
			},
			RegistrationOptions: []EventRegistrationOption{
				{RegistrationType: ByTeam, Price: Money{Amount: 10000, Currency: "USD"}},
			},
			AllowedTeamSizeRange: Range{Min: 2, Max: 6},
			RulesDocLink:         ptr.String("https://example.com/new-rules"),
			ImageName:            ptr.String("new-image.jpg"),
		}

		req := PatchEventsV1IdRequestObject{
			Id:   eventID,
			Body: &reqBody,
		}

		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PatchEventsV1Id200JSONResponse:
			assert.Equal(t, reqBody.Name, r.Event.Name)
			assert.Equal(t, reqBody.TimeZone, r.Event.TimeZone)
			assert.Equal(t, reqBody.StartTime, r.Event.StartTime)
			assert.Equal(t, reqBody.EndTime, r.Event.EndTime)
			assert.Equal(t, reqBody.RegistrationCloseTime, r.Event.RegistrationCloseTime)
			assert.Equal(t, reqBody.Location, r.Event.Location)
			assert.Equal(t, reqBody.RegistrationOptions, r.Event.RegistrationOptions)
			assert.Equal(t, reqBody.AllowedTeamSizeRange, r.Event.AllowedTeamSizeRange)
			assert.Equal(t, reqBody.RulesDocLink, r.Event.RulesDocLink)
			assert.Equal(t, reqBody.ImageName, r.Event.ImageName)
			assert.Equal(t, existingEvent.NumTotalPlayers, r.Event.SignUpStats.NumTotalPlayers)
			assert.Equal(t, existingEvent.NumRosteredPlayers, r.Event.SignUpStats.NumRosteredPlayers)
			assert.Equal(t, existingEvent.NumTeams, r.Event.SignUpStats.NumTeams)
			// Version should be incremented
			assert.Equal(t, 2, *r.Event.Version)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("invalid request body", func(t *testing.T) {
		eventID := uuid.New()
		mock := &mockDB{}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		// Create invalid request body with invalid registration type
		reqBody := Event{
			Name: "Test Event",
			RegistrationOptions: []EventRegistrationOption{
				{RegistrationType: RegistrationType("INVALID"), Price: Money{Amount: 5000, Currency: "USD"}},
			},
			SignUpStats: &SignUpStats{
				NumTeams:           0,
				NumRosteredPlayers: 0,
				NumTotalPlayers:    0,
			},
		}

		req := PatchEventsV1IdRequestObject{
			Id:   eventID,
			Body: &reqBody,
		}

		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PatchEventsV1Id400JSONResponse:
			assert.Equal(t, InvalidBody, r.Code)
			assert.Equal(t, "Invalid event body", r.Message)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("event not found", func(t *testing.T) {
		eventID := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{}, &events.Error{Reason: events.REASON_EVENT_DOES_NOT_EXIST}
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		reqBody := Event{
			Name: "Test Event",
			RegistrationOptions: []EventRegistrationOption{
				{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}},
			},
			SignUpStats: &SignUpStats{
				NumTeams:           0,
				NumRosteredPlayers: 0,
				NumTotalPlayers:    0,
			},
		}

		req := PatchEventsV1IdRequestObject{
			Id:   eventID,
			Body: &reqBody,
		}

		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PatchEventsV1Id404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
			assert.Equal(t, "Event not found", r.Message)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("update event error", func(t *testing.T) {
		eventID := uuid.New()
		existingEvent := events.Event{
			ID:      eventID,
			Version: 1,
			Name:    "Original Event",
		}

		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return existingEvent, nil
			},
			UpdateEventFunc: func(ctx context.Context, event events.Event) error {
				return errors.New("database connection failed")
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		reqBody := Event{
			Name: "Updated Event",
			RegistrationOptions: []EventRegistrationOption{
				{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}},
			},
			SignUpStats: &SignUpStats{
				NumTeams:           0,
				NumRosteredPlayers: 0,
				NumTotalPlayers:    0,
			},
		}

		req := PatchEventsV1IdRequestObject{
			Id:   eventID,
			Body: &reqBody,
		}

		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PatchEventsV1Id500JSONResponse:
			assert.Equal(t, InternalError, r.Code)
			assert.Equal(t, "Updating event failed", r.Message)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestTimeZoneHandling(t *testing.T) {
	t.Run("create event with valid timezone", func(t *testing.T) {
		now := time.Now()
		reqBody := PostEventsV1JSONRequestBody{
			Name:                  "Timezone Test Event",
			TimeZone:              ptr.String("America/New_York"),
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
			RegistrationCloseTime: now,
			RegistrationOptions:   []EventRegistrationOption{{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}}},
		}
		mock := &mockDB{
			CreateEventFunc: func(ctx context.Context, event events.Event) error {
				assert.Equal(t, "America/New_York", event.TimeZone.String())
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		req := PostEventsV1RequestObject{
			Body: &reqBody,
		}

		resp, err := api.PostEventsV1(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1200JSONResponse:
			assert.Equal(t, ptr.String("America/New_York"), r.TimeZone)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("create event with invalid timezone", func(t *testing.T) {
		now := time.Now()
		reqBody := PostEventsV1JSONRequestBody{
			Name:                  "Invalid Timezone Event",
			TimeZone:              ptr.String("Invalid/Timezone"),
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
			RegistrationCloseTime: now,
			RegistrationOptions:   []EventRegistrationOption{{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}}},
		}
		mock := &mockDB{}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		req := PostEventsV1RequestObject{
			Body: &reqBody,
		}

		resp, err := api.PostEventsV1(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1400JSONResponse:
			assert.Equal(t, InvalidBody, r.Code)
			assert.Equal(t, "Failed to create the event", r.Message)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("create event without timezone defaults to UTC", func(t *testing.T) {
		now := time.Now()
		reqBody := PostEventsV1JSONRequestBody{
			Name:                  "No Timezone Event",
			StartTime:             now,
			EndTime:               now.Add(time.Hour),
			RegistrationCloseTime: now,
			RegistrationOptions:   []EventRegistrationOption{{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}}},
		}
		mock := &mockDB{
			CreateEventFunc: func(ctx context.Context, event events.Event) error {
				assert.Equal(t, time.UTC, event.TimeZone)
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		req := PostEventsV1RequestObject{
			Body: &reqBody,
		}

		resp, err := api.PostEventsV1(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PostEventsV1200JSONResponse:
			assert.Nil(t, r.TimeZone)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("update event timezone", func(t *testing.T) {
		eventID := uuid.New()
		now := time.Now()
		existingEvent := events.Event{
			ID:       eventID,
			Version:  1,
			Name:     "Original Event",
			TimeZone: time.UTC,
		}

		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				assert.Equal(t, eventID, id)
				return existingEvent, nil
			},
			UpdateEventFunc: func(ctx context.Context, event events.Event) error {
				assert.Equal(t, eventID, event.ID)
				assert.Equal(t, "Pacific/Auckland", event.TimeZone.String())
				return nil
			},
		}

		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		reqBody := Event{
			Name:                  "Updated Event",
			TimeZone:              ptr.String("Pacific/Auckland"),
			StartTime:             now,
			EndTime:               now.Add(2 * time.Hour),
			RegistrationCloseTime: now.Add(-time.Hour),
			RegistrationOptions: []EventRegistrationOption{
				{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}},
			},
			SignUpStats: &SignUpStats{
				NumTeams:           0,
				NumRosteredPlayers: 0,
				NumTotalPlayers:    0,
			},
		}

		req := PatchEventsV1IdRequestObject{
			Id:   eventID,
			Body: &reqBody,
		}

		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PatchEventsV1Id200JSONResponse:
			assert.Equal(t, ptr.String("Pacific/Auckland"), r.Event.TimeZone)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("update event with invalid timezone", func(t *testing.T) {
		eventID := uuid.New()
		now := time.Now()
		existingEvent := events.Event{
			ID:      eventID,
			Version: 1,
			Name:    "Original Event",
		}

		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return existingEvent, nil
			},
		}

		api := NewAPI(mock, noopLogger, LOCAL, &mockAuthValidator{}, &mockCaptchaValidator{}, &mockEmailSender{}, &mockCheckoutManager{})

		reqBody := Event{
			Name:                  "Updated Event",
			TimeZone:              ptr.String("Not/A/Real/Timezone"),
			StartTime:             now,
			EndTime:               now.Add(2 * time.Hour),
			RegistrationCloseTime: now.Add(-time.Hour),
			RegistrationOptions: []EventRegistrationOption{
				{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}},
			},
			SignUpStats: &SignUpStats{
				NumTeams:           0,
				NumRosteredPlayers: 0,
				NumTotalPlayers:    0,
			},
		}

		req := PatchEventsV1IdRequestObject{
			Id:   eventID,
			Body: &reqBody,
		}

		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), req)
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PatchEventsV1Id400JSONResponse:
			assert.Equal(t, InvalidBody, r.Code)
			assert.Equal(t, "Invalid event body", r.Message)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}
