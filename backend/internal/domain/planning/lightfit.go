package planning

import (
	"fmt"
	"math"
	"slices"
)

// trimLightMeals brings a light meal that holds more than its limit (S6) down
// to it by lowering the portions of its main (to at least 0.5), and the batch
// the main eats from by the same amount, so coverage holds. A meal marked
// light is never above its limit in the saved plan when the main alone can
// bring it there; the energy it gives up is the day's other meals' to carry.
func trimLightMeals(in ChooseInput, prop Proposal) (Proposal, []string) {
	if len(in.State.Light) == 0 {
		return prop, nil
	}
	var notes []string
	out := cloneProposal(prop)
	ev := Evaluate(Merge(in.State, in.Target, out), in.Target)
	for _, pe := range ev.Plates {
		if pe.Intent != LevelLight || pe.MaxKcal <= 0 || pe.Kcal <= pe.MaxKcal+0.5 || pe.PlateID >= 0 {
			continue
		}
		i := int(-pe.PlateID) - 1
		if i < 0 || i >= len(out.Plates) {
			continue
		}
		pl := &out.Plates[i]
		lead := mainID(in.State, *pl)
		for ci, c := range pl.Components {
			f := in.State.Foods[c.FoodID]
			if c.FoodID != lead || c.Portions <= 0 || c.SessionID != 0 || f.Macros.Kcal <= 0 {
				continue
			}
			// The portions that bring the plate to its limit, to 0.05.
			over := pe.Kcal - pe.MaxKcal
			p := math.Floor((c.Portions-over/f.Macros.Kcal)*20) / 20
			if p < 0.5 || p >= c.Portions {
				break
			}
			if c.SessionKey != "" {
				si := slices.IndexFunc(out.Sessions, func(s ProposedSession) bool { return s.Key == c.SessionKey })
				if si < 0 {
					break
				}
				out.Sessions[si].Portions -= pl.eaters(in.State) * (c.Portions - p)
				out.Sessions[si].FrozenPortions = min(out.Sessions[si].FrozenPortions, max(out.Sessions[si].Portions, 0))
			}
			notes = append(notes, fmt.Sprintf("Light meal: %s: %s lowered from %s to %s portions to stay light.", cellShort(in.State, pl.Date, pl.SlotID), f.Name, num(c.Portions), num(p)))
			pl.Components[ci].Portions = p
			break
		}
	}
	if len(notes) == 0 {
		return prop, nil
	}
	return out, notes
}
