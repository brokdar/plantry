package planning_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

func TestToSnapshot(t *testing.T) {
	s := baseState()
	s.Profile.HouseholdPortions = 2
	p := planning.Proposal{
		Sessions: []planning.ProposedSession{{Key: "s1", FoodID: chili, CookDate: day(sat), CookSlotID: dinner, Portions: 6, FrozenPortions: 2, Reason: "batch"}},
		Plates: []planning.ProposedPlate{
			pplate(sat, dinner, onKey(chili, "s1"), fresh(rice)),
			{Date: day(sun), SlotID: lunch, Eaters: 4, Components: []planning.ProposedComponent{onID(curry, 42)}},
		},
	}

	snap := planning.ToSnapshot(p, s)

	assert.Equal(t, []plate.SnapshotSession{{Ref: "s1", FoodID: chili, CookDate: sat, CookSlotID: dinner, Portions: 6, FrozenPortions: 2}}, snap.Sessions)
	require.Len(t, snap.Plates, 2)
	sp := snap.Plates[0]
	assert.Equal(t, sat, sp.Date)
	assert.Equal(t, dinner, sp.SlotID)
	assert.InDelta(t, 2.0, sp.Eaters, 1e-9, "household portions by default (D16)")
	require.Len(t, sp.Components, 2)
	assert.Equal(t, &plate.SessionLink{Ref: "s1"}, sp.Components[0].Session)
	assert.Equal(t, 0, sp.Components[0].SortOrder)
	assert.Nil(t, sp.Components[1].Session, "no link: Apply cooks it fresh")
	assert.Equal(t, 1, sp.Components[1].SortOrder)
	assert.InDelta(t, 1.0, *sp.Components[1].Portions, 1e-9)
	assert.InDelta(t, 4.0, snap.Plates[1].Eaters, 1e-9, "explicit eaters kept")
	assert.Equal(t, &plate.SessionLink{ID: 42}, snap.Plates[1].Components[0].Session)
}

// B5-AC3: a 1-session/3-plate proposal applies through plate.Service.Apply.
func TestToSnapshot_AppliesThroughPlateService(t *testing.T) {
	db := testhelper.NewTestDB(t)
	svc, _ := testsvc.Plan(db)
	ctx := context.Background()
	exec := func(q string) int64 {
		res, err := db.Exec(q)
		require.NoError(t, err, q)
		id, err := res.LastInsertId()
		require.NoError(t, err)
		return id
	}
	lunchID := exec(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.lunch', 'sun', 1, 1)`)
	dinnerID := exec(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.dinner', 'moon', 2, 1)`)
	chiliID := exec(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Chili', 'composed', 'main', 4)`)
	exec(`UPDATE user_profile SET household_portions = 1 WHERE id = 1`)

	s := baseState()
	p := planning.Proposal{
		Sessions: []planning.ProposedSession{session("s1", chiliID, sat, dinnerID, 3)},
		Plates: []planning.ProposedPlate{
			pplate(sat, dinnerID, onKey(chiliID, "s1")),
			pplate(sun, lunchID, onKey(chiliID, "s1")),
			pplate(sun, dinnerID, onKey(chiliID, "s1")),
		},
	}

	res, err := svc.Apply(ctx, planning.ToSnapshot(p, s), plate.ApplyOptions{OnConflict: plate.ConflictFail})
	require.NoError(t, err)
	require.Len(t, res.PlateIDs, 3)

	first, err := svc.Get(ctx, res.PlateIDs[0])
	require.NoError(t, err)
	require.Len(t, first.Components, 1)
	sid := *first.Components[0].CookSessionID
	for _, id := range res.PlateIDs[1:] {
		pl, err := svc.Get(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, sid, *pl.Components[0].CookSessionID, "every plate eats from the one session")
	}
	v, err := svc.Session(ctx, sid)
	require.NoError(t, err)
	assert.Equal(t, 3, v.Meals)
	assert.InDelta(t, 3.0, v.Portions, 1e-9)
	assert.Empty(t, v.Warnings)
}
