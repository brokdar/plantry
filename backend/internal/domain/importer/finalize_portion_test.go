package importer_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// The review step shows the parser's per-ingredient estimate ("1 clove ≈ 3 g")
// but sends no portion_grams unless the user edits it, so Finalize must seed
// a missing portion from the same estimate, not the flat unit default.
func TestFinalize_SeedsPortionFromIngredientName(t *testing.T) {
	cases := []struct {
		name, leaf, unit string
		reviewed         *float64
		want             float64
	}{
		{"garlic clove", "Knoblauch", "clove", nil, 3},
		{"onion piece", "Zwiebel", "piece", nil, 110},
		{"egg piece", "Eier", "piece", nil, 50},
		{"a reviewed weight wins", "Zwiebel", "piece", ptr(95), 95},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newFinalizeEnv(t)
			id := e.leaf(t, tc.leaf)
			_, err := e.importer.Finalize(context.Background(), importer.FinalizeInput{
				Name: "R", Role: "main", ReferencePortions: 2,
				Children: []importer.FinalizedChild{{
					Resolution: importer.ResolutionExisting, FoodID: id,
					Amount: 2, Unit: tc.unit, PortionGrams: tc.reviewed,
				}},
			})
			require.NoError(t, err)
			ps := e.portions(t, id)
			require.Len(t, ps, 1)
			assert.Equal(t, tc.want, ps[0].Grams)
		})
	}
}

func ptr(v float64) *float64 { return &v }

// A count of something nobody can estimate is not seeded with a guess: the
// review must supply the weight.
func TestFinalize_UnknownCountNeedsWeight(t *testing.T) {
	e := newFinalizeEnv(t)
	id := e.leaf(t, "Fantasiefrucht")
	_, err := e.importer.Finalize(context.Background(), importer.FinalizeInput{
		Name: "R", Role: "main", ReferencePortions: 2,
		Children: []importer.FinalizedChild{{
			Resolution: importer.ResolutionExisting, FoodID: id, Amount: 2, Unit: "piece",
		}},
	})
	require.ErrorIs(t, err, domain.ErrImportInvalidResolution)
	assert.Empty(t, e.portions(t, id))
}
