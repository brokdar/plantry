package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// UneatenRecipes counts eating as FoodHistory does: confirmed plates in
// [since, today] that are not marked not_eaten.
func TestFoodRepo_UneatenRecipes(t *testing.T) {
	db := testhelper.NewTestDB(t)
	exec := func(q string, args ...any) int64 {
		t.Helper()
		res, err := db.Exec(q, args...)
		require.NoError(t, err, q)
		id, err := res.LastInsertId()
		require.NoError(t, err)
		return id
	}
	slot := exec(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 0, 1)`)
	recipe := func(name string, role any) int64 {
		return exec(`INSERT INTO foods (name, kind, role, reference_portions) VALUES (?, 'composed', ?, 1)`, name, role)
	}
	eaten := recipe("Eaten", "main")
	early := recipe("Eaten before since", "main")
	late := recipe("Planned after today", "main")
	draft := recipe("Only a draft", "side_veg")
	skipped := recipe("Not eaten", nil)
	never := recipe("Never", nil)
	exec(`INSERT INTO foods (name, kind, source) VALUES ('Leaf', 'leaf', 'manual')`)
	plateOf := func(foodID int64, date, state string) int64 {
		p := exec(`INSERT INTO plates (slot_id, date, state) VALUES (?, ?, ?)`, slot, date, state)
		s := exec(`INSERT INTO cook_sessions (food_id, cook_date, cook_slot_id, portions, state) VALUES (?, ?, ?, 1, ?)`, foodID, date, slot, state)
		exec(`INSERT INTO plate_components (plate_id, food_id, portions, cook_session_id) VALUES (?, ?, 1, ?)`, p, foodID, s)
		return p
	}
	plateOf(eaten, "2026-10-20", "confirmed")
	plateOf(early, "2026-10-01", "confirmed")
	plateOf(late, "2026-11-05", "confirmed")
	plateOf(draft, "2026-10-19", "draft")
	p := plateOf(skipped, "2026-10-21", "confirmed")
	exec(`INSERT INTO plate_feedback (plate_id, status) VALUES (?, 'not_eaten')`, p)

	got, err := sqlite.NewFoodRepo(db).UneatenRecipes(context.Background(),
		time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))

	require.NoError(t, err)
	main, side := food.RoleMain, food.RoleSideVeg
	assert.Equal(t, []food.RecipeRole{
		{ID: early, Role: &main}, {ID: late, Role: &main}, {ID: draft, Role: &side}, {ID: skipped}, {ID: never},
	}, got)
	assert.NotContains(t, got, food.RecipeRole{ID: eaten, Role: &main})
}
