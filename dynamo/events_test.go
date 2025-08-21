package dynamo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully create an event", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:           uuid.New(),
			Name:         "Test Event",
			StartTime:    time.Now(),
			EndTime:      time.Now().Add(time.Hour),
			RulesDocLink: ptr.String("https://example.com/rules"),
			Version:      1,
		}

		require.NoError(t, db.CreateEvent(ctx, event))
	})

	t.Run("fail to create an event that already exists", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:        uuid.New(),
			Name:      "Test Event",
			StartTime: time.Now(),
			EndTime:   time.Now().Add(time.Hour),
			Version:   1,
		}

		require.NoError(t, db.CreateEvent(ctx, event))

		eventErr := db.CreateEvent(ctx, event)
		require.Error(t, eventErr)
		var eventError *events.Error
		require.ErrorAs(t, eventErr, &eventError)
		assert.Equal(t, events.REASON_EVENT_ALREADY_EXISTS, eventError.Reason)
	})

	t.Run("successfully create an event and verify data", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:   uuid.New(),
			Name: "Test Event",
			EventLocation: events.Location{
				Name: "Test Location",
				LocAddress: events.Address{
					Street:     "123 Test St",
					City:       "Test City",
					State:      "TS",
					PostalCode: "12345",
					Country:    "Testland",
				},
			},
			StartTime:             time.Now().UTC().Truncate(time.Second),
			EndTime:               time.Now().Add(time.Hour).UTC().Truncate(time.Second),
			RegistrationCloseTime: time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second),
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL}, {RegType: events.BY_TEAM}},
			AllowedTeamSizeRange:  events.Range{Min: 3, Max: 5},
			NumTeams:              10,
			NumRosteredPlayers:    50,
			NumTotalPlayers:       60,
			RulesDocLink:          ptr.String("https://example.com/rules"),
			Version:               1,
		}

		require.NoError(t, db.CreateEvent(ctx, event))

		dynamoEvent := newEventDynamo(event)
		key, err := attributevalue.MarshalMap(map[string]any{
			"PK": dynamoEvent.PK,
			"SK": dynamoEvent.SK,
		})
		require.NoError(t, err)

		out, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(tableName),
			Key:       key,
		})
		require.NoError(t, err)

		var savedEvent eventDynamo
		err = attributevalue.UnmarshalMap(out.Item, &savedEvent)
		require.NoError(t, err)

		actualID, err := uuid.Parse(savedEvent.ID)
		require.NoError(t, err)
		assert.Equal(t, event.ID, actualID)
		assert.Equal(t, event.Name, savedEvent.Name)
		assert.Equal(t, event.EventLocation, savedEvent.EventLocation)
		assert.WithinDuration(t, event.StartTime, savedEvent.StartTime, time.Second)
		assert.WithinDuration(t, event.EndTime, savedEvent.EndTime, time.Second)
		assert.WithinDuration(t, event.RegistrationCloseTime, savedEvent.RegistrationCloseTime, time.Second)
		assert.Equal(t, event.RegistrationOptions, savedEvent.RegistrationOptions)
		assert.Equal(t, event.AllowedTeamSizeRange, savedEvent.AllowedTeamSizeRange)
		assert.Equal(t, event.NumTeams, savedEvent.NumTeams)
		assert.Equal(t, event.NumRosteredPlayers, savedEvent.NumRosteredPlayers)
		assert.Equal(t, event.NumTotalPlayers, savedEvent.NumTotalPlayers)
		assert.Equal(t, event.RulesDocLink, savedEvent.RulesDocLink)
		assert.Equal(t, event.Version, savedEvent.Version)
	})
}

func TestGetEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully get an event", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:   uuid.New(),
			Name: "Test Event",
			EventLocation: events.Location{
				Name: "Test Location",
				LocAddress: events.Address{
					Street:     "123 Test St",
					City:       "Test City",
					State:      "TS",
					PostalCode: "12345",
					Country:    "Testland",
				},
			},
			StartTime:             time.Now().UTC().Truncate(time.Second),
			EndTime:               time.Now().Add(time.Hour).UTC().Truncate(time.Second),
			RegistrationCloseTime: time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second),
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL}, {RegType: events.BY_TEAM}},
			AllowedTeamSizeRange:  events.Range{Min: 3, Max: 5},
			NumTeams:              10,
			NumRosteredPlayers:    50,
			NumTotalPlayers:       60,
			RulesDocLink:          ptr.String("https://example.com/rules"),
			Version:               1,
		}
		require.NoError(t, db.CreateEvent(ctx, event))

		actual, err := db.GetEvent(ctx, event.ID)
		require.NoError(t, err)

		assert.Equal(t, event.ID, actual.ID)
		assert.Equal(t, event.Name, actual.Name)
		assert.Equal(t, event.EventLocation, actual.EventLocation)
		assert.WithinDuration(t, event.StartTime, actual.StartTime, time.Second)
		assert.WithinDuration(t, event.EndTime, actual.EndTime, time.Second)
		assert.WithinDuration(t, event.RegistrationCloseTime, actual.RegistrationCloseTime, time.Second)
		assert.Equal(t, event.RegistrationOptions, actual.RegistrationOptions)
		assert.Equal(t, event.AllowedTeamSizeRange, actual.AllowedTeamSizeRange)
		assert.Equal(t, event.NumTeams, actual.NumTeams)
		assert.Equal(t, event.NumRosteredPlayers, actual.NumRosteredPlayers)
		assert.Equal(t, event.NumTotalPlayers, actual.NumTotalPlayers)
		assert.Equal(t, event.RulesDocLink, actual.RulesDocLink)
		assert.Equal(t, event.Version, actual.Version)
	})

	t.Run("fail to get an event that does not exist", func(t *testing.T) {
		resetTable(ctx)

		_, err := db.GetEvent(ctx, uuid.New())
		require.Error(t, err)
		var eventError *events.Error
		require.ErrorAs(t, err, &eventError)
		assert.Equal(t, events.REASON_EVENT_DOES_NOT_EXIST, eventError.Reason)
	})
}

func TestGetEvents(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully get no events", func(t *testing.T) {
		resetTable(ctx)
		resp, err := db.GetEvents(ctx, 10, nil)
		require.NoError(t, err)
		assert.Empty(t, resp.Data)
		assert.False(t, resp.HasNextPage)
	})

	t.Run("successfully get a single event", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:   uuid.New(),
			Name: "Test Event",
			EventLocation: events.Location{
				Name: "Test Location",
				LocAddress: events.Address{
					Street:     "123 Test St",
					City:       "Test City",
					State:      "TS",
					PostalCode: "12345",
					Country:    "Testland",
				},
			},
			StartTime:             time.Now().UTC().Truncate(time.Second),
			EndTime:               time.Now().Add(time.Hour).UTC().Truncate(time.Second),
			RegistrationCloseTime: time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second),
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL}, {RegType: events.BY_TEAM}},
			AllowedTeamSizeRange:  events.Range{Min: 3, Max: 5},
			NumTeams:              10,
			NumRosteredPlayers:    50,
			NumTotalPlayers:       60,
			RulesDocLink:          ptr.String("https://example.com/rules"),
			Version:               1,
		}
		require.NoError(t, db.CreateEvent(ctx, event))

		resp, err := db.GetEvents(ctx, 10, nil)
		require.NoError(t, err)
		assert.Len(t, resp.Data, 1)
		assert.Equal(t, event.ID, resp.Data[0].ID)
		assert.False(t, resp.HasNextPage)
		assert.Equal(t, event.Version, resp.Data[0].Version)
	})

	t.Run("successfully get multiple events", func(t *testing.T) {
		resetTable(ctx)
		for i := range 5 {
			event := events.Event{
				ID:   uuid.New(),
				Name: fmt.Sprintf("Test Event %d", i),
				EventLocation: events.Location{
					Name: fmt.Sprintf("Test Location %d", i),
					LocAddress: events.Address{
						Street:     fmt.Sprintf("%d Test St", i),
						City:       "Test City",
						State:      "TS",
						PostalCode: "12345",
						Country:    "Testland",
					},
				},
				StartTime:             time.Now().Add(time.Duration(i) * time.Hour).UTC().Truncate(time.Second),
				EndTime:               time.Now().Add(time.Duration(i+1) * time.Hour).UTC().Truncate(time.Second),
				RegistrationCloseTime: time.Now().Add(time.Duration(i)*time.Hour + 30*time.Minute).UTC().Truncate(time.Second),
				RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL}, {RegType: events.BY_TEAM}},
				AllowedTeamSizeRange:  events.Range{Min: 3, Max: 5},
				NumTeams:              10 + i,
				NumRosteredPlayers:    50 + i,
				NumTotalPlayers:       60 + i,
				RulesDocLink:          ptr.String("https://example.com/rules"),
				Version:               1,
			}
			require.Nil(t, db.CreateEvent(ctx, event))
		}

		resp, err := db.GetEvents(ctx, 10, nil)
		require.NoError(t, err)
		assert.Len(t, resp.Data, 5)
		assert.False(t, resp.HasNextPage)
		for _, e := range resp.Data {
			assert.Equal(t, 1, e.Version)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		resetTable(ctx)
		for i := range 15 {
			event := events.Event{
				ID:   uuid.New(),
				Name: fmt.Sprintf("Test Event %d", i),
				EventLocation: events.Location{
					Name: fmt.Sprintf("Test Location %d", i),
					LocAddress: events.Address{
						Street:     fmt.Sprintf("%d Test St", i),
						City:       "Test City",
						State:      "TS",
						PostalCode: "12345",
						Country:    "Testland",
					},
				},
				StartTime:             time.Now().Add(time.Duration(i) * time.Hour).UTC().Truncate(time.Second),
				EndTime:               time.Now().Add(time.Duration(i+1) * time.Hour).UTC().Truncate(time.Second),
				RegistrationCloseTime: time.Now().Add(time.Duration(i)*time.Hour + 30*time.Minute).UTC().Truncate(time.Second),
				RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL}, {RegType: events.BY_TEAM}},
				AllowedTeamSizeRange:  events.Range{Min: 3, Max: 5},
				NumTeams:              10 + i,
				NumRosteredPlayers:    50 + i,
				NumTotalPlayers:       60 + i,
				RulesDocLink:          ptr.String("https://example.com/rules"),
				Version:               1,
			}
			require.Nil(t, db.CreateEvent(ctx, event))
		}

		// Get first page
		resp, err := db.GetEvents(ctx, 10, nil)
		require.NoError(t, err)
		assert.Len(t, resp.Data, 10)
		assert.True(t, resp.HasNextPage)
		for _, e := range resp.Data {
			assert.Equal(t, 1, e.Version)
		}

		// Get second page
		resp2, err := db.GetEvents(ctx, 10, resp.Cursor)
		require.NoError(t, err)
		assert.Len(t, resp2.Data, 5)
		assert.False(t, resp2.HasNextPage)
		for _, e := range resp2.Data {
			assert.Equal(t, 1, e.Version)
		}
	})
}

func TestUpdateEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully update an event", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:           uuid.New(),
			Name:         "Test Event",
			StartTime:    time.Now(),
			EndTime:      time.Now().Add(time.Hour),
			RulesDocLink: ptr.String("https://example.com/rules"),
			Version:      1,
		}
		require.NoError(t, db.CreateEvent(ctx, event))

		event.Name = "New name"
		event.Version++
		require.NoError(t, db.UpdateEvent(ctx, event))
	})

	t.Run("fail to update an event that does not exist", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:        uuid.New(),
			Name:      "Test Event",
			StartTime: time.Now(),
			EndTime:   time.Now().Add(time.Hour),
			Version:   1,
		}

		event.Version++
		eventErr := db.UpdateEvent(ctx, event)
		require.Error(t, eventErr)
		var eventError *events.Error
		require.ErrorAs(t, eventErr, &eventError)
		assert.Equal(t, events.REASON_EVENT_DOES_NOT_EXIST, eventError.Reason)
	})

	t.Run("successfully update an event and verify data", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:   uuid.New(),
			Name: "Test Event",
			EventLocation: events.Location{
				Name: "Test Location",
				LocAddress: events.Address{
					Street:     "123 Test St",
					City:       "Test City",
					State:      "TS",
					PostalCode: "12345",
					Country:    "Testland",
				},
			},
			StartTime:             time.Now().UTC().Truncate(time.Second),
			EndTime:               time.Now().Add(time.Hour).UTC().Truncate(time.Second),
			RegistrationCloseTime: time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second),
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL}, {RegType: events.BY_TEAM}},
			AllowedTeamSizeRange:  events.Range{Min: 3, Max: 5},
			NumTeams:              10,
			NumRosteredPlayers:    50,
			NumTotalPlayers:       60,
			RulesDocLink:          ptr.String("https://example.com/rules"),
			Version:               1,
		}
		require.NoError(t, db.CreateEvent(ctx, event))

		event.Name = "New name"
		event.StartTime = time.Now().Add(time.Hour).UTC().Truncate(time.Second)
		event.EndTime = time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
		event.RulesDocLink = ptr.String("https://example.com/new-rules")
		event.Version++
		require.NoError(t, db.UpdateEvent(ctx, event))

		dynamoEvent := newEventDynamo(event)
		key, err := attributevalue.MarshalMap(map[string]any{
			"PK": dynamoEvent.PK,
			"SK": dynamoEvent.SK,
		})
		require.NoError(t, err)

		out, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(tableName),
			Key:       key,
		})
		require.NoError(t, err)

		var savedEvent eventDynamo
		err = attributevalue.UnmarshalMap(out.Item, &savedEvent)
		require.NoError(t, err)

		actualID, err := uuid.Parse(savedEvent.ID)
		require.NoError(t, err)
		assert.Equal(t, event.ID, actualID)
		assert.Equal(t, event.Name, savedEvent.Name)
		assert.Equal(t, event.EventLocation, savedEvent.EventLocation)
		assert.WithinDuration(t, event.StartTime, savedEvent.StartTime, time.Second)
		assert.WithinDuration(t, event.EndTime, savedEvent.EndTime, time.Second)
		assert.WithinDuration(t, event.RegistrationCloseTime, savedEvent.RegistrationCloseTime, time.Second)
		assert.Equal(t, event.RegistrationOptions, savedEvent.RegistrationOptions)
		assert.Equal(t, event.AllowedTeamSizeRange, savedEvent.AllowedTeamSizeRange)
		assert.Equal(t, event.NumTeams, savedEvent.NumTeams)
		assert.Equal(t, event.NumRosteredPlayers, savedEvent.NumRosteredPlayers)
		assert.Equal(t, event.NumTotalPlayers, savedEvent.NumTotalPlayers)
		assert.Equal(t, event.RulesDocLink, savedEvent.RulesDocLink)
		assert.Equal(t, event.Version, savedEvent.Version)
	})
}
