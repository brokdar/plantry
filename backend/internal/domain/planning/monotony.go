package planning

import (
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
)

// Monotony (TASTE-9) is tracked apart from dislike: the plates eaten per
// cuisine, protein and format over the monotonyDays up to a cell. A main
// that would pass a soft cap ranks after one that does not (pickMain);
// nothing is excluded. Caps are tunable: per 7 days about 4 plates of one
// cuisine or protein and 3 of one format, doubled for the 14 days.
const monotonyDays = 14

var monotonyCaps = map[string]int{"cuisine": 8, "protein": 8, "format": 6}

// monotonyKeys are a main's exposure keys: its cuisine (not neutral), its
// protein and its format, each when known.
func monotonyKeys(f FoodFacts) []string {
	var keys []string
	if c := f.Attrs.Cuisine; c != "" && c != "neutral" {
		keys = append(keys, "cuisine:"+c)
	}
	if f.Protein != "" {
		keys = append(keys, "protein:"+f.Protein)
	}
	if f.Attrs.Format != "" {
		keys = append(keys, "format:"+f.Attrs.Format)
	}
	return keys
}

// proteinOf is a recipe's protein for monotony: its first hero ingredient
// that is meat, fish or a legume ("" without one).
func proteinOf(a flavor.Attributes) string {
	for _, h := range a.Heroes {
		switch diet.GroupOf(h) {
		case diet.GroupMeat, diet.GroupFish, diet.GroupLegume:
			return taste.NormKey(h)
		}
	}
	return ""
}

// exposure is one eaten plate's main and date.
type exposure struct {
	date time.Time
	main int64
}

// historyMain is the main among a history plate's foods: the first recipe
// that counts as a lunch or dinner main.
func (s State) historyMain(ids []int64) int64 {
	for _, id := range ids {
		if s.countsAsMain(id) {
			return id
		}
	}
	return 0
}

// initExposure records the mains of the history and of the fixed plates:
// confirmed ones and those outside the target.
func (h *planner) initExposure() {
	for _, hp := range h.s.History {
		if m := h.s.historyMain(hp.FoodIDs); m != 0 {
			h.exposed = append(h.exposed, exposure{hp.Date, m})
		}
	}
	for _, p := range h.s.Plates {
		if p.Proposed() || (!p.Confirmed && h.x.inTarget(p.Cell())) {
			continue
		}
		if m := h.s.mainOf(p); m != 0 {
			h.exposed = append(h.exposed, exposure{p.Date, m})
		}
	}
}

// expose records a placed plate's main.
func (h *planner) expose(pp ProposedPlate) {
	if m := h.s.proposedMain(pp); m != 0 {
		h.exposed = append(h.exposed, exposure{pp.Date, m})
	}
}

// exposureAt counts the plates per monotony key in the monotonyDays up to
// and including c's day.
func (h *planner) exposureAt(c plate.CellKey) map[string]int {
	since := c.Date.AddDate(0, 0, -monotonyDays)
	counts := map[string]int{}
	for _, e := range h.exposed {
		if e.date.After(since) && !e.date.After(c.Date) {
			for _, k := range monotonyKeys(h.s.Foods[e.main]) {
				counts[k]++
			}
		}
	}
	return counts
}

// overMonotony counts the soft caps one more plate of f would pass.
func overMonotony(f FoodFacts, counts map[string]int) int {
	n := 0
	if f.Staple { // a staple is never held back by a soft cap (THEME-1)
		return n
	}
	for _, k := range monotonyKeys(f) {
		kind, _, _ := strings.Cut(k, ":")
		if counts[k]+1 > monotonyCaps[kind] {
			n++
		}
	}
	return n
}
