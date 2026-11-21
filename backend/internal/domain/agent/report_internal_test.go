package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// c: no tell item is empty or said twice.
func TestBuildTell_NoEmptyAndNoDuplicateItems(t *testing.T) {
	t.Parallel()
	one, two := 500, 500
	r := changeReport{Changed: []reportMeal{
		{Day: "Thu 26 Nov", Meal: "breakfast", Before: []string{"Oats 1 portion"}, After: []string{"Oats 1 portion"}}, // nothing to say: was "Thu 26 Nov breakfast: ."
		{Day: "Thu 26 Nov", Meal: "dinner", Before: []string{"Chili 1 portion"}, After: []string{"Rice 1 portion"}, KcalFrom: &one, KcalTo: &two},
		{Day: "Thu 26 Nov", Meal: "dinner", Before: []string{"Chili 1 portion"}, After: []string{"Rice 1 portion"}, KcalFrom: &one, KcalTo: &two},
	}, Leftovers: []string{"Same.", "same."}}
	tell := buildTell(reportInput{}, r)
	seen := map[string]bool{}
	for _, s := range tell {
		assert.NotRegexp(t, emptyTellItem, s)
		assert.False(t, seen[s], "twice: %s", s)
		seen[s] = true
	}
	assert.Len(t, tell, 2, "%v", tell)
}

// The dish that moved names its earliest new cooking, whatever the map order.
func TestLeftovers_MovedDishIsDeterministic(t *testing.T) {
	t.Parallel()
	foods := map[int64]planning.FoodFacts{1: {Name: "Lachs"}}
	day := func(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }
	bs := planning.State{Foods: foods, Sessions: []planning.Session{{CookSession: plate.CookSession{ID: 1, FoodID: 1, CookDate: day(23), CookSlotID: 3, Portions: 1}}}}
	as := planning.State{Foods: foods, Sessions: []planning.Session{
		{CookSession: plate.CookSession{ID: 2, FoodID: 1, CookDate: day(27), CookSlotID: 3, Portions: 1}},
		{CookSession: plate.CookSession{ID: 3, FoodID: 1, CookDate: day(24), CookSlotID: 3, Portions: 1}},
	}}
	// the old cooking fed one meal, so it counts as eaten
	bs.Plates = []planning.Plate{{ID: 1, Date: day(23), SlotID: 3, Eaters: 1, Components: []planning.Component{{SessionID: 1, Portions: ptrF(1)}}}}
	for i := 0; i < 30; i++ {
		out := leftovers(bs, as, &mustSink{})
		if assert.NotEmpty(t, out) {
			assert.Contains(t, out[0], "Tue 24 Nov", "run %d", i)
		}
	}
}

func ptrF(v float64) *float64 { return &v }
