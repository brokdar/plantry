package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/jaltszeimer/plantry/backend/db"
)

// TestMigration00013_UnifiedFood verifies that migrating ingredients + components
// into the unified foods schema preserves rows, grams sums, and referential
// integrity.
func TestMigration00013_UnifiedFood(t *testing.T) {
	path := filepath.Join(t.TempDir(), "food-migration.db")
	conn, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	goose.SetBaseFS(db.Migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))

	// Apply migrations 1-12 (pre-unified-food).
	require.NoError(t, goose.UpTo(conn, "migrations", 12))

	// ── Seed ──────────────────────────────────────────────────────────
	// 3 ingredients
	_, err = conn.Exec(`
        INSERT INTO ingredients (name, source, kcal_100g, protein_100g, fat_100g, carbs_100g, fiber_100g, sodium_100g, calcium_100g)
        VALUES
            ('Apple',  'manual', 52, 0.3, 0.2, 14, 2.4, 0.001, 6),
            ('Chicken', 'fdc',   165, 31, 3.6, 0, 0, 0.074, 15),
            ('Rice',   'manual', 130, 2.7, 0.3, 28, 0.4, 0.005, 10);
    `)
	require.NoError(t, err)

	// Portions
	_, err = conn.Exec(`
        INSERT INTO ingredient_portions (ingredient_id, unit, grams)
        VALUES
            (1, 'piece', 180),
            (2, 'breast', 170),
            (3, 'cup',   158);
    `)
	require.NoError(t, err)

	// 2 components, one with a variant group
	_, err = conn.Exec(`INSERT INTO variant_groups (name) VALUES ('CurryFam')`)
	require.NoError(t, err)

	_, err = conn.Exec(`
        INSERT INTO components (name, role, variant_group_id, reference_portions, prep_minutes, cook_minutes, notes, cook_count, favorite)
        VALUES
            ('Chicken Curry', 'main', 1, 2, 10, 30, 'Spicy', 3, 1),
            ('Rice Side',     'side_starch', NULL, 1, 5, 20, NULL, 0, 0);
    `)
	require.NoError(t, err)

	// Component ingredients (curry uses chicken + rice, side uses just rice)
	_, err = conn.Exec(`
        INSERT INTO component_ingredients (component_id, ingredient_id, amount, unit, grams, sort_order)
        VALUES
            (1, 2, 300, 'g', 300, 0),
            (1, 3, 200, 'g', 200, 1),
            (2, 3, 150, 'g', 150, 0);
    `)
	require.NoError(t, err)

	// Instructions + tags
	_, err = conn.Exec(`
        INSERT INTO component_instructions (component_id, step_number, text)
        VALUES (1, 1, 'Sear chicken'), (1, 2, 'Add rice');
    `)
	require.NoError(t, err)

	_, err = conn.Exec(`
        INSERT INTO component_tags (component_id, tag)
        VALUES (1, 'spicy'), (1, 'asian');
    `)
	require.NoError(t, err)

	// Planner data: 1 week, 1 slot, 1 plate with 1 component, 1 template with 1 component
	_, err = conn.Exec(`INSERT INTO weeks (year, week_number) VALUES (2026, 17)`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO time_slots (name_key, icon) VALUES ('slot.dinner', 'utensils')`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO plates (week_id, day, slot_id) VALUES (1, 0, 1)`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO plate_components (plate_id, component_id, portions, sort_order) VALUES (1, 1, 1, 0)`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO templates (name) VALUES ('Weekday')`)
	require.NoError(t, err)
	_, err = conn.Exec(`INSERT INTO template_components (template_id, component_id, portions, sort_order) VALUES (1, 2, 1, 0)`)
	require.NoError(t, err)

	// ── Apply migration 13 ────────────────────────────────────────────
	require.NoError(t, goose.UpTo(conn, "migrations", 13))

	// ── Row-count preservation ────────────────────────────────────────
	var leafCount, composedCount int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM foods WHERE kind='leaf'`).Scan(&leafCount))
	require.Equal(t, 3, leafCount, "3 ingredients should become 3 leaf foods")

	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM foods WHERE kind='composed'`).Scan(&composedCount))
	require.Equal(t, 2, composedCount, "2 components should become 2 composed foods")

	var fcCount int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM food_components`).Scan(&fcCount))
	require.Equal(t, 3, fcCount, "3 component_ingredients should become 3 food_components")

	var instCount int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM food_instructions`).Scan(&instCount))
	require.Equal(t, 2, instCount)

	var tagCount int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM food_tags`).Scan(&tagCount))
	require.Equal(t, 2, tagCount)

	var portionCount int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM food_portions`).Scan(&portionCount))
	require.Equal(t, 3, portionCount)

	// ── Grams sum preservation ────────────────────────────────────────
	var gramsSum float64
	require.NoError(t, conn.QueryRow(`SELECT COALESCE(SUM(grams),0) FROM food_components`).Scan(&gramsSum))
	require.Equal(t, 650.0, gramsSum, "grams sum 300+200+150 preserved")

	// ── Referential integrity ─────────────────────────────────────────
	// Every parent_id is a composed food.
	var badParents int
	require.NoError(t, conn.QueryRow(`
        SELECT COUNT(*) FROM food_components fc
        JOIN foods f ON f.id = fc.parent_id
        WHERE f.kind != 'composed'
    `).Scan(&badParents))
	require.Equal(t, 0, badParents, "all food_components.parent_id must point to composed foods")

	// plate_components.food_id resolves.
	var badPlateFood int
	require.NoError(t, conn.QueryRow(`
        SELECT COUNT(*) FROM plate_components pc
        LEFT JOIN foods f ON f.id = pc.food_id
        WHERE f.id IS NULL
    `).Scan(&badPlateFood))
	require.Equal(t, 0, badPlateFood)

	// template_components.food_id resolves.
	var badTmplFood int
	require.NoError(t, conn.QueryRow(`
        SELECT COUNT(*) FROM template_components tc
        LEFT JOIN foods f ON f.id = tc.food_id
        WHERE f.id IS NULL
    `).Scan(&badTmplFood))
	require.Equal(t, 0, badTmplFood)

	// ── Kind invariants ───────────────────────────────────────────────
	var leafWithRefPortions int
	require.NoError(t, conn.QueryRow(`
        SELECT COUNT(*) FROM foods WHERE kind='leaf' AND reference_portions IS NOT NULL
    `).Scan(&leafWithRefPortions))
	require.Equal(t, 0, leafWithRefPortions)

	var composedWithoutRefPortions int
	require.NoError(t, conn.QueryRow(`
        SELECT COUNT(*) FROM foods WHERE kind='composed' AND reference_portions IS NULL
    `).Scan(&composedWithoutRefPortions))
	require.Equal(t, 0, composedWithoutRefPortions)

	var leafWithSource int
	require.NoError(t, conn.QueryRow(`
        SELECT COUNT(*) FROM foods WHERE kind='leaf' AND source IS NULL
    `).Scan(&leafWithSource))
	require.Equal(t, 0, leafWithSource, "leaf foods must have source")

	// ── Nutrition field preservation for leaf foods ───────────────────
	var apple struct {
		kcal, protein, calcium float64
	}
	require.NoError(t, conn.QueryRow(`
        SELECT kcal_100g, protein_100g, calcium_100g FROM foods WHERE name='Apple' AND kind='leaf'
    `).Scan(&apple.kcal, &apple.protein, &apple.calcium))
	require.InDelta(t, 52.0, apple.kcal, 0.01)
	require.InDelta(t, 0.3, apple.protein, 0.01)
	require.InDelta(t, 6.0, apple.calcium, 0.01)

	// ── Variant group preserved on composed ───────────────────────────
	var curryVG sql.NullInt64
	require.NoError(t, conn.QueryRow(`
        SELECT variant_group_id FROM foods WHERE name='Chicken Curry' AND kind='composed'
    `).Scan(&curryVG))
	require.True(t, curryVG.Valid)
	require.Equal(t, int64(1), curryVG.Int64)

	// ── FTS content matches foods ─────────────────────────────────────
	var ftsCount int
	require.NoError(t, conn.QueryRow(`SELECT COUNT(*) FROM foods_fts`).Scan(&ftsCount))
	require.Equal(t, 5, ftsCount, "FTS should index all 5 foods (3 leaf + 2 composed)")

	var matched int
	require.NoError(t, conn.QueryRow(`
        SELECT COUNT(*) FROM foods_fts WHERE foods_fts MATCH 'curry'
    `).Scan(&matched))
	require.Equal(t, 1, matched, "FTS search for 'curry' finds 1 food")

	// ── Old tables are gone ───────────────────────────────────────────
	for _, name := range []string{
		"ingredients", "ingredients_fts", "ingredient_portions",
		"components", "components_fts", "component_ingredients",
		"component_instructions", "component_tags",
	} {
		var exists int
		require.NoError(t, conn.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
		).Scan(&exists))
		require.Equalf(t, 0, exists, "table %s should be dropped", name)
	}

	// ── Kind invariant enforced on insert ─────────────────────────────
	_, err = conn.Exec(`
        INSERT INTO foods (name, kind, reference_portions) VALUES ('Bad Leaf', 'leaf', 2)
    `)
	require.Error(t, err, "leaf with reference_portions must be rejected")

	_, err = conn.Exec(`
        INSERT INTO foods (name, kind, source) VALUES ('Bad Composed', 'composed', 'manual')
    `)
	require.Error(t, err, "composed without reference_portions must be rejected")

	// ── Unique leaf name enforced ─────────────────────────────────────
	_, err = conn.Exec(`
        INSERT INTO foods (name, kind, source, kcal_100g) VALUES ('Apple', 'leaf', 'manual', 50)
    `)
	require.Error(t, err, "duplicate leaf name must be rejected")

	// Same name OK on composed (e.g., variants)
	_, err = conn.Exec(`
        INSERT INTO foods (name, kind, reference_portions) VALUES ('Chicken Curry', 'composed', 4)
    `)
	require.NoError(t, err, "composed foods may share names")
}
