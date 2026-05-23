package preset_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// applyTestEnv wires a full real-SQLite stack so the apply pipeline can be
// exercised against the actual repos. The plate.Service is required because
// preset.Service expects a PlateService (Get) and PlateRange (Range).
type applyTestEnv struct {
	ctx       context.Context
	presetSvc *preset.Service
	plateSvc  *plate.Service
	slotSvc   *slot.Service
	slotID    int64
	foodID    int64
}

func newApplyTestEnv(t *testing.T) *applyTestEnv {
	t.Helper()
	db := testhelper.NewTestDB(t)
	ctx := context.Background()

	// Seed: one slot, one leaf food.
	res, err := db.ExecContext(ctx,
		`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'utensils', 1, 1)`)
	require.NoError(t, err)
	slotID, err := res.LastInsertId()
	require.NoError(t, err)

	res, err = db.ExecContext(ctx,
		`INSERT INTO foods (name, kind, source) VALUES ('Rice', 'leaf', 'manual')`)
	require.NoError(t, err)
	foodID, err := res.LastInsertId()
	require.NoError(t, err)

	foodRepo := sqlite.NewFoodRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	plateSvc := plate.NewService(plateRepo, slotRepo, foodRepo)
	slotSvc := slot.NewService(slotRepo)
	presetRepo := sqlite.NewPresetRepo(db)
	txRunner := sqlite.NewTxRunner(db)

	presetSvc := preset.NewService(presetRepo, foodRepo, plateSvc, txRunner, foodRepo, foodRepo).
		WithSlots(slotSvc).
		WithPlateRange(plateSvc)

	return &applyTestEnv{ctx: ctx, presetSvc: presetSvc, plateSvc: plateSvc, slotSvc: slotSvc, slotID: slotID, foodID: foodID}
}

func mkLeafComp(foodID int64, amount float64) preset.Component {
	unit := "g"
	grams := amount
	src := "direct"
	return preset.Component{
		FoodID:      foodID,
		Amount:      &amount,
		Unit:        &unit,
		Grams:       &grams,
		GramsSource: &src,
	}
}

// seedPreset creates a 1-plate preset bound to env.slotID with one leaf comp.
func (e *applyTestEnv) seedPreset(t *testing.T, name string) *preset.Preset {
	t.Helper()
	// Seed a real plate so CreateFromPlates has something to read.
	amount := 150.0
	unit := "g"
	grams := 150.0
	src := "direct"
	srcPlate := &plate.Plate{
		Date:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		SlotID: e.slotID,
		Components: []plate.PlateComponent{
			{FoodID: e.foodID, Amount: &amount, Unit: &unit, Grams: &grams, GramsSource: &src},
		},
	}
	require.NoError(t, e.plateSvc.Create(e.ctx, srcPlate))

	p, err := e.presetSvc.CreateFromPlates(e.ctx, preset.CreateFromPlatesInput{
		Name: name, PlateIDs: []int64{srcPlate.ID},
	})
	require.NoError(t, err)

	// Delete the source plate so the target date stays clean for later apply tests.
	require.NoError(t, e.plateSvc.Delete(e.ctx, srcPlate.ID))
	_ = mkLeafComp // keep helper referenced
	return p
}

func TestApply_HappyPath_CreatesPlate(t *testing.T) {
	e := newApplyTestEnv(t)
	p := e.seedPreset(t, "X")

	target := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	res, err := e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{TargetDate: target})
	require.NoError(t, err)
	require.Len(t, res.Created, 1)
	assert.Equal(t, e.slotID, res.Created[0].SlotID)

	// last_used_at gets touched.
	got, err := e.presetSvc.Get(e.ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, got.LastUsedAt)
}

func TestApply_OccupiedSkip(t *testing.T) {
	e := newApplyTestEnv(t)
	p := e.seedPreset(t, "X")

	// Occupy the target slot first.
	amount := 50.0
	unit := "g"
	grams := 50.0
	src := "direct"
	occ := &plate.Plate{
		Date:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		SlotID: e.slotID,
		Components: []plate.PlateComponent{
			{FoodID: e.foodID, Amount: &amount, Unit: &unit, Grams: &grams, GramsSource: &src},
		},
	}
	require.NoError(t, e.plateSvc.Create(e.ctx, occ))

	res, err := e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{
		TargetDate: occ.Date, OnConflict: preset.ConflictSkip,
	})
	require.NoError(t, err)
	assert.Empty(t, res.Created)
	require.Len(t, res.SkippedOccupied, 1)
	assert.Equal(t, e.slotID, res.SkippedOccupied[0].SlotID)
}

func TestApply_OccupiedOverwrite_AndUndo(t *testing.T) {
	e := newApplyTestEnv(t)
	p := e.seedPreset(t, "X")

	// Occupy the target slot.
	amount := 50.0
	unit := "g"
	grams := 50.0
	src := "direct"
	occ := &plate.Plate{
		Date:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		SlotID: e.slotID,
		Components: []plate.PlateComponent{
			{FoodID: e.foodID, Amount: &amount, Unit: &unit, Grams: &grams, GramsSource: &src},
		},
	}
	require.NoError(t, e.plateSvc.Create(e.ctx, occ))
	originalID := occ.ID

	res, err := e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{
		TargetDate: occ.Date, OnConflict: preset.ConflictOverwrite,
	})
	require.NoError(t, err)
	require.Len(t, res.Replaced, 1)
	assert.Equal(t, originalID, res.Replaced[0].OldPlate.ID)
	// Snapshot has one created, one replaced.
	require.Len(t, res.Snapshot.CreatedPlateIDs, 1)
	require.Len(t, res.Snapshot.ReplacedPlates, 1)

	// Day should now have exactly one plate (the new one).
	day, err := e.plateSvc.Day(e.ctx, occ.Date)
	require.NoError(t, err)
	require.Len(t, day, 1)
	assert.Equal(t, res.Replaced[0].NewPlate.ID, day[0].ID)

	// Undo restores the original.
	require.NoError(t, e.presetSvc.UndoApply(e.ctx, res.Snapshot))
	day, err = e.plateSvc.Day(e.ctx, occ.Date)
	require.NoError(t, err)
	require.Len(t, day, 1)
	// New ID after undo because Create allocates fresh ID. Components must match.
	require.Len(t, day[0].Components, 1)
	require.NotNil(t, day[0].Components[0].Amount)
	assert.Equal(t, 50.0, *day[0].Components[0].Amount, "undo restored original components")
}

func TestApply_InactiveSlot_SkippedNoSlot(t *testing.T) {
	e := newApplyTestEnv(t)
	p := e.seedPreset(t, "X")

	// Deactivate the slot.
	require.NoError(t, e.slotSvc.Update(e.ctx, &slot.TimeSlot{
		ID: e.slotID, NameKey: "slot.dinner", Icon: "utensils", SortOrder: 1, Active: false,
	}))

	target := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	res, err := e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{TargetDate: target})
	require.NoError(t, err)
	assert.Empty(t, res.Created)
	require.Len(t, res.SkippedNoSlot, 1)
}

var _ = food.KindLeaf

func TestApply_SlotIDsFilter(t *testing.T) {
	e := newApplyTestEnv(t)

	// Seed a second slot.
	// We can't easily seed via plateSvc, so build a preset with two plates
	// (both bound to env.slotID — feature.md allows it; second plate hits
	// "occupied" on the same slot).
	amount := 150.0
	unit := "g"
	grams := 150.0
	src := "direct"
	source := &plate.Plate{
		Date:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		SlotID: e.slotID,
		Components: []plate.PlateComponent{
			{FoodID: e.foodID, Amount: &amount, Unit: &unit, Grams: &grams, GramsSource: &src},
		},
	}
	require.NoError(t, e.plateSvc.Create(e.ctx, source))
	p, err := e.presetSvc.CreateFromPlates(e.ctx, preset.CreateFromPlatesInput{
		Name: "OneSlot", PlateIDs: []int64{source.ID},
	})
	require.NoError(t, err)
	require.NoError(t, e.plateSvc.Delete(e.ctx, source.ID))

	target := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	// Filter to a slot ID the preset does NOT contain → zero plates apply.
	res, err := e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{
		TargetDate: target, SlotIDsFilter: []int64{99999},
	})
	require.NoError(t, err)
	assert.Empty(t, res.Created)
	assert.Empty(t, res.SkippedOccupied)
	assert.Empty(t, res.SkippedNoSlot)
}

func TestCopyWeek_EmptySourceWeek(t *testing.T) {
	e := newApplyTestEnv(t)

	// No source plates seeded — call CopyWeek with a clean source week.
	sourceStart := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	targetStart := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	res, err := e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{
		SourceStart: sourceStart,
		TargetStart: targetStart,
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Empty(t, res.Created)
	assert.Empty(t, res.Replaced)
	assert.Empty(t, res.SkippedOccupied)
	assert.Empty(t, res.SkippedNoSlot)
	assert.Empty(t, res.Snapshot.CreatedPlateIDs)
	assert.Empty(t, res.Snapshot.ReplacedPlates)
}

func TestCopyWeek_OverwriteConflict(t *testing.T) {
	e := newApplyTestEnv(t)

	amount := 100.0
	unit := "g"
	grams := 100.0
	src := "direct"
	mkLeafPC := func() plate.PlateComponent {
		return plate.PlateComponent{FoodID: e.foodID, Amount: &amount, Unit: &unit, Grams: &grams, GramsSource: &src}
	}

	sourceStart := time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)  // Monday
	targetStart := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC) // next Monday

	// Seed one source plate (Monday of source week).
	srcPlate := &plate.Plate{
		Date:       sourceStart,
		SlotID:     e.slotID,
		Components: []plate.PlateComponent{mkLeafPC()},
	}
	require.NoError(t, e.plateSvc.Create(e.ctx, srcPlate))

	// Seed a conflicting plate on the target week, same offset, same slot.
	conflictAmount := 50.0
	conflictGrams := 50.0
	conflictPlate := &plate.Plate{
		Date:   targetStart,
		SlotID: e.slotID,
		Components: []plate.PlateComponent{
			{FoodID: e.foodID, Amount: &conflictAmount, Unit: &unit, Grams: &conflictGrams, GramsSource: &src},
		},
	}
	require.NoError(t, e.plateSvc.Create(e.ctx, conflictPlate))

	res, err := e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{
		SourceStart: sourceStart,
		TargetStart: targetStart,
		OnConflict:  preset.ConflictOverwrite,
	})
	require.NoError(t, err)
	require.Len(t, res.Replaced, 1)
	require.Len(t, res.Snapshot.ReplacedPlates, 1)
	require.Len(t, res.Snapshot.CreatedPlateIDs, 1)

	// Target slot now holds the new plate (100g from source, not 50g).
	day, err := e.plateSvc.Day(e.ctx, targetStart)
	require.NoError(t, err)
	require.Len(t, day, 1)
	require.Len(t, day[0].Components, 1)
	require.NotNil(t, day[0].Components[0].Amount)
	assert.Equal(t, 100.0, *day[0].Components[0].Amount, "target slot now holds the source plate's amount")
}

func TestUndoApply_ToleratesAlreadyDeletedPlates(t *testing.T) {
	e := newApplyTestEnv(t)

	// Build a snapshot whose CreatedPlateIDs contains a plate ID that doesn't exist.
	snap := preset.ApplySnapshot{
		CreatedPlateIDs: []int64{99999},
	}
	err := e.presetSvc.UndoApply(e.ctx, snap)
	require.NoError(t, err, "UndoApply should be idempotent when created plates already gone")
}

func TestCopyWeek_HappyPath(t *testing.T) {
	e := newApplyTestEnv(t)

	// Seed plates in source week.
	amount := 100.0
	unit := "g"
	grams := 100.0
	src := "direct"
	mkLeafPC := func() plate.PlateComponent {
		return plate.PlateComponent{FoodID: e.foodID, Amount: &amount, Unit: &unit, Grams: &grams, GramsSource: &src}
	}
	sourceStart := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC) // Monday
	for offset := 0; offset < 3; offset++ {
		p := &plate.Plate{
			Date:       sourceStart.AddDate(0, 0, offset),
			SlotID:     e.slotID,
			Components: []plate.PlateComponent{mkLeafPC()},
		}
		require.NoError(t, e.plateSvc.Create(e.ctx, p))
	}

	targetStart := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC) // next Monday
	res, err := e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{
		SourceStart: sourceStart,
		TargetStart: targetStart,
	})
	require.NoError(t, err)
	require.Len(t, res.Created, 3, "three plates copied")
	for i, p := range res.Created {
		expected := targetStart.AddDate(0, 0, i)
		assert.Equal(t, expected.Format("2006-01-02"), p.DateString(), "plate %d landed on right day", i)
	}
}
