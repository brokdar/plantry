package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upDropWeekAggregate, downDropWeekAggregate)
}

// upDropWeekAggregate rebuilds the plates table without week_id and day columns,
// then drops the weeks table. ai_conversations.week_id is NULLed via FK ON DELETE SET NULL.
func upDropWeekAggregate(ctx context.Context, tx *sql.Tx) error {
	// Drop indexes that reference week_id or day first.
	for _, idx := range []string{"ix_plates_week_day_slot"} {
		if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS `+idx); err != nil {
			return fmt.Errorf("drop index %s: %w", idx, err)
		}
	}

	// Rebuild plates without week_id and day.
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE plates_new (
			id         INTEGER PRIMARY KEY,
			slot_id    INTEGER NOT NULL REFERENCES time_slots(id) ON DELETE RESTRICT,
			note       TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			skipped    INTEGER NOT NULL DEFAULT 0 CHECK (skipped IN (0, 1)),
			date       TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create plates_new: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO plates_new (id, slot_id, note, created_at, skipped, date)
		SELECT                  id, slot_id, note, created_at, skipped, date
		FROM plates
	`); err != nil {
		return fmt.Errorf("copy plates to plates_new: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DROP TABLE plates`); err != nil {
		return fmt.Errorf("drop old plates: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `ALTER TABLE plates_new RENAME TO plates`); err != nil {
		return fmt.Errorf("rename plates_new: %w", err)
	}

	// Recreate date indexes.
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS ix_plates_date ON plates(date)`); err != nil {
		return fmt.Errorf("create ix_plates_date: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS ix_plates_date_slot ON plates(date, slot_id)`); err != nil {
		return fmt.Errorf("create ix_plates_date_slot: %w", err)
	}

	// Rebuild ai_conversations without the REFERENCES weeks(id) FK.
	// week_id is kept as a plain nullable INTEGER so the agent can still group
	// conversations (the weeks table itself is going away).
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS ix_ai_conversations_week`); err != nil {
		return fmt.Errorf("drop ix_ai_conversations_week: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE ai_conversations_new (
			id         INTEGER PRIMARY KEY,
			week_id    INTEGER,
			title      TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		return fmt.Errorf("create ai_conversations_new: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ai_conversations_new (id, week_id, title, created_at, updated_at)
		SELECT id, week_id, title, created_at, updated_at FROM ai_conversations
	`); err != nil {
		return fmt.Errorf("copy ai_conversations: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DROP TABLE ai_conversations`); err != nil {
		return fmt.Errorf("drop old ai_conversations: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `ALTER TABLE ai_conversations_new RENAME TO ai_conversations`); err != nil {
		return fmt.Errorf("rename ai_conversations_new: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS ix_ai_conversations_week ON ai_conversations(week_id)`); err != nil {
		return fmt.Errorf("create ix_ai_conversations_week: %w", err)
	}

	// Drop weeks table (FK from ai_conversations is already removed above).
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS weeks`); err != nil {
		return fmt.Errorf("drop weeks table: %w", err)
	}

	return nil
}

// downDropWeekAggregate re-adds week_id/day to plates, restores the weeks table,
// and restores the FK from ai_conversations.week_id to weeks.
// This is a best-effort down migration — week data cannot be fully recovered.
func downDropWeekAggregate(ctx context.Context, tx *sql.Tx) error {
	// Recreate weeks table first (needed for FK references below).
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS weeks (
			id          INTEGER PRIMARY KEY,
			year        INTEGER NOT NULL,
			week_number INTEGER NOT NULL,
			created_at  TEXT    NOT NULL DEFAULT (datetime('now')),
			UNIQUE(year, week_number)
		)
	`); err != nil {
		return fmt.Errorf("create weeks: %w", err)
	}

	// Insert a placeholder week row so we can satisfy NOT NULL FK.
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO weeks (id, year, week_number) VALUES (0, 1970, 1)`); err != nil {
		return fmt.Errorf("insert placeholder week: %w", err)
	}

	// Restore FK from ai_conversations.week_id → weeks(id).
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS ix_ai_conversations_week`); err != nil {
		return fmt.Errorf("drop ix_ai_conversations_week: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE ai_conversations_new (
			id         INTEGER PRIMARY KEY,
			week_id    INTEGER REFERENCES weeks(id) ON DELETE SET NULL,
			title      TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`); err != nil {
		return fmt.Errorf("create ai_conversations_new: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ai_conversations_new (id, week_id, title, created_at, updated_at)
		SELECT id, week_id, title, created_at, updated_at FROM ai_conversations
	`); err != nil {
		return fmt.Errorf("copy ai_conversations: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DROP TABLE ai_conversations`); err != nil {
		return fmt.Errorf("drop old ai_conversations: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `ALTER TABLE ai_conversations_new RENAME TO ai_conversations`); err != nil {
		return fmt.Errorf("rename ai_conversations_new: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `CREATE INDEX ix_ai_conversations_week ON ai_conversations(week_id)`); err != nil {
		return fmt.Errorf("create ix_ai_conversations_week: %w", err)
	}

	// Rebuild plates with week_id and day restored.
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS ix_plates_date`); err != nil {
		return fmt.Errorf("drop ix_plates_date: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS ix_plates_date_slot`); err != nil {
		return fmt.Errorf("drop ix_plates_date_slot: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE plates_new (
			id         INTEGER PRIMARY KEY,
			week_id    INTEGER NOT NULL REFERENCES weeks(id) ON DELETE CASCADE,
			day        INTEGER NOT NULL CHECK (day BETWEEN 0 AND 6),
			slot_id    INTEGER NOT NULL REFERENCES time_slots(id) ON DELETE RESTRICT,
			note       TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			skipped    INTEGER NOT NULL DEFAULT 0 CHECK (skipped IN (0, 1)),
			date       TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create plates_new: %w", err)
	}

	// Re-derive week_id (placeholder 0) and day from the date column.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO plates_new (id, week_id, day, slot_id, note, created_at, skipped, date)
		SELECT id, 0, 0, slot_id, note, created_at, skipped, date
		FROM plates
	`); err != nil {
		return fmt.Errorf("copy plates to plates_new: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DROP TABLE plates`); err != nil {
		return fmt.Errorf("drop old plates: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `ALTER TABLE plates_new RENAME TO plates`); err != nil {
		return fmt.Errorf("rename plates_new: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `CREATE INDEX ix_plates_week_day_slot ON plates(week_id, day, slot_id)`); err != nil {
		return fmt.Errorf("create ix_plates_week_day_slot: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX ix_plates_date ON plates(date)`); err != nil {
		return fmt.Errorf("create ix_plates_date: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX ix_plates_date_slot ON plates(date, slot_id)`); err != nil {
		return fmt.Errorf("create ix_plates_date_slot: %w", err)
	}

	return nil
}
