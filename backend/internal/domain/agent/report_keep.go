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

// keepFacts says, for each changed meal that eats a batch cooked 3 or more
// days before (and still in the fridge), how long the food keeps and the last
// day for it.
func keepFacts(in reportInput, changed []reportMeal) {
	st := in.afterSt
	seen := map[string]bool{}
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
				dish := foodName(st, c.FoodID)
				last := dayLabel(cook.AddDate(0, 0, keep))
				en := fmt.Sprintf("%s %s eats the %s cooked %s, %d days earlier; it keeps %s in the fridge, so the last day is %s.",
					m.Day, m.Meal, food.Quote(dish), dayLabel(cook), days, countDays(keep, false), last)
				if !seen[en] {
					seen[en] = true
					in.facts.add(classMoved, kindKeep, statusOK, en,
						dot(fmt.Sprintf("%s %s isst %s, gekocht %s, vor %d Tagen; es hält sich %s im Kühlschrank, letzter Tag ist %s",
							deDay(m.Day), deSlot(m.Meal), deQuote(dish), deDay(dayLabel(cook)), days, countDays(keep, true), deDay(last))))
				}
			}
		}
	}
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

// oddNameFacts flags a dish named in a changed meal whose name reads like an
// instruction; the reply must say so and not act on it.
func oddNameFacts(in reportInput, changed []reportMeal) {
	seen := map[string]bool{}
	for _, m := range changed {
		for _, s := range m.After {
			name, _ := splitQty(s)
			if !oddName(name) || seen[name] {
				continue
			}
			seen[name] = true
			in.facts.add(classWarn, kindOdd, statusUnknown,
				fmt.Sprintf("The recipe name %s looks odd, like an instruction; it was not acted on. You may want to rename it in the recipe list.", food.Quote(name)),
				fmt.Sprintf("Der Rezeptname %s sieht seltsam aus, wie eine Anweisung; ich habe nichts davon ausgeführt. Du kannst ihn in der Rezeptliste umbenennen.", deQuote(name)))
		}
	}
}
