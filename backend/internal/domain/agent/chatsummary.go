package agent

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// chatView is the plan's evaluation with the issues the chat prompt and the
// user see: what the board shows (planning.BoardIssues: H1, H2, H5 including
// the meat-day limit, H8), the session rules H3/H4 (the board reports them
// as session warnings, but the prompt has none, so a batch eaten 8 of 6
// would read "Issues: none.") and the soft rules S2/S3 of Evaluate with
// their options. Check's full list stays in AllIssues.
func chatView(st planning.State) planning.Evaluation {
	ev := planning.Evaluate(st, planning.WindowTarget(st))
	out := planning.BoardIssues(st)
	for _, is := range ev.Issues {
		if is.Rule == "H3" || is.Rule == "H4" || is.Severity == planning.SeveritySoft {
			out = append(out, is)
		}
	}
	return ev.ViewedAs(out)
}

// chatIssues are the issues of chatView.
func chatIssues(st planning.State) []planning.Issue { return chatView(st).Issues }

// evaluateAsSeen is chatView, the evaluation the run log stores.
func evaluateAsSeen(st planning.State) planning.Evaluation { return chatView(st) }

// chatFacts renders what the summary lacks: each food on a plate of the
// window with its role, diet class, a recipe's total minutes (prep + cook,
// when known) and kcal and protein per portion (a leaf: per 100 g), and every
// day's kcal per eater with its target, band, floor, room to the band top,
// the slots that cook and the light or hearty meals. The prompt puts it before
// the cells, so a cut never takes the computed facts.
func chatFacts(st planning.State) string {
	var foods []int64
	seen := map[int64]bool{}
	for _, p := range st.Plates {
		if !st.Window.Contains(p.Date) || p.Proposed() {
			continue
		}
		for _, c := range p.Components {
			_, ok := st.Foods[c.FoodID]
			if !ok {
				continue
			}
			if !seen[c.FoodID] {
				seen[c.FoodID] = true
				foods = append(foods, c.FoodID)
			}
		}
	}
	if len(foods) == 0 {
		return ""
	}
	slices.Sort(foods)
	var b strings.Builder
	b.WriteString("\nFoods on the plan (role, diet class, a recipe's total minutes, kcal and protein per portion, a leaf per 100 g; other foods: find_foods):\n")
	for _, id := range foods {
		f := st.Foods[id]
		role := string(f.Role)
		if role == "" {
			role = "no role"
		}
		per := "per portion"
		if !f.Composed() {
			per = "per 100 g"
		}
		minutes := ""
		if f.Composed() && f.Minutes > 0 {
			minutes = fmt.Sprintf(" | %d min", f.Minutes)
		}
		class := ""
		if f.DietClass != "" {
			class = " | " + string(f.DietClass)
		}
		fmt.Fprintf(&b, "food_id %d %s | %s%s%s | %.0f kcal, %.0f g protein %s\n", f.ID, f.Name, role, class, minutes, f.Macros.Kcal, f.Macros.Protein, per)
	}
	// Day figures come from the evaluation; the day's target is scaled to its
	// planned meals (a "No plate" meal asks less) and moved by light meals.
	target := ""
	if k := st.Profile.KcalTarget; k != nil && *k > 0 {
		target = fmt.Sprintf(" (normal target %.0f)", *k)
	}
	withPlates := map[string]bool{}
	ev := planning.Evaluate(st, planning.WindowTarget(st))
	for _, p := range ev.Plates {
		withPlates[p.Date] = true
	}
	b.WriteString("\nKcal per eater and day" + target + "; then target if moved, S2 band, H9 floor, room to the band top, cooking slots, light or hearty meals:\n")
	for _, d := range ev.Days {
		if !withPlates[d.Date] {
			continue
		}
		fmt.Fprintf(&b, "%s: %.0f kcal", d.Date, d.Kcal)
		if d.NutritionMissing {
			b.WriteString(" (some nutrition data missing)")
		}
		if d.TargetKcal != nil && st.Profile.KcalTarget != nil && math.Abs(*d.TargetKcal-*st.Profile.KcalTarget) > 0.5 {
			fmt.Fprintf(&b, " | target %.0f", *d.TargetKcal)
		}
		if d.BandHigh > 0 {
			fmt.Fprintf(&b, " | band %.0f-%.0f | floor %.0f | room %.0f", d.BandLow, d.BandHigh, d.FloorKcal, d.RoomKcal)
		}
		if len(d.CookSlots) > 0 {
			b.WriteString(" | cooks " + strings.Join(d.CookSlots, ", "))
		}
		if len(d.LightMeals) > 0 {
			b.WriteString(" | " + strings.Join(d.LightMeals, ", "))
		}
		b.WriteByte('\n')
	}
	return b.String()
}
