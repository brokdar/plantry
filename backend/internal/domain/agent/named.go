package agent

import (
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// namedFoods are the foods the user's message names (SF3): a food whose whole
// name (folded, at least 4 letters) is in the text and that no other food of
// that name shares. A name inside a longer matched name does not count
// ("Reis" in "Reispfanne" when only the pan is named). It is set from the
// request text, never from a model argument.
func namedFoods(text string, foods map[int64]planning.FoodFacts) map[int64]bool {
	msg := " " + diet.Fold(text) + " "
	byName := map[string][]int64{}
	for id, f := range foods {
		n := diet.Fold(f.Name)
		if len(n) >= 4 && strings.Contains(msg, n) {
			byName[n] = append(byName[n], id)
		}
	}
	out := map[int64]bool{}
	for n, ids := range byName {
		if len(ids) != 1 {
			continue
		}
		inner := false
		for other := range byName {
			if other != n && strings.Contains(other, n) {
				inner = true
				break
			}
		}
		if !inner {
			out[ids[0]] = true
		}
	}
	return out
}
