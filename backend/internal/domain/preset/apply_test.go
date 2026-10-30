package preset_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// applyTestEnv wires a full real-SQLite stack so apply and copy week run
// through the real plate write path. Household portions are 2.
type applyTestEnv struct {
	ctx       context.Context
	db        *sql.DB
	presetSvc *preset.Service
	plateSvc  *plate.Service
	slotSvc   *slot.Service
	availSvc  *availability.Service
	slotID    int64 // dinner
	lunchID   int64
	foodID    int64 // leaf Rice
	chiliID   int64 // composed
}

func newApplyTestEnv(t *testing.T) *applyTestEnv {
	t.Helper()
	db := testhelper.NewTestDB(t)
	ctx := context.Background()
	insert := func(q string) int64 {
		res, err := db.ExecContext(ctx, q)
		require.NoError(t, err, q)
		id, err := res.LastInsertId()
		require.NoError(t, err)
		return id
	}
	slotID := insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'utensils', 1, 1)`)
	lunchID := insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.lunch', 'sun', 0, 1)`)
	foodID := insert(`INSERT INTO foods (name, kind, source) VALUES ('Rice', 'leaf', 'manual')`)
	chiliID := insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Chili', 'composed', 'main', 4)`)
	_, err := db.ExecContext(ctx, `UPDATE user_profile SET household_portions = 2 WHERE id = 1`)
	require.NoError(t, err)

	foodRepo := sqlite.NewFoodRepo(db)
	plateSvc, availSvc := testsvc.Plan(db)
	slotSvc := slot.NewService(sqlite.NewSlotRepo(db))
	presetSvc := preset.NewService(sqlite.NewPresetRepo(db), foodRepo, plateSvc, sqlite.NewTxRunner(db), foodRepo, foodRepo).
		WithSlots(slotSvc).
		WithAvailability(availSvc)

	return &applyTestEnv{
		ctx: ctx, db: db, presetSvc: presetSvc, plateSvc: plateSvc, slotSvc: slotSvc, availSvc: availSvc,
		slotID: slotID, lunchID: lunchID, foodID: foodID, chiliID: chiliID,
	}
}

// setNone marks a dinner cell "none" with an optional note.
func (e *applyTestEnv) setNone(t *testing.T, date time.Time, note *string) {
	t.Helper()
	_, _, err := e.availSvc.SetOverride(e.ctx, availability.OverrideInput{
		Date: date, SlotID: e.slotID, Mode: availability.ModeNone, Note: note,
	})
	require.NoError(t, err)
}

func leafComp(foodID int64, amount float64) plate.PlateComponent {
	unit := "g"
	return plate.PlateComponent{FoodID: foodID, Amount: &amount, Unit: &unit}
}

func composedComp(foodID int64) plate.PlateComponent {
	return plate.PlateComponent{FoodID: foodID, Portions: ptr(1.0)}
}

// leafPlate creates a dinner plate with one 100 g leaf component.
func (e *applyTestEnv) leafPlate(t *testing.T, date time.Time) *plate.Plate {
	t.Helper()
	return e.create(t, date, e.slotID, leafComp(e.foodID, 100))
}

func (e *applyTestEnv) create(t *testing.T, date time.Time, slotID int64, comps ...plate.PlateComponent) *plate.Plate {
	t.Helper()
	out, err := e.plateSvc.Create(e.ctx, plate.PlateInput{Date: date, SlotID: slotID, Components: comps})
	require.NoError(t, err)
	return out
}

func (e *applyTestEnv) remove(t *testing.T, id int64) {
	t.Helper()
	_, err := e.plateSvc.Delete(e.ctx, id)
	require.NoError(t, err)
}

func (e *applyTestEnv) cell(t *testing.T, date time.Time) availability.Cell {
	t.Helper()
	cells, err := e.availSvc.Resolve(e.ctx, date, date)
	require.NoError(t, err)
	for _, c := range cells {
		if c.SlotID == e.slotID {
			return c
		}
	}
	t.Fatalf("no cell for %s", date)
	return availability.Cell{}
}

func (e *applyTestEnv) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n))
	return n
}

// undo reverses an apply or copy week the way the client does: delete the
// created plates, restore Removed, clear the created overrides.
func (e *applyTestEnv) undo(t *testing.T, res *preset.ApplyResult) {
	t.Helper()
	for _, id := range res.CreatedPlateIDs {
		e.remove(t, id)
	}
	_, err := e.plateSvc.Apply(e.ctx, res.Removed, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	require.NoError(t, err)
	for _, c := range res.CreatedOverrides {
		require.NoError(t, e.availSvc.ClearOverride(e.ctx, c.Date, c.SlotID))
	}
}

// plan describes the plates and sessions in [from, to] without ids, so two
// plans compare equal after an undo.
func (e *applyTestEnv) plan(t *testing.T, from, to time.Time) []string {
	t.Helper()
	plates, err := e.plateSvc.Range(e.ctx, from, to)
	require.NoError(t, err)
	out := []string{}
	for _, p := range plates {
		line := fmt.Sprintf("%s/%d eaters=%g", p.DateString(), p.SlotID, p.Eaters)
		for _, c := range p.Components {
			line += fmt.Sprintf(" [food=%d", c.FoodID)
			if c.Portions != nil {
				line += fmt.Sprintf(" portions=%g", *c.Portions)
			}
			if c.Grams != nil {
				line += fmt.Sprintf(" grams=%g", *c.Grams)
			}
			if c.CookSessionID != nil {
				v, err := e.plateSvc.Session(e.ctx, *c.CookSessionID)
				require.NoError(t, err)
				line += fmt.Sprintf(" session=%s/%d portions=%g meals=%d", v.CookDate.Format(time.DateOnly), v.CookSlotID, v.Portions, v.Meals)
			}
			line += "]"
		}
		out = append(out, line)
	}
	sort.Strings(out)
	return out
}

// seedPreset creates a 1-plate dinner preset holding comps.
func (e *applyTestEnv) seedPreset(t *testing.T, name string, comps ...plate.PlateComponent) *preset.Preset {
	t.Helper()
	if len(comps) == 0 {
		comps = []plate.PlateComponent{leafComp(e.foodID, 150)}
	}
	src := e.create(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), e.slotID, comps...)
	p, err := e.presetSvc.CreateFromPlates(e.ctx, preset.CreateFromPlatesInput{Name: name, PlateIDs: []int64{src.ID}})
	require.NoError(t, err)
	e.remove(t, src.ID)
	return p
}

var applyTarget = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

// ── Apply ───────────────────────────────────────────────────────────────

func TestApply_HappyPath_CreatesPlate(t *testing.T) {
	e := newApplyTestEnv(t)
	p := e.seedPreset(t, "X")

	res, err := e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{TargetDate: applyTarget})
	require.NoError(t, err)
	require.Len(t, res.CreatedPlateIDs, 1)
	got, err := e.plateSvc.Get(e.ctx, res.CreatedPlateIDs[0])
	require.NoError(t, err)
	assert.Equal(t, e.slotID, got.SlotID)
	assert.InDelta(t, 2.0, got.Eaters, 1e-9, "eaters default to the household")
	assert.Empty(t, res.Removed.Plates)

	gotPreset, err := e.presetSvc.Get(e.ctx, p.ID)
	require.NoError(t, err)
	require.NotNil(t, gotPreset.LastUsedAt)
}

// B2-AC2
func TestApply_ComposedComponent_FreshSessionOrSkippedOnNone(t *testing.T) {
	e := newApplyTestEnv(t)
	p := e.seedPreset(t, "Chili night", composedComp(e.chiliID))
	require.Equal(t, 0, e.count(t, "cook_sessions"))

	res, err := e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{TargetDate: applyTarget})
	require.NoError(t, err)
	require.Len(t, res.CreatedPlateIDs, 1)
	got, err := e.plateSvc.Get(e.ctx, res.CreatedPlateIDs[0])
	require.NoError(t, err)
	require.NotNil(t, got.Components[0].CookSessionID)
	v, err := e.plateSvc.Session(e.ctx, *got.Components[0].CookSessionID)
	require.NoError(t, err)
	assert.Equal(t, applyTarget, v.CookDate)
	assert.Equal(t, e.slotID, v.CookSlotID)
	assert.InDelta(t, 2.0, v.Portions, 1e-9, "1 portion per eater × 2 eaters")
	assert.Equal(t, 1, v.Meals)

	none := applyTarget.AddDate(0, 0, 1)
	e.setNone(t, none, nil)
	res, err = e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{TargetDate: none})
	require.NoError(t, err)
	assert.Empty(t, res.CreatedPlateIDs)
	assert.Equal(t, []plate.CellKey{{Date: none, SlotID: e.slotID}}, res.SkippedUnavailable)
	assert.Equal(t, 1, e.count(t, "cook_sessions"), "a skipped plate creates no session")
}

func TestApply_NoneCell_SkippedUnavailable(t *testing.T) {
	for _, conflict := range []preset.ConflictMode{preset.ConflictSkip, preset.ConflictOverwrite} {
		t.Run(string(conflict), func(t *testing.T) {
			e := newApplyTestEnv(t)
			p := e.seedPreset(t, "X")
			e.setNone(t, applyTarget, nil)

			res, err := e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{TargetDate: applyTarget, OnConflict: conflict})
			require.NoError(t, err)
			assert.Empty(t, res.CreatedPlateIDs)
			assert.Empty(t, res.SkippedOccupied)
			assert.Equal(t, []plate.CellKey{{Date: applyTarget, SlotID: e.slotID}}, res.SkippedUnavailable)

			day, err := e.plateSvc.Day(e.ctx, applyTarget)
			require.NoError(t, err)
			assert.Empty(t, day)
			got, err := e.presetSvc.Get(e.ctx, p.ID)
			require.NoError(t, err)
			assert.Nil(t, got.LastUsedAt, "nothing placed, so the preset was not used")
		})
	}
}

func TestApply_OccupiedSkip(t *testing.T) {
	e := newApplyTestEnv(t)
	p := e.seedPreset(t, "X")
	e.create(t, applyTarget, e.slotID, leafComp(e.foodID, 50))

	res, err := e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{TargetDate: applyTarget, OnConflict: preset.ConflictSkip})
	require.NoError(t, err)
	assert.Empty(t, res.CreatedPlateIDs)
	assert.Equal(t, []plate.CellKey{{Date: applyTarget, SlotID: e.slotID}}, res.SkippedOccupied)
}

// B2-AC3: overwrite apply then undo yields the pre-apply plan, sessions
// included.
func TestApply_Overwrite_UndoRestoresThePlan(t *testing.T) {
	e := newApplyTestEnv(t)
	p := e.seedPreset(t, "X")
	// The target cell holds the fresh plate of a batch with a leftover the
	// next day: the batch survives the overwrite with one link.
	cook := e.create(t, applyTarget, e.slotID, composedComp(e.chiliID), leafComp(e.foodID, 30))
	sid := *cook.Components[0].CookSessionID
	_, err := e.plateSvc.AddLeftovers(e.ctx, sid, []plate.CellKey{{Date: applyTarget.AddDate(0, 0, 1), SlotID: e.lunchID}})
	require.NoError(t, err)
	window := applyTarget.AddDate(0, 0, 1)
	before := e.plan(t, applyTarget, window)

	res, err := e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{TargetDate: applyTarget, OnConflict: preset.ConflictOverwrite})
	require.NoError(t, err)
	require.Len(t, res.CreatedPlateIDs, 1)
	require.Len(t, res.Removed.Plates, 1)
	assert.Equal(t, sid, res.Removed.Plates[0].Components[0].Session.ID, "the batch survives, so it is recorded by id")
	day, err := e.plateSvc.Day(e.ctx, applyTarget)
	require.NoError(t, err)
	require.Len(t, day, 1)
	assert.Equal(t, res.CreatedPlateIDs[0], day[0].ID)
	assert.InDelta(t, 150.0, *day[0].Components[0].Amount, 1e-9)

	e.undo(t, res)
	assert.Equal(t, before, e.plan(t, applyTarget, window))
}

func TestApply_InactiveSlot_SkippedNoSlot(t *testing.T) {
	e := newApplyTestEnv(t)
	p := e.seedPreset(t, "X")
	require.NoError(t, e.slotSvc.Update(e.ctx, &slot.TimeSlot{
		ID: e.slotID, NameKey: "slot.dinner", Icon: "utensils", SortOrder: 1, Active: false,
	}))

	res, err := e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{TargetDate: applyTarget})
	require.NoError(t, err)
	assert.Empty(t, res.CreatedPlateIDs)
	assert.Equal(t, []plate.CellKey{{Date: applyTarget, SlotID: e.slotID}}, res.SkippedNoSlot)
}

func TestApply_SlotIDsFilter(t *testing.T) {
	e := newApplyTestEnv(t)
	p := e.seedPreset(t, "OneSlot")

	// Filter to a slot ID the preset does NOT contain → zero plates apply.
	res, err := e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{TargetDate: applyTarget, SlotIDsFilter: []int64{99999}})
	require.NoError(t, err)
	assert.Empty(t, res.CreatedPlateIDs)
	assert.Empty(t, res.SkippedOccupied)
	assert.Empty(t, res.SkippedNoSlot)
}

func TestApply_Validation(t *testing.T) {
	e := newApplyTestEnv(t)
	p := e.seedPreset(t, "X")
	_, err := e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = e.presetSvc.Apply(e.ctx, p.ID, preset.ApplyRequest{TargetDate: applyTarget, OnConflict: "merge"})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = e.presetSvc.Apply(e.ctx, 9999, preset.ApplyRequest{TargetDate: applyTarget})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// ── Copy week ───────────────────────────────────────────────────────────

var (
	copySource = time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC) // Monday
	copyTarget = copySource.AddDate(0, 0, 7)
)

func ptrStr(s string) *string { return &s }

func TestCopyWeek_HappyPath(t *testing.T) {
	e := newApplyTestEnv(t)
	for offset := range 3 {
		e.leafPlate(t, copySource.AddDate(0, 0, offset))
	}

	res, err := e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{SourceStart: copySource, TargetStart: copyTarget})
	require.NoError(t, err)
	require.Len(t, res.CreatedPlateIDs, 3, "three plates copied")
	for i, id := range res.CreatedPlateIDs {
		p, err := e.plateSvc.Get(e.ctx, id)
		require.NoError(t, err)
		assert.Equal(t, copyTarget.AddDate(0, 0, i), p.Date, "plate %d landed on the right day", i)
		assert.InDelta(t, 100.0, *p.Components[0].Amount, 1e-9)
	}
}

// B2-AC1: a Sun batch with 2 leftovers copies as one session with the same
// portions and 3 links, not 3 fresh sessions.
func TestCopyWeek_BatchStaysOneSession(t *testing.T) {
	e := newApplyTestEnv(t)
	sat := copySource.AddDate(0, 0, 5)
	sun := copySource.AddDate(0, 0, 6)
	cook := e.create(t, sat, e.slotID, composedComp(e.chiliID))
	sid := *cook.Components[0].CookSessionID
	_, err := e.plateSvc.AddLeftovers(e.ctx, sid, []plate.CellKey{{Date: sun, SlotID: e.lunchID}, {Date: sun, SlotID: e.slotID}})
	require.NoError(t, err)
	src, err := e.plateSvc.Session(e.ctx, sid)
	require.NoError(t, err)
	require.InDelta(t, 6.0, src.Portions, 1e-9)
	_, err = e.plateSvc.SetCooked(e.ctx, sid, true)
	require.NoError(t, err)

	res, err := e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{SourceStart: copySource, TargetStart: copyTarget})
	require.NoError(t, err)
	require.Len(t, res.CreatedPlateIDs, 3)
	assert.Equal(t, 2, e.count(t, "cook_sessions"), "one session per week, not one per plate")

	first, err := e.plateSvc.Get(e.ctx, res.CreatedPlateIDs[0])
	require.NoError(t, err)
	copied := *first.Components[0].CookSessionID
	for _, id := range res.CreatedPlateIDs {
		p, err := e.plateSvc.Get(e.ctx, id)
		require.NoError(t, err)
		assert.Equal(t, copied, *p.Components[0].CookSessionID)
	}
	v, err := e.plateSvc.Session(e.ctx, copied)
	require.NoError(t, err)
	assert.Equal(t, sat.AddDate(0, 0, 7), v.CookDate)
	assert.Equal(t, e.slotID, v.CookSlotID)
	assert.InDelta(t, 6.0, v.Portions, 1e-9)
	assert.Zero(t, v.FrozenPortions)
	assert.Nil(t, v.CookedAt, "the copy is not cooked yet")
	assert.Equal(t, 3, v.Meals)
	assert.Equal(t, []plate.LinkKind{plate.KindFresh, plate.KindLeftover, plate.KindLeftover},
		[]plate.LinkKind{v.Links[0].Kind, v.Links[1].Kind, v.Links[2].Kind})
}

// A leftover whose batch was cooked before the source window becomes a fresh
// session of its own in the target window.
func TestCopyWeek_SessionCookedBeforeWindowBecomesFresh(t *testing.T) {
	e := newApplyTestEnv(t)
	sun := copySource.AddDate(0, 0, -1)
	cook := e.create(t, sun, e.slotID, composedComp(e.chiliID))
	sid := *cook.Components[0].CookSessionID
	_, err := e.plateSvc.AddLeftovers(e.ctx, sid, []plate.CellKey{{Date: copySource, SlotID: e.lunchID}})
	require.NoError(t, err)

	res, err := e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{SourceStart: copySource, TargetStart: copyTarget})
	require.NoError(t, err)
	require.Len(t, res.CreatedPlateIDs, 1)
	p, err := e.plateSvc.Get(e.ctx, res.CreatedPlateIDs[0])
	require.NoError(t, err)
	v, err := e.plateSvc.Session(e.ctx, *p.Components[0].CookSessionID)
	require.NoError(t, err)
	assert.NotEqual(t, sid, v.ID)
	assert.Equal(t, copyTarget, v.CookDate)
	assert.Equal(t, e.lunchID, v.CookSlotID)
	assert.InDelta(t, 2.0, v.Portions, 1e-9)
	assert.Equal(t, 1, v.Meals)
}

// B2-AC3 for copy week: overwriting a week that holds a batch, then undoing,
// restores the batch.
func TestCopyWeek_Overwrite_UndoRestoresThePlan(t *testing.T) {
	e := newApplyTestEnv(t)
	e.leafPlate(t, copySource)
	e.leafPlate(t, copySource.AddDate(0, 0, 1))
	// The target week: a batch on its first two days, plus a lunch.
	cook := e.create(t, copyTarget, e.slotID, composedComp(e.chiliID))
	sid := *cook.Components[0].CookSessionID
	_, err := e.plateSvc.AddLeftovers(e.ctx, sid, []plate.CellKey{{Date: copyTarget.AddDate(0, 0, 1), SlotID: e.slotID}})
	require.NoError(t, err)
	e.create(t, copyTarget, e.lunchID, leafComp(e.foodID, 20))
	targetEnd := copyTarget.AddDate(0, 0, 6)
	before := e.plan(t, copyTarget, targetEnd)

	res, err := e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{SourceStart: copySource, TargetStart: copyTarget, OnConflict: preset.ConflictOverwrite})
	require.NoError(t, err)
	require.Len(t, res.CreatedPlateIDs, 2)
	require.Len(t, res.Removed.Plates, 2)
	require.Len(t, res.Removed.Sessions, 1, "the whole batch went, recorded once by ref")
	assert.Equal(t, 0, e.count(t, "cook_sessions"))

	e.undo(t, res)
	assert.Equal(t, before, e.plan(t, copyTarget, targetEnd))
}

func TestCopyWeek_AvailabilityAndUndo(t *testing.T) {
	e := newApplyTestEnv(t)
	src, tgt := copySource, copyTarget

	e.leafPlate(t, src)                                   // Mon: plate → copied
	e.setNone(t, src.AddDate(0, 0, 1), ptrStr("canteen")) // Tue: none → copied with note
	e.leafPlate(t, src.AddDate(0, 0, 2))                  // Wed: plate, but target Wed is none
	e.setNone(t, tgt.AddDate(0, 0, 2), nil)
	e.setNone(t, src.AddDate(0, 0, 3), nil) // Thu: none, but target Thu has a plate
	e.leafPlate(t, tgt.AddDate(0, 0, 3))

	res, err := e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{SourceStart: src, TargetStart: tgt})
	require.NoError(t, err)

	require.Len(t, res.CreatedPlateIDs, 1)
	assert.Equal(t, []plate.CellKey{{Date: tgt.AddDate(0, 0, 2), SlotID: e.slotID}}, res.SkippedUnavailable)
	assert.Equal(t, []plate.CellKey{{Date: tgt.AddDate(0, 0, 3), SlotID: e.slotID}}, res.SkippedOccupied)
	assert.Equal(t, []plate.CellKey{{Date: tgt.AddDate(0, 0, 1), SlotID: e.slotID}}, res.CreatedOverrides)

	tue := e.cell(t, tgt.AddDate(0, 0, 1))
	assert.Equal(t, availability.ModeNone, tue.Mode)
	assert.Equal(t, availability.SourceOverride, tue.Source)
	assert.Equal(t, ptrStr("canteen"), tue.Note)

	e.undo(t, res)

	assert.Equal(t, availability.ModePlan, e.cell(t, tgt.AddDate(0, 0, 1)).Mode, "undo removes the copied override")
	assert.Equal(t, availability.ModeNone, e.cell(t, tgt.AddDate(0, 0, 2)).Mode, "undo keeps overrides it did not create")
	day, err := e.plateSvc.Day(e.ctx, tgt)
	require.NoError(t, err)
	assert.Empty(t, day, "undo removes the copied plate")
}

func TestCopyWeek_OnlyNoneOverrides(t *testing.T) {
	e := newApplyTestEnv(t)
	src, tgt := copySource, copyTarget
	e.setNone(t, src.AddDate(0, 0, 4), nil)
	// A routine override in the source is not copied.
	presetP := e.seedPreset(t, "Oats")
	_, _, err := e.availSvc.SetOverride(e.ctx, availability.OverrideInput{
		Date: src.AddDate(0, 0, 5), SlotID: e.slotID, Mode: availability.ModeRoutine, PresetID: &presetP.ID,
	})
	require.NoError(t, err)

	res, err := e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{SourceStart: src, TargetStart: tgt})
	require.NoError(t, err)
	assert.Empty(t, res.CreatedPlateIDs)
	assert.Equal(t, []plate.CellKey{{Date: tgt.AddDate(0, 0, 4), SlotID: e.slotID}}, res.CreatedOverrides)
	assert.Equal(t, availability.ModeNone, e.cell(t, tgt.AddDate(0, 0, 4)).Mode)
	assert.Equal(t, availability.ModePlan, e.cell(t, tgt.AddDate(0, 0, 5)).Mode, "routine overrides are not copied")
}

// failingAvailability wraps the real availability service and fails
// SetNoneIfUnset for the cells in setErrs.
type failingAvailability struct {
	*availability.Service
	setErrs map[string]error // key: YYYY-MM-DD
}

func (f failingAvailability) SetNoneIfUnset(ctx context.Context, date time.Time, slotID int64, note *string) (bool, error) {
	if err, ok := f.setErrs[date.Format("2006-01-02")]; ok {
		return false, err
	}
	return f.Service.SetNoneIfUnset(ctx, date, slotID, note)
}

func TestCopyWeek_OverrideCopyIsBestEffort(t *testing.T) {
	e := newApplyTestEnv(t)
	src, tgt := copySource, copyTarget
	e.leafPlate(t, src)
	for i := 1; i <= 3; i++ {
		e.setNone(t, src.AddDate(0, 0, i), nil)
	}
	e.presetSvc.WithAvailability(failingAvailability{Service: e.availSvc, setErrs: map[string]error{
		tgt.AddDate(0, 0, 1).Format("2006-01-02"): fmt.Errorf("race: %w", domain.ErrCellHasPlate),
		tgt.AddDate(0, 0, 2).Format("2006-01-02"): errors.New("disk on fire"),
	}})

	res, err := e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{SourceStart: src, TargetStart: tgt})
	require.NoError(t, err, "a failed override copy must not lose what undoes the copy")

	require.Len(t, res.CreatedPlateIDs, 1)
	assert.Equal(t, []plate.CellKey{{Date: tgt.AddDate(0, 0, 1), SlotID: e.slotID}}, res.SkippedOccupied,
		"a plate that appeared in the cell is reported as occupied")
	assert.Equal(t, []plate.CellKey{{Date: tgt.AddDate(0, 0, 3), SlotID: e.slotID}}, res.CreatedOverrides,
		"only overrides that were written are reported")
	assert.Equal(t, availability.ModePlan, e.cell(t, tgt.AddDate(0, 0, 2)).Mode)
}

func TestCopyWeek_EmptySourceWeek(t *testing.T) {
	e := newApplyTestEnv(t)
	res, err := e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{SourceStart: copySource, TargetStart: copyTarget})
	require.NoError(t, err)
	assert.Empty(t, res.CreatedPlateIDs)
	assert.Empty(t, res.SkippedOccupied)
	assert.Empty(t, res.SkippedNoSlot)
	assert.Empty(t, res.Removed.Plates)
	assert.Empty(t, res.CreatedOverrides)
}

func TestCopyWeek_OverwriteConflict(t *testing.T) {
	e := newApplyTestEnv(t)
	e.leafPlate(t, copySource)
	e.create(t, copyTarget, e.slotID, leafComp(e.foodID, 50))

	res, err := e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{SourceStart: copySource, TargetStart: copyTarget, OnConflict: preset.ConflictOverwrite})
	require.NoError(t, err)
	require.Len(t, res.CreatedPlateIDs, 1)
	require.Len(t, res.Removed.Plates, 1)
	assert.InDelta(t, 50.0, *res.Removed.Plates[0].Components[0].Amount, 1e-9)

	day, err := e.plateSvc.Day(e.ctx, copyTarget)
	require.NoError(t, err)
	require.Len(t, day, 1)
	assert.InDelta(t, 100.0, *day[0].Components[0].Amount, 1e-9, "target slot now holds the source plate's amount")
}

func TestCopyWeek_Validation(t *testing.T) {
	e := newApplyTestEnv(t)
	_, err := e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{SourceStart: copySource})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = e.presetSvc.CopyWeek(e.ctx, preset.CopyWeekRequest{SourceStart: copySource, TargetStart: copyTarget, OnConflict: "merge"})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}
