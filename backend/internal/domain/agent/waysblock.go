package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// waysBlock is the reply block of a find_ways call that found no way to apply
// on its own: nothing changes, the reason with its numbers is a code-rendered
// refused fact (G4.1, SF2; a day number under the minimum names the minimum and
// the asked number, C-11), and the search's one offer is the offer (G3). Nil
// for a result with a way or one that does not parse. It reaches the reply only
// when no write of the turn saved anything (see replyTurn.final).
func waysBlock(output json.RawMessage) *replyBlock {
	var obj map[string]any
	if json.Unmarshal(output, &obj) != nil || obj == nil {
		return nil
	}
	if ways, _ := obj["ways"].([]any); len(ways) > 0 {
		return nil
	}
	why, _ := obj["empty_reason"].(string)
	if why == "" {
		return nil
	}
	goal, _ := obj["goal"].(map[string]any)
	kind, _ := goal["kind"].(string)
	date, _ := goal["date"].(string)
	before, _ := obj["before"].(map[string]any)
	meal, day := math.Round(num(before["meal_kcal"])), math.Round(num(before["day_kcal"]))
	floor, top := math.Round(num(obj["minimum_kcal"])), math.Round(num(obj["range_high"]))
	failed := strings.HasPrefix(why, "every way failed the check")
	var en, de string
	switch {
	case kind == planning.GoalLowerDay && num(goal["target_kcal"]) > 0 && num(goal["target_kcal"]) < floor:
		asked := math.Round(num(goal["target_kcal"]))
		label := dayLabelStr(date)
		en = fmt.Sprintf("%s stays as it is: %.0f kcal is under the day's minimum of %.0f kcal.", label, asked, floor)
		de = fmt.Sprintf("%s bleibt, wie er ist: %.0f kcal liegen unter dem Mindestwert des Tages von %.0f kcal.", deDay(label), asked, floor)
	case failed:
		en = "No change of this meal on its own passed the check: each one would add a problem to the plan."
		de = "Keine Änderung dieser Mahlzeit allein hat die Prüfung bestanden: jede würde dem Plan ein Problem hinzufügen."
	case kind == planning.GoalLighter && floor > 0:
		en = fmt.Sprintf("This meal cannot get lighter on its own: it holds %.0f kcal and the day holds %.0f kcal, only %.0f kcal above its minimum of %.0f kcal.", meal, day, math.Max(0, day-floor), floor)
		de = fmt.Sprintf("Diese Mahlzeit kann allein nicht leichter werden: sie hat %.0f kcal und der Tag %.0f kcal, nur %.0f kcal über seinem Mindestwert von %.0f kcal.", meal, day, math.Max(0, day-floor), floor)
	case kind == planning.GoalHeartier && top > 0:
		en = fmt.Sprintf("This meal cannot get heartier on its own: the day holds %.0f kcal and a good amount for it ends at %.0f kcal.", day, top)
		de = fmt.Sprintf("Diese Mahlzeit kann allein nicht reichhaltiger werden: der Tag hat %.0f kcal und ein passender Bereich endet bei %.0f kcal.", day, top)
	case kind == planning.GoalMoreProtein && top > 0 && day >= top:
		en = fmt.Sprintf("No protein food fits on its own: the day holds %.0f kcal, already over a good amount for it (up to %.0f kcal).", day, top)
		de = fmt.Sprintf("Kein eiweißreiches Essen passt allein dazu: der Tag hat %.0f kcal, schon über dem passenden Bereich (bis %.0f kcal).", day, top)
	case kind == planning.GoalMoreProtein && proteinWhyDE(why) != "":
		// The search's own reason, naming the checks that dropped every
		// candidate (l1r7-tune g2-protein-06: the reply had none).
		en, de = why, proteinWhyDE(why)
	default:
		en = "No checked change does this on its own."
		de = "Keine geprüfte Änderung schafft das allein."
	}
	b := &replyBlock{facts: []fact{{Class: classRefused, Kind: kindRefused, Status: statusOK, EN: en, DE: de}}, nothingSaved: true}
	if list, _ := obj["offer"].([]any); len(list) > 0 {
		if o, _ := list[0].(map[string]any); o != nil {
			if what, _ := o["what"].(string); what != "" {
				enO, deO := what, deWayWhat(what)
				if after, _ := o["after"].(map[string]any); after != nil && num(after["day_kcal"]) > 0 && !strings.Contains(what, "kcal") {
					enO += fmt.Sprintf(" (the day would then hold %.0f kcal)", math.Round(num(after["day_kcal"])))
					deO += fmt.Sprintf(" (der Tag hätte dann %.0f kcal)", math.Round(num(after["day_kcal"])))
				}
				b.offer = makeOffer(enO, deO)
			}
		}
	}
	return b
}

// The more-protein reasons of planning's noWayWhy and their clauses.
var (
	proteinNoWay  = regexp.MustCompile(`^No change adds (\d+) g of protein or more without (.+)\.$`)
	proteinNone   = regexp.MustCompile(`^No checked change adds (\d+) g of protein or more\.$`)
	proteinOver   = regexp.MustCompile(`^taking the day \((\d+) kcal\) over its range top of (\d+) kcal$`)
	proteinCook   = regexp.MustCompile(`^putting more than (\d+) dishes on one cooking time$`)
	proteinWaste  = regexp.MustCompile(`^adding over (\d+) kcal for under (\d+) g of protein$`)
	proteinClause = map[string]string{
		"breaking a hard rule":                              "eine feste Regel zu brechen",
		"taking a day under its energy minimum":             "einen Tag unter seinen Mindestwert zu bringen",
		"adding meat to a week already over its meat limit": "einer Woche über ihrer Fleischgrenze mehr Fleisch hinzuzufügen",
		"leaving a meal without full nutrition data":        "eine Mahlzeit ohne vollständige Nährwerte zu lassen",
	}
)

// proteinWhyDE says a more-protein search's reason in German; "" for any
// other text (a reason with the real check's drops appended stays generic).
func proteinWhyDE(why string) string {
	if m := proteinNone.FindStringSubmatch(why); m != nil {
		return fmt.Sprintf("Keine geprüfte Änderung bringt %s g Eiweiß oder mehr.", m[1])
	}
	m := proteinNoWay.FindStringSubmatch(why)
	if m == nil {
		return ""
	}
	var de []string
	for _, c := range strings.Split(m[2], " or ") {
		switch o, k, w := proteinOver.FindStringSubmatch(c), proteinCook.FindStringSubmatch(c), proteinWaste.FindStringSubmatch(c); {
		case o != nil:
			de = append(de, fmt.Sprintf("den Tag (%s kcal) über die Obergrenze seines Bereichs von %s kcal zu bringen", o[1], o[2]))
		case k != nil:
			de = append(de, fmt.Sprintf("mehr als %s Gerichte auf eine Kochzeit zu legen", k[1]))
		case w != nil:
			de = append(de, fmt.Sprintf("über %s kcal für unter %s g Eiweiß hinzuzufügen", w[1], w[2]))
		case proteinClause[c] != "":
			de = append(de, proteinClause[c])
		default:
			return ""
		}
	}
	return fmt.Sprintf("Keine Änderung bringt %s g Eiweiß oder mehr, ohne %s.", m[1], strings.Join(de, " oder "))
}

var (
	wayDayDown  = regexp.MustCompile(`^take (\w{3} \d{1,2} \w{3}) down to (\d+) kcal, the lowest it goes inside its range: (.+)$`)
	wayCell     = regexp.MustCompile(`^(\w{3} \d{1,2} \w{3} \w+): (.+)$`)
	wayPortions = regexp.MustCompile(`^"([^"]+)": ([\d.]+) portions \(was ([\d.]+)\)$`)
	wayGrams    = regexp.MustCompile(`^"([^"]+)": (\d+) g \(was (\d+) g\)$`)
	wayLeave    = regexp.MustCompile(`^leave out (.+)$`)
)

// deWayWhat says a find_ways way's "what" in German: the day or meal it is
// about and its portion, gram and leave-out steps; any other shape is told as a
// checked change.
func deWayWhat(en string) string {
	prefix := ""
	rest := en
	if m := wayDayDown.FindStringSubmatch(en); m != nil {
		prefix, rest = fmt.Sprintf("%s auf %s kcal senken, so tief es innerhalb seines Bereichs geht: ", deDay(m[1]), m[2]), m[3]
	} else if m := wayCell.FindStringSubmatch(en); m != nil {
		prefix, rest = "bei "+deCell(m[1])+": ", m[2]
	}
	var parts []string
	for _, p := range strings.Split(rest, ", ") {
		if m := wayPortions.FindStringSubmatch(p); m != nil {
			parts = append(parts, fmt.Sprintf("%s: %s Portionen (vorher %s)", deQuote(m[1]), strings.Replace(m[2], ".", ",", 1), strings.Replace(m[3], ".", ",", 1)))
		} else if m := wayGrams.FindStringSubmatch(p); m != nil {
			parts = append(parts, fmt.Sprintf("%s: %s g (vorher %s g)", deQuote(m[1]), m[2], m[3]))
		} else if m := wayLeave.FindStringSubmatch(p); m != nil {
			parts = append(parts, deQuoted(m[1])+" weglassen")
		} else {
			return "eine geprüfte Änderung anwenden"
		}
	}
	return prefix + strings.Join(parts, ", ")
}
