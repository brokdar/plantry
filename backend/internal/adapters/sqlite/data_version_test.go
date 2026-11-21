package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func TestDataVersion_ChangesWithEveryWriteAndUnitOfWork(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	v := sqlite.NewDataVersion(db)
	tok := func(ctx context.Context) string {
		t.Helper()
		s, ok := v.DataVersion(ctx)
		require.True(t, ok)
		return s
	}

	a := tok(ctx)
	assert.Equal(t, a, tok(ctx), "no write, same token")
	mustExec(t, db, `UPDATE user_profile SET kcal_target = 2000 WHERE id = 1`)
	b := tok(ctx)
	assert.NotEqual(t, a, b, "a write")

	var inside string
	err := sqlite.NewTxRunner(db).InTx(ctx, func(ctx context.Context) error {
		inside = tok(ctx)
		return errors.New("rollback")
	})
	require.Error(t, err)
	assert.NotEqual(t, b, inside, "a unit of work has its own token")
	assert.Equal(t, b, tok(ctx), "nothing written: the pool's token is unchanged")
}

func TestDataVersion_NoTokenWithSeveralConnections(t *testing.T) {
	db := testhelper.NewTestDB(t)
	db.SetMaxOpenConns(2)
	_, ok := sqlite.NewDataVersion(db).DataVersion(context.Background())
	assert.False(t, ok, "counters of different connections do not compare")
}
