package planning

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
)

// Plate fit in the LLM chooser and the assistant (PLATE-5): a clash the
// proposal brings to a plate is a problem only in the chooser's repair
// input; it never enters Check, so apply_changes and the assistant's day
// tools never refuse for plate fit. find_alternatives reads each
// candidate's plate in a short phrase (Alternative.Fit).

// plateReading reads p's plate fit when plan.plate_fit is on and p has a
// main; keep picks the components read (nil reads them all).
func (s State) plateReading(p Plate, keep func(Component) bool) (platefit.Reading, bool) {
	var comps []Component
	for _, c := range p.Components {
		if keep == nil || keep(c) {
			comps = append(comps, c)
		}
	}
	mainID := s.mainOf(Plate{Components: comps})
	if !s.plateFitOn(mainID) {
		return platefit.Reading{}, false
	}
	pl := platefit.Plate{Main: plateComponent(s.Foods[mainID])}
	for _, c := range comps {
		if f, ok := s.Foods[c.FoodID]; ok && c.FoodID != mainID {
			pl.Sides = append(pl.Sides, plateComponent(f))
		}
	}
	return platefit.Read(pl), true
}

// plateFitPhrase is p's plate fit in a few words; "" when plate fit does not
// judge p.
func (s State) plateFitPhrase(p Plate) string {
	r, ok := s.plateReading(p, nil)
	if !ok {
		return ""
	}
	return platefit.Phrase(r)
}

// plateClashes are the repair lines for the clashes the proposal brings to
// a plate: clashes of the whole plate that the plate without the proposed
// components did not have (the user's own plate is never sent back).
func (r *repairer) plateClashes() []string {
	var out []string
	for _, p := range r.s.Plates {
		if !slices.ContainsFunc(p.Components, func(c Component) bool { return c.Proposed }) {
			continue
		}
		after, ok := r.s.plateReading(p, nil)
		if !ok || len(after.Clashes) == 0 {
			continue
		}
		before, _ := r.s.plateReading(p, func(c Component) bool { return !c.Proposed })
		var words []string
		for _, d := range after.Clashes {
			if !slices.Contains(before.Clashes, d) {
				words = append(words, platefit.ClashWords(d))
			}
		}
		if len(words) == 0 {
			continue
		}
		names := make([]string, 0, len(p.Components))
		for _, c := range p.Components {
			names = append(names, foodName(r.s, c.FoodID))
		}
		out = append(out, fmt.Sprintf("Plate fit: %s puts %s on one plate (%s). Fix: replace the side that clashes with the main by one that fits it, or drop it and keep the day's energy with another side.",
			r.cellLabel(p.Cell()), strings.Join(names, " + "), strings.Join(words, ", ")))
	}
	return out
}
