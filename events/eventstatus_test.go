package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEventStatusValid(t *testing.T) {
	assert.True(t, EventStatusOpened.Valid())
	assert.True(t, EventStatusInProgress.Valid())
	assert.True(t, EventStatusFinalized.Valid())
	assert.False(t, EventStatus("").Valid())
	assert.False(t, EventStatus("BOGUS").Valid())
}

func TestEventStatusNormalizeDefault(t *testing.T) {
	assert.Equal(t, EventStatusOpened, EventStatus("").NormalizeDefault())
	assert.Equal(t, EventStatusOpened, EventStatusOpened.NormalizeDefault())
	assert.Equal(t, EventStatusInProgress, EventStatusInProgress.NormalizeDefault())
	assert.Equal(t, EventStatusFinalized, EventStatusFinalized.NormalizeDefault())
}

func TestAllowedTransition(t *testing.T) {
	t.Run("same is a no-op", func(t *testing.T) {
		assert.True(t, AllowedTransition(EventStatusOpened, EventStatusOpened))
		assert.True(t, AllowedTransition(EventStatusInProgress, EventStatusInProgress))
		assert.True(t, AllowedTransition(EventStatusFinalized, EventStatusFinalized))
	})

	t.Run("empty means OPENED", func(t *testing.T) {
		assert.True(t, AllowedTransition("", ""))
		assert.True(t, AllowedTransition("", EventStatusOpened))
		assert.True(t, AllowedTransition(EventStatusOpened, ""))
	})

	t.Run("forward transitions allowed", func(t *testing.T) {
		assert.True(t, AllowedTransition(EventStatusOpened, EventStatusInProgress))
		assert.True(t, AllowedTransition(EventStatusOpened, EventStatusFinalized))
		assert.True(t, AllowedTransition(EventStatusInProgress, EventStatusFinalized))
	})

	t.Run("backward transitions rejected", func(t *testing.T) {
		assert.False(t, AllowedTransition(EventStatusInProgress, EventStatusOpened))
		assert.False(t, AllowedTransition(EventStatusFinalized, EventStatusOpened))
		assert.False(t, AllowedTransition(EventStatusFinalized, EventStatusInProgress))
	})

	t.Run("invalid values rejected", func(t *testing.T) {
		assert.False(t, AllowedTransition(EventStatus("BOGUS"), EventStatus("BOGUS")))
		assert.False(t, AllowedTransition(EventStatusOpened, EventStatus("BOGUS")))
		assert.False(t, AllowedTransition(EventStatus("BOGUS"), EventStatusOpened))
	})
}
