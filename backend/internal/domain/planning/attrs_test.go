package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
)

// The Builder gives every food its attributes: derived ones, and the stored
// user value in place of the derived one.
func TestBuild_FoodFactsCarryAttributes(t *testing.T) {
	e, build := buildWeek(t)
	st := build(e.ctx)
	var id int64
	for fid, f := range st.Foods {
		if f.Composed() && len(f.Attrs.Heroes) > 0 {
			id = fid
			break
		}
	}
	require.NotZero(t, id, "a recipe with derived heroes")
	derived := st.Foods[id].Attrs
	require.NotNil(t, derived.Richness)

	require.NoError(t, sqlite.NewFoodAttributeRepo(e.db).Put(e.ctx, flavor.Record{
		FoodID: id, Version: flavor.Version,
		Values:  flavor.Attributes{Kind: "stew", Colour: "purple"},
		Sources: map[flavor.Field]flavor.Source{flavor.FieldKind: flavor.SourceUser, flavor.FieldColour: flavor.SourceUser},
	}))
	got := build(e.ctx).Foods[id].Attrs
	assert.Equal(t, "stew", got.Kind)
	assert.Equal(t, "purple", got.Colour)
	assert.Equal(t, derived.Heroes, got.Heroes)
	assert.Equal(t, derived.Richness, got.Richness)
}
