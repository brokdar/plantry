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
	// The shape of CountPlateEventsAfter: an OR of two IN lists is a
	// multi-index OR, so both indexes show in the plan.
	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT COUNT(*) FROM plate_events WHERE id > 0 AND (plate_id IN (1, 2) OR cook_session_id IN (3, 4))`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var plan string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		plan += detail + "\n"
	}
	require.NoError(t, rows.Err())
	assert.Contains(t, plan, "ix_plate_events_plate")
	assert.Contains(t, plan, "ix_plate_events_session")
}
