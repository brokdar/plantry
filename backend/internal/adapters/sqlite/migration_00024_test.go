package sqlite_test

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

func seedAvailabilitySlotAndPreset(t *testing.T, conn *sql.DB) (slotID, presetID int64) {
	t.Helper()
	res, err := conn.Exec(`INSERT INTO time_slots (name_key, icon) VALUES ('slot.breakfast', 'Coffee')`)
	require.NoError(t, err)
	slotID, err = res.LastInsertId()
	require.NoError(t, err)
	res, err = conn.Exec(`INSERT INTO presets (name) VALUES ('Oatmeal')`)
	require.NoError(t, err)
	presetID, err = res.LastInsertId()
	require.NoError(t, err)
	return slotID, presetID
}

func countAvailabilityRows(t *testing.T, conn *sql.DB, table string) int {
	t.Helper()
	var n int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n))
	return n
}

func TestMigration00024_TablesExist(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	for _, table := range []string{"availability_defaults", "availability_overrides"} {
		var name string
		err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		require.NoError(t, err, table)
	}
}

func TestMigration00024_CheckConstraints(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	slotID, presetID := seedAvailabilitySlotAndPreset(t, conn)

	tests := []struct {
		name    string
		query   string
		args    []any
		wantErr bool
	}{
		{"default routine without preset", `INSERT INTO availability_defaults (weekday, slot_id, mode) VALUES (0, ?, 'routine')`, []any{slotID}, true},
		{"default none with preset", `INSERT INTO availability_defaults (weekday, slot_id, mode, preset_id) VALUES (1, ?, 'none', ?)`, []any{slotID, presetID}, true},
		{"default weekday out of range", `INSERT INTO availability_defaults (weekday, slot_id, mode) VALUES (7, ?, 'none')`, []any{slotID}, true},
		{"default bad mode", `INSERT INTO availability_defaults (weekday, slot_id, mode) VALUES (2, ?, 'away')`, []any{slotID}, true},
		{"default routine with preset", `INSERT INTO availability_defaults (weekday, slot_id, mode, preset_id) VALUES (3, ?, 'routine', ?)`, []any{slotID, presetID}, false},
		{"override routine without preset", `INSERT INTO availability_overrides (date, slot_id, mode) VALUES ('2026-10-26', ?, 'routine')`, []any{slotID}, true},
		{"override none with preset", `INSERT INTO availability_overrides (date, slot_id, mode, preset_id) VALUES ('2026-10-27', ?, 'none', ?)`, []any{slotID, presetID}, true},
		{"override none with note", `INSERT INTO availability_overrides (date, slot_id, mode, note) VALUES ('2026-10-28', ?, 'none', 'away')`, []any{slotID}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := conn.Exec(tt.query, tt.args...)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMigration00024_PresetDeleteCascades(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	slotID, presetID := seedAvailabilitySlotAndPreset(t, conn)
	_, err := conn.Exec(`INSERT INTO availability_defaults (weekday, slot_id, mode, preset_id) VALUES (0, ?, 'routine', ?)`, slotID, presetID)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO availability_overrides (date, slot_id, mode, preset_id) VALUES ('2026-10-26', ?, 'routine', ?)`, slotID, presetID)
	require.NoError(t, err)

	_, err = conn.Exec(`DELETE FROM presets WHERE id = ?`, presetID)
	require.NoError(t, err)
	assert.Equal(t, 0, countAvailabilityRows(t, conn, "availability_defaults"))
	assert.Equal(t, 0, countAvailabilityRows(t, conn, "availability_overrides"))
}

func TestMigration00024_SlotDeleteCascades(t *testing.T) {
	conn := testhelper.NewTestDB(t)
	slotID, _ := seedAvailabilitySlotAndPreset(t, conn)
	_, err := conn.Exec(`INSERT INTO availability_defaults (weekday, slot_id, mode) VALUES (0, ?, 'none')`, slotID)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO availability_overrides (date, slot_id, mode) VALUES ('2026-10-26', ?, 'none')`, slotID)
	require.NoError(t, err)

	_, err = conn.Exec(`DELETE FROM time_slots WHERE id = ?`, slotID)
	require.NoError(t, err)
	assert.Equal(t, 0, countAvailabilityRows(t, conn, "availability_defaults"))
	assert.Equal(t, 0, countAvailabilityRows(t, conn, "availability_overrides"))
}
