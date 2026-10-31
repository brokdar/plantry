package plate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Phase 3 B6: RemoveDrafts is the delete half of the generator's replace
// (D10).

func TestRemoveDrafts_WholePlatesAndTheirSessions(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid, other := e.draftBatch()
	monLunch, monDinner := e.plateAt(mon, e.lunch), e.plateAt(mon, e.dinner)

	snap, err := e.svc.RemoveDrafts(e.ctx, plate.RowSet{PlateIDs: []int64{sunPlate.ID, monLunch.ID, monDinner.ID}, SessionIDs: []int64{sid}})
	require.NoError(t, err)

	assert.Equal(t, 1, e.count("plates"), "only the unrelated Tue plate stays")
	assert.Equal(t, other.ID, e.plateAt(tue, e.dinner).ID)
	assert.Equal(t, 1, e.count("cook_sessions"))
	require.Len(t, snap.Plates, 3)
	require.Len(t, snap.Sessions, 1)
	assert.Equal(t, plate.StateDraft, snap.Plates[0].State, "the snapshot restores drafts")

	_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
	require.NoError(t, err)
	assert.Equal(t, 4, e.count("plates"))
}

func TestRemoveDrafts_SessionTakesOnlyItsComponents(t *testing.T) {
	e := newEnv(t)
	gen := e.run(actor.Generator, 7)
	p, err := e.svc.Create(gen, plate.PlateInput{Date: day(sun), SlotID: e.dinner, Components: []plate.PlateComponent{composed(e.chili, 1), composed(e.curry, 1)}})
	require.NoError(t, err)
	side := sessionOf(p, 1)

	snap, err := e.svc.RemoveDrafts(e.ctx, plate.RowSet{SessionIDs: []int64{side}})
	require.NoError(t, err)

	kept := e.plate(p.ID)
	require.Len(t, kept.Components, 1, "the plate keeps its main")
	assert.Equal(t, e.chili, kept.Components[0].FoodID)
	assert.Equal(t, 1, e.count("cook_sessions"), "the side's session is gone")
	require.Len(t, snap.Plates, 1)
	require.NotNil(t, snap.Plates[0].PlateID, "the snapshot merges back into the kept plate")
	assert.Equal(t, p.ID, *snap.Plates[0].PlateID)
}

func TestDraftClosureAndPlateClosure(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid, other := e.draftBatch()
	monLunch, monDinner := e.plateAt(mon, e.lunch), e.plateAt(mon, e.dinner)
	e.create(wed, e.dinner, nil, composed(e.chili, 1)) // confirmed, not a draft

	c, plates, err := e.svc.DraftClosure(e.ctx, day(mon), day(mon))
	require.NoError(t, err)
	assert.Equal(t, plate.RowSet{PlateIDs: []int64{sunPlate.ID, monLunch.ID, monDinner.ID}, SessionIDs: []int64{sid}}, c,
		"Monday's leftovers pull in Sunday's batch")
	assert.Len(t, plates, 3)

	c, _, err = e.svc.DraftClosure(e.ctx, day(wed), day(wed))
	require.NoError(t, err)
	assert.True(t, c.Empty(), "no drafts: empty, no error")

	c, plates, err = e.svc.PlateClosure(e.ctx, other.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{other.ID}, c.PlateIDs)
	require.Len(t, plates, 1)

	_, _, err = e.svc.PlateClosure(e.ctx, 9999)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestRemoveDrafts_RefusesConfirmedOrMissingRows(t *testing.T) {
	e := newEnv(t)
	confirmed := e.create(wed, e.dinner, nil, composed(e.chili, 1))
	_, _, draft := e.draftBatch()
	before := e.count("plates")

	for name, rows := range map[string]plate.RowSet{
		"confirmed plate":   {PlateIDs: []int64{draft.ID, confirmed.ID}},
		"confirmed session": {SessionIDs: []int64{sessionOf(confirmed, 0)}},
		"missing plate":     {PlateIDs: []int64{draft.ID, 9999}},
		"missing session":   {SessionIDs: []int64{9999}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := e.svc.RemoveDrafts(e.ctx, rows)
			require.ErrorIs(t, err, domain.ErrStateMismatch)
			assert.Equal(t, before, e.count("plates"), "nothing removed")
		})
	}
}
