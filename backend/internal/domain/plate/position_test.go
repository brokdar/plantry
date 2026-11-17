package plate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

func positions(p *plate.Plate) []plate.Position {
	out := make([]plate.Position, len(p.Components))
	for i, c := range p.Components {
		out[i] = c.Position
	}
	return out
}

func TestPosition_DerivedOnCreate(t *testing.T) {
	e := newEnv(t)
	gurke := e.insert(`INSERT INTO foods (name, kind, source, shop_section) VALUES ('Gurke', 'leaf', 'manual', 'produce')`)
	cake := e.insert(`INSERT INTO foods (name, kind, role, reference_portions) VALUES ('Cake', 'composed', 'dessert', 4)`)
	salt := e.insert(`INSERT INTO foods (name, kind, source, shop_section) VALUES ('Salz', 'leaf', 'manual', 'spices_oils')`)

	// A second main-like food becomes a side; produce is a side, dessert and
	// a role-less leaf are extras.
	p := e.create(mon, e.lunch, nil, composed(e.chili, 1), composed(e.curry, 1), grams(gurke, 100), composed(cake, 1), grams(salt, 5))
	assert.Equal(t, []plate.Position{plate.PositionMain, plate.PositionSide, plate.PositionSide, plate.PositionExtra, plate.PositionExtra}, positions(p))

	got := e.plateAt(mon, e.lunch)
	assert.Equal(t, positions(p), positions(got), "stored")
}

func TestPosition_ExplicitKeptAndSecondMainRefused(t *testing.T) {
	e := newEnv(t)
	c := composed(e.chili, 1)
	c.Position = plate.PositionExtra
	p := e.create(mon, e.lunch, nil, c)
	assert.Equal(t, plate.PositionExtra, p.Components[0].Position)

	// The plate has no main now: an explicit main is allowed once.
	m := composed(e.curry, 1)
	m.Position = plate.PositionMain
	p, err := e.svc.AddComponent(e.ctx, p.ID, m)
	require.NoError(t, err)

	m2 := composed(e.chili, 1)
	m2.Position = plate.PositionMain
	_, err = e.svc.AddComponent(e.ctx, p.ID, m2)
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	require.ErrorIs(t, err, plate.ErrSecondMain)
	key, ok := plate.MessageKey(err)
	assert.True(t, ok)
	assert.Equal(t, "error.plate.second_main", key)

	// Derived: a main-like food on a plate with a main becomes a side.
	p, err = e.svc.AddComponent(e.ctx, p.ID, composed(e.chili, 1))
	require.NoError(t, err)
	assert.Equal(t, plate.PositionSide, p.Components[len(p.Components)-1].Position)

	bad := composed(e.chili, 1)
	bad.Position = "top"
	_, err = e.svc.AddComponent(e.ctx, p.ID, bad)
	assert.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestPosition_SwapKeepsPositionAndSnapshotRoundTrips(t *testing.T) {
	e := newEnv(t)
	p := e.create(mon, e.lunch, nil, composed(e.chili, 1), grams(e.rice, 100))
	require.Equal(t, []plate.Position{plate.PositionMain, plate.PositionExtra}, positions(p))

	swapped, err := e.svc.SwapComponent(e.ctx, p.Components[0].ID, e.curry, nil)
	require.NoError(t, err)
	assert.Equal(t, plate.PositionMain, swapped.Components[0].Position)

	snap, err := e.svc.Delete(e.ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, snap.Plates, 1)
	assert.Equal(t, plate.PositionMain, snap.Plates[0].Components[0].Position)
	assert.Equal(t, plate.PositionExtra, snap.Plates[0].Components[1].Position)

	_, err = e.svc.Apply(e.ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail, Restore: true})
	require.NoError(t, err)
	assert.Equal(t, []plate.Position{plate.PositionMain, plate.PositionExtra}, positions(e.plateAt(mon, e.lunch)))
}
