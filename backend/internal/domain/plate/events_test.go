package plate_test

import (
	"context"
	"encoding/json"
	"errors"
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

// kinds lists each event as "plate:<action>" or "session:<action>".
func kinds(evs []plate.Event) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		subject := "plate"
		if ev.PlateID == nil {
			subject = "session"
		}
		out[i] = subject + ":" + string(ev.Action)
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

func decodeSession(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	require.NotNil(t, raw)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func TestEvents_CreateWritesFullAfterSnapshot(t *testing.T) {
	e := newEnv(t)
	p := e.create(mon, e.dinner, nil, composed(e.chili, 1), grams(e.rice, 150))
	sid := sessionOf(p, 0)

	evs := e.events()
	require.Equal(t, []string{"plate:create", "session:create"}, kinds(evs))
	ev := evs[0]
	assert.Equal(t, actor.User, ev.Actor)
	assert.Nil(t, ev.AIRunID)
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
	assert.Equal(t, sid, after.Components[0].Session.ID)
	assert.Equal(t, e.rice, after.Components[1].FoodID)
	assert.Equal(t, 150.0, *after.Components[1].Grams)

	// The fresh session (D8) is created with the plate.
	sess := evs[1]
	assert.Equal(t, sid, *sess.CookSessionID)
	assert.Nil(t, sess.Before)
	assert.EqualValues(t, 2, decodeSession(t, sess.After)["portions"])
	assert.Equal(t, mon, sess.Date.Format("2006-01-02"))
}

func TestEvents_UpdateCarriesBeforeAndAfter(t *testing.T) {
	e := newEnv(t)
	p := e.create(mon, e.dinner, nil, grams(e.rice, 100))
	_, err := e.svc.Update(e.ctx, p.ID, plate.PlatePatch{Eaters: f64(3)})
	require.NoError(t, err)

	evs := e.eventsSince(1)
	require.Equal(t, []string{"plate:update"}, kinds(evs))
	assert.Equal(t, 2.0, decodePlate(t, evs[0].Before).Eaters)
	assert.Equal(t, 3.0, decodePlate(t, evs[0].After).Eaters)
}

func TestEvents_DeleteHasNullAfter(t *testing.T) {
	e := newEnv(t)
	p := e.create(mon, e.dinner, nil, grams(e.rice, 100))
	_, err := e.svc.Delete(e.ctx, p.ID)
	require.NoError(t, err)

	evs := e.eventsSince(1)
	require.Equal(t, []string{"plate:delete"}, kinds(evs))
	assert.Equal(t, p.ID, *evs[0].PlateID)
	assert.Nil(t, evs[0].After)
	assert.Len(t, decodePlate(t, evs[0].Before).Components, 1)
}

func TestEvents_ActorAndRunFromContext(t *testing.T) {
	e := newEnv(t)
	ctx := actor.With(e.ctx, actor.Agent, 42)
	_, err := e.svc.Create(ctx, plate.PlateInput{Date: day(mon), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
	require.NoError(t, err)

	evs := e.events()
	require.Len(t, evs, 2)
	for _, ev := range evs {
		assert.Equal(t, actor.Agent, ev.Actor)
		require.NotNil(t, ev.AIRunID)
		assert.Equal(t, int64(42), *ev.AIRunID)
	}
}

// Every exported mutator writes at least one event (D20). A new mutator must
// be added here. Setup: the batch (Sun dinner Chili, leftovers Mon lunch and
// Mon dinner, an exact session of 6).
func TestEvents_EveryMutatorWrites(t *testing.T) {
	tests := []struct {
		name string
		run  func(e *env) error
		want []string
	}{
		{"Create", func(e *env) error {
			_, err := e.svc.Create(e.ctx, plate.PlateInput{Date: day(tue), SlotID: e.lunch, Components: []plate.PlateComponent{grams(e.rice, 1)}})
			return err
		}, []string{"plate:create"}},
		{"Update moves the cook plate", func(e *env) error {
			_, err := e.svc.Update(e.ctx, e.plateAt(sun, e.dinner).ID, plate.PlatePatch{Date: ptrDay(tue)})
			return err
		}, []string{"plate:update", "session:update"}},
		{"Delete shrinks the exact session", func(e *env) error {
			_, err := e.svc.Delete(e.ctx, e.plateAt(mon, e.dinner).ID)
			return err
		}, []string{"plate:delete", "session:update"}},
		{"ClearRange", func(e *env) error {
			_, err := e.svc.ClearRange(e.ctx, day(mon), day(mon))
			return err
		}, []string{"plate:delete", "plate:delete", "session:update"}},
		{"AddComponent of a leaf", func(e *env) error {
			_, err := e.svc.AddComponent(e.ctx, e.plateAt(sun, e.dinner).ID, grams(e.rice, 50))
			return err
		}, []string{"plate:update"}},
		{"UpdateComponentQuantity grows the session", func(e *env) error {
			_, err := e.svc.UpdateComponentQuantity(e.ctx, e.plateAt(sun, e.dinner).Components[0].ID, plate.PlateComponent{Portions: f64(2)})
			return err
		}, []string{"plate:update", "session:update"}},
		{"SwapComponent to a new recipe", func(e *env) error {
			_, err := e.svc.SwapComponent(e.ctx, e.plateAt(mon, e.dinner).Components[0].ID, e.curry, nil)
			return err
		}, []string{"plate:update", "session:create", "session:update"}},
		{"RemoveComponent", func(e *env) error {
			_, err := e.svc.RemoveComponent(e.ctx, e.plateAt(mon, e.dinner).Components[0].ID)
			return err
		}, []string{"plate:update", "session:update"}},
		{"SetSessionPortions", func(e *env) error {
			_, err := e.svc.SetSessionPortions(e.ctx, sessionOf(e.plateAt(sun, e.dinner), 0), plate.SessionPortions{Portions: 8})
			return err
		}, []string{"plate:update", "plate:update", "plate:update", "session:update"}},
		{"SetCooked", func(e *env) error {
			_, err := e.svc.SetCooked(e.ctx, sessionOf(e.plateAt(sun, e.dinner), 0), true)
			return err
		}, []string{"plate:update", "plate:update", "plate:update", "session:update"}},
		{"AddLeftovers", func(e *env) error {
			_, err := e.svc.AddLeftovers(e.ctx, sessionOf(e.plateAt(sun, e.dinner), 0), []plate.CellKey{cell(tue, lunch)})
			return err
		}, []string{"plate:create", "session:update"}},
		{"DeleteSession", func(e *env) error {
			_, err := e.svc.DeleteSession(e.ctx, sessionOf(e.plateAt(sun, e.dinner), 0))
			return err
		}, []string{"plate:delete", "plate:delete", "plate:delete", "session:delete"}},
		{"Apply", func(e *env) error {
			_, err := e.svc.Apply(e.ctx, plate.PlanSnapshot{Plates: []plate.SnapshotPlate{{
				Date: tue, SlotID: e.lunch, Eaters: 2,
				Components: []plate.SnapshotComponent{{FoodID: e.rice, Amount: f64(10), Unit: strp("g")}},
			}}}, plate.ApplyOptions{OnConflict: plate.ConflictFail})
			return err
		}, []string{"plate:create"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t)
			e.batch()
			n := len(e.events())
			require.NoError(t, tt.run(e))
			assert.Equal(t, tt.want, kinds(e.eventsSince(n)))
		})
	}
}

// D11 side effects are in the trail: a plate edit that resizes the exact
// session writes the session's update.
func TestEvents_UpdateEatersResizesSession(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	n := len(e.events())
	_, err := e.svc.Update(e.ctx, p.ID, plate.PlatePatch{Eaters: f64(3)})
	require.NoError(t, err)

	evs := e.eventsSince(n)
	require.Equal(t, []string{"plate:update", "session:update"}, kinds(evs))
	assert.Nil(t, evs[0].CookSessionID, "a plate edit is not a session edit")
	assert.Equal(t, sessionOf(p, 0), *evs[1].CookSessionID)
	assert.EqualValues(t, 2, decodeSession(t, evs[1].Before)["portions"])
	assert.EqualValues(t, 3, decodeSession(t, evs[1].After)["portions"])
}

// Removing a session's only component deletes the session (D11).
func TestEvents_RemoveLastComponentDeletesSession(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, nil, composed(e.chili, 1), grams(e.rice, 100))
	n := len(e.events())
	_, err := e.svc.RemoveComponent(e.ctx, p.Components[0].ID)
	require.NoError(t, err)

	evs := e.eventsSince(n)
	require.Equal(t, []string{"plate:update", "session:delete"}, kinds(evs))
	assert.Equal(t, sessionOf(p, 0), *evs[1].CookSessionID)
	assert.NotNil(t, evs[1].Before)
	assert.Nil(t, evs[1].After)
	assert.Equal(t, sun, evs[1].Date.Format("2006-01-02"))
}

// Moving the cook plate moves the session's cook cell (D13).
func TestEvents_MoveCookPlateMovesSession(t *testing.T) {
	e := newEnv(t)
	p := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	n := len(e.events())
	_, err := e.svc.Update(e.ctx, p.ID, plate.PlatePatch{Date: ptrDay(tue), SlotID: i64(e.lunch)})
	require.NoError(t, err)

	evs := e.eventsSince(n)
	require.Equal(t, []string{"plate:update", "session:update"}, kinds(evs))
	assert.Equal(t, sun, decodeSession(t, evs[1].Before)["cook_date"])
	assert.Equal(t, tue, decodeSession(t, evs[1].After)["cook_date"])
	assert.Equal(t, tue, evs[1].Date.Format("2006-01-02"))
	assert.Equal(t, e.lunch, evs[1].SlotID)
}

// A session with a surplus keeps its portions (D11), so a leftover's eaters
// edit logs only the plate.
func TestEvents_SurplusSessionUnchangedIsNotLogged(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()
	_, err := e.svc.SetSessionPortions(e.ctx, sid, plate.SessionPortions{Portions: 10})
	require.NoError(t, err)
	monLunch := e.plateAt(mon, e.lunch)
	n := len(e.events())

	_, err = e.svc.Update(e.ctx, monLunch.ID, plate.PlatePatch{Eaters: f64(1)})
	require.NoError(t, err)
	assert.Equal(t, []string{"plate:update"}, kinds(e.eventsSince(n)))
	assert.Equal(t, 10.0, e.session(sid).Portions)
}

// B3-AC3: two leftovers write two create events plus one session event, in
// one transaction.
func TestEvents_AddLeftovers(t *testing.T) {
	e := newEnv(t)
	sunPlate := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	sid := sessionOf(sunPlate, 0)
	n := len(e.events())
	_, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(mon, lunch), cell(mon, dinner)})
	require.NoError(t, err)

	evs := e.eventsSince(n)
	require.Equal(t, []string{"plate:create", "plate:create", "session:update"}, kinds(evs))
	for _, ev := range evs[:2] {
		require.NotNil(t, ev.CookSessionID)
		assert.Equal(t, sid, *ev.CookSessionID)
		assert.Nil(t, ev.Before)
	}
	assert.Equal(t, e.plateAt(mon, e.lunch).ID, *evs[0].PlateID)
	assert.Equal(t, e.plateAt(mon, e.dinner).ID, *evs[1].PlateID)

	sess := evs[2]
	assert.Equal(t, sid, *sess.CookSessionID)
	assert.Equal(t, sun, sess.Date.Format("2006-01-02"))
	assert.Equal(t, e.dinner, sess.SlotID)
	before, after := decodeSession(t, sess.Before), decodeSession(t, sess.After)
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

// failingLog fails the Append after failAfter successful ones; 0 never fails.
type failingLog struct {
	plate.EventLog
	calls, failAfter int
}

var errEventLog = errors.New("event log down")

func (f *failingLog) Append(ctx context.Context, ev plate.Event) error {
	f.calls++
	if f.failAfter > 0 && f.calls > f.failAfter {
		return errEventLog
	}
	return f.EventLog.Append(ctx, ev)
}

// B3-AC3: the events are written in the mutation's transaction. When the
// second event fails, the first event, both plates and the session's growth
// are rolled back with it.
func TestEvents_AddLeftoversRollsBackOnEventFailure(t *testing.T) {
	fl := &failingLog{}
	e := newEnvWithEvents(t, func(l plate.EventLog) plate.EventLog { fl.EventLog = l; return fl })
	sunPlate := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	sid := sessionOf(sunPlate, 0)
	n := len(e.events())
	fl.failAfter = fl.calls + 1

	_, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(mon, lunch), cell(mon, dinner)})
	require.ErrorIs(t, err, errEventLog)
	assert.Equal(t, fl.failAfter+1, fl.calls, "the first Append succeeded, the second failed")
	assert.Equal(t, 1, e.count("plates"))
	assert.Equal(t, 2.0, e.session(sid).Portions)
	assert.Empty(t, e.eventsSince(n))
}

// B3-AC4: restore writes restore events with a null before, and the
// recreated session's create.
func TestEvents_RestoreWritesRestore(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()
	snap, err := e.svc.DeleteSession(e.ctx, sid)
	require.NoError(t, err)
	n := len(e.events())

	res, err := e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
	require.NoError(t, err)
	evs := e.eventsSince(n)
	require.Equal(t, []string{"plate:restore", "plate:restore", "plate:restore", "session:create"}, kinds(evs))
	for _, ev := range evs {
		assert.Nil(t, ev.Before)
		assert.NotNil(t, ev.After)
	}
	newSID := sessionOf(e.plateAt(sun, e.dinner), 0)
	assert.NotEqual(t, sid, newSID, "session ids are not reused")
	assert.Equal(t, newSID, *evs[3].CookSessionID)
	assert.Len(t, res.PlateIDs, 3)
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
	require.Equal(t, []string{"plate:update", "plate:delete", "session:delete"}, kinds(evs))
	for _, ev := range evs {
		assert.Equal(t, sid, *ev.CookSessionID)
	}
	kept, gone, sessEv := evs[0], evs[1], evs[2]
	assert.Equal(t, sunPlate.ID, *kept.PlateID)
	assert.Len(t, decodePlate(t, kept.Before).Components, 2)
	assert.Len(t, decodePlate(t, kept.After).Components, 1)
	assert.Equal(t, monPlate.ID, *gone.PlateID)
	assert.Nil(t, gone.After)
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
	require.Equal(t, []string{"plate:create", "plate:delete"}, kinds(evs))
	for _, ev := range evs {
		assert.Equal(t, p.ID, *ev.PlateID)
	}
	assert.Zero(t, e.count("plates"))
}

// Overwrite logs the replaced plate's delete and the new plate's create,
// under different ids.
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
	require.Equal(t, []string{"plate:delete", "plate:create"}, kinds(evs))
	assert.Equal(t, old.ID, *evs[0].PlateID)
	assert.Equal(t, res.PlateIDs[0], *evs[1].PlateID)
	assert.NotEqual(t, old.ID, res.PlateIDs[0])
}

// A new plate never gets a deleted plate's id (00030 AUTOINCREMENT), so each
// plate_id in the trail names one plate.
func TestEvents_NoPlateIDReuse(t *testing.T) {
	e := newEnv(t)
	e.create(mon, e.lunch, nil, grams(e.rice, 100))
	last := e.create(mon, e.dinner, nil, grams(e.rice, 100))
	_, err := e.svc.Delete(e.ctx, last.ID)
	require.NoError(t, err)
	next := e.create(mon, e.dinner, nil, grams(e.rice, 100))
	assert.Greater(t, next.ID, last.ID)

	byPlate := map[int64][]plate.Action{}
	for _, ev := range e.events() {
		byPlate[*ev.PlateID] = append(byPlate[*ev.PlateID], ev.Action)
	}
	assert.Equal(t, []plate.Action{plate.ActionCreate, plate.ActionDelete}, byPlate[last.ID])
	assert.Equal(t, []plate.Action{plate.ActionCreate}, byPlate[next.ID])
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
