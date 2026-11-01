package plate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Phase 4 B5: Revert replays the before snapshots of one change's events
// (D13).

// draftBatchOf builds a Sunday dinner Chili draft batch of run, with
// leftovers on Monday lunch (an exact session of 4 for 2 eaters).
func (e *env) draftBatchOf(runID int64) (*plate.Plate, int64) {
	e.t.Helper()
	gen := e.run(actor.Generator, runID)
	sunPlate, err := e.svc.Create(gen, plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
	require.NoError(e.t, err)
	sid := sessionOf(sunPlate, 0)
	_, err = e.svc.Create(gen, plate.PlateInput{Date: day(mon), SlotID: e.lunch, Components: []plate.PlateComponent{linked(e.chili, 1, sid)}})
	require.NoError(e.t, err)
	return sunPlate, sid
}

func TestRevert_SessionPortionsComeBackAsUserEvents(t *testing.T) {
	e := newEnv(t)
	e.set("plan.freezing_enabled", "true")
	sunPlate, sid := e.draftBatchOf(7)
	agent := e.run(actor.Agent, 8)
	n := len(e.events())

	_, err := e.svc.SetSessionPortions(agent, sid, plate.SessionPortions{Portions: 6, FrozenPortions: 2})
	require.NoError(t, err)
	change := e.eventsSince(n)
	require.NotEmpty(t, change)

	n = len(e.events())
	rows, err := e.svc.Revert(e.ctx, change)
	require.NoError(t, err)

	v := e.session(sid)
	assert.InDelta(t, 4, v.Portions, 1e-9)
	assert.InDelta(t, 0, v.FrozenPortions, 1e-9)
	assert.Equal(t, plate.StateDraft, v.State)
	require.NotNil(t, v.AIRunID)
	assert.Equal(t, int64(7), *v.AIRunID, "the run that wrote it stays")
	assert.Contains(t, rows.SessionIDs, sid)
	assert.Contains(t, rows.PlateIDs, sunPlate.ID)
	assert.Equal(t, sunPlate.ID, e.plateAt(sun, e.dinner).ID, "plates keep their ids")
	for _, ev := range e.eventsSince(n) {
		assert.Equal(t, actor.User, ev.Actor)
		assert.Nil(t, ev.AIRunID)
	}
}

func TestRevert_DeletedPlateComesBackWithItsStateAndRun(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.draftBatchOf(7)
	monPlate := e.plateAt(mon, e.lunch)
	agent := e.run(actor.Agent, 8)
	n := len(e.events())

	_, err := e.svc.Delete(agent, monPlate.ID)
	require.NoError(t, err)
	assert.InDelta(t, 2, e.session(sid).Portions, 1e-9, "D11 shrank the batch")
	change := e.eventsSince(n)

	n = len(e.events())
	_, err = e.svc.Revert(e.ctx, change)
	require.NoError(t, err)

	back := e.plateAt(mon, e.lunch)
	require.NotNil(t, back)
	assert.Equal(t, monPlate.ID, back.ID)
	assert.Equal(t, plate.StateDraft, back.State)
	require.NotNil(t, back.AIRunID)
	assert.Equal(t, int64(7), *back.AIRunID)
	require.Len(t, back.Components, 1)
	assert.Equal(t, sid, *back.Components[0].CookSessionID)
	assert.InDelta(t, 4, e.session(sid).Portions, 1e-9)
	assert.Equal(t, sunPlate.ID, e.plateAt(sun, e.dinner).ID)
	assert.Contains(t, kinds(e.eventsSince(n)), "plate:restore")
}

func TestRevert_CreatedPlateAndSessionGo(t *testing.T) {
	e := newEnv(t)
	agent := e.run(actor.Agent, 8)
	n := len(e.events())
	_, err := e.svc.Create(agent, plate.PlateInput{Date: day(tue), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.curry, 1)}})
	require.NoError(t, err)
	change := e.eventsSince(n)

	n = len(e.events())
	_, err = e.svc.Revert(e.ctx, change)
	require.NoError(t, err)
	assert.Equal(t, 0, e.count("plates"))
	assert.Equal(t, 0, e.count("cook_sessions"))
	assert.ElementsMatch(t, []string{"plate:delete", "session:delete"}, kinds(e.eventsSince(n)))
}

func TestRevert_CreatedSessionStillEatenFromIsStale(t *testing.T) {
	e := newEnv(t)
	agent := e.run(actor.Agent, 8)
	n := len(e.events())
	p, err := e.svc.Create(agent, plate.PlateInput{Date: day(tue), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.curry, 1)}})
	require.NoError(t, err)
	change := e.eventsSince(n)
	e.create(wed, e.lunch, nil, linked(e.curry, 1, sessionOf(p, 0)))
	before := len(e.events())

	_, err = e.svc.Revert(e.ctx, change)
	require.ErrorIs(t, err, domain.ErrAIChangeStale)
	assert.Equal(t, 2, e.count("plates"), "nothing written")
	assert.Len(t, e.events(), before)
}

func TestRevert_DeletedPlateRestoresLeafComponent(t *testing.T) {
	e := newEnv(t)
	gen := e.run(actor.Generator, 7)
	p, err := e.svc.Create(gen, plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{grams(e.rice, 80)}})
	require.NoError(t, err)
	agent := e.run(actor.Agent, 8)
	n := len(e.events())
	_, err = e.svc.Delete(agent, p.ID)
	require.NoError(t, err)
	change := e.eventsSince(n)

	_, err = e.svc.Revert(e.ctx, change)
	require.NoError(t, err)

	back := e.plate(p.ID)
	require.Len(t, back.Components, 1)
	c := back.Components[0]
	assert.Equal(t, e.rice, c.FoodID)
	require.NotNil(t, c.Grams)
	assert.InDelta(t, 80, *c.Grams, 1e-9, "grams restored")
	// grams_source is not part of the snapshot's guarantees: a restored leaf
	// may come back with a NULL grams_source, which is accepted.
}

func TestRevert_DeletedPlateRestoresFeedback(t *testing.T) {
	e := newEnv(t)
	gen := e.run(actor.Generator, 7)
	p, err := e.svc.Create(gen, plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1)}})
	require.NoError(t, err)
	e.exec(`INSERT INTO plate_feedback (plate_id, status, note, rated_at) VALUES (?, 'loved', 'great', '2026-11-02 08:00:00')`, p.ID)
	agent := e.run(actor.Agent, 8)
	n := len(e.events())
	_, err = e.svc.Delete(agent, p.ID)
	require.NoError(t, err)
	change := e.eventsSince(n)

	_, err = e.svc.Revert(e.ctx, change)
	require.NoError(t, err)

	back := e.plate(p.ID)
	require.NotNil(t, back.Feedback)
	assert.Equal(t, "loved", back.Feedback.Status)
	require.NotNil(t, back.Feedback.Note)
	assert.Equal(t, "great", *back.Feedback.Note)
}
