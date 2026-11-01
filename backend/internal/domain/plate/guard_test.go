package plate_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Phase 4 B2 (D5): under the agent, a write that would change or delete a
// confirmed plate or session fails with ErrRowConfirmed and writes nothing.

// guarded is a confirmed Chili batch (Sun dinner, leftovers Mon lunch and
// dinner) plus a Rice plate at Tue dinner whose state the case picks.
type guarded struct {
	sun   *plate.Plate
	sid   int64
	monL  *plate.Plate
	other *plate.Plate
}

func (e *env) guarded() guarded {
	e.t.Helper()
	sun, sid := e.batch()
	other := e.create(tue, e.dinner, nil, grams(e.rice, 100))
	return guarded{sun: sun, sid: sid, monL: e.plateAt(mon, e.lunch), other: other}
}

// dump is every plate, component, session and event row, to show a refused
// write changed nothing.
func (e *env) dump() string {
	e.t.Helper()
	var b strings.Builder
	for _, q := range []string{
		`SELECT id, date, slot_id, eaters, state, COALESCE(note, '') FROM plates ORDER BY id`,
		`SELECT id, plate_id, food_id, COALESCE(portions, 0), COALESCE(amount, 0), COALESCE(cook_session_id, 0) FROM plate_components ORDER BY id`,
		`SELECT id, cook_date || ' ' || cook_slot_id, portions, frozen_portions, state, COALESCE(cooked_at, '') FROM cook_sessions ORDER BY id`,
		`SELECT COUNT(*), '', '', '', '', '' FROM plate_events`,
	} {
		rows, err := e.db.Query(q)
		require.NoError(e.t, err)
		for rows.Next() {
			v := make([]any, 6)
			ptrs := make([]any, 6)
			for i := range v {
				ptrs[i] = &v[i]
			}
			require.NoError(e.t, rows.Scan(ptrs...))
			fmt.Fprintln(&b, v...)
		}
		require.NoError(e.t, rows.Err())
		require.NoError(e.t, rows.Close())
	}
	return b.String()
}

func TestGuard_AgentCannotWriteConfirmedRows(t *testing.T) {
	cases := []struct {
		name  string
		write func(e *env, ctx context.Context, g guarded) error
	}{
		{"update", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.Update(ctx, g.sun.ID, plate.PlatePatch{Eaters: f64(3)})
			return err
		}},
		{"add component", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.AddComponent(ctx, g.sun.ID, grams(e.rice, 50))
			return err
		}},
		{"swap component", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.SwapComponent(ctx, g.sun.Components[0].ID, e.curry, nil)
			return err
		}},
		{"update component quantity", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.UpdateComponentQuantity(ctx, g.sun.Components[0].ID, plate.PlateComponent{Portions: f64(2)})
			return err
		}},
		{"remove component", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.RemoveComponent(ctx, g.sun.Components[0].ID)
			return err
		}},
		{"delete", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.Delete(ctx, g.sun.ID)
			return err
		}},
		{"set session portions", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.SetSessionPortions(ctx, g.sid, plate.SessionPortions{Portions: 8})
			return err
		}},
		{"set cooked", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.SetCooked(ctx, g.sid, true)
			return err
		}},
		{"add leftovers", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.AddLeftovers(ctx, g.sid, []plate.CellKey{cell(tue, e.lunch)})
			return err
		}},
		{"link a new plate to the confirmed session", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.Create(ctx, plate.PlateInput{Date: day(tue), SlotID: e.lunch, Components: []plate.PlateComponent{linked(e.chili, 1, g.sid)}})
			return err
		}},
		{"delete session", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.DeleteSession(ctx, g.sid)
			return err
		}},
		{"clear range", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.ClearRange(ctx, day(sun), day(sun), 0)
			return err
		}},
		{"remove plates", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.RemovePlates(ctx, plate.Removal{PlateIDs: []int64{g.sun.ID}})
			return err
		}},
		{"apply overwrite", func(e *env, ctx context.Context, g guarded) error {
			_, err := e.svc.Apply(ctx, plate.PlanSnapshot{Plates: []plate.SnapshotPlate{{
				Date: sun, SlotID: e.dinner, Eaters: 2,
				Components: []plate.SnapshotComponent{{FoodID: e.rice, Amount: f64(80), Unit: ptrStr("g")}},
			}}}, plate.ApplyOptions{OnConflict: plate.ConflictOverwrite})
			return err
		}},
		{"draft plate linked to a confirmed session", func(e *env, ctx context.Context, g guarded) error {
			e.exec(`UPDATE plates SET state = 'draft' WHERE id = ?`, g.monL.ID)
			_, err := e.svc.Update(ctx, g.monL.ID, plate.PlatePatch{Eaters: f64(3)})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/agent", func(t *testing.T) {
			e := newEnv(t)
			g := e.guarded()
			agent := e.run(actor.Agent, 8)
			// The draft case flips its plate inside write; take the dump after.
			if tc.name == "draft plate linked to a confirmed session" {
				e.exec(`UPDATE plates SET state = 'draft' WHERE id = ?`, g.monL.ID)
			}
			before := e.dump()
			err := tc.write(e, agent, g)
			require.ErrorIs(t, err, domain.ErrRowConfirmed)
			key, _ := domain.MessageKey(err, "plate")
			assert.Equal(t, "error.plan.row_confirmed", key)
			assert.Equal(t, before, e.dump(), "a refused write changes nothing")
		})
		t.Run(tc.name+"/user", func(t *testing.T) {
			e := newEnv(t)
			g := e.guarded()
			require.NoError(t, tc.write(e, e.ctx, g))
		})
	}
}

func TestGuard_ErrorNamesTheConfirmedRows(t *testing.T) {
	e := newEnv(t)
	g := e.guarded()
	_, err := e.svc.Update(e.run(actor.Agent, 8), g.sun.ID, plate.PlatePatch{Eaters: f64(3)})
	require.ErrorIs(t, err, domain.ErrRowConfirmed)
	assert.Contains(t, err.Error(), fmt.Sprintf("plate %d (%s slot %d: Chili)", g.sun.ID, sun, e.dinner))
}

// The agent still writes drafts and new rows.
func TestGuard_AgentWritesDrafts(t *testing.T) {
	e := newEnv(t)
	agent := e.run(actor.Agent, 8)
	p, err := e.svc.Create(agent, plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
	require.NoError(t, err)
	_, err = e.svc.Update(agent, p.ID, plate.PlatePatch{Eaters: f64(3)})
	require.NoError(t, err)
	_, err = e.svc.AddLeftovers(agent, sessionOf(p, 0), []plate.CellKey{cell(mon, e.lunch)})
	require.NoError(t, err)
	_, err = e.svc.SetSessionPortions(agent, sessionOf(p, 0), plate.SessionPortions{Portions: 9})
	require.NoError(t, err)
	_, err = e.svc.Delete(agent, p.ID)
	require.NoError(t, err)
}

// No plate with remove_plate deletes through plate.Delete, so the guard
// holds for availability too and the override is not written.
func TestGuard_AvailabilityRemovePlate(t *testing.T) {
	e := newEnv(t)
	g := e.guarded()
	agent := e.run(actor.Agent, 8)
	in := availability.OverrideInput{Date: day(sun), SlotID: e.dinner, Mode: availability.ModeNone, RemovePlate: true}
	before := e.dump()
	_, _, err := e.avail.SetOverride(agent, in)
	require.ErrorIs(t, err, domain.ErrRowConfirmed)
	assert.Equal(t, before, e.dump())
	assert.Equal(t, 0, e.count("availability_overrides"))

	e.exec(`UPDATE plates SET state = 'draft' WHERE id = ?`, g.other.ID)
	in.Date = day(tue)
	_, removed, err := e.avail.SetOverride(agent, in)
	require.NoError(t, err, "a draft plate goes")
	require.NotNil(t, removed)
	assert.Nil(t, e.plateAt(tue, e.dinner))

	in.Date = day(sun)
	_, _, err = e.avail.SetOverride(e.ctx, in)
	require.NoError(t, err, "the user may")
}

// B2-AC2: Apply's overwrite onto a confirmed cell fails under the agent;
// onto a draft cell it replaces the draft.
func TestGuard_ApplyOverwrite(t *testing.T) {
	e := newEnv(t)
	agent := e.run(actor.Agent, 8)
	e.create(sun, e.dinner, nil, composed(e.chili, 1))
	draft, err := e.svc.Create(agent, plate.PlateInput{Date: day(mon), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.curry, 1)}})
	require.NoError(t, err)
	snap := func(date string) plate.PlanSnapshot {
		return plate.PlanSnapshot{Plates: []plate.SnapshotPlate{{
			Date: date, SlotID: e.dinner, Eaters: 2,
			Components: []plate.SnapshotComponent{{FoodID: e.rice, Amount: f64(80), Unit: ptrStr("g")}},
		}}}
	}

	_, err = e.svc.Apply(agent, snap(sun), plate.ApplyOptions{OnConflict: plate.ConflictOverwrite})
	require.ErrorIs(t, err, domain.ErrRowConfirmed)

	res, err := e.svc.Apply(agent, snap(mon), plate.ApplyOptions{OnConflict: plate.ConflictOverwrite})
	require.NoError(t, err)
	require.Len(t, res.Removed.Plates, 1)
	got := e.plateAt(mon, e.dinner)
	require.NotNil(t, got)
	assert.NotEqual(t, draft.ID, got.ID)
	assert.Equal(t, e.rice, got.Components[0].FoodID)
	assert.Equal(t, plate.StateDraft, got.State)
}
