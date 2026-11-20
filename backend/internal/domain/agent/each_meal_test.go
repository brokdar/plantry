package agent_test

import (
	"fmt"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// eachDraftMeal runs fn for every draft lunch and dinner of the relEnv week,
// each on its own parallel subtest with its own copy of the week: the checks
// of one plate do not depend on another's, and one test that loops over the
// whole week dominated the package's wall time under -race. The subtests are
// grouped, so the caller's counters are final when eachDraftMeal returns.
func eachDraftMeal(t *testing.T, fn func(t *testing.T, e *planToolEnv, p plate.Plate)) {
	t.Helper()
	var ids []int64
	for _, p := range relEnv(t).draftMeals() {
		ids = append(ids, p.ID)
	}
	t.Run("plates", func(t *testing.T) {
		for _, id := range ids {
			t.Run(fmt.Sprint("plate_", id), func(t *testing.T) {
				t.Parallel()
				e := relEnv(t)
				for _, p := range e.draftMeals() {
					if p.ID == id {
						fn(t, e, p)
						return
					}
				}
				t.Fatalf("plate %d is not in the week", id)
			})
		}
	})
}

// eachDraftDay is eachDraftMeal for checks that depend on the day only (a
// whole-day goal gives the same answer for the day's lunch and dinner): it
// runs fn once per date, with the date's first draft meal.
func eachDraftDay(t *testing.T, fn func(t *testing.T, e *planToolEnv, p plate.Plate)) {
	t.Helper()
	eachDraftMeal(t, func(t *testing.T, e *planToolEnv, p plate.Plate) {
		for _, q := range e.draftMeals() {
			if q.DateString() == p.DateString() {
				if q.ID == p.ID {
					fn(t, e, p)
				}
				return
			}
		}
	})
}
