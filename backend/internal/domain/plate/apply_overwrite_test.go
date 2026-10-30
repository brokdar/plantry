package plate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
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

// B1 review carry-over: only the {id} sessions of plates Apply actually writes
// are pinned. A skipped plate's {id} session that loses its only link to an
// overwrite is recorded by ref, so undoing the overwrite restores it.
func TestApply_OverwritePinsOnlyWrittenPlatesSessions(t *testing.T) {
	e := newEnv(t)
	sunPlate := e.create(sun, e.dinner, nil, composed(e.chili, 1))
	sid := sessionOf(sunPlate, 0)
	_, _, err := e.avail.SetOverride(e.ctx, availability.OverrideInput{Date: day(mon), SlotID: e.lunch, Mode: availability.ModeNone})
	require.NoError(t, err)
	before := e.plan(sun, mon)

	snap := plate.PlanSnapshot{Plates: []plate.SnapshotPlate{
		// Skipped: its cell is No plate.
		{Date: mon, SlotID: e.lunch, Eaters: 2, Components: []plate.SnapshotComponent{
			{FoodID: e.chili, Portions: f64(1), Session: &plate.SessionLink{ID: sid}},
		}},
		// Overwrites the plate holding the session's only link.
		{Date: sun, SlotID: e.dinner, Eaters: 2, Components: []plate.SnapshotComponent{
			{FoodID: e.rice, Amount: f64(50), Unit: ptrStr("g")},
		}},
	}}
	res, err := e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictOverwrite})
	require.NoError(t, err)
	assert.Equal(t, []plate.CellKey{cell(mon, e.lunch)}, res.SkippedUnavailable)
	require.Len(t, res.Removed.Plates, 1)
	require.Len(t, res.Removed.Sessions, 1, "the session went with the overwritten plate")
	assert.Equal(t, res.Removed.Sessions[0].Ref, res.Removed.Plates[0].Components[0].Session.Ref)
	assert.Equal(t, 0, e.count("cook_sessions"))

	// Undo: delete the created plate, restore what the overwrite removed.
	for _, id := range res.PlateIDs {
		_, err := e.svc.Delete(e.ctx, id)
		require.NoError(t, err)
	}
	_, err = e.svc.Apply(e.ctx, res.Removed, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	require.NoError(t, err)
	assert.Equal(t, before, e.plan(sun, mon))
}

// Overwriting every plate of a batch removes them as one: the session is
// recorded once by ref, never by an id the final settle deletes.
func TestApply_OverwriteWholeBatch_UndoRestoresIt(t *testing.T) {
	e := newEnv(t)
	_, sid := e.batch() // Sun dinner + Mon lunch + Mon dinner, exact 6
	_, err := e.svc.SetSessionPortions(e.ctx, sid, plate.SessionPortions{Portions: 7})
	require.NoError(t, err)
	before := e.plan(sun, mon)

	leaf := func(date string, slot int64) plate.SnapshotPlate {
		return plate.SnapshotPlate{Date: date, SlotID: slot, Components: []plate.SnapshotComponent{
			{FoodID: e.rice, Amount: f64(50), Unit: ptrStr("g")},
		}}
	}
	snap := plate.PlanSnapshot{Plates: []plate.SnapshotPlate{leaf(sun, e.dinner), leaf(mon, e.lunch), leaf(mon, e.dinner)}}
	res, err := e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictOverwrite})
	require.NoError(t, err)
	require.Len(t, res.PlateIDs, 3)
	require.Len(t, res.Removed.Plates, 3)
	require.Len(t, res.Removed.Sessions, 1)
	assert.InDelta(t, 7.0, res.Removed.Sessions[0].Portions, 1e-9, "the snapshot keeps the batch's own portions")
	for _, p := range res.Removed.Plates {
		assert.Equal(t, res.Removed.Sessions[0].Ref, p.Components[0].Session.Ref)
	}
	assert.Equal(t, 0, e.count("cook_sessions"))

	for _, id := range res.PlateIDs {
		_, err := e.svc.Delete(e.ctx, id)
		require.NoError(t, err)
	}
	_, err = e.svc.Apply(e.ctx, res.Removed, plate.ApplyOptions{OnConflict: plate.ConflictFail})
	require.NoError(t, err)
	assert.Equal(t, before, e.plan(sun, mon))
}

// Two snapshot plates on one cell would make Apply overwrite its own plate.
func TestApply_DuplicateCellIsInvalid(t *testing.T) {
	e := newEnv(t)
	p := plate.SnapshotPlate{Date: mon, SlotID: e.lunch, Components: []plate.SnapshotComponent{{FoodID: e.rice, Amount: f64(5), Unit: ptrStr("g")}}}
	_, err := e.svc.Apply(e.ctx, plate.PlanSnapshot{Plates: []plate.SnapshotPlate{p, p}}, plate.ApplyOptions{OnConflict: plate.ConflictOverwrite})
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Equal(t, 0, e.count("plates"))
}
