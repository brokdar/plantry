package planning

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Review of #184 (M1): a more-protein candidate that adds enough protein and
// is dropped by a check without a clause makes the reason claim no clause and
// no absolute "nothing adds 10 g"; only what holds for every candidate.
func TestNoWayWhy_AnUnexplainedDropClaimsNoClause(t *testing.T) {
	t.Parallel()
	res := WayResult{Before: WayFigures{DayKcal: 1701}, BandHigh: 1785}

	none := &waySearch{goal: GoalMoreProtein}
	assert.Equal(t, "No checked change adds 10 g of protein or more.", none.noWayWhy(res))

	over := &waySearch{goal: GoalMoreProtein}
	over.blockedBy(blockOverTop)
	assert.Equal(t, "No change adds 10 g of protein or more without taking the day (1701 kcal) over its range top of 1785 kcal.", over.noWayWhy(res))

	for _, w := range []*waySearch{{goal: GoalMoreProtein, unexplained: true}, over} {
		w.unexplained = true
		assert.Equal(t, "No checked change adds 10 g of protein or more and passes every check.", w.noWayWhy(res))
	}
}
