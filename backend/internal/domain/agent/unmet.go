package agent

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// changeAsk is a message that asks for a change (folded text): "instead",
// "swap", "can I have", "stattdessen", "kann ich ... haben".
var changeAsk = regexp.MustCompile(`\b(?:instead|stattdessen|statt|swap\w*|tausch\w*|replace\w*|ersetz\w*|put|setz\w*|change|aender\w*|add|hinzu\w*|remove|entfern\w*|move|verschieb\w*|(?:can|could|may) i (?:get|have)|(?:kann|koennte|darf) ich)\b`)

// infoAsk is a message that only asks what is there ("what is on Sunday
// dinner?", "why did you put pasta on Monday dinner?", "warum gibt es ...").
var infoAsk = regexp.MustCompile(`^\s*(?:what|which|how|when|why|who|where|is|are|does|do|did|was|welche\w*|wie|wann|warum|wieso|weshalb|wer|wo|ist|sind)\b`)

// requestAsk starts a message that asks for a change with a trailing "?"
// ("can I get ... instead?"): such a message is no question about the plan.
var requestAsk = regexp.MustCompile(`^\s*(?:please|bitte|(?:can|could|may|would|will) (?:i|you|we)|(?:kann|koennte|koenntest|darf|magst|wuerdest|moechte|moechten) (?:ich|du|man|wir)\b)`)

// asksOnly reports a message that asks about the plan and requests nothing:
// it starts with a question word, or it ends in "?" without starting like a
// request.
func asksOnly(msg string) bool {
	if infoAsk.MatchString(msg) {
		return true
	}
	return strings.HasSuffix(strings.TrimSpace(msg), "?") && !requestAsk.MatchString(msg)
}

// meatWords are words for meat or fish (folded), with the class they need.
var meatWords = map[string]diet.Class{
	"beef": diet.Omnivore, "rind": diet.Omnivore, "rinder": diet.Omnivore, "pork": diet.Omnivore, "schwein": diet.Omnivore,
	"chicken": diet.Omnivore, "haehnchen": diet.Omnivore, "huhn": diet.Omnivore, "lamb": diet.Omnivore, "lamm": diet.Omnivore,
	"turkey": diet.Omnivore, "pute": diet.Omnivore, "bacon": diet.Omnivore, "speck": diet.Omnivore, "ham": diet.Omnivore,
	"schinken": diet.Omnivore, "sausage": diet.Omnivore, "wurst": diet.Omnivore, "veal": diet.Omnivore, "kalb": diet.Omnivore,
	"fish": diet.Pescetarian, "fisch": diet.Pescetarian, "salmon": diet.Pescetarian, "lachs": diet.Pescetarian,
	"tuna": diet.Pescetarian, "thunfisch": diet.Pescetarian, "shrimp": diet.Pescetarian, "garnele": diet.Pescetarian,
}

var dietDE = map[diet.Class]string{diet.Vegan: "veganen", diet.Vegetarian: "vegetarischen", diet.Pescetarian: "pescetarischen"}

// unmetChange is the block a turn tells when the message asks to change a
// meal it names and the turn saves nothing without any write (S-B, ledger
// V77, safety s-diet-00/-01: "can I get the Beef goulash on Sunday dinner
// instead" for a vegetarian household was answered in prose alone, sometimes
// without saying that nothing changed or what was left out). The refused part
// is told in code: "Left out: the change to Sun 22 Nov dinner (...)", with the
// diet as the reason when the message names a food or a meat the household's
// diet excludes. nil when the message asks for no change of a named meal.
func unmetChange(text string, cells map[string]bool, slotNames map[int64]string, st planning.State) *replyBlock {
	msg := diet.Fold(text)
	if len(cells) == 0 || !changeAsk.MatchString(msg) || asksOnly(msg) {
		return nil
	}
	keys := make([]string, 0, len(cells))
	for k := range cells {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var en, de []string
	for _, k := range keys {
		date, slotID, _ := strings.Cut(k, "|")
		id, _ := strconv.ParseInt(slotID, 10, 64)
		meal := strings.TrimPrefix(slotNames[id], "slot.")
		day := dayLabelStr(date)
		en = append(en, day+" "+meal)
		de = append(de, deDay(day)+" "+deSlot(meal))
	}
	whyEN, whyDE := unmetWhyEN, unmetWhyDE
	if household := diet.Class(st.Profile.DietType); planning.DietRestricts(st.Profile.DietType) {
		if name := addedAgainstDiet(text, st, household); name != "" {
			whyEN = fmt.Sprintf("%q does not fit your %s diet", name, household)
			whyDE = fmt.Sprintf("„%s“ passt nicht zu deiner %s Ernährung", name, dietDE[household])
		}
	}
	return &replyBlock{nothingSaved: true, facts: []fact{{
		Class: classRefused, Kind: kindRefused, Status: statusOK,
		EN: fmt.Sprintf("Left out: the change to %s (%s).", strings.Join(en, ", "), whyEN),
		DE: fmt.Sprintf("Weggelassen: die Änderung an %s (%s).", strings.Join(de, ", "), whyDE),
	}}}
}

// The reason of the unmet line when no tool ran, and the reasons that replace
// it on the other no-save paths (spec G2/SF2, round 7 R2): a write the program
// refused, a change only checked, a turn that only looked things up.
const (
	unmetWhyEN   = "nothing was checked and saved for it"
	unmetWhyDE   = "dafür wurde nichts geprüft und gespeichert"
	failedWhyEN  = "the program refused it"
	failedWhyDE  = "das Programm hat sie abgelehnt"
	checkedWhyEN = "it was only checked, not saved"
	checkedWhyDE = "sie wurde nur geprüft, nicht gespeichert"
	lookedWhyEN  = "nothing was saved for it"
	lookedWhyDE  = "dafür wurde nichts gespeichert"
)

// removeCue and addCue are words (folded) that mark the food after them as
// the one taken out or the one put in.
var (
	removeCue = map[string]bool{
		"replace": true, "replaced": true, "replacing": true, "remove": true, "removing": true, "delete": true, "drop": true, "swap": true, "swapping": true,
		"ersetze": true, "ersetzen": true, "ersetz": true, "entferne": true, "entfernen": true, "loesche": true, "tausche": true, "tauschen": true, "statt": true, "anstatt": true, "anstelle": true,
	}
	addCue = map[string]bool{
		"with": true, "mit": true, "for": true, "fuer": true, "gegen": true, "durch": true, "by": true, "add": true, "put": true, "stattdessen": true, "instead": true,
		"hinzu": true, "setze": true, "setzen": true, "insert": true, "have": true, "get": true, "haben": true,
	}
)

// foodSpan is a food the message names: the user's words and where they start.
type foodSpan struct {
	shown string
	pos   int
	class diet.Class
}

// addedAgainstDiet is the food the message puts in that the household's diet
// excludes, in the user's own words, "" when none. A food that is taken out or
// replaced (after replace, remove, swap, "instead of", statt) is never blamed:
// the nearest cue before a food within four words decides, and with no cue the
// food is the one added. Candidates are sorted by position.
func addedAgainstDiet(text string, st planning.State, household diet.Class) string {
	orig := strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	fold := make([]string, len(orig))
	for i, w := range orig {
		fold[i] = diet.Fold(w)
	}
	var spans []foodSpan
	taken := make([]bool, len(orig))
	for id := range namedFoods(text, st.Foods) {
		f := st.Foods[id]
		if !diet.Valid(f.DietClass) || diet.Compatible(f.DietClass, household) {
			continue
		}
		var name []string
		for _, w := range strings.FieldsFunc(f.Name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			name = append(name, diet.Fold(w))
		}
		for i := 0; i+len(name) <= len(fold) && len(name) > 0; i++ {
			if slices.Equal(fold[i:i+len(name)], name) {
				spans = append(spans, foodSpan{shown: strings.Join(orig[i:i+len(name)], " "), pos: i, class: diet.Class(f.DietClass)})
				for j := i; j < i+len(name); j++ {
					taken[j] = true
				}
				break
			}
		}
	}
	for i, w := range fold {
		if c, ok := meatWords[w]; ok && !taken[i] && !diet.Compatible(c, household) {
			spans = append(spans, foodSpan{shown: orig[i], pos: i, class: c})
		}
	}
	sort.Slice(spans, func(a, b int) bool { return spans[a].pos < spans[b].pos })
	for _, sp := range spans {
		if !removedAt(fold, sp.pos) {
			return sp.shown
		}
	}
	return ""
}

// removedAt reports whether the food at word i is the one taken out: the
// nearest cue within the four words before it is a remove cue.
func removedAt(fold []string, i int) bool {
	for j := i - 1; j >= 0 && j >= i-4; j-- {
		switch {
		case removeCue[fold[j]]:
			return true
		case fold[j] == "instead" && j+1 < len(fold) && fold[j+1] == "of":
			return true
		case addCue[fold[j]]:
			return false
		}
	}
	return false
}

// underFloorBlock is the block a turn tells when the message sets a day to a
// kcal number under that day's minimum (C-11, NU9) and the turn saves
// nothing: "Sat 21 Nov stays as it is: 250 kcal is under the day's minimum of
// 793 kcal." nil when no named day number is under its minimum.
func underFloorBlock(dayKcal map[string]float64, st planning.State) *replyBlock {
	if len(dayKcal) == 0 {
		return nil
	}
	for _, d := range planning.Evaluate(st, planning.WindowTarget(st)).Days {
		want, ok := dayKcal[d.Date]
		if !ok || d.FloorKcal <= 0 || want >= d.FloorKcal-0.5 {
			continue
		}
		label := dayLabelStr(d.Date)
		return &replyBlock{nothingSaved: true, facts: []fact{{
			Class: classRefused, Kind: kindRefused, Status: statusOK,
			EN: fmt.Sprintf("%s stays as it is: %.0f kcal is under the day's minimum of %.0f kcal.", label, math.Round(want), math.Round(d.FloorKcal)),
			DE: fmt.Sprintf("%s bleibt, wie er ist: %.0f kcal liegen unter dem Mindestwert des Tages von %.0f kcal.", deDay(label), math.Round(want), math.Round(d.FloorKcal)),
		}}}
	}
	return nil
}
