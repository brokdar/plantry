package planning

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The meat guard never buys less meat with a third fish meal; a week already
// past the limit does not grow either, but one at the limit may stay.
func TestGuardKeepsFishLimit(t *testing.T) {
	ev := func(meat float64, fish int) Evaluation {
		return Evaluation{Week: &WeekEval{MeatG: meat, FishMeals: fish}}
	}
	assert.True(t, guardKeeps(ev(400, 1), ev(300, 2), nil, nil, swapMeat), "second fish meal for less meat")
	assert.False(t, guardKeeps(ev(400, 2), ev(300, 3), nil, nil, swapMeat), "third fish meal")
	assert.False(t, guardKeeps(ev(400, 4), ev(300, 6), nil, nil, swapMeat), "past the limit grows")
	assert.True(t, guardKeeps(ev(400, 4), ev(300, 4), nil, nil, swapMeat), "past the limit, no change")
}
