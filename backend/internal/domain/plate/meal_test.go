package plate_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Meal editing slice 2b: move, swap, occupant handling, replace by
// position, move cook, copy and the hand Undo. Each test pins the plan
// before and after the action, sessions included. Keep days default 3.

// week lists every plate component in [sun, sat] as "date/slot food" plus,
// for a composed one, "cook=date/slot portions=n" of its session.
func (e *env) week() []string {
	e.t.Helper()
	plates, err := e.svc.Range(e.ctx, day(sun), day(sat))
	require.NoError(e.t, err)
	names := map[int64]string{e.lunch: "lunch", e.dinner: "dinner", e.brkfst: "breakfast"}
	foods := map[int64]string{e.chili: "chili", e.curry: "curry", e.rice: "rice", e.eg: "egg"}
	out := []string{}
	for _, p := range plates {
		for _, c := range p.Components {
			line := fmt.Sprintf("%s/%s %s x%g", p.DateString(), names[p.SlotID], foods[c.FoodID], p.Eaters)
			if c.CookSessionID != nil {
				v := e.session(*c.CookSessionID)
				line += fmt.Sprintf(" cook=%s/%s portions=%g", v.CookDate.Format("2006-01-02"), names[v.CookSlotID], v.Portions)
			}
			out = append(out, line)
		}
	}
	return out
}

func (e *env) move(p *plate.Plate, date string, slot int64, occ plate.Occupant) (plate.MealResult, error) {
	e.t.Helper()
	return e.svc.MoveMeal(e.ctx, p.ID, cell(date, slot), occ)
}

func whats(also []plate.AlsoChanged) []string {
	out := []string{}
	for _, a := range also {
		out = append(out, a.What)
	}
	return out
}

// ── Moves ───────────────────────────────────────────────────────────────

func TestMoveMeal_ToEmptyKeepsALeftoverWithinKeep(t *testing.T) {
	e := newEnv(t)
	e.batch()

	res, err := e.move(e.plateAt(mon, e.lunch), tue, e.lunch, plate.Occupant{})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-01/dinner chili x2 cook=2026-11-01/dinner portions=6",
		"2026-11-02/dinner chili x2 cook=2026-11-01/dinner portions=6",
		"2026-11-03/lunch chili x2 cook=2026-11-01/dinner portions=6",
	}, e.week())
	assert.Empty(t, res.AlsoChanged)
	require.Len(t, res.Plates, 1)
	assert.Equal(t, day(tue), res.Plates[0].Date)
	assert.NotZero(t, res.Undo.FirstEventID)
}

// lateBatch cooks chili on Tue dinner with leftovers Wed lunch and Wed
// dinner.
func (e *env) lateBatch() int64 {
	e.t.Helper()
	p := e.create(tue, e.dinner, nil, composed(e.chili, 1))
	sid := sessionOf(p, 0)
	_, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(wed, e.lunch), cell(wed, e.dinner)})
	require.NoError(e.t, err)
	return sid
}

// freezableChili turns freezing on, keep days to 1 and makes chili freezable.
func (e *env) freezableChili() {
	e.t.Helper()
	e.set("plan.freezing_enabled", "true")
	e.set("plan.freezer_keep_days", "30")
	e.set("plan.default_keep_days", "1")
	e.exec(`UPDATE foods SET freezable = 1 WHERE id = ?`, e.chili)
}

// frozenBatch is a chili batch cooked Mon dinner with a leftover on Wed dinner
// (past the 1 keep day, so classified frozen before any move). It returns the
// session and the Mon plate.
func (e *env) frozenBatch() (sid int64, monPlate *plate.Plate) {
	e.t.Helper()
	e.freezableChili()
	monPlate = e.create(mon, e.dinner, nil, composed(e.chili, 1))
	sid = sessionOf(monPlate, 0)
	_, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(wed, e.dinner)})
	require.NoError(e.t, err)
	return sid, monPlate
}

func TestMoveMeal_LeftoverBeforeItsCookReanchorsTheBatch(t *testing.T) {
	e := newEnv(t)
	sid := e.lateBatch()
	wedDinner := e.plateAt(wed, e.dinner)

	res, err := e.move(wedDinner, mon, e.lunch, plate.Occupant{})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-02/lunch chili x2 cook=2026-11-02/lunch portions=6",
		"2026-11-03/dinner chili x2 cook=2026-11-02/lunch portions=6",
		"2026-11-04/lunch chili x2 cook=2026-11-02/lunch portions=6",
	}, e.week())
	assert.Equal(t, []plate.AlsoChanged{{
		SessionID: sid, PlateID: wedDinner.ID, Date: mon, SlotID: e.lunch, What: plate.AlsoCookMoved, FromDate: tue, FromSlotID: e.dinner,
	}}, res.AlsoChanged, "the cook day is reported")
}

func TestMoveMeal_LeftoverBeforeItsCookCooksFreshWhenKeepWouldBreak(t *testing.T) {
	e := newEnv(t)
	e.set("plan.default_keep_days", "1")
	sid := e.lateBatch()
	wedDinner := e.plateAt(wed, e.dinner)

	res, err := e.move(wedDinner, sun, e.lunch, plate.Occupant{})

	require.NoError(t, err)
	got := e.week()
	assert.Equal(t, []string{
		"2026-11-01/lunch chili x2 cook=2026-11-01/lunch portions=2",
		"2026-11-03/dinner chili x2 cook=2026-11-03/dinner portions=4",
		"2026-11-04/lunch chili x2 cook=2026-11-03/dinner portions=4",
	}, got, "Wed lunch would be past Sun's keep, so the moved meal cooks fresh")
	assert.Equal(t, []string{plate.AlsoCooksFresh, plate.AlsoBatchResized}, whats(res.AlsoChanged))
	assert.Equal(t, "before_cook", res.AlsoChanged[0].Why)
	assert.Equal(t, sid, res.AlsoChanged[1].SessionID)
}

func TestMoveMeal_CookingMealEarlierTakesTheBatchWhenKeepHolds(t *testing.T) {
	e := newEnv(t)
	sid := e.lateBatch()
	tueDinner := e.plateAt(tue, e.dinner)

	res, err := e.move(tueDinner, mon, e.lunch, plate.Occupant{})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-02/lunch chili x2 cook=2026-11-02/lunch portions=6",
		"2026-11-04/lunch chili x2 cook=2026-11-02/lunch portions=6",
		"2026-11-04/dinner chili x2 cook=2026-11-02/lunch portions=6",
	}, e.week(), "the batch follows its cooking meal")
	assert.Equal(t, []plate.AlsoChanged{{
		SessionID: sid, PlateID: tueDinner.ID, Date: mon, SlotID: e.lunch, What: plate.AlsoCookMoved, FromDate: tue, FromSlotID: e.dinner,
	}}, res.AlsoChanged)
	assert.Empty(t, e.session(sid).Warnings)
}

func TestMoveMeal_CookingMealEarlierCooksFreshWhenKeepWouldBreak(t *testing.T) {
	e := newEnv(t)
	e.set("plan.default_keep_days", "1")
	sid := e.lateBatch()
	tueDinner := e.plateAt(tue, e.dinner)
	wedLunch := e.plateAt(wed, e.lunch)

	res, err := e.move(tueDinner, mon, e.lunch, plate.Occupant{})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-02/lunch chili x2 cook=2026-11-02/lunch portions=2",
		"2026-11-04/lunch chili x2 cook=2026-11-04/lunch portions=4",
		"2026-11-04/dinner chili x2 cook=2026-11-04/lunch portions=4",
	}, e.week(), "Wed would be past Mon's keep: the named meal cooks fresh, the batch re-anchors to Wed lunch")
	assert.Equal(t, []string{plate.AlsoBecameCooking, plate.AlsoCooksFresh, plate.AlsoBatchResized}, whats(res.AlsoChanged))
	assert.Equal(t, wedLunch.ID, res.AlsoChanged[0].PlateID)
	assert.Equal(t, tueDinner.ID, res.AlsoChanged[1].PlateID)
	assert.Equal(t, "before_cook", res.AlsoChanged[1].Why)
	assert.Empty(t, e.session(sid).Warnings)
	for _, p := range res.Plates {
		for _, c := range p.Components {
			assert.Empty(t, e.session(*c.CookSessionID).Warnings)
		}
	}
}

func TestMoveMeal_LeftoverPastKeepCooksFreshAndTheBatchShrinks(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()

	res, err := e.move(e.plateAt(mon, e.dinner), thu, e.dinner, plate.Occupant{})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-01/dinner chili x2 cook=2026-11-01/dinner portions=4",
		"2026-11-02/lunch chili x2 cook=2026-11-01/dinner portions=4",
		"2026-11-05/dinner chili x2 cook=2026-11-05/dinner portions=2",
	}, e.week())
	assert.Equal(t, []string{plate.AlsoCooksFresh, plate.AlsoBatchResized}, whats(res.AlsoChanged))
	assert.Equal(t, "past_keep_days", res.AlsoChanged[0].Why)
	assert.Equal(t, sid, res.AlsoChanged[1].SessionID)
}

func TestMoveMeal_CookingMealLaterHandsTheCookToTheEarliestMeal(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()
	monLunch := e.plateAt(mon, e.lunch)

	res, err := e.move(sunPlate, tue, e.dinner, plate.Occupant{})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-02/lunch chili x2 cook=2026-11-02/lunch portions=6",
		"2026-11-02/dinner chili x2 cook=2026-11-02/lunch portions=6",
		"2026-11-03/dinner chili x2 cook=2026-11-02/lunch portions=6",
	}, e.week())
	assert.Equal(t, []plate.AlsoChanged{{
		SessionID: sid, PlateID: monLunch.ID, Date: mon, SlotID: e.lunch, What: plate.AlsoBecameCooking, FromDate: sun, FromSlotID: e.dinner,
	}}, res.AlsoChanged)
}

func TestMoveMeal_IntoANoneCellIsRefused(t *testing.T) {
	e := newEnv(t)
	e.batch()
	_, _, err := e.avail.SetOverride(e.ctx, availability.OverrideInput{Date: day(tue), SlotID: e.lunch, Mode: availability.ModeNone})
	require.NoError(t, err)
	before := e.week()

	_, err = e.move(e.plateAt(mon, e.lunch), tue, e.lunch, plate.Occupant{})

	require.ErrorIs(t, err, domain.ErrCellUnavailable)
	assert.Equal(t, before, e.week())
}

func TestMoveMeal_IntoOrFromThePastIsRefused(t *testing.T) {
	e := newEnvAt(t, nil, time.Date(2026, 11, 3, 12, 0, 0, 0, time.Local))
	e.batch()
	thuPlate := e.create(thu, e.lunch, nil, composed(e.curry, 1))
	before := e.week()

	_, err := e.move(e.plateAt(mon, e.lunch), thu, e.dinner, plate.Occupant{})
	require.ErrorIs(t, err, domain.ErrPastCell, "from the past")
	_, err = e.move(thuPlate, mon, e.brkfst, plate.Occupant{})
	require.ErrorIs(t, err, domain.ErrPastCell, "into the past")

	assert.Equal(t, before, e.week())
}

func TestMoveMeal_AConfirmedMealMovesByHandAndItsBatchIsReported(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()
	_, err := e.svc.Confirm(e.ctx, plate.RowSet{PlateIDs: []int64{sunPlate.ID}})
	require.NoError(t, err)

	res, err := e.move(e.plateAt(mon, e.dinner), thu, e.dinner, plate.Occupant{})

	require.NoError(t, err)
	require.Len(t, res.AlsoChanged, 2)
	assert.Equal(t, plate.AlsoBatchResized, res.AlsoChanged[1].What)
	assert.Equal(t, sid, res.AlsoChanged[1].SessionID, "the confirmed batch's other meals lose a co-eater")
	assert.InDelta(t, 4.0, e.session(sid).Portions, 1e-9)
}

// ── Swaps and occupants ─────────────────────────────────────────────────

func TestMoveMeal_SwapTwoFreshDinnersKeepsTheEatersWithTheCells(t *testing.T) {
	e := newEnv(t)
	chili := e.create(mon, e.dinner, f64(2), composed(e.chili, 1))
	e.create(tue, e.dinner, f64(4), composed(e.curry, 1))

	res, err := e.move(chili, tue, e.dinner, plate.Occupant{Kind: plate.OccupantSwap})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-02/dinner curry x2 cook=2026-11-02/dinner portions=2",
		"2026-11-03/dinner chili x4 cook=2026-11-03/dinner portions=4",
	}, e.week())
	assert.Len(t, res.Plates, 2)
}

func TestMoveMeal_SwapACookingMealWithALeftoverOfAnotherBatch(t *testing.T) {
	e := newEnv(t)
	sunPlate, chiliSID := e.batch()
	curry := e.create(sun, e.lunch, nil, composed(e.curry, 1))
	_, err := e.svc.AddLeftovers(e.ctx, sessionOf(curry, 0), []plate.CellKey{cell(tue, e.lunch)})
	require.NoError(t, err)
	monLunch := e.plateAt(mon, e.lunch)

	res, err := e.move(sunPlate, tue, e.lunch, plate.Occupant{Kind: plate.OccupantSwap})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-01/lunch curry x2 cook=2026-11-01/lunch portions=4",
		"2026-11-01/dinner curry x2 cook=2026-11-01/lunch portions=4",
		"2026-11-02/lunch chili x2 cook=2026-11-02/lunch portions=6",
		"2026-11-02/dinner chili x2 cook=2026-11-02/lunch portions=6",
		"2026-11-03/lunch chili x2 cook=2026-11-02/lunch portions=6",
	}, e.week())
	assert.Equal(t, []plate.AlsoChanged{{
		SessionID: chiliSID, PlateID: monLunch.ID, Date: mon, SlotID: e.lunch, What: plate.AlsoBecameCooking, FromDate: sun, FromSlotID: e.dinner,
	}}, res.AlsoChanged)
}

func TestMoveMeal_SwapTwoMealsOfOneBatchKeepsTheBatch(t *testing.T) {
	e := newEnv(t)
	sunPlate, _ := e.batch()
	before := e.week()

	res, err := e.move(sunPlate, mon, e.dinner, plate.Occupant{Kind: plate.OccupantSwap})

	require.NoError(t, err)
	assert.Equal(t, before, e.week(), "same dishes, same cook cell")
	assert.Empty(t, res.AlsoChanged)
}

func TestMoveMeal_OccupantMovesOnToAThirdCell(t *testing.T) {
	e := newEnv(t)
	e.batch()
	curry := e.create(tue, e.dinner, nil, composed(e.curry, 1))

	_, err := e.move(curry, mon, e.lunch, plate.Occupant{Kind: plate.OccupantTo, To: cell(wed, e.lunch)})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-01/dinner chili x2 cook=2026-11-01/dinner portions=6",
		"2026-11-02/lunch curry x2 cook=2026-11-02/lunch portions=2",
		"2026-11-02/dinner chili x2 cook=2026-11-01/dinner portions=6",
		"2026-11-04/lunch chili x2 cook=2026-11-01/dinner portions=6",
	}, e.week())
}

func TestMoveMeal_DroppedOccupantsBatchReanchors(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()
	curry := e.create(tue, e.dinner, nil, composed(e.curry, 1))
	monLunch := e.plateAt(mon, e.lunch)

	res, err := e.move(curry, sun, e.dinner, plate.Occupant{Kind: plate.OccupantDrop})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-01/dinner curry x2 cook=2026-11-01/dinner portions=2",
		"2026-11-02/lunch chili x2 cook=2026-11-02/lunch portions=4",
		"2026-11-02/dinner chili x2 cook=2026-11-02/lunch portions=4",
	}, e.week())
	require.NotEmpty(t, res.AlsoChanged)
	assert.Equal(t, plate.AlsoChanged{
		SessionID: sid, PlateID: monLunch.ID, Date: mon, SlotID: e.lunch, What: plate.AlsoCookMoved, FromDate: sun, FromSlotID: e.dinner,
	}, res.AlsoChanged[0])
}

func TestMoveMeal_DroppedCookingMealAndTheNextMealMovesInTakesTheBatch(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()
	monLunch := e.plateAt(mon, e.lunch)

	res, err := e.move(monLunch, sun, e.dinner, plate.Occupant{Kind: plate.OccupantDrop})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-01/dinner chili x2 cook=2026-11-01/dinner portions=4",
		"2026-11-02/dinner chili x2 cook=2026-11-01/dinner portions=4",
	}, e.week(), "the moved meal is the batch's cooking meal again")
	assert.NotContains(t, whats(res.AlsoChanged), plate.AlsoCooksFresh)
	assert.Empty(t, e.session(sid).Warnings)
}

func TestMoveMeal_SwapFromANoneSourceIsRefused(t *testing.T) {
	e := newEnv(t)
	e.batch()
	// a plate can sit on a none cell when the override came after it
	_, err := e.db.Exec(`INSERT INTO availability_overrides (date, slot_id, mode) VALUES (?, ?, 'none')`, sun, e.dinner)
	require.NoError(t, err)
	before := e.week()

	_, err = e.move(e.plateAt(sun, e.dinner), mon, e.lunch, plate.Occupant{Kind: plate.OccupantSwap})

	require.ErrorIs(t, err, domain.ErrCellUnavailable)
	assert.Equal(t, before, e.week())
}

func TestMoveMeal_TakenCellWithoutOccupantIs409WithTheOccupant(t *testing.T) {
	e := newEnv(t)
	sunPlate, _ := e.batch()
	before := e.week()

	_, err := e.move(e.plateAt(mon, e.lunch), sun, e.dinner, plate.Occupant{})

	require.ErrorIs(t, err, domain.ErrCellOccupied)
	var occ *plate.OccupiedError
	require.ErrorAs(t, err, &occ)
	assert.Equal(t, sunPlate.ID, occ.PlateID)
	assert.Equal(t, before, e.week())
}

// ── Replace ─────────────────────────────────────────────────────────────

func TestReplaceComponent_MainKeepsTheSide(t *testing.T) {
	e := newEnv(t)
	p := e.create(mon, e.dinner, nil, composed(e.chili, 1), grams(e.rice, 100))

	res, err := e.svc.ReplaceComponent(e.ctx, p.ID, plate.Selector{Position: plate.PositionMain}, e.curry, plate.Source{}, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-02/dinner curry x2 cook=2026-11-02/dinner portions=2",
		"2026-11-02/dinner rice x2",
	}, e.week())
	assert.Equal(t, plate.PositionMain, res.Plates[0].Components[0].Position)
	assert.Equal(t, 0, e.componentCount(e.chili))
}

func TestReplaceComponent_SideKeepsTheMain(t *testing.T) {
	e := newEnv(t)
	side := grams(e.rice, 100)
	side.Position = plate.PositionSide
	p := e.create(mon, e.dinner, nil, composed(e.chili, 1), side)

	_, err := e.svc.ReplaceComponent(e.ctx, p.ID, plate.Selector{Position: plate.PositionSide}, e.eg, plate.Source{}, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-02/dinner chili x2 cook=2026-11-02/dinner portions=2",
		"2026-11-02/dinner egg x2",
	}, e.week())
	got := e.plateAt(mon, e.dinner)
	assert.Equal(t, plate.PositionSide, got.Components[1].Position)
}

func TestReplaceComponent_FromASession(t *testing.T) {
	e := newEnv(t)
	curry := e.create(sun, e.lunch, nil, composed(e.curry, 1))
	csid := sessionOf(curry, 0)
	mon := e.create(mon, e.dinner, nil, composed(e.chili, 1))
	thu := e.create(thu, e.dinner, nil, composed(e.chili, 1))

	_, err := e.svc.ReplaceComponent(e.ctx, thu.ID, plate.Selector{Position: plate.PositionMain}, e.curry, plate.Source{SessionID: csid}, nil)
	require.ErrorIs(t, err, domain.ErrSessionPastKeepDays, "Thu is past Sun's keep")

	_, err = e.svc.ReplaceComponent(e.ctx, mon.ID, plate.Selector{Position: plate.PositionMain}, e.curry, plate.Source{SessionID: csid}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-01/lunch curry x2 cook=2026-11-01/lunch portions=4",
		"2026-11-02/dinner curry x2 cook=2026-11-01/lunch portions=4",
		"2026-11-05/dinner chili x2 cook=2026-11-05/dinner portions=2",
	}, e.week())
}

func TestReplaceComponent_SameFoodIsRefused(t *testing.T) {
	e := newEnv(t)
	p := e.create(mon, e.dinner, nil, composed(e.chili, 1))

	_, err := e.svc.ReplaceComponent(e.ctx, p.ID, plate.Selector{Position: plate.PositionMain}, e.chili, plate.Source{}, nil)

	require.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestReplaceComponent_TheCookingComponentReanchorsItsBatch(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()
	monLunch := e.plateAt(mon, e.lunch)

	res, err := e.svc.ReplaceComponent(e.ctx, sunPlate.ID, plate.Selector{ComponentID: sunPlate.Components[0].ID}, e.curry, plate.Source{}, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-01/dinner curry x2 cook=2026-11-01/dinner portions=2",
		"2026-11-02/lunch chili x2 cook=2026-11-02/lunch portions=4",
		"2026-11-02/dinner chili x2 cook=2026-11-02/lunch portions=4",
	}, e.week())
	assert.Equal(t, []plate.AlsoChanged{{
		SessionID: sid, PlateID: monLunch.ID, Date: mon, SlotID: e.lunch, What: plate.AlsoCookMoved, FromDate: sun, FromSlotID: e.dinner,
	}}, res.AlsoChanged)
}

func TestReplaceComponent_CookAtCooksAhead(t *testing.T) {
	e := newEnv(t)
	p := e.create(tue, e.dinner, nil, composed(e.chili, 1))
	at := cell(mon, e.dinner)

	_, err := e.svc.ReplaceComponent(e.ctx, p.ID, plate.Selector{Position: plate.PositionMain}, e.curry, plate.Source{CookAt: &at}, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"2026-11-03/dinner curry x2 cook=2026-11-02/dinner portions=2"}, e.week())
}

// ── Move cook ───────────────────────────────────────────────────────────

func TestMoveCook_CooksAhead(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()

	res, err := e.svc.MoveCook(e.ctx, sid, cell(sun, e.lunch))

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-01/dinner chili x2 cook=2026-11-01/lunch portions=6",
		"2026-11-02/lunch chili x2 cook=2026-11-01/lunch portions=6",
		"2026-11-02/dinner chili x2 cook=2026-11-01/lunch portions=6",
	}, e.week())
	require.Len(t, res.Sessions, 1)
	assert.Empty(t, e.session(sid).Warnings)
}

func TestMoveCook_Refusals(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()
	_, _, err := e.avail.SetOverride(e.ctx, availability.OverrideInput{Date: day(sun), SlotID: e.brkfst, Mode: availability.ModeNone})
	require.NoError(t, err)
	before := e.week()

	_, err = e.svc.MoveCook(e.ctx, sid, cell(mon, e.dinner))
	require.ErrorIs(t, err, domain.ErrInvalidInput, "after the first meal")
	assert.Contains(t, err.Error(), "comes before the cook")

	_, err = e.svc.MoveCook(e.ctx, sid, cell(sun, e.brkfst))
	require.ErrorIs(t, err, domain.ErrCellUnavailable, "on a none cell")

	e.set("plan.default_keep_days", "1")
	_, err = e.svc.MoveCook(e.ctx, sid, cell("2026-10-31", e.dinner))
	require.ErrorIs(t, err, domain.ErrInvalidInput, "past keep")
	assert.Contains(t, err.Error(), "past the keeping time")
	e.set("plan.default_keep_days", "3")

	_, err = e.svc.SetCooked(e.ctx, sid, true)
	require.NoError(t, err)
	_, err = e.svc.MoveCook(e.ctx, sid, cell(sun, e.lunch))
	require.ErrorIs(t, err, domain.ErrInvalidInput, "cooked")

	assert.Equal(t, before, e.week())
}

// ── Copy ────────────────────────────────────────────────────────────────

func TestCopyMeal_AsLeftover(t *testing.T) {
	e := newEnv(t)
	sunPlate, _ := e.batch()
	before := e.week()

	_, err := e.svc.CopyMeal(e.ctx, sunPlate.ID, []plate.CellKey{cell(tue, e.lunch), cell(thu, e.lunch)}, plate.CopyLeftover)
	require.ErrorIs(t, err, domain.ErrSessionPastKeepDays, "Thu is out of keep: the whole call fails")
	assert.Equal(t, before, e.week())

	res, err := e.svc.CopyMeal(e.ctx, sunPlate.ID, []plate.CellKey{cell(tue, e.lunch)}, plate.CopyLeftover)
	require.NoError(t, err)
	assert.Len(t, res.Plates, 1)
	assert.Equal(t, []string{
		"2026-11-01/dinner chili x2 cook=2026-11-01/dinner portions=8",
		"2026-11-02/lunch chili x2 cook=2026-11-01/dinner portions=8",
		"2026-11-02/dinner chili x2 cook=2026-11-01/dinner portions=8",
		"2026-11-03/lunch chili x2 cook=2026-11-01/dinner portions=8",
	}, e.week())
}

func TestCopyMeal_AsFresh(t *testing.T) {
	e := newEnv(t)
	p := e.create(mon, e.dinner, nil, composed(e.chili, 1), grams(e.rice, 100))

	_, err := e.svc.CopyMeal(e.ctx, p.ID, []plate.CellKey{cell(thu, e.lunch)}, plate.CopyFresh)

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-02/dinner chili x2 cook=2026-11-02/dinner portions=2",
		"2026-11-02/dinner rice x2",
		"2026-11-05/lunch chili x2 cook=2026-11-05/lunch portions=2",
		"2026-11-05/lunch rice x2",
	}, e.week())
}

// ── Revert ──────────────────────────────────────────────────────────────

func TestRevert_EachOperationIsUndone(t *testing.T) {
	ops := map[string]func(e *env, sunPlate *plate.Plate, sid int64) (plate.MealResult, error){
		"move": func(e *env, sunPlate *plate.Plate, _ int64) (plate.MealResult, error) {
			return e.move(sunPlate, tue, e.lunch, plate.Occupant{})
		},
		"swap": func(e *env, sunPlate *plate.Plate, _ int64) (plate.MealResult, error) {
			return e.move(sunPlate, tue, e.dinner, plate.Occupant{Kind: plate.OccupantSwap})
		},
		"drop": func(e *env, _ *plate.Plate, _ int64) (plate.MealResult, error) {
			return e.move(e.plateAt(mon, e.lunch), sun, e.dinner, plate.Occupant{Kind: plate.OccupantDrop})
		},
		"replace": func(e *env, sunPlate *plate.Plate, _ int64) (plate.MealResult, error) {
			return e.svc.ReplaceComponent(e.ctx, sunPlate.ID, plate.Selector{Position: plate.PositionMain}, e.curry, plate.Source{}, nil)
		},
		"move cook": func(e *env, _ *plate.Plate, sid int64) (plate.MealResult, error) {
			return e.svc.MoveCook(e.ctx, sid, cell(sun, e.lunch))
		},
		"copy": func(e *env, sunPlate *plate.Plate, _ int64) (plate.MealResult, error) {
			return e.svc.CopyMeal(e.ctx, sunPlate.ID, []plate.CellKey{cell(tue, e.lunch)}, plate.CopyLeftover)
		},
		"remove": func(e *env, sunPlate *plate.Plate, _ int64) (plate.MealResult, error) {
			return e.svc.RemoveMeals(e.ctx, []int64{sunPlate.ID})
		},
	}
	for name, op := range ops {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			sunPlate, sid := e.batch()
			e.create(tue, e.dinner, f64(4), composed(e.curry, 1))
			before := e.week()
			res, err := op(e, sunPlate, sid)
			require.NoError(t, err)
			require.NotEqual(t, before, e.week())

			_, err = e.avail.Revert(e.ctx, res.Undo)

			require.NoError(t, err)
			assert.Equal(t, before, e.week())
		})
	}
}

func TestRevert_StaleIsRefused(t *testing.T) {
	e := newEnv(t)
	sunPlate, _ := e.batch()
	res, err := e.move(sunPlate, tue, e.dinner, plate.Occupant{})
	require.NoError(t, err)
	_, err = e.svc.Update(e.ctx, sunPlate.ID, plate.PlatePatch{Eaters: f64(3)})
	require.NoError(t, err)
	after := e.week()

	_, err = e.avail.Revert(e.ctx, res.Undo)

	require.ErrorIs(t, err, domain.ErrPlanChangeStale)
	assert.Equal(t, after, e.week())
}

func TestRevert_AnAwayCellIsRestored(t *testing.T) {
	e := newEnv(t)
	_, _, err := e.avail.SetOverride(e.ctx, availability.OverrideInput{Date: day(mon), SlotID: e.dinner, Mode: availability.ModeNone, Note: ptrStr("out")})
	require.NoError(t, err)
	e.create(mon, e.lunch, nil, composed(e.chili, 1))
	before := e.week()

	res, err := e.avail.SetAway(e.ctx, availability.AwayInput{Date: ptrTime(day(mon)), Note: ptrStr("trip"), Today: day(sun)})
	require.NoError(t, err)
	require.Len(t, res.Undo.Cells, 3)
	require.Empty(t, e.week())

	_, err = e.avail.Revert(e.ctx, res.Undo)

	require.NoError(t, err)
	assert.Equal(t, before, e.week())
	o, err := e.avail.Override(e.ctx, day(mon), e.dinner)
	require.NoError(t, err)
	require.NotNil(t, o)
	assert.Equal(t, "out", *o.Note)
	o, err = e.avail.Override(e.ctx, day(mon), e.lunch)
	require.NoError(t, err)
	assert.Nil(t, o)
}

func TestRevert_AnAwayCellEditedSinceIsStale(t *testing.T) {
	e := newEnv(t)
	e.create(mon, e.lunch, nil, composed(e.chili, 1))
	res, err := e.avail.SetAway(e.ctx, availability.AwayInput{Cells: []plate.CellKey{cell(mon, e.lunch)}, Today: day(sun)})
	require.NoError(t, err)
	require.NoError(t, e.avail.ClearOverride(e.ctx, day(mon), e.lunch))

	_, err = e.avail.Revert(e.ctx, res.Undo)

	require.ErrorIs(t, err, domain.ErrPlanChangeStale)
	assert.Empty(t, e.week())
}

// A meal operation never uses the freezer silently (design rule 1a): a named
// meal that moves past the keeping time leaves its batch and cooks fresh.
func TestMoveMeal_SwapPastKeepDoesNotFreezeSilently(t *testing.T) {
	e := newEnv(t)
	e.freezableChili()
	monPlate := e.create(mon, e.dinner, nil, composed(e.chili, 1))
	sid := sessionOf(monPlate, 0)
	_, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(tue, e.dinner)})
	require.NoError(t, err)
	e.create(thu, e.dinner, f64(3), composed(e.curry, 1))
	before := e.week()

	res, err := e.move(monPlate, thu, e.dinner, plate.Occupant{Kind: plate.OccupantSwap})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-02/dinner curry x2 cook=2026-11-02/dinner portions=2",
		"2026-11-03/dinner chili x2 cook=2026-11-03/dinner portions=2",
		"2026-11-05/dinner chili x3 cook=2026-11-05/dinner portions=3",
	}, e.week())
	v := e.session(sid)
	assert.Equal(t, day(tue), v.CookDate)
	assert.InDelta(t, 0.0, v.FrozenPortions, 1e-9)
	for _, w := range v.Warnings {
		assert.NotEqual(t, plate.WarnFrozenDeficit, w.Code)
	}
	var fresh *plate.AlsoChanged
	for i := range res.AlsoChanged {
		if res.AlsoChanged[i].What == plate.AlsoCooksFresh {
			fresh = &res.AlsoChanged[i]
		}
	}
	require.NotNil(t, fresh, "the moved meal cooks fresh: %v", whats(res.AlsoChanged))
	assert.Equal(t, monPlate.ID, fresh.PlateID)
	assert.Equal(t, "past_keep_days", fresh.Why)
	assert.Contains(t, whats(res.AlsoChanged), plate.AlsoBecameCooking)

	_, err = e.avail.Revert(e.ctx, res.Undo)
	require.NoError(t, err)
	assert.Equal(t, before, e.week())
}

func TestMoveMeal_MealClassifiedPastUseByStaysFrozenInItsBatch(t *testing.T) {
	e := newEnv(t)
	sid, _ := e.frozenBatch()

	res, err := e.move(e.plateAt(wed, e.dinner), thu, e.dinner, plate.Occupant{})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-02/dinner chili x2 cook=2026-11-02/dinner portions=4",
		"2026-11-05/dinner chili x2 cook=2026-11-02/dinner portions=4",
	}, e.week())
	assert.Empty(t, res.AlsoChanged)
	assert.InDelta(t, 4.0, e.session(sid).Portions, 1e-9)
}

func TestMoveMeal_SwapCookWithAFrozenMealOfTheSameBatchKeepsTheCookWhereTheFrozenMealIs(t *testing.T) {
	e := newEnv(t)
	sid, monPlate := e.frozenBatch()
	wedPlate := e.plateAt(wed, e.dinner)

	res, err := e.move(monPlate, wed, e.dinner, plate.Occupant{Kind: plate.OccupantSwap})

	require.NoError(t, err)
	got := e.week()
	require.Len(t, got, 2, "%v", got)
	assert.Contains(t, got[0], "2026-11-02/dinner chili x2 cook=2026-11-02/dinner", "the frozen meal stays in the batch, now its cook")
	assert.Contains(t, got[1], "2026-11-04/dinner chili x2 cook=2026-11-04/dinner", "the old cook is past the keeping time and cooks fresh")
	assert.Equal(t, 2.0, e.session(sid).Portions)
	var fresh *plate.AlsoChanged
	for i := range res.AlsoChanged {
		if res.AlsoChanged[i].What == plate.AlsoCooksFresh {
			fresh = &res.AlsoChanged[i]
		}
	}
	require.NotNil(t, fresh, "%v", whats(res.AlsoChanged))
	assert.Equal(t, monPlate.ID, fresh.PlateID)
	assert.Equal(t, "past_keep_days", fresh.Why)
	assert.NotEqual(t, wedPlate.ID, fresh.PlateID)
	assert.NotContains(t, whats(res.AlsoChanged), "before_cook")
}

func TestMoveMeal_DroppingTheCookingMealKeepsTheLaterMealInTheBatch(t *testing.T) {
	e := newEnv(t)
	sid, _ := e.frozenBatch()
	curry := e.create(tue, e.dinner, nil, composed(e.curry, 1))

	res, err := e.move(curry, mon, e.dinner, plate.Occupant{Kind: plate.OccupantDrop})

	require.NoError(t, err)
	assert.Equal(t, []string{
		"2026-11-02/dinner curry x2 cook=2026-11-02/dinner portions=2",
		"2026-11-04/dinner chili x2 cook=2026-11-02/dinner portions=2",
	}, e.week(), "the later meal stays in its batch; it does not leave or cook fresh")
	assert.NotContains(t, whats(res.AlsoChanged), plate.AlsoCooksFresh)
	assert.Equal(t, day(mon), e.session(sid).CookDate)
}

func TestMoveMeal_FrozenMealMovedPastFreezerDaysCooksFresh(t *testing.T) {
	e := newEnv(t)
	e.frozenBatch()
	e.set("plan.freezer_keep_days", "3")

	res, err := e.move(e.plateAt(wed, e.dinner), sat, e.dinner, plate.Occupant{})

	require.NoError(t, err)
	require.Contains(t, whats(res.AlsoChanged), plate.AlsoCooksFresh)
	for _, a := range res.AlsoChanged {
		if a.What == plate.AlsoCooksFresh {
			assert.Equal(t, "past_freezer_days", a.Why)
		}
	}
}

func TestMoveMeal_AnUnnamedPastKeepLinkIsUntouchedWhenTheCookMoves(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()
	_, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(wed, e.dinner)})
	require.NoError(t, err)
	e.set("plan.default_keep_days", "1") // Wed dinner is now past the keeping time of any cook before Tue
	wedLeftover := e.plateAt(wed, e.dinner)

	res, err := e.move(sunPlate, sat, e.lunch, plate.Occupant{})

	require.NoError(t, err)
	for _, a := range res.AlsoChanged {
		assert.NotEqual(t, wedLeftover.ID, a.PlateID, "the unnamed meal is not touched: %v", res.AlsoChanged)
	}
	wedPlate := e.plateAt(wed, e.dinner)
	require.NotNil(t, wedPlate.Components[0].CookSessionID)
	assert.Equal(t, sid, *wedPlate.Components[0].CookSessionID, "still in the batch")
	assert.Equal(t, day(mon), e.session(sid).CookDate, "the cook went to the earliest meal")
}

// Design rule 1a for the other operations: a link a meal operation creates
// never relies on the freezer silently, so with freezing on they fail with
// the keep-time error exactly as with freezing off.
func TestMoveCook_DoesNotFreezeSilently(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()
	e.freezableChili()
	before := e.week()

	_, err := e.svc.MoveCook(e.ctx, sid, cell("2026-10-31", e.dinner))

	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Contains(t, err.Error(), "past the keeping time")
	assert.Equal(t, before, e.week())
}

func TestReplaceComponent_FromASessionDoesNotFreezeSilently(t *testing.T) {
	e := newEnv(t)
	e.freezableChili()
	e.exec(`UPDATE foods SET freezable = 1 WHERE id = ?`, e.curry)
	curry := e.create(sun, e.lunch, nil, composed(e.curry, 1))
	csid := sessionOf(curry, 0)
	thu := e.create(thu, e.dinner, nil, composed(e.chili, 1))
	before := e.week()

	_, err := e.svc.ReplaceComponent(e.ctx, thu.ID, plate.Selector{Position: plate.PositionMain}, e.curry, plate.Source{SessionID: csid}, nil)

	require.ErrorIs(t, err, domain.ErrSessionPastKeepDays)
	assert.Equal(t, before, e.week())
}

func TestCopyMeal_AsLeftoverDoesNotFreezeSilently(t *testing.T) {
	e := newEnv(t)
	sunPlate, _ := e.batch()
	e.freezableChili()
	before := e.week()

	_, err := e.svc.CopyMeal(e.ctx, sunPlate.ID, []plate.CellKey{cell(thu, e.lunch)}, plate.CopyLeftover)

	require.ErrorIs(t, err, domain.ErrSessionPastKeepDays)
	assert.Equal(t, before, e.week())
}

// The no-silent-freezer rule only covers links a move would newly push into
// the freezer: a meal that was frozen before keeps relying on it.
func TestMoveCook_KeepsAnAlreadyFrozenLink(t *testing.T) {
	e := newEnv(t)
	sid, _ := e.frozenBatch()

	_, err := e.svc.MoveCook(e.ctx, sid, cell(mon, e.lunch))

	require.NoError(t, err)
	assert.Equal(t, e.lunch, e.session(sid).CookSlotID)
	assert.Equal(t, day(mon), e.session(sid).CookDate)
}
