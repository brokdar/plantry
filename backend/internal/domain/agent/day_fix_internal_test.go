package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// A lighter dinner that stops short of light ends with the recommended
// bigger-lunch offer, said as the sentence to close the reply with.
func TestNextStep_StopsShortClosesWithTheRecommendedOffer(t *testing.T) {
	t.Parallel()
	ways := []planning.Way{{Kind: "swap", What: "Tue dinner: swap", Before: planning.WayFigures{MealKcal: 1583}, After: planning.WayFigures{MealKcal: 1212}}}
	offers := []planning.Way{
		{Kind: "compound", What: "Tue dinner: first", After: planning.WayFigures{MealKcal: 700, DayKcal: 1900}},
		{Kind: "compound", What: "Tue dinner: halve the salmon", Recommended: true, After: planning.WayFigures{MealKcal: 544, DayKcal: 1825}},
	}
	res := planning.WayResult{Goal: planning.GoalLighter, MaxKcal: 550, FloorKcal: 1650}
	got := nextStep(ways, offers, res, false)
	assert.Contains(t, got, "END YOUR REPLY with the recommended offer")
	assert.Contains(t, got, "I can also Tue dinner: halve the salmon; the day would then hold 1825 kcal")
}
