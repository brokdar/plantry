package substitution

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

func line(id int64, name string, g float64) food.FoodComponent {
	return food.FoodComponent{ChildID: id, ChildName: name, Grams: g}
}

func TestDiffSwaps(t *testing.T) {
	t.Parallel()
	base := []food.FoodComponent{line(1, "A", 100), line(2, "C", 50), line(9, "X", 5)}
	// A to B, then B to E: one swap A to E; C to D beside it.
	variant := []food.FoodComponent{line(5, "E", 100), line(4, "D", 50), line(9, "X", 5)}
	changes, targets := diffSwaps(base, variant)
	assert.Equal(t, []string{"A → E", "C → D"}, changes)
	assert.Equal(t, []string{"E", "D"}, targets)

	// The substitute already in the recipe grows: A merges into X.
	changes, _ = diffSwaps(base, []food.FoodComponent{line(2, "C", 50), line(9, "X", 105)})
	assert.Equal(t, []string{"A → X"}, changes)

	changes, _ = diffSwaps(base, base)
	assert.Empty(t, changes)
}
