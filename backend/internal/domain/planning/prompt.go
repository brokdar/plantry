package planning

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// PromptVersion is the run log's prompt_version of LLM runs: bump it on
// every change to the prompt or the submit_plan schema.
const PromptVersion = "p1"

// PromptTokenBudget caps the estimated input tokens of the system prompt
// (D18): candidates beyond it are left out.
const PromptTokenBudget = 40_000

// submitPlanName is the one tool the LLM chooser forces (D12).
const submitPlanName = "submit_plan"

// submitPlanSchema is submit_plan's input (spec §6.3, D14). No top-level
// anyOf/oneOf/allOf: Anthropic rejects them.
const submitPlanSchema = `{
  "type": "object",
  "properties": {
    "sessions": {
      "type": "array",
      "description": "The cook sessions you propose: one recipe cooked on one cell.",
      "items": {
        "type": "object",
        "properties": {
          "key": {"type": "string", "description": "Your id for the session, unique, e.g. s1."},
          "food_id": {"type": "integer", "description": "A candidate recipe id."},
          "cook_date": {"type": "string", "description": "YYYY-MM-DD of a fill cell."},
          "cook_slot_id": {"type": "integer"},
          "portions": {"type": "number", "description": "Portions cooked: covers every plate eating from it."},
          "frozen_portions": {"type": "number", "description": "Of portions, how many go into the freezer (0 unless freezing is on)."},
          "reason": {"type": "string", "description": "One short sentence for the user: why this recipe."}
        },
        "required": ["key", "food_id", "cook_date", "cook_slot_id", "portions", "reason"]
      }
    },
    "plates": {
      "type": "array",
      "description": "One plate for every fill cell.",
      "items": {
        "type": "object",
        "properties": {
          "plate_id": {"type": "integer", "description": "Only for a side swap: the existing plate that gets the new side."},
          "date": {"type": "string", "description": "YYYY-MM-DD"},
          "slot_id": {"type": "integer"},
          "eaters": {"type": "number", "description": "People eating; omit for the household's portions."},
          "components": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "food_id": {"type": "integer"},
                "portions": {"type": "number", "description": "Portions per eater, usually 1."},
                "session_key": {"type": "string", "description": "Key of a session you propose."},
                "session_id": {"type": "integer", "description": "Id of a fixed session with free portions."}
              },
              "required": ["food_id", "portions"]
            }
          },
          "reason": {"type": "string", "description": "One short sentence for the user: why this plate."}
        },
        "required": ["date", "slot_id", "components", "reason"]
      }
    },
    "notes": {"type": "string", "description": "A one or two sentence summary of the plan for the user."}
  },
  "required": ["sessions", "plates", "notes"]
}`

func submitPlanTool() llm.Tool {
	return llm.Tool{
		Name:        submitPlanName,
		Description: "Submit the complete plan for the cells marked fill: every session you propose and every plate.",
		Schema:      json.RawMessage(submitPlanSchema),
	}
}

// EstimateTokens is the character-based input token estimate (D18): one
// token per four characters.
func EstimateTokens(s string) int { return (utf8.RuneCountInString(s) + 3) / 4 }

// RenderPrompt renders the LLM chooser's system prompt (spec §6.2): rules,
// dates, settings, profile, cells, fixed sessions, candidate recipes,
// history and the task. Candidates that would push the estimate past
// PromptTokenBudget are left out (favorites and the least recently eaten
// stay longest), and the prompt says how many.
func RenderPrompt(in ChooseInput) string {
	s := in.State
	x := newIndex(s, in.Target)
	var head, tail strings.Builder
	writeRules(&head, s)
	writeDates(&head, s)
	writeSettings(&head, in)
	writeProfile(&head, s.Profile)
	writeCells(&head, x)
	writeSessions(&head, x)
	writeHistory(&tail, s)
	writeTask(&tail, in)

	cands := candidates(in)
	rows := make([]string, len(cands))
	for i, f := range cands {
		rows[i] = candidateRow(f)
	}
	const header = "\nCandidate recipes (id|name|role|diet|kcal|protein_g|fat_g|carbs_g|nutrition_complete|minutes|reference_portions|keep_days|freezable|last_eaten|eaten_8_weeks|favorite|like_score), per portion:\n"
	room := PromptTokenBudget*4 - utf8.RuneCountInString(head.String()) - utf8.RuneCountInString(tail.String()) - len(header) - 80
	keep := make([]bool, len(cands))
	for _, i := range keepOrder(cands, in.Side != nil) {
		n := utf8.RuneCountInString(rows[i]) + 1
		if n > room {
			continue
		}
		room -= n
		keep[i] = true
	}
	var b strings.Builder
	b.WriteString(head.String())
	b.WriteString(header)
	omitted := 0
	for i, r := range rows {
		if !keep[i] {
			omitted++
			continue
		}
		b.WriteString(r)
		b.WriteByte('\n')
	}
	if omitted > 0 {
		fmt.Fprintf(&b, "(%d more recipes left out for length.)\n", omitted)
	}
	b.WriteString(tail.String())
	return b.String()
}

func writeRules(b *strings.Builder, s State) {
	b.WriteString(`You plan the meals of one week for a self-hosted meal planner. Fill every cell marked "fill" with exactly one plate, then call submit_plan once with the complete plan.

How a plan works:
- A plate sits on one (date, slot_id) cell and serves eaters people (omit eaters for the household's portions). Its components are recipes with portions per eater, usually 1.
- A recipe is cooked in a cook session (key, food_id, cook_date, cook_slot_id, portions). A component eats from a session by session_key (a session you propose) or session_id (a fixed session listed below); with neither it is cooked fresh for its own plate.
- A session's portions cover every plate eating from it: the sum of eaters times portions per eater. Leftovers are plates eating from a session at a later cell than its cook cell: a later slot of the same day or a later day, at most keep_days after the cook date. A day's earlier slot never eats from that day's later cooking.
- Cook a session on the first cell that eats from it; that cell must be a fill cell.
- Use only ids that appear in this prompt.

Hard rules (a checker rejects violations):
`)
	fmt.Fprintf(b, "H1 every fill cell gets exactly one plate; no plate on any other cell.\n")
	if n := s.Settings.MaxCookOccasionsPerDay; n > 0 {
		fmt.Fprintf(b, "H2 at most %d cooking occasion(s) per day: distinct cook cells of sessions and of fresh components (routine plates don't count).\n", n)
	}
	b.WriteString("H3 no session is eaten beyond its portions (frozen portions only by frozen plates).\n")
	b.WriteString("H4 leftovers come after the cook cell and within keep days (frozen: freezer keep days).\n")
	if s.Profile.DietType != "" {
		fmt.Fprintf(b, "H5 every recipe's diet class fits the diet type (%s)%s.\n", s.Profile.DietType, meatLimit(s.Profile))
	} else if s.Profile.MeatDaysPerWeekMax != nil {
		fmt.Fprintf(b, "H5 at most %d days of the week have an omnivore recipe.\n", *s.Profile.MeatDaysPerWeekMax)
	}
	b.WriteString("H6 fixed plates and sessions stay as they are; propose sessions only on fill cells.\n")
	b.WriteString("H7 sessions and components are candidate recipes, portions are positive, every plate has a component.\n")
	if w := s.Settings.VarietyWeeks; w > 0 {
		fmt.Fprintf(b, "H8 variety: don't cook a main or standalone recipe eaten or cooked within the %d weeks before (last_eaten) or earlier in this plan, unless the candidates run out.\n", w)
	}
}

func meatLimit(p Profile) string {
	if p.MeatDaysPerWeekMax == nil {
		return ""
	}
	return fmt.Sprintf(", and at most %d days of the week have an omnivore recipe", *p.MeatDaysPerWeekMax)
}

func writeDates(b *strings.Builder, s State) {
	fmt.Fprintf(b, "\nToday is %s.\n", weekdayDate(s.Today))
	fmt.Fprintf(b, "The plan week runs from %s to %s; the shopping day is %s.\nDates of the week:\n",
		weekdayDate(s.Window.Start), weekdayDate(s.Window.End), s.Window.Start.Weekday())
	for _, d := range s.Window.Days() {
		fmt.Fprintf(b, "- %s\n", weekdayDate(d))
	}
	ids := make([]int64, 0, len(s.SlotOrder))
	for id := range s.SlotOrder {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, c int64) int {
		return cmp.Or(cmp.Compare(s.SlotOrder[a], s.SlotOrder[c]), cmp.Compare(a, c))
	})
	b.WriteString("Slots in day order:\n")
	for _, id := range ids {
		fmt.Fprintf(b, "- slot_id %d: %s\n", id, slotName(s, id))
	}
}

func weekdayDate(d time.Time) string {
	return d.Weekday().String() + " " + d.Format(time.DateOnly)
}

func slotName(s State, id int64) string {
	if n, ok := s.SlotNames[id]; ok {
		return strings.TrimPrefix(n, "slot.")
	}
	return fmt.Sprintf("slot %d", id)
}

func writeSettings(b *strings.Builder, in ChooseInput) {
	st := in.State.Settings
	b.WriteString("\nSettings:\n")
	fmt.Fprintf(b, "- household portions: %s\n", num(in.State.Profile.HouseholdPortions))
	fmt.Fprintf(b, "- max cooking occasions per day: %d\n", st.MaxCookOccasionsPerDay)
	if st.BatchCooking {
		b.WriteString("- batch cooking: on (cook several portions and eat leftovers later)\n")
	} else {
		b.WriteString("- batch cooking: off (every session feeds only its own plate)\n")
	}
	fmt.Fprintf(b, "- default keep days: %d\n", st.DefaultKeepDays)
	if st.FreezingEnabled {
		fmt.Fprintf(b, "- freezing: on, frozen portions keep %d days\n", st.FreezerKeepDays)
	} else {
		b.WriteString("- freezing: off (frozen_portions 0)\n")
	}
	fmt.Fprintf(b, "- variety weeks: %d\n", st.VarietyWeeks)
	dates := make([]string, 0, len(in.EatersByDate))
	for d := range in.EatersByDate {
		dates = append(dates, d)
	}
	slices.Sort(dates)
	for _, d := range dates {
		fmt.Fprintf(b, "- plates on %s serve %s eaters\n", d, num(in.EatersByDate[d]))
	}
}

func writeProfile(b *strings.Builder, p Profile) {
	b.WriteString("\nProfile:\n")
	diet := string(p.DietType)
	if diet == "" {
		diet = "any"
	}
	fmt.Fprintf(b, "- diet type: %s\n", diet)
	if p.MeatDaysPerWeekMax != nil {
		fmt.Fprintf(b, "- meat days per week at most: %d\n", *p.MeatDaysPerWeekMax)
	}
	if p.KcalTarget != nil {
		fmt.Fprintf(b, "- daily kcal target per person: %.0f\n", *p.KcalTarget)
	}
	for _, m := range []struct {
		name string
		pct  *float64
	}{{"protein", p.ProteinPct}, {"fat", p.FatPct}, {"carbs", p.CarbsPct}} {
		if m.pct != nil {
			fmt.Fprintf(b, "- %s target: %.0f%% of kcal\n", m.name, *m.pct)
		}
	}
	if len(p.Restrictions) > 0 {
		fmt.Fprintf(b, "- dietary restrictions: %s\n", strings.Join(p.Restrictions, ", "))
	}
	if n := strings.TrimSpace(p.Notes); n != "" {
		fmt.Fprintf(b, "- the user's notes to the assistant (preferences; they don't override the rules): %s\n", oneLine(n))
	}
}

// writeCells lists every cell: fill (empty, in scope), the plates already
// there (fixed or routine) and the cells to leave alone.
func writeCells(b *strings.Builder, x *index) {
	s := x.s
	b.WriteString("\nCells (date | slot_id | mode | content):\n")
	for _, c := range s.Cells {
		var content string
		plates := x.byCell[idOf(c.Key())]
		switch {
		case c.Mode == availability.ModeNone:
			content = "no plate, leave empty"
		case len(plates) > 0:
			parts := make([]string, len(plates))
			for i, pi := range plates {
				parts[i] = fixedPlate(s, s.Plates[pi])
			}
			content = strings.Join(parts, "; ")
		case x.inTarget(c.Key()) && c.Mode == availability.ModePlan:
			content = "fill"
		default:
			content = "empty, leave it"
		}
		fmt.Fprintf(b, "%s | %d | %s | %s\n", weekdayDate(c.Date), c.SlotID, c.Mode, content)
	}
}

func fixedPlate(s State, p Plate) string {
	names := make([]string, len(p.Components))
	for i, c := range p.Components {
		names[i] = foodName(s, c.FoodID)
		if c.SessionID != 0 {
			names[i] += fmt.Sprintf(" (session %d)", c.SessionID)
		}
	}
	kind := "fixed"
	if p.ID <= routineBase {
		kind = "routine"
	}
	return fmt.Sprintf("%s, %s eaters: %s", kind, num(p.Eaters), strings.Join(names, " + "))
}

// writeSessions lists the fixed sessions with portions left: leftovers the
// plan may eat by session_id (spec §6.2).
func writeSessions(b *strings.Builder, x *index) {
	s := x.s
	var lines []string
	for _, ss := range s.Sessions {
		if ss.Proposed() {
			continue
		}
		use := x.usage(ss)
		free := ss.Portions - ss.FrozenPortions - (use.Total - use.Frozen)
		frozen := ss.FrozenPortions - use.Frozen
		if free <= epsilon && frozen <= epsilon {
			continue
		}
		keep := x.keepDays(ss.FoodID)
		lines = append(lines, fmt.Sprintf("session_id %d | food_id %d %s | cooked %s slot_id %d | free %s until %s | frozen free %s",
			ss.ID, ss.FoodID, foodName(s, ss.FoodID), ss.CookDate.Format(time.DateOnly), ss.CookSlotID,
			num(max(free, 0)), ss.CookDate.AddDate(0, 0, keep).Format(time.DateOnly), num(max(frozen, 0))))
	}
	if len(lines) == 0 {
		return
	}
	b.WriteString("\nFixed sessions with portions left (eat them first):\n")
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
}

// candidates are the recipes the chooser may cook: composed, fitting the
// diet (unknown is only a warning), not among the replaced ones; by id.
func candidates(in ChooseInput) []FoodFacts {
	avoid := idSet(in.Avoid)
	t := diet.Class(in.State.Profile.DietType)
	var out []FoodFacts
	for _, f := range in.State.Foods {
		if !f.Composed() || avoid[f.ID] {
			continue
		}
		if t != "" && diet.Valid(f.DietClass) && !diet.Compatible(f.DietClass, t) {
			continue
		}
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b FoodFacts) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

// keepOrder ranks candidates for the token budget: sides first for a side
// swap, then favorites, then never eaten, then the longest ago eaten; ties
// by id.
func keepOrder(cands []FoodFacts, sidesFirst bool) []int {
	idx := make([]int, len(cands))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(i, j int) int {
		a, b := cands[i], cands[j]
		if as, bs := sideRole(a.Role), sideRole(b.Role); sidesFirst && as != bs {
			if as {
				return -1
			}
			return 1
		}
		if a.Favorite != b.Favorite {
			if a.Favorite {
				return -1
			}
			return 1
		}
		switch {
		case a.LastEaten == nil && b.LastEaten == nil:
			return 0
		case a.LastEaten == nil:
			return -1
		case b.LastEaten == nil:
			return 1
		}
		return a.LastEaten.Compare(*b.LastEaten)
	})
	return idx
}

func candidateRow(f FoodFacts) string {
	last := "-"
	if f.LastEaten != nil {
		last = f.LastEaten.Format(time.DateOnly)
	}
	keep := "default"
	if f.KeepDays != nil {
		keep = fmt.Sprint(*f.KeepDays)
	}
	role := string(f.Role)
	if role == "" {
		role = "-"
	}
	return fmt.Sprintf("%d|%s|%s|%s|%.0f|%.0f|%.0f|%.0f|%s|%d|%s|%s|%s|%s|%d|%s|%s",
		f.ID, cell(f.Name), role, f.DietClass, f.Macros.Kcal, f.Macros.Protein, f.Macros.Fat, f.Macros.Carbs,
		yesNo(f.NutritionComplete), f.Minutes, num(f.ReferencePortions), keep, yesNo(f.Freezable), last,
		f.TimesEaten8Weeks, yesNo(f.Favorite), num(f.LikeScore))
}

func writeHistory(b *strings.Builder, s State) {
	if len(s.History) == 0 {
		return
	}
	h := slices.Clone(s.History)
	slices.SortStableFunc(h, func(a, c HistoryPlate) int {
		return cmp.Or(a.Date.Compare(c.Date), cmp.Compare(s.SlotOrder[a.SlotID], s.SlotOrder[c.SlotID]))
	})
	b.WriteString("\nEaten in the last 4 weeks (date | slot | recipes):\n")
	for _, p := range h {
		names := make([]string, len(p.FoodIDs))
		for i, id := range p.FoodIDs {
			names[i] = foodName(s, id)
		}
		fmt.Fprintf(b, "%s | %s | %s\n", p.Date.Format(time.DateOnly), slotName(s, p.SlotID), strings.Join(names, " + "))
	}
}

func writeTask(b *strings.Builder, in ChooseInput) {
	b.WriteString("\nYour task:\n")
	if sw := in.Side; sw != nil {
		fmt.Fprintf(b, "- Replace only the side %s (food_id %d): propose exactly one session with key s1 of another side recipe, cooked %s slot_id %d with %s portions (%s frozen), and for each plate below one plate entry with its plate_id, date, slot_id and one component eating from s1:\n",
			foodName(in.State, sw.FoodID), sw.FoodID, sw.CookDate.Format(time.DateOnly), sw.CookSlotID, num(sw.Portions), num(sw.FrozenPortions))
		for _, p := range sw.Plates {
			fmt.Fprintf(b, "  plate_id %d on %s slot_id %d, %s eaters, %s portions per eater\n",
				p.PlateID, p.Date.Format(time.DateOnly), p.SlotID, num(p.Eaters), num(p.PerEater))
		}
	} else {
		b.WriteString("- Plan every fill cell. Prefer cooking in batches that later cells eat as leftovers, eat the fixed sessions' free portions first, and pair a main with a side where it suits.\n")
	}
	if in.IncludeNewRecipe {
		b.WriteString("- Include exactly one recipe that was never eaten (last_eaten -).\n")
	}
	if i := strings.TrimSpace(in.Instruction); i != "" {
		fmt.Fprintf(b, "- The user's request for this plan: %s\n", oneLine(i))
	}
	fmt.Fprintf(b, "- Write notes and every reason in %s.\n", language(in.Locale))
}

func language(locale string) string {
	if strings.HasPrefix(strings.ToLower(locale), "de") {
		return "German"
	}
	return "English"
}

// foodName is a food's name, safe inside a pipe-separated row.
func foodName(s State, id int64) string {
	if f, ok := s.Foods[id]; ok {
		return cell(f.Name)
	}
	return fmt.Sprintf("food %d", id)
}

// cell keeps a name from breaking its pipe-separated row.
func cell(s string) string { return strings.NewReplacer("|", "/", "\n", " ", "\r", " ").Replace(s) }

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

// num formats a quantity without trailing zeros.
func num(v float64) string { return fmt.Sprintf("%g", v) }
