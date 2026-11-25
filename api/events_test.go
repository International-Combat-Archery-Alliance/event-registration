package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/Rhymond/go-money"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
				Media: []events.EventMediaItem{
					{ID: uuid.New(), Type: events.MediaTypeAssetFolder, Source: "/images/events/boston-2026"},
				},
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
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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
			require.NotNil(t, r.Data[0].Media)
			require.Len(t, *r.Data[0].Media, 1)
			assert.Equal(t, expectedEvents[0].Media[0].ID, (*r.Data[0].Media)[0].Id)
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
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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

func TestPostEventsMedia(t *testing.T) {
	t.Run("created event includes media", func(t *testing.T) {
		media := []EventMediaItem{
			{
				Id:          uuid.New(),
				Type:        EventMediaTypeAssetFolder,
				Source:      "/images/events/boston-2026",
				Title:       ptr.String("Event Photos"),
				DisplayMode: ptr.To(Carousel),
			},
		}
		var created events.Event
		mock := &mockDB{
			CreateEventFunc: func(ctx context.Context, event events.Event) error {
				created = event
				return nil
			},
		}
		api := newEventsTestAPI(mock, LOCAL)

		resp, err := api.PostEventsV1(ctxWithLogger(context.Background(), noopLogger), PostEventsV1RequestObject{
			Body: &PostEventsV1JSONRequestBody{Name: "Test Event", Media: &media},
		})
		assert.NoError(t, err)

		require.Len(t, created.Media, 1)
		assert.Equal(t, media[0].Id, created.Media[0].ID)
		assert.Equal(t, events.MediaDisplayModeCarousel, *created.Media[0].DisplayMode)

		r, ok := resp.(PostEventsV1200JSONResponse)
		require.True(t, ok)
		require.NotNil(t, r.Media)
		require.Len(t, *r.Media, 1)
		assert.Equal(t, media[0].Id, (*r.Media)[0].Id)
	})

	t.Run("empty media is omitted from the response", func(t *testing.T) {
		media := []EventMediaItem{}
		mock := &mockDB{
			CreateEventFunc: func(ctx context.Context, event events.Event) error {
				assert.Empty(t, event.Media)
				return nil
			},
		}
		api := newEventsTestAPI(mock, LOCAL)

		resp, err := api.PostEventsV1(ctxWithLogger(context.Background(), noopLogger), PostEventsV1RequestObject{
			Body: &PostEventsV1JSONRequestBody{Name: "Test Event", Media: &media},
		})
		assert.NoError(t, err)

		r, ok := resp.(PostEventsV1200JSONResponse)
		require.True(t, ok)
		assert.Nil(t, r.Media)
	})

	t.Run("invalid media is rejected before the repository call", func(t *testing.T) {
		media := []EventMediaItem{
			{Id: uuid.New(), Type: EventMediaTypeExternalLink, Source: "javascript:alert(1)"},
		}
		createCalled := false
		mock := &mockDB{
			CreateEventFunc: func(ctx context.Context, event events.Event) error {
				createCalled = true
				return nil
			},
		}
		api := newEventsTestAPI(mock, LOCAL)

		resp, err := api.PostEventsV1(ctxWithLogger(context.Background(), noopLogger), PostEventsV1RequestObject{
			Body: &PostEventsV1JSONRequestBody{Name: "Test Event", Media: &media},
		})
		assert.NoError(t, err)

		r, ok := resp.(PostEventsV1400JSONResponse)
		require.True(t, ok)
		assert.Equal(t, InvalidBody, r.Code)
		assert.False(t, createCalled)
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
			Media: []events.EventMediaItem{
				{ID: uuid.New(), Type: events.MediaTypeVideo, Source: "https://www.youtube.com/watch?v=abc123", Title: ptr.String("Final")},
			},
		}
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, eventId uuid.UUID) (events.Event, error) {
				assert.Equal(t, id, eventId)
				return expectedEvent, nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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
			require.NotNil(t, r.Event.Media)
			require.Len(t, *r.Event.Media, 1)
			assert.Equal(t, expectedEvent.Media[0].ID, (*r.Event.Media)[0].Id)
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
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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

		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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

	t.Run("concurrent modification returns 409", func(t *testing.T) {
		eventID := uuid.New()
		mock := &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{ID: eventID, Version: 1, Name: "Original Event"}, nil
			},
			UpdateEventFunc: func(ctx context.Context, event events.Event) error {
				return events.NewVersionConflictError("changed concurrently", nil)
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

		reqBody := Event{
			Name: "Updated Event",
			RegistrationOptions: []EventRegistrationOption{
				{RegistrationType: ByIndividual, Price: Money{Amount: 5000, Currency: "USD"}},
			},
			SignUpStats: &SignUpStats{},
		}
		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), PatchEventsV1IdRequestObject{Id: eventID, Body: &reqBody})
		assert.NoError(t, err)

		switch r := resp.(type) {
		case PatchEventsV1Id409JSONResponse:
			assert.Equal(t, Conflict, r.Code)
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
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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

		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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

		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })

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

func newEventsTestAPI(db DB, env Environment) *API {
	return NewAPI(db, noopLogger, env, newTestTokenValidator(), &mockCaptchaValidator{}, &mockEmailSender{}, &mockSubscriberManager{}, &mockCheckoutManager{}, func(context.Context) error { return nil })
}

func TestPatchEventsV1IdMedia(t *testing.T) {
	existingMedia := []events.EventMediaItem{
		{ID: uuid.New(), Type: events.MediaTypeAssetFolder, Source: "/images/events/boston-2026"},
	}

	newMock := func(captured *events.Event) *mockDB {
		eventID := uuid.New()
		return &mockDB{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (events.Event, error) {
				return events.Event{ID: eventID, Version: 1, Media: existingMedia}, nil
			},
			UpdateEventFunc: func(ctx context.Context, event events.Event) error {
				*captured = event
				return nil
			},
		}
	}

	t.Run("omitted media clears existing media", func(t *testing.T) {
		var captured events.Event
		api := newEventsTestAPI(newMock(&captured), LOCAL)

		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), PatchEventsV1IdRequestObject{
			Id:   uuid.New(),
			Body: &Event{Name: "Updated Event"},
		})
		assert.NoError(t, err)

		assert.Nil(t, captured.Media)

		r, ok := resp.(PatchEventsV1Id200JSONResponse)
		require.True(t, ok)
		assert.Nil(t, r.Event.Media)
	})

	t.Run("explicit empty media clears existing media", func(t *testing.T) {
		var captured events.Event
		api := newEventsTestAPI(newMock(&captured), LOCAL)

		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), PatchEventsV1IdRequestObject{
			Id:   uuid.New(),
			Body: &Event{Name: "Updated Event", Media: &[]EventMediaItem{}},
		})
		assert.NoError(t, err)

		assert.Nil(t, captured.Media)

		r, ok := resp.(PatchEventsV1Id200JSONResponse)
		require.True(t, ok)
		assert.Nil(t, r.Event.Media)
	})

	t.Run("provided media replaces existing media", func(t *testing.T) {
		var captured events.Event
		api := newEventsTestAPI(newMock(&captured), LOCAL)

		replacementID := uuid.New()
		media := []EventMediaItem{
			{Id: replacementID, Type: EventMediaTypeVideo, Source: "https://www.youtube.com/watch?v=abc123", Title: ptr.String("Final")},
		}

		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), PatchEventsV1IdRequestObject{
			Id:   uuid.New(),
			Body: &Event{Name: "Updated Event", Media: &media},
		})
		assert.NoError(t, err)

		assert.Equal(t, []events.EventMediaItem{
			{ID: replacementID, Type: events.MediaTypeVideo, Source: "https://www.youtube.com/watch?v=abc123", Title: ptr.String("Final")},
		}, captured.Media)

		r, ok := resp.(PatchEventsV1Id200JSONResponse)
		require.True(t, ok)
		require.NotNil(t, r.Event.Media)
		assert.Len(t, *r.Event.Media, 1)
	})

	t.Run("invalid media is rejected before the repository call", func(t *testing.T) {
		var captured events.Event
		mock := newMock(&captured)
		api := newEventsTestAPI(mock, LOCAL)

		media := []EventMediaItem{{Id: uuid.New(), Type: EventMediaTypeAssetFolder, Source: "/images/../secret"}}

		resp, err := api.PatchEventsV1Id(ctxWithLogger(context.Background(), noopLogger), PatchEventsV1IdRequestObject{
			Id:   uuid.New(),
			Body: &Event{Name: "Updated Event", Media: &media},
		})
		assert.NoError(t, err)

		r, ok := resp.(PatchEventsV1Id400JSONResponse)
		require.True(t, ok)
		assert.Equal(t, InvalidBody, r.Code)
		assert.Zero(t, captured)
	})
}

func TestOpenAPIValidationMedia(t *testing.T) {
	swagger, err := GetSwagger()
	require.NoError(t, err)
	swagger.Servers = nil

	api := newEventsTestAPI(&mockDB{}, LOCAL)
	handlerCalled := false
	handler := api.openapiValidateMiddleware(swagger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	authHeader := "Bearer " + generateTestToken("admin@example.com", true)
	eventID := uuid.New()

	bodyWithMedia := func(media string) string {
		mediaField := ""
		if media != "" {
			mediaField = `, "media": ` + media
		}
		return `{
			"name": "Media Validation Event",
			"location": {
				"name": "Test Venue",
				"address": {
					"street": "123 Main St",
					"city": "Anytown",
					"state": "CA",
					"postalCode": "90210",
					"country": "USA"
				}
			},
			"startTime": "2026-08-19T18:00:00.000Z",
			"endTime": "2026-08-19T22:00:00.000Z",
			"registrationCloseTime": "2026-08-17T23:59:59.000Z",
			"registrationOptions": [
				{"registrationType": "ByIndividual", "price": {"currency": "USD", "amount": 0}}
			],
			"allowedTeamSizeRange": {"min": 1, "max": 5}` + mediaField + `
		}`
	}

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "omitted media", body: bodyWithMedia(""), wantStatus: http.StatusOK},
		{name: "empty media", body: bodyWithMedia("[]"), wantStatus: http.StatusOK},
		{name: "null media", body: bodyWithMedia("null"), wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handlerCalled = false
			req := httptest.NewRequest(http.MethodPatch, "/events/v1/"+eventID.String(), strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", authHeader)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantStatus == http.StatusOK, handlerCalled)
		})
	}
}
