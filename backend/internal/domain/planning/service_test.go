package planning_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// The service tests run the whole pipeline over SQLite with the "Batch
// cooker" fixture (plan §0.1): Saturday shopping, window Sat 2026-11-07 …
// Fri 11-13, today Fri 11-06; Mon–Fri breakfast routine Oats; batch cooking,
// keep 3 days, 1 occasion per day, variety 3 weeks, no freezing; household
// portions 1; 10 mains (reference portions 4, 3 eaten in the last 2 weeks)
// and 4 sides.

const svcToday = "2026-11-06"

type svcEnv struct {
	t                        testing.TB
	ctx                      context.Context
	db                       *sql.DB
	deps                     planning.Deps
	svc                      *planning.Service
	runs                     *fakeRuns
	breakfast, lunch, dinner int64
	mains, sides             []int64
	oats                     int64
	plates                   *plate.Service
	avail                    *availability.Service
	start                    time.Time
}

func newSvcEnv(t testing.TB) *svcEnv {
	t.Helper()
	db := testhelper.NewTestDB(t)
	e := &svcEnv{t: t, ctx: context.Background(), db: db, start: day(sat)}
	e.breakfast = e.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.breakfast', 'coffee', 0, 1)`)
	e.lunch = e.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.lunch', 'sun', 1, 1)`)
	e.dinner = e.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 2, 1)`)
	set := testsvc.Settings(db)
	for k, v := range map[string]string{
		"plan.shopping_day": "5", "plan.batch_cooking": "true", "plan.default_keep_days": "3",
		"plan.max_cook_occasions_per_day": "1", "plan.variety_weeks": "3", "plan.freezing_enabled": "false",
	} {
		require.NoError(t, set.Set(e.ctx, k, v))
	}
	e.exec(`UPDATE user_profile SET household_portions = 1, diet_type = 'omnivore' WHERE id = 1`)
	beef := e.insert(`INSERT INTO foods (name, kind, source, kcal_100g, protein_100g, fat_100g, carbs_100g) VALUES ('Rinderhackfleisch', 'leaf', 'manual', 250, 20, 18, 0)`)
	beans := e.insert(`INSERT INTO foods (name, kind, source, kcal_100g, protein_100g, fat_100g, carbs_100g) VALUES ('Kichererbsen', 'leaf', 'manual', 120, 7, 2, 18)`)
	for i := range 10 {
		id := e.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES (?, 'composed', 'main', 4)`, fmt.Sprintf("Main %02d", i+1))
		leaf := beans
		if i%2 == 0 {
			leaf = beef
		}
		e.exec(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams) VALUES (?, ?, 600, 'g', 600)`, id, leaf)
		e.mains = append(e.mains, id)
	}
	for i := range 4 {
		id := e.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES (?, 'composed', 'side_veg', 4)`, fmt.Sprintf("Side %d", i+1))
		e.exec(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams) VALUES (?, ?, 400, 'g', 400)`, id, beans)
		e.sides = append(e.sides, id)
	}
	e.oats = e.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Oats', 'composed', 'standalone', 1)`)
	e.exec(`INSERT INTO food_components (parent_id, child_id, amount, unit, grams) VALUES (?, ?, 80, 'g', 80)`, e.oats, beans)
	preset := e.insert(`INSERT INTO presets (name) VALUES ('Oats')`)
	pp := e.insert(`INSERT INTO preset_plates (preset_id, slot_id) VALUES (?, ?)`, preset, e.breakfast)
	e.exec(`INSERT INTO preset_components (preset_plate_id, food_id, portions) VALUES (?, ?, 1)`, pp, e.oats)

	e.deps = testsvc.PlanningDeps(db)
	e.plates = testsvc.Plates(db)
	_, e.avail = testsvc.Plan(db)
	var defaults []availability.Default
	for wd := 0; wd < 5; wd++ {
		defaults = append(defaults, availability.Default{Weekday: wd, SlotID: e.breakfast, Mode: availability.ModeRoutine, PresetID: &preset})
	}
	require.NoError(t, e.avail.SetDefaults(e.ctx, defaults))
	// The first three mains were eaten in the last two weeks.
	for i, d := range []string{"2026-10-25", "2026-10-28", "2026-11-01"} {
		e.userPlate(d, e.dinner, e.mains[i])
	}
	e.runs = newFakeRuns(t, db)
	e.deps.RunLog, e.deps.Runs = e.runs, e.runs
	e.deps.Go = func(f func()) { f() }
	e.svc = planning.NewService(e.deps)
	return e
}

func (e *svcEnv) insert(q string, args ...any) int64 {
	e.t.Helper()
	res, err := e.db.Exec(q, args...)
	require.NoError(e.t, err, q)
	id, err := res.LastInsertId()
	require.NoError(e.t, err)
	return id
}

func (e *svcEnv) exec(q string, args ...any) {
	e.t.Helper()
	_, err := e.db.Exec(q, args...)
	require.NoError(e.t, err, q)
}

// userPlate creates a confirmed plate of one fresh recipe.
func (e *svcEnv) userPlate(date string, slot, foodID int64) *plate.Plate {
	e.t.Helper()
	one := 1.0
	p, err := e.plates.Create(e.ctx, plate.PlateInput{Date: day(date), SlotID: slot, Components: []plate.PlateComponent{{FoodID: foodID, Portions: &one}}})
	require.NoError(e.t, err)
	return p
}

func (e *svcEnv) none(date string, slot int64) {
	e.t.Helper()
	_, _, err := e.avail.SetOverride(e.ctx, availability.OverrideInput{Date: day(date), SlotID: slot, Mode: availability.ModeNone})
	require.NoError(e.t, err)
}

func (e *svcEnv) req() planning.Request {
	return planning.Request{Start: e.start, Today: day(svcToday)}
}

func (e *svcEnv) generate(req planning.Request) int64 {
	e.t.Helper()
	id, err := e.svc.Generate(e.ctx, req)
	require.NoError(e.t, err)
	return id
}

func (e *svcEnv) regenerate(sc planning.Scope) int64 {
	e.t.Helper()
	req := e.req()
	req.Scope = sc
	id, err := e.svc.Regenerate(e.ctx, req)
	require.NoError(e.t, err)
	return id
}

// window returns the window's plates by "date/slot".
func (e *svcEnv) window() map[string]plate.Plate {
	e.t.Helper()
	plates, err := e.plates.Range(e.ctx, e.start, e.start.AddDate(0, 0, 6))
	require.NoError(e.t, err)
	out := map[string]plate.Plate{}
	for _, p := range plates {
		out[key(p.DateString(), p.SlotID)] = p
	}
	return out
}

func key(date string, slot int64) string { return fmt.Sprintf("%s/%d", date, slot) }

// ok asserts run id ended ok and returns its end.
func (e *svcEnv) ok(id int64) planning.RunEnd {
	e.t.Helper()
	end := e.runs.end(id)
	require.Equal(e.t, airun.StatusOK, end.Status, "error key %q, violations %s", end.ErrorKey, end.ViolationsJSON)
	return end
}

func (e *svcEnv) scope(id int64) map[string]any {
	e.t.Helper()
	var m map[string]any
	require.NoError(e.t, json.Unmarshal([]byte(e.runs.start(id).ScopeJSON), &m))
	return m
}

// B6-AC1: the batch cooker's week without an AI key.
func TestService_BatchCookerWeek(t *testing.T) {
	e := newSvcEnv(t)
	e.none("2026-11-12", e.dinner)
	e.none(fri, e.lunch)

	id := e.generate(e.req())

	start := e.runs.start(id)
	assert.Equal(t, airun.KindGenerate, start.Kind)
	assert.Equal(t, airun.ChooserHeuristic, start.Chooser)
	assert.Equal(t, planning.HeuristicVersion, start.PromptVersion)
	end := e.ok(id)
	assert.JSONEq(t, `[]`, end.ViolationsJSON)
	var score map[string]any
	require.NoError(t, json.Unmarshal([]byte(end.ScoreJSON), &score))
	for _, k := range []string{"coverage", "hard_violations", "macro_deviation", "cook_occasions", "variety", "preference_fit", "waste_portions", "unknown_data_share"} {
		assert.Contains(t, score, k)
	}
	assert.InDelta(t, 1.0, score["coverage"], 1e-9)

	w := e.window()
	sessions := map[int64]*plate.SessionView{}
	cookDays := map[string]int{}
	for _, d := range []string{sat, sun, mon, tue, wed, "2026-11-12", fri} {
		for _, slot := range []int64{e.breakfast, e.lunch, e.dinner} {
			p, ok := w[key(d, slot)]
			isNone := d == "2026-11-12" && slot == e.dinner || d == fri && slot == e.lunch
			if isNone {
				assert.False(t, ok, "none cell %s stays empty", key(d, slot))
				continue
			}
			require.True(t, ok, "cell %s has a plate", key(d, slot))
			assert.Equal(t, plate.StateDraft, p.State)
			require.NotNil(t, p.AIRunID)
			assert.Equal(t, id, *p.AIRunID)
			weekday := day(d).Weekday()
			if slot == e.breakfast && weekday >= time.Monday && weekday <= time.Friday {
				require.Len(t, p.Components, 1)
				assert.Equal(t, e.oats, p.Components[0].FoodID, "routine breakfast %s", d)
				continue
			}
			for _, c := range p.Components {
				assert.NotContains(t, e.mains[:3], c.FoodID, "recent main cooked on %s", key(d, slot))
				if _, seen := sessions[*c.CookSessionID]; !seen {
					v, err := e.plates.Session(e.ctx, *c.CookSessionID)
					require.NoError(t, err)
					sessions[v.ID] = v
					assert.Empty(t, v.Warnings, "session %d", v.ID)
					assert.NotNil(t, v.Reason)
				}
			}
		}
	}
	maxLater := 0
	for _, v := range sessions {
		if v.FoodID == e.oats {
			continue
		}
		cookDays[v.CookDate.Format(time.DateOnly)+fmt.Sprint(v.CookSlotID)]++
		maxLater = max(maxLater, v.Meals-1)
	}
	assert.GreaterOrEqual(t, maxLater, 3, "a session feeds ≥ 3 later cells")
	perDay := map[string]int{}
	for k := range cookDays {
		perDay[k[:10]]++
	}
	for d, n := range perDay {
		assert.Equal(t, 1, n, "one cooking occasion on %s", d)
	}
	sc := e.scope(id)
	assert.Equal(t, "window", sc["kind"])
	assert.InDelta(t, 0, sc["kept_confirmed"], 1e-9)
}

// B6-AC3: a window generate keeps confirmed plates as they are.
func TestService_GenerateKeepsConfirmedPlates(t *testing.T) {
	e := newSvcEnv(t)
	sunDinner := e.userPlate(sun, e.dinner, e.mains[5])
	tueLunch := e.userPlate(tue, e.lunch, e.mains[6])
	e.ok(e.generate(e.req()))

	id := e.generate(e.req()) // regenerate week (D7)

	e.ok(id)
	w := e.window()
	assert.Equal(t, *sunDinner, w[key(sun, e.dinner)], "byte-identical")
	assert.Equal(t, *tueLunch, w[key(tue, e.lunch)], "byte-identical")
	v, err := e.svc.Run(e.ctx, id)
	require.NoError(t, err)
	assert.Equal(t, 2, v.Counts.KeptConfirmed)
	assert.Equal(t, airun.KindGenerate, v.Kind)
}

// Generator plates that eat from a confirmed session are confirmed (D3),
// and a later window generate keeps them.
func TestService_LeftoversOfAConfirmedBatchAreConfirmedAndKept(t *testing.T) {
	e := newSvcEnv(t)
	two := 3.0
	p, err := e.plates.Create(e.ctx, plate.PlateInput{Date: day(sat), SlotID: e.breakfast, Eaters: &two, Components: []plate.PlateComponent{{FoodID: e.mains[5], Portions: f64(1)}}})
	require.NoError(t, err)
	sid := *p.Components[0].CookSessionID
	e.exec(`UPDATE cook_sessions SET portions = 5 WHERE id = ?`, sid) // 2 spare

	first := e.generate(e.req())
	e.ok(first)
	v, err := e.plates.Session(e.ctx, sid)
	require.NoError(t, err)
	require.Len(t, v.Links, 3, "the spare portions feed two cells first")
	var kept []plate.Plate
	for _, l := range v.Links[1:] {
		pl, err := e.plates.Get(e.ctx, l.PlateID)
		require.NoError(t, err)
		assert.Equal(t, plate.StateConfirmed, pl.State, "linked to a confirmed session")
		assert.Equal(t, first, *pl.AIRunID)
		kept = append(kept, *pl)
	}

	again := e.generate(e.req())
	e.ok(again)
	for _, pl := range kept {
		got, err := e.plates.Get(e.ctx, pl.ID)
		require.NoError(t, err, "never deleted")
		assert.Equal(t, pl, *got)
	}
	run, err := e.svc.Run(e.ctx, again)
	require.NoError(t, err)
	assert.Equal(t, 3, run.Counts.KeptConfirmed)
}

// regenerated tells which plates of before changed in after: gone or with
// other components.
func regenerated(before, after map[string]plate.Plate) []string {
	var out []string
	for k, b := range before {
		a, ok := after[k]
		if !ok || a.ID != b.ID || len(a.Components) != len(b.Components) {
			out = append(out, k)
			continue
		}
		for i := range a.Components {
			if a.Components[i].ID != b.Components[i].ID || a.Components[i].FoodID != b.Components[i].FoodID {
				out = append(out, k)
				break
			}
		}
	}
	return out
}

func closureKeys(e *svcEnv, before map[string]plate.Plate, plateID int64) []string {
	e.t.Helper()
	_, plates, err := e.plates.PlateClosure(e.ctx, plateID)
	require.NoError(e.t, err)
	var out []string
	for _, p := range plates {
		if _, ok := before[key(p.DateString(), p.SlotID)]; ok {
			out = append(out, key(p.DateString(), p.SlotID))
		}
	}
	return out
}

func idsOf(v any) []int64 {
	var out []int64
	for _, x := range v.([]any) {
		out = append(out, int64(x.(float64)))
	}
	return out
}

// B6-AC2: each regenerate scope changes only its target.
func TestService_RegenerateScopes(t *testing.T) {
	t.Run("plate replaces its batch closure", func(t *testing.T) {
		e := newSvcEnv(t)
		e.ok(e.generate(e.req()))
		before := e.window()
		monLunch := before[key(mon, e.lunch)]
		closure := closureKeys(e, before, monLunch.ID)
		rows, _, err := e.plates.PlateClosure(e.ctx, monLunch.ID)
		require.NoError(t, err)

		id := e.regenerate(planning.Scope{Kind: planning.ScopePlate, PlateID: monLunch.ID})

		e.ok(id)
		assert.Equal(t, airun.KindRegenerate, e.runs.start(id).Kind)
		after := e.window()
		assert.ElementsMatch(t, closure, regenerated(before, after))
		assert.NotEqual(t, monLunch.Components[0].FoodID, after[key(mon, e.lunch)].Components[0].FoodID, "a different recipe")
		sc := e.scope(id)
		assert.Equal(t, "plate", sc["kind"])
		assert.Equal(t, rows.PlateIDs, idsOf(sc["plate_ids"]), "closure expansion recorded")
		assert.Equal(t, rows.SessionIDs, idsOf(sc["session_ids"]))
	})

	t.Run("side changes only the side", func(t *testing.T) {
		e := newSvcEnv(t)
		e.ok(e.generate(e.req()))
		before := e.window()
		p := before[key(sun, e.dinner)]
		require.Len(t, p.Components, 2)
		side := p.Components[1]

		e.ok(e.regenerate(planning.Scope{Kind: planning.ScopeSide, ComponentID: side.ID}))

		after := e.window()
		got := after[key(sun, e.dinner)]
		assert.Equal(t, p.ID, got.ID, "the plate stays")
		require.Len(t, got.Components, 2)
		assert.Equal(t, p.Components[0], got.Components[0], "the main stays")
		assert.NotEqual(t, side.FoodID, got.Components[1].FoodID, "a different side")
		for k, b := range before {
			a := after[k]
			assert.Equal(t, b.ID, a.ID, k)
			if len(b.Components) == 2 && *b.Components[1].CookSessionID == *side.CookSessionID {
				assert.Equal(t, got.Components[1].CookSessionID, a.Components[1].CookSessionID, "every plate of the side's session gets the new side")
				continue
			}
			assert.Equal(t, b.Components, a.Components, k)
		}
	})

	t.Run("day changes that day and its closures", func(t *testing.T) {
		e := newSvcEnv(t)
		e.ok(e.generate(e.req()))
		before := e.window()
		allowed := map[string]bool{}
		for _, slot := range []int64{e.breakfast, e.lunch, e.dinner} {
			if p, ok := before[key(tue, slot)]; ok {
				allowed[key(tue, slot)] = true
				for _, k := range closureKeys(e, before, p.ID) {
					allowed[k] = true
				}
			}
		}

		e.ok(e.regenerate(planning.Scope{Kind: planning.ScopeDay, Date: day(tue)}))

		changed := regenerated(before, e.window())
		assert.NotEmpty(t, changed)
		for _, k := range changed {
			assert.True(t, allowed[k], "%s is outside Tuesday's closure", k)
		}
	})

	t.Run("empty_cells fills exactly the empty cells", func(t *testing.T) {
		e := newSvcEnv(t)
		e.ok(e.generate(e.req()))
		before := e.window()
		var deleted []string
		// Sunday's and Tuesday's cook plates: their batches stay, so refilling them cooks
		// at the same occasion.
		for _, k := range []string{key(sun, e.dinner), key(tue, e.dinner)} {
			_, err := e.plates.Delete(e.ctx, before[k].ID)
			require.NoError(t, err)
			deleted = append(deleted, k)
		}
		mid := e.window()

		id := e.regenerate(planning.Scope{Kind: planning.ScopeEmptyCells})

		e.ok(id)
		after := e.window()
		assert.Empty(t, regenerated(mid, after), "nothing else changes")
		assert.Len(t, after, len(before))
		for _, k := range deleted {
			assert.Contains(t, after, k)
		}
	})

	t.Run("a confirmed plate or side is 409", func(t *testing.T) {
		e := newSvcEnv(t)
		e.ok(e.generate(e.req()))
		p := e.window()[key(sun, e.dinner)]
		_, err := e.plates.Confirm(e.ctx, plate.RowSet{PlateIDs: []int64{p.ID}})
		require.NoError(t, err)
		for _, sc := range []planning.Scope{
			{Kind: planning.ScopePlate, PlateID: p.ID},
			{Kind: planning.ScopeSide, ComponentID: p.Components[1].ID},
		} {
			req := e.req()
			req.Scope = sc
			_, err := e.svc.Regenerate(e.ctx, req)
			assert.ErrorIs(t, err, domain.ErrScopeConfirmed, sc.Kind)
		}
		_, running := e.svc.Running(e.start)
		assert.False(t, running, "a refused request frees the window")
	})

	t.Run("invalid scopes", func(t *testing.T) {
		e := newSvcEnv(t)
		e.ok(e.generate(e.req()))
		main := e.window()[key(sun, e.dinner)].Components[0]
		for name, sc := range map[string]planning.Scope{
			"window":           {Kind: planning.ScopeWindow},
			"unknown kind":     {Kind: "week"},
			"day outside":      {Kind: planning.ScopeDay, Date: day("2026-11-20")},
			"past day":         {Kind: planning.ScopeDay, Date: day(svcToday)},
			"side on the main": {Kind: planning.ScopeSide, ComponentID: main.ID},
		} {
			req := e.req()
			req.Scope = sc
			_, err := e.svc.Regenerate(e.ctx, req)
			assert.ErrorIs(t, err, domain.ErrInvalidScope, name)
		}
		req := e.req()
		req.Scope = planning.Scope{Kind: planning.ScopePlate, PlateID: 9999}
		_, err := e.svc.Regenerate(e.ctx, req)
		assert.ErrorIs(t, err, domain.ErrNotFound)
	})
}

// blockingChooser waits for release before it lets the heuristic choose.
type blockingChooser struct {
	entered chan struct{}
	release chan struct{}
}

func (b blockingChooser) Choose(ctx context.Context, in planning.ChooseInput) (planning.Proposal, error) {
	close(b.entered)
	<-b.release
	return planning.Heuristic{}.Choose(ctx, in)
}

// B6-AC4: one run per window, and a run outlives its request.
func TestService_OneRunPerWindowAndDetachedFromTheRequest(t *testing.T) {
	e := newSvcEnv(t)
	b := blockingChooser{entered: make(chan struct{}), release: make(chan struct{})}
	e.deps.Go = nil
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: b}
	e.svc = planning.NewService(e.deps)
	reqCtx, cancel := context.WithCancel(e.ctx)

	id, err := e.svc.Generate(reqCtx, e.req())
	require.NoError(t, err)
	cancel() // the client went away
	<-b.entered

	_, err = e.svc.Generate(e.ctx, e.req())
	assert.ErrorIs(t, err, domain.ErrGenerationRunning)
	running, ok := e.svc.Running(e.start)
	assert.True(t, ok)
	assert.Equal(t, id, running)
	v, err := e.svc.Run(e.ctx, id)
	require.NoError(t, err)
	assert.Equal(t, airun.StatusRunning, v.Status)
	assert.Equal(t, planning.StageChoosing, v.Stage)

	close(b.release)
	e.svc.Wait()
	e.ok(id)
	_, ok = e.svc.Running(e.start)
	assert.False(t, ok)
	v, err = e.svc.Run(e.ctx, id)
	require.NoError(t, err)
	assert.Equal(t, airun.StatusOK, v.Status)
	assert.Empty(t, v.Stage)
	assert.Positive(t, v.Counts.Plates)
	assert.Positive(t, v.Counts.Sessions)
	assert.Positive(t, v.Counts.CookDays)
	require.NotNil(t, v.Score)
	assert.Equal(t, day(sat), v.Window.Start)
}

// failingApply writes the snapshot, then fails: the unit of work must roll
// everything back.
type failingApply struct{ planning.PlateService }

func (f failingApply) Apply(ctx context.Context, snap plate.PlanSnapshot, opts plate.ApplyOptions) (*plate.ApplyResult, error) {
	if _, err := f.PlateService.Apply(ctx, snap, opts); err != nil {
		return nil, err
	}
	return nil, domain.ErrCellOccupied
}

// B6-AC5: a failing apply leaves nothing behind.
func TestService_FailingApplyLeavesNoRows(t *testing.T) {
	e := newSvcEnv(t)
	e.ok(e.generate(e.req()))
	before := e.window()
	e.deps.Plates = failingApply{e.deps.Plates}
	e.svc = planning.NewService(e.deps)

	id := e.generate(e.req())

	end := e.runs.end(id)
	assert.Equal(t, airun.StatusError, end.Status)
	assert.Equal(t, "error.plate.cell_occupied", end.ErrorKey)
	assert.Equal(t, before, e.window(), "the drafts it was to replace are back")
	var n int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM plates WHERE ai_run_id = ?`, id).Scan(&n))
	assert.Zero(t, n)
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM cook_sessions WHERE ai_run_id = ?`, id).Scan(&n))
	assert.Zero(t, n)
}

// B6-AC6: guests and a new recipe.
func TestService_EatersByDateAndNewRecipe(t *testing.T) {
	e := newSvcEnv(t)
	req := e.req()
	req.EatersByDate = map[string]float64{sun: 3}
	e.ok(e.generate(req))
	for k, p := range e.window() {
		want := 1.0
		if p.DateString() == sun {
			want = 3
		}
		assert.InDelta(t, want, p.Eaters, 1e-9, k)
		for _, c := range p.Components {
			v, err := e.plates.Session(e.ctx, *c.CookSessionID)
			require.NoError(t, err)
			assert.InDelta(t, v.Portions, v.Consumed, 1e-9, "session %d sized to its plates", v.ID)
		}
	}

	newRecipes := func(e *svcEnv) int {
		n := 0
		for _, p := range e.window() {
			for _, c := range p.Components {
				if c.FoodID == e.mains[8] || c.FoodID == e.mains[9] {
					v, err := e.plates.Session(e.ctx, *c.CookSessionID)
					require.NoError(t, err)
					if v.Links[0].PlateID == p.ID {
						n++
					}
				}
			}
		}
		return n
	}
	withHistory := func(t *testing.T) *svcEnv {
		e := newSvcEnv(t)
		// Mains 4–8 were eaten five weeks ago; 9 and 10 never.
		for i := 3; i < 8; i++ {
			e.userPlate(fmt.Sprintf("2026-10-%02d", 1+i), e.dinner, e.mains[i])
		}
		return e
	}
	e = withHistory(t)
	yes := true
	req = e.req()
	req.IncludeNewRecipe = &yes
	e.ok(e.generate(req))
	assert.Equal(t, 1, newRecipes(e), "exactly one never-eaten recipe")
	e = withHistory(t)
	e.ok(e.generate(e.req()))
	assert.Zero(t, newRecipes(e), "plan.explore_per_week 0: none")
}

// A regenerate keeps the guests of the plates it replaces (D16).
func TestService_RegenerateKeepsGuests(t *testing.T) {
	e := newSvcEnv(t)
	req := e.req()
	req.EatersByDate = map[string]float64{tue: 3}
	e.ok(e.generate(req))
	p := e.window()[key(tue, e.dinner)]
	require.InDelta(t, 3.0, p.Eaters, 1e-9)

	id := e.regenerate(planning.Scope{Kind: planning.ScopePlate, PlateID: p.ID})

	e.ok(id)
	got := e.window()[key(tue, e.dinner)]
	assert.NotEqual(t, p.ID, got.ID, "replaced")
	assert.InDelta(t, 3.0, got.Eaters, 1e-9, "the guests stay")
}

type panicChooser struct{}

func (panicChooser) Choose(context.Context, planning.ChooseInput) (planning.Proposal, error) {
	panic("chooser bug")
}

// waitChooser blocks until the run's context ends.
type waitChooser struct{}

func (waitChooser) Choose(ctx context.Context, _ planning.ChooseInput) (planning.Proposal, error) {
	<-ctx.Done()
	return planning.Proposal{}, ctx.Err()
}

// A panicking chooser and a run past its timeout end the run as an error
// without rows and free the window.
func TestService_PanicAndTimeoutEndTheRun(t *testing.T) {
	for name, edit := range map[string]func(*planning.Deps){
		"panic": func(d *planning.Deps) {
			d.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: panicChooser{}}
		},
		"timeout": func(d *planning.Deps) {
			d.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: waitChooser{}}
			d.RunTimeout = time.Millisecond
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newSvcEnv(t)
			edit(&e.deps)
			e.svc = planning.NewService(e.deps)

			id := e.generate(e.req())

			end := e.runs.end(id)
			assert.Equal(t, airun.StatusError, end.Status)
			assert.Equal(t, "error.server", end.ErrorKey)
			assert.Empty(t, e.window(), "no rows")
			_, running := e.svc.Running(e.start)
			assert.False(t, running, "the window is free")
		})
	}
}

// B6-AC7.
func TestService_NotEnoughRecipes(t *testing.T) {
	e := newSvcEnv(t)
	e.exec(`DELETE FROM foods WHERE role = 'side_veg' OR name IN ('Main 08', 'Main 09', 'Main 10')`)
	_, err := e.svc.Generate(e.ctx, e.req())
	assert.ErrorIs(t, err, domain.ErrNotEnoughRecipes)
	_, running := e.svc.Running(e.start)
	assert.False(t, running)
}

func TestService_RequestValidation(t *testing.T) {
	e := newSvcEnv(t)
	tests := []struct {
		name string
		edit func(*planning.Request)
		want error
	}{
		{"start is not a shopping day", func(r *planning.Request) { r.Start = day(sun) }, domain.ErrInvalidPlanStart},
		{"guests outside the window", func(r *planning.Request) { r.EatersByDate = map[string]float64{"2026-11-20": 2} }, domain.ErrInvalidInput},
		{"guests not a date", func(r *planning.Request) { r.EatersByDate = map[string]float64{"sunday": 2} }, domain.ErrInvalidInput},
		{"zero guests", func(r *planning.Request) { r.EatersByDate = map[string]float64{sun: 0} }, domain.ErrInvalidInput},
		{"unknown chooser", func(r *planning.Request) { r.Chooser = "dice" }, domain.ErrInvalidInput},
		{"no llm chooser", func(r *planning.Request) { r.Chooser = airun.ChooserLLM }, domain.ErrAIProviderMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := e.req()
			tt.edit(&req)
			_, err := e.svc.Generate(e.ctx, req)
			assert.ErrorIs(t, err, tt.want)
		})
	}
	_, err := e.svc.Run(e.ctx, 9999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// fakeRuns is the run log: it inserts a bare ai_runs row per run (plates
// reference it) and keeps the starts and ends in memory.
type fakeRuns struct {
	t      testing.TB
	db     *sql.DB
	mu     sync.Mutex
	starts map[int64]planning.RunStart
	ends   map[int64]planning.RunEnd
}

func newFakeRuns(t testing.TB, db *sql.DB) *fakeRuns {
	return &fakeRuns{t: t, db: db, starts: map[int64]planning.RunStart{}, ends: map[int64]planning.RunEnd{}}
}

type fakeRun struct {
	f  *fakeRuns
	id int64
}

func (f *fakeRuns) Start(ctx context.Context, s planning.RunStart) (context.Context, planning.Run, error) {
	res, err := f.db.ExecContext(ctx, `INSERT INTO ai_runs (kind, started_at) VALUES (?, '2026-11-06 10:00:00')`, string(s.Kind))
	if err != nil {
		return nil, nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.starts[id] = s
	return ctx, fakeRun{f: f, id: id}, nil
}

func (r fakeRun) ID() int64 { return r.id }

func (r fakeRun) Finish(_ context.Context, end planning.RunEnd) {
	r.f.mu.Lock()
	defer r.f.mu.Unlock()
	r.f.ends[r.id] = end
}

func (f *fakeRuns) start(id int64) planning.RunStart {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.starts[id]
	require.True(f.t, ok, "run %d started", id)
	return s
}

func (f *fakeRuns) end(id int64) planning.RunEnd {
	f.mu.Lock()
	defer f.mu.Unlock()
	end, ok := f.ends[id]
	require.True(f.t, ok, "run %d finished", id)
	return end
}

func (f *fakeRuns) PlanRun(_ context.Context, id int64) (planning.RunRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.starts[id]
	if !ok {
		return planning.RunRecord{}, domain.ErrNotFound
	}
	rec := planning.RunRecord{ID: id, Kind: s.Kind, Status: airun.StatusRunning, Chooser: s.Chooser, ScopeJSON: s.ScopeJSON, WindowStart: s.WindowStart, WindowEnd: s.WindowEnd}
	if end, done := f.ends[id]; done {
		rec.Status, rec.ErrorKey, rec.Notes = end.Status, end.ErrorKey, end.Notes
		rec.ViolationsJSON, rec.ScoreJSON = end.ViolationsJSON, end.ScoreJSON
	}
	return rec, nil
}

func (f *fakeRuns) PlanRunCounts(ctx context.Context, id int64) (planning.RunCounts, error) {
	var c planning.RunCounts
	err := f.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM plates WHERE ai_run_id = ?),
		(SELECT COUNT(*) FROM cook_sessions WHERE ai_run_id = ?),
		(SELECT COUNT(DISTINCT cook_date) FROM cook_sessions WHERE ai_run_id = ?)`, id, id, id).Scan(&c.Plates, &c.Sessions, &c.CookDays)
	return c, err
}
