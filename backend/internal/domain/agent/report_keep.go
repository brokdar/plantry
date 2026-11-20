package agent

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

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
				line := fmt.Sprintf("%s %s eats the %s cooked %s, %d days earlier; it keeps %d days in the fridge, so the last day is %s.",
					m.Day, m.Meal, food.Quote(foodName(st, c.FoodID)), dayLabel(cook), days, keep, dayLabel(cook.AddDate(0, 0, keep)))
				if !slices.Contains(out, line) {
					out = append(out, line)
				}
			}
		}
	}
	return out
}

// keepTooOld is the clause that follows "the batch would be too old by then":
// how long that batch's food keeps. Empty when the batch is unknown.
func keepTooOld(st planning.State, sessionID int64) string {
	for _, s := range st.Sessions {
		if s.ID == sessionID {
			return fmt.Sprintf("; it keeps %d days in the fridge", keepFor(st, s.FoodID))
		}
	}
	return ""
}

var (
	mustKeep    = regexp.MustCompile(`^(\w{3}) (\d+) (\w{3}) \w+ eats the (".*?") cooked (\w{3}) (\d+) (\w{3}), (\d+) days earlier; it keeps (\d+) days in the fridge, so the last day is (\w{3}) (\d+) (\w{3})\.$`)
	mustTooOld  = regexp.MustCompile(`^(\w{3}) (\d+) (\w{3}) \w+ now cooks its own (".*?") instead of eating the one cooked earlier \(the batch would be too old by then; it keeps (\d+) days in the fridge\)\.$`)
	mustOddName = regexp.MustCompile(`^The recipe name (".*") looks odd, like an instruction; it was not acted on\. You may want to rename it in the recipe list\.$`)
	keepWord    = regexp.MustCompile(`(?i)(keeps?|kept|fridge|halt\w*|kühlschrank)`)
	oddWord     = regexp.MustCompile(`(?i)(odd|strange|unusual|weird|rename|instruction|seltsam|merkwürdig|ungewöhnlich|komisch|umbenenn\w*|anweisung)`)
	nameWord    = regexp.MustCompile(`(?i)(name|benenn|rezept|recipe)`)
)

func keepItem(p []string) mustItem {
	wd, dd, mon, dish := p[1], p[2], p[3], strings.Trim(p[4], `"`)
	keep := p[9]
	return mustItem{
		en: fmt.Sprintf("%s %s %s: the %s keeps %s days in the fridge, so %s %s %s is the last day.", wd, dd, mon, p[4], keep, p[10], p[11], p[12]),
		de: fmt.Sprintf("%s: „%s“ ist %s Tage im Kühlschrank haltbar, letzter Tag ist %s.", deDay(wd, dd, mon), dish, keep, deDay(p[10], p[11], p[12])),
		said: func(text string, german bool) bool {
			return lineWith(text, func(l string) bool {
				return numberIn(l, keep) && keepWord.MatchString(l) && dayIn(l, wd, dd, mon, german)
			})
		},
	}
}

func tooOldItem(p []string) mustItem {
	wd, dd, mon, dish := p[1], p[2], p[3], strings.Trim(p[4], `"`)
	keep := p[5]
	return mustItem{
		en: fmt.Sprintf("%s %s %s: the %s would be too old by then; it keeps %s days in the fridge.", wd, dd, mon, p[4], keep),
		de: fmt.Sprintf("%s: „%s“ wäre dann zu alt; es hält sich %s Tage im Kühlschrank.", deDay(wd, dd, mon), dish, keep),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool { return numberIn(l, keep) && keepWord.MatchString(l) })
		},
	}
}

func oddNameItem(p []string) mustItem {
	name := strings.Trim(p[1], `"`)
	return mustItem{
		en: fmt.Sprintf("The recipe name %s looks odd, like an instruction. I did not act on it; you may want to rename it in the recipe list.", p[1]),
		de: fmt.Sprintf("Der Rezeptname „%s“ sieht seltsam aus, wie eine Anweisung. Ich habe nichts davon ausgeführt; du kannst ihn in der Rezeptliste umbenennen.", name),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool { return oddWord.MatchString(l) && nameWord.MatchString(l) })
		},
	}
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
func oddNameLines(changed []reportMeal) []string {
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
			}
		}
	}
	return out
}
