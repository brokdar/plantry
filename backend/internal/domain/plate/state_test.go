package plate_test

import (
	"context"
	"database/sql"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Phase 3 B1: draft/confirmed state (D1–D5) and run outcomes (D25).

const (
	fri     = "2026-11-06"
	sat     = "2026-11-07"
	nextSun = "2026-11-08"
	nextMon = "2026-11-09"
	nextTue = "2026-11-10"
)

// run inserts AI run id and returns a context acting as kind with it.
func (e *env) run(kind actor.Kind, id int64) context.Context {
	e.t.Helper()
	e.exec(`INSERT INTO ai_runs (id, kind, started_at) VALUES (?, 'generate', '2026-11-01 10:00:00')`, id)
	return actor.With(e.ctx, kind, id)
}

func (e *env) outcome(runID int64) string {
	e.t.Helper()
	var o sql.NullString
	require.NoError(e.t, e.db.QueryRow(`SELECT outcome FROM ai_runs WHERE id = ?`, runID).Scan(&o))
	return o.String
}

func (e *env) plate(id int64) *plate.Plate {
	e.t.Helper()
	p, err := e.svc.Get(e.ctx, id)
	require.NoError(e.t, err)
	return p
}

func (e *env) cookSession(id int64) *plate.CookSession {
	e.t.Helper()
	s, err := e.svc.CookSession(e.ctx, id)
	require.NoError(e.t, err)
	return s
}

// states returns the state of every plate and session, keyed "p<id>" and
// "s<id>".
func (e *env) states() map[string]plate.State {
	e.t.Helper()
	out := map[string]plate.State{}
	for _, q := range []struct{ prefix, sql string }{
		{"p", `SELECT id, state FROM plates`},
		{"s", `SELECT id, state FROM cook_sessions`},
	} {
		rows, err := e.db.Query(q.sql)
		require.NoError(e.t, err)
		for rows.Next() {
			var id int64
			var st string
			require.NoError(e.t, rows.Scan(&id, &st))
			out[q.prefix+strconv.FormatInt(id, 10)] = plate.State(st)
		}
		require.NoError(e.t, rows.Err())
		require.NoError(e.t, rows.Close())
	}
	return out
}

// allIn asserts every plate and session is in st.
func (e *env) allIn(st plate.State) {
	e.t.Helper()
	for k, got := range e.states() {
		assert.Equal(e.t, st, got, k)
	}
}

// ── B1-AC2: state of new rows follows the actor (D2) ────────────────────

func TestState_NewRowsFollowTheActor(t *testing.T) {
	e := newEnv(t)
	user := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	assert.Equal(t, plate.StateConfirmed, user.State)
	assert.Nil(t, user.AIRunID)
	assert.Equal(t, plate.StateConfirmed, e.cookSession(sessionOf(user, 0)).State)

	gen := e.run(actor.Generator, 7)
	p, err := e.svc.Create(gen, plate.PlateInput{Date: day(mon), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.curry, 1)}})
	require.NoError(t, err)
	assert.Equal(t, plate.StateDraft, p.State)
	require.NotNil(t, p.AIRunID)
	assert.Equal(t, int64(7), *p.AIRunID)
	sess := e.cookSession(sessionOf(p, 0))
	assert.Equal(t, plate.StateDraft, sess.State)
	require.NotNil(t, sess.AIRunID)
	assert.Equal(t, int64(7), *sess.AIRunID)

	agent := e.run(actor.Agent, 8)
	a, err := e.svc.Create(agent, plate.PlateInput{Date: day(tue), SlotID: e.dinner, Components: []plate.PlateComponent{grams(e.rice, 100)}})
	require.NoError(t, err)
	assert.Equal(t, plate.StateDraft, a.State)
	assert.Equal(t, int64(8), *a.AIRunID)
}

// D2 holds for Apply too: a snapshot's state is ignored unless it restores;
// its reason is kept.
func TestState_ApplyFollowsTheActorAndKeepsReasons(t *testing.T) {
	e := newEnv(t)
	gen := e.run(actor.Generator, 7)
	snap := plate.PlanSnapshot{
		Sessions: []plate.SnapshotSession{{Ref: "s1", FoodID: e.chili, CookDate: sun, CookSlotID: e.dinner, Portions: 2, Reason: strp("batch")}},
		Plates: []plate.SnapshotPlate{{
			Date: sun, SlotID: e.dinner, Eaters: 2, State: plate.StateConfirmed, Reason: strp("loved last time"),
			Components: []plate.SnapshotComponent{{FoodID: e.chili, Portions: f64(1), Session: &plate.SessionLink{Ref: "s1"}}},
		}},
	}
	res, err := e.svc.Apply(gen, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	require.NoError(t, err)
	p := e.plate(res.PlateIDs[0])
	assert.Equal(t, plate.StateDraft, p.State, "D2 beats the snapshot's state")
	assert.Equal(t, int64(7), *p.AIRunID)
	assert.Equal(t, "loved last time", *p.Reason)
	sess := e.cookSession(sessionOf(p, 0))
	assert.Equal(t, plate.StateDraft, sess.State)
	assert.Equal(t, "batch", *sess.Reason)

	snap.Plates[0].Date = mon
	snap.Sessions[0].CookDate = mon
	res, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	require.NoError(t, err)
	assert.Equal(t, plate.StateConfirmed, e.plate(res.PlateIDs[0]).State)
}

func TestState_ApplyRejectsUnknownState(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.Apply(e.ctx, plate.PlanSnapshot{Plates: []plate.SnapshotPlate{{
		Date: sun, SlotID: e.dinner, Eaters: 2, State: "locked",
		Components: []plate.SnapshotComponent{},
	}}}, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

// ── B1-AC3: Confirm and Unlock act on the batch closure (D3) ────────────

// draftBatch has the generator (run 7) cook Chili at Sun dinner with
// leftovers Mon lunch and Mon dinner, plus an unrelated draft Curry at Tue
// dinner.
func (e *env) draftBatch() (sunPlate *plate.Plate, sid int64, other *plate.Plate) {
	e.t.Helper()
	gen := e.run(actor.Generator, 7)
	sunPlate, err := e.svc.Create(gen, plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
	require.NoError(e.t, err)
	sid = sessionOf(sunPlate, 0)
	_, err = e.svc.AddLeftovers(gen, sid, []plate.CellKey{cell(mon, lunch), cell(mon, dinner)})
	require.NoError(e.t, err)
	other, err = e.svc.Create(gen, plate.PlateInput{Date: day(tue), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.curry, 1)}})
	require.NoError(e.t, err)
	return sunPlate, sid, other
}

func TestConfirm_LeftoverConfirmsItsWholeBatch(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid, other := e.draftBatch()
	monLunch, monDinner := e.plateAt(mon, e.lunch), e.plateAt(mon, e.dinner)
	n := len(e.events())

	got, err := e.svc.Confirm(e.ctx, plate.RowSet{PlateIDs: []int64{monDinner.ID}})
	require.NoError(t, err)
	assert.Equal(t, plate.RowSet{PlateIDs: []int64{sunPlate.ID, monLunch.ID, monDinner.ID}, SessionIDs: []int64{sid}}, got)
	for _, id := range got.PlateIDs {
		p := e.plate(id)
		assert.Equal(t, plate.StateConfirmed, p.State)
		assert.Equal(t, int64(7), *p.AIRunID, "confirm keeps the run")
	}
	assert.Equal(t, plate.StateConfirmed, e.cookSession(sid).State)
	assert.Equal(t, plate.StateDraft, e.plate(other.ID).State, "another batch stays draft")
	assert.Equal(t, plate.StateDraft, e.cookSession(sessionOf(other, 0)).State)

	evs := e.eventsSince(n)
	require.Equal(t, []string{"plate:confirm", "plate:confirm", "plate:confirm", "session:confirm"}, kinds(evs))
	for _, ev := range evs {
		assert.Equal(t, actor.User, ev.Actor)
	}
	assert.Equal(t, plate.StateDraft, decodePlate(t, evs[0].Before).State)
	assert.Equal(t, plate.StateConfirmed, decodePlate(t, evs[0].After).State)
	assert.Equal(t, "confirmed", decodeSession(t, evs[3].After)["state"])
	assert.Empty(t, e.outcome(7), "confirm leaves the outcome alone")

	// Confirming again changes nothing and logs nothing.
	n = len(e.events())
	again, err := e.svc.Confirm(e.ctx, plate.RowSet{SessionIDs: []int64{sid}})
	require.NoError(t, err)
	assert.Equal(t, got, again)
	assert.Empty(t, e.eventsSince(n))
}

func TestUnlock_SessionUnlocksEveryLinkedPlate(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch() // the user's batch: confirmed
	monLunch, monDinner := e.plateAt(mon, e.lunch), e.plateAt(mon, e.dinner)
	n := len(e.events())

	got, err := e.svc.Unlock(e.ctx, plate.RowSet{SessionIDs: []int64{sid}})
	require.NoError(t, err)
	assert.Equal(t, plate.RowSet{PlateIDs: []int64{sunPlate.ID, monLunch.ID, monDinner.ID}, SessionIDs: []int64{sid}}, got)
	e.allIn(plate.StateDraft)
	assert.Equal(t, []string{"plate:unlock", "plate:unlock", "plate:unlock", "session:unlock"}, kinds(e.eventsSince(n)))
}

// A plate that eats from two batches joins them into one closure.
func TestConfirm_ClosureFollowsSideBatches(t *testing.T) {
	e := newEnv(t)
	gen := e.run(actor.Generator, 7)
	chiliPlate, err := e.svc.Create(gen, plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
	require.NoError(t, err)
	curryPlate, err := e.svc.Create(gen, plate.PlateInput{Date: day(wed), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.curry, 1)}})
	require.NoError(t, err)
	both, err := e.svc.Create(gen, plate.PlateInput{Date: day(tue), SlotID: e.lunch, Components: []plate.PlateComponent{
		linked(e.chili, 1, sessionOf(chiliPlate, 0)), composed(e.curry, 1),
	}})
	require.NoError(t, err)
	_, err = e.svc.AddLeftovers(gen, sessionOf(both, 1), []plate.CellKey{cell(wed, lunch)})
	require.NoError(t, err)

	got, err := e.svc.Confirm(e.ctx, plate.RowSet{PlateIDs: []int64{chiliPlate.ID}})
	require.NoError(t, err)
	assert.Len(t, got.PlateIDs, 3, "Sun chili, Tue chili+curry, Wed lunch curry")
	assert.Len(t, got.SessionIDs, 2)
	assert.Equal(t, plate.StateDraft, e.plate(curryPlate.ID).State)
}

func TestConfirm_Errors(t *testing.T) {
	e := newEnv(t)
	_, err := e.svc.Confirm(e.ctx, plate.RowSet{})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = e.svc.Confirm(e.ctx, plate.RowSet{PlateIDs: []int64{999}})
	assert.ErrorIs(t, err, domain.ErrNotFound)
	_, err = e.svc.Unlock(e.ctx, plate.RowSet{SessionIDs: []int64{999}})
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

// ── B1-AC4: a link adopts the existing session's state (D3) ─────────────

func TestAddLeftovers_ToDraftSessionMakesDrafts(t *testing.T) {
	e := newEnv(t)
	_, sid, _ := e.draftBatch()
	_, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(tue, lunch)}) // as the user
	require.NoError(t, err)
	p := e.plateAt(tue, e.lunch)
	assert.Equal(t, plate.StateDraft, p.State, "adoption beats D2")
	assert.Nil(t, p.AIRunID, "the user's row carries no run")
}

func TestAddComponent_LinkToConfirmedSessionConfirmsThePlate(t *testing.T) {
	e := newEnv(t)
	userPlate := e.create(sun, e.dinner, nil, composed(e.chili, 1)) // confirmed batch
	gen := e.run(actor.Generator, 7)
	draft, err := e.svc.Create(gen, plate.PlateInput{Date: day(mon), SlotID: e.lunch, Components: []plate.PlateComponent{composed(e.curry, 1)}})
	require.NoError(t, err)
	n := len(e.events())

	got, err := e.svc.AddComponent(e.ctx, draft.ID, linked(e.chili, 1, sessionOf(userPlate, 0)))
	require.NoError(t, err)
	assert.Equal(t, plate.StateConfirmed, got.State)
	assert.Equal(t, plate.StateConfirmed, e.cookSession(sessionOf(draft, 0)).State, "the plate's own batch follows")

	evs := e.eventsSince(n)
	require.Equal(t, "plate:update", kinds(evs)[0])
	assert.Equal(t, plate.StateDraft, decodePlate(t, evs[0].Before).State)
	assert.Equal(t, plate.StateConfirmed, decodePlate(t, evs[0].After).State)
	assert.Contains(t, kinds(evs), "session:update")
}

// A new plate that links a draft and a confirmed batch is confirmed, and so
// becomes the draft batch it joins.
func TestCreate_LinkingMixedBatchesConfirmsTheClosure(t *testing.T) {
	e := newEnv(t)
	_, draftSid, _ := e.draftBatch()
	confirmed := e.create(sun, e.lunch, nil, composed(e.curry, 1))

	p, err := e.svc.Create(e.ctx, plate.PlateInput{Date: day(tue), SlotID: e.lunch, Components: []plate.PlateComponent{
		linked(e.chili, 1, draftSid), linked(e.curry, 1, sessionOf(confirmed, 0)),
	}})
	require.NoError(t, err)
	assert.Equal(t, plate.StateConfirmed, p.State)
	assert.Equal(t, plate.StateConfirmed, e.cookSession(draftSid).State)
	for _, d := range []string{sun, mon} {
		for _, sl := range []int64{e.lunch, e.dinner} {
			if q := e.plateAt(d, sl); q != nil {
				assert.Equal(t, plate.StateConfirmed, q.State, d)
			}
		}
	}
	assert.Equal(t, plate.StateDraft, e.plateAt(tue, e.dinner).State, "the unrelated draft stays")
}

// Confirmed wins in both directions: a confirmed plate (whose batch also
// feeds another confirmed plate) that links a draft session confirms the
// draft batch instead of unlocking its own.
func TestAddComponent_ConfirmedPlateLinkingDraftSessionConfirmsIt(t *testing.T) {
	e := newEnv(t)
	_, chiliSid := e.batch() // the user's: Sun dinner, Mon lunch, Mon dinner
	gen := e.run(actor.Generator, 7)
	draft, err := e.svc.Create(gen, plate.PlateInput{Date: day(sun), SlotID: e.lunch, Components: []plate.PlateComponent{composed(e.curry, 1)}})
	require.NoError(t, err)
	currySid := sessionOf(draft, 0)
	monLunch := e.plateAt(mon, e.lunch)
	n := len(e.events())

	got, err := e.svc.AddComponent(e.ctx, monLunch.ID, linked(e.curry, 1, currySid))
	require.NoError(t, err)
	assert.Equal(t, plate.StateConfirmed, got.State)
	e.allIn(plate.StateConfirmed)
	assert.Equal(t, plate.StateConfirmed, e.cookSession(chiliSid).State)

	evs := e.eventsSince(n)
	assert.Contains(t, kinds(evs), "session:update", "the curry batch's state change is logged")
	var draftUpdated bool
	for _, ev := range evs {
		if ev.PlateID != nil && *ev.PlateID == draft.ID {
			draftUpdated = true
			assert.Equal(t, plate.ActionUpdate, ev.Action)
			assert.Equal(t, plate.StateConfirmed, decodePlate(t, ev.After).State)
		}
	}
	assert.True(t, draftUpdated, "the draft Sun lunch plate gets an update event")
}

// Undo of a delete restores a draft leftover onto a batch that was accepted
// meanwhile: it comes back confirmed, so the closure stays uniform.
func TestRestore_DraftLeftoverOnAcceptedBatchIsConfirmed(t *testing.T) {
	e := newEnv(t)
	_, sid, _ := e.draftBatch()
	snap, err := e.svc.Delete(e.ctx, e.plateAt(mon, e.dinner).ID)
	require.NoError(t, err)
	require.Equal(t, plate.StateDraft, snap.Plates[0].State)
	_, err = e.svc.Accept(e.ctx, day(sun), day(sat))
	require.NoError(t, err)

	res, err := e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
	require.NoError(t, err)
	assert.Equal(t, plate.StateConfirmed, e.plate(res.PlateIDs[0]).State)
	assert.Equal(t, plate.StateConfirmed, e.cookSession(sid).State)
	e.allIn(plate.StateConfirmed)
}

// The same on a surviving plate (mergeInto): a session delete that kept a
// plate restores the batch as drafts, but the plate was confirmed meanwhile,
// so the restored batch is confirmed.
func TestRestore_MergeIntoConfirmedPlateConfirmsTheBatch(t *testing.T) {
	e := newEnv(t)
	gen := e.run(actor.Generator, 7)
	sunPlate, err := e.svc.Create(gen, plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
	require.NoError(t, err)
	sid := sessionOf(sunPlate, 0)
	mixed, err := e.svc.Create(gen, plate.PlateInput{Date: day(mon), SlotID: e.dinner, Components: []plate.PlateComponent{
		linked(e.chili, 1, sid), grams(e.rice, 100),
	}})
	require.NoError(t, err)

	snap, err := e.svc.DeleteSession(e.ctx, sid)
	require.NoError(t, err)
	require.Equal(t, plate.StateDraft, e.plate(mixed.ID).State, "the plate kept its rice")
	_, err = e.svc.Confirm(e.ctx, plate.RowSet{PlateIDs: []int64{mixed.ID}})
	require.NoError(t, err)

	_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
	require.NoError(t, err)
	got := e.plate(mixed.ID)
	require.Len(t, got.Components, 2)
	assert.Equal(t, plate.StateConfirmed, got.State)
	e.allIn(plate.StateConfirmed)
}

// Discard → restore stays all drafts (the uniformity rule is a no-op).
func TestRestore_DiscardRoundTripStaysDraft(t *testing.T) {
	e := newEnv(t)
	e.draftBatch()
	snap, err := e.svc.Discard(e.ctx, day(sun), day(sat))
	require.NoError(t, err)
	_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
	require.NoError(t, err)
	e.allIn(plate.StateDraft)
}

// ── B1-AC5: Accept, Unaccept, Discard and restore (D4, D5) ──────────────

// A draft leftover dated after to that eats from a batch cooked in the
// window is part of the window closure for every window action.
func TestWindowActions_IncludeLeftoversAfterTo(t *testing.T) {
	setup := func(t *testing.T) (*env, plate.RowSet) {
		e := newEnv(t)
		gen := e.run(actor.Generator, 7)
		cook, err := e.svc.Create(gen, plate.PlateInput{Date: day(nextSun), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
		require.NoError(t, err)
		sid := sessionOf(cook, 0)
		_, err = e.svc.AddLeftovers(gen, sid, []plate.CellKey{cell(nextMon, lunch)})
		require.NoError(t, err)
		after := e.plateAt(nextMon, e.lunch)
		return e, plate.RowSet{PlateIDs: []int64{cook.ID, after.ID}, SessionIDs: []int64{sid}}
	}
	from, to := day(mon), day(nextSun)

	t.Run("accept and unaccept", func(t *testing.T) {
		e, rows := setup(t)
		got, err := e.svc.Accept(e.ctx, from, to)
		require.NoError(t, err)
		assert.Equal(t, rows, got)
		e.allIn(plate.StateConfirmed)
		back, err := e.svc.Unaccept(e.ctx, got)
		require.NoError(t, err)
		assert.Equal(t, rows, back)
		e.allIn(plate.StateDraft)
	})
	t.Run("discard and restore", func(t *testing.T) {
		e, _ := setup(t)
		snap, err := e.svc.Discard(e.ctx, from, to)
		require.NoError(t, err)
		assert.Zero(t, e.count("plates"))
		assert.Zero(t, e.count("cook_sessions"))
		require.Len(t, snap.Plates, 2)
		assert.Equal(t, nextMon, snap.Plates[1].Date)
		_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
		require.NoError(t, err)
		after := e.plateAt(nextMon, e.lunch)
		require.NotNil(t, after)
		assert.Equal(t, plate.StateDraft, after.State)
		assert.Equal(t, int64(7), *after.AIRunID)
		assert.Equal(t, e.plateAt(nextSun, e.dinner).Components[0].CookSessionID, after.Components[0].CookSessionID)
		e.allIn(plate.StateDraft)
	})
}

// draftWeek has the generator (run 7) plan, through Apply with reasons:
// Chili cooked Sun dinner (the day before the window Mon–Sun) eaten Sun
// dinner, Mon lunch, Mon dinner; Curry cooked Tue lunch eaten Tue lunch, Tue
// dinner, Wed lunch; Chili cooked Thu dinner eaten Thu dinner, Fri lunch, Fri
// dinner, Sat lunch. 10 plates, 3 sessions. The user has a confirmed Rice
// plate at Wed dinner, and run 8 a draft at next Tue dinner.
func (e *env) draftWeek() {
	e.t.Helper()
	gen := e.run(actor.Generator, 7)
	sess := func(ref string, food int64, date string, slotID int64, portions float64) plate.SnapshotSession {
		return plate.SnapshotSession{Ref: ref, FoodID: food, CookDate: date, CookSlotID: slotID, Portions: portions, Reason: strp("why " + ref)}
	}
	pl := func(date string, slotID, food int64, ref string) plate.SnapshotPlate {
		return plate.SnapshotPlate{
			Date: date, SlotID: slotID, Eaters: 2, Reason: strp("why " + date),
			Components: []plate.SnapshotComponent{{FoodID: food, Portions: f64(1), Session: &plate.SessionLink{Ref: ref}}},
		}
	}
	snap := plate.PlanSnapshot{
		Sessions: []plate.SnapshotSession{
			sess("s1", e.chili, sun, e.dinner, 6), sess("s2", e.curry, tue, e.lunch, 6), sess("s3", e.chili, thu, e.dinner, 8),
		},
		Plates: []plate.SnapshotPlate{
			pl(sun, e.dinner, e.chili, "s1"), pl(mon, e.lunch, e.chili, "s1"), pl(mon, e.dinner, e.chili, "s1"),
			pl(tue, e.lunch, e.curry, "s2"), pl(tue, e.dinner, e.curry, "s2"), pl(wed, e.lunch, e.curry, "s2"),
			pl(thu, e.dinner, e.chili, "s3"), pl(fri, e.lunch, e.chili, "s3"), pl(fri, e.dinner, e.chili, "s3"), pl(sat, e.lunch, e.chili, "s3"),
		},
	}
	res, err := e.svc.Apply(gen, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	require.NoError(e.t, err)
	require.Len(e.t, res.PlateIDs, 10)
	e.create(wed, e.dinner, nil, grams(e.rice, 100))
	other := e.run(actor.Generator, 8)
	_, err = e.svc.Create(other, plate.PlateInput{Date: day(nextTue), SlotID: e.dinner, Components: []plate.PlateComponent{grams(e.rice, 50)}})
	require.NoError(e.t, err)
}

func TestAccept_ConfirmsTheWindowClosure(t *testing.T) {
	e := newEnv(t)
	e.draftWeek()
	n := len(e.events())

	got, err := e.svc.Accept(e.ctx, day(mon), day(nextSun))
	require.NoError(t, err)
	assert.Len(t, got.PlateIDs, 10, "the Sun cook plate before from is in the closure")
	assert.Len(t, got.SessionIDs, 3)
	for _, id := range got.PlateIDs {
		assert.Equal(t, plate.StateConfirmed, e.plate(id).State)
	}
	for _, id := range got.SessionIDs {
		assert.Equal(t, plate.StateConfirmed, e.cookSession(id).State)
	}
	assert.Equal(t, plate.StateDraft, e.plateAt(nextTue, e.dinner).State, "outside the window")

	evs := e.eventsSince(n)
	require.Len(t, evs, 13)
	for _, k := range kinds(evs) {
		assert.Contains(t, []string{"plate:accept", "session:accept"}, k)
	}
	assert.Equal(t, "accepted", e.outcome(7))
	assert.Empty(t, e.outcome(8))

	// Undo accept: exactly those rows go back to draft.
	n = len(e.events())
	back, err := e.svc.Unaccept(e.ctx, got)
	require.NoError(t, err)
	assert.Equal(t, got, back)
	for _, id := range got.PlateIDs {
		assert.Equal(t, plate.StateDraft, e.plate(id).State)
	}
	assert.Equal(t, plate.StateConfirmed, e.plateAt(wed, e.dinner).State, "the user's plate stays")
	assert.Len(t, e.eventsSince(n), 13)
	assert.Equal(t, "open", e.outcome(7))
}

// Accept is one unit of work: a failure leaves every row draft.
func TestAccept_FailureChangesNothing(t *testing.T) {
	fl := &failingLog{}
	e := newEnvWithEvents(t, func(l plate.EventLog) plate.EventLog { fl.EventLog = l; return fl })
	e.draftWeek()
	fl.calls, fl.failAfter = 0, 5
	_, err := e.svc.Accept(e.ctx, day(mon), day(nextSun))
	require.ErrorIs(t, err, errEventLog)
	for _, d := range []string{sun, mon, tue, thu, fri} {
		assert.Equal(t, plate.StateDraft, e.plateAt(d, e.dinner).State, d)
	}
	assert.Empty(t, e.outcome(7))
}

func TestDiscard_DeletesDraftsAndRestoreBringsThemBack(t *testing.T) {
	e := newEnv(t)
	e.draftWeek()
	before := e.count("plates")
	n := len(e.events())

	snap, err := e.svc.Discard(e.ctx, day(mon), day(nextSun))
	require.NoError(t, err)
	assert.Equal(t, before-10, e.count("plates"))
	assert.Zero(t, e.count("cook_sessions"), "the two plates left are leaves")
	require.Len(t, snap.Plates, 10)
	require.Len(t, snap.Sessions, 3)
	for _, sp := range snap.Plates {
		assert.Equal(t, plate.StateDraft, sp.State)
		assert.Equal(t, int64(7), *sp.AIRunID)
		assert.Equal(t, "why "+sp.Date, *sp.Reason)
	}
	evs := e.eventsSince(n)
	require.Len(t, evs, 13)
	for _, k := range kinds(evs) {
		assert.Contains(t, []string{"plate:discard", "session:discard"}, k)
	}
	assert.Equal(t, "discarded", e.outcome(7))

	// Undo: restore brings back drafts with their runs and reasons.
	_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
	require.NoError(t, err)
	monLunch := e.plateAt(mon, e.lunch)
	assert.Equal(t, plate.StateDraft, monLunch.State)
	assert.Equal(t, int64(7), *monLunch.AIRunID)
	assert.Equal(t, "why "+mon, *monLunch.Reason)
	sess := e.cookSession(sessionOf(monLunch, 0))
	assert.Equal(t, plate.StateDraft, sess.State)
	assert.Equal(t, int64(7), *sess.AIRunID)
	assert.Equal(t, "why s1", *sess.Reason)
	assert.Equal(t, sun, sess.CookDate.Format("2006-01-02"))
}

// Restoring a deleted draft brings it back as a draft (D5).
func TestDelete_RestoreKeepsDraftState(t *testing.T) {
	e := newEnv(t)
	_, _, other := e.draftBatch()
	snap, err := e.svc.Delete(e.ctx, other.ID)
	require.NoError(t, err)
	assert.Equal(t, plate.StateDraft, snap.Plates[0].State)
	res, err := e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
	require.NoError(t, err)
	p := e.plate(res.PlateIDs[0])
	assert.Equal(t, plate.StateDraft, p.State)
	assert.Equal(t, int64(7), *p.AIRunID)
}

func TestAcceptAndDiscard_NothingToAccept(t *testing.T) {
	e := newEnv(t)
	e.create(mon, e.dinner, nil, composed(e.chili, 1)) // confirmed only
	_, err := e.svc.Accept(e.ctx, day(mon), day(nextSun))
	assert.ErrorIs(t, err, domain.ErrNothingToAccept)
	_, err = e.svc.Discard(e.ctx, day(mon), day(nextSun))
	assert.ErrorIs(t, err, domain.ErrNothingToAccept)
	_, err = e.svc.Accept(e.ctx, day(nextSun), day(mon))
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestUnaccept_Errors(t *testing.T) {
	e := newEnv(t)
	_, sid, other := e.draftBatch()
	_, err := e.svc.Unaccept(e.ctx, plate.RowSet{})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	_, err = e.svc.Unaccept(e.ctx, plate.RowSet{PlateIDs: []int64{other.ID}})
	assert.ErrorIs(t, err, domain.ErrStateMismatch, "a draft row")
	_, err = e.svc.Unaccept(e.ctx, plate.RowSet{SessionIDs: []int64{sid}})
	assert.ErrorIs(t, err, domain.ErrStateMismatch, "a draft session")
	_, err = e.svc.Unaccept(e.ctx, plate.RowSet{PlateIDs: []int64{999}})
	assert.ErrorIs(t, err, domain.ErrStateMismatch, "a row that is gone")
}

// ── B1-AC6: run outcomes through the service (D25) ──────────────────────

func TestOutcome_Machine(t *testing.T) {
	// Run 7 drafts a plate in each of two weeks.
	setup := func(t *testing.T) *env {
		e := newEnv(t)
		gen := e.run(actor.Generator, 7)
		for _, d := range []string{mon, nextMon} {
			_, err := e.svc.Create(gen, plate.PlateInput{Date: day(d), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
			require.NoError(t, err)
		}
		return e
	}
	week1, week2 := [2]string{mon, nextSun}, [2]string{nextMon, nextMon}

	t.Run("accept", func(t *testing.T) {
		e := setup(t)
		_, err := e.svc.Accept(e.ctx, day(week1[0]), day(week1[1]))
		require.NoError(t, err)
		assert.Equal(t, "accepted", e.outcome(7))
	})
	t.Run("discard", func(t *testing.T) {
		e := setup(t)
		_, err := e.svc.Discard(e.ctx, day(week1[0]), day(week1[1]))
		require.NoError(t, err)
		assert.Equal(t, "discarded", e.outcome(7))
	})
	t.Run("accept then discard the rest", func(t *testing.T) {
		e := setup(t)
		_, err := e.svc.Accept(e.ctx, day(week1[0]), day(week1[1]))
		require.NoError(t, err)
		_, err = e.svc.Discard(e.ctx, day(week2[0]), day(week2[1]))
		require.NoError(t, err)
		assert.Equal(t, "partially_accepted", e.outcome(7))
	})
	t.Run("unaccept", func(t *testing.T) {
		e := setup(t)
		rows, err := e.svc.Accept(e.ctx, day(week1[0]), day(week1[1]))
		require.NoError(t, err)
		_, err = e.svc.Unaccept(e.ctx, rows)
		require.NoError(t, err)
		assert.Equal(t, "open", e.outcome(7))
	})
	t.Run("confirm and unlock leave it alone", func(t *testing.T) {
		e := setup(t)
		p := e.plateAt(mon, e.dinner)
		_, err := e.svc.Confirm(e.ctx, plate.RowSet{PlateIDs: []int64{p.ID}})
		require.NoError(t, err)
		_, err = e.svc.Unlock(e.ctx, plate.RowSet{PlateIDs: []int64{p.ID}})
		require.NoError(t, err)
		assert.Empty(t, e.outcome(7))
	})
}

// ── B1-AC7: edits never change state ────────────────────────────────────

func TestEdits_KeepState(t *testing.T) {
	for _, st := range []plate.State{plate.StateDraft, plate.StateConfirmed} {
		t.Run(string(st), func(t *testing.T) {
			e := newEnv(t)
			gen := e.run(actor.Generator, 7)
			p, err := e.svc.Create(gen, plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
			require.NoError(t, err)
			if st == plate.StateConfirmed {
				_, err := e.svc.Confirm(e.ctx, plate.RowSet{PlateIDs: []int64{p.ID}})
				require.NoError(t, err)
			}
			// The other actor edits: the user a draft, the agent a confirmed plate.
			ctx := e.ctx
			if st == plate.StateConfirmed {
				ctx = e.run(actor.Agent, 8)
			}
			_, err = e.svc.Update(ctx, p.ID, plate.PlatePatch{Eaters: f64(3)})
			require.NoError(t, err)
			_, err = e.svc.UpdateComponentQuantity(ctx, p.Components[0].ID, plate.PlateComponent{Portions: f64(2)})
			require.NoError(t, err)
			swapped, err := e.svc.SwapComponent(ctx, p.Components[0].ID, e.curry, nil)
			require.NoError(t, err)
			_, err = e.svc.AddComponent(ctx, p.ID, composed(e.chili, 1))
			require.NoError(t, err)

			got := e.plate(p.ID)
			assert.Equal(t, st, got.State)
			assert.Equal(t, int64(7), *got.AIRunID, "edits keep the run")
			assert.Equal(t, st, e.cookSession(sessionOf(swapped, 0)).State, "a swap's fresh session takes the plate's state")
			e.allIn(st)
		})
	}
}
