package teams

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeTeamName(t *testing.T) {
	assert.Equal(t, "boston renegades", NormalizeTeamName("Boston Renegades"))
	assert.Equal(t, "boston renegades", NormalizeTeamName("  BOSTON RENEGADES  "))
	assert.Equal(t, "", NormalizeTeamName("   "))
}

func TestTeamStatusValid(t *testing.T) {
	assert.True(t, TeamStatusActive.Valid())
	assert.True(t, TeamStatusArchived.Valid())
	assert.False(t, TeamStatus("").Valid())
	assert.False(t, TeamStatus("BOGUS").Valid())
}

func TestTeamValidate(t *testing.T) {
	valid := Team{
		ID:       uuid.New(),
		Version:  1,
		Name:     "Boston Renegades",
		HomeCity: "Boston, USA",
		Status:   TeamStatusActive,
	}
	assert.NoError(t, valid.Validate())

	t.Run("nullable captain allowed in MVP", func(t *testing.T) {
		assert.NoError(t, valid.Validate())
		captain := uuid.New()
		withCaptain := valid
		withCaptain.CaptainPlayerID = &captain
		assert.NoError(t, withCaptain.Validate())
	})

	t.Run("name bounds", func(t *testing.T) {
		short := valid
		short.Name = "AB"
		assert.Error(t, short.Validate())

		blank := valid
		blank.Name = "   "
		assert.Error(t, blank.Validate(), "whitespace-only names normalize to empty")

		long := valid
		long.Name = strings.Repeat("A", 101)
		assert.Error(t, long.Validate())
	})

	t.Run("city bounds", func(t *testing.T) {
		short := valid
		short.HomeCity = "AB"
		assert.Error(t, short.Validate())
	})

	t.Run("invalid status rejected", func(t *testing.T) {
		bad := valid
		bad.Status = TeamStatus("BOGUS")
		err := bad.Validate()
		assert.Error(t, err)
		var teamErr *Error
		assert.ErrorAs(t, err, &teamErr)
		assert.Equal(t, REASON_INVALID_TEAM, teamErr.Reason)
	})
}
