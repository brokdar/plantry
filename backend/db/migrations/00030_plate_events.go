package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationNoTxContext(upPlateEvents, downPlateEvents)
}

// upPlateEvents adds the plate audit trail (Phase 2 D20) and makes plate and
// cook-session ids unique for the life of the database: plates and
// cook_sessions are rebuilt with AUTOINCREMENT so SQLite never hands a
// deleted row's id to a new row (an event's plate_id or cook_session_id, and
// an undo snapshot's {id} session link, always name one entity).
//
// The rebuild follows SQLite's "12-step" table change: foreign keys off on a
// pinned connection (the pragma is a no-op inside a transaction, and with
// keys on DROP TABLE plates would cascade-delete plate_components and
// plate_feedback), the copy in one transaction, then foreign_key_check
// before the commit. Children keep their textual REFERENCES plates(id) /
// cook_sessions(id), which bind to the rebuilt tables.
func upPlateEvents(ctx context.Context, db *sql.DB) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("00030: pin connection: %w", err)
	}
	// The pool holds one connection: release it before goose records the
	// version.
	defer func() { err = errors.Join(err, conn.Close()) }()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("00030: foreign keys off: %w", err)
	}
	defer func() {
		if _, onErr := conn.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys = ON`); onErr != nil {
			err = errors.Join(err, fmt.Errorf("00030: foreign keys on: %w", onErr))
		}
	}()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("00030: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, s := range plateEventsUp {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("00030: %w", err)
		}
	}
	if err := checkForeignKeys(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("00030: commit: %w", err)
	}
	return nil
}

var plateEventsUp = []string{
	`CREATE TABLE plates_new (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		slot_id    INTEGER NOT NULL REFERENCES time_slots(id) ON DELETE RESTRICT,
		note       TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		date       TEXT NOT NULL,
		eaters     REAL NOT NULL DEFAULT 1 CHECK (eaters > 0),
		updated_at TEXT NOT NULL DEFAULT ''
	)`,
	`INSERT INTO plates_new (id, slot_id, note, created_at, date, eaters, updated_at)
		SELECT id, slot_id, note, created_at, date, eaters, updated_at FROM plates`,
	`DROP TABLE plates`,
	`ALTER TABLE plates_new RENAME TO plates`,
	`CREATE INDEX ix_plates_date ON plates(date)`,
	`CREATE INDEX ix_plates_date_slot ON plates(date, slot_id)`,
	`CREATE UNIQUE INDEX ux_plates_date_slot ON plates(date, slot_id)`,

	`CREATE TABLE cook_sessions_new (
		id              INTEGER PRIMARY KEY AUTOINCREMENT,
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
	`INSERT INTO cook_sessions_new (id, food_id, cook_date, cook_slot_id, portions, frozen_portions, cooked_at, note, created_at, updated_at)
		SELECT id, food_id, cook_date, cook_slot_id, portions, frozen_portions, cooked_at, note, created_at, updated_at FROM cook_sessions`,
	`DROP TABLE cook_sessions`,
	`ALTER TABLE cook_sessions_new RENAME TO cook_sessions`,
	`CREATE INDEX ix_cook_sessions_cook_date ON cook_sessions(cook_date)`,
	`CREATE INDEX ix_cook_sessions_food ON cook_sessions(food_id)`,

	// No foreign keys, so rows outlive their plates, sessions, slots and AI
	// runs. The service validates action and actor (Phase 3 adds values),
	// so there are no CHECK constraints to rebuild later.
	`CREATE TABLE plate_events (
		id              INTEGER PRIMARY KEY,
		plate_id        INTEGER,
		cook_session_id INTEGER,
		date            TEXT    NOT NULL,
		slot_id         INTEGER NOT NULL,
		action          TEXT    NOT NULL,
		actor           TEXT    NOT NULL,
		ai_run_id       INTEGER,
		before_json     TEXT,
		after_json      TEXT,
		at              TEXT    NOT NULL DEFAULT (datetime('now'))
	)`,
	`CREATE INDEX ix_plate_events_plate ON plate_events(plate_id)`,
	`CREATE INDEX ix_plate_events_run ON plate_events(ai_run_id)`,
}

// checkForeignKeys fails when any row violates a foreign key.
func checkForeignKeys(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("00030: foreign_key_check: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var table string
		var rowid sql.NullInt64
		var parent string
		var fkid int64
		if err := rows.Scan(&table, &rowid, &parent, &fkid); err != nil {
			return fmt.Errorf("00030: foreign_key_check: %w", err)
		}
		return fmt.Errorf("00030: foreign key violation: %s row %d → %s", table, rowid.Int64, parent)
	}
	return rows.Err()
}

// downPlateEvents drops the audit trail. plates and cook_sessions keep
// AUTOINCREMENT: it is harmless to the older schema and rebuilding them
// back would only reintroduce id reuse.
func downPlateEvents(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `DROP TABLE plate_events`); err != nil {
		return fmt.Errorf("00030 down: %w", err)
	}
	return nil
}
