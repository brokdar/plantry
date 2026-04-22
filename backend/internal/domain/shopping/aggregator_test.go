package shopping_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

const tol = 0.01

func assertGrams(t *testing.T, want, got float64) {
	t.Helper()
	if math.Abs(want-got) > tol {
		t.Errorf("total_grams: want %.4f, got %.4f", want, got)
	}
}

func mkPlate(day int, comps ...plate.PlateComponent) plate.Plate {
	return plate.Plate{Day: day, Components: comps}
}

func mkPC(componentID int64, portions float64) plate.PlateComponent {
	return plate.PlateComponent{ComponentID: componentID, Portions: portions}
}

func mkRef(refPortions float64, ings ...shopping.ComponentIngredient) shopping.ComponentRef {
	return shopping.ComponentRef{ReferencePortions: refPortions, Ingredients: ings}
}

func mkIng(id int64, name string, grams float64) shopping.ComponentIngredient {
	return shopping.ComponentIngredient{IngredientID: id, Name: name, Grams: grams}
}

func TestFromPlates_Empty(t *testing.T) {
	items := shopping.FromPlates(nil, nil)
	assert.Empty(t, items)
}

func TestFromPlates_EmptyRefs(t *testing.T) {
	plates := []plate.Plate{mkPlate(0, mkPC(1, 1))}
	items := shopping.FromPlates(plates, nil)
	assert.Empty(t, items)
}

func TestFromPlates_SingleComponent_OnePortionEqualsReference(t *testing.T) {
	refs := map[int64]shopping.ComponentRef{
		1: mkRef(1, mkIng(10, "Chicken", 100)),
	}
	plates := []plate.Plate{mkPlate(0, mkPC(1, 1))}

	items := shopping.FromPlates(plates, refs)
	require.Len(t, items, 1)
	assert.Equal(t, "Chicken", items[0].Name)
	assertGrams(t, 100, items[0].TotalGrams)
}

func TestFromPlates_SingleComponent_TwoPortionsOverOneReference(t *testing.T) {
	refs := map[int64]shopping.ComponentRef{
		1: mkRef(1, mkIng(10, "Rice", 200)),
	}
	plates := []plate.Plate{mkPlate(0, mkPC(1, 2))}

	items := shopping.FromPlates(plates, refs)
	require.Len(t, items, 1)
	assertGrams(t, 400, items[0].TotalGrams)
}

func TestFromPlates_TwoPlates_SameComponent_Accumulates(t *testing.T) {
	refs := map[int64]shopping.ComponentRef{
		1: mkRef(1, mkIng(10, "Pasta", 150)),
	}
	plates := []plate.Plate{
		mkPlate(0, mkPC(1, 1)),
		mkPlate(1, mkPC(1, 1)),
	}

	items := shopping.FromPlates(plates, refs)
	require.Len(t, items, 1)
	assertGrams(t, 300, items[0].TotalGrams)
}

func TestFromPlates_TwoComponents_TwoItems(t *testing.T) {
	refs := map[int64]shopping.ComponentRef{
		1: mkRef(1, mkIng(10, "Chicken", 100)),
		2: mkRef(1, mkIng(20, "Rice", 80)),
	}
	plates := []plate.Plate{mkPlate(0, mkPC(1, 1), mkPC(2, 1))}

	items := shopping.FromPlates(plates, refs)
	require.Len(t, items, 2)
}

func TestFromPlates_MissingRef_Skipped(t *testing.T) {
	refs := map[int64]shopping.ComponentRef{
		1: mkRef(1, mkIng(10, "Chicken", 100)),
	}
	plates := []plate.Plate{mkPlate(0, mkPC(1, 1), mkPC(99, 1))} // 99 not in refs

	items := shopping.FromPlates(plates, refs)
	require.Len(t, items, 1)
	assert.Equal(t, "Chicken", items[0].Name)
}

func TestFromPlates_ZeroReferencePortions_Skipped(t *testing.T) {
	refs := map[int64]shopping.ComponentRef{
		1: mkRef(0, mkIng(10, "Chicken", 100)), // ReferencePortions=0 → skip
	}
	plates := []plate.Plate{mkPlate(0, mkPC(1, 1))}

	items := shopping.FromPlates(plates, refs)
	assert.Empty(t, items)
}

func TestFromPlates_SortedAlphabetically(t *testing.T) {
	refs := map[int64]shopping.ComponentRef{
		1: mkRef(1, mkIng(10, "Zucchini", 50), mkIng(20, "Apple", 30)),
	}
	plates := []plate.Plate{mkPlate(0, mkPC(1, 1))}

	items := shopping.FromPlates(plates, refs)
	require.Len(t, items, 2)
	assert.Equal(t, "Apple", items[0].Name)
	assert.Equal(t, "Zucchini", items[1].Name)
}
