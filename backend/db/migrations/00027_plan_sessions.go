package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upPlanSessions, downPlanSessions)
}

// upPlanSessions introduces cook sessions (Phase 2 D8–D10, D15):
//
//  1. cook_sessions
//  2. plates.eaters (= the profile's household_portions) and plates.updated_at
//  3. plate_components rebuilt with REAL portions and cook_session_id, where
//     every composed row must link a session and leaf rows must not
//  4. one fresh session per existing composed component, with portions =
//     component portions × plate eaters
//  5. preset_components rebuilt with REAL portions
//  6. foods.keep_days and foods.freezable (composed only)
//
// Sessions are created before the rebuild (step 4 runs against the old
// table) because the new CHECK rejects a composed row without a session.
func upPlanSessions(ctx context.Context, tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE cook_sessions (
			id              INTEGER PRIMARY KEY,
			food_id         INTEGER NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
			cook_date       TEXT NOT NULL,
			cook_slot_id    INTEGER NOT NULL REFERENCES time_slots(id) ON DELETE RESTRICT,
			portions        REAL NOT NULL CHECK (portions > 0),
			frozen_portions REAL NOT NULL DEFAULT 0 CHECK (frozen_portions >= 0 AND frozen_portions <= portions),
			cooked_at       TEXT,
			note            TEXT,
			created_at      TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE INDEX ix_cook_sessions_cook_date ON cook_sessions(cook_date)`,
		`CREATE INDEX ix_cook_sessions_food ON cook_sessions(food_id)`,

		`ALTER TABLE plates ADD COLUMN eaters REAL NOT NULL DEFAULT 1 CHECK (eaters > 0)`,
		`ALTER TABLE plates ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''`,
		`UPDATE plates SET
			eaters = COALESCE((SELECT household_portions FROM user_profile WHERE id = 1), 1),
			updated_at = created_at`,
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("00027: %w", err)
		}
	}

	sessions, err := createSessionsForComposed(ctx, tx)
	if err != nil {
		return err
	}

	stmts = []string{
		`CREATE TABLE plate_components_new (
			id              INTEGER PRIMARY KEY,
			plate_id        INTEGER NOT NULL REFERENCES plates(id) ON DELETE CASCADE,
			food_id         INTEGER NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
			portions        REAL,
			amount          REAL,
			unit            TEXT,
			grams           REAL,
			grams_source    TEXT,
			cook_session_id INTEGER REFERENCES cook_sessions(id) ON DELETE RESTRICT,
			sort_order      INTEGER NOT NULL DEFAULT 0,
			CHECK (
				(portions IS NOT NULL AND portions > 0 AND cook_session_id IS NOT NULL
					AND amount IS NULL AND unit IS NULL AND grams IS NULL AND grams_source IS NULL)
				OR
				(amount IS NOT NULL AND amount > 0
					AND unit IS NOT NULL AND unit <> ''
					AND grams IS NOT NULL AND grams >= 0
					AND portions IS NULL AND cook_session_id IS NULL)
			)
		)`,
		`INSERT INTO plate_components_new
			(id, plate_id, food_id, portions, amount, unit, grams, grams_source, cook_session_id, sort_order)
		 SELECT id, plate_id, food_id, CAST(portions AS REAL), amount, unit, grams, grams_source, cook_session_id, sort_order
		 FROM plate_components`,
		`DROP INDEX IF EXISTS ix_plate_components_plate`,
		`DROP TABLE plate_components`,
		`ALTER TABLE plate_components_new RENAME TO plate_components`,
		`CREATE INDEX ix_plate_components_plate ON plate_components(plate_id)`,
		`CREATE INDEX ix_plate_components_session ON plate_components(cook_session_id)`,

		`CREATE TABLE preset_components_new (
			id              INTEGER PRIMARY KEY,
			preset_plate_id INTEGER NOT NULL REFERENCES preset_plates(id) ON DELETE CASCADE,
			food_id         INTEGER NOT NULL REFERENCES foods(id)         ON DELETE RESTRICT,
			portions        REAL,
			amount          REAL,
			unit            TEXT,
			grams           REAL,
			grams_source    TEXT,
			note            TEXT,
			sort_order      INTEGER NOT NULL DEFAULT 0,
			CHECK (
				(portions IS NOT NULL AND portions > 0
					AND amount IS NULL AND unit IS NULL AND grams IS NULL AND grams_source IS NULL)
				OR
				(portions IS NULL
					AND amount IS NOT NULL AND amount > 0
					AND unit IS NOT NULL AND unit <> ''
					AND grams IS NOT NULL AND grams >= 0)
			)
		)`,
		`INSERT INTO preset_components_new
			(id, preset_plate_id, food_id, portions, amount, unit, grams, grams_source, note, sort_order)
		 SELECT id, preset_plate_id, food_id, CAST(portions AS REAL), amount, unit, grams, grams_source, note, sort_order
		 FROM preset_components`,
		`DROP INDEX IF EXISTS ix_preset_components_plate`,
		`DROP TABLE preset_components`,
		`ALTER TABLE preset_components_new RENAME TO preset_components`,
		`CREATE INDEX ix_preset_components_plate ON preset_components(preset_plate_id)`,

		`ALTER TABLE foods ADD COLUMN keep_days INTEGER
			CHECK (keep_days IS NULL OR (kind = 'composed' AND keep_days BETWEEN 1 AND 7))`,
		`ALTER TABLE foods ADD COLUMN freezable INTEGER
			CHECK (freezable IS NULL OR (kind = 'composed' AND freezable IN (0, 1)))`,
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("00027: %w", err)
		}
	}
	slog.InfoContext(ctx, "migration.00027", "sessions_created", sessions)
	return nil
}

// createSessionsForComposed adds cook_session_id to the old plate_components
// table and links every composed row to a new fresh session at its plate's
// cell. Rows are processed in cell order (date, slot sort_order, slot id),
// then component id, so session ids follow the plan.
func createSessionsForComposed(ctx context.Context, tx *sql.Tx) (int, error) {
	if _, err := tx.ExecContext(ctx, `ALTER TABLE plate_components ADD COLUMN cook_session_id INTEGER`); err != nil {
		return 0, fmt.Errorf("00027: add cook_session_id: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT pc.id, pc.food_id, p.date, p.slot_id, pc.portions * p.eaters
		FROM plate_components pc
		JOIN plates p ON p.id = pc.plate_id
		JOIN time_slots ts ON ts.id = p.slot_id
		WHERE pc.portions IS NOT NULL
		ORDER BY p.date, ts.sort_order, ts.id, pc.id`)
	if err != nil {
		return 0, fmt.Errorf("00027: list composed components: %w", err)
	}
	type composed struct {
		id, foodID, slotID int64
		date               string
		portions           float64
	}
	var all []composed
	for rows.Next() {
		var c composed
		if err := rows.Scan(&c.id, &c.foodID, &c.date, &c.slotID, &c.portions); err != nil {
			_ = rows.Close()
			return 0, err
		}
		all = append(all, c)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, c := range all {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO cook_sessions (food_id, cook_date, cook_slot_id, portions, frozen_portions)
			VALUES (?, ?, ?, ?, 0)`, c.foodID, c.date, c.slotID, c.portions)
		if err != nil {
			return 0, fmt.Errorf("00027: create session for component %d: %w", c.id, err)
		}
		sid, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE plate_components SET cook_session_id = ? WHERE id = ?`, sid, c.id); err != nil {
			return 0, fmt.Errorf("00027: link component %d: %w", c.id, err)
		}
	}
	return len(all), nil
}

// downPlanSessions reverses the schema. It is lossy: batches cannot be
// represented without sessions, so every session is dropped (a leftover
// plate keeps its component, which no longer knows where it was cooked),
// session portions, frozen portions and cooked_at are lost, and fractional
// component portions are rounded half-up to an integer with a minimum of 1.
func downPlanSessions(ctx context.Context, tx *sql.Tx) error {
	stmts := []string{
		`ALTER TABLE foods DROP COLUMN freezable`,
		`ALTER TABLE foods DROP COLUMN keep_days`,

		`CREATE TABLE preset_components_old (
			id              INTEGER PRIMARY KEY,
			preset_plate_id INTEGER NOT NULL REFERENCES preset_plates(id) ON DELETE CASCADE,
			food_id         INTEGER NOT NULL REFERENCES foods(id)         ON DELETE RESTRICT,
			portions        INTEGER,
			amount          REAL,
			unit            TEXT,
			grams           REAL,
			grams_source    TEXT,
			note            TEXT,
			sort_order      INTEGER NOT NULL DEFAULT 0,
			CHECK (
				(portions IS NOT NULL AND portions > 0
					AND amount IS NULL AND unit IS NULL AND grams IS NULL AND grams_source IS NULL)
				OR
				(portions IS NULL
					AND amount IS NOT NULL AND amount > 0
					AND unit IS NOT NULL AND unit != ''
					AND grams IS NOT NULL AND grams >= 0)
			)
		)`,
		`INSERT INTO preset_components_old
			(id, preset_plate_id, food_id, portions, amount, unit, grams, grams_source, note, sort_order)
		 SELECT id, preset_plate_id, food_id,
		        CASE WHEN portions IS NULL THEN NULL ELSE MAX(1, CAST(portions + 0.5 AS INTEGER)) END,
		        amount, unit, grams, grams_source, note, sort_order
		 FROM preset_components`,
		`DROP INDEX IF EXISTS ix_preset_components_plate`,
		`DROP TABLE preset_components`,
		`ALTER TABLE preset_components_old RENAME TO preset_components`,
		`CREATE INDEX ix_preset_components_plate ON preset_components(preset_plate_id)`,

		`CREATE TABLE plate_components_old (
			id            INTEGER PRIMARY KEY,
			plate_id      INTEGER NOT NULL REFERENCES plates(id) ON DELETE CASCADE,
			food_id       INTEGER NOT NULL REFERENCES foods(id) ON DELETE RESTRICT,
			portions      INTEGER,
			amount        REAL,
			unit          TEXT,
			grams         REAL,
			grams_source  TEXT,
			sort_order    INTEGER NOT NULL DEFAULT 0,
			CHECK (
				(portions IS NOT NULL AND portions > 0
					AND amount IS NULL AND unit IS NULL AND grams IS NULL AND grams_source IS NULL)
				OR
				(portions IS NULL
					AND amount IS NOT NULL AND amount > 0
					AND unit IS NOT NULL AND unit != ''
					AND grams IS NOT NULL AND grams >= 0)
			)
		)`,
		`INSERT INTO plate_components_old
			(id, plate_id, food_id, portions, amount, unit, grams, grams_source, sort_order)
		 SELECT id, plate_id, food_id,
		        CASE WHEN portions IS NULL THEN NULL ELSE MAX(1, CAST(portions + 0.5 AS INTEGER)) END,
		        amount, unit, grams, grams_source, sort_order
		 FROM plate_components`,
		`DROP INDEX IF EXISTS ix_plate_components_session`,
		`DROP INDEX IF EXISTS ix_plate_components_plate`,
		`DROP TABLE plate_components`,
		`ALTER TABLE plate_components_old RENAME TO plate_components`,
		`CREATE INDEX ix_plate_components_plate ON plate_components(plate_id)`,

		`ALTER TABLE plates DROP COLUMN updated_at`,
		`ALTER TABLE plates DROP COLUMN eaters`,

		`DROP INDEX IF EXISTS ix_cook_sessions_food`,
		`DROP INDEX IF EXISTS ix_cook_sessions_cook_date`,
		`DROP TABLE cook_sessions`,
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("00027 down: %w", err)
		}
	}
	return nil
}
