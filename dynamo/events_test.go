package dynamo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
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
			ID:            uuid.New(),
			Name:          "Test Event",
			EventDateTime: time.Now(),
		}

		require.NoError(t, db.CreateEvent(ctx, event))
	})

	t.Run("fail to create an event that already exists", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:            uuid.New(),
			Name:          "Test Event",
			EventDateTime: time.Now(),
		}

		require.NoError(t, db.CreateEvent(ctx, event))

		eventErr := db.CreateEvent(ctx, event)
		require.Error(t, eventErr)
		var eventError *events.EventError
		require.ErrorAs(t, eventErr, &eventError)
		assert.Equal(t, events.REASON_EVENT_ALREADY_EXISTS, eventError.Reason)
	})

	t.Run("successfully create an event and verify data", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:            uuid.New(),
			Name:          "Test Event",
			EventDateTime: time.Now().UTC().Truncate(time.Second),
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

		assert.Equal(t, event.ID, savedEvent.ID)
		assert.Equal(t, event.Name, savedEvent.Name)
		assert.WithinDuration(t, event.EventDateTime, savedEvent.EventDateTime, time.Second)
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
			ID:            uuid.New(),
			Name:          "Test Event",
			EventDateTime: time.Now(),
		}
		require.NoError(t, db.CreateEvent(ctx, event))

		resp, err := db.GetEvents(ctx, 10, nil)
		require.NoError(t, err)
		assert.Len(t, resp.Data, 1)
		assert.Equal(t, event.ID, resp.Data[0].ID)
		assert.False(t, resp.HasNextPage)
	})

	t.Run("successfully get multiple events", func(t *testing.T) {
		resetTable(ctx)
		for i := range 5 {
			event := events.Event{
				ID:            uuid.New(),
				Name:          fmt.Sprintf("Test Event %d", i),
				EventDateTime: time.Now().Add(time.Duration(i) * time.Hour),
			}
			require.Nil(t, db.CreateEvent(ctx, event))
		}

		resp, err := db.GetEvents(ctx, 10, nil)
		require.NoError(t, err)
		assert.Len(t, resp.Data, 5)
		assert.False(t, resp.HasNextPage)
	})

	t.Run("pagination", func(t *testing.T) {
		resetTable(ctx)
		for i := range 15 {
			event := events.Event{
				ID:            uuid.New(),
				Name:          fmt.Sprintf("Test Event %d", i),
				EventDateTime: time.Now().Add(time.Duration(i) * time.Hour),
			}
			require.Nil(t, db.CreateEvent(ctx, event))
		}

		// Get first page
		resp, err := db.GetEvents(ctx, 10, nil)
		require.NoError(t, err)
		assert.Len(t, resp.Data, 10)
		assert.True(t, resp.HasNextPage)

		// Get second page
		resp2, err := db.GetEvents(ctx, 10, resp.Cursor)
		require.NoError(t, err)
		assert.Len(t, resp2.Data, 5)
		assert.False(t, resp2.HasNextPage)
	})
}

func TestUpdateEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully update an event", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:            uuid.New(),
			Name:          "Test Event",
			EventDateTime: time.Now(),
		}
		require.NoError(t, db.CreateEvent(ctx, event))

		event.Name = "New name"
		require.NoError(t, db.UpdateEvent(ctx, event))
	})

	t.Run("fail to update an event that does not exist", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:            uuid.New(),
			Name:          "Test Event",
			EventDateTime: time.Now(),
		}

		eventErr := db.UpdateEvent(ctx, event)
		require.Error(t, eventErr)
		var eventError *events.EventError
		require.ErrorAs(t, eventErr, &eventError)
		assert.Equal(t, events.REASON_EVENT_DOES_NOT_EXIST, eventError.Reason)
	})

	t.Run("successfully update an event and verify data", func(t *testing.T) {
		resetTable(ctx)
		event := events.Event{
			ID:            uuid.New(),
			Name:          "Test Event",
			EventDateTime: time.Now().UTC().Truncate(time.Second),
		}
		require.NoError(t, db.CreateEvent(ctx, event))

		event.Name = "New name"
		event.EventDateTime = time.Now().Add(time.Hour).UTC().Truncate(time.Second)
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

		assert.Equal(t, event.ID, savedEvent.ID)
		assert.Equal(t, event.Name, savedEvent.Name)
		assert.WithinDuration(t, event.EventDateTime, savedEvent.EventDateTime, time.Second)
	})
}