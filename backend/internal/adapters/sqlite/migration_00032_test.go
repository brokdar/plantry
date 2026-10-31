package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
	_ "github.com/jaltszeimer/plantry/backend/db/migrations"
)

// openDBAt00031WithConversation opens a fresh SQLite file migrated to 00031
// holding conversation 4 (with a week_id) and one message on it.
func openDBAt00031WithConversation(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration32.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 31))
	mustExec(t, conn, `INSERT INTO ai_conversations (id, week_id, title) VALUES (4, 17, 'Plan my week')`)
	mustExec(t, conn, `INSERT INTO ai_messages (conversation_id, role, content) VALUES (4, 'user', '"hi"')`)
	return conn
}

// B8-AC4: week_id and its index are gone; conversations and their messages
// survive.
func TestMigration00032_DropsConversationWeek(t *testing.T) {
	conn := openDBAt00031WithConversation(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 32))

	assert.False(t, columnExists(t, conn, "ai_conversations", "week_id"))
	assert.False(t, indexExists(t, conn, "ix_ai_conversations_week"))
	assert.Equal(t, "4|Plan my week", queryString(t, conn, `SELECT id || '|' || title FROM ai_conversations`))
	assert.Equal(t, "1", queryString(t, conn, `SELECT COUNT(*) FROM ai_messages WHERE conversation_id = 4`))
}

// B8-AC4: Down restores the nullable column and its index, Up drops them again.
func TestMigration00032_DownUpRoundTrips(t *testing.T) {
	conn := openDBAt00031WithConversation(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 32))
	require.NoError(t, goose.DownTo(conn, "migrations", 31))
	assert.True(t, columnExists(t, conn, "ai_conversations", "week_id"))
	assert.True(t, indexExists(t, conn, "ix_ai_conversations_week"))
	assert.Equal(t, "1", queryString(t, conn, `SELECT COUNT(*) FROM ai_conversations WHERE week_id IS NULL`))

	require.NoError(t, goose.UpTo(conn, "migrations", 32))
	assert.False(t, columnExists(t, conn, "ai_conversations", "week_id"))
	assert.False(t, indexExists(t, conn, "ix_ai_conversations_week"))
	assert.Equal(t, "1", queryString(t, conn, `SELECT COUNT(*) FROM ai_conversations`))
}
