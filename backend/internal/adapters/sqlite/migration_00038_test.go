package sqlite_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func TestMigration00038_PlateEventsIndexes(t *testing.T) {
	db := testhelper.NewTestDB(t)
	for _, name := range []string{"ix_plate_events_plate", "ix_plate_events_session"} {
		var n int
		require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&n))
		assert.Equal(t, 1, n, name)
	}
	var plan string
	require.NoError(t, db.QueryRow(`EXPLAIN QUERY PLAN SELECT COUNT(*) FROM plate_events WHERE cook_session_id IN (1, 2)`).Scan(new(int), new(int), new(int), &plan))
	assert.Contains(t, plan, "ix_plate_events_session")
}
