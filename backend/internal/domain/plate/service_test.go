package plate_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// The service tests run against real SQLite: the write path's invariants
// (sessions, D11, the CHECK constraints, atomicity) live across the service,
// the repositories and the schema.
//
// Calendar: Sun 2026-11-01, Mon 11-02, Tue 11-03, Wed 11-04.
// Slots: lunch (sort 1), dinner (sort 2), breakfast (sort 0, created last so
// its id is the highest). Household portions 2. Default keep days 3.

const (
	sun = "2026-11-01"
	mon = "2026-11-02"
	tue = "2026-11-03"
	wed = "2026-11-04"
	thu = "2026-11-05"
)

type env struct {
	t                      *testing.T
	ctx                    context.Context
	db                     *sql.DB
	svc                    *plate.Service
	avail                  *availability.Service
	settings               *settings.Service
	lunch, dinner, brkfst  int64
	chili, curry, rice, eg int64
}

func newEnv(t *testing.T) *env {
	t.Helper()
	return newEnvWithEvents(t, nil)
}

// newEnvWithEvents is newEnv with the plate service's event log wrapped by
// wrap (nil: the sqlite repo as is).
func newEnvWithEvents(t *testing.T, wrap func(plate.EventLog) plate.EventLog) *env {
	t.Helper()
	db := testhelper.NewTestDB(t)
	var events plate.EventLog = sqlite.NewPlateEventRepo(db)
	if wrap != nil {
		events = wrap(events)
	}
	svc, avail := testsvc.PlanWithEvents(db, events)
	e := &env{t: t, ctx: context.Background(), db: db, svc: svc, avail: avail, settings: testsvc.Settings(db)}
	e.lunch = e.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.lunch', 'sun', 1, 1)`)
	e.dinner = e.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 2, 1)`)
	e.brkfst = e.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.breakfast', 'coffee', 0, 1)`)
	e.chili = e.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Chili', 'composed', 'main', 4)`)
	e.curry = e.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Curry', 'composed', 'main', 2)`)
	e.rice = e.insert(`INSERT INTO foods (name, kind, source) VALUES ('Rice', 'leaf', 'manual')`)
	e.eg = e.insert(`INSERT INTO foods (name, kind, source) VALUES ('Egg', 'leaf', 'manual')`)
	e.exec(`UPDATE user_profile SET household_portions = 2 WHERE id = 1`)
	return e
}

func (e *env) insert(q string, args ...any) int64 {
	e.t.Helper()
	res, err := e.db.Exec(q, args...)
	require.NoError(e.t, err, q)
	id, err := res.LastInsertId()
	require.NoError(e.t, err)
	return id
}

func (e *env) exec(q string, args ...any) {
	e.t.Helper()
	_, err := e.db.Exec(q, args...)
	require.NoError(e.t, err, q)
}

func (e *env) count(table string) int {
	e.t.Helper()
	var n int
	require.NoError(e.t, e.db.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n))
	return n
}

func (e *env) set(key, value string) {
	e.t.Helper()
	require.NoError(e.t, e.settings.Set(e.ctx, key, value))
}

func f64(v float64) *float64 { return &v }
func i64(v int64) *int64     { return &v }

func composed(foodID int64, portions float64) plate.PlateComponent {
	return plate.PlateComponent{FoodID: foodID, Portions: &portions}
}

func linked(foodID int64, portions float64, sessionID int64) plate.PlateComponent {
	return plate.PlateComponent{FoodID: foodID, Portions: &portions, CookSessionID: &sessionID}
}

func grams(foodID int64, g float64) plate.PlateComponent {
	unit := "g"
	return plate.PlateComponent{FoodID: foodID, Amount: &g, Unit: &unit}
}

func (e *env) create(date string, slotID int64, eaters *float64, comps ...plate.PlateComponent) *plate.Plate {
	e.t.Helper()
	p, err := e.svc.Create(e.ctx, plate.PlateInput{Date: day(date), SlotID: slotID, Eaters: eaters, Components: comps})
	require.NoError(e.t, err)
	return p
}

func (e *env) session(id int64) *plate.SessionView {
	e.t.Helper()
	v, err := e.svc.Session(e.ctx, id)
	require.NoError(e.t, err)
	return v
}

func sessionOf(p *plate.Plate, i int) int64 { return *p.Components[i].CookSessionID }

// batch builds the AC-1 Sunday batch: Chili at Sun dinner (eaters 2,
// portions 1) plus leftovers Mon lunch and Mon dinner: an exact session of 6.
func (e *env) batch() (sunPlate *plate.Plate, sessionID int64) {
	e.t.Helper()
	sunPlate = e.create(sun, e.dinner, nil, composed(e.chili, 1))
	sessionID = sessionOf(sunPlate, 0)
	_, err := e.svc.AddLeftovers(e.ctx, sessionID, []plate.CellKey{cell(mon, lunch), cell(mon, dinner)})
	require.NoError(e.t, err)
	return sunPlate, sessionID
}

func (e *env) plateAt(date string, slotID int64) *plate.Plate {
	e.t.Helper()
	plates, err := e.svc.Day(e.ctx, day(date))
	require.NoError(e.t, err)
	for i := range plates {
		if plates[i].SlotID == slotID {
			return &plates[i]
		}
	}
	return nil
}

// ── Create, eaters and fresh sessions ───────────────────────────────────

// B1-AC1
func TestCreate_ComposedGetsFreshSessionScaledByEaters(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, f64(2), composed(e.chili, 1))

	require.Len(t, p.Components, 1)
	require.NotNil(t, p.Components[0].CookSessionID)
	assert.Equal(t, 1, e.count("cook_sessions"))
	v := e.session(sessionOf(p, 0))
	assert.InDelta(t, 2.0, v.Portions, 1e-9)
	assert.Equal(t, e.chili, v.FoodID)
	assert.Equal(t, day(sun), v.CookDate)
	assert.Equal(t, e.dinner, v.CookSlotID)
	assert.Equal(t, "Chili", v.FoodName)
	require.Len(t, v.Links, 1)
	assert.Equal(t, p.ID, v.Links[0].PlateID)
	assert.Equal(t, plate.KindFresh, v.Links[0].Kind)
}

func TestCookSession_ReturnsPlainSession(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, f64(2), composed(e.chili, 1))

	got, err := e.svc.CookSession(e.ctx, sessionOf(p, 0))
	require.NoError(t, err)
	assert.Equal(t, sessionOf(p, 0), got.ID)
	assert.Equal(t, e.chili, got.FoodID)
	assert.InDelta(t, 2.0, got.Portions, 1e-9)

	_, err = e.svc.CookSession(e.ctx, 999999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestCreate_EatersDefaultToHousehold(t *testing.T) {
	e := newEnv(t)
	e.exec(`UPDATE user_profile SET household_portions = 3.5 WHERE id = 1`)
	p := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	assert.InDelta(t, 3.5, p.Eaters, 1e-9)
	assert.InDelta(t, 3.5, e.session(sessionOf(p, 0)).Portions, 1e-9)
}

func TestCreate_ComposedPortionsDefaultToOne(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, nil, plate.PlateComponent{FoodID: e.chili})
	require.NotNil(t, p.Components[0].Portions)
	assert.InDelta(t, 1.0, *p.Components[0].Portions, 1e-9)
}

func TestCreate_LeafGetsNoSession(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, nil, grams(e.rice, 150))
	assert.Nil(t, p.Components[0].CookSessionID)
	assert.InDelta(t, 150.0, *p.Components[0].Grams, 1e-9)
	assert.Equal(t, 0, e.count("cook_sessions"))
	assert.Equal(t, "Rice", p.Components[0].FoodName)
}

func TestCreate_Validation(t *testing.T) {
	tests := []struct {
		name string
		in   func(e *env) plate.PlateInput
		want error
	}{
		{"zero eaters", func(e *env) plate.PlateInput {
			return plate.PlateInput{Date: day(sun), SlotID: e.dinner, Eaters: f64(0)}
		}, domain.ErrInvalidInput},
		{"too many eaters", func(e *env) plate.PlateInput {
			return plate.PlateInput{Date: day(sun), SlotID: e.dinner, Eaters: f64(20.5)}
		}, domain.ErrInvalidInput},
		{"unknown slot", func(e *env) plate.PlateInput {
			return plate.PlateInput{Date: day(sun), SlotID: 999}
		}, domain.ErrSlotUnknown},
		{"missing date", func(e *env) plate.PlateInput {
			return plate.PlateInput{SlotID: e.dinner}
		}, domain.ErrInvalidInput},
		{"leaf with a session", func(e *env) plate.PlateInput {
			c := grams(e.rice, 10)
			c.CookSessionID = i64(1)
			return plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{c}}
		}, domain.ErrInvalidInput},
		{"composed with amount", func(e *env) plate.PlateInput {
			return plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{grams(e.chili, 10)}}
		}, plate.ErrInvalidQuantityForComposed},
		{"leaf with portions", func(e *env) plate.PlateInput {
			return plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.rice, 1)}}
		}, plate.ErrInvalidQuantityForLeaf},
		{"leaf without quantity", func(e *env) plate.PlateInput {
			return plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{{FoodID: e.rice}}}
		}, plate.ErrInvalidQuantityShape},
		{"leaf count unit without portion", func(e *env) plate.PlateInput {
			unit := "slice"
			return plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{{FoodID: e.rice, Amount: f64(1), Unit: &unit}}}
		}, plate.ErrUnitRequiresPortion},
		{"negative portions", func(e *env) plate.PlateInput {
			return plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, -1)}}
		}, domain.ErrInvalidInput},
		{"unknown food", func(e *env) plate.PlateInput {
			return plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{composed(999, 1)}}
		}, domain.ErrNotFound},
		{"unknown session", func(e *env) plate.PlateInput {
			return plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{linked(e.chili, 1, 999)}}
		}, domain.ErrSessionNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			_, err := e.svc.Create(e.ctx, tt.in(e))
			assert.True(t, errors.Is(err, tt.want), "got %v", err)
			assert.Equal(t, 0, e.count("plates"), "nothing written")
			assert.Equal(t, 0, e.count("cook_sessions"), "nothing written")
		})
	}
}

func TestCreate_CellRules(t *testing.T) {
	e := newEnv(t)
	e.create(sun, e.dinner, nil, grams(e.rice, 100))
	_, err := e.svc.Create(e.ctx, plate.PlateInput{Date: day(sun), SlotID: e.dinner})
	assert.ErrorIs(t, err, domain.ErrCellOccupied)

	_, _, err = e.avail.SetOverride(e.ctx, availability.OverrideInput{Date: day(mon), SlotID: e.dinner, Mode: availability.ModeNone})
	require.NoError(t, err)
	_, err = e.svc.Create(e.ctx, plate.PlateInput{Date: day(mon), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
	assert.ErrorIs(t, err, domain.ErrCellUnavailable)
	assert.Equal(t, 0, e.count("cook_sessions"), "the failed create left no session")
}

// ── D11 on eaters and quantities ────────────────────────────────────────

// B1-AC2
func TestUpdate_EatersChangeFollowsExactSession(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, f64(2), composed(e.chili, 1))
	got, err := e.svc.Update(e.ctx, p.ID, plate.PlatePatch{Eaters: f64(3)})
	require.NoError(t, err)
	assert.InDelta(t, 3.0, got.Eaters, 1e-9)
	assert.InDelta(t, 3.0, e.session(sessionOf(p, 0)).Portions, 1e-9)
}

// B1-AC3
func TestUpdate_EatersChangeKeepsExplicitPortions(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, f64(2), composed(e.chili, 1))
	sid := sessionOf(p, 0)
	_, err := e.svc.SetSessionPortions(e.ctx, sid, plate.SessionPortions{Portions: 5})
	require.NoError(t, err)
	_, err = e.svc.Update(e.ctx, p.ID, plate.PlatePatch{Eaters: f64(3)})
	require.NoError(t, err)
	assert.InDelta(t, 5.0, e.session(sid).Portions, 1e-9)
}

func TestUpdate_NoteAndValidation(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, nil, grams(e.rice, 100))
	note := "late"
	got, err := e.svc.Update(e.ctx, p.ID, plate.PlatePatch{Note: &note, SetNote: true})
	require.NoError(t, err)
	require.NotNil(t, got.Note)
	assert.Equal(t, "late", *got.Note)
	got, err = e.svc.Update(e.ctx, p.ID, plate.PlatePatch{SetNote: true})
	require.NoError(t, err)
	assert.Nil(t, got.Note, "SetNote with nil clears")

	_, err = e.svc.Update(e.ctx, p.ID, plate.PlatePatch{Eaters: f64(0)})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = e.svc.Update(e.ctx, p.ID, plate.PlatePatch{SlotID: i64(999)})
	assert.ErrorIs(t, err, domain.ErrSlotUnknown)
	_, err = e.svc.Update(e.ctx, 999, plate.PlatePatch{Eaters: f64(1)})
	assert.ErrorIs(t, err, domain.ErrNotFound)

	other := e.create(mon, e.dinner, nil, grams(e.rice, 100))
	_, err = e.svc.Update(e.ctx, other.ID, plate.PlatePatch{Date: ptrTime(day(sun))})
	assert.ErrorIs(t, err, domain.ErrCellOccupied)
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestUpdateComponentQuantity_FollowsD11(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, f64(2), composed(e.chili, 1))
	got, err := e.svc.UpdateComponentQuantity(e.ctx, p.Components[0].ID, plate.PlateComponent{Portions: f64(1.5)})
	require.NoError(t, err)
	assert.InDelta(t, 1.5, *got.Components[0].Portions, 1e-9)
	assert.Equal(t, sessionOf(p, 0), sessionOf(got, 0), "the link is kept")
	assert.InDelta(t, 3.0, e.session(sessionOf(p, 0)).Portions, 1e-9)
}

func TestUpdateComponentQuantity_LeafReresolvesGrams(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, nil, grams(e.rice, 100))
	unit := "kg"
	got, err := e.svc.UpdateComponentQuantity(e.ctx, p.Components[0].ID, plate.PlateComponent{Amount: f64(0.2), Unit: &unit})
	require.NoError(t, err)
	assert.InDelta(t, 200.0, *got.Components[0].Grams, 1e-9)
}

// ── Leftovers ───────────────────────────────────────────────────────────

// B1-AC4
func TestAddLeftovers_GrowsExactBatch(t *testing.T) {
	e := newEnv(t)
	sunPlate := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	sid := sessionOf(sunPlate, 0)
	require.InDelta(t, 2.0, e.session(sid).Portions, 1e-9)

	v, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{
		{Date: day(mon), SlotID: e.dinner}, {Date: day(mon), SlotID: e.lunch},
	})
	require.NoError(t, err)
	assert.InDelta(t, 6.0, v.Portions, 1e-9)
	assert.Equal(t, 3, v.Meals)
	require.Len(t, v.Links, 3)
	assert.Equal(t, []int{1, 2, 3}, []int{v.Links[0].Position, v.Links[1].Position, v.Links[2].Position})
	assert.Equal(t, []plate.LinkKind{plate.KindFresh, plate.KindLeftover, plate.KindLeftover},
		[]plate.LinkKind{v.Links[0].Kind, v.Links[1].Kind, v.Links[2].Kind})
	assert.Equal(t, sunPlate.ID, v.Links[0].PlateID)
	assert.Equal(t, plate.CellKey{Date: day(mon), SlotID: e.lunch}, v.Links[1].Cell, "cell order: Mon lunch before Mon dinner")
	assert.Equal(t, plate.CellKey{Date: day(mon), SlotID: e.dinner}, v.Links[2].Cell)
	assert.InDelta(t, 2.0, v.Links[1].Portions, 1e-9, "household eaters × the fresh link's per-eater portions")
	assert.Empty(t, v.Warnings)

	monLunch := e.plateAt(mon, e.lunch)
	require.NotNil(t, monLunch)
	assert.InDelta(t, 2.0, monLunch.Eaters, 1e-9)
	assert.Equal(t, sid, sessionOf(monLunch, 0))
}

// AddLeftovers writes all cells or none.
func TestAddLeftovers_IsAtomic(t *testing.T) {
	e := newEnv(t)
	sunPlate := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	sid := sessionOf(sunPlate, 0)
	e.create(mon, e.dinner, nil, grams(e.rice, 100))

	_, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(mon, lunch), cell(mon, dinner)})
	assert.ErrorIs(t, err, domain.ErrCellOccupied)
	assert.Equal(t, 2, e.count("plates"), "Mon lunch was rolled back")
	assert.InDelta(t, 2.0, e.session(sid).Portions, 1e-9)
}

func TestAddLeftovers_Validation(t *testing.T) {
	e := newEnv(t)
	sunPlate := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	sid := sessionOf(sunPlate, 0)
	_, err := e.svc.AddLeftovers(e.ctx, sid, nil)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = e.svc.AddLeftovers(e.ctx, 999, []plate.CellKey{cell(mon, lunch)})
	assert.ErrorIs(t, err, domain.ErrNotFound)
	_, err = e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(sun, lunch)})
	assert.ErrorIs(t, err, domain.ErrSessionBeforeCook)
}

// B1-AC10
func TestLeftoverCells_SkipsNoneOccupiedAndPastUseBy(t *testing.T) {
	e := newEnv(t)
	e.exec(`UPDATE foods SET keep_days = 2 WHERE id = ?`, e.chili) // use-by Tue
	sunPlate := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	sid := sessionOf(sunPlate, 0)
	_, _, err := e.avail.SetOverride(e.ctx, availability.OverrideInput{Date: day(mon), SlotID: e.brkfst, Mode: availability.ModeNone})
	require.NoError(t, err)
	e.create(mon, e.lunch, nil, grams(e.rice, 100))

	cells, err := e.svc.LeftoverCells(e.ctx, sid, 3)
	require.NoError(t, err)
	assert.Equal(t, []plate.CellKey{
		{Date: day(mon), SlotID: e.dinner},
		{Date: day(tue), SlotID: e.brkfst}, // sort order 0 beats its higher id
		{Date: day(tue), SlotID: e.lunch},
	}, cells)

	cells, err = e.svc.LeftoverCells(e.ctx, sid, 14)
	require.NoError(t, err)
	require.Len(t, cells, 4, "nothing after use-by (Tue)")
	assert.Equal(t, plate.CellKey{Date: day(tue), SlotID: e.dinner}, cells[3])

	_, err = e.svc.LeftoverCells(e.ctx, sid, 0)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = e.svc.LeftoverCells(e.ctx, sid, plate.MaxLeftoverCells+1)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = e.svc.LeftoverCells(e.ctx, 999, 1)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// ── Hard link rules ─────────────────────────────────────────────────────

// B1-AC5
func TestLink_HardRules(t *testing.T) {
	e := newEnv(t)
	sunPlate := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	sid := sessionOf(sunPlate, 0)
	sunLunch := e.create(sun, e.lunch, nil, grams(e.rice, 50))
	thuLunch := e.create(thu, e.lunch, nil, grams(e.rice, 50))
	monLunch := e.create(mon, e.lunch, nil, grams(e.rice, 50))

	_, err := e.svc.AddComponent(e.ctx, sunLunch.ID, linked(e.chili, 1, sid))
	assert.ErrorIs(t, err, domain.ErrSessionBeforeCook)
	_, err = e.svc.AddComponent(e.ctx, thuLunch.ID, linked(e.chili, 1, sid))
	assert.ErrorIs(t, err, domain.ErrSessionPastKeepDays)
	_, err = e.svc.AddComponent(e.ctx, monLunch.ID, linked(e.curry, 1, sid))
	assert.ErrorIs(t, err, domain.ErrSessionFoodMismatch)
	_, err = e.svc.Create(e.ctx, plate.PlateInput{Date: day(sun), SlotID: e.brkfst, Components: []plate.PlateComponent{linked(e.chili, 1, sid)}})
	assert.ErrorIs(t, err, domain.ErrSessionBeforeCook, "create links through the same rules")
	assert.InDelta(t, 2.0, e.session(sid).Portions, 1e-9, "failed links change nothing")

	got, err := e.svc.AddComponent(e.ctx, monLunch.ID, linked(e.chili, 1, sid))
	require.NoError(t, err)
	assert.Equal(t, sid, sessionOf(got, 1))
	assert.InDelta(t, 4.0, e.session(sid).Portions, 1e-9)
}

func TestLink_FreezingAllowsPastUseByUpToFreezerDays(t *testing.T) {
	e := newEnv(t)
	e.set(settings.KeyPlanFreezingEnabled, "true")
	e.set(settings.KeyPlanFreezerKeepDays, "10")
	sid := sessionOf(e.create(sun, e.dinner, nil, composed(e.chili, 1)), 0)

	v, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(thu, lunch)})
	require.NoError(t, err)
	assert.Equal(t, plate.KindFrozen, v.Links[1].Kind)
	assert.InDelta(t, 2.0, v.FrozenConsumed, 1e-9)

	_, err = e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{{Date: day("2026-11-12"), SlotID: e.lunch}})
	assert.ErrorIs(t, err, domain.ErrSessionPastFreezerDays)
}

// ── Moves ───────────────────────────────────────────────────────────────

// B1-AC6
func TestUpdate_MoveCarriesCookCellAndLeavesWarnings(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()

	got, err := e.svc.Update(e.ctx, sunPlate.ID, plate.PlatePatch{Date: ptrTime(day(tue))})
	require.NoError(t, err, "moves never fail on batch rules")
	assert.Equal(t, day(tue), got.Date)

	v := e.session(sid)
	assert.Equal(t, day(tue), v.CookDate)
	assert.Equal(t, e.dinner, v.CookSlotID)
	monLunch, monDinner := e.plateAt(mon, e.lunch), e.plateAt(mon, e.dinner)
	var before []int64
	for _, w := range v.Warnings {
		if w.Code == plate.WarnLeftoverBeforeCook {
			before = append(before, *w.PlateID)
		}
	}
	assert.ElementsMatch(t, []int64{monLunch.ID, monDinner.ID}, before)
}

// Moving a leftover plate does not touch the session's cook cell.
func TestUpdate_MovingALeftoverKeepsCookCell(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()
	monLunch := e.plateAt(mon, e.lunch)
	_, err := e.svc.Update(e.ctx, monLunch.ID, plate.PlatePatch{Date: ptrTime(day(tue))})
	require.NoError(t, err)
	v := e.session(sid)
	assert.Equal(t, day(sun), v.CookDate)
	assert.InDelta(t, 6.0, v.Portions, 1e-9)
}

// ── Removal ─────────────────────────────────────────────────────────────

// B1-AC7
func TestRemoveComponent_KeepsSessionWhileLinkedThenDeletesIt(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()

	got, err := e.svc.RemoveComponent(e.ctx, sunPlate.Components[0].ID)
	require.NoError(t, err)
	assert.Empty(t, got.Components, "the plate stays, empty")
	v := e.session(sid)
	assert.Equal(t, 2, v.Meals, "two leftovers remain")
	assert.InDelta(t, 4.0, v.Portions, 1e-9, "exact batch follows D11")

	for _, p := range []*plate.Plate{e.plateAt(mon, e.lunch), e.plateAt(mon, e.dinner)} {
		_, err := e.svc.RemoveComponent(e.ctx, p.Components[0].ID)
		require.NoError(t, err)
	}
	_, err = e.svc.Session(e.ctx, sid)
	assert.ErrorIs(t, err, domain.ErrNotFound, "the last link took the session with it")
	assert.Equal(t, 0, e.count("cook_sessions"))
}

func TestSwapComponent(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()
	monLunch := e.plateAt(mon, e.lunch)

	got, err := e.svc.SwapComponent(e.ctx, monLunch.Components[0].ID, e.curry, nil)
	require.NoError(t, err)
	newSID := sessionOf(got, 0)
	assert.NotEqual(t, sid, newSID)
	fresh := e.session(newSID)
	assert.Equal(t, e.curry, fresh.FoodID)
	assert.Equal(t, day(mon), fresh.CookDate)
	assert.InDelta(t, 2.0, fresh.Portions, 1e-9)
	assert.InDelta(t, 4.0, e.session(sid).Portions, 1e-9, "the batch lost a link and follows D11")

	// Swapping the only link of a session away deletes it.
	got, err = e.svc.SwapComponent(e.ctx, got.Components[0].ID, e.rice, &plate.PlateComponent{Amount: f64(100), Unit: ptrStr("g")})
	require.NoError(t, err)
	assert.Nil(t, got.Components[0].CookSessionID)
	_, err = e.svc.Session(e.ctx, newSID)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	// A composed→leaf swap without a quantity keeps portions and fails.
	_, err = e.svc.SwapComponent(e.ctx, e.plateAt(mon, e.dinner).Components[0].ID, e.rice, nil)
	assert.ErrorIs(t, err, plate.ErrInvalidQuantityForLeaf)
}

func ptrStr(s string) *string { return &s }

// B1-AC14
func TestDelete_LeftoverPlateRestoresToSameSession(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()
	monLunch := e.plateAt(mon, e.lunch)

	snap, err := e.svc.Delete(e.ctx, monLunch.ID)
	require.NoError(t, err)
	assert.InDelta(t, 4.0, e.session(sid).Portions, 1e-9)
	require.Len(t, snap.Plates, 1)
	assert.Empty(t, snap.Sessions, "the session survived")
	require.NotNil(t, snap.Plates[0].Components[0].Session)
	assert.Equal(t, sid, snap.Plates[0].Components[0].Session.ID)

	res, err := e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	require.NoError(t, err)
	require.Len(t, res.PlateIDs, 1)
	restored := e.plateAt(mon, e.lunch)
	assert.Equal(t, sid, sessionOf(restored, 0), "re-linked to the same session id")
	v := e.session(sid)
	assert.InDelta(t, 6.0, v.Portions, 1e-9)
	assert.Equal(t, 3, v.Meals)
}

// B1-AC8
func TestDeleteSession_RemovesLinksAndEmptyPlates(t *testing.T) {
	for _, withSide := range []bool{true, false} {
		t.Run(fmt.Sprintf("sun plate with side=%v", withSide), func(t *testing.T) {
			e := newEnv(t)
			sunPlate, sid := e.batch()
			if withSide {
				_, err := e.svc.AddComponent(e.ctx, sunPlate.ID, grams(e.rice, 120))
				require.NoError(t, err)
			}
			_, err := e.svc.SetSessionPortions(e.ctx, sid, plate.SessionPortions{Portions: 7, Note: ptrStr("double garlic")})
			require.NoError(t, err)
			_, err = e.svc.SetCooked(e.ctx, sid, true)
			require.NoError(t, err)
			before := e.plan(sun, wed)

			snap, err := e.svc.DeleteSession(e.ctx, sid)
			require.NoError(t, err)
			assert.Equal(t, 0, e.count("cook_sessions"))
			assert.Nil(t, e.plateAt(mon, e.lunch))
			assert.Nil(t, e.plateAt(mon, e.dinner))
			kept := e.plateAt(sun, e.dinner)
			if withSide {
				require.NotNil(t, kept, "the Sun plate keeps its side")
				require.Len(t, kept.Components, 1)
				assert.Equal(t, e.rice, kept.Components[0].FoodID)
			} else {
				assert.Nil(t, kept, "the Sun plate became empty")
			}
			assert.Equal(t, 0, e.componentCount(e.chili), "all 3 components are gone")
			require.Len(t, snap.Sessions, 1)
			assert.InDelta(t, 7.0, snap.Sessions[0].Portions, 1e-9)

			_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
			require.NoError(t, err)
			assert.Equal(t, before, e.plan(sun, wed), "snapshot fidelity")
		})
	}
}

func (e *env) componentCount(foodID int64) int {
	e.t.Helper()
	var n int
	require.NoError(e.t, e.db.QueryRow(`SELECT COUNT(*) FROM plate_components WHERE food_id = ?`, foodID).Scan(&n))
	return n
}

// Snapshot fidelity: Delete then Apply recreates an equal plan.
func TestDelete_ThenApplyRecreatesAnEqualPlan(t *testing.T) {
	e := newEnv(t)
	e.set(settings.KeyPlanFreezingEnabled, "true")
	note := "birthday"
	p, err := e.svc.Create(e.ctx, plate.PlateInput{
		Date: day(sun), SlotID: e.dinner, Eaters: f64(3.5), Note: &note,
		Components: []plate.PlateComponent{grams(e.rice, 80), composed(e.chili, 1.5), composed(e.curry, 0.5)},
	})
	require.NoError(t, err)
	sid := sessionOf(p, 1)
	_, err = e.svc.SetSessionPortions(e.ctx, sid, plate.SessionPortions{Portions: 8, FrozenPortions: 2, Note: ptrStr("freeze two")})
	require.NoError(t, err)
	_, err = e.svc.SetCooked(e.ctx, sid, true)
	require.NoError(t, err)
	e.exec(`INSERT INTO plate_feedback (plate_id, status, note, rated_at) VALUES (?, 'loved', 'great', '2026-11-02 08:00:00')`, p.ID)
	before := e.plan(sun, sun)

	snap, err := e.svc.Delete(e.ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, e.count("plates"))
	assert.Equal(t, 0, e.count("cook_sessions"), "both sessions were orphaned")
	assert.Equal(t, 0, e.count("plate_feedback"))
	require.Len(t, snap.Sessions, 2)
	for _, c := range snap.Plates[0].Components {
		if c.Portions != nil {
			require.NotNil(t, c.Session)
			assert.NotEmpty(t, c.Session.Ref, "a session deleted with the plate is recorded by ref")
		}
	}

	_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	require.NoError(t, err)
	assert.Equal(t, before, e.plan(sun, sun))
}

// ── Clear range and apply ───────────────────────────────────────────────

func TestClearRange_RemovesPlatesAndOrphanedSessions(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch() // Sun dinner + Mon lunch + Mon dinner
	e.create(tue, e.lunch, nil, composed(e.curry, 1))
	before := e.plan(sun, wed)

	snap, err := e.svc.ClearRange(e.ctx, day(sun), day(sun))
	require.NoError(t, err)
	assert.Nil(t, e.plateAt(sun, e.dinner))
	v := e.session(sid)
	assert.InDelta(t, 4.0, v.Portions, 1e-9, "the batch keeps its leftovers outside the range")
	require.Len(t, snap.Plates, 1)
	assert.Equal(t, sid, snap.Plates[0].Components[0].Session.ID)
	_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	require.NoError(t, err)
	assert.Equal(t, before, e.plan(sun, wed))

	snap, err = e.svc.ClearRange(e.ctx, day(sun), day(wed))
	require.NoError(t, err)
	assert.Equal(t, 0, e.count("plates"))
	assert.Equal(t, 0, e.count("cook_sessions"))
	assert.Len(t, snap.Plates, 4)
	assert.Len(t, snap.Sessions, 2)
	_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	require.NoError(t, err)
	assert.Equal(t, before, e.plan(sun, wed))

	_, err = e.svc.ClearRange(e.ctx, day(wed), day(sun))
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

// B1-AC11
func TestApply_FailOnOccupiedCellWritesNothing(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()
	snap, err := e.svc.ClearRange(e.ctx, day(sun), day(mon))
	require.NoError(t, err)
	require.Len(t, snap.Plates, 3)
	e.create(mon, e.dinner, nil, grams(e.egg(), 50)) // the last cell gets taken

	_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	assert.ErrorIs(t, err, domain.ErrCellOccupied)
	assert.Equal(t, 1, e.count("plates"), "the first two plates were rolled back")
	assert.Equal(t, 0, e.count("cook_sessions"), "the recreated session was rolled back")
	_, err = e.svc.Session(e.ctx, sid)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func (e *env) egg() int64 { return e.eg }

func TestApply_FailOnNoneCell(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, nil, grams(e.rice, 50))
	snap, err := e.svc.Delete(e.ctx, p.ID)
	require.NoError(t, err)
	_, _, err = e.avail.SetOverride(e.ctx, availability.OverrideInput{Date: day(sun), SlotID: e.dinner, Mode: availability.ModeNone})
	require.NoError(t, err)

	_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	assert.ErrorIs(t, err, domain.ErrCellUnavailable)
	res, err := e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictSkip})
	require.NoError(t, err)
	assert.Equal(t, []plate.CellKey{{Date: day(sun), SlotID: e.dinner}}, res.SkippedUnavailable)
	assert.Equal(t, 0, e.count("plates"))
}

func TestApply_SkipAndOverwrite(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	snap, err := e.svc.Delete(e.ctx, p.ID)
	require.NoError(t, err)
	e.create(sun, e.dinner, nil, composed(e.curry, 1))

	res, err := e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictSkip})
	require.NoError(t, err)
	assert.Equal(t, []plate.CellKey{{Date: day(sun), SlotID: e.dinner}}, res.SkippedOccupied)
	assert.Empty(t, res.PlateIDs)
	assert.Equal(t, 1, e.count("cook_sessions"), "a skipped plate creates no session")

	res, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictOverwrite})
	require.NoError(t, err)
	require.Len(t, res.PlateIDs, 1)
	require.Len(t, res.Removed.Plates, 1)
	assert.Equal(t, e.curry, res.Removed.Plates[0].Components[0].FoodID)
	got := e.plateAt(sun, e.dinner)
	assert.Equal(t, e.chili, got.Components[0].FoodID)
	assert.Len(t, got.Components, 1)
	assert.Equal(t, 1, e.count("cook_sessions"), "the curry session went with the overwritten plate")
}

func TestApply_UnknownSessionIDWritesNothing(t *testing.T) {
	e := newEnv(t)
	snap := plate.PlanSnapshot{Plates: []plate.SnapshotPlate{
		{Date: tue, SlotID: e.lunch, Eaters: 2, Components: []plate.SnapshotComponent{{FoodID: e.rice, Amount: f64(10), Unit: ptrStr("g")}}},
		{Date: mon, SlotID: e.lunch, Eaters: 2, Components: []plate.SnapshotComponent{{FoodID: e.chili, Portions: f64(1), Session: &plate.SessionLink{ID: 999}}}},
	}}
	_, err := e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	assert.ErrorIs(t, err, domain.ErrSessionNotFound)
	assert.Equal(t, 0, e.count("plates"))
}

func TestApply_ComposedWithoutLinkGetsFreshSession(t *testing.T) {
	e := newEnv(t)
	snap := plate.PlanSnapshot{Plates: []plate.SnapshotPlate{
		{Date: mon, SlotID: e.lunch, Components: []plate.SnapshotComponent{{FoodID: e.chili, Portions: f64(1)}}},
	}}
	res, err := e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	require.NoError(t, err)
	p, err := e.svc.Get(e.ctx, res.PlateIDs[0])
	require.NoError(t, err)
	assert.InDelta(t, 2.0, p.Eaters, 1e-9, "missing eaters default to the household")
	assert.InDelta(t, 2.0, e.session(sessionOf(p, 0)).Portions, 1e-9)
}

func TestApply_InvalidSnapshots(t *testing.T) {
	e := newEnv(t)
	tests := []struct {
		name string
		snap plate.PlanSnapshot
		opts plate.ApplyOptions
	}{
		{"bad mode", plate.PlanSnapshot{}, plate.ApplyOptions{OnConflict: "merge"}},
		{"bad date", plate.PlanSnapshot{Plates: []plate.SnapshotPlate{{Date: "11/02", SlotID: e.lunch}}}, plate.ApplyOptions{OnConflict: plate.ConflictFail}},
		{"unknown ref", plate.PlanSnapshot{Plates: []plate.SnapshotPlate{{Date: mon, SlotID: e.lunch, Components: []plate.SnapshotComponent{
			{FoodID: e.chili, Portions: f64(1), Session: &plate.SessionLink{Ref: "s9"}},
		}}}}, plate.ApplyOptions{OnConflict: plate.ConflictFail}},
		{"ref and id", plate.PlanSnapshot{Plates: []plate.SnapshotPlate{{Date: mon, SlotID: e.lunch, Components: []plate.SnapshotComponent{
			{FoodID: e.chili, Portions: f64(1), Session: &plate.SessionLink{Ref: "s1", ID: 3}},
		}}}, Sessions: []plate.SnapshotSession{{Ref: "s1", FoodID: e.chili, CookDate: mon, CookSlotID: e.lunch, Portions: 1}}}, plate.ApplyOptions{OnConflict: plate.ConflictFail}},
		{"duplicate ref", plate.PlanSnapshot{Sessions: []plate.SnapshotSession{
			{Ref: "s1", FoodID: e.chili, CookDate: mon, CookSlotID: e.lunch, Portions: 1},
			{Ref: "s1", FoodID: e.chili, CookDate: mon, CookSlotID: e.lunch, Portions: 1},
		}}, plate.ApplyOptions{OnConflict: plate.ConflictFail}},
		{"frozen above portions", plate.PlanSnapshot{Sessions: []plate.SnapshotSession{
			{Ref: "s1", FoodID: e.chili, CookDate: mon, CookSlotID: e.lunch, Portions: 1, FrozenPortions: 2},
		}}, plate.ApplyOptions{OnConflict: plate.ConflictFail}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := e.svc.Apply(e.ctx, tt.snap, tt.opts)
			assert.ErrorIs(t, err, domain.ErrInvalidInput)
		})
	}
	snap := plate.PlanSnapshot{
		Plates: []plate.SnapshotPlate{{Date: mon, SlotID: e.lunch, Components: []plate.SnapshotComponent{
			{FoodID: e.curry, Portions: f64(1), Session: &plate.SessionLink{Ref: "s1"}},
		}}},
		Sessions: []plate.SnapshotSession{{Ref: "s1", FoodID: e.chili, CookDate: mon, CookSlotID: e.lunch, Portions: 2}},
	}
	_, err := e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	assert.ErrorIs(t, err, domain.ErrSessionFoodMismatch)
	assert.Equal(t, 0, e.count("cook_sessions"))
}

// ── Session edits ───────────────────────────────────────────────────────

// B1-AC9
func TestSetSessionPortions_FreezerRules(t *testing.T) {
	e := newEnv(t)
	sid := sessionOf(e.create(sun, e.dinner, nil, composed(e.chili, 1)), 0)

	_, err := e.svc.SetSessionPortions(e.ctx, sid, plate.SessionPortions{Portions: 6, FrozenPortions: 2})
	assert.ErrorIs(t, err, domain.ErrSessionFreezingDisabled)

	e.set(settings.KeyPlanFreezingEnabled, "true")
	e.exec(`UPDATE foods SET freezable = 0 WHERE id = ?`, e.chili)
	_, err = e.svc.SetSessionPortions(e.ctx, sid, plate.SessionPortions{Portions: 6, FrozenPortions: 2})
	assert.ErrorIs(t, err, domain.ErrSessionNotFreezable)

	e.exec(`UPDATE foods SET freezable = NULL WHERE id = ?`, e.chili)
	v, err := e.svc.SetSessionPortions(e.ctx, sid, plate.SessionPortions{Portions: 6, FrozenPortions: 2})
	require.NoError(t, err, "unknown freezable allows freezing")
	assert.InDelta(t, 2.0, v.FrozenPortions, 1e-9)
	assert.InDelta(t, 4.0, v.Surplus, 1e-9)
}

func TestSetSessionPortions_Validation(t *testing.T) {
	e := newEnv(t)
	sid := sessionOf(e.create(sun, e.dinner, nil, composed(e.chili, 1)), 0)
	for name, in := range map[string]plate.SessionPortions{
		"zero portions":         {Portions: 0},
		"negative frozen":       {Portions: 2, FrozenPortions: -1},
		"frozen above portions": {Portions: 2, FrozenPortions: 3},
		"note too long":         {Portions: 2, Note: ptrStr(strings.Repeat("x", plate.MaxSessionNoteLen+1))},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := e.svc.SetSessionPortions(e.ctx, sid, in)
			assert.ErrorIs(t, err, domain.ErrInvalidInput)
		})
	}
	_, err := e.svc.SetSessionPortions(e.ctx, 999, plate.SessionPortions{Portions: 2})
	assert.ErrorIs(t, err, domain.ErrNotFound)

	v, err := e.svc.SetSessionPortions(e.ctx, sid, plate.SessionPortions{Portions: 3, Note: ptrStr("  " + strings.Repeat("ü", plate.MaxSessionNoteLen) + " ")})
	require.NoError(t, err, "the limit counts characters after trimming")
	require.NotNil(t, v.Note)
	v, err = e.svc.SetSessionPortions(e.ctx, sid, plate.SessionPortions{Portions: 1, Note: ptrStr("   ")})
	require.NoError(t, err)
	assert.Nil(t, v.Note, "a blank note clears it")
	assert.Contains(t, codes(v.Warnings), plate.WarnDeficit)
}

func codes(ws []plate.Warning) []string {
	out := []string{}
	for _, w := range ws {
		out = append(out, w.Code)
	}
	return out
}

func TestSetCooked(t *testing.T) {
	e := newEnv(t)
	sid := sessionOf(e.create(sun, e.dinner, nil, composed(e.chili, 1)), 0)
	v, err := e.svc.SetCooked(e.ctx, sid, true)
	require.NoError(t, err)
	require.NotNil(t, v.CookedAt)
	v, err = e.svc.SetCooked(e.ctx, sid, false)
	require.NoError(t, err)
	assert.Nil(t, v.CookedAt)
	_, err = e.svc.SetCooked(e.ctx, 999, true)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestSession_KeepDaysAndUseBy(t *testing.T) {
	e := newEnv(t)
	sid := sessionOf(e.create(sun, e.dinner, nil, composed(e.chili, 1)), 0)
	v := e.session(sid)
	assert.Equal(t, 3, v.KeepDays, "plan.default_keep_days")
	assert.Equal(t, day(wed), v.UseBy)

	e.exec(`UPDATE foods SET keep_days = 5 WHERE id = ?`, e.chili)
	assert.Equal(t, 5, e.session(sid).KeepDays)
	_, err := e.svc.Session(e.ctx, 999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// ── Plan snapshot comparison ────────────────────────────────────────────

// planState is a plan with ids replaced by stable keys, so two plans can be
// compared after a delete and restore.
type planState struct {
	Plates   []string
	Sessions []string
}

func (e *env) plan(from, to string) planState {
	e.t.Helper()
	plates, err := e.svc.Range(e.ctx, day(from), day(to))
	require.NoError(e.t, err)
	var st planState
	sessions := map[int64]bool{}
	for _, p := range plates {
		line := fmt.Sprintf("%s/%d eaters=%g note=%v", p.DateString(), p.SlotID, p.Eaters, deref(p.Note))
		if p.Feedback != nil {
			line += fmt.Sprintf(" feedback=%s:%v@%s", p.Feedback.Status, deref(p.Feedback.Note), p.Feedback.RatedAt.Format(time.RFC3339))
		}
		for _, c := range p.Components {
			line += fmt.Sprintf(" [#%d food=%d portions=%v amount=%v unit=%v grams=%v", c.SortOrder, c.FoodID, derefF(c.Portions), derefF(c.Amount), deref(c.Unit), derefF(c.Grams))
			if c.CookSessionID != nil {
				s := e.session(*c.CookSessionID)
				line += " session=" + sessionKey(s)
				sessions[s.ID] = true
			}
			line += "]"
		}
		st.Plates = append(st.Plates, line)
	}
	for id := range sessions {
		v := e.session(id)
		cooked := "nil"
		if v.CookedAt != nil {
			cooked = "set"
		}
		links := []string{}
		for _, l := range v.Links {
			links = append(links, fmt.Sprintf("%s/%d:%g:%s:%d", l.Cell.Date.Format(time.DateOnly), l.Cell.SlotID, l.Portions, l.Kind, l.Position))
		}
		st.Sessions = append(st.Sessions, fmt.Sprintf("%s portions=%g frozen=%g cooked=%s note=%v links=%v",
			sessionKey(v), v.Portions, v.FrozenPortions, cooked, deref(v.Note), links))
	}
	sort.Strings(st.Sessions)
	return st
}

func sessionKey(v *plate.SessionView) string {
	return fmt.Sprintf("food%d@%s/%d", v.FoodID, v.CookDate.Format(time.DateOnly), v.CookSlotID)
}

func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func derefF(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}
