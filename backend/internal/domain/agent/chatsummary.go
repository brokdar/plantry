package agent

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// chatIssues are the issues the chat prompt shows: what the board shows
// (planning.BoardIssues: H1, H2, H5, H8) plus H3/H4. The board reports those
// as session warnings, but the prompt has no session warnings, so a batch
// eaten 8 of 6 would read "Issues: none."
func chatIssues(st planning.State) []planning.Issue {
	out := planning.BoardIssues(st)
	t := planning.Target{}
	for _, c := range st.Cells {
		t.Cells = append(t.Cells, c.Key())
	}
	for _, is := range planning.Check(st, t) {
		if is.Rule == "H3" || is.Rule == "H4" {
			out = append(out, is)
		}
	}
	return out
}

// chatFacts renders what the summary lacks: each food on a plate of the
// window with its role and kcal and protein per portion (a leaf: per 100 g),
// and every day's kcal per eater against the target.
func chatFacts(st planning.State) string {
	var foods []int64
	seen := map[int64]bool{}
	days := map[string]float64{}
	incomplete := map[string]bool{}
	for _, p := range st.Plates {
		if !st.Window.Contains(p.Date) || p.Proposed() {
			continue
		}
		d := p.Date.Format(time.DateOnly)
		for _, c := range p.Components {
			f, ok := st.Foods[c.FoodID]
			if !ok {
				continue
			}
			if !seen[c.FoodID] {
				seen[c.FoodID] = true
				foods = append(foods, c.FoodID)
			}
			days[d] += f.Macros.Kcal * c.Multiplier()
			if !f.NutritionComplete {
				incomplete[d] = true
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
	target := ""
	if k := st.Profile.KcalTarget; k != nil && *k > 0 {
		target = fmt.Sprintf(" (target %.0f)", *k)
	}
	b.WriteString("\nKcal per eater and day" + target + ":\n")
	keys := make([]string, 0, len(days))
	for d := range days {
		keys = append(keys, d)
	}
	slices.Sort(keys)
	for _, d := range keys {
		mark := ""
		if incomplete[d] {
			mark = " (some nutrition data missing)"
		}
		fmt.Fprintf(&b, "%s: %.0f kcal%s\n", d, days[d], mark)
	}
	return b.String()
}
