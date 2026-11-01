package planning_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// Mid-week re-plan (B8, AC-5) on the "Batch cooker" fixture: today is
// Wednesday 2026-11-11 of the window Sat 11-07 … Fri 11-13.
//
//	Mon batch (cooked):  Mon dinner, Tue lunch, Thu dinner      3 portions
//	Chili (cooks today): Wed dinner, Thu lunch, Fri lunch       3 portions
//	Fri dinner:          a draft plate with its own fresh batch
//
// Every row but Fri dinner is confirmed. The re-plan marks Thu and Fri lunch
// and dinner No plate.

const (
	thu        = "2026-11-12"
	replanDay  = wed
	replanFrom = wed
)

type replanEnv struct {
	*svcEnv
	monBatch, chiliBatch, friBatch int64
	monDinner, tueLunch, wedDinner int64
	thuLunch, thuDinner, friLunch  int64
	friDinner                      int64
}

func newReplanEnv(t *testing.T, freezing bool) *replanEnv {
	t.Helper()
	e := &replanEnv{svcEnv: newSvcEnv(t)}
	if freezing {
		require.NoError(t, testsvc.Settings(e.db).Set(e.ctx, "plan.freezing_enabled", "true"))
	}
	batch := func(foodID int64, cook string, leftovers ...plate.CellKey) (int64, []int64) {
		p := e.userPlate(cook, e.dinner, foodID)
		sid := *p.Components[0].CookSessionID
		v, err := e.plates.AddLeftovers(e.ctx, sid, leftovers)
		require.NoError(t, err)
		ids := make([]int64, 0, len(v.Links))
		for _, l := range v.Links {
			ids = append(ids, l.PlateID)
		}
		return sid, ids
	}
	var ids []int64
	e.monBatch, ids = batch(e.mains[5], mon, ck(tue, e.lunch), ck(thu, e.dinner))
	e.monDinner, e.tueLunch, e.thuDinner = ids[0], ids[1], ids[2]
	_, err := e.plates.SetCooked(e.ctx, e.monBatch, true)
	require.NoError(t, err)
	e.chiliBatch, ids = batch(e.mains[4], wed, ck(thu, e.lunch), ck(fri, e.lunch))
	e.wedDinner, e.thuLunch, e.friLunch = ids[0], ids[1], ids[2]

	one := 1.0
	draft, err := e.plates.Create(actor.With(e.ctx, actor.Agent, 0), plate.PlateInput{
		Date: day(fri), SlotID: e.dinner, Components: []plate.PlateComponent{{FoodID: e.mains[6], Portions: &one}},
	})
	require.NoError(t, err)
	require.Equal(t, plate.StateDraft, draft.State)
	e.friDinner, e.friBatch = draft.ID, *draft.Components[0].CookSessionID
	return e
}

func ck(date string, slot int64) plate.CellKey { return plate.CellKey{Date: day(date), SlotID: slot} }

func (e *replanEnv) query() planning.ReplanQuery {
	return planning.ReplanQuery{
		From: day(replanFrom), To: day(fri), Today: day(replanDay),
		NoneCells: []plate.CellKey{ck(thu, e.lunch), ck(thu, e.dinner), ck(fri, e.lunch), ck(fri, e.dinner)},
	}
}

func (e *replanEnv) replan(unlock bool, spare planning.SpareMode, regenerate bool) (*planning.ReplanResult, error) {
	return e.svc.Replan(e.ctx, planning.ReplanRequest{
		ReplanQuery: e.query(), UnlockConfirmed: unlock, Spare: spare, Regenerate: regenerate, Chooser: airun.ChooserHeuristic,
	})
}

func (e *replanEnv) session(id int64) *plate.SessionView {
	e.t.Helper()
	v, err := e.plates.Session(e.ctx, id)
	require.NoError(e.t, err)
	return v
}

func (e *replanEnv) plateGone(id int64) bool {
	_, err := e.plates.Get(e.ctx, id)
	return errors.Is(err, domain.ErrNotFound)
}

// beforeFrom returns the plates dated before the re-plan's from.
func (e *replanEnv) beforeFrom() []plate.Plate {
	e.t.Helper()
	plates, err := e.plates.Range(e.ctx, e.start, day(replanFrom).AddDate(0, 0, -1))
	require.NoError(e.t, err)
	return plates
}

// B8-AC1: the preview lists the No-plate cells with plates, the confirmed
// Thu/Fri plates and the batches left with spare.
func TestReplan_Preview(t *testing.T) {
	e := newReplanEnv(t, true)

	pv, err := e.svc.PreviewReplan(e.ctx, e.query())

	require.NoError(t, err)
	assert.Equal(t, []plate.CellKey{ck(thu, e.lunch), ck(thu, e.dinner), ck(fri, e.lunch), ck(fri, e.dinner)}, pv.NoneCells)
	assert.Equal(t, []planning.AffectedPlate{
		{PlateID: e.thuLunch, Date: day(thu), SlotID: e.lunch, FoodName: "Main 05"},
		{PlateID: e.thuDinner, Date: day(thu), SlotID: e.dinner, FoodName: "Main 06"},
		{PlateID: e.friLunch, Date: day(fri), SlotID: e.lunch, FoodName: "Main 05"},
	}, pv.ConfirmedAffected, "the draft Fri dinner is not listed")
	assert.Equal(t, []planning.Spare{
		{SessionID: e.chiliBatch, FoodName: "Main 05", Portions: 2, Freezable: true, Mutable: true},
		{SessionID: e.monBatch, FoodName: "Main 06", Portions: 1, Freezable: false}, // cooked: nothing to freeze
	}, pv.Spare, "Fri dinner's batch loses every link and goes")
	assert.False(t, e.plateGone(e.thuLunch), "a preview writes nothing")
}

// A cell that already is No plate (here by a weekday default) counts without
// being asked for; asked cells without plates are not listed.
func TestReplan_PreviewReadsExistingNoneCells(t *testing.T) {
	e := newReplanEnv(t, false)
	defaults, err := e.avail.Defaults(e.ctx)
	require.NoError(t, err)
	wd := availability.WeekdayOf(day(thu))
	defaults = slices.DeleteFunc(defaults, func(d availability.Default) bool { return d.Weekday == wd && d.SlotID == e.lunch })
	defaults = append(defaults, availability.Default{Weekday: wd, SlotID: e.lunch, Mode: availability.ModeNone})
	require.NoError(t, e.avail.SetDefaults(e.ctx, defaults))

	pv, err := e.svc.PreviewReplan(e.ctx, planning.ReplanQuery{From: day(wed), To: day(fri), Today: day(wed), NoneCells: []plate.CellKey{ck(wed, e.lunch)}})

	require.NoError(t, err)
	assert.Equal(t, []plate.CellKey{ck(thu, e.lunch)}, pv.NoneCells)
	assert.Equal(t, []planning.Spare{{SessionID: e.chiliBatch, FoodName: "Main 05", Portions: 1, Mutable: true}}, pv.Spare, "not freezable with freezing off, but it may shrink")
}

// B8-AC1: without unlock the confirmed plates block the re-plan (409 with
// them) and nothing changes.
func TestReplan_ConfirmedNeedUnlock(t *testing.T) {
	e := newReplanEnv(t, true)

	_, err := e.replan(false, planning.SpareFreeze, false)

	require.ErrorIs(t, err, domain.ErrConfirmedAffected)
	var ce *planning.ConfirmedAffectedError
	require.ErrorAs(t, err, &ce)
	ids := []int64{}
	for _, p := range ce.Plates {
		ids = append(ids, p.PlateID)
	}
	assert.Equal(t, []int64{e.thuLunch, e.thuDinner, e.friLunch}, ids)
	for _, id := range []int64{e.thuLunch, e.thuDinner, e.friLunch, e.friDinner} {
		assert.False(t, e.plateGone(id))
	}
	cells, err := e.avail.Resolve(e.ctx, day(thu), day(thu))
	require.NoError(t, err)
	for _, c := range cells {
		assert.NotEqual(t, availability.ModeNone, c.Mode, "no override written")
	}
}

// B8-AC1: with unlock the plates go; Chili's spare is frozen when freezing is
// on and the recipe freezable, else Chili shrinks; the cooked Mon batch and
// the days before from stay as they are.
func TestReplan_RemovesAndSettlesBatches(t *testing.T) {
	no := false
	cases := []struct {
		name           string
		freezing       bool
		unfreezable    bool
		spare          planning.SpareMode
		portions, froz float64
	}{
		{"freeze, freezable", true, false, planning.SpareFreeze, 3, 2},
		{"freeze, freezing off", false, false, planning.SpareFreeze, 1, 0},
		{"freeze, not freezable", true, true, planning.SpareFreeze, 1, 0},
		{"keep", true, false, planning.SpareKeep, 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newReplanEnv(t, tc.freezing)
			if tc.unfreezable {
				e.exec(`UPDATE foods SET freezable = ? WHERE id = ?`, no, e.mains[4])
			}
			past := e.beforeFrom()
			mon := e.session(e.monBatch)

			res, err := e.replan(true, tc.spare, false)

			require.NoError(t, err)
			assert.Nil(t, res.RunID)
			assert.Len(t, res.Removed.Plates, 4)
			for _, id := range []int64{e.thuLunch, e.thuDinner, e.friLunch, e.friDinner} {
				assert.True(t, e.plateGone(id), "plate %d", id)
			}
			chili := e.session(e.chiliBatch)
			assert.InDelta(t, tc.portions, chili.Portions, 1e-9)
			assert.InDelta(t, tc.froz, chili.FrozenPortions, 1e-9)
			assert.Len(t, chili.Links, 1)
			after := e.session(e.monBatch)
			assert.Equal(t, mon.Portions, after.Portions, "a cooked batch keeps its portions")
			assert.Equal(t, mon.CookedAt, after.CookedAt)
			_, err = e.plates.CookSession(e.ctx, e.friBatch)
			require.ErrorIs(t, err, domain.ErrNotFound, "the draft's own batch goes with it")
			assert.Equal(t, past, e.beforeFrom(), "days before from are untouched")
			cells, err := e.avail.Resolve(e.ctx, day(thu), day(fri))
			require.NoError(t, err)
			for _, c := range cells {
				if c.SlotID != e.breakfast {
					assert.Equal(t, availability.ModeNone, c.Mode, "%s/%d", c.Date.Format(time.DateOnly), c.SlotID)
				}
			}
		})
	}
}

// A cooked batch is never deleted, not even when every plate eating it goes.
func TestReplan_KeepsACookedBatchWithoutPlates(t *testing.T) {
	e := newReplanEnv(t, false)
	_, err := e.plates.SetCooked(e.ctx, e.friBatch, true)
	require.NoError(t, err)

	_, err = e.replan(true, planning.SpareKeep, false)

	require.NoError(t, err)
	v := e.session(e.friBatch)
	assert.Empty(t, v.Links)
	assert.InDelta(t, 1, v.Portions, 1e-9)
}

// B8-AC2: a re-plan starts today at the earliest; its range lies in one
// window.
func TestReplan_Validation(t *testing.T) {
	e := newReplanEnv(t, false)
	q := e.query()
	q.From = day(tue)
	_, err := e.svc.PreviewReplan(e.ctx, q)
	require.ErrorIs(t, err, domain.ErrReplanPast)
	_, err = e.svc.Replan(e.ctx, planning.ReplanRequest{ReplanQuery: q, Spare: planning.SpareKeep})
	require.ErrorIs(t, err, domain.ErrReplanPast)

	for name, q := range map[string]planning.ReplanQuery{
		"to before from":    {From: day(thu), To: day(wed), Today: day(wed)},
		"to in next week":   {From: day(thu), To: day(fri).AddDate(0, 0, 1), Today: day(wed)},
		"cell out of range": {From: day(thu), To: day(fri), Today: day(wed), NoneCells: []plate.CellKey{ck(wed, e.lunch)}},
	} {
		_, err := e.svc.PreviewReplan(e.ctx, q)
		assert.ErrorIs(t, err, domain.ErrInvalidInput, name)
	}
	_, err = e.svc.Replan(e.ctx, planning.ReplanRequest{ReplanQuery: e.query(), UnlockConfirmed: true, Spare: "toss"})
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.False(t, e.plateGone(e.thuLunch))
}

// B8-AC3: with regenerate an empty_cells run fills only the empty plan cells
// from from on, eating the remaining batches' leftovers first.
func TestReplan_RegenerateFillsTheRestFromFrom(t *testing.T) {
	e := newReplanEnv(t, false)
	past := e.beforeFrom()

	res, err := e.replan(true, planning.SpareKeep, true)

	require.NoError(t, err)
	require.NotNil(t, res.RunID)
	e.ok(*res.RunID)
	sc := e.scope(*res.RunID)
	assert.Equal(t, "empty_cells", sc["kind"])
	assert.Equal(t, wed, sc["from"])
	assert.Equal(t, fri, sc["to"])
	assert.Equal(t, past, e.beforeFrom(), "nothing before from is filled or changed")
	w := e.window()
	wedLunch, ok := w[key(wed, e.lunch)]
	require.True(t, ok, "Wednesday lunch is filled")
	require.Len(t, wedLunch.Components, 1)
	require.NotNil(t, wedLunch.Components[0].CookSessionID)
	assert.Equal(t, e.monBatch, *wedLunch.Components[0].CookSessionID, "the cooked batch's spare portion first")
	for _, k := range []string{key(thu, e.lunch), key(thu, e.dinner), key(fri, e.lunch), key(fri, e.dinner)} {
		assert.NotContains(t, w, k, "No-plate cells stay empty")
	}
}

// A re-plan does not remove plates under a running generation of the
// window: 409, nothing removed.
func TestReplan_GenerationRunning(t *testing.T) {
	e := newReplanEnv(t, false)
	b := blockingChooser{entered: make(chan struct{}), release: make(chan struct{})}
	e.deps.Go = nil
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: b}
	e.svc = planning.NewService(e.deps)
	req := e.req()
	req.Today = day(replanDay)
	_, err := e.svc.Generate(e.ctx, req)
	require.NoError(t, err)
	<-b.entered

	_, err = e.replan(true, planning.SpareKeep, false)

	require.ErrorIs(t, err, domain.ErrGenerationRunning)
	close(b.release)
	e.svc.Wait()
	assert.False(t, e.plateGone(e.thuLunch))
}

// hookedFoods runs onCount on the at-th CountComposedFoods call: the
// re-plan's second one is the run start after the commit.
type hookedFoods struct {
	planning.FoodRows
	n, at   int
	onCount func() error
}

func (h *hookedFoods) CountComposedFoods(ctx context.Context) (int64, error) {
	h.n++
	if h.n == h.at {
		if err := h.onCount(); err != nil {
			return 0, err
		}
	}
	return h.FoodRows.CountComposedFoods(ctx)
}

// The re-plan holds the window from before its commit until its run has
// started: a generate in between is 409.
func TestReplan_HoldsTheWindowUntilTheRunStarts(t *testing.T) {
	e := newReplanEnv(t, false)
	var raced error
	h := &hookedFoods{FoodRows: e.deps.FoodRows, at: 2}
	h.onCount = func() error {
		req := e.req()
		req.Today = day(replanDay)
		_, raced = e.svc.Generate(e.ctx, req)
		return nil
	}
	e.deps.FoodRows = h
	e.svc = planning.NewService(e.deps)

	res, err := e.replan(true, planning.SpareKeep, true)

	require.NoError(t, err)
	require.ErrorIs(t, raced, domain.ErrGenerationRunning)
	require.NotNil(t, res.RunID)
	e.ok(*res.RunID)
}

// A run that fails to start after the commit leaves the removal, no run id
// and a free window.
func TestReplan_RunThatDoesNotStartKeepsTheRemoval(t *testing.T) {
	e := newReplanEnv(t, false)
	e.deps.FoodRows = &hookedFoods{FoodRows: e.deps.FoodRows, at: 2, onCount: func() error { return errors.New("db gone") }}
	e.svc = planning.NewService(e.deps)

	res, err := e.replan(true, planning.SpareKeep, true)

	require.NoError(t, err)
	assert.Nil(t, res.RunID)
	assert.Len(t, res.Removed.Plates, 4)
	assert.True(t, e.plateGone(e.thuLunch))
	_, running := e.svc.Running(e.start)
	assert.False(t, running)
	req := e.req()
	req.Today = day(replanDay)
	_, err = e.svc.Generate(e.ctx, req)
	assert.NoError(t, err, "the window is free again")
}

// orphanEnv is a batch cooked Thu lunch and eaten again Fri lunch; the
// re-plan from Wed marks Thu lunch No plate.
func orphanEnv(t *testing.T) (*replanEnv, *plate.Plate, planning.ReplanQuery) {
	t.Helper()
	e := &replanEnv{svcEnv: newSvcEnv(t)}
	cook := e.userPlate(thu, e.lunch, e.mains[4])
	_, err := e.plates.AddLeftovers(e.ctx, *cook.Components[0].CookSessionID, []plate.CellKey{ck(fri, e.lunch)})
	require.NoError(t, err)
	return e, cook, planning.ReplanQuery{From: day(wed), To: day(fri), Today: day(wed), NoneCells: []plate.CellKey{ck(thu, e.lunch)}}
}

// A batch that loses its cook plate but keeps a later plate cooks there
// instead of on the No-plate day: Fri lunch now eats it fresh, and the batch
// shrinks to it (or, freezing off, cannot freeze its spare).
func TestReplan_MovesAnOrphanedCookToItsFirstPlate(t *testing.T) {
	for _, spare := range []planning.SpareMode{planning.SpareKeep, planning.SpareFreeze} {
		t.Run(string(spare), func(t *testing.T) {
			e, cook, q := orphanEnv(t)
			sid := *cook.Components[0].CookSessionID

			pv, err := e.svc.PreviewReplan(e.ctx, q)
			require.NoError(t, err)
			assert.Equal(t, []planning.Spare{{SessionID: sid, FoodName: "Main 05", Portions: 1}}, pv.Spare)

			_, err = e.svc.Replan(e.ctx, planning.ReplanRequest{ReplanQuery: q, UnlockConfirmed: true, Spare: spare})

			require.NoError(t, err)
			assert.True(t, e.plateGone(cook.ID))
			v := e.session(sid)
			assert.True(t, ck(fri, e.lunch).Equal(v.CookCell()), "cooks where it is still eaten, not on the No-plate day: %v", v.CookCell())
			assert.InDelta(t, 1, v.Portions, 1e-9)
			assert.Zero(t, v.FrozenPortions)
			require.Len(t, v.Links, 1)
			assert.Equal(t, plate.KindFresh, v.Links[0].Kind)
			assert.Empty(t, v.Warnings)
		})
	}
}

// A cooked batch keeps its cook cell even when its cook plate goes.
func TestReplan_KeepsACookedBatchsCookCell(t *testing.T) {
	e, cook, q := orphanEnv(t)
	sid := *cook.Components[0].CookSessionID
	_, err := e.plates.SetCooked(e.ctx, sid, true)
	require.NoError(t, err)

	_, err = e.svc.Replan(e.ctx, planning.ReplanRequest{ReplanQuery: q, UnlockConfirmed: true, Spare: planning.SpareKeep})

	require.NoError(t, err)
	assert.True(t, ck(thu, e.lunch).Equal(e.session(sid).CookCell()))
}
