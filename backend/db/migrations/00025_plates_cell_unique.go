package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upPlatesCellUnique, downPlatesCellUnique)
}

// upPlatesCellUnique turns skipped plates into "none" availability overrides,
// merges duplicate plates per (date, slot) into the lowest id, then drops
// plates.skipped and makes (date, slot_id) unique (Phase 1 D11).
func upPlatesCellUnique(ctx context.Context, tx *sql.Tx) error {
	// (1) A skipped plate becomes a "none" override carrying its note, unless
	// its cell also holds a non-skipped plate (that plate wins) or already
	// has an override. Several skipped plates in one cell: the lowest id's
	// note is used.
	res, err := tx.ExecContext(ctx, `
		INSERT INTO availability_overrides (date, slot_id, mode, note)
		SELECT s.date, s.slot_id, 'none', s.note
		FROM plates s
		WHERE s.skipped = 1
		  AND s.id = (SELECT MIN(x.id) FROM plates x
		              WHERE x.date = s.date AND x.slot_id = s.slot_id AND x.skipped = 1)
		  AND NOT EXISTS (SELECT 1 FROM plates p
		                  WHERE p.date = s.date AND p.slot_id = s.slot_id AND p.skipped = 0)
		ON CONFLICT (date, slot_id) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("convert skipped plates to overrides: %w", err)
	}
	overrides, err := res.RowsAffected()
	if err != nil {
		return err
	}

	res, err = tx.ExecContext(ctx, `DELETE FROM plates WHERE skipped = 1`)
	if err != nil {
		return fmt.Errorf("delete skipped plates: %w", err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return err
	}

	// (2) Merge duplicates per cell into the lowest id.
	merged, err := mergeDuplicatePlates(ctx, tx)
	if err != nil {
		return err
	}

	// (3) Drop the column (its only constraint is its own CHECK, so no table
	// rebuild is needed) and enforce one plate per cell.
	if _, err := tx.ExecContext(ctx, `ALTER TABLE plates DROP COLUMN skipped`); err != nil {
		return fmt.Errorf("drop plates.skipped: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX ux_plates_date_slot ON plates(date, slot_id)`); err != nil {
		return fmt.Errorf("create ux_plates_date_slot: %w", err)
	}

	slog.InfoContext(ctx, "migration.00025",
		"overrides_created", overrides,
		"skipped_plates_deleted", deleted,
		"duplicates_merged", merged)
	return nil
}

type plateCell struct {
	date     string
	slotID   int64
	survivor int64
}

// mergeDuplicatePlates re-parents the components of every extra plate in a
// cell onto the cell's lowest-id plate, appending them after the survivor's
// components in (plate id, sort_order, id) order, and deletes the extras
// (cascading their feedback). The survivor keeps its note, or takes the
// first non-null note of the extras when it has none. It returns the number
// of plates deleted.
func mergeDuplicatePlates(ctx context.Context, tx *sql.Tx) (int64, error) {
	cells, err := queryCells(ctx, tx)
	if err != nil {
		return 0, err
	}
	var merged int64
	for _, c := range cells {
		extras, err := queryInt64s(ctx, tx,
			`SELECT id FROM plates WHERE date = ? AND slot_id = ? AND id <> ? ORDER BY id`,
			c.date, c.slotID, c.survivor)
		if err != nil {
			return 0, fmt.Errorf("list duplicates of plate %d: %w", c.survivor, err)
		}
		// A survivor without a note takes the first non-null note of its
		// duplicates, by id.
		if _, err := tx.ExecContext(ctx, `
			UPDATE plates SET note = (
				SELECT d.note FROM plates d
				WHERE d.date = ? AND d.slot_id = ? AND d.id <> ? AND d.note IS NOT NULL
				ORDER BY d.id LIMIT 1)
			WHERE id = ? AND note IS NULL`,
			c.date, c.slotID, c.survivor, c.survivor); err != nil {
			return 0, fmt.Errorf("merge note into plate %d: %w", c.survivor, err)
		}
		var next int64
		if err := tx.QueryRowContext(ctx,
			`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM plate_components WHERE plate_id = ?`,
			c.survivor).Scan(&next); err != nil {
			return 0, fmt.Errorf("max sort_order of plate %d: %w", c.survivor, err)
		}
		for _, extra := range extras {
			comps, err := queryInt64s(ctx, tx,
				`SELECT id FROM plate_components WHERE plate_id = ? ORDER BY sort_order, id`, extra)
			if err != nil {
				return 0, fmt.Errorf("list components of plate %d: %w", extra, err)
			}
			for _, compID := range comps {
				if _, err := tx.ExecContext(ctx,
					`UPDATE plate_components SET plate_id = ?, sort_order = ? WHERE id = ?`,
					c.survivor, next, compID); err != nil {
					return 0, fmt.Errorf("re-parent component %d: %w", compID, err)
				}
				next++
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM plates WHERE id = ?`, extra); err != nil {
				return 0, fmt.Errorf("delete duplicate plate %d: %w", extra, err)
			}
			merged++
		}
	}
	return merged, nil
}

func queryCells(ctx context.Context, tx *sql.Tx) ([]plateCell, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT date, slot_id, MIN(id) FROM plates
		GROUP BY date, slot_id HAVING COUNT(*) > 1
		ORDER BY date, slot_id`)
	if err != nil {
		return nil, fmt.Errorf("find duplicate cells: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []plateCell
	for rows.Next() {
		var c plateCell
		if err := rows.Scan(&c.date, &c.slotID, &c.survivor); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func queryInt64s(ctx context.Context, tx *sql.Tx, q string, args ...any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// downPlatesCellUnique drops the unique index and re-adds skipped (default 0).
// Converted overrides stay; skipped plates are not recreated.
func downPlatesCellUnique(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `DROP INDEX IF EXISTS ux_plates_date_slot`); err != nil {
		return fmt.Errorf("drop ux_plates_date_slot: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`ALTER TABLE plates ADD COLUMN skipped INTEGER NOT NULL DEFAULT 0 CHECK (skipped IN (0, 1))`); err != nil {
		return fmt.Errorf("re-add plates.skipped: %w", err)
	}
	return nil
}
