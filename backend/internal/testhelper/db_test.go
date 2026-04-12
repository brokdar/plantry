package testhelper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func TestNewTestDB(t *testing.T) {
	conn := testhelper.NewTestDB(t)

	var fk int
	require.NoError(t, conn.QueryRow("PRAGMA foreign_keys").Scan(&fk))
	assert.Equal(t, 1, fk)

	var version int64
	require.NoError(t, conn.QueryRow("SELECT version_id FROM goose_db_version ORDER BY id DESC LIMIT 1").Scan(&version))
	assert.GreaterOrEqual(t, version, int64(1))
}
