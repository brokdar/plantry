package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
	_ "github.com/jaltszeimer/plantry/backend/db/migrations"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/template"
)

// setupMigration17DB applies migrations up to 16 and seeds a template with two
// entries: one at day_offset=0 and one at day_offset=6 (the legacy week-shaped
// case). It returns the connection so callers can run migration 17 themselves.
func setupMigration17DB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migration17.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 16))

	_, err = conn.Exec(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'utensils', 1, 1)`)
	require.NoError(t, err)

	_, err = conn.Exec(`INSERT INTO foods (name, kind, source) VALUES ('Test Food', 'leaf', 'manual')`)
	require.NoError(t, err)

	_, err = conn.Exec(`INSERT INTO templates (name) VALUES ('Legacy Week Pattern')`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO template_components (template_id, food_id, portions, sort_order, day_offset) VALUES (1, 1, 1, 0, 0)`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO template_components (template_id, food_id, portions, sort_order, day_offset) VALUES (1, 1, 2, 1, 6)`)
	require.NoError(t, err)

	return conn
}

// TestMigration_TemplateScopes_AllLegacyAreSlotScope is the primary regression
// guard for issue surfaced in review: legacy templates with day_offset>0
// entries must NOT be auto-classified as 'week'. They land at scope='slot' so
// applySlot's day_offset grouping continues to handle them.
func TestMigration_TemplateScopes_AllLegacyAreSlotScope(t *testing.T) {
	conn := setupMigration17DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 17))

	var scope string
	require.NoError(t, conn.QueryRow(`SELECT scope FROM templates WHERE id = 1`).Scan(&scope))
	assert.Equal(t, "slot", scope, "legacy templates must not be auto-classified as week scope")
}

// TestMigration_TemplateScopes_EntriesPreserved verifies the data shape after
// migration: rows survive the rename, day_offset is preserved, slot_id is
// NULL (no per-entry slot binding existed pre-17), note is NULL.
func TestMigration_TemplateScopes_EntriesPreserved(t *testing.T) {
	conn := setupMigration17DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 17))

	rows, err := conn.Query(`SELECT day_offset, slot_id, note FROM template_entries WHERE template_id = 1 ORDER BY sort_order`)
	require.NoError(t, err)
	defer rows.Close()

	type row struct {
		dayOffset int64
		slotID    sql.NullInt64
		note      sql.NullString
	}
	var got []row
	for rows.Next() {
		var r row
		require.NoError(t, rows.Scan(&r.dayOffset, &r.slotID, &r.note))
		got = append(got, r)
	}
	require.NoError(t, rows.Err())
	require.Len(t, got, 2)

	assert.Equal(t, int64(0), got[0].dayOffset)
	assert.False(t, got[0].slotID.Valid, "legacy entries must have slot_id=NULL")
	assert.False(t, got[0].note.Valid, "legacy entries must have note=NULL")

	assert.Equal(t, int64(6), got[1].dayOffset, "day_offset must survive rebuild")
	assert.False(t, got[1].slotID.Valid)
}

// TestMigration_TemplateScopes_LegacyApplySucceeds drives the post-migration
// template through Service.Apply with a slot-scope payload. This is the
// behaviour-level guard: if someone re-adds the auto-classify UPDATE, this
// test fails because applyMultiSlot rejects entries with slot_id=NULL.
func TestMigration_TemplateScopes_LegacyApplySucceeds(t *testing.T) {
	conn := setupMigration17DB(t)
	require.NoError(t, goose.UpTo(conn, "migrations", 17))

	templateRepo := sqlite.NewTemplateRepo(conn)
	plateRepo := sqlite.NewPlateRepo(conn)
	foodRepo := sqlite.NewFoodRepo(conn)
	txRunner := sqlite.NewTxRunner(conn)
	svc := template.NewService(templateRepo, foodRepo, plateRepo, txRunner)

	var slotID int64
	require.NoError(t, conn.QueryRow(`SELECT id FROM time_slots LIMIT 1`).Scan(&slotID))

	start := time.Date(2026, 4, 25, 0, 0, 0, 0, time.UTC)
	res, err := svc.Apply(context.Background(), 1, template.ApplyPayload{
		Date:   &start,
		SlotID: &slotID,
	})
	require.NoError(t, err)
	require.Len(t, res.Created, 2, "legacy multi-offset template must produce one plate per offset")

	// Entry at day_offset=0 → 2026-04-25; entry at day_offset=6 → 2026-05-01.
	assert.Equal(t, "2026-04-25", res.Created[0].Date.Format("2006-01-02"))
	assert.Equal(t, "2026-05-01", res.Created[1].Date.Format("2006-01-02"))
	assert.Equal(t, slotID, res.Created[0].SlotID)
	assert.Equal(t, slotID, res.Created[1].SlotID)
}
