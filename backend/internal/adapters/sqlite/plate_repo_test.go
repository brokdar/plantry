package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

type plateFixture struct {
	db        *sql.DB
	plates    *sqlite.PlateRepo
	weeks     *sqlite.WeekRepo
	slots     *sqlite.SlotRepo
	week      *planner.Week
	slot      *slot.TimeSlot
	component *component.Component
}

func setupPlateFixture(t *testing.T) *plateFixture {
	t.Helper()
	db := testhelper.NewTestDB(t)
	weekRepo := sqlite.NewWeekRepo(db)
	plateRepo := sqlite.NewPlateRepo(db)
	slotRepo := sqlite.NewSlotRepo(db)
	ingRepo := sqlite.NewIngredientRepo(db)
	compRepo := sqlite.NewComponentRepo(db)

	ctx := context.Background()
	w := &planner.Week{Year: 2026, WeekNumber: 16}
	require.NoError(t, weekRepo.Create(ctx, w))
	s := &slot.TimeSlot{NameKey: "slot.dinner", Icon: "Moon", SortOrder: 1, Active: true}
	require.NoError(t, slotRepo.Create(ctx, s))

	ing := &ingredient.Ingredient{Name: "Chicken", Source: "manual", Kcal100g: 100}
	require.NoError(t, ingRepo.Create(ctx, ing))
	c := &component.Component{
		Name: "Curry", Role: component.RoleMain, ReferencePortions: 2,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 100, Unit: "g", Grams: 100},
		},
	}
	require.NoError(t, compRepo.Create(ctx, c))

	return &plateFixture{
		db: db, plates: plateRepo, weeks: weekRepo, slots: slotRepo,
		week: w, slot: s, component: c,
	}
}

func TestPlateRepo_RoundTrip(t *testing.T) {
	f := setupPlateFixture(t)
	ctx := context.Background()

	note := "leftovers"
	p := &plate.Plate{WeekID: f.week.ID, Day: 1, SlotID: f.slot.ID, Note: &note}
	require.NoError(t, f.plates.Create(ctx, p))
	assert.NotZero(t, p.ID)
	assert.False(t, p.CreatedAt.IsZero())

	got, err := f.plates.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Day)
	require.NotNil(t, got.Note)
	assert.Equal(t, "leftovers", *got.Note)

	got.Note = nil
	got.Day = 2
	require.NoError(t, f.plates.Update(ctx, got))
	reloaded, err := f.plates.Get(ctx, p.ID)
	require.NoError(t, err)
	assert.Nil(t, reloaded.Note)
	assert.Equal(t, 2, reloaded.Day)

	require.NoError(t, f.plates.Delete(ctx, p.ID))
	_, err = f.plates.Get(ctx, p.ID)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestPlateRepo_CreateWithComponents(t *testing.T) {
	f := setupPlateFixture(t)
	ctx := context.Background()
	p := &plate.Plate{
		WeekID: f.week.ID, Day: 0, SlotID: f.slot.ID,
		Components: []plate.PlateComponent{
			{ComponentID: f.component.ID, Portions: 1.5, SortOrder: 0},
		},
	}
	require.NoError(t, f.plates.Create(ctx, p))
	require.Len(t, p.Components, 1)
	assert.NotZero(t, p.Components[0].ID)

	got, err := f.plates.Get(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, got.Components, 1)
	assert.InDelta(t, 1.5, got.Components[0].Portions, 1e-9)
}

func TestPlateRepo_PlateComponentSwap_PreservesSortOrder(t *testing.T) {
	f := setupPlateFixture(t)
	ctx := context.Background()
	// Need second component for swap.
	ingRepo := sqlite.NewIngredientRepo(f.db)
	compRepo := sqlite.NewComponentRepo(f.db)
	ing := &ingredient.Ingredient{Name: "Tofu", Source: "manual", Kcal100g: 80}
	require.NoError(t, ingRepo.Create(ctx, ing))
	c2 := &component.Component{
		Name: "Tofu Curry", Role: component.RoleMain, ReferencePortions: 1,
		Ingredients: []component.ComponentIngredient{
			{IngredientID: ing.ID, Amount: 100, Unit: "g", Grams: 100},
		},
	}
	require.NoError(t, compRepo.Create(ctx, c2))

	p := &plate.Plate{WeekID: f.week.ID, Day: 3, SlotID: f.slot.ID}
	require.NoError(t, f.plates.Create(ctx, p))
	pc := &plate.PlateComponent{PlateID: p.ID, ComponentID: f.component.ID, Portions: 1, SortOrder: 5}
	require.NoError(t, f.plates.CreateComponent(ctx, pc))

	pc.ComponentID = c2.ID
	require.NoError(t, f.plates.UpdateComponent(ctx, pc))

	got, err := f.plates.GetComponent(ctx, pc.ID)
	require.NoError(t, err)
	assert.Equal(t, c2.ID, got.ComponentID)
	assert.Equal(t, 5, got.SortOrder, "sort_order preserved across swap")
}

func TestPlateRepo_DeleteCascadesComponents(t *testing.T) {
	f := setupPlateFixture(t)
	ctx := context.Background()
	p := &plate.Plate{
		WeekID: f.week.ID, Day: 0, SlotID: f.slot.ID,
		Components: []plate.PlateComponent{
			{ComponentID: f.component.ID, Portions: 1, SortOrder: 0},
		},
	}
	require.NoError(t, f.plates.Create(ctx, p))
	pcID := p.Components[0].ID

	require.NoError(t, f.plates.Delete(ctx, p.ID))

	_, err := f.plates.GetComponent(ctx, pcID)
	assert.True(t, errors.Is(err, domain.ErrNotFound))
}

func TestPlateRepo_CountUsing(t *testing.T) {
	f := setupPlateFixture(t)
	ctx := context.Background()
	p := &plate.Plate{
		WeekID: f.week.ID, Day: 0, SlotID: f.slot.ID,
		Components: []plate.PlateComponent{
			{ComponentID: f.component.ID, Portions: 1, SortOrder: 0},
		},
	}
	require.NoError(t, f.plates.Create(ctx, p))

	cs, err := f.plates.CountUsingComponent(ctx, f.component.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), cs)

	ts, err := f.plates.CountUsingTimeSlot(ctx, f.slot.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), ts)
}

func TestPlateRepo_ListByWeek_ComponentsLoaded(t *testing.T) {
	f := setupPlateFixture(t)
	ctx := context.Background()
	for d := 0; d < 3; d++ {
		p := &plate.Plate{
			WeekID: f.week.ID, Day: d, SlotID: f.slot.ID,
			Components: []plate.PlateComponent{
				{ComponentID: f.component.ID, Portions: 1, SortOrder: 0},
			},
		}
		require.NoError(t, f.plates.Create(ctx, p))
	}
	plates, err := f.plates.ListByWeek(ctx, f.week.ID)
	require.NoError(t, err)
	require.Len(t, plates, 3)
	for _, p := range plates {
		assert.Len(t, p.Components, 1)
	}
}

func TestTxRunner_CommitsAndRollsBack(t *testing.T) {
	f := setupPlateFixture(t)
	ctx := context.Background()
	runner := sqlite.NewTxRunner(f.db)

	// Commit on success.
	err := runner.RunInTx(ctx, func(weeks planner.WeekRepository, plates plate.Repository) error {
		w := &planner.Week{Year: 2026, WeekNumber: 17}
		if err := weeks.Create(ctx, w); err != nil {
			return err
		}
		return plates.Create(ctx, &plate.Plate{WeekID: w.ID, Day: 0, SlotID: f.slot.ID})
	})
	require.NoError(t, err)

	got, err := f.weeks.GetByYearAndNumber(ctx, 2026, 17)
	require.NoError(t, err)
	assert.NotZero(t, got.ID)

	// Rollback on error.
	err = runner.RunInTx(ctx, func(weeks planner.WeekRepository, _ plate.Repository) error {
		return weeks.Create(ctx, &planner.Week{Year: 2026, WeekNumber: 17}) // unique violation
	})
	require.Error(t, err)
}

func TestComponentDelete_BlockedWhenOnPlate(t *testing.T) {
	f := setupPlateFixture(t)
	ctx := context.Background()
	p := &plate.Plate{
		WeekID: f.week.ID, Day: 0, SlotID: f.slot.ID,
		Components: []plate.PlateComponent{
			{ComponentID: f.component.ID, Portions: 1, SortOrder: 0},
		},
	}
	require.NoError(t, f.plates.Create(ctx, p))

	compRepo := sqlite.NewComponentRepo(f.db)
	err := compRepo.Delete(ctx, f.component.ID)
	assert.True(t, errors.Is(err, domain.ErrInUse), "got %v", err)
}

func TestPlateRepo_SlotDeleteRestricted(t *testing.T) {
	f := setupPlateFixture(t)
	ctx := context.Background()
	p := &plate.Plate{WeekID: f.week.ID, Day: 0, SlotID: f.slot.ID}
	require.NoError(t, f.plates.Create(ctx, p))
	err := f.slots.Delete(ctx, f.slot.ID)
	assert.True(t, errors.Is(err, domain.ErrInUse), "got %v", err)
}
