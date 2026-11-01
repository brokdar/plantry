package plate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Phase 3 B8: RemovePlates is the re-plan's delete, with the batches it
// settles itself.

func TestRemovePlates_MissingPlateRemovesNothing(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch()
	monLunch := e.plateAt(mon, e.lunch)

	_, err := e.svc.RemovePlates(e.ctx, plate.Removal{PlateIDs: []int64{monLunch.ID, 9999}})
	require.ErrorIs(t, err, domain.ErrNotFound)
	_, err = e.svc.RemovePlates(e.ctx, plate.Removal{PlateIDs: []int64{monLunch.ID}, Drop: []int64{sid, 9999}})
	require.ErrorIs(t, err, domain.ErrNotFound, "a dropped session that is gone")

	assert.Equal(t, 3, e.count("plates"))
}

func TestRemovePlates_KeptSessionGetsItsPortionsEvenWithoutLinks(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()
	ids := []int64{sunPlate.ID, e.plateAt(mon, e.lunch).ID, e.plateAt(mon, e.dinner).ID}

	snap, err := e.svc.RemovePlates(e.ctx, plate.Removal{PlateIDs: ids, Keep: map[int64]plate.SessionPortions{sid: {Portions: 5, FrozenPortions: 2}}})

	require.NoError(t, err)
	assert.Equal(t, 0, e.count("plates"))
	v := e.session(sid)
	assert.Empty(t, v.Links)
	assert.InDelta(t, 5, v.Portions, 1e-9)
	assert.InDelta(t, 2, v.FrozenPortions, 1e-9)
	assert.Empty(t, snap.Sessions, "the kept session is linked by id, not recreated")
	require.Len(t, snap.Plates, 3)
	require.NotNil(t, snap.Plates[0].Components[0].Session)
	assert.Equal(t, sid, snap.Plates[0].Components[0].Session.ID)
}

func TestRemovePlates_OtherSessionsFollowD11(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch() // exactly 6 portions for three plates of 2 eaters
	monDinner := e.plateAt(mon, e.dinner)

	snap, err := e.svc.RemovePlates(e.ctx, plate.Removal{PlateIDs: []int64{monDinner.ID}})

	require.NoError(t, err)
	assert.InDelta(t, 4, e.session(sid).Portions, 1e-9, "an exact batch stays exact")
	assert.Nil(t, e.plateAt(mon, e.dinner))
	_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
	require.NoError(t, err)
	assert.InDelta(t, 6, e.session(sid).Portions, 1e-9, "the snapshot restores it")
}

// A dropped batch takes every component eating it: a plate left empty goes,
// a plate with other food keeps it, and the snapshot restores it all.
func TestRemovePlates_DropRemovesEveryLink(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()
	monDinner := e.plateAt(mon, e.dinner)
	_, err := e.svc.AddComponent(e.ctx, monDinner.ID, grams(e.rice, 150))
	require.NoError(t, err)

	snap, err := e.svc.RemovePlates(e.ctx, plate.Removal{PlateIDs: []int64{sunPlate.ID}, Drop: []int64{sid}})

	require.NoError(t, err)
	assert.Nil(t, e.plateAt(sun, e.dinner))
	assert.Nil(t, e.plateAt(mon, e.lunch), "left without components")
	kept := e.plateAt(mon, e.dinner)
	require.NotNil(t, kept)
	require.Len(t, kept.Components, 1)
	assert.Equal(t, e.rice, kept.Components[0].FoodID)
	assert.Equal(t, 0, e.componentCount(e.chili))
	assert.Equal(t, 0, e.count("cook_sessions"), "the dropped batch is gone")

	_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
	require.NoError(t, err)
	assert.Equal(t, 3, e.componentCount(e.chili))
	require.Len(t, e.plateAt(mon, e.dinner).Components, 2)
}

// A moved batch cooks at its new cell; that plate eats it fresh.
func TestRemovePlates_MoveCook(t *testing.T) {
	e := newEnv(t)
	sunPlate, sid := e.batch()

	_, err := e.svc.RemovePlates(e.ctx, plate.Removal{PlateIDs: []int64{sunPlate.ID}, Move: map[int64]plate.CellKey{sid: cell(mon, e.lunch)}})

	require.NoError(t, err)
	v := e.session(sid)
	assert.True(t, cell(mon, e.lunch).Equal(v.CookCell()))
	require.Len(t, v.Links, 2)
	assert.Equal(t, plate.KindFresh, v.Links[0].Kind)
	assert.Empty(t, v.Warnings)
}
