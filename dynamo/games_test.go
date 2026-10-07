package dynamo

import (
	"context"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/games"
	"github.com/Rhymond/go-money"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testGame(eventID uuid.UUID, phase games.GamePhase, round, seq int) games.Game {
	a, b := uuid.New(), uuid.New()
	return games.Game{
		ID:      uuid.New(),
		EventID: eventID,
		Phase:   phase,
		Round:   round,
		Seq:     seq,
		Status:  games.GameStatusScheduled,
		SideA:   games.Side{TeamID: &a},
		SideB:   games.Side{TeamID: &b},
		Version: 1,
	}
}

func TestGameKeys(t *testing.T) {
	assert.Equal(t, "GAME#QUALIFYING#03#07", gameSK(games.GamePhaseQualifying, 3, 7))
	assert.Equal(t, "GAME#PLAYOFF#01#10", gameSK(games.GamePhasePlayoff, 1, 10))

	t.Run("lexical order equals schedule order within a phase", func(t *testing.T) {
		keys := []string{
			gameSK(games.GamePhaseQualifying, 1, 9),
			gameSK(games.GamePhaseQualifying, 1, 10),
			gameSK(games.GamePhaseQualifying, 2, 1),
			gameSK(games.GamePhaseQualifying, 10, 1),
		}
		for i := 1; i < len(keys); i++ {
			assert.Less(t, keys[i-1], keys[i])
		}
	})
}

func TestCreateAndGetGame(t *testing.T) {
	ctx := context.Background()

	t.Run("round trip preserves every field", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()
		nextID, prevA, prevB := uuid.New(), uuid.New(), uuid.New()
		nextSlot := games.GameSlotB
		forfeitSide := games.GameSlotA
		notes := "semifinal"
		start := time.Now().UTC().Truncate(time.Second)
		a, b := uuid.New(), uuid.New()

		game := games.Game{
			ID:           uuid.New(),
			EventID:      eventID,
			Phase:        games.GamePhasePlayoff,
			Round:        2,
			Seq:          3,
			Status:       games.GameStatusForfeit,
			SideA:        games.Side{TeamID: &a},
			SideB:        games.Side{TeamID: &b},
			ScoreA:       ptrInt(7),
			ScoreB:       ptrInt(2),
			ForfeitSide:  &forfeitSide,
			NextGameID:   &nextID,
			NextGameSlot: &nextSlot,
			PrevAID:      &prevA,
			PrevBID:      &prevB,
			Notes:        &notes,
			StartTime:    &start,
			Version:      1,
		}
		require.NoError(t, db.CreateGames(ctx, []games.Game{game}))

		got, err := db.GetGame(ctx, eventID, game.ID)
		require.NoError(t, err)
		assert.Equal(t, game.ID, got.ID)
		assert.Equal(t, game.EventID, got.EventID)
		assert.Equal(t, game.Phase, got.Phase)
		assert.Equal(t, game.Round, got.Round)
		assert.Equal(t, game.Seq, got.Seq)
		assert.Equal(t, game.Status, got.Status)
		assert.Equal(t, *game.SideA.TeamID, *got.SideA.TeamID)
		assert.Equal(t, *game.SideB.TeamID, *got.SideB.TeamID)
		assert.Equal(t, game.ScoreA, got.ScoreA)
		assert.Equal(t, game.ScoreB, got.ScoreB)
		assert.Equal(t, *game.ForfeitSide, *got.ForfeitSide)
		assert.Equal(t, *game.NextGameID, *got.NextGameID)
		assert.Equal(t, *game.NextGameSlot, *got.NextGameSlot)
		assert.Equal(t, *game.PrevAID, *got.PrevAID)
		assert.Equal(t, *game.PrevBID, *got.PrevBID)
		assert.Equal(t, *game.Notes, *got.Notes)
		assert.True(t, game.StartTime.Equal(*got.StartTime))
		assert.Equal(t, 1, got.Version)
	})

	t.Run("stored sort key is zero-padded", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()
		game := testGame(eventID, games.GamePhaseQualifying, 3, 7)
		require.NoError(t, db.CreateGames(ctx, []games.Game{game}))

		key, err := attributevalue.MarshalMap(map[string]any{
			"PK": gamePK(eventID),
			"SK": gameSK(games.GamePhaseQualifying, 3, 7),
		})
		require.NoError(t, err)
		out, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(tableName),
			Key:       key,
		})
		require.NoError(t, err)
		require.NotEmpty(t, out.Item)
	})

	t.Run("get missing game returns not-exist", func(t *testing.T) {
		resetTable(ctx)
		_, err := db.GetGame(ctx, uuid.New(), uuid.New())
		require.Error(t, err)
		var gameErr *games.Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, games.REASON_GAME_DOES_NOT_EXIST, gameErr.Reason)
	})
}

func TestListGamesForEventOrdering(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	eventID := uuid.New()
	seeds := []games.Game{
		testGame(eventID, games.GamePhasePlayoff, 1, 1),
		testGame(eventID, games.GamePhaseQualifying, 2, 1),
		testGame(eventID, games.GamePhaseQualifying, 1, 2),
		testGame(eventID, games.GamePhaseQualifying, 1, 1),
		testGame(eventID, games.GamePhasePlayoff, 1, 2),
	}
	require.NoError(t, db.CreateGames(ctx, seeds))

	got, err := db.ListGamesForEvent(ctx, eventID)
	require.NoError(t, err)
	require.Len(t, got, 5)

	want := []struct {
		phase games.GamePhase
		round int
		seq   int
	}{
		{games.GamePhaseQualifying, 1, 1},
		{games.GamePhaseQualifying, 1, 2},
		{games.GamePhaseQualifying, 2, 1},
		{games.GamePhasePlayoff, 1, 1},
		{games.GamePhasePlayoff, 1, 2},
	}
	for i, w := range want {
		assert.Equal(t, w.phase, got[i].Phase, "index %d phase", i)
		assert.Equal(t, w.round, got[i].Round, "index %d round", i)
		assert.Equal(t, w.seq, got[i].Seq, "index %d seq", i)
	}
}

func TestCreateGamesBatches(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	eventID := uuid.New()
	var batch []games.Game
	for i := 0; i < 30; i++ {
		g := testGame(eventID, games.GamePhaseQualifying, i/5+1, i%5+1)
		batch = append(batch, g)
	}
	require.NoError(t, db.CreateGames(ctx, batch))

	got, err := db.ListGamesForEvent(ctx, eventID)
	require.NoError(t, err)
	assert.Len(t, got, 30)
}

func TestUpdateGame(t *testing.T) {
	ctx := context.Background()

	t.Run("update persists with version check", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()
		game := testGame(eventID, games.GamePhaseQualifying, 1, 1)
		require.NoError(t, db.CreateGames(ctx, []games.Game{game}))

		game.Status = games.GameStatusCompleted
		game.ScoreA, game.ScoreB = ptrInt(9), ptrInt(4)
		game.Version++
		require.NoError(t, db.UpdateGame(ctx, game))

		got, err := db.GetGame(ctx, eventID, game.ID)
		require.NoError(t, err)
		assert.Equal(t, games.GameStatusCompleted, got.Status)
		assert.Equal(t, ptrInt(9), got.ScoreA)
		assert.Equal(t, 2, got.Version)
	})

	t.Run("update missing game returns not-exist", func(t *testing.T) {
		resetTable(ctx)
		game := testGame(uuid.New(), games.GamePhaseQualifying, 1, 1)
		game.Version = 2
		err := db.UpdateGame(ctx, game)
		require.Error(t, err)
		var gameErr *games.Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, games.REASON_GAME_DOES_NOT_EXIST, gameErr.Reason)
	})

	t.Run("stale version returns not-exist", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()
		game := testGame(eventID, games.GamePhaseQualifying, 1, 1)
		require.NoError(t, db.CreateGames(ctx, []games.Game{game}))

		// Stored version is 1; writing version 3 skips the expected v-1 == 1
		// precondition, so the conditional write fails like a missing row.
		game.Status = games.GameStatusCancelled
		game.Version = 3
		err := db.UpdateGame(ctx, game)
		require.Error(t, err)
		var gameErr *games.Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, games.REASON_GAME_DOES_NOT_EXIST, gameErr.Reason)
	})
}

func TestDeleteGames(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	eventID := uuid.New()
	keep := testGame(eventID, games.GamePhaseQualifying, 1, 1)
	drop1 := testGame(eventID, games.GamePhaseQualifying, 1, 2)
	drop2 := testGame(eventID, games.GamePhaseQualifying, 2, 1)
	require.NoError(t, db.CreateGames(ctx, []games.Game{keep, drop1, drop2}))

	require.NoError(t, db.DeleteGames(ctx, eventID, []uuid.UUID{drop1.ID, drop2.ID}))

	got, err := db.ListGamesForEvent(ctx, eventID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, keep.ID, got[0].ID)
}

func TestDeleteGamesUnknownIDsAreNoOps(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	eventID := uuid.New()
	keep := testGame(eventID, games.GamePhaseQualifying, 1, 1)
	require.NoError(t, db.CreateGames(ctx, []games.Game{keep}))

	require.NoError(t, db.DeleteGames(ctx, eventID, []uuid.UUID{uuid.New()}))

	got, err := db.ListGamesForEvent(ctx, eventID)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestCreateGamesOverwriteIsLastWriteWins(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	eventID := uuid.New()
	game := testGame(eventID, games.GamePhaseQualifying, 1, 1)
	require.NoError(t, db.CreateGames(ctx, []games.Game{game}))

	// Unconditional puts: the service precondition (generate refuses when
	// games exist; replace deletes first) keeps this unreachable via API.
	game.Status = games.GameStatusCancelled
	require.NoError(t, db.CreateGames(ctx, []games.Game{game}))

	got, err := db.GetGame(ctx, eventID, game.ID)
	require.NoError(t, err)
	assert.Equal(t, games.GameStatusCancelled, got.Status)
}

func TestGamesDoNotPolluteEventsListing(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	event := events.Event{
		ID:                    uuid.New(),
		Name:                  "GSI Guard Event",
		TimeZone:              time.UTC,
		StartTime:             time.Now().UTC(),
		EndTime:               time.Now().Add(time.Hour).UTC(),
		RegistrationCloseTime: time.Now().Add(-time.Hour).UTC(),
		RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_TEAM, Price: money.New(1000, "USD")}},
		Version:               1,
		Status:                events.EventStatusOpened,
	}
	require.NoError(t, db.CreateEvent(ctx, event))
	require.NoError(t, db.CreateGames(ctx, []games.Game{
		testGame(event.ID, games.GamePhaseQualifying, 1, 1),
		testGame(event.ID, games.GamePhasePlayoff, 1, 1),
	}))

	// Game items must carry no GSI attributes at all; check one item per
	// phase since keys differ by phase prefix.
	for _, sk := range []string{
		gameSK(games.GamePhaseQualifying, 1, 1),
		gameSK(games.GamePhasePlayoff, 1, 1),
	} {
		key, err := attributevalue.MarshalMap(map[string]any{
			"PK": gamePK(event.ID),
			"SK": sk,
		})
		require.NoError(t, err)
		out, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(tableName),
			Key:       key,
		})
		require.NoError(t, err)
		require.NotEmpty(t, out.Item)
		for _, attr := range []string{"GSI1PK", "GSI1SK"} {
			_, present := out.Item[attr]
			assert.False(t, present, "game item must not carry %s", attr)
		}
	}

	// The events listing still returns exactly the event master.
	resp, err := db.GetEvents(ctx, 10, nil)
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, event.ID, resp.Data[0].ID)
	assert.Equal(t, "GSI Guard Event", resp.Data[0].Name)
}

func ptrInt(i int) *int {
	return &i
}

func TestUpdateGameChecked(t *testing.T) {
	ctx := context.Background()

	newOpenEvent := func() events.Event {
		return events.Event{
			ID:                    uuid.New(),
			Name:                  "Checked Event",
			TimeZone:              time.UTC,
			StartTime:             time.Now().UTC(),
			EndTime:               time.Now().Add(time.Hour).UTC(),
			RegistrationCloseTime: time.Now().Add(-time.Hour).UTC(),
			RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_TEAM, Price: money.New(1000, "USD")}},
			Version:               1,
			Status:                events.EventStatusInProgress,
		}
	}

	t.Run("writes when event not finalized", func(t *testing.T) {
		resetTable(ctx)
		event := newOpenEvent()
		require.NoError(t, db.CreateEvent(ctx, event))
		game := testGame(event.ID, games.GamePhaseQualifying, 1, 1)
		require.NoError(t, db.CreateGames(ctx, []games.Game{game}))

		game.Status = games.GameStatusCancelled
		game.Version++
		require.NoError(t, db.UpdateGameChecked(ctx, event.ID, game))

		got, err := db.GetGame(ctx, event.ID, game.ID)
		require.NoError(t, err)
		assert.Equal(t, games.GameStatusCancelled, got.Status)
	})

	t.Run("refuses when event finalized", func(t *testing.T) {
		resetTable(ctx)
		event := newOpenEvent()
		event.Status = events.EventStatusFinalized
		require.NoError(t, db.CreateEvent(ctx, event))
		game := testGame(event.ID, games.GamePhaseQualifying, 1, 1)
		require.NoError(t, db.CreateGames(ctx, []games.Game{game}))

		game.Status = games.GameStatusCancelled
		game.Version++
		err := db.UpdateGameChecked(ctx, event.ID, game)
		require.Error(t, err)
		var gameErr *games.Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, games.REASON_EVENT_FINALIZED, gameErr.Reason)
	})

	t.Run("version conflict surfaces as not-exist", func(t *testing.T) {
		resetTable(ctx)
		event := newOpenEvent()
		require.NoError(t, db.CreateEvent(ctx, event))
		game := testGame(event.ID, games.GamePhaseQualifying, 1, 1)
		require.NoError(t, db.CreateGames(ctx, []games.Game{game}))

		game.Status = games.GameStatusCancelled
		game.Version = 99
		err := db.UpdateGameChecked(ctx, event.ID, game)
		require.Error(t, err)
		var gameErr *games.Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, games.REASON_GAME_DOES_NOT_EXIST, gameErr.Reason)
	})
}
