package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// Hand-written SQL may run the same query again inside a unit of work while
// its earlier rows are open: dbtx hands out no cached statement, since two
// cursors on one SQLite statement corrupt each other (with the cache this
// loop never ended).
func TestConn_SameRawQueryWhileItsRowsAreOpen(t *testing.T) {
	db := testhelper.NewTestDB(t)
	for _, name := range []string{"slot.a", "slot.b"} {
		_, err := db.Exec(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES (?, 'moon', 0, 1)`, name)
		require.NoError(t, err)
	}
	c := newConn(db)
	const q = `SELECT name_key FROM time_slots ORDER BY id`
	require.NoError(t, inTx(context.Background(), db, func(ctx context.Context) error {
		d := c.dbtx(ctx)
		outer, err := d.QueryContext(ctx, q)
		require.NoError(t, err)
		defer func() { _ = outer.Close() }()
		var outerNames, innerNames []string
		for outer.Next() {
			var n string
			require.NoError(t, outer.Scan(&n))
			outerNames = append(outerNames, n)
			inner, err := d.QueryContext(ctx, q)
			require.NoError(t, err, "the same query while its rows are open")
			for inner.Next() {
				var m string
				require.NoError(t, inner.Scan(&m))
				innerNames = append(innerNames, m)
			}
			require.NoError(t, inner.Close())
		}
		require.NoError(t, outer.Err())
		assert.Equal(t, []string{"slot.a", "slot.b"}, outerNames)
		assert.Len(t, innerNames, 4)
		return nil
	}))
}
