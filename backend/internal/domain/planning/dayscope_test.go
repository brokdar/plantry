package planning_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// Slice 3b: "Regenerate day" stays inside the day. The day's drafts go
// through RemoveMeals, so a batch cooked on the day keeps feeding its later
// meals (the cook re-anchors) and a leftover of an earlier batch shrinks it.

// newDayEnv is newSvcEnv with the plate service's clock at svcToday, so the
// R1 re-anchor may move a cook within the fixture week.
func newDayEnv(t *testing.T) *svcEnv {
	t.Helper()
	e := newSvcEnv(t)
	plates, avail := testsvc.PlanAt(e.db, nil, func() time.Time { return day(svcToday) })
	e.plates, e.avail = plates, avail
	e.deps.Plates, e.deps.Cells, e.deps.Overrides = plates, avail, avail
	e.svc = planning.NewService(e.deps)
	return e
}

// dayShape is a generated day with a batch cooked on it that a later day
// eats (cook) and a plate eating a batch cooked on an earlier day (left).
type dayShape struct {
	date               string
	cook, left         plate.Plate
	cookBatch, leftBat *plate.SessionView
}

func findDayShape(t *testing.T, e *svcEnv, plates map[string]plate.Plate) dayShape {
	t.Helper()
	for _, date := range []string{"2026-11-08", "2026-11-09", tue, "2026-11-11", "2026-11-12"} {
		sh := dayShape{date: date}
		for _, p := range plates {
			if p.DateString() != date || p.State != plate.StateDraft {
				continue
			}
			for _, c := range p.Components {
				if c.CookSessionID == nil {
					continue
				}
				v, err := e.plates.Session(e.ctx, *c.CookSessionID)
				require.NoError(t, err)
				later := slices.ContainsFunc(v.Links, func(l plate.Link) bool { return l.Cell.Date.After(p.Date) })
				switch {
				case v.CookCell().Equal(p.Cell()) && later && sh.cookBatch == nil:
					sh.cook, sh.cookBatch = p, v
				case v.CookDate.Before(p.Date) && sh.leftBat == nil:
					sh.left, sh.leftBat = p, v
				}
			}
		}
		if sh.cookBatch != nil && sh.leftBat != nil {
			return sh
		}
	}
	t.Fatal("the generated week has a day that cooks for a later day and eats an earlier batch")
	return dayShape{}
}

func finished(t *testing.T, e *svcEnv, id int64) planning.RunEnd {
	t.Helper()
	end := e.runs.end(id)
	require.Contains(t, []airun.Status{airun.StatusOK, airun.StatusPartial}, end.Status, "error key %q", end.ErrorKey)
	return end
}

func endScope(t *testing.T, end planning.RunEnd) (moved []plate.AlsoChanged, undo *plate.Undo) {
	t.Helper()
	var sc struct {
		AlsoChanged []plate.AlsoChanged `json:"also_changed"`
		Undo        *plate.Undo         `json:"undo"`
	}
	require.NoError(t, json.Unmarshal([]byte(end.ScopeJSON), &sc))
	return sc.AlsoChanged, sc.Undo
}

func TestService_DayScopeStaysInsideTheDay(t *testing.T) {
	e := newDayEnv(t)
	e.ok(e.generate(e.req()))
	before := e.window()
	sh := findDayShape(t, e, before)
	d := day(sh.date)

	pv, err := e.svc.PreviewDay(e.ctx, e.start, day(svcToday), d)
	require.NoError(t, err)
	assert.Equal(t, before, e.window(), "the preview writes nothing")
	assert.Contains(t, pv.Replaced, sh.cook.ID)
	assert.Contains(t, pv.Replaced, sh.left.ID)
	assert.Empty(t, pv.ConfirmedAffected)

	id := e.regenerate(planning.Scope{Kind: planning.ScopeDay, Date: d})
	end := finished(t, e, id)

	after := e.window()
	for _, k := range regenerated(before, after) {
		assert.True(t, strings.HasPrefix(k, sh.date+"/"), "%s is outside %s", k, sh.date)
	}

	// The batch cooked on the day keeps its later meals: same session, same
	// dish, the cook moved to its earliest remaining meal.
	moved, err := e.plates.Session(e.ctx, sh.cookBatch.ID)
	require.NoError(t, err, "the batch survives")
	assert.True(t, moved.CookDate.After(d), "the cook moved off %s to %s", sh.date, moved.CookDate)
	for _, l := range sh.cookBatch.Links {
		if l.Cell.Date.After(d) {
			assert.True(t, slices.ContainsFunc(moved.Links, func(m plate.Link) bool { return m.PlateID == l.PlateID }), "plate %d keeps its dish", l.PlateID)
		}
	}

	// The earlier batch the day ate from shrinks (D11).
	shrunk, err := e.plates.Session(e.ctx, sh.leftBat.ID)
	require.NoError(t, err)
	assert.Less(t, shrunk.Portions, sh.leftBat.Portions)

	// Recorded: scope_json, a run note, and the preview said the same.
	got, undo := endScope(t, end)
	require.NotEmpty(t, got)
	assert.True(t, slices.ContainsFunc(got, func(a plate.AlsoChanged) bool {
		return a.SessionID == sh.cookBatch.ID && a.What == "cook_moved"
	}))
	assert.Equal(t, pv.AlsoChanged, got, "the preview equals what the run did")
	assert.Contains(t, end.Notes, "now cooked")
	require.NotNil(t, undo)
	assert.Positive(t, undo.FirstEventID)

	v, err := e.svc.Run(e.ctx, id)
	require.NoError(t, err)
	assert.Equal(t, undo, v.Undo, "GET /api/plan/runs/{id} carries the undo")
}

func TestService_DayScopeKeepsAConfirmedPlate(t *testing.T) {
	e := newDayEnv(t)
	e.ok(e.generate(e.req()))
	sh := findDayShape(t, e, e.window())
	_, err := e.plates.Confirm(e.ctx, plate.RowSet{PlateIDs: []int64{sh.left.ID}})
	require.NoError(t, err)
	before := e.window()
	k := key(sh.date, sh.left.SlotID)
	require.Equal(t, plate.StateConfirmed, before[k].State)

	pv, err := e.svc.PreviewDay(e.ctx, e.start, day(svcToday), day(sh.date))
	require.NoError(t, err)
	assert.NotContains(t, pv.Replaced, sh.left.ID)

	id := e.regenerate(planning.Scope{Kind: planning.ScopeDay, Date: day(sh.date)})
	finished(t, e, id)

	after := e.window()
	assert.Equal(t, before[k].ID, after[k].ID)
	assert.Equal(t, before[k].Components, after[k].Components, "the confirmed plate is untouched")
	assert.NotContains(t, idsOf(e.scope(id)["plate_ids"]), sh.left.ID)
}

func TestService_DayScopeFailedRunChangesNothing(t *testing.T) {
	e := newDayEnv(t)
	e.ok(e.generate(e.req()))
	sh := findDayShape(t, e, e.window())
	before := e.window()
	sessions := func() []plate.CookSession {
		ss, _, err := e.deps.Sessions.ListInRange(e.ctx, e.start, e.start.AddDate(0, 0, 6))
		require.NoError(t, err)
		return ss
	}
	beforeSessions := sessions()
	e.deps.Choosers = map[airun.Chooser]planning.Chooser{airun.ChooserHeuristic: failingChooser{err: errors.New("chooser failed")}}
	e.svc = planning.NewService(e.deps)

	id := e.regenerate(planning.Scope{Kind: planning.ScopeDay, Date: day(sh.date)})

	end := e.runs.end(id)
	assert.Equal(t, airun.StatusError, end.Status)
	assert.Equal(t, before, e.window(), "no plate changed")
	assert.Equal(t, beforeSessions, sessions(), "no batch moved or shrank")
	v, err := e.svc.Run(e.ctx, id)
	require.NoError(t, err)
	assert.Nil(t, v.Undo, "a failed run has no undo")
}

func TestService_DayPreviewRejectsDaysOutsideTheWindow(t *testing.T) {
	e := newDayEnv(t)
	for name, d := range map[string]string{"past": "2026-11-05", "after the window": "2026-11-14"} {
		_, err := e.svc.PreviewDay(e.ctx, e.start, day(svcToday), day(d))
		assert.ErrorIs(t, err, domain.ErrInvalidScope, name)
	}
	_, err := e.svc.PreviewDay(e.ctx, day("2026-11-08"), day(svcToday), day(tue))
	assert.ErrorIs(t, err, domain.ErrInvalidPlanStart)
}

// A regenerate's save is one revert token: POST /api/plan/revert restores the
// plan, and a later edit of one of its rows makes it stale.
func TestService_RevertARegenerate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		scope func(e *svcEnv, sh dayShape) planning.Scope
	}{
		{"day", func(_ *svcEnv, sh dayShape) planning.Scope {
			return planning.Scope{Kind: planning.ScopeDay, Date: day(sh.date)}
		}},
		{"plate", func(_ *svcEnv, sh dayShape) planning.Scope {
			return planning.Scope{Kind: planning.ScopePlate, PlateID: sh.left.ID}
		}},
	} {
		t.Run(tc.name+" restores", func(t *testing.T) {
			e := newDayEnv(t)
			e.ok(e.generate(e.req()))
			before := e.window()
			sh := findDayShape(t, e, before)
			_, undo := endScope(t, finished(t, e, e.regenerate(tc.scope(e, sh))))
			require.NotNil(t, undo)
			require.NotEqual(t, before, e.window())

			_, err := e.avail.Revert(e.ctx, *undo)
			require.NoError(t, err)

			assert.Equal(t, dishes(before), dishes(e.window()), "the plan is back")
			cook, err := e.plates.Session(e.ctx, sh.cookBatch.ID)
			require.NoError(t, err)
			assert.Equal(t, sh.cookBatch.CookCell(), cook.CookCell(), "the moved cook is back")
		})
		t.Run(tc.name+" stale after an edit", func(t *testing.T) {
			e := newDayEnv(t)
			e.ok(e.generate(e.req()))
			sh := findDayShape(t, e, e.window())
			id := e.regenerate(tc.scope(e, sh))
			_, undo := endScope(t, finished(t, e, id))
			require.NotNil(t, undo)
			var edited bool
			for _, p := range e.window() {
				if p.AIRunID != nil && *p.AIRunID == id {
					_, err := e.plates.Delete(e.ctx, p.ID)
					require.NoError(t, err)
					edited = true
					break
				}
			}
			require.True(t, edited, "the run wrote a plate")

			_, err := e.avail.Revert(e.ctx, *undo)
			assert.ErrorIs(t, err, domain.ErrPlanChangeStale)
		})
	}
}

// dishes maps each cell to its plate id and foods in order: what a revert
// restores (component rows are written back under new ids).
func dishes(plates map[string]plate.Plate) map[string][]int64 {
	out := map[string][]int64{}
	for k, p := range plates {
		ids := []int64{p.ID}
		for _, c := range p.Components {
			ids = append(ids, c.FoodID)
		}
		out[k] = ids
	}
	return out
}
