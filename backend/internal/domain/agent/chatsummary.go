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
// window with its role and kcal and protein per portion (a leaf: per 100 g),
// and every day's kcal per eater against the target.
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
	b.WriteString("\nFoods on the plan (role, kcal and protein per portion; leaf foods per 100 g):\n")
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
		fmt.Fprintf(&b, "food_id %d %s | %s | %.0f kcal, %.0f g protein %s\n", f.ID, f.Name, role, f.Macros.Kcal, f.Macros.Protein, per)
	}
	// Day figures come from the evaluation; the day's target is scaled to its
	// planned meals (a "No plate" meal asks less).
	target := ""
	if k := st.Profile.KcalTarget; k != nil && *k > 0 {
		target = fmt.Sprintf(" (target %.0f; a day with a No-plate meal shows its scaled target)", *k)
	}
	withPlates := map[string]bool{}
	ev := planning.Evaluate(st, planning.WindowTarget(st))
	for _, p := range ev.Plates {
		withPlates[p.Date] = true
	}
	b.WriteString("\nKcal per eater and day" + target + ":\n")
	for _, d := range ev.Days {
		if !withPlates[d.Date] {
			continue
		}
		mark := ""
		if d.NutritionMissing {
			mark = " (some nutrition data missing)"
		}
		if d.TargetKcal != nil && st.Profile.KcalTarget != nil && math.Abs(*d.TargetKcal-*st.Profile.KcalTarget) > 0.5 {
			mark = fmt.Sprintf(" against %.0f%s", *d.TargetKcal, mark)
		}
		fmt.Fprintf(&b, "%s: %.0f kcal%s\n", d.Date, d.Kcal, mark)
	}
	return b.String()
}
