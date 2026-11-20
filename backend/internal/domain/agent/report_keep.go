package agent

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// keepNoticeDays is how many days after cooking a meal eats a batch before
// the tell says how long that food keeps.
const keepNoticeDays = 3

// keepFor is a session's food's keep days: its own, else the plan default.
func keepFor(st planning.State, foodID int64) int {
	return st.Settings.KeepDays(st.Foods[foodID].KeepDays)
}

// keepLines says, for each changed meal that eats a batch cooked 3 or more
// days before (and still in the fridge), how long the food keeps and the last
// day for it.
func keepLines(in reportInput, changed []reportMeal) []string {
	st := in.afterSt
	var out []string
	for _, m := range changed {
		if len(m.After) == 0 {
			continue
		}
		for _, p := range st.Plates {
			if dayLabel(p.Date) != m.Day || slotWord(st, p.SlotID) != m.Meal {
				continue
			}
			for _, c := range p.Components {
				if c.SessionID == 0 {
					continue
				}
				i := slices.IndexFunc(st.Sessions, func(s planning.Session) bool { return s.ID == c.SessionID })
				if i < 0 {
					continue
				}
				cook := st.Sessions[i].CookDate
				days := int(p.Date.Sub(cook).Hours()/24 + 0.5)
				keep := keepFor(st, c.FoodID)
				if days < keepNoticeDays || days > keep {
					continue
				}
				line := fmt.Sprintf("%s %s eats the %s cooked %s, %d days earlier; it keeps %s in the fridge, so the last day is %s.",
					m.Day, m.Meal, food.Quote(foodName(st, c.FoodID)), dayLabel(cook), days, countDays(keep, false), dayLabel(cook.AddDate(0, 0, keep)))
				if !slices.Contains(out, line) {
					out = append(out, line)
					in.must.add(mustFact{Kind: factKeep, Day: m.Day, Meal: m.Meal, Dish: foodName(st, c.FoodID), Day2: dayLabel(cook), Days: days, Keep: keep, Last: dayLabel(cook.AddDate(0, 0, keep))})
				}
			}
		}
	}
	return out
}

// keepTooOld is the clause that follows "the batch would be too old by then":
// how long that batch's food keeps. Empty when the batch is unknown.
func keepTooOld(st planning.State, sessionID int64) string {
	if keep := keepDaysOf(st, sessionID); keep > 0 {
		return fmt.Sprintf("; it keeps %s in the fridge", countDays(keep, false))
	}
	return ""
}

// keepDaysOf is how long a batch's food keeps; 0 when the batch is unknown.
func keepDaysOf(st planning.State, sessionID int64) int {
	for _, s := range st.Sessions {
		if s.ID == sessionID {
			return keepFor(st, s.FoodID)
		}
	}
	return 0
}

var (
	oddWords   = regexp.MustCompile(`(?i)\b(assistant|assistent|ignore|ignoriere|disregard|instructions?|anweisung(en)?|system prompt)\b`)
	oddMark    = regexp.MustCompile(`(?i)\b(mark|markiere)\b.*\b(as|als)\b`)
	oddDashCmd = regexp.MustCompile(`(?i)\s[—–-]+\s+(please|bitte|mark|markiere|set|delete|remove|add|swap|replace|move|cancel|say|tell|reply|always|never|do not|don't|lösche|entferne|setze|ersetze|sage|antworte)\b`)
)

// oddName reports a recipe name with instruction-like text in it: a speaker
// word (assistant, ignore, instruction), "mark ... as", or a long dash
// followed by an imperative. Conservative: an ordinary name never matches.
func oddName(name string) bool {
	return oddWords.MatchString(name) || oddMark.MatchString(name) || oddDashCmd.MatchString(name)
}

// oddNameLines flags a dish named in a changed meal whose name reads like an
// instruction; the reply must say so and not act on it.
func oddNameLines(in reportInput, changed []reportMeal) []string {
	var out []string
	for _, m := range changed {
		for _, s := range m.After {
			name, _ := splitQty(s)
			if !oddName(name) {
				continue
			}
			line := fmt.Sprintf("The recipe name %s looks odd, like an instruction; it was not acted on. You may want to rename it in the recipe list.", food.Quote(name))
			if !slices.Contains(out, line) {
				out = append(out, line)
				in.must.add(mustFact{Kind: factOdd, Dish: name})
			}
		}
	}
	return out
}
