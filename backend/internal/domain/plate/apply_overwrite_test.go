package plate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// An overwrite that removes a plate sharing a session with a {id} component
// settles once, after relinking: an exact batch stays exact.
func TestApply_OverwriteSharedSession_ExactBatchStaysExact(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch() // Sun dinner + Mon lunch + Mon dinner, exact 6
	snap, err := e.svc.Delete(e.ctx, e.plateAt(mon, e.lunch).ID)
	require.NoError(t, err)
	require.Equal(t, sid, snap.Plates[0].Components[0].Session.ID)
	require.InDelta(t, 4.0, e.session(sid).Portions, 1e-9)

	// Mon lunch is taken again by a plate eating from the same batch.
	e.create(mon, e.lunch, nil, linked(e.chili, 1, sid))
	require.InDelta(t, 6.0, e.session(sid).Portions, 1e-9)

	res, err := e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictOverwrite})
	require.NoError(t, err)
	require.Len(t, res.Removed.Plates, 1)
	assert.Equal(t, sid, res.Removed.Plates[0].Components[0].Session.ID, "the kept session is recorded by id")
	assert.Empty(t, res.Removed.Sessions)

	v := e.session(sid)
	assert.InDelta(t, 6.0, v.Portions, 1e-9)
	assert.InDelta(t, 6.0, v.Consumed, 1e-9)
	assert.NotContains(t, codes(v.Warnings), plate.WarnDeficit)
	restored := e.plateAt(mon, e.lunch)
	assert.Equal(t, sid, sessionOf(restored, 0))
}

// When the overwritten plate held the session's only other link, the session
// is not collected before the {id} component links to it again.
func TestApply_OverwriteSharedSession_OnlyOtherLinkKeepsSession(t *testing.T) {
	e := newEnv(t)
	sunPlate := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	sid := sessionOf(sunPlate, 0)
	_, err := e.svc.AddLeftovers(e.ctx, sid, []plate.CellKey{cell(mon, lunch)})
	require.NoError(t, err)
	snap, err := e.svc.Delete(e.ctx, sunPlate.ID)
	require.NoError(t, err)
	require.Equal(t, sid, snap.Plates[0].Components[0].Session.ID)

	// The leftover moves onto Sun dinner: it now holds the session's only link.
	_, err = e.svc.Update(e.ctx, e.plateAt(mon, e.lunch).ID, plate.PlatePatch{Date: ptrTime(day(sun)), SlotID: &e.dinner})
	require.NoError(t, err)

	res, err := e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictOverwrite})
	require.NoError(t, err)
	require.Len(t, res.PlateIDs, 1)
	assert.Empty(t, res.Removed.Sessions, "the session survives, so it is not snapshotted by ref")

	v := e.session(sid)
	assert.Equal(t, 1, v.Meals)
	assert.InDelta(t, 2.0, v.Portions, 1e-9)
	assert.Equal(t, 1, e.count("cook_sessions"))
}

// plan.freezing_enabled accepts every strconv.ParseBool spelling, like the
// settings validator.
func TestSetSessionPortions_FreezingEnabledParsesBool(t *testing.T) {
	e := newEnv(t)
	sid := sessionOf(e.create(sun, e.dinner, nil, composed(e.chili, 1)), 0)
	e.set(settings.KeyPlanFreezingEnabled, "1")
	v, err := e.svc.SetSessionPortions(e.ctx, sid, plate.SessionPortions{Portions: 4, FrozenPortions: 2})
	require.NoError(t, err)
	assert.InDelta(t, 2.0, v.FrozenPortions, 1e-9)
}
