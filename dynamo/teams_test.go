package dynamo

import (
	"context"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/event-registration/events"
	"github.com/International-Combat-Archery-Alliance/event-registration/teams"
	"github.com/Rhymond/go-money"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTeam(name, city string) teams.Team {
	now := time.Now().UTC().Truncate(time.Second)
	return teams.Team{
		ID:        uuid.New(),
		Version:   1,
		Name:      name,
		HomeCity:  city,
		Status:    teams.TeamStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestCreateAndGetTeam(t *testing.T) {
	ctx := context.Background()

	t.Run("round trip preserves every field", func(t *testing.T) {
		resetTable(ctx)
		captain := uuid.New()
		team := testTeam("Boston Renegades", "Boston, USA")
		team.CaptainPlayerID = &captain
		require.NoError(t, db.CreateTeam(ctx, team))

		got, err := db.GetTeam(ctx, team.ID)
		require.NoError(t, err)
		assert.Equal(t, team.ID, got.ID)
		assert.Equal(t, team.Name, got.Name)
		assert.Equal(t, team.HomeCity, got.HomeCity)
		assert.Equal(t, teams.TeamStatusActive, got.Status)
		require.NotNil(t, got.CaptainPlayerID)
		assert.Equal(t, captain, *got.CaptainPlayerID)
		assert.True(t, team.CreatedAt.Equal(got.CreatedAt))
		assert.Equal(t, 1, got.Version)
	})

	t.Run("nullable captain round trips", func(t *testing.T) {
		resetTable(ctx)
		team := testTeam("Rosterless Club", "Nowhere, USA")
		require.NoError(t, db.CreateTeam(ctx, team))

		got, err := db.GetTeam(ctx, team.ID)
		require.NoError(t, err)
		assert.Nil(t, got.CaptainPlayerID)
	})

	t.Run("get missing team returns not-exist", func(t *testing.T) {
		resetTable(ctx)
		_, err := db.GetTeam(ctx, uuid.New())
		require.Error(t, err)
		var teamErr *teams.Error
		require.ErrorAs(t, err, &teamErr)
		assert.Equal(t, teams.REASON_TEAM_DOES_NOT_EXIST, teamErr.Reason)
	})
}

func TestTeamNameReservation(t *testing.T) {
	ctx := context.Background()

	t.Run("duplicate name rejected", func(t *testing.T) {
		resetTable(ctx)
		require.NoError(t, db.CreateTeam(ctx, testTeam("Boston Renegades", "Boston, USA")))

		err := db.CreateTeam(ctx, testTeam("Boston Renegades", "Cambridge, USA"))
		require.Error(t, err)
		var teamErr *teams.Error
		require.ErrorAs(t, err, &teamErr)
		assert.Equal(t, teams.REASON_TEAM_NAME_TAKEN, teamErr.Reason)
	})

	t.Run("name clash is case-insensitive", func(t *testing.T) {
		resetTable(ctx)
		require.NoError(t, db.CreateTeam(ctx, testTeam("Boston Renegades", "Boston, USA")))

		err := db.CreateTeam(ctx, testTeam("  BOSTON RENEGADES  ", "Boston, USA"))
		require.Error(t, err)
		var teamErr *teams.Error
		require.ErrorAs(t, err, &teamErr)
		assert.Equal(t, teams.REASON_TEAM_NAME_TAKEN, teamErr.Reason)
	})
}

func TestTeamGSIRule(t *testing.T) {
	ctx := context.Background()
	resetTable(ctx)

	team := testTeam("GSI Guard Club", "Boston, USA")
	require.NoError(t, db.CreateTeam(ctx, team))

	// The TEAM master carries GSI1PK/SK for the name index.
	masterKey, err := attributevalue.MarshalMap(map[string]any{
		"PK": teamPK(team.ID),
		"SK": teamPK(team.ID),
	})
	require.NoError(t, err)
	masterOut, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       masterKey,
	})
	require.NoError(t, err)
	require.NotEmpty(t, masterOut.Item)
	var master teamDynamo
	require.NoError(t, attributevalue.UnmarshalMap(masterOut.Item, &master))
	assert.Equal(t, "TEAM", master.GSI1PK)
	assert.Contains(t, master.GSI1SK, "NAME#gsi guard club#")

	// The TEAM_NAME# reservation row carries no GSI attributes.
	resKey, err := attributevalue.MarshalMap(map[string]any{
		"PK": teamNamePK(teams.NormalizeTeamName(team.Name)),
		"SK": teamNamePK(teams.NormalizeTeamName(team.Name)),
	})
	require.NoError(t, err)
	resOut, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       resKey,
	})
	require.NoError(t, err)
	require.NotEmpty(t, resOut.Item)
	for _, attr := range []string{"GSI1PK", "GSI1SK"} {
		_, present := resOut.Item[attr]
		assert.False(t, present, "reservation row must not carry %s", attr)
	}
}

func TestTeamsDoNotPolluteEventsListing(t *testing.T) {
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
	require.NoError(t, db.CreateTeam(ctx, testTeam("Boston Renegades", "Boston, USA")))
	require.NoError(t, db.CreateTeam(ctx, testTeam("Cambridge Collective", "Cambridge, USA")))

	resp, err := db.GetEvents(ctx, 10, nil)
	require.NoError(t, err)
	require.Len(t, resp.Data, 1)
	assert.Equal(t, event.ID, resp.Data[0].ID)
}
