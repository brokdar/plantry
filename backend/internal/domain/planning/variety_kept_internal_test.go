package planning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// S1 (ledger V73, tune-v2 g2-planner-09/-14, h2-planner-02): a replan whose
// only open hard issues are dish-variety repeats, no more than the plan had
// at the start, is saved and names each repeat; a new repeat, or any other
// hard rule (the energy floor), still saves nothing (PL1b).
func TestVarietyOnlyKept(t *testing.T) {
	t.Parallel()
	h8 := func(date string, id int64) Issue {
		d, _ := time.Parse(time.DateOnly, date)
		return issue("H8", CodeVarietyRepeat, SeverityViolation).with("food_id", id).onDate(d)
	}
	floor := issue("H9", CodeKcalBelowFloor, SeverityViolation)
	two := []Issue{h8("2026-11-30", 1), h8("2026-12-01", 2)}

	assert.True(t, varietyOnlyKept(two, 2), "as many repeats as at the start")
	assert.True(t, varietyOnlyKept(two, 3), "fewer repeats than at the start")
	assert.False(t, varietyOnlyKept(two, 1), "a repeat more than at the start")
	assert.False(t, varietyOnlyKept(append(two[:1:1], floor), 5), "an energy rule is never kept")
	assert.False(t, varietyOnlyKept(nil, 2), "nothing open: nothing to keep")

	s := State{Foods: map[int64]FoodFacts{1: {ID: 1, Name: "Chili", Kind: food.KindComposed}, 2: {ID: 2, Name: "Curry", Kind: food.KindComposed}}}
	note := varietyKeptNote(s, two)
	assert.Contains(t, note, `"Chili" on 2026-11-30`)
	assert.Contains(t, note, `"Curry" on 2026-12-01`)
}
