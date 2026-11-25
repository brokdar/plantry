package devclock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClockSetAndAdvance(t *testing.T) {
	c := New()
	assert.WithinDuration(t, time.Now(), c.Now(), time.Second)

	target := time.Date(2031, 3, 1, 8, 0, 0, 0, time.UTC)
	c.Set(target)
	assert.WithinDuration(t, target, c.Now(), time.Second)

	c.Advance(48 * time.Hour)
	assert.WithinDuration(t, target.Add(48*time.Hour), c.Now(), time.Second)
}

func TestParse(t *testing.T) {
	got, err := Parse("2031-03-01")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2031, 3, 1, 8, 0, 0, 0, time.UTC), got)

	got, err = Parse("2031-03-01T10:30:00Z")
	require.NoError(t, err)
	assert.Equal(t, time.Date(2031, 3, 1, 10, 30, 0, 0, time.UTC), got)

	_, err = Parse("tomorrow")
	assert.Error(t, err)
}
