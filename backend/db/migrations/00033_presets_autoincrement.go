package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationNoTxContext(upPresetsAutoincrement, downPresetsAutoincrement)
}

// upPresetsAutoincrement rebuilds presets with AUTOINCREMENT so SQLite never
// hands a deleted preset's id to a new one (a stale routine or undo payload
// then names nothing instead of the wrong preset).
//
// Same safe rebuild as 00030: foreign keys off on a pinned connection (with
// keys on, DROP TABLE presets would cascade-delete preset_plates,
// preset_tags and the routine availability rows), the copy in one
// transaction, foreign_key_check before the commit. Dropping presets drops
// its search triggers, so they are recreated; presets_fts keeps its rows
// because the ids do not change.
func upPresetsAutoincrement(ctx context.Context, db *sql.DB) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("00033: pin connection: %w", err)
	}
	defer func() { err = errors.Join(err, conn.Close()) }()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("00033: foreign keys off: %w", err)
	}
	defer func() {
		if _, onErr := conn.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys = ON`); onErr != nil {
			err = errors.Join(err, fmt.Errorf("00033: foreign keys on: %w", onErr))
		}
	}()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("00033: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, s := range presetsAutoincrementUp {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("00033: %w", err)
		}
	}
	if err := checkForeignKeys(ctx, tx, "00033"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("00033: commit: %w", err)
	}
	return nil
}

var presetsAutoincrementUp = []string{
	`CREATE TABLE presets_new (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		name         TEXT NOT NULL,
		created_at   TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at   TEXT NOT NULL DEFAULT (datetime('now')),
		last_used_at TEXT
	)`,
	`INSERT INTO presets_new (id, name, created_at, updated_at, last_used_at)
		SELECT id, name, created_at, updated_at, last_used_at FROM presets`,
	`DROP TABLE presets`,
	`ALTER TABLE presets_new RENAME TO presets`,
	`CREATE TRIGGER presets_ai AFTER INSERT ON presets BEGIN
		INSERT INTO presets_fts(rowid, name) VALUES (new.id, new.name);
	END`,
	`CREATE TRIGGER presets_ad AFTER DELETE ON presets BEGIN
		INSERT INTO presets_fts(presets_fts, rowid, name) VALUES('delete', old.id, old.name);
	END`,
	`CREATE TRIGGER presets_au AFTER UPDATE ON presets BEGIN
		INSERT INTO presets_fts(presets_fts, rowid, name) VALUES('delete', old.id, old.name);
		INSERT INTO presets_fts(rowid, name) VALUES (new.id, new.name);
	END`,
}

// downPresetsAutoincrement keeps AUTOINCREMENT: it is harmless to the older
// schema, and rebuilding back would only reintroduce id reuse.
func downPresetsAutoincrement(context.Context, *sql.DB) error {
	return nil
}
