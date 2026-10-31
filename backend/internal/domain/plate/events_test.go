package plate_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// events returns the audit trail written so far.
func (e *env) events() []plate.Event {
	e.t.Helper()
	evs, err := sqlite.NewPlateEventRepo(e.db).List(e.ctx)
	require.NoError(e.t, err)
	return evs
}

// eventsSince returns the events written after the first n.
func (e *env) eventsSince(n int) []plate.Event {
	e.t.Helper()
	return e.events()[n:]
}

func actions(evs []plate.Event) []plate.Action {
	out := make([]plate.Action, len(evs))
	for i, ev := range evs {
		out[i] = ev.Action
	}
	return out
}

func decodePlate(t *testing.T, raw json.RawMessage) plate.SnapshotPlate {
	t.Helper()
	require.NotNil(t, raw)
	var sp plate.SnapshotPlate
	require.NoError(t, json.Unmarshal(raw, &sp))
	return sp
}

func TestEvents_CreateWritesFullAfterSnapshot(t *testing.T) {
	e := newEnv(t)
	p := e.create(mon, e.dinner, nil, composed(e.chili, 1), grams(e.rice, 150))

	evs := e.events()
	require.Len(t, evs, 1)
	ev := evs[0]
	assert.Equal(t, plate.ActionCreate, ev.Action)
	assert.Equal(t, actor.User, ev.Actor)
	assert.Nil(t, ev.AIRunID)
	require.NotNil(t, ev.PlateID)
	assert.Equal(t, p.ID, *ev.PlateID)
	assert.Nil(t, ev.CookSessionID)
	assert.Equal(t, mon, ev.Date.Format("2006-01-02"))
	assert.Equal(t, e.dinner, ev.SlotID)
	assert.Nil(t, ev.Before)

	after := decodePlate(t, ev.After)
	require.NotNil(t, after.PlateID)
	assert.Equal(t, p.ID, *after.PlateID)
	assert.Equal(t, 2.0, after.Eaters)
	require.Len(t, after.Components, 2)
	assert.Equal(t, e.chili, after.Components[0].FoodID)
	require.NotNil(t, after.Components[0].Session)
	assert.Equal(t, sessionOf(p, 0), after.Components[0].Session.ID)
	assert.Equal(t, e.rice, after.Components[1].FoodID)
	assert.Equal(t, 150.0, *after.Components[1].Grams)
}

func TestEvents_UpdateCarriesBeforeAndAfter(t *testing.T) {
	e := newEnv(t)
	p := e.create(mon, e.dinner, nil, grams(e.rice, 100))
	_, err := e.svc.Update(e.ctx, p.ID, plate.PlatePatch{Eaters: f64(3)})
	require.NoError(t, err)

	evs := e.eventsSince(1)
	require.Len(t, evs, 1)
	assert.Equal(t, plate.ActionUpdate, evs[0].Action)
	assert.Equal(t, 2.0, decodePlate(t, evs[0].Before).Eaters)
	assert.Equal(t, 3.0, decodePlate(t, evs[0].After).Eaters)
}

func TestEvents_DeleteHasNullAfter(t *testing.T) {
	e := newEnv(t)
	p := e.create(mon, e.dinner, nil, grams(e.rice, 100))
	_, err := e.svc.Delete(e.ctx, p.ID)
	require.NoError(t, err)

	evs := e.eventsSince(1)
	require.Len(t, evs, 1)
	assert.Equal(t, plate.ActionDelete, evs[0].Action)
	assert.Equal(t, p.ID, *evs[0].PlateID)
	assert.Nil(t, evs[0].After)
	assert.Len(t, decodePlate(t, evs[0].Before).Components, 1)
}

func TestEvents_ActorAndRunFromContext(t *testing.T) {
	e := newEnv(t)
	ctx := actor.With(e.ctx, actor.Agent, 42)
	_, err := e.svc.Create(ctx, plate.PlateInput{Date: day(mon), SlotID: e.dinner, Components: []plate.PlateComponent{grams(e.rice, 100)}})
	require.NoError(t, err)

	evs := e.events()
	require.Len(t, evs, 1)
	assert.Equal(t, actor.Agent, evs[0].Actor)
	require.NotNil(t, evs[0].AIRunID)
	assert.Equal(t, int64(42), *evs[0].AIRunID)
}

// Every exported mutator writes at least one event (D20). A new mutator must
// be added here.
func TestEvents_EveryMutatorWrites(t *testing.T) {
	tests := []struct {
		name string
		run  func(e *env) error
		want []plate.Action
	}{
		{"Create", func(e *env) error {
			_, err := e.svc.Create(e.ctx, plate.PlateInput{Date: day(tue), SlotID: e.lunch, Components: []plate.PlateComponent{grams(e.rice, 1)}})
			return err
		}, []plate.Action{plate.ActionCreate}},
		{"Update", func(e *env) error {
			_, err := e.svc.Update(e.ctx, e.plateAt(sun, e.dinner).ID, plate.PlatePatch{Date: ptrDay(tue)})
			return err
		}, []plate.Action{plate.ActionUpdate}},
		{"Delete", func(e *env) error {
			_, err := e.svc.Delete(e.ctx, e.plateAt(mon, e.dinner).ID)
			return err
		}, []plate.Action{plate.ActionDelete}},
		{"ClearRange", func(e *env) error {
			_, err := e.svc.ClearRange(e.ctx, day(mon), day(mon))
			return err
		}, []plate.Action{plate.ActionDelete, plate.ActionDelete}},
		{"AddComponent", func(e *env) error {
			_, err := e.svc.AddComponent(e.ctx, e.plateAt(sun, e.dinner).ID, grams(e.rice, 50))
			return err
		}, []plate.Action{plate.ActionUpdate}},
		{"UpdateComponentQuantity", func(e *env) error {
			_, err := e.svc.UpdateComponentQuantity(e.ctx, e.plateAt(sun, e.dinner).Components[0].ID, plate.PlateComponent{Portions: f64(2)})
			return err
		}, []plate.Action{plate.ActionUpdate}},
		{"SwapComponent", func(e *env) error {
			_, err := e.svc.SwapComponent(e.ctx, e.plateAt(mon, e.dinner).Components[0].ID, e.curry, nil)
			return err
		}, []plate.Action{plate.ActionUpdate}},
		{"RemoveComponent", func(e *env) error {
			_, err := e.svc.RemoveComponent(e.ctx, e.plateAt(mon, e.dinner).Components[0].ID)
			return err
		}, []plate.Action{plate.ActionUpdate}},
		{"SetSessionPortions", func(e *env) error {
			_, err := e.svc.SetSessionPortions(e.ctx, sessionOf(e.plateAt(sun, e.dinner), 0), plate.SessionPortions{Portions: 8})
			return err
		}, []plate.Action{plate.ActionUpdate, plate.ActionUpdate, plate.ActionUpdate, plate.ActionUpdate}},
		{"SetCooked", func(e *env) error {
			_, err := e.svc.SetCooked(e.ctx, sessionOf(e.plateAt(sun, e.dinner), 0), true)
			return err
		}, []plate.Action{plate.ActionUpdate, plate.ActionUpdate, plate.ActionUpdate, plate.ActionUpdate}},
		{"AddLeftovers", func(e *env) error {
			_, err := e.svc.AddLeftovers(e.ctx, sessionOf(e.plateAt(sun, e.dinner), 0), []plate.CellKey{cell(tue, lunch)})
			return err
		}, []plate.Action{plate.ActionCreate, plate.ActionUpdate}},
		{"DeleteSession", func(e *env) error {
			_, err := e.svc.DeleteSession(e.ctx, sessionOf(e.plateAt(sun, e.dinner), 0))
			return err
		}, []plate.Action{plate.ActionDelete, plate.ActionDelete, plate.ActionDelete, plate.ActionDelete}},
		{"Apply", func(e *env) error {
			_, err := e.svc.Apply(e.ctx, plate.PlanSnapshot{Plates: []plate.SnapshotPlate{{
				Date: tue, SlotID: e.lunch, Eaters: 2,
				Components: []plate.SnapshotComponent{{FoodID: e.rice, Amount: f64(10), Unit: strp("g")}},
			}}}, plate.ApplyOptions{OnConflict: plate.ConflictFail})
			return err
		}, []plate.Action{plate.ActionCreate}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.batch()
			n := len(e.events())
			require.NoError(t, tt.run(e))
			assert.Equal(t, tt.want, actions(e.eventsSince(n)))
		})
	}
}

// B3-AC3: two leftovers write two create events plus one session event, in
// one transaction.
func TestEvents_AddLeftovers(t *testing.T) {
	e := newEnv(t)
	sunPlate := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	sid := sessionOf(sunPlate, 0)
	_, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(mon, lunch), cell(mon, dinner)})
	require.NoError(t, err)

	evs := e.eventsSince(1)
	require.Len(t, evs, 3)
	for _, ev := range evs[:2] {
		assert.Equal(t, plate.ActionCreate, ev.Action)
		require.NotNil(t, ev.PlateID)
		require.NotNil(t, ev.CookSessionID)
		assert.Equal(t, sid, *ev.CookSessionID)
		assert.Nil(t, ev.Before)
	}
	assert.Equal(t, e.plateAt(mon, e.lunch).ID, *evs[0].PlateID)
	assert.Equal(t, e.plateAt(mon, e.dinner).ID, *evs[1].PlateID)

	sess := evs[2]
	assert.Nil(t, sess.PlateID)
	assert.Equal(t, plate.ActionUpdate, sess.Action)
	assert.Equal(t, sid, *sess.CookSessionID)
	assert.Equal(t, sun, sess.Date.Format("2006-01-02"))
	assert.Equal(t, e.dinner, sess.SlotID)
	var before, after map[string]any
	require.NoError(t, json.Unmarshal(sess.Before, &before))
	require.NoError(t, json.Unmarshal(sess.After, &after))
	assert.EqualValues(t, sid, before["id"])
	assert.NotContains(t, before, "ref")
	assert.EqualValues(t, 2, before["portions"])
	assert.EqualValues(t, 6, after["portions"])
}

func TestEvents_AddLeftoversFailureWritesNone(t *testing.T) {
	e := newEnv(t)
	sunPlate := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	e.create(mon, e.dinner, nil, grams(e.rice, 100)) // the second cell is taken
	n := len(e.events())

	_, err := e.svc.AddLeftovers(e.ctx, sessionOf(sunPlate, 0), []plate.CellKey{cell(mon, lunch), cell(mon, dinner)})
	require.Error(t, err)
	assert.Empty(t, e.eventsSince(n))
	assert.Equal(t, 2, e.count("plates"))
}

// B3-AC4: restore writes restore events with a null before.
func TestEvents_RestoreWritesRestore(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()
	snap, err := e.svc.DeleteSession(e.ctx, sid)
	require.NoError(t, err)
	n := len(e.events())

	_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
	require.NoError(t, err)
	evs := e.eventsSince(n)
	require.Len(t, evs, 3)
	for _, ev := range evs {
		assert.Equal(t, plate.ActionRestore, ev.Action)
		assert.Nil(t, ev.Before)
		assert.NotNil(t, ev.After)
	}
}

// B3-AC4: DeleteSession deletes plates left empty, updates plates that keep
// other components and closes the session.
func TestEvents_DeleteSession(t *testing.T) {
	e := newEnv(t)
	sunPlate := e.create(sun, e.dinner, nil, composed(e.chili, 1), grams(e.rice, 100))
	sid := sessionOf(sunPlate, 0)
	_, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(mon, lunch)})
	require.NoError(t, err)
	monPlate := e.plateAt(mon, e.lunch)
	n := len(e.events())

	_, err = e.svc.DeleteSession(e.ctx, sid)
	require.NoError(t, err)
	evs := e.eventsSince(n)
	require.Len(t, evs, 3)

	byPlate := map[int64]plate.Event{}
	var sessEv *plate.Event
	for i, ev := range evs {
		assert.Equal(t, sid, *ev.CookSessionID)
		if ev.PlateID == nil {
			sessEv = &evs[i]
			continue
		}
		byPlate[*ev.PlateID] = ev
	}
	kept := byPlate[sunPlate.ID]
	assert.Equal(t, plate.ActionUpdate, kept.Action)
	assert.Len(t, decodePlate(t, kept.Before).Components, 2)
	assert.Len(t, decodePlate(t, kept.After).Components, 1)
	gone := byPlate[monPlate.ID]
	assert.Equal(t, plate.ActionDelete, gone.Action)
	assert.Nil(t, gone.After)
	require.NotNil(t, sessEv)
	assert.Equal(t, plate.ActionDelete, sessEv.Action)
	assert.Nil(t, sessEv.After)
	assert.NotNil(t, sessEv.Before)
}

// B3-AC5: a plate removed by marking its cell No plate keeps its events.
func TestEvents_AvailabilityRemovePlate(t *testing.T) {
	e := newEnv(t)
	p := e.create(mon, e.dinner, nil, grams(e.rice, 100))
	_, removed, err := e.avail.SetOverride(e.ctx, availability.OverrideInput{Date: day(mon), SlotID: e.dinner, Mode: availability.ModeNone, RemovePlate: true})
	require.NoError(t, err)
	require.NotNil(t, removed)

	evs := e.events()
	require.Len(t, evs, 2)
	assert.Equal(t, []plate.Action{plate.ActionCreate, plate.ActionDelete}, actions(evs))
	for _, ev := range evs {
		assert.Equal(t, p.ID, *ev.PlateID)
	}
	assert.Zero(t, e.count("plates"))
}

// Overwrite logs the replaced plate's delete and the new plate's create.
func TestEvents_ApplyOverwrite(t *testing.T) {
	e := newEnv(t)
	old := e.create(mon, e.dinner, nil, grams(e.rice, 100))
	n := len(e.events())
	res, err := e.svc.Apply(e.ctx, plate.PlanSnapshot{Plates: []plate.SnapshotPlate{{
		Date: mon, SlotID: e.dinner, Eaters: 2,
		Components: []plate.SnapshotComponent{{FoodID: e.eg, Amount: f64(50), Unit: strp("g")}},
	}}}, plate.ApplyOptions{OnConflict: plate.ConflictOverwrite})
	require.NoError(t, err)

	evs := e.eventsSince(n)
	require.Len(t, evs, 2)
	assert.Equal(t, plate.ActionDelete, evs[0].Action)
	assert.Equal(t, old.ID, *evs[0].PlateID)
	assert.Equal(t, plate.ActionCreate, evs[1].Action)
	assert.Equal(t, res.PlateIDs[0], *evs[1].PlateID)
}

func TestEvents_AtUsesClock(t *testing.T) {
	e := newEnv(t)
	before := time.Now().UTC().Add(-time.Second)
	e.create(mon, e.dinner, nil, grams(e.rice, 100))
	at := e.events()[0].At
	assert.False(t, at.Before(before.Truncate(time.Second)))
	assert.False(t, at.After(time.Now().UTC().Add(time.Second)))
}

func ptrDay(s string) *time.Time {
	d := day(s)
	return &d
}

func strp(s string) *string { return &s }
