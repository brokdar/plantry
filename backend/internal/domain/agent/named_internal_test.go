package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// SF3: only a food whose whole name is in the request counts as named.
func TestNamedFoods(t *testing.T) {
	t.Parallel()
	foods := map[int64]planning.FoodFacts{
		1: {Name: "Hasselback potatoes"}, 2: {Name: "Reis"}, 3: {Name: "Reispfanne"},
		4: {Name: "Chili"}, 5: {Name: "Chili"}, 6: {Name: "Käsespätzle"},
	}
	assert.Equal(t, map[int64]bool{1: true}, namedFoods("Put the hasselback potatoes on Friday", foods))
	assert.Equal(t, map[int64]bool{3: true}, namedFoods("Eine Reispfanne am Freitag", foods), "a name inside a longer matched one is not named")
	assert.Empty(t, namedFoods("make it chili", foods), "two foods of one name: not one food")
	assert.Equal(t, map[int64]bool{6: true}, namedFoods("KÄSESPÄTZLE bitte", foods), "folded")
	assert.Empty(t, namedFoods("make dinner lighter", foods))
}
