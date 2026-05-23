package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// seedSlot inserts a time_slot and returns its id.
func seedSlot(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	res, err := db.ExecContext(context.Background(),
		`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'utensils', 1, 1)`)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

// seedLeafFood inserts a leaf food and returns its id.
func seedLeafFood(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	res, err := db.ExecContext(context.Background(),
		`INSERT INTO foods (name, kind, source) VALUES (?, 'leaf', 'manual')`, name)
	require.NoError(t, err)
	id, err := res.LastInsertId()
	require.NoError(t, err)
	return id
}

func mkLeafComponent(foodID int64) preset.Component {
	amount := 150.0
	unit := "g"
	grams := 150.0
	source := "direct"
	return preset.Component{
		FoodID:      foodID,
		Amount:      &amount,
		Unit:        &unit,
		Grams:       &grams,
		GramsSource: &source,
	}
}

func TestPresetRepo_CreateGet_RoundTrip(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotID := seedSlot(t, db)
	foodID := seedLeafFood(t, db, "Chicken Breast")
	repo := sqlite.NewPresetRepo(db)

	p := &preset.Preset{
		Name: "Chicken Bowl",
		Tags: []string{"quick", "high-protein"},
		Plates: []preset.Plate{{
			SlotID:     slotID,
			Components: []preset.Component{mkLeafComponent(foodID)},
		}},
	}

	require.NoError(t, repo.Create(ctx, p))
	require.NotZero(t, p.ID)
	require.Len(t, p.Plates, 1)
	require.NotZero(t, p.Plates[0].ID)
	require.Len(t, p.Plates[0].Components, 1)
	require.NotZero(t, p.Plates[0].Components[0].ID)

	got, err := repo.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Chicken Bowl", got.Name)
	assert.Equal(t, []string{"high-protein", "quick"}, got.Tags)
	require.Len(t, got.Plates, 1)
	assert.Equal(t, slotID, got.Plates[0].SlotID)
	require.Len(t, got.Plates[0].Components, 1)
	assert.Equal(t, foodID, got.Plates[0].Components[0].FoodID)
	require.NotNil(t, got.Plates[0].Components[0].Amount)
	assert.Equal(t, 150.0, *got.Plates[0].Components[0].Amount)
}

func TestPresetRepo_Delete_Cascades(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotID := seedSlot(t, db)
	foodID := seedLeafFood(t, db, "Egg")
	repo := sqlite.NewPresetRepo(db)

	p := &preset.Preset{
		Name: "Eggs",
		Tags: []string{"breakfast"},
		Plates: []preset.Plate{{
			SlotID:     slotID,
			Components: []preset.Component{mkLeafComponent(foodID)},
		}},
	}
	require.NoError(t, repo.Create(ctx, p))

	require.NoError(t, repo.Delete(ctx, p.ID))

	_, err := repo.Get(ctx, p.ID)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	// Tags and plates rows should be gone too.
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM preset_tags WHERE preset_id = ?`, p.ID).Scan(&count))
	assert.Zero(t, count, "tags cascaded")
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM preset_plates WHERE preset_id = ?`, p.ID).Scan(&count))
	assert.Zero(t, count, "plates cascaded")
}

func TestPresetRepo_Delete_NotFound(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewPresetRepo(db)
	err := repo.Delete(context.Background(), 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestPresetRepo_FoodFKRestrict(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotID := seedSlot(t, db)
	foodID := seedLeafFood(t, db, "Rice")
	repo := sqlite.NewPresetRepo(db)

	p := &preset.Preset{
		Name:   "Rice Bowl",
		Plates: []preset.Plate{{SlotID: slotID, Components: []preset.Component{mkLeafComponent(foodID)}}},
	}
	require.NoError(t, repo.Create(ctx, p))

	// Deleting the food while a preset references it must be blocked by FK.
	_, err := db.ExecContext(ctx, `DELETE FROM foods WHERE id = ?`, foodID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FOREIGN KEY constraint failed")
}

func TestPresetRepo_List_FilterByTag_AND(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotID := seedSlot(t, db)
	foodID := seedLeafFood(t, db, "Oats")
	repo := sqlite.NewPresetRepo(db)

	makePreset := func(name string, tags []string) {
		t.Helper()
		p := &preset.Preset{
			Name:   name,
			Tags:   tags,
			Plates: []preset.Plate{{SlotID: slotID, Components: []preset.Component{mkLeafComponent(foodID)}}},
		}
		require.NoError(t, repo.Create(ctx, p))
	}
	makePreset("A", []string{"quick", "vegan"})
	makePreset("B", []string{"quick"})
	makePreset("C", []string{"vegan"})

	res, err := repo.List(ctx, preset.ListFilter{Tags: []string{"quick", "vegan"}})
	require.NoError(t, err)
	require.Len(t, res.Items, 1)
	assert.Equal(t, "A", res.Items[0].Name)
	assert.Equal(t, 1, res.Total)
}

func TestPresetRepo_List_SearchFTS(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotID := seedSlot(t, db)
	foodID := seedLeafFood(t, db, "Quinoa")
	repo := sqlite.NewPresetRepo(db)

	for _, name := range []string{"Sunday Meal Prep", "Quick Lunch", "Quick Dinner"} {
		p := &preset.Preset{
			Name:   name,
			Plates: []preset.Plate{{SlotID: slotID, Components: []preset.Component{mkLeafComponent(foodID)}}},
		}
		require.NoError(t, repo.Create(ctx, p))
	}

	res, err := repo.List(ctx, preset.ListFilter{Search: "quick"})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Total)
	require.Len(t, res.Items, 2)
}

func TestPresetRepo_KnownTags_RankedByFrequency(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotID := seedSlot(t, db)
	foodID := seedLeafFood(t, db, "Tofu")
	repo := sqlite.NewPresetRepo(db)

	makePreset := func(name string, tags []string) {
		t.Helper()
		p := &preset.Preset{Name: name, Tags: tags, Plates: []preset.Plate{{SlotID: slotID, Components: []preset.Component{mkLeafComponent(foodID)}}}}
		require.NoError(t, repo.Create(ctx, p))
	}
	makePreset("A", []string{"quick", "vegan"})
	makePreset("B", []string{"quick"})
	makePreset("C", []string{"quick", "vegan"})

	got, err := repo.KnownTags(ctx, 10)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "quick", got[0].Tag)
	assert.EqualValues(t, 3, got[0].Count)
	assert.Equal(t, "vegan", got[1].Tag)
	assert.EqualValues(t, 2, got[1].Count)
}

func TestPresetRepo_TouchLastUsed(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotID := seedSlot(t, db)
	foodID := seedLeafFood(t, db, "Salmon")
	repo := sqlite.NewPresetRepo(db)

	p := &preset.Preset{Name: "X", Plates: []preset.Plate{{SlotID: slotID, Components: []preset.Component{mkLeafComponent(foodID)}}}}
	require.NoError(t, repo.Create(ctx, p))

	got, err := repo.Get(ctx, p.ID)
	require.NoError(t, err)
	require.Nil(t, got.LastUsedAt)

	require.NoError(t, repo.TouchLastUsed(ctx, p.ID))

	got, err = repo.Get(ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
}

func TestPresetRepo_FTSSanitisesOperators(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	slotID := seedSlot(t, db)
	foodID := seedLeafFood(t, db, "Lentils")
	repo := sqlite.NewPresetRepo(db)

	p := &preset.Preset{Name: "Spicy Lentils", Plates: []preset.Plate{{SlotID: slotID, Components: []preset.Component{mkLeafComponent(foodID)}}}}
	require.NoError(t, repo.Create(ctx, p))

	// FTS5 reserved-word search must not crash; it returns the matching row.
	res, err := repo.List(ctx, preset.ListFilter{Search: "AND OR NEAR"})
	require.NoError(t, err)
	assert.Zero(t, res.Total)

	// And a normal search still works after sanitisation.
	res, err = repo.List(ctx, preset.ListFilter{Search: "spicy"})
	require.NoError(t, err)
	require.Equal(t, 1, res.Total)
}

// Ensure errors.Is detects ErrNotFound on wrapped values.
func TestPresetRepo_Get_NotFoundErrorsIs(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewPresetRepo(db)
	_, err := repo.Get(context.Background(), 99999)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

// --- UpdateName ---

func TestPresetRepo_UpdateName_Roundtrip(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotID := seedSlot(t, db)
	foodID := seedLeafFood(t, db, "Beans")
	repo := sqlite.NewPresetRepo(db)

	p := &preset.Preset{
		Name: "Old",
		Tags: []string{"quick"},
		Plates: []preset.Plate{{
			SlotID:     slotID,
			Components: []preset.Component{mkLeafComponent(foodID)},
		}},
	}
	require.NoError(t, repo.Create(ctx, p))

	updated, err := repo.UpdateName(ctx, p.ID, "New")
	require.NoError(t, err)
	assert.Equal(t, "New", updated.Name)

	got, err := repo.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "New", got.Name)
	assert.Equal(t, []string{"quick"}, got.Tags)
	require.Len(t, got.Plates, 1)
	require.Len(t, got.Plates[0].Components, 1)
	assert.Equal(t, foodID, got.Plates[0].Components[0].FoodID)
}

func TestPresetRepo_UpdateName_NotFound(t *testing.T) {
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewPresetRepo(db)
	_, err := repo.UpdateName(context.Background(), 99999, "X")
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

// --- ReplacePlates ---

func TestPresetRepo_ReplacePlates_SwapsContent(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotA := seedSlot(t, db)
	// seed a second slot
	res, err := db.ExecContext(ctx,
		`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.lunch', 'utensils', 2, 1)`)
	require.NoError(t, err)
	slotB, err := res.LastInsertId()
	require.NoError(t, err)

	foodA := seedLeafFood(t, db, "FoodA")
	foodB := seedLeafFood(t, db, "FoodB")

	repo := sqlite.NewPresetRepo(db)
	p := &preset.Preset{
		Name: "Test",
		Plates: []preset.Plate{{
			SlotID:     slotA,
			Components: []preset.Component{mkLeafComponent(foodA)},
		}},
	}
	require.NoError(t, repo.Create(ctx, p))

	// Sanity: one plate row before swap.
	var count int
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM preset_plates WHERE preset_id = ?`, p.ID).Scan(&count))
	require.Equal(t, 1, count)

	newPlates := []preset.Plate{{
		SlotID:     slotB,
		Components: []preset.Component{mkLeafComponent(foodB)},
	}}
	require.NoError(t, repo.ReplacePlates(ctx, p.ID, newPlates))

	got, err := repo.Get(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, got.Plates, 1)
	assert.Equal(t, slotB, got.Plates[0].SlotID)
	require.Len(t, got.Plates[0].Components, 1)
	assert.Equal(t, foodB, got.Plates[0].Components[0].FoodID)

	// Old plate row is gone.
	require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM preset_plates WHERE preset_id = ?`, p.ID).Scan(&count))
	assert.Equal(t, 1, count, "exactly one plate after swap (the new one)")
}

func TestPresetRepo_ReplacePlates_InvalidFoodFK(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotID := seedSlot(t, db)
	foodID := seedLeafFood(t, db, "RealFood")
	repo := sqlite.NewPresetRepo(db)

	p := &preset.Preset{
		Name:   "Test",
		Plates: []preset.Plate{{SlotID: slotID, Components: []preset.Component{mkLeafComponent(foodID)}}},
	}
	require.NoError(t, repo.Create(ctx, p))

	// Build a component referencing a non-existent food id.
	amount := 100.0
	unit := "g"
	grams := 100.0
	src := "direct"
	bad := []preset.Plate{{
		SlotID: slotID,
		Components: []preset.Component{{
			FoodID: 999999, Amount: &amount, Unit: &unit, Grams: &grams, GramsSource: &src,
		}},
	}}
	err := repo.ReplacePlates(ctx, p.ID, bad)
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidInput))
}

// --- AddTag / RemoveTag ---

func TestPresetRepo_AddRemoveTag_Roundtrip(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotID := seedSlot(t, db)
	foodID := seedLeafFood(t, db, "Tofu2")
	repo := sqlite.NewPresetRepo(db)

	p := &preset.Preset{
		Name:   "Test",
		Plates: []preset.Plate{{SlotID: slotID, Components: []preset.Component{mkLeafComponent(foodID)}}},
	}
	require.NoError(t, repo.Create(ctx, p))

	before, err := repo.Get(ctx, p.ID)
	require.NoError(t, err)
	updatedBefore := before.UpdatedAt

	// Sleep 1.1s so timestamps (1-second granularity) advance.
	time.Sleep(1100 * time.Millisecond)

	require.NoError(t, repo.AddTag(ctx, p.ID, "quick"))
	got, err := repo.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"quick"}, got.Tags)
	assert.True(t, got.UpdatedAt.After(updatedBefore), "updated_at advances after AddTag")
	updatedAfterAdd := got.UpdatedAt

	time.Sleep(1100 * time.Millisecond)
	require.NoError(t, repo.RemoveTag(ctx, p.ID, "quick"))
	got, err = repo.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Tags)
	assert.True(t, got.UpdatedAt.After(updatedAfterAdd), "updated_at advances after RemoveTag")
}

// --- CountUsingFood ---

func TestPresetRepo_CountUsingFood(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotID := seedSlot(t, db)
	usedFoodID := seedLeafFood(t, db, "Used")
	unusedFoodID := seedLeafFood(t, db, "Unused")
	repo := sqlite.NewPresetRepo(db)

	makePreset := func(name string) {
		t.Helper()
		p := &preset.Preset{
			Name:   name,
			Plates: []preset.Plate{{SlotID: slotID, Components: []preset.Component{mkLeafComponent(usedFoodID)}}},
		}
		require.NoError(t, repo.Create(ctx, p))
	}
	makePreset("A")
	makePreset("B")

	usedCount, err := repo.CountUsingFood(ctx, usedFoodID)
	require.NoError(t, err)
	assert.EqualValues(t, 2, usedCount)

	unusedCount, err := repo.CountUsingFood(ctx, unusedFoodID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, unusedCount)
}

// --- List filter by SlotID ---

func TestPresetRepo_List_FilterBySlotID(t *testing.T) {
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	slotA := seedSlot(t, db)
	res, err := db.ExecContext(ctx,
		`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.lunch', 'utensils', 2, 1)`)
	require.NoError(t, err)
	slotB, err := res.LastInsertId()
	require.NoError(t, err)

	foodID := seedLeafFood(t, db, "Rice2")
	repo := sqlite.NewPresetRepo(db)

	a := &preset.Preset{Name: "A", Plates: []preset.Plate{{SlotID: slotA, Components: []preset.Component{mkLeafComponent(foodID)}}}}
	require.NoError(t, repo.Create(ctx, a))
	b := &preset.Preset{Name: "B", Plates: []preset.Plate{{SlotID: slotB, Components: []preset.Component{mkLeafComponent(foodID)}}}}
	require.NoError(t, repo.Create(ctx, b))

	got, err := repo.List(ctx, preset.ListFilter{SlotIDs: []int64{slotA}})
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	assert.Equal(t, "A", got.Items[0].Name)
	assert.Equal(t, 1, got.Total)
}
