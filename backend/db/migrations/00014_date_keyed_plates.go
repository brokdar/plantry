package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upDateKeyedPlates, downDateKeyedPlates)
}

func upDateKeyedPlates(ctx context.Context, tx *sql.Tx) error {
	// Step 1: add the nullable date column.
	if _, err := tx.ExecContext(ctx, `ALTER TABLE plates ADD COLUMN date TEXT`); err != nil {
		return fmt.Errorf("add date column: %w", err)
	}

	// Step 2: query all plates joined to weeks so we can compute dates.
	rows, err := tx.QueryContext(ctx, `
		SELECT p.id, w.year, w.week_number, p.day
		FROM plates p
		JOIN weeks w ON w.id = p.week_id
	`)
	if err != nil {
		return fmt.Errorf("query plates: %w", err)
	}

	type update struct {
		id   int64
		date string
	}
	var updates []update
	for rows.Next() {
		var id int64
		var year, week, day int
		if err := rows.Scan(&id, &year, &week, &day); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan plate row: %w", err)
		}
		d := IsoWeekStart(year, week).AddDate(0, 0, day)
		updates = append(updates, update{id: id, date: d.Format("2006-01-02")})
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close rows: %w", err)
	}

	// Step 3: write the computed date back to each plate.
	for _, u := range updates {
		if _, err := tx.ExecContext(ctx,
			`UPDATE plates SET date = ? WHERE id = ?`, u.date, u.id,
		); err != nil {
			return fmt.Errorf("update plate %d: %w", u.id, err)
		}
	}

	// Step 4: verify no NULLs remain (catches orphaned week_ids).
	var nullCount int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM plates WHERE date IS NULL`,
	).Scan(&nullCount); err != nil {
		return fmt.Errorf("check null dates: %w", err)
	}
	if nullCount > 0 {
		return fmt.Errorf("migration aborted: %d plate(s) have NULL date (orphaned week_id)", nullCount)
	}

	// Step 5: rebuild the table to enforce NOT NULL on date.
	// SQLite does not support ALTER COLUMN; the only way to add a NOT NULL
	// constraint on an existing column is to recreate the table.
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

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO plates_new (id, week_id, day, slot_id, note, created_at, skipped, date)
		SELECT                  id, week_id, day, slot_id, note, created_at, skipped, date
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

	// Step 6: recreate the original index and add the new date indexes.
	if _, err := tx.ExecContext(ctx,
		`CREATE INDEX ix_plates_week_day_slot ON plates(week_id, day, slot_id)`,
	); err != nil {
		return fmt.Errorf("create ix_plates_week_day_slot: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`CREATE INDEX ix_plates_date ON plates(date)`,
	); err != nil {
		return fmt.Errorf("create ix_plates_date: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`CREATE INDEX ix_plates_date_slot ON plates(date, slot_id)`,
	); err != nil {
		return fmt.Errorf("create ix_plates_date_slot: %w", err)
	}

	return nil
}

func downDateKeyedPlates(ctx context.Context, tx *sql.Tx) error {
	// Drop the date-related indexes first.
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS ix_plates_date`); err != nil {
		return fmt.Errorf("drop ix_plates_date: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS ix_plates_date_slot`); err != nil {
		return fmt.Errorf("drop ix_plates_date_slot: %w", err)
	}

	// Rebuild the table without the date column.
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE plates_new (
			id         INTEGER PRIMARY KEY,
			week_id    INTEGER NOT NULL REFERENCES weeks(id) ON DELETE CASCADE,
			day        INTEGER NOT NULL CHECK (day BETWEEN 0 AND 6),
			slot_id    INTEGER NOT NULL REFERENCES time_slots(id) ON DELETE RESTRICT,
			note       TEXT,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			skipped    INTEGER NOT NULL DEFAULT 0 CHECK (skipped IN (0, 1))
		)
	`); err != nil {
		return fmt.Errorf("create plates_new: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO plates_new (id, week_id, day, slot_id, note, created_at, skipped)
		SELECT                  id, week_id, day, slot_id, note, created_at, skipped
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

	// Recreate the original index.
	if _, err := tx.ExecContext(ctx,
		`CREATE INDEX ix_plates_week_day_slot ON plates(week_id, day, slot_id)`,
	); err != nil {
		return fmt.Errorf("create ix_plates_week_day_slot: %w", err)
	}

	return nil
}

// IsoWeekStart returns the Monday that begins ISO week `week` of `year`.
//
// Algorithm: Jan 4 of the given year is always in ISO week 1.
// Find that week's Monday, then add (week-1)*7 days.
func IsoWeekStart(year, week int) time.Time {
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	// Sunday=0→6, Monday=1→0, Tuesday=2→1, ..., Saturday=6→5
	daysFromMonday := int(jan4.Weekday()+6) % 7
	week1Monday := jan4.AddDate(0, 0, -daysFromMonday)
	return week1Monday.AddDate(0, 0, (week-1)*7)
}
