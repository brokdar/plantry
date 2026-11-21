package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// When no compound way reaches the light limit the closest one is still the
// offer, and it says plainly that it is not a light meal (E048b: Wednesday's
// beef ragu, where the kitchen rules keep the meal at 560 kcal or more).
func TestOrClosest(t *testing.T) {
	t.Parallel()
	near := planning.Way{What: "swap the salad for broccoli", After: planning.WayFigures{MealKcal: 754}, Why: "the lunch stays within a filling lunch"}
	hit := planning.Way{What: "reaches", Reaches: true, After: planning.WayFigures{MealKcal: 540}}

	got := orClosest(nil, []planning.Way{near}, 550)
	if assert.Len(t, got, 1) {
		assert.Equal(t, "the closest it gets: the meal holds 754 kcal, not a light meal (the limit is 550 kcal); the lunch stays within a filling lunch", got[0].Why)
		assert.False(t, got[0].Reaches)
	}
	assert.Equal(t, []planning.Way{hit}, orClosest([]planning.Way{hit}, []planning.Way{near}, 550), "an offer that reaches wins")
	assert.Empty(t, orClosest(nil, nil, 550))
}
