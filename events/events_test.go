package events

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Rhymond/go-money"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
)

type mockRepository struct {
	GetEventFunc    func(ctx context.Context, id uuid.UUID) (Event, error)
	GetEventsFunc   func(ctx context.Context, limit int32, cursor *string) (GetEventsResponse, error)
	CreateEventFunc func(ctx context.Context, event Event) error
	UpdateEventFunc func(ctx context.Context, event Event) error
}

func (m *mockRepository) GetEvent(ctx context.Context, id uuid.UUID) (Event, error) {
	return m.GetEventFunc(ctx, id)
}

func (m *mockRepository) GetEvents(ctx context.Context, limit int32, cursor *string) (GetEventsResponse, error) {
	return m.GetEventsFunc(ctx, limit, cursor)
}

func (m *mockRepository) CreateEvent(ctx context.Context, event Event) error {
	return m.CreateEventFunc(ctx, event)
}

func (m *mockRepository) UpdateEvent(ctx context.Context, event Event) error {
	return m.UpdateEventFunc(ctx, event)
}

func TestUpdateEvent(t *testing.T) {
	// Test data setup
	eventID := uuid.New()
	startTime := time.Now().Add(24 * time.Hour)
	endTime := time.Now().Add(48 * time.Hour)
	regCloseTime := time.Now().Add(12 * time.Hour)
	location := Location{
		Name: "Test Venue",
		LocAddress: Address{
			Street:     "123 Main St",
			City:       "Test City",
			State:      "TS",
			PostalCode: "12345",
			Country:    "USA",
		},
	}
	rulesLink := "https://example.com/rules"
	imageName := "test-image.jpg"

	t.Run("successful update", func(t *testing.T) {
		tz, _ := time.LoadLocation("America/Denver")
		existingEvent := Event{
			ID:                 eventID,
			Version:            1,
			Name:               "Original Event",
			TimeZone:           tz,
			NumTeams:           5,
			NumRosteredPlayers: 25,
			NumTotalPlayers:    30,
		}

		updatedTz, _ := time.LoadLocation("Europe/Berlin")
		updatedEventData := Event{
			Name:                  "Updated Event Name",
			TimeZone:              updatedTz,
			EventLocation:         location,
			StartTime:             startTime,
			EndTime:               endTime,
			RegistrationCloseTime: regCloseTime,
			RegistrationOptions: []EventRegistrationOption{
				{RegType: BY_INDIVIDUAL, Price: money.New(5000, "USD")},
			},
			AllowedTeamSizeRange: Range{Min: 1, Max: 5},
			RulesDocLink:         &rulesLink,
			ImageName:            &imageName,
		}

		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				assert.Equal(t, eventID, id)
				return existingEvent, nil
			},
			UpdateEventFunc: func(ctx context.Context, event Event) error {
				assert.Equal(t, eventID, event.ID)
				assert.Equal(t, 2, event.Version)
				assert.Equal(t, "Updated Event Name", event.Name)
				assert.Equal(t, "Europe/Berlin", event.TimeZone.String())
				assert.Equal(t, location, event.EventLocation)
				assert.Equal(t, startTime, event.StartTime)
				assert.Equal(t, endTime, event.EndTime)
				assert.Equal(t, regCloseTime, event.RegistrationCloseTime)
				assert.Equal(t, 1, len(event.RegistrationOptions))
				assert.Equal(t, BY_INDIVIDUAL, event.RegistrationOptions[0].RegType)
				assert.Equal(t, Range{Min: 1, Max: 5}, event.AllowedTeamSizeRange)
				assert.Equal(t, &rulesLink, event.RulesDocLink)
				assert.Equal(t, &imageName, event.ImageName)
				// Verify existing stats are preserved
				assert.Equal(t, 5, event.NumTeams)
				assert.Equal(t, 25, event.NumRosteredPlayers)
				assert.Equal(t, 30, event.NumTotalPlayers)
				return nil
			},
		}

		result, err := UpdateEvent(context.Background(), repo, eventID, updatedEventData)

		assert.NoError(t, err)
		assert.Equal(t, eventID, result.ID)
		assert.Equal(t, 2, result.Version)
		assert.Equal(t, "Updated Event Name", result.Name)
		assert.Equal(t, "Europe/Berlin", result.TimeZone.String())
		assert.Equal(t, 5, result.NumTeams)
		assert.Equal(t, 25, result.NumRosteredPlayers)
		assert.Equal(t, 30, result.NumTotalPlayers)
	})

	t.Run("event does not exist", func(t *testing.T) {
		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				return Event{}, &Error{Reason: REASON_EVENT_DOES_NOT_EXIST}
			},
		}

		updatedEventData := Event{Name: "Test Event"}

		result, err := UpdateEvent(context.Background(), repo, eventID, updatedEventData)

		assert.Error(t, err)
		assert.Equal(t, Event{}, result)
		var eventErr *Error
		assert.True(t, errors.As(err, &eventErr))
		assert.Equal(t, REASON_EVENT_DOES_NOT_EXIST, eventErr.Reason)
	})

	t.Run("GetEvent repository error", func(t *testing.T) {
		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				return Event{}, errors.New("database connection failed")
			},
		}

		updatedEventData := Event{Name: "Test Event"}

		result, err := UpdateEvent(context.Background(), repo, eventID, updatedEventData)

		assert.Error(t, err)
		assert.Equal(t, Event{}, result)
		assert.Contains(t, err.Error(), "database connection failed")
	})

	t.Run("UpdateEvent repository error", func(t *testing.T) {
		existingEvent := Event{
			ID:      eventID,
			Version: 1,
			Name:    "Original Event",
		}

		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				return existingEvent, nil
			},
			UpdateEventFunc: func(ctx context.Context, event Event) error {
				return errors.New("update failed")
			},
		}

		updatedEventData := Event{Name: "Updated Event"}

		result, err := UpdateEvent(context.Background(), repo, eventID, updatedEventData)

		assert.Error(t, err)
		assert.Equal(t, Event{}, result)
		assert.Contains(t, err.Error(), "update failed")
	})

	t.Run("version increment", func(t *testing.T) {
		existingEvent := Event{
			ID:      eventID,
			Version: 42,
			Name:    "Original Event",
		}

		var capturedEvent Event
		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				return existingEvent, nil
			},
			UpdateEventFunc: func(ctx context.Context, event Event) error {
				capturedEvent = event
				return nil
			},
		}

		updatedEventData := Event{Name: "Updated Event"}

		result, err := UpdateEvent(context.Background(), repo, eventID, updatedEventData)

		assert.NoError(t, err)
		assert.Equal(t, 43, result.Version)
		assert.Equal(t, 43, capturedEvent.Version)
	})

	t.Run("existing stats preserved", func(t *testing.T) {
		existingEvent := Event{
			ID:                 eventID,
			Version:            1,
			Name:               "Original Event",
			NumTeams:           10,
			NumRosteredPlayers: 50,
			NumTotalPlayers:    60,
		}

		var capturedEvent Event
		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				return existingEvent, nil
			},
			UpdateEventFunc: func(ctx context.Context, event Event) error {
				capturedEvent = event
				return nil
			},
		}

		updatedEventData := Event{
			Name:               "Updated Event",
			NumTeams:           999, // These should be ignored
			NumRosteredPlayers: 999, // These should be ignored
			NumTotalPlayers:    999, // These should be ignored
		}

		result, err := UpdateEvent(context.Background(), repo, eventID, updatedEventData)

		assert.NoError(t, err)
		// Verify the result has existing stats, not the new ones
		assert.Equal(t, 10, result.NumTeams)
		assert.Equal(t, 50, result.NumRosteredPlayers)
		assert.Equal(t, 60, result.NumTotalPlayers)
		// Verify the captured event also has existing stats
		assert.Equal(t, 10, capturedEvent.NumTeams)
		assert.Equal(t, 50, capturedEvent.NumRosteredPlayers)
		assert.Equal(t, 60, capturedEvent.NumTotalPlayers)
	})
}

func TestUpdateEventMedia(t *testing.T) {
	eventID := uuid.New()
	existingMedia := []EventMediaItem{
		{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: "/images/events/boston-2026", Title: ptr.String("Photos")},
		{ID: uuid.New(), Type: MediaTypeVideo, Source: "https://www.youtube.com/watch?v=abc123", DisplayMode: displayModePtr(MediaDisplayModeEmbed)},
	}

	runUpdate := func(t *testing.T, existing Event, update Event) Event {
		t.Helper()

		var captured Event
		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				return existing, nil
			},
			UpdateEventFunc: func(ctx context.Context, event Event) error {
				captured = event
				return nil
			},
		}

		result, err := UpdateEvent(context.Background(), repo, eventID, update)
		assert.NoError(t, err)
		assert.Equal(t, captured.Media, result.Media)

		return result
	}

	t.Run("omitted media preserves the existing list", func(t *testing.T) {
		existing := Event{ID: eventID, Version: 1, Media: existingMedia}

		result := runUpdate(t, existing, Event{Name: "Updated"})

		assert.Equal(t, existingMedia, result.Media)
	})

	t.Run("provided media replaces the existing list", func(t *testing.T) {
		existing := Event{ID: eventID, Version: 1, Media: existingMedia}
		replacement := []EventMediaItem{
			{ID: uuid.New(), Type: MediaTypeExternalLink, Source: "https://example.com/bracket.pdf", Icon: iconPtr(MediaIconPDF)},
		}

		result := runUpdate(t, existing, Event{Name: "Updated", Media: replacement})

		assert.Equal(t, replacement, result.Media)
	})

	t.Run("empty media clears the existing list", func(t *testing.T) {
		existing := Event{ID: eventID, Version: 1, Media: existingMedia}

		result := runUpdate(t, existing, Event{Name: "Updated", Media: []EventMediaItem{}})

		assert.Empty(t, result.Media)
	})

	t.Run("media order is preserved", func(t *testing.T) {
		existing := Event{ID: eventID, Version: 1}
		ordered := []EventMediaItem{
			{ID: uuid.New(), Type: MediaTypeVideo, Source: "https://www.youtube.com/watch?v=third"},
			{ID: uuid.New(), Type: MediaTypeAssetFolder, Source: "/images/events/first"},
			{ID: uuid.New(), Type: MediaTypeExternalLink, Source: "https://example.com/second"},
		}

		result := runUpdate(t, existing, Event{Name: "Updated", Media: ordered})

		assert.Equal(t, ordered, result.Media)
	})
}

func displayModePtr(mode EventMediaDisplayMode) *EventMediaDisplayMode {
	return &mode
}

func iconPtr(icon EventMediaIcon) *EventMediaIcon {
	return &icon
}

func TestUpdateEventTimeZone(t *testing.T) {
	eventID := uuid.New()

	t.Run("timezone is updated correctly", func(t *testing.T) {
		originalTz, _ := time.LoadLocation("America/New_York")
		existingEvent := Event{
			ID:       eventID,
			Version:  1,
			Name:     "Original Event",
			TimeZone: originalTz,
		}

		newTz, _ := time.LoadLocation("Pacific/Auckland")
		updatedEventData := Event{
			Name:     "Updated Event",
			TimeZone: newTz,
		}

		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				assert.Equal(t, eventID, id)
				return existingEvent, nil
			},
			UpdateEventFunc: func(ctx context.Context, event Event) error {
				assert.Equal(t, eventID, event.ID)
				assert.Equal(t, 2, event.Version)
				assert.Equal(t, "Pacific/Auckland", event.TimeZone.String())
				return nil
			},
		}

		result, err := UpdateEvent(context.Background(), repo, eventID, updatedEventData)

		assert.NoError(t, err)
		assert.Equal(t, eventID, result.ID)
		assert.Equal(t, 2, result.Version)
		assert.Equal(t, "Pacific/Auckland", result.TimeZone.String())
	})

	t.Run("nil timezone passed through as nil", func(t *testing.T) {
		existingEvent := Event{
			ID:       eventID,
			Version:  1,
			Name:     "Original Event",
			TimeZone: time.UTC,
		}

		updatedEventData := Event{
			Name:     "Updated Event",
			TimeZone: nil, // Explicitly nil timezone
		}

		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				return existingEvent, nil
			},
			UpdateEventFunc: func(ctx context.Context, event Event) error {
				assert.Nil(t, event.TimeZone)
				return nil
			},
		}

		result, err := UpdateEvent(context.Background(), repo, eventID, updatedEventData)

		assert.NoError(t, err)
		assert.Nil(t, result.TimeZone)
	})

	t.Run("explicit timezone update", func(t *testing.T) {
		originalTz, _ := time.LoadLocation("Europe/London")
		existingEvent := Event{
			ID:       eventID,
			Version:  1,
			Name:     "Original Event",
			TimeZone: originalTz,
		}

		newTz, _ := time.LoadLocation("Asia/Tokyo")
		updatedEventData := Event{
			Name:     "Updated Event Name",
			TimeZone: newTz, // Explicit timezone change
		}

		repo := &mockRepository{
			GetEventFunc: func(ctx context.Context, id uuid.UUID) (Event, error) {
				return existingEvent, nil
			},
			UpdateEventFunc: func(ctx context.Context, event Event) error {
				assert.Equal(t, "Asia/Tokyo", event.TimeZone.String())
				return nil
			},
		}

		result, err := UpdateEvent(context.Background(), repo, eventID, updatedEventData)

		assert.NoError(t, err)
		assert.Equal(t, "Asia/Tokyo", result.TimeZone.String())
	})
}
