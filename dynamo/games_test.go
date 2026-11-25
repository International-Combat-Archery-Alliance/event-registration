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

func testGame(eventID uuid.UUID, gameNo int, phase games.GamePhase, round, seq int) games.Game {
	a, b := uuid.New(), uuid.New()
	return games.Game{
		ID:      uuid.New(),
		EventID: eventID,
		GameNo:  gameNo,
		Phase:   phase,
		Round:   round,
		Seq:     seq,
		Status:  games.GameStatusScheduled,
		SideA:   games.Side{TeamID: &a},
		SideB:   games.Side{TeamID: &b},
		Version: 1,
	}
}

func testGameEvent(eventID uuid.UUID, status events.EventStatus) events.Event {
	return events.Event{
		ID:                    eventID,
		Name:                  "Games Test Event",
		TimeZone:              time.UTC,
		StartTime:             time.Now().UTC(),
		EndTime:               time.Now().Add(time.Hour).UTC(),
		RegistrationCloseTime: time.Now().Add(-time.Hour).UTC(),
		RegistrationOptions:   []events.EventRegistrationOption{{RegType: events.BY_TEAM, Price: money.New(1000, "USD")}},
		Version:               1,
		Status:                status,
	}
}

func seedGameEvent(t *testing.T, ctx context.Context, eventID uuid.UUID, status events.EventStatus) {
	t.Helper()
	require.NoError(t, db.CreateEvent(ctx, testGameEvent(eventID, status)))
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
		seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)
		nextID, prevA, prevB := uuid.New(), uuid.New(), uuid.New()
		nextSlot := games.GameSlotB
		forfeitSide := games.GameSlotA
		notes := "semifinal"
		start := time.Now().UTC().Truncate(time.Second)
		a, b := uuid.New(), uuid.New()

		game := games.Game{
			ID:           uuid.New(),
			EventID:      eventID,
			GameNo:       7,
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
		require.NoError(t, db.CreateGames(ctx, eventID, 1, []games.Game{game}))

		got, err := db.GetGameByNumber(ctx, eventID, game.GameNo)
		require.NoError(t, err)
		assert.Equal(t, game.ID, got.ID)
		assert.Equal(t, game.EventID, got.EventID)
		assert.Equal(t, game.GameNo, got.GameNo)
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
		seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)
		game := testGame(eventID, 1, games.GamePhaseQualifying, 3, 7)
		require.NoError(t, db.CreateGames(ctx, eventID, 1, []games.Game{game}))

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
		_, err := db.GetGameByNumber(ctx, uuid.New(), 1)
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
	seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)
	seeds := []games.Game{
		testGame(eventID, 1, games.GamePhasePlayoff, 1, 1),
		testGame(eventID, 2, games.GamePhaseQualifying, 2, 1),
		testGame(eventID, 3, games.GamePhaseQualifying, 1, 2),
		testGame(eventID, 4, games.GamePhaseQualifying, 1, 1),
		testGame(eventID, 5, games.GamePhasePlayoff, 1, 2),
	}
	require.NoError(t, db.CreateGames(ctx, eventID, 1, seeds))

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
	seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)
	var batch []games.Game
	for i := 0; i < 30; i++ {
		g := testGame(eventID, i+1, games.GamePhaseQualifying, i/5+1, i%5+1)
		batch = append(batch, g)
	}
	require.NoError(t, db.CreateGames(ctx, eventID, 1, batch))

	got, err := db.ListGamesForEvent(ctx, eventID)
	require.NoError(t, err)
	assert.Len(t, got, 30)
}

func TestCreateGamesValidates(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	eventID := uuid.New()
	game := testGame(eventID, 1, games.GamePhaseQualifying, 1, 1)
	game.GameNo = 0

	err := db.CreateGames(ctx, eventID, 1, []games.Game{game})
	require.Error(t, err)
	var gameErr *games.Error
	require.ErrorAs(t, err, &gameErr)
	assert.Equal(t, games.REASON_INVALID_GAME_RESULT, gameErr.Reason)

	got, err := db.ListGamesForEvent(ctx, eventID)
	require.NoError(t, err)
	assert.Empty(t, got, "a rejected batch must write nothing")
}

func TestDeleteGames(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	eventID := uuid.New()
	seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)
	keep := testGame(eventID, 1, games.GamePhaseQualifying, 1, 1)
	drop1 := testGame(eventID, 2, games.GamePhaseQualifying, 1, 2)
	drop2 := testGame(eventID, 3, games.GamePhaseQualifying, 2, 1)
	require.NoError(t, db.CreateGames(ctx, eventID, 1, []games.Game{keep, drop1, drop2}))

	require.NoError(t, db.DeleteGames(ctx, eventID, 1, []int{drop1.GameNo, drop2.GameNo}))

	got, err := db.ListGamesForEvent(ctx, eventID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, keep.ID, got[0].ID)
}

func TestDeleteGamesUnknownIDsAreNoOps(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	eventID := uuid.New()
	seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)
	keep := testGame(eventID, 1, games.GamePhaseQualifying, 1, 1)
	require.NoError(t, db.CreateGames(ctx, eventID, 1, []games.Game{keep}))

	require.NoError(t, db.DeleteGames(ctx, eventID, 1, []int{99}))

	got, err := db.ListGamesForEvent(ctx, eventID)
	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestDeleteGamesRefusesGamesWithResults(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	eventID := uuid.New()
	seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)
	scheduled := testGame(eventID, 1, games.GamePhaseQualifying, 1, 1)
	completed := testGame(eventID, 2, games.GamePhaseQualifying, 1, 2)
	completed.Status = games.GameStatusCompleted
	completed.ScoreA, completed.ScoreB = ptrInt(10), ptrInt(5)
	require.NoError(t, db.CreateGames(ctx, eventID, 1, []games.Game{scheduled, completed}))

	err := db.DeleteGames(ctx, eventID, 1, []int{completed.GameNo})
	require.Error(t, err)
	var gameErr *games.Error
	require.ErrorAs(t, err, &gameErr)
	assert.Equal(t, games.REASON_VERSION_CONFLICT, gameErr.Reason)

	got, err := db.ListGamesForEvent(ctx, eventID)
	require.NoError(t, err)
	require.Len(t, got, 2, "a refused delete must destroy nothing")
}

func TestDeleteGamesChunksBeyondTransactionCap(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	eventID := uuid.New()
	seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)
	var batch []games.Game
	var gameNos []int
	for i := 0; i < 120; i++ {
		g := testGame(eventID, i+1, games.GamePhaseQualifying, i/10+1, i%10+1)
		batch = append(batch, g)
		gameNos = append(gameNos, g.GameNo)
	}
	require.NoError(t, db.CreateGames(ctx, eventID, 1, batch))

	require.NoError(t, db.DeleteGames(ctx, eventID, 1, gameNos))

	got, err := db.ListGamesForEvent(ctx, eventID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestCreateGamesOverwriteIsLastWriteWins(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	eventID := uuid.New()
	seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)
	game := testGame(eventID, 1, games.GamePhaseQualifying, 1, 1)
	require.NoError(t, db.CreateGames(ctx, eventID, 1, []games.Game{game}))

	// Puts are conditional on the slot not holding a result, not on emptiness:
	// an unplayed SCHEDULED slot may be replaced, which is what lets a retried
	// replace recover a partially-deleted schedule.
	game.Status = games.GameStatusCancelled
	require.NoError(t, db.CreateGames(ctx, eventID, 1, []games.Game{game}))

	got, err := db.GetGameByNumber(ctx, eventID, game.GameNo)
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
	require.NoError(t, db.CreateGames(ctx, event.ID, event.Version, []games.Game{
		testGame(event.ID, 1, games.GamePhaseQualifying, 1, 1),
		testGame(event.ID, 2, games.GamePhasePlayoff, 1, 1),
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
		game := testGame(event.ID, 1, games.GamePhaseQualifying, 1, 1)
		require.NoError(t, db.CreateGames(ctx, event.ID, 1, []games.Game{game}))

		game.Status = games.GameStatusCancelled
		game.Version++
		require.NoError(t, db.UpdateGameChecked(ctx, event.ID, game))

		got, err := db.GetGameByNumber(ctx, event.ID, game.GameNo)
		require.NoError(t, err)
		assert.Equal(t, games.GameStatusCancelled, got.Status)
	})

	t.Run("refuses when event finalized", func(t *testing.T) {
		resetTable(ctx)
		event := newOpenEvent()
		require.NoError(t, db.CreateEvent(ctx, event))
		game := testGame(event.ID, 1, games.GamePhaseQualifying, 1, 1)
		require.NoError(t, db.CreateGames(ctx, event.ID, 1, []games.Game{game}))

		// Finalize after the schedule exists: the schedule write barrier now
		// also refuses to seed rows into a finalized event.
		event.Status = events.EventStatusFinalized
		event.Version++
		require.NoError(t, db.UpdateEvent(ctx, event))

		game.Status = games.GameStatusCancelled
		game.Version++
		err := db.UpdateGameChecked(ctx, event.ID, game)
		require.Error(t, err)
		var gameErr *games.Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, games.REASON_EVENT_FINALIZED, gameErr.Reason)
	})

	t.Run("version conflict surfaces as a conflict", func(t *testing.T) {
		resetTable(ctx)
		event := newOpenEvent()
		require.NoError(t, db.CreateEvent(ctx, event))
		game := testGame(event.ID, 1, games.GamePhaseQualifying, 1, 1)
		require.NoError(t, db.CreateGames(ctx, event.ID, 1, []games.Game{game}))

		game.Status = games.GameStatusCancelled
		game.Version = 99
		err := db.UpdateGameChecked(ctx, event.ID, game)
		require.Error(t, err)
		var gameErr *games.Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, games.REASON_VERSION_CONFLICT, gameErr.Reason)
	})
}

// TestUpdateGameCheckedRejectsRegeneratedSlot pins the identity guard on
// result writes. A generate that replaces an unplayed row writes a NEW game at
// the same (phase, round, seq) key with a fresh gameNo and Version 1, so a
// result read before the replace also sees Version 1 and the version condition
// alone would pass by accident. The GameNo condition makes that stale write a
// plain version conflict instead of a silent revert of the regenerated
// pairing (and a resurrection of the deleted gameNo).
func TestUpdateGameCheckedRejectsRegeneratedSlot(t *testing.T) {
	ctx := context.Background()

	// regeneratedSlot is the reviewer's interleaving, faithful to the handler
	// order: create the original row, count it in LastGameNo, read it for a
	// result, then run Begin -> delete -> reserve -> create with a new gameNo
	// at the same slot.
	regeneratedSlot := func(t *testing.T, eventID uuid.UUID) (stale games.Game, a, b, c, d uuid.UUID) {
		t.Helper()
		require.NoError(t, db.CreateEvent(ctx, testGameEvent(eventID, events.EventStatusInProgress)))

		a, b, c, d = uuid.New(), uuid.New(), uuid.New(), uuid.New()
		oldGame := testGame(eventID, 1, games.GamePhaseQualifying, 1, 1)
		oldGame.SideA, oldGame.SideB = games.Side{TeamID: &a}, games.Side{TeamID: &b}
		require.NoError(t, db.CreateGames(ctx, eventID, 1, []games.Game{oldGame}))

		event, err := db.GetEvent(ctx, eventID)
		require.NoError(t, err)
		_, err = db.ReserveGameNumbers(ctx, eventID, event.Version, 1)
		require.NoError(t, err)

		stale, err = db.GetGameByNumber(ctx, eventID, 1)
		require.NoError(t, err)

		event, err = db.GetEvent(ctx, eventID)
		require.NoError(t, err)
		epoch, err := db.BeginScheduleWrite(ctx, eventID, event.Version)
		require.NoError(t, err)
		require.NoError(t, db.DeleteGames(ctx, eventID, epoch, []int{1}))
		epoch, err = db.ReserveGameNumbers(ctx, eventID, epoch, 2)
		require.NoError(t, err)
		newGame := testGame(eventID, 2, games.GamePhaseQualifying, 1, 1)
		newGame.SideA, newGame.SideB = games.Side{TeamID: &c}, games.Side{TeamID: &d}
		require.NoError(t, db.CreateGames(ctx, eventID, epoch, []games.Game{newGame}))

		// The regeneration really did land at the stale read's slot.
		regenerated, err := db.GetGameByNumber(ctx, eventID, 2)
		require.NoError(t, err)
		require.Equal(t, stale.Round, regenerated.Round)
		require.Equal(t, stale.Seq, regenerated.Seq)
		return stale, a, b, c, d
	}

	cases := []struct {
		name       string
		writeStale bool
		wantStatus games.GameStatus
	}{
		{name: "a result read before the replace is rejected", writeStale: true, wantStatus: games.GameStatusScheduled},
		{name: "a result write to the current row succeeds", writeStale: false, wantStatus: games.GameStatusCompleted},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetTable(ctx)
			eventID := uuid.New()
			stale, a, b, c, d := regeneratedSlot(t, eventID)

			toWrite := stale
			if !tc.writeStale {
				// Re-read the row that now occupies the slot: the legitimate
				// writer's view.
				var err error
				toWrite, err = db.GetGameByNumber(ctx, eventID, 2)
				require.NoError(t, err)
			}
			toWrite.Status = games.GameStatusCompleted
			toWrite.ScoreA, toWrite.ScoreB = ptrInt(10), ptrInt(5)
			toWrite.Version++

			err := db.UpdateGameChecked(ctx, eventID, toWrite)
			if tc.writeStale {
				require.Error(t, err)
				var gameErr *games.Error
				require.ErrorAs(t, err, &gameErr)
				assert.Equal(t, games.REASON_VERSION_CONFLICT, gameErr.Reason)
			} else {
				require.NoError(t, err)
			}

			// The regenerated row survives with its own identity and pairing.
			current, err := db.GetGameByNumber(ctx, eventID, 2)
			require.NoError(t, err)
			assert.Equal(t, 2, current.GameNo)
			assert.Equal(t, games.GamePhaseQualifying, current.Phase)
			assert.Equal(t, 1, current.Round)
			assert.Equal(t, 1, current.Seq)
			assert.Equal(t, c, *current.SideA.TeamID)
			assert.Equal(t, d, *current.SideB.TeamID)
			assert.Equal(t, tc.wantStatus, current.Status)
			if tc.wantStatus == games.GameStatusCompleted {
				assert.Equal(t, ptrInt(10), current.ScoreA)
				assert.Equal(t, ptrInt(5), current.ScoreB)
			}

			// The deleted gameNo is not resurrected by the stale write, and
			// the slot still holds exactly one row.
			_, err = db.GetGameByNumber(ctx, eventID, 1)
			require.Error(t, err)
			var gameErr *games.Error
			require.ErrorAs(t, err, &gameErr)
			assert.Equal(t, games.REASON_GAME_DOES_NOT_EXIST, gameErr.Reason)

			all, err := db.ListGamesForEvent(ctx, eventID)
			require.NoError(t, err)
			require.Len(t, all, 1, "the slot must not hold the old game as well")
			assert.NotEqual(t, a, *all[0].SideA.TeamID, "the old pairing must not win")
			assert.NotEqual(t, b, *all[0].SideB.TeamID, "the old pairing must not win")

			event, err := db.GetEvent(ctx, eventID)
			require.NoError(t, err)
			assert.Equal(t, 2, event.LastGameNo)
		})
	}
}

func TestBeginScheduleWrite(t *testing.T) {
	ctx := context.Background()

	t.Run("bumps the event version and the schedule generation", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()
		seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)

		epoch, err := db.BeginScheduleWrite(ctx, eventID, 1)
		require.NoError(t, err)
		assert.Equal(t, 2, epoch, "the grant returns the new event version")

		got, err := db.GetEvent(ctx, eventID)
		require.NoError(t, err)
		assert.Equal(t, 2, got.Version, "the grant invalidates stale schedule writers")
		assert.Equal(t, 1, got.ScheduleGeneration)

		epoch, err = db.BeginScheduleWrite(ctx, eventID, epoch)
		require.NoError(t, err)
		assert.Equal(t, 3, epoch)
		got, err = db.GetEvent(ctx, eventID)
		require.NoError(t, err)
		assert.Equal(t, 2, got.ScheduleGeneration)
	})

	t.Run("a stale version is a conflict", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()
		seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)

		_, err := db.BeginScheduleWrite(ctx, eventID, 99)
		require.Error(t, err)
		var gameErr *games.Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, games.REASON_VERSION_CONFLICT, gameErr.Reason)
	})

	t.Run("finalized events are refused", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()
		seedGameEvent(t, ctx, eventID, events.EventStatusFinalized)

		_, err := db.BeginScheduleWrite(ctx, eventID, 1)
		require.Error(t, err)
		var gameErr *games.Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, games.REASON_VERSION_CONFLICT, gameErr.Reason)
	})
}

func TestReserveGameNumbers(t *testing.T) {
	ctx := context.Background()

	t.Run("advances the high-water mark and the version", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()
		seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)

		epoch, err := db.ReserveGameNumbers(ctx, eventID, 1, 120)
		require.NoError(t, err)
		assert.Equal(t, 2, epoch)

		got, err := db.GetEvent(ctx, eventID)
		require.NoError(t, err)
		assert.Equal(t, 120, got.LastGameNo)

		// Re-reserving the same value is allowed (monotone, not strictly).
		_, err = db.ReserveGameNumbers(ctx, eventID, 2, 120)
		require.NoError(t, err)
		got, err = db.GetEvent(ctx, eventID)
		require.NoError(t, err)
		assert.Equal(t, 120, got.LastGameNo)
	})

	t.Run("never moves backwards", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()
		seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)
		_, err := db.ReserveGameNumbers(ctx, eventID, 1, 50)
		require.NoError(t, err)

		_, err = db.ReserveGameNumbers(ctx, eventID, 2, 40)
		require.Error(t, err)
		var gameErr *games.Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, games.REASON_VERSION_CONFLICT, gameErr.Reason)
	})
}

func TestCreateScheduledGameIsAtomic(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)
	eventID := uuid.New()
	seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)

	first := testGame(eventID, 1, games.GamePhaseQualifying, 1, 1)
	require.NoError(t, db.CreateScheduledGame(ctx, eventID, 1, first))

	// A second writer that minted from the same stale read (gameNo 1) loses
	// the Version condition instead of minting a duplicate.
	stale := testGame(eventID, 1, games.GamePhaseQualifying, 1, 2)
	err := db.CreateScheduledGame(ctx, eventID, 1, stale)
	require.Error(t, err)
	var gameErr *games.Error
	require.ErrorAs(t, err, &gameErr)
	assert.Equal(t, games.REASON_VERSION_CONFLICT, gameErr.Reason)

	got, err := db.ListGamesForEvent(ctx, eventID)
	require.NoError(t, err)
	require.Len(t, got, 1, "a refused create must write nothing")

	event, err := db.GetEvent(ctx, eventID)
	require.NoError(t, err)
	assert.Equal(t, 2, event.Version)
	assert.Equal(t, 1, event.LastGameNo)

	second := testGame(eventID, event.LastGameNo+1, games.GamePhaseQualifying, 1, 2)
	require.NoError(t, db.CreateScheduledGame(ctx, eventID, event.Version, second))

	event, err = db.GetEvent(ctx, eventID)
	require.NoError(t, err)
	assert.Equal(t, 2, event.LastGameNo)
}

func TestScheduledGameNumbersAreNotRecycledAfterDelete(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)
	eventID := uuid.New()
	seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)

	first := testGame(eventID, 1, games.GamePhaseQualifying, 1, 1)
	require.NoError(t, db.CreateScheduledGame(ctx, eventID, 1, first))

	event, err := db.GetEvent(ctx, eventID)
	require.NoError(t, err)
	require.NoError(t, db.DeleteScheduledGame(ctx, eventID, event.Version, first))

	event, err = db.GetEvent(ctx, eventID)
	require.NoError(t, err)
	assert.Equal(t, 1, event.LastGameNo, "deleting the row keeps the high-water mark")

	second := testGame(eventID, event.LastGameNo+1, games.GamePhaseQualifying, 1, 1)
	require.NoError(t, db.CreateScheduledGame(ctx, eventID, event.Version, second))

	got, err := db.GetGameByNumber(ctx, eventID, 2)
	require.NoError(t, err)
	assert.Equal(t, 2, got.GameNo, "the deleted number 1 is not reissued")
}

func TestDeleteScheduledGameRefusesResults(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)
	eventID := uuid.New()
	seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)

	game := testGame(eventID, 1, games.GamePhaseQualifying, 1, 1)
	require.NoError(t, db.CreateGames(ctx, eventID, 1, []games.Game{game}))

	game.Status = games.GameStatusCompleted
	game.ScoreA, game.ScoreB = ptrInt(10), ptrInt(5)
	game.Version = 2
	require.NoError(t, db.UpdateGameChecked(ctx, eventID, game))

	err := db.DeleteScheduledGame(ctx, eventID, 1, game)
	require.Error(t, err, "a result row is never deletable")
	got, err := db.GetGameByNumber(ctx, eventID, 1)
	require.NoError(t, err)
	assert.Equal(t, games.GameStatusCompleted, got.Status)
}

func TestDeleteGamesRefusesAProtectedResultAtChunkScale(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)
	eventID := uuid.New()
	seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)

	// 120 rows means the delete spans two transactions (99 + 21). The result
	// sits in the second chunk; the pre-delete read refuses the whole set
	// before any chunk runs, so nothing is destroyed.
	var batch []games.Game
	var gameNos []int
	for i := 0; i < 120; i++ {
		g := testGame(eventID, i+1, games.GamePhaseQualifying, i/10+1, i%10+1)
		batch = append(batch, g)
		gameNos = append(gameNos, g.GameNo)
	}
	require.NoError(t, db.CreateGames(ctx, eventID, 1, batch))

	protected := batch[110]
	protected.Status = games.GameStatusCompleted
	protected.ScoreA, protected.ScoreB = ptrInt(10), ptrInt(5)
	protected.Version = 2
	require.NoError(t, db.UpdateGameChecked(ctx, eventID, protected))

	err := db.DeleteGames(ctx, eventID, 1, gameNos)
	require.Error(t, err)
	var gameErr *games.Error
	require.ErrorAs(t, err, &gameErr)
	assert.Equal(t, games.REASON_VERSION_CONFLICT, gameErr.Reason)

	got, err := db.ListGamesForEvent(ctx, eventID)
	require.NoError(t, err)
	assert.Len(t, got, 120, "a refused delete set destroys nothing")

	// Recovery: the unplayed leftovers are sweepable; the result stays.
	var unplayed []int
	for _, g := range batch {
		if g.GameNo != protected.GameNo {
			unplayed = append(unplayed, g.GameNo)
		}
	}
	require.NoError(t, db.DeleteGames(ctx, eventID, 1, unplayed))
	got, err = db.ListGamesForEvent(ctx, eventID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, protected.GameNo, got[0].GameNo)
	assert.Equal(t, games.GameStatusCompleted, got[0].Status)
}

func TestCreateGamesRefusesAResultSlot(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)
	eventID := uuid.New()
	seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)

	a, b := uuid.New(), uuid.New()
	completed := testGame(eventID, 1, games.GamePhaseQualifying, 1, 1)
	completed.Status = games.GameStatusCompleted
	completed.SideA, completed.SideB = games.Side{TeamID: &a}, games.Side{TeamID: &b}
	completed.ScoreA, completed.ScoreB = ptrInt(10), ptrInt(5)
	require.NoError(t, db.CreateGames(ctx, eventID, 1, []games.Game{completed}))

	overwrite := testGame(eventID, 2, games.GamePhaseQualifying, 1, 1)
	err := db.CreateGames(ctx, eventID, 1, []games.Game{overwrite})
	require.Error(t, err)
	var gameErr *games.Error
	require.ErrorAs(t, err, &gameErr)
	assert.Equal(t, games.REASON_SLOT_OCCUPIED, gameErr.Reason)

	got, err := db.GetGameByNumber(ctx, eventID, 1)
	require.NoError(t, err)
	assert.Equal(t, games.GameStatusCompleted, got.Status, "the result is preserved")
}

func TestUpdateGameScheduleCheckedMovesChangedPosition(t *testing.T) {
	ctx := context.Background()

	t.Run("round and seq edits rewrite the sort key without duplicating", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()
		seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)
		a, b, c, d := uuid.New(), uuid.New(), uuid.New(), uuid.New()
		game := testGame(eventID, 1, games.GamePhaseQualifying, 1, 1)
		game.SideA, game.SideB = games.Side{TeamID: &a}, games.Side{TeamID: &b}
		require.NoError(t, db.CreateGames(ctx, eventID, 1, []games.Game{game}))

		moved := game
		moved.Round, moved.Seq = 2, 1
		moved.SideA, moved.SideB = games.Side{TeamID: &c}, games.Side{TeamID: &d}
		moved.Version++
		require.NoError(t, db.UpdateGameScheduleChecked(ctx, eventID, 1, game, moved))

		got, err := db.ListGamesForEvent(ctx, eventID)
		require.NoError(t, err)
		require.Len(t, got, 1, "the old row must be removed by the same transaction")
		assert.Equal(t, 2, got[0].Round)
		assert.Equal(t, 1, got[0].Seq)
		assert.Equal(t, game.GameNo, got[0].GameNo)
		assert.Equal(t, c, *got[0].SideA.TeamID)

		event, err := db.GetEvent(ctx, eventID)
		require.NoError(t, err)
		assert.Equal(t, 2, event.Version, "the edit grants a new schedule epoch")
	})

	t.Run("moving onto an occupied position is a slot conflict", func(t *testing.T) {
		resetTable(ctx)
		eventID := uuid.New()
		seedGameEvent(t, ctx, eventID, events.EventStatusInProgress)
		moving := testGame(eventID, 1, games.GamePhaseQualifying, 1, 1)
		occupant := testGame(eventID, 2, games.GamePhaseQualifying, 1, 2)
		require.NoError(t, db.CreateGames(ctx, eventID, 1, []games.Game{moving, occupant}))

		moved := moving
		moved.Seq = 2
		moved.Version++
		err := db.UpdateGameScheduleChecked(ctx, eventID, 1, moving, moved)
		require.Error(t, err)
		var gameErr *games.Error
		require.ErrorAs(t, err, &gameErr)
		assert.Equal(t, games.REASON_SLOT_OCCUPIED, gameErr.Reason)

		got, err := db.ListGamesForEvent(ctx, eventID)
		require.NoError(t, err)
		require.Len(t, got, 2, "a refused move must change nothing")
	})
}
