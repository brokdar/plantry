package planning

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// PromptVersion is the run log's prompt_version of LLM runs: bump it on
// every change to the prompt or the submit_plan schema.
const PromptVersion = "p6"

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
          "portions": {"type": "number", "description": "Total portions cooked. Must be at least the sum over every plate eating from the session (the plate on the cook cell included) of eaters x component portions; e.g. 3 plates of 1 eater x 1 portion need 3."},
          "frozen_portions": {"type": "number", "description": "Of portions, how many are eaten after the recipe's keep days (plates later than cook_date + keep_days). Each such plate needs eaters x portions here. Only when freezing is on and the recipe is freezable; otherwise 0 and no plate may eat it that late."},
          "reason": {"type": "string", "description": "One short sentence for the user: why this recipe."}
        },
        "required": ["key", "food_id", "cook_date", "cook_slot_id", "portions", "reason"]
      }
    },
    "plates": {
      "type": "array",
      "description": "One plate for EVERY fill cell, all in this call. Every lunch and dinner plate has a main (role main); a standalone recipe (oats, yoghurt) is a breakfast; sides only accompany a main.",
      "items": {
        "type": "object",
        "properties": {
          "plate_id": {"type": "integer", "description": "Only for a side swap: the existing plate that gets the new side."},
          "date": {"type": "string", "description": "YYYY-MM-DD"},
          "slot_id": {"type": "integer"},
          "eaters": {"type": "number", "description": "People eating this plate; omit for the household's portions."},
          "components": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "food_id": {"type": "integer"},
                "portions": {"type": "number", "description": "Portions PER EATER, usually 1, up to 1.5 only if the user asked for more (never the number of eaters: 3 eaters eating 1 portion each consume 3 portions of the session)."},
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
    "notes": {"type": "string", "description": "One or two sentences describing exactly the plan you submit (its recipes and days), nothing it does not contain."}
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
	text, _ := renderPrompt(in)
	return text
}

// renderPrompt is RenderPrompt plus the candidates the token budget left out.
func renderPrompt(in ChooseInput) (string, []FoodFacts) {
	s := in.State
	x := newIndex(s, in.Target)
	var head, tail strings.Builder
	writeRules(&head, s)
	writeDates(&head, s)
	writeSettings(&head, in)
	writeProfile(&head, s.Profile)
	writeDayBudgets(&head, x)
	writeCells(&head, x)
	writeSessions(&head, x)
	writeFixedCooks(&head, x)
	writeKeepTable(&head, in)
	writeHistory(&tail, s)
	writeCorrections(&tail, s)
	writeTask(&tail, in)

	cands := candidates(in)
	rows := make([]string, len(cands))
	for i, f := range cands {
		rows[i] = candidateRow(f, s.Settings.DefaultKeepDays)
	}
	const header = "\nCandidate recipes (id|name|role|suits|diet|kcal|protein_g|fat_g|carbs_g|nutrition_complete|minutes|reference_portions|keep_days|freezable|last_eaten|eaten_8_weeks|favorite|like_score|explore), per portion:\n"
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
	var trimmed []FoodFacts
	for i, r := range rows {
		if !keep[i] {
			omitted++
			trimmed = append(trimmed, cands[i])
			continue
		}
		b.WriteString(r)
		b.WriteByte('\n')
	}
	if omitted > 0 {
		fmt.Fprintf(&b, "(%d more recipes left out for length.)\n", omitted)
	}
	b.WriteString(tail.String())
	return b.String(), trimmed
}

// RenderPlanSummary renders the plan window of s for the chat's system prompt
// (spec §7.6): today, the window's dates and shopping day, the settings, every
// cell with its plate (ids, state, eaters, components), each session's
// portions, frozen portions and consumption, and issues. It lists no candidate
// recipes: the chat reads the library with tools.
func RenderPlanSummary(s State, issues []Issue) string {
	var b strings.Builder
	writeDates(&b, s)
	writeSettings(&b, ChooseInput{State: s})
	x := newIndex(s, Target{})
	writeSummaryCells(&b, x)
	writeSummarySessions(&b, x)
	writeIssues(&b, issues)
	return b.String()
}

func writeSummaryCells(b *strings.Builder, x *index) {
	s := x.s
	b.WriteString("\nCells (date | slot_id | mode | content):\n")
	for _, c := range s.Cells {
		var parts []string
		for _, pi := range x.byCell[idOf(c.Key())] {
			parts = append(parts, summaryPlate(s, s.Plates[pi]))
		}
		content := strings.Join(parts, "; ")
		if content == "" {
			content = "empty"
			if c.Mode == availability.ModeNone {
				content = "no plate, leave empty"
			}
		}
		fmt.Fprintf(b, "%s | %d %s | %s | %s\n", weekdayDate(c.Date), c.SlotID, slotName(s, c.SlotID), c.Mode, content)
	}
}

func summaryPlate(s State, p Plate) string {
	state := "draft"
	if p.Confirmed {
		state = "confirmed"
	}
	parts := make([]string, len(p.Components))
	for i, c := range p.Components {
		var qty string
		switch {
		case c.Portions != nil:
			qty = num(*c.Portions) + " portions/eater"
		case c.Grams != nil:
			qty = num(*c.Grams) + " g/eater"
		}
		parts[i] = fmt.Sprintf("%s (component_id %d, food_id %d", foodName(s, c.FoodID), c.ID, c.FoodID)
		if qty != "" {
			parts[i] += ", " + qty
		}
		if c.SessionID != 0 {
			parts[i] += fmt.Sprintf(", session_id %d", c.SessionID)
		}
		parts[i] += ")"
	}
	eaters := num(p.Eaters) + " eaters"
	if p.Eaters == 1 {
		eaters = "1 eater"
	}
	return fmt.Sprintf("plate_id %d, %s, %s: %s", p.ID, state, eaters, strings.Join(parts, " + "))
}

// writeSummarySessions lists the sessions that cook in the window or feed a
// plate in it, with what their links consume.
func writeSummarySessions(b *strings.Builder, x *index) {
	s := x.s
	linked := map[int64]bool{}
	for _, p := range s.Plates {
		if !s.Window.Contains(p.Date) {
			continue
		}
		for _, c := range p.Components {
			if c.SessionID != 0 {
				linked[c.SessionID] = true
			}
		}
	}
	var lines []string
	for _, ss := range s.Sessions {
		if ss.Proposed() || (!s.Window.Contains(ss.CookDate) && !linked[ss.ID]) {
			continue
		}
		use := x.usage(ss)
		state := "draft"
		if ss.Confirmed {
			state = "confirmed"
		}
		cooked := "not cooked"
		if ss.CookedAt != nil {
			cooked = "cooked"
		}
		keep := x.keepDays(ss.FoodID)
		lines = append(lines, fmt.Sprintf("session_id %d | food_id %d %s | cook %s slot_id %d | %s, %s | portions %s, frozen %s | eaten %s, of which frozen %s | keeps until %s",
			ss.ID, ss.FoodID, foodName(s, ss.FoodID), ss.CookDate.Format(time.DateOnly), ss.CookSlotID, state, cooked,
			num(ss.Portions), num(ss.FrozenPortions), num(use.Total), num(use.Frozen),
			ss.CookDate.AddDate(0, 0, keep).Format(time.DateOnly)))
	}
	if len(lines) == 0 {
		return
	}
	b.WriteString("\nCook sessions:\n")
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
}

func writeIssues(b *strings.Builder, issues []Issue) {
	if len(issues) == 0 {
		b.WriteString("\nIssues: none.\n")
		return
	}
	b.WriteString("\nIssues:\n")
	for _, is := range issues {
		fmt.Fprintf(b, "- %s %s (%s)", is.Rule, is.Code, is.Severity)
		if is.Date != "" {
			fmt.Fprintf(b, " %s", is.Date)
		}
		if is.SlotID != nil {
			fmt.Fprintf(b, " slot_id %d", *is.SlotID)
		}
		if is.PlateID != nil {
			fmt.Fprintf(b, " plate_id %d", *is.PlateID)
		}
		if is.SessionID != nil {
			fmt.Fprintf(b, " session_id %d", *is.SessionID)
		}
		keys := make([]string, 0, len(is.Params))
		for k := range is.Params {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			fmt.Fprintf(b, " %s=%v", k, is.Params[k])
		}
		b.WriteByte('\n')
	}
}

func writeRules(b *strings.Builder, s State) {
	b.WriteString(`You plan the meals of one week for a self-hosted meal planner. Fill every cell marked "fill" with exactly one plate, then call submit_plan once with the complete plan.

How a plan works:
- A plate sits on one (date, slot_id) cell and serves eaters people (omit eaters for the household's portions). Its components are recipes with portions per eater, usually 1.
- A recipe is cooked in a cook session (key, food_id, cook_date, cook_slot_id, portions). A component eats from a session by session_key (a session you propose) or session_id (a fixed session listed below); with neither it is cooked fresh for its own plate.
- Portions are per eater. A plate with 3 eaters and a component of 1 portion consumes 3 portions of its session; never write the number of eaters as portions. A session's portions must cover every plate eating from it: the sum of eaters x portions per eater, the plate on the cook cell included.
- Leftovers are plates eating from a session at a later cell than its cook cell: a later slot of the same day or a later day, at most keep_days after the cook date (see the keep table below for the last allowed date). A day's earlier slot never eats from that day's later cooking. A recipe can feed plates only up to that date: a 3-day curry cooked Saturday cannot feed Wednesday. For later plates cook it again (a new session) or choose another recipe.
- Frozen: a plate after the keep days is a frozen link. It is allowed only when freezing is on and the recipe is freezable (freezable column yes), and the session's frozen_portions must cover every such plate (eaters x portions). Without freezing, or for a non-freezable recipe, no plate may eat it after its last allowed date. frozen_portions never exceeds portions.
- Cooking occasions: every cook cell of a session counts, and so does every component without a session_key/session_id (it is cooked fresh on its own cell). Several sessions cooked on the same cell are one occasion, so cook a side on the same cell as its main, and let other plates eat from a session.
- Every lunch and dinner plate needs a main (role main, suits lunch_dinner in the candidate table); a side alone is not a meal. Recipes that suit breakfast (standalone: oats, yoghurt) are for breakfast cells only, never a lunch or dinner. Pair mains with a side where it suits.
- Submit the COMPLETE plan in your first call: a plate for every fill cell (the task lists them with their count) and every session they need. The checker allows only two repairs; do not use them to find missing cells. notes describe only what is in the plan.
- Daily targets: the day's plates per eater should land near the daily kcal target and the macro shares. The candidate rows list kcal and macros per portion, so sum portions x kcal over the day's plates (weekday meals with 600 kcal against a 2200 target miss it). A day with a No-plate meal has a lower target (see Short days). Mix several different mains over the week rather than one recipe for most meals.
- notes and reasons must describe the plan you submit, not an earlier idea.
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

// writeFixedCooks lists every fixed session that cooks in the window, with
// or without portions left: each distinct cook cell is a cooking occasion of
// its day (H2), and the model cannot cook there again for free.
func writeFixedCooks(b *strings.Builder, x *index) {
	s := x.s
	type occ struct {
		cell  plate.CellKey
		foods []string
	}
	var occs []occ
	for _, ss := range s.Sessions {
		c := ss.CookCell()
		if ss.Proposed() || !s.Window.Contains(c.Date) || x.routine(c) {
			continue
		}
		name := fmt.Sprintf("%s (session %d)", foodName(s, ss.FoodID), ss.ID)
		if i := slices.IndexFunc(occs, func(o occ) bool { return o.cell.Equal(c) }); i >= 0 {
			occs[i].foods = append(occs[i].foods, name)
			continue
		}
		occs = append(occs, occ{cell: c, foods: []string{name}})
	}
	if len(occs) == 0 {
		return
	}
	slices.SortFunc(occs, func(a, c occ) int { return plate.Compare(a.cell, c.cell, s.SlotOrder) })
	b.WriteString("\nCooking already fixed (each is a cooking occasion of its day for H2; cook another session there, on the same cell, or not that day):\n")
	for _, o := range occs {
		fmt.Fprintf(b, "- %s %s slot_id %d: %s\n", weekdayDate(o.cell.Date), slotName(s, o.cell.SlotID), o.cell.SlotID, strings.Join(o.foods, ", "))
	}
}

// candidates are the recipes the chooser may cook: composed, fitting the
// diet (unknown is only a warning), not among the replaced ones; by id.
func candidates(in ChooseInput) []FoodFacts {
	all := eligible(in)
	sw := in.Side
	if sw == nil || sw.Role == "" {
		return all
	}
	// A side swap keeps the side's role: veg stays veg. Without another
	// side of that role the table holds every side (see roleKept).
	if same := slices.DeleteFunc(slices.Clone(all), func(f FoodFacts) bool { return f.Role != sw.Role }); len(same) > 0 {
		return same
	}
	if sides := slices.DeleteFunc(slices.Clone(all), func(f FoodFacts) bool { return !sideRole(f.Role) }); len(sides) > 0 {
		return sides
	}
	return all
}

// roleKept reports whether a side swap's candidates keep the replaced
// side's role: false for no swap, a role-less side, or when no other
// recipe has that role.
func roleKept(in ChooseInput) bool {
	sw := in.Side
	if sw == nil || sw.Role == "" {
		return false
	}
	for _, f := range eligible(in) {
		if f.Role == sw.Role {
			return true
		}
	}
	return false
}

// eligible are the recipes fitting the diet that are not among the
// replaced ones, by id.
func eligible(in ChooseInput) []FoodFacts {
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
// swap, then explore recipes, favorites, never eaten ones, then the longest
// ago eaten; ties by id.
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
		if ae, be := a.Explore(), b.Explore(); ae != be {
			if ae {
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

func candidateRow(f FoodFacts, defaultKeep int) string {
	last := "-"
	if f.LastEaten != nil {
		last = f.LastEaten.Format(time.DateOnly)
	}
	keep := fmt.Sprint(defaultKeep)
	if f.KeepDays != nil {
		keep = fmt.Sprint(*f.KeepDays)
	}
	role := string(f.Role)
	if role == "" {
		role = "-"
	}
	return fmt.Sprintf("%d|%s|%s|%s|%s|%.0f|%.0f|%.0f|%.0f|%s|%d|%s|%s|%s|%s|%d|%s|%s|%s",
		f.ID, cell(f.Name), role, suits(f.Role), f.DietClass, f.Macros.Kcal, f.Macros.Protein, f.Macros.Fat, f.Macros.Carbs,
		yesNo(f.NutritionComplete), f.Minutes, num(f.ReferencePortions), keep, yesNo(f.Freezable), last,
		f.TimesEaten8Weeks, yesNo(f.Favorite), num(f.LikeScore), yesNo(f.Explore()))
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

// maxCorrectionLines bounds the corrections section.
const maxCorrectionLines = 20

// writeCorrections lists the recipes the user swapped out of plates in the
// last 4 weeks (the newest first), so the plan does not propose them again.
func writeCorrections(b *strings.Builder, s State) {
	var lines []string
	for _, c := range slices.Backward(s.Corrections) {
		if len(lines) == maxCorrectionLines {
			break
		}
		var removed, added []string
		for _, id := range c.Removed {
			if _, ok := s.Foods[id]; ok {
				removed = append(removed, foodName(s, id))
			}
		}
		for _, id := range c.Added {
			if _, ok := s.Foods[id]; ok {
				added = append(added, foodName(s, id))
			}
		}
		if len(removed) == 0 {
			continue
		}
		to := "something else"
		if len(added) > 0 {
			to = strings.Join(added, " + ")
		}
		lines = append(lines, fmt.Sprintf("%s -> %s (%s %s, %s)", strings.Join(removed, " + "), to,
			c.Date.Weekday().String()[:3], slotName(s, c.SlotID), c.Date.Format(time.DateOnly)))
	}
	if len(lines) == 0 {
		return
	}
	b.WriteString("\nThe user replaced these planned recipes in the last 4 weeks (planned -> chosen instead); do not propose the planned ones for the same kind of meal again:\n")
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
}

func writeTask(b *strings.Builder, in ChooseInput) {
	b.WriteString("\nYour task:\n")
	if sw := in.Side; sw != nil {
		fmt.Fprintf(b, "- Replace only the side %s (food_id %d): propose exactly one session with key s1 of another side recipe, cooked %s slot_id %d with %s portions (%s frozen), and for each plate below one plate entry with its plate_id, date, slot_id and one component eating from s1:\n",
			foodName(in.State, sw.FoodID), sw.FoodID, sw.CookDate.Format(time.DateOnly), sw.CookSlotID, num(sw.Portions), num(sw.FrozenPortions))
		for _, p := range sw.Plates {
			fmt.Fprintf(b, "  plate_id %d on %s slot_id %d, %s eaters, %s portions per eater%s\n",
				p.PlateID, p.Date.Format(time.DateOnly), p.SlotID, num(p.Eaters), num(p.PerEater), plateKeeps(in.State, p.PlateID, sw.FoodID))
		}
		if sw.Role != "" && !roleKept(in) {
			fmt.Fprintf(b, "- No other recipe has the role %s of %s: choose any side from the candidate table; what a plate keeps is listed above, do not add a second side of a kind it already has.\n", sw.Role, foodName(in.State, sw.FoodID))
		} else if sw.Role != "" {
			fmt.Fprintf(b, "- Keep the role: %s has role %s, so the new side must have role %s (the candidate table holds only those); what a plate keeps is listed above, do not add a second side of a kind it already has.\n", foodName(in.State, sw.FoodID), sw.Role, sw.Role)
		}
	} else {
		writeFillCells(b, in)
		b.WriteString("- Plan every fill cell. Prefer cooking in batches that later cells eat as leftovers, eat the fixed sessions' free portions first, and pair a main with a side where it suits.\n")
	}
	if in.ExploreQuota > 0 {
		fmt.Fprintf(b, "- Include %d recipes marked explore (explore column yes).\n", in.ExploreQuota)
	}
	if i := strings.TrimSpace(in.Instruction); i != "" {
		fmt.Fprintf(b, "- The user's request for this plan: %s\n", oneLine(i))
	}
	fmt.Fprintf(b, "- Write notes and every reason in %s.\n", language(in.Locale))
}

// plateKeeps names what a side-swap plate keeps next to the replaced side.
func plateKeeps(s State, id, replaced int64) string {
	i := slices.IndexFunc(s.Plates, func(p Plate) bool { return p.ID == id })
	if i < 0 {
		return ""
	}
	var parts []string
	for _, c := range s.Plates[i].Components {
		if c.FoodID == replaced {
			continue
		}
		role := "-"
		if f, ok := s.Foods[c.FoodID]; ok && f.Role != "" {
			role = string(f.Role)
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", foodName(s, c.FoodID), role))
	}
	if len(parts) == 0 {
		return ""
	}
	return "; the plate keeps " + strings.Join(parts, " + ")
}

// fillCellLabels lists the cells the chooser must fill: empty plan cells in
// the target. Target.Cells alone also holds No-plate, routine and occupied
// cells, so it is no count of what is missing.
func fillCellLabels(in ChooseInput) []string {
	x := newIndex(in.State, in.Target)
	var labels []string
	for _, c := range in.State.Cells {
		if c.Mode == availability.ModePlan && x.inTarget(c.Key()) && len(x.byCell[idOf(c.Key())]) == 0 {
			labels = append(labels, fmt.Sprintf("%s %s (slot_id %d)", weekdayDate(c.Date), slotName(in.State, c.SlotID), c.SlotID))
		}
	}
	return labels
}

// writeFillCells states the number of fill cells and lists them, so the
// first submit_plan is complete.
func writeFillCells(b *strings.Builder, in ChooseInput) {
	labels := fillCellLabels(in)
	if len(labels) == 0 {
		return
	}
	fmt.Fprintf(b, "- Submit the complete plan in one call: %d fill cells, so exactly %d plates, every session they need included: %s.\n", len(labels), len(labels), strings.Join(labels, "; "))
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

// writeKeepTable precomputes the last day a batch cooked on each date of
// the week can still be eaten, for every keep_days value among the
// candidates, so the model does not count days itself.
func writeKeepTable(b *strings.Builder, in ChooseInput) {
	s := in.State
	keeps := map[int]bool{}
	for _, f := range candidates(in) {
		k := s.Settings.DefaultKeepDays
		if f.KeepDays != nil {
			k = *f.KeepDays
		}
		keeps[k] = true
	}
	if len(keeps) == 0 {
		return
	}
	b.WriteString("\nKeep table (a batch cooked on a date feeds plates up to this date; later plates need frozen_portions or a new session):\n")
	for _, k := range slices.Sorted(maps.Keys(keeps)) {
		parts := make([]string, 0, 7)
		for _, d := range s.Window.Days() {
			parts = append(parts, fmt.Sprintf("%s->%s", d.Format("01-02"), d.AddDate(0, 0, k).Format("01-02")))
		}
		fmt.Fprintf(b, "- keep_days %d: %s\n", k, strings.Join(parts, ", "))
	}
}
