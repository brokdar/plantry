package plate_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Meal editing slice 2a: every removal keeps the batch invariant R1 (an
// uncooked batch cooks at its earliest meal unless cooked ahead, never on a
// No-plate cell) and reports the cook moves it made.

// describePlan lists the plates and the sessions they eat from in [sun, thu]
// as "date/slot food cook=date/slot portions=n", so a test can pin the plan
// before and after an action.
func (e *env) describePlan() []string {
	e.t.Helper()
	plates, err := e.svc.Range(e.ctx, day(sun), day(thu))
	require.NoError(e.t, err)
	names := map[int64]string{e.lunch: "lunch", e.dinner: "dinner", e.brkfst: "breakfast"}
	foods := map[int64]string{e.chili: "chili", e.curry: "curry", e.rice: "rice", e.eg: "egg"}
	out := []string{}
	for _, p := range plates {
		for _, c := range p.Components {
			line := fmt.Sprintf("%s/%s %s", p.DateString(), names[p.SlotID], foods[c.FoodID])
			if c.CookSessionID != nil {
				v := e.session(*c.CookSessionID)
				line += fmt.Sprintf(" cook=%s/%s portions=%g", v.CookDate.Format("2006-01-02"), names[v.CookSlotID], v.Portions)
			}
			out = append(out, line)
		}
	}
	return out
}

// lasagne cooks chili on Wed lunch with leftovers Wed dinner and Thu lunch
// (2 eaters each: an exact batch of 6).
func (e *env) lasagne() (wedLunch *plate.Plate, sid int64) {
	e.t.Helper()
	wedLunch = e.create(wed, e.lunch, nil, composed(e.chili, 1))
	sid = sessionOf(wedLunch, 0)
	_, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(wed, e.dinner), cell(thu, e.lunch)})
	require.NoError(e.t, err)
	return wedLunch, sid
}

func TestSetAway_MovesTheCookToTheFirstMealLeft(t *testing.T) {
	e := newEnv(t)
	_, sid := e.lasagne()
	require.Equal(t, []string{
		"2026-11-04/lunch chili cook=2026-11-04/lunch portions=6",
		"2026-11-04/dinner chili cook=2026-11-04/lunch portions=6",
		"2026-11-05/lunch chili cook=2026-11-04/lunch portions=6",
	}, e.describePlan())

	res, err := e.avail.SetAway(e.ctx, availability.AwayInput{
		Cells: []plate.CellKey{cell(wed, e.lunch), cell(wed, e.dinner)}, Note: ptrStr("trip"), Today: day(mon),
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"2026-11-05/lunch chili cook=2026-11-05/lunch portions=2"}, e.describePlan(),
		"the batch cooks Thu lunch with Thu's consumption")
	require.Len(t, res.AlsoChanged, 1)
	thuLunch := e.plateAt(thu, e.lunch)
	assert.Equal(t, plate.AlsoChanged{
		SessionID: sid, PlateID: thuLunch.ID, Date: thu, SlotID: e.lunch, What: plate.AlsoCookMoved, FromDate: wed, FromSlotID: e.lunch,
	}, res.AlsoChanged[0])
	assert.Len(t, res.Removed.Plates, 2)
	require.Len(t, res.Cells, 2)
	for _, c := range res.Cells {
		assert.Equal(t, availability.ModeNone, c.Mode)
		assert.Equal(t, "trip", *c.Note)
	}
	require.Len(t, res.Before, 2)
	assert.Nil(t, res.Before[0].Override)
	var onWed int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM cook_sessions WHERE cook_date = ?`, wed).Scan(&onWed))
	assert.Zero(t, onWed, "no session cooks on Wed")
	assert.Empty(t, e.session(sid).Warnings)
}

func TestSetAway_UndoRestoresThePlates(t *testing.T) {
	e := newEnv(t)
	e.lasagne()
	before := e.describePlan()
	res, err := e.avail.SetAway(e.ctx, availability.AwayInput{Cells: []plate.CellKey{cell(wed, e.lunch), cell(wed, e.dinner)}, Today: day(mon)})
	require.NoError(t, err)

	for _, b := range res.Before {
		require.NoError(t, e.avail.RestoreOverride(e.ctx, b.Date, b.SlotID, b.Override))
	}
	_, err = e.svc.Apply(e.ctx, res.Removed, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})

	require.NoError(t, err)
	assert.Equal(t, before, e.describePlan(), "plates back, and the cook back on Wed lunch")
}

func TestSetAway_PastDateFails(t *testing.T) {
	e := newEnv(t)
	e.lasagne()

	_, err := e.avail.SetAway(e.ctx, availability.AwayInput{Cells: []plate.CellKey{cell(wed, e.lunch)}, Today: day(thu)})

	require.ErrorIs(t, err, domain.ErrPastCell)
	assert.Equal(t, 3, e.count("plates"))
	assert.Equal(t, 0, e.count("availability_overrides"))
}

func TestSetAway_DateSkipsRoutineCells(t *testing.T) {
	e := newEnv(t)
	preset := e.insert(`INSERT INTO presets (name) VALUES ('Muesli')`)
	_, _, err := e.avail.SetOverride(e.ctx, availability.OverrideInput{Date: day(wed), SlotID: e.brkfst, Mode: availability.ModeRoutine, PresetID: &preset})
	require.NoError(t, err)
	e.lasagne()
	date := day(wed)

	res, err := e.avail.SetAway(e.ctx, availability.AwayInput{Date: &date, Today: day(mon)})

	require.NoError(t, err)
	require.Len(t, res.Cells, 2)
	assert.Equal(t, e.lunch, res.Cells[0].SlotID)
	assert.Equal(t, e.dinner, res.Cells[1].SlotID)
	mode, err := e.avail.ModeAt(e.ctx, day(wed), e.brkfst)
	require.NoError(t, err)
	assert.Equal(t, availability.ModeRoutine, mode)
	require.Len(t, res.AlsoChanged, 1)

	_, err = e.avail.SetAway(e.ctx, availability.AwayInput{Today: day(mon)})
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "cells or date required")
}

func TestDelete_CookingMealReanchorsToTheNextMeal(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()

	_, err := e.svc.Delete(e.ctx, sunPlate.ID)

	require.NoError(t, err)
	v := e.session(sid)
	assert.True(t, cell(mon, e.lunch).Equal(v.CookCell()))
	assert.InDelta(t, 4, v.Portions, 1e-9)
	assert.Empty(t, v.Warnings)
}

func TestRemoveMeals_ReportsTheMove(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()

	res, err := e.svc.RemoveMeals(e.ctx, []int64{sunPlate.ID})

	require.NoError(t, err)
	require.Len(t, res.AlsoChanged, 1)
	assert.Equal(t, sid, res.AlsoChanged[0].SessionID)
	assert.Equal(t, e.plateAt(mon, e.lunch).ID, res.AlsoChanged[0].PlateID)
	assert.Equal(t, mon, res.AlsoChanged[0].Date)
	assert.Equal(t, sun, res.AlsoChanged[0].FromDate)
	assert.Equal(t, e.dinner, res.AlsoChanged[0].FromSlotID)
}

func TestDelete_CookedBatchKeepsItsCook(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()
	_, err := e.svc.SetCooked(e.ctx, sid, true)
	require.NoError(t, err)

	res, err := e.svc.RemoveMeals(e.ctx, []int64{sunPlate.ID})

	require.NoError(t, err)
	assert.Empty(t, res.AlsoChanged)
	v := e.session(sid)
	assert.True(t, cell(sun, e.dinner).Equal(v.CookCell()))
	assert.InDelta(t, 4, v.Portions, 1e-9, "portions follow D11 as before")
}

// A batch cooked ahead on Tue dinner (no plate there) for Wed and Thu keeps
// its cook when Wed's plate goes.
func TestDelete_CookedAheadBatchKeepsItsCook(t *testing.T) {
	e := newEnv(t)
	tueDinner := e.create(tue, e.dinner, nil, composed(e.chili, 1))
	sid := sessionOf(tueDinner, 0)
	wedLunch := e.create(wed, e.lunch, nil, linked(e.chili, 1, sid))
	e.create(thu, e.lunch, nil, linked(e.chili, 1, sid))
	_, err := e.svc.RemoveComponent(e.ctx, tueDinner.Components[0].ID)
	require.NoError(t, err)
	_, err = e.svc.Delete(e.ctx, tueDinner.ID)
	require.NoError(t, err)
	_, err = e.svc.SetSessionPortions(e.ctx, sid, plate.SessionPortions{Portions: 4})
	require.NoError(t, err)
	// Pin the cook ahead: Tue dinner, nothing eaten there.
	e.exec(`UPDATE cook_sessions SET cook_date = ?, cook_slot_id = ? WHERE id = ?`, tue, e.dinner, sid)

	res, err := e.svc.RemoveMeals(e.ctx, []int64{wedLunch.ID})

	require.NoError(t, err)
	assert.Empty(t, res.AlsoChanged)
	v := e.session(sid)
	assert.True(t, cell(tue, e.dinner).Equal(v.CookCell()))
	assert.InDelta(t, 2, v.Portions, 1e-9)

	// The cook cell turning No plate moves it to its earliest meal.
	away, err := e.avail.SetAway(e.ctx, availability.AwayInput{Cells: []plate.CellKey{cell(tue, e.dinner)}, Today: day(mon)})
	require.NoError(t, err)
	require.Len(t, away.AlsoChanged, 1)
	assert.True(t, cell(thu, e.lunch).Equal(e.session(sid).CookCell()))
}

func TestRemoveComponent_CookingComponentReanchors(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()
	_, err := e.svc.AddComponent(e.ctx, sunPlate.ID, grams(e.rice, 100))
	require.NoError(t, err)

	_, err = e.svc.RemoveComponent(e.ctx, sunPlate.Components[0].ID)

	require.NoError(t, err)
	v := e.session(sid)
	assert.True(t, cell(mon, e.lunch).Equal(v.CookCell()))
	assert.Empty(t, v.Warnings)
}

func TestSwapComponent_CookingComponentReanchorsTheOldBatch(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()

	got, err := e.svc.SwapComponent(e.ctx, sunPlate.Components[0].ID, e.curry, nil)

	require.NoError(t, err)
	v := e.session(sid)
	assert.True(t, cell(mon, e.lunch).Equal(v.CookCell()), "the old batch cooks at its first meal left")
	assert.InDelta(t, 4, v.Portions, 1e-9)
	fresh := e.session(sessionOf(got, 0))
	assert.True(t, cell(sun, e.dinner).Equal(fresh.CookCell()), "the new food cooks fresh on the plate's cell")
}

func TestRemoveMeals_MissingPlateRemovesNothing(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()

	_, err := e.svc.RemoveMeals(e.ctx, []int64{sunPlate.ID, 9999})

	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Equal(t, 3, e.count("plates"))
	assert.True(t, cell(sun, e.dinner).Equal(e.session(sid).CookCell()))
}

func TestPutOverride_RemovePlateReanchors(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()

	_, res, err := e.avail.SetOverrideMeals(e.ctx, availability.OverrideInput{Date: day(sun), SlotID: e.dinner, Mode: availability.ModeNone, RemovePlate: true})

	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Removed.Plates, 1)
	assert.Equal(t, sunPlate.DateString(), res.Removed.Plates[0].Date)
	require.Len(t, res.AlsoChanged, 1)
	assert.True(t, cell(mon, e.lunch).Equal(e.session(sid).CookCell()))
}

// Clearing the cook slot's plates (what the e2e slot cleanup does) moves the
// batch off that slot, so the slot can be deleted: cook_sessions.cook_slot_id
// is ON DELETE RESTRICT. Before R1 the session kept cooking there and pinned
// the slot, which then stayed active and gave later plans extra meals.
func TestClearRange_CookSlotIsFreedForDeletion(t *testing.T) {
	e := newEnv(t)
	cook := e.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.cook', 'moon', 1000000, 1)`)
	sunCook := e.create(sun, cook, nil, composed(e.chili, 1))
	sid := sessionOf(sunCook, 0)
	_, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(mon, e.lunch), cell(mon, e.dinner)})
	require.NoError(t, err)

	_, err = e.svc.ClearRange(e.ctx, day(sun), day(sun), cook)

	require.NoError(t, err)
	assert.True(t, cell(mon, e.lunch).Equal(e.session(sid).CookCell()), "the batch cooks at its first meal left")
	_, err = e.db.Exec(`DELETE FROM time_slots WHERE id = ?`, cook)
	assert.NoError(t, err, "no session pins the cook slot")
}
