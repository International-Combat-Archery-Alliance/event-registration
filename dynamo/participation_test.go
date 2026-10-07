package dynamo

import (
	"context"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testParticipation(eventID, teamID uuid.UUID) (teams.Participation, teams.TeamHistory) {
	participation := teams.Participation{
		EventID:           eventID,
		TeamID:            teamID,
		Status:            teams.ParticipationStatusConfirmed,
		RosterSnapshot:    []uuid.UUID{},
		RosterSizeAtEvent: 0,
		Version:           1,
	}
	history := teams.TeamHistory{
		TeamID:    teamID,
		EventID:   eventID,
		EventName: "Seed Cup",
		EventDate: time.Now().UTC().Truncate(time.Second),
		Status:    teams.ParticipationStatusConfirmed,
		Version:   1,
	}
	return participation, history
}

func TestSeedAndGetParticipation(t *testing.T) {
	ctx := context.Background()

	t.Run("round trip writes participation and history", func(t *testing.T) {
		resetTable(ctx)
		eventID, teamID := uuid.New(), uuid.New()
		participation, history := testParticipation(eventID, teamID)
		require.NoError(t, db.SeedParticipation(ctx, participation, history))

		got, err := db.GetParticipation(ctx, eventID, teamID)
		require.NoError(t, err)
		assert.Equal(t, eventID, got.EventID)
		assert.Equal(t, teamID, got.TeamID)
		assert.Equal(t, teams.ParticipationStatusConfirmed, got.Status)
		assert.Empty(t, got.RosterSnapshot)
		assert.Equal(t, 0, got.RosterSizeAtEvent)
		assert.Equal(t, 1, got.Version)

		key, err := attributevalue.MarshalMap(map[string]any{
			"PK": teamHistoryPK(teamID),
			"SK": teamHistorySK(eventID),
		})
		require.NoError(t, err)
		out, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(tableName),
			Key:       key,
		})
		require.NoError(t, err)
		require.NotEmpty(t, out.Item, "team-history row written in the same transaction")
	})

	t.Run("second seed conflicts", func(t *testing.T) {
		resetTable(ctx)
		eventID, teamID := uuid.New(), uuid.New()
		participation, history := testParticipation(eventID, teamID)
		require.NoError(t, db.SeedParticipation(ctx, participation, history))

		err := db.SeedParticipation(ctx, participation, history)
		require.Error(t, err)
		var teamErr *teams.Error
		require.ErrorAs(t, err, &teamErr)
		assert.Equal(t, teams.REASON_PARTICIPATION_ALREADY_EXISTS, teamErr.Reason)
	})

	t.Run("get missing returns not-exist", func(t *testing.T) {
		resetTable(ctx)
		_, err := db.GetParticipation(ctx, uuid.New(), uuid.New())
		require.Error(t, err)
		var teamErr *teams.Error
		require.ErrorAs(t, err, &teamErr)
		assert.Equal(t, teams.REASON_PARTICIPATION_DOES_NOT_EXIST, teamErr.Reason)
	})

	t.Run("seeded rows carry no GSI attributes", func(t *testing.T) {
		resetTable(ctx)
		eventID, teamID := uuid.New(), uuid.New()
		participation, history := testParticipation(eventID, teamID)
		require.NoError(t, db.SeedParticipation(ctx, participation, history))

		for _, key := range []map[string]any{
			{"PK": participationPK(eventID), "SK": participationSK(teamID)},
			{"PK": teamHistoryPK(teamID), "SK": teamHistorySK(eventID)},
		} {
			marshaled, err := attributevalue.MarshalMap(key)
			require.NoError(t, err)
			out, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
				TableName: aws.String(tableName),
				Key:       marshaled,
			})
			require.NoError(t, err)
			require.NotEmpty(t, out.Item)
			for _, attr := range []string{"GSI1PK", "GSI1SK"} {
				_, present := out.Item[attr]
				assert.False(t, present, "seeded row must not carry %s", attr)
			}
		}
	})
}

func TestListParticipationsForEvent(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	eventID, otherEventID := uuid.New(), uuid.New()
	teamA, teamB := uuid.New(), uuid.New()
	partA, histA := testParticipation(eventID, teamA)
	partB, histB := testParticipation(eventID, teamB)
	partB.Status = teams.ParticipationStatusWithdrawn
	other, otherHist := testParticipation(otherEventID, teamA)
	require.NoError(t, db.SeedParticipation(ctx, partA, histA))
	require.NoError(t, db.SeedParticipation(ctx, partB, histB))
	require.NoError(t, db.SeedParticipation(ctx, other, otherHist))

	got, err := db.ListParticipationsForEvent(ctx, eventID)
	require.NoError(t, err)
	require.Len(t, got, 2)
}
