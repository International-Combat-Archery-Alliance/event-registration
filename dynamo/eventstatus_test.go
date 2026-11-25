package dynamo

import (
	"context"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/Rhymond/go-money"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventStatusRoundTrip(t *testing.T) {
	ctx := context.Background()

	t.Run("status persists through create and get", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:                    uuid.New(),
			Name:                  "Status Event",
			TimeZone:              time.UTC,
			StartTime:             time.Now().UTC().Truncate(time.Second),
			EndTime:               time.Now().Add(time.Hour).UTC().Truncate(time.Second),
			RegistrationCloseTime: time.Now().UTC().Truncate(time.Second),
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(1000, "USD")}},
			Version:               1,
			Status:                events.EventStatusInProgress,
		}
		require.NoError(t, db.CreateEvent(ctx, event))

		got, err := db.GetEvent(ctx, event.ID)
		require.NoError(t, err)
		assert.Equal(t, events.EventStatusInProgress, got.Status)
	})

	t.Run("empty status defaults to OPENED on write and read", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:                    uuid.New(),
			Name:                  "Default Status Event",
			TimeZone:              time.UTC,
			StartTime:             time.Now().UTC().Truncate(time.Second),
			EndTime:               time.Now().Add(time.Hour).UTC().Truncate(time.Second),
			RegistrationCloseTime: time.Now().UTC().Truncate(time.Second),
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(1000, "USD")}},
			Version:               1,
		}
		require.NoError(t, db.CreateEvent(ctx, event))

		got, err := db.GetEvent(ctx, event.ID)
		require.NoError(t, err)
		assert.Equal(t, events.EventStatusOpened, got.Status)
	})

	t.Run("pre-status row without attribute reads as OPENED", func(t *testing.T) {
		resetTable(ctx)
		id := uuid.New()
		// Write a legacy item with no Status attribute at all.
		legacy := map[string]any{
			"PK":                    eventPK(id),
			"SK":                    eventSK(id),
			"GSI1PK":                eventEntityName,
			"GSI1SK":                "EVENT#2024-01-01T00:00:00Z#" + id.String(),
			"ID":                    id.String(),
			"Version":               1,
			"Name":                  "Legacy Event",
			"EventLocation":         map[string]any{"Name": "V", "LocAddress": map[string]any{}},
			"StartTime":             time.Now().UTC(),
			"EndTime":               time.Now().Add(time.Hour).UTC(),
			"RegistrationCloseTime": time.Now().UTC(),
			"RegistrationOptions":   []any{},
			"AllowedTeamSizeRange":  map[string]any{"Min": 1, "Max": 5},
		}
		item, err := attributevalue.MarshalMap(legacy)
		require.NoError(t, err)
		_, err = dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(tableName),
			Item:      item,
		})
		require.NoError(t, err)

		got, err := db.GetEvent(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, events.EventStatusOpened, got.Status)
	})

	t.Run("status persists through update", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:                    uuid.New(),
			Name:                  "Update Status Event",
			TimeZone:              time.UTC,
			StartTime:             time.Now().UTC().Truncate(time.Second),
			EndTime:               time.Now().Add(time.Hour).UTC().Truncate(time.Second),
			RegistrationCloseTime: time.Now().UTC().Truncate(time.Second),
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_INDIVIDUAL, Price: money.New(1000, "USD")}},
			Version:               1,
			Status:                events.EventStatusOpened,
		}
		require.NoError(t, db.CreateEvent(ctx, event))

		event.Status = events.EventStatusFinalized
		event.Version++
		require.NoError(t, db.UpdateEvent(ctx, event))

		got, err := db.GetEvent(ctx, event.ID)
		require.NoError(t, err)
		assert.Equal(t, events.EventStatusFinalized, got.Status)
		assert.Equal(t, 2, got.Version)
	})
}
