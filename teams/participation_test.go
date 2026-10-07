package teams

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParticipationStatusValid(t *testing.T) {
	assert.True(t, ParticipationStatusRegistered.Valid())
	assert.True(t, ParticipationStatusConfirmed.Valid())
	assert.True(t, ParticipationStatusWithdrawn.Valid())
	assert.True(t, ParticipationStatusDNS.Valid())
	assert.False(t, ParticipationStatus("").Valid())
	assert.False(t, ParticipationStatus("BOGUS").Valid())
}

func TestParticipationValidate(t *testing.T) {
	t.Run("empty snapshot seed passes", func(t *testing.T) {
		p := Participation{
			EventID:           uuid.New(),
			TeamID:            uuid.New(),
			Status:            ParticipationStatusConfirmed,
			RosterSnapshot:    []uuid.UUID{},
			RosterSizeAtEvent: 0,
			Version:           1,
		}
		require.NoError(t, p.Validate())
	})

	t.Run("size must match snapshot", func(t *testing.T) {
		p := Participation{
			EventID:           uuid.New(),
			TeamID:            uuid.New(),
			Status:            ParticipationStatusConfirmed,
			RosterSnapshot:    []uuid.UUID{uuid.New()},
			RosterSizeAtEvent: 0,
			Version:           1,
		}
		require.Error(t, p.Validate())
	})

	t.Run("invalid status rejected", func(t *testing.T) {
		p := Participation{
			EventID: uuid.New(),
			TeamID:  uuid.New(),
			Status:  ParticipationStatus("BOGUS"),
		}
		err := p.Validate()
		require.Error(t, err)
		var teamErr *Error
		require.ErrorAs(t, err, &teamErr)
		assert.Equal(t, REASON_INVALID_TEAM, teamErr.Reason)
	})
}
