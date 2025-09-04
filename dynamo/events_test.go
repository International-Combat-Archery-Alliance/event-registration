package dynamo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/ptr"
	"github.com/International-Combat-Archery-Alliance/event-registration/slices"
	"github.com/Rhymond/go-money"
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
		tz, _ := time.LoadLocation("America/New_York")
		event := events.Event{
			ID:           uuid.New(),
			Name:         "Test Event",
			TimeZone:     tz,
			StartTime:    time.Now(),
			EndTime:      time.Now().Add(time.Hour),
			RulesDocLink: ptr.String("https://example.com/rules"),
			Version:      1,
		}

		require.NoError(t, db.CreateEvent(ctx, event))
	})

	t.Run("fail to create an event that already exists", func(t *testing.T) {
		resetTable(ctx)
		tz, _ := time.LoadLocation("Europe/Paris")
		event := events.Event{
			ID:        uuid.New(),
			Name:      "Test Event",
			TimeZone:  tz,
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
		tz, _ := time.LoadLocation("America/Chicago")
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
			TimeZone:              tz,
			StartTime:             time.Now().UTC().Truncate(time.Second),
			EndTime:               time.Now().Add(time.Hour).UTC().Truncate(time.Second),
			RegistrationCloseTime: time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second),
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(5000, "USD")}, {RegType: events.BY_TEAM, Price: money.New(4000, "USD")}},
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
		assert.Equal(t, event.RegistrationOptions, slices.Map(savedEvent.RegistrationOptions, func(o eventRegistrationOptionDynamo) events.EventRegistrationOption {
			return dynamoEventRegOptionToEventRegOption(o)
		}))
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
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(1000, "USD")}, {RegType: events.BY_TEAM, Price: money.New(2500, "USD")}},
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
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(1500, "USD")}, {RegType: events.BY_TEAM, Price: money.New(2500, "EUR")}},
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
				RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(3000, "EUR")}, {RegType: events.BY_TEAM, Price: money.New(3500, "EUR")}},
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
				RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(1200, "USD")}, {RegType: events.BY_TEAM, Price: money.New(1750, "USD")}},
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
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(7500, "USD")}, {RegType: events.BY_TEAM, Price: money.New(10000, "USD")}},
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
		assert.Equal(t, event.RegistrationOptions, slices.Map(savedEvent.RegistrationOptions, func(o eventRegistrationOptionDynamo) events.EventRegistrationOption {
			return dynamoEventRegOptionToEventRegOption(o)
		}))
		assert.Equal(t, event.AllowedTeamSizeRange, savedEvent.AllowedTeamSizeRange)
		assert.Equal(t, event.NumTeams, savedEvent.NumTeams)
		assert.Equal(t, event.NumRosteredPlayers, savedEvent.NumRosteredPlayers)
		assert.Equal(t, event.NumTotalPlayers, savedEvent.NumTotalPlayers)
		assert.Equal(t, event.RulesDocLink, savedEvent.RulesDocLink)
		assert.Equal(t, event.Version, savedEvent.Version)
	})
}

func TestEventMoneyPriceSavingAndFetching(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully save and fetch event with Money price in registration options", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:   uuid.New(),
			Name: "Test Event with Pricing",
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
			RegistrationOptions: []events.EventRegistrationOption{
				{RegType: events.BY_INDIVIDUAL, Price: money.New(5000, "USD")},
				{RegType: events.BY_TEAM, Price: money.New(15000, "USD")},
			},
			AllowedTeamSizeRange: events.Range{Min: 3, Max: 5},
			NumTeams:             10,
			NumRosteredPlayers:   50,
			NumTotalPlayers:      60,
			RulesDocLink:         ptr.String("https://example.com/rules"),
			Version:              1,
		}

		require.NoError(t, db.CreateEvent(ctx, event))

		retrieved, err := db.GetEvent(ctx, event.ID)
		require.NoError(t, err)

		require.Len(t, retrieved.RegistrationOptions, 2)

		// Test individual registration price
		assert.Equal(t, events.BY_INDIVIDUAL, retrieved.RegistrationOptions[0].RegType)
		require.NotNil(t, retrieved.RegistrationOptions[0].Price)
		assert.Equal(t, int64(5000), retrieved.RegistrationOptions[0].Price.Amount())
		assert.Equal(t, "USD", retrieved.RegistrationOptions[0].Price.Currency().Code)

		// Test team registration price
		assert.Equal(t, events.BY_TEAM, retrieved.RegistrationOptions[1].RegType)
		require.NotNil(t, retrieved.RegistrationOptions[1].Price)
		assert.Equal(t, int64(15000), retrieved.RegistrationOptions[1].Price.Amount())
		assert.Equal(t, "USD", retrieved.RegistrationOptions[1].Price.Currency().Code)
	})

	t.Run("successfully save and fetch event with different currencies", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:   uuid.New(),
			Name: "Multi-Currency Event",
			EventLocation: events.Location{
				Name: "International Location",
			},
			StartTime:             time.Now().UTC().Truncate(time.Second),
			EndTime:               time.Now().Add(time.Hour).UTC().Truncate(time.Second),
			RegistrationCloseTime: time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second),
			RegistrationOptions: []events.EventRegistrationOption{
				{RegType: events.BY_INDIVIDUAL, Price: money.New(2500, "EUR")},
				{RegType: events.BY_TEAM, Price: money.New(750000, "JPY")},
			},
			Version: 1,
		}

		require.NoError(t, db.CreateEvent(ctx, event))

		retrieved, err := db.GetEvent(ctx, event.ID)
		require.NoError(t, err)

		require.Len(t, retrieved.RegistrationOptions, 2)

		// Test EUR pricing
		assert.Equal(t, int64(2500), retrieved.RegistrationOptions[0].Price.Amount())
		assert.Equal(t, "EUR", retrieved.RegistrationOptions[0].Price.Currency().Code)

		// Test JPY pricing
		assert.Equal(t, int64(750000), retrieved.RegistrationOptions[1].Price.Amount())
		assert.Equal(t, "JPY", retrieved.RegistrationOptions[1].Price.Currency().Code)
	})

	t.Run("successfully update event with changed prices", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:   uuid.New(),
			Name: "Price Update Event",
			EventLocation: events.Location{
				Name: "Test Location",
			},
			StartTime:             time.Now().UTC().Truncate(time.Second),
			EndTime:               time.Now().Add(time.Hour).UTC().Truncate(time.Second),
			RegistrationCloseTime: time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second),
			RegistrationOptions: []events.EventRegistrationOption{
				{RegType: events.BY_INDIVIDUAL, Price: money.New(5000, "USD")},
			},
			Version: 1,
		}

		require.NoError(t, db.CreateEvent(ctx, event))

		// Update the price
		event.RegistrationOptions[0].Price = money.New(7500, "USD")
		event.Version++
		require.NoError(t, db.UpdateEvent(ctx, event))

		retrieved, err := db.GetEvent(ctx, event.ID)
		require.NoError(t, err)

		require.Len(t, retrieved.RegistrationOptions, 1)
		require.NotNil(t, retrieved.RegistrationOptions[0].Price)
		assert.Equal(t, int64(7500), retrieved.RegistrationOptions[0].Price.Amount())
		assert.Equal(t, "USD", retrieved.RegistrationOptions[0].Price.Currency().Code)
		assert.Equal(t, 2, retrieved.Version)
	})

	t.Run("verify direct dynamo storage of Money fields", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:   uuid.New(),
			Name: "Direct Storage Test",
			EventLocation: events.Location{
				Name: "Test Location",
			},
			StartTime:             time.Now().UTC().Truncate(time.Second),
			EndTime:               time.Now().Add(time.Hour).UTC().Truncate(time.Second),
			RegistrationCloseTime: time.Now().Add(30 * time.Minute).UTC().Truncate(time.Second),
			RegistrationOptions: []events.EventRegistrationOption{
				{RegType: events.BY_INDIVIDUAL, Price: money.New(12345, "GBP")},
			},
			Version: 1,
		}

		require.NoError(t, db.CreateEvent(ctx, event))

		// Verify data was saved correctly by checking raw DynamoDB item
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

		require.Len(t, savedEvent.RegistrationOptions, 1)
		assert.Equal(t, int64(12345), savedEvent.RegistrationOptions[0].PriceAmount)
		assert.Equal(t, "GBP", savedEvent.RegistrationOptions[0].PriceCurrency)
	})
}

func TestTimeZoneStorage(t *testing.T) {
	ctx := context.Background()

	t.Run("event with timezone stored and retrieved correctly", func(t *testing.T) {
		resetTable(ctx)
		tz, _ := time.LoadLocation("America/New_York")
		baseTime := time.Date(2024, 12, 15, 10, 0, 0, 0, time.UTC)

		event := events.Event{
			ID:   uuid.New(),
			Name: "Timezone Test Event",
			EventLocation: events.Location{
				Name: "NYC Venue",
				LocAddress: events.Address{
					Street:     "123 Broadway",
					City:       "New York",
					State:      "NY",
					PostalCode: "10001",
					Country:    "USA",
				},
			},
			TimeZone:              tz,
			StartTime:             baseTime.In(tz),
			EndTime:               baseTime.Add(time.Hour).In(tz),
			RegistrationCloseTime: baseTime.Add(-time.Hour).In(tz),
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(5000, "USD")}},
			AllowedTeamSizeRange:  events.Range{Min: 1, Max: 5},
			Version:               1,
		}

		require.NoError(t, db.CreateEvent(ctx, event))

		// Retrieve and verify
		retrieved, err := db.GetEvent(ctx, event.ID)
		require.NoError(t, err)

		assert.Equal(t, event.ID, retrieved.ID)
		assert.Equal(t, event.Name, retrieved.Name)
		assert.Equal(t, "America/New_York", retrieved.TimeZone.String())

		// Times should be in the timezone when retrieved
		assert.Equal(t, "EST", retrieved.StartTime.Format("MST"))
		assert.Equal(t, "EST", retrieved.EndTime.Format("MST"))
		assert.Equal(t, "EST", retrieved.RegistrationCloseTime.Format("MST"))

		// But the actual times should be equivalent
		assert.True(t, event.StartTime.Equal(retrieved.StartTime))
		assert.True(t, event.EndTime.Equal(retrieved.EndTime))
		assert.True(t, event.RegistrationCloseTime.Equal(retrieved.RegistrationCloseTime))
	})

	t.Run("event without timezone uses UTC", func(t *testing.T) {
		resetTable(ctx)
		baseTime := time.Date(2024, 12, 15, 10, 0, 0, 0, time.UTC)

		event := events.Event{
			ID:                    uuid.New(),
			Name:                  "UTC Event",
			TimeZone:              time.UTC,
			StartTime:             baseTime,
			EndTime:               baseTime.Add(time.Hour),
			RegistrationCloseTime: baseTime.Add(-time.Hour),
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(3000, "USD")}},
			Version:               1,
		}

		require.NoError(t, db.CreateEvent(ctx, event))

		retrieved, err := db.GetEvent(ctx, event.ID)
		require.NoError(t, err)

		assert.Equal(t, "UTC", retrieved.TimeZone.String())
		assert.Equal(t, "UTC", retrieved.StartTime.Format("MST"))
	})

	t.Run("timezone persists through updates", func(t *testing.T) {
		resetTable(ctx)
		tz, _ := time.LoadLocation("Europe/London")
		baseTime := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC) // Summer time

		event := events.Event{
			ID:                    uuid.New(),
			Name:                  "London Event",
			TimeZone:              tz,
			StartTime:             baseTime.In(tz),
			EndTime:               baseTime.Add(time.Hour).In(tz),
			RegistrationCloseTime: baseTime.Add(-time.Hour).In(tz),
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(4000, "GBP")}},
			Version:               1,
		}

		require.NoError(t, db.CreateEvent(ctx, event))

		// Update the event
		event.Name = "Updated London Event"
		event.Version = 2
		require.NoError(t, db.UpdateEvent(ctx, event))

		// Retrieve and verify timezone is preserved
		retrieved, err := db.GetEvent(ctx, event.ID)
		require.NoError(t, err)

		assert.Equal(t, "Europe/London", retrieved.TimeZone.String())
		assert.Equal(t, "Updated London Event", retrieved.Name)
		assert.Equal(t, 2, retrieved.Version)
		// Should show BST (British Summer Time) since it's June
		assert.Equal(t, "BST", retrieved.StartTime.Format("MST"))
	})

	t.Run("times stored as UTC in database", func(t *testing.T) {
		resetTable(ctx)
		tz, _ := time.LoadLocation("Asia/Tokyo")
		// 3 PM Tokyo time = 6 AM UTC
		tokyoTime := time.Date(2024, 12, 15, 15, 0, 0, 0, tz)

		event := events.Event{
			ID:                    uuid.New(),
			Name:                  "Tokyo Event",
			TimeZone:              tz,
			StartTime:             tokyoTime,
			EndTime:               tokyoTime.Add(2 * time.Hour),
			RegistrationCloseTime: tokyoTime.Add(-time.Hour),
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(8000, "JPY")}},
			Version:               1,
		}

		require.NoError(t, db.CreateEvent(ctx, event))

		// Check raw database storage
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

		// Verify times are stored as UTC (6 AM)
		assert.Equal(t, 6, savedEvent.StartTime.Hour())
		assert.Equal(t, "UTC", savedEvent.StartTime.Location().String())
		assert.Equal(t, 8, savedEvent.EndTime.Hour()) // 3 PM + 2 hours = 5 PM Tokyo = 8 AM UTC
		assert.Equal(t, "UTC", savedEvent.EndTime.Location().String())

		// But when retrieved through the service, should be in Tokyo time
		retrieved, err := db.GetEvent(ctx, event.ID)
		require.NoError(t, err)

		assert.Equal(t, "Asia/Tokyo", retrieved.TimeZone.String())
		assert.Equal(t, 15, retrieved.StartTime.Hour()) // 3 PM Tokyo
		assert.Equal(t, "JST", retrieved.StartTime.Format("MST"))
		assert.Equal(t, 17, retrieved.EndTime.Hour()) // 5 PM Tokyo
	})

	t.Run("different timezones in multiple events", func(t *testing.T) {
		resetTable(ctx)
		baseTime := time.Date(2024, 12, 15, 12, 0, 0, 0, time.UTC)

		timezones := []string{
			"America/Los_Angeles",
			"America/New_York",
			"Europe/London",
			"Asia/Tokyo",
			"Australia/Sydney",
		}

		testEvents := make([]events.Event, len(timezones))
		for i, tzName := range timezones {
			tz, _ := time.LoadLocation(tzName)
			testEvents[i] = events.Event{
				ID:                    uuid.New(),
				Name:                  fmt.Sprintf("Event %d", i),
				TimeZone:              tz,
				StartTime:             baseTime.In(tz),
				EndTime:               baseTime.Add(time.Hour).In(tz),
				RegistrationCloseTime: baseTime.Add(-time.Hour).In(tz),
				RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(5000, "USD")}},
				Version:               1,
			}
			require.NoError(t, db.CreateEvent(ctx, testEvents[i]))
		}

		// Retrieve all events and verify timezones
		for i, event := range testEvents {
			retrieved, err := db.GetEvent(ctx, event.ID)
			require.NoError(t, err)

			assert.Equal(t, timezones[i], retrieved.TimeZone.String())
			assert.Equal(t, fmt.Sprintf("Event %d", i), retrieved.Name)

			// All events should have the same UTC time but different local representations
			assert.True(t, event.StartTime.Equal(retrieved.StartTime))
		}
	})
}
