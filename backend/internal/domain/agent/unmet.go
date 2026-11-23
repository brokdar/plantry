package agent

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// changeAsk is a message that asks for a change (folded text): "instead",
// "swap", "can I have", "stattdessen", "kann ich ... haben".
var changeAsk = regexp.MustCompile(`\b(?:instead|stattdessen|statt|swap\w*|tausch\w*|replace\w*|ersetz\w*|put|setz\w*|change|aender\w*|add|hinzu\w*|remove|entfern\w*|move|verschieb\w*|(?:can|could|may) i (?:get|have)|(?:kann|koennte|darf) ich)\b`)

// infoAsk is a message that only asks what is there ("what is on Sunday dinner?").
var infoAsk = regexp.MustCompile(`^\s*(?:what|which|how|when|was|welche\w*|wie|wann)\b`)

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
	if len(cells) == 0 || !changeAsk.MatchString(msg) || infoAsk.MatchString(msg) {
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
	whyEN, whyDE := "nothing was checked and saved for it", "dafür wurde nichts geprüft und gespeichert"
	if household := diet.Class(st.Profile.DietType); planning.DietRestricts(st.Profile.DietType) {
		name := ""
		for id := range namedFoods(text, st.Foods) {
			if f := st.Foods[id]; diet.Valid(f.DietClass) && !diet.Compatible(f.DietClass, household) {
				name = f.Name
			}
		}
		if name == "" {
			for _, w := range strings.FieldsFunc(msg, func(r rune) bool { return r < 'a' || r > 'z' }) {
				if c, ok := meatWords[w]; ok && !diet.Compatible(c, household) {
					name = w
					break
				}
			}
		}
		if name != "" {
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
