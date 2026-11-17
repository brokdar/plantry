package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

func init() {
	goose.AddMigrationContext(upPlatePosition, downPlatePosition)
}

// upPlatePosition adds plate_components.position (main, side or extra), the
// address of "the main" and "a side" on a plate, and backfills it per plate in
// component order: the first composed main, standalone or role-less
// component is the main; other composed components are sides, except dessert
// and drink (extra); a leaf is a side when its effective role is a side role,
// else extra. It is a Go migration because the leaf's effective role needs
// the name classifier; sqlc reads the end state from
// schema_patches/00048_plate_position.sql.
func upPlatePosition(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `ALTER TABLE plate_components ADD COLUMN position TEXT NOT NULL DEFAULT 'side' CHECK (position IN ('main','side','extra'))`); err != nil {
		return fmt.Errorf("add position: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
SELECT pc.id, pc.plate_id, f.kind, f.role, f.name, f.shop_section
FROM plate_components pc
JOIN foods f ON f.id = pc.food_id
ORDER BY pc.plate_id, pc.sort_order, pc.id`)
	if err != nil {
		return fmt.Errorf("read components: %w", err)
	}
	type update struct {
		id       int64
		position string
	}
	var updates []update
	var curPlate int64 = -1
	hasMain := false
	for rows.Next() {
		var (
			id, plateID int64
			kind, name  string
			role, sec   sql.NullString
		)
		if err := rows.Scan(&id, &plateID, &kind, &role, &name, &sec); err != nil {
			_ = rows.Close()
			return err
		}
		if plateID != curPlate {
			curPlate, hasMain = plateID, false
		}
		pos := "side"
		if kind == "composed" {
			switch role.String {
			case "dessert", "drink":
				pos = "extra"
			case "", "main", "standalone":
				if !hasMain {
					pos, hasMain = "main", true
				}
			}
		} else {
			r := role.String
			if r == "" {
				r = leafRoleBySection(name, sec)
			}
			switch r {
			case "side_starch", "side_veg", "side_protein":
			default:
				pos = "extra"
			}
		}
		if pos != "side" {
			updates = append(updates, update{id, pos})
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, u := range updates {
		if _, err := tx.ExecContext(ctx, `UPDATE plate_components SET position = ? WHERE id = ?`, u.position, u.id); err != nil {
			return fmt.Errorf("set position %d: %w", u.id, err)
		}
	}
	return nil
}

// leafRoleBySection derives a leaf's role from its store section: produce is
// a vegetable side, bakery a starch side, meat and fish a protein side.
func leafRoleBySection(name string, override sql.NullString) string {
	section := shopping.Classify(name)
	if override.Valid {
		section = food.ShopSection(override.String)
	}
	switch section {
	case food.SectionProduce:
		return "side_veg"
	case food.SectionBakery:
		return "side_starch"
	case food.SectionMeatFish:
		return "side_protein"
	}
	return ""
}

func downPlatePosition(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `ALTER TABLE plate_components DROP COLUMN position`)
	return err
}
