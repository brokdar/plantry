package planning_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// buildWeek returns the fixture week's state and a function that builds it
// again with the same Builder (the memo), inside a unit of work.
func buildWeek(t *testing.T) (*svcEnv, func(ctx context.Context) planning.State) {
	t.Helper()
	e := newSvcEnv(t)
	e.exec(`UPDATE user_profile SET kcal_target = 2200, weight_kg = 75 WHERE id = 1`)
	e.generate(e.req())
	e.exec(`INSERT INTO plan_light (date, scope, slot) VALUES (?, 'meal', 'dinner')`, wed)
	start, today := day(sat), day(svcToday)
	board, err := testsvc.PlanView(e.db).Plan(e.ctx, &start, today)
	require.NoError(t, err)
	build := func(ctx context.Context) planning.State {
		t.Helper()
		var st planning.State
		require.NoError(t, e.deps.Tx.InTx(ctx, func(ctx context.Context) error {
			st, err = e.deps.Builder.Build(ctx, board.Window, today, board.Window.Start, board.Window.End)
			return err
		}))
		return st
	}
	return e, build
}

// noAlias fails when a and b share a map, a non-empty slice or a pointer
// anywhere: a memoized State goes to callers that change it.
func noAlias(t *testing.T, a, b reflect.Value, path string) {
	t.Helper()
	switch a.Kind() {
	case reflect.Pointer:
		if a.IsNil() || b.IsNil() {
			return
		}
		assert.NotEqual(t, a.Pointer(), b.Pointer(), "%s shares a pointer", path)
		noAlias(t, a.Elem(), b.Elem(), path)
	case reflect.Map:
		if a.IsNil() || b.IsNil() {
			return
		}
		assert.NotEqual(t, a.Pointer(), b.Pointer(), "%s shares a map", path)
		for _, k := range a.MapKeys() {
			noAlias(t, a.MapIndex(k), b.MapIndex(k), path+"[k]")
		}
	case reflect.Slice:
		if a.Len() == 0 || b.Len() == 0 {
			return
		}
		assert.NotEqual(t, a.Pointer(), b.Pointer(), "%s shares a slice", path)
		for i := range min(a.Len(), b.Len()) {
			noAlias(t, a.Index(i), b.Index(i), path+"[i]")
		}
	case reflect.Struct:
		for i := range a.NumField() {
			if a.Type().Field(i).IsExported() {
				noAlias(t, a.Field(i), b.Field(i), path+"."+a.Type().Field(i).Name)
			}
		}
	case reflect.Interface:
		if !a.IsNil() && !b.IsNil() {
			noAlias(t, a.Elem(), b.Elem(), path)
		}
	}
}

// Build reuses the State it built while no row changed, as a copy that
// shares nothing: a caller that changes its copy does not change the next.
func TestBuild_MemoCopiesShareNothing(t *testing.T) {
	e, build := buildWeek(t)
	first := build(e.ctx)
	second := build(e.ctx) // from the memo
	require.Equal(t, first, second)
	require.NotEmpty(t, first.Foods)
	require.NotEmpty(t, first.Plates)
	noAlias(t, reflect.ValueOf(first), reflect.ValueOf(second), "State")

	for id, f := range second.Foods {
		f.Name = "changed"
		second.Foods[id] = f
	}
	second.Foods[-1] = planning.FoodFacts{ID: -1}
	second.Plates[0].Components = append(second.Plates[0].Components, planning.Component{FoodID: -1})
	assert.Equal(t, first, build(e.ctx), "a caller's change does not reach the next Build")
}

// Every write the connection makes, inside a unit of work too, is seen by
// the next Build, and a rolled-back write is gone again.
func TestBuild_MemoSeesEveryWrite(t *testing.T) {
	e, build := buildWeek(t)
	before := build(e.ctx)
	require.NotNil(t, before.Profile.KcalTarget)

	e.exec(`UPDATE user_profile SET kcal_target = 2400 WHERE id = 1`)
	assert.InDelta(t, 2400, *build(e.ctx).Profile.KcalTarget, 0.1, "a committed write")

	set := testsvc.Settings(e.db)
	errRollback := errors.New("rollback")
	err := e.deps.Tx.InTx(e.ctx, func(ctx context.Context) error {
		require.NoError(t, set.Set(ctx, "plan.max_cook_occasions_per_day", "3")) // the fixture has 1
		assert.Equal(t, 3, build(ctx).Settings.MaxCookOccasionsPerDay, "a write inside the unit of work")
		return errRollback
	})
	require.ErrorIs(t, err, errRollback)
	assert.Equal(t, 1, build(e.ctx).Settings.MaxCookOccasionsPerDay, "the rolled-back write is gone")
}
