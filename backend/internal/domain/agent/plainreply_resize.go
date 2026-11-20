package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// mustTold returns the items of a result's report.tell the reply must carry
// (see mustItems; e.g. "Sun 22 Nov: cook 3.5 portions of the ... (was 3)."),
// in order, and whether the result is a saved change's report (a dry run's is
// renamed away).
func mustTold(view json.RawMessage) (items []string, saved bool) {
	var v struct {
		Report *struct {
			Tell []string `json:"tell"`
		} `json:"report"`
	}
	if json.Unmarshal(view, &v) != nil || v.Report == nil {
		return nil, false
	}
	var out []string
	for _, t := range v.Report.Tell {
		if len(mustItems(t)) > 0 {
			out = append(out, t)
		}
	}
	return out, true
}

var (
	englishWord = regexp.MustCompile(`(?i)\b(the|and|is|are|now|with)\b`)
	germanWord  = regexp.MustCompile(`(?i)\b(der|die|das|und|ist|wird|werden|mit|nicht|für|jetzt|auf|dem|den)\b`)
	resizeParts = regexp.MustCompile(`^(\w{3}) (\d+) (\w{3}): cook ([\d.]+) portions? of the (".*") \(was ([\d.]+)\)\.$`)
)

var deWeekday = map[string][2]string{
	"Mon": {"Montag", "Mo."}, "Tue": {"Dienstag", "Di."}, "Wed": {"Mittwoch", "Mi."},
	"Thu": {"Donnerstag", "Do."}, "Fri": {"Freitag", "Fr."}, "Sat": {"Samstag", "Sa."},
	"Sun": {"Sonntag", "So."},
}

// deMonth holds the month number and German abbreviation per English one.
var deMonth = map[string]struct {
	num int
	abb string
}{
	"Jan": {1, "Jan."}, "Feb": {2, "Feb."}, "Mar": {3, "Mär."}, "Apr": {4, "Apr."},
	"May": {5, "Mai"}, "Jun": {6, "Juni"}, "Jul": {7, "Juli"}, "Aug": {8, "Aug."},
	"Sep": {9, "Sept."}, "Oct": {10, "Okt."}, "Nov": {11, "Nov."}, "Dec": {12, "Dez."},
}

// restoreMust keeps the facts a household must hear from dropping out of the
// final reply: a batch cooked on another day (day and portions), a day outside
// its range, a week figure that got worse, a cooking that is gone or moved.
// A fact the reply does not carry (its day and number on one line) is added
// as a line in the reply's language, before the closing offer. English and
// German replies only; any other language is left as the model wrote it.
func restoreMust(m *llm.Message, tell []string) {
	var items []mustItem
	for _, t := range tell {
		items = append(items, mustItems(t)...)
	}
	if len(items) == 0 {
		return
	}
	for i := range m.Content {
		b := &m.Content[i]
		if b.Type != llm.ContentTypeText {
			continue
		}
		german := false
		switch {
		case len(englishWord.FindAllString(b.Text, -1)) >= 3:
		case len(germanWord.FindAllString(b.Text, -1)) >= 3:
			german = true
		default:
			continue
		}
		var missing []string
		for _, it := range items {
			if !it.said(b.Text, german) {
				line := it.en
				if german {
					line = it.de
				}
				missing = append(missing, "- "+line)
			}
		}
		if len(missing) == 0 {
			return
		}
		add := strings.Join(missing, "\n")
		paras := strings.Split(b.Text, "\n\n")
		if n := len(paras); n > 1 && strings.Contains(paras[n-1], "if you want it") {
			paras = append(paras[:n-1], add, paras[n-1])
			b.Text = strings.Join(paras, "\n\n")
		} else {
			b.Text = strings.TrimRight(b.Text, "\n") + "\n\n" + add
		}
		return
	}
}

// germanResize renders a resize tell item as a German line; the dish name
// stays as stored.
func germanResize(item string) string {
	p := resizeParts.FindStringSubmatch(item)
	if p == nil {
		return item
	}
	wd, ok1 := deWeekday[p[1]]
	mo, ok2 := deMonth[p[3]]
	if !ok1 || !ok2 {
		return item
	}
	unit := "Portionen"
	if p[4] == "1" {
		unit = "Portion"
	}
	return fmt.Sprintf("%s, %s. %s: %s %s „%s“ kochen (statt %s).", wd[1], p[2], mo.abb,
		strings.Replace(p[4], ".", ",", 1), unit, strings.Trim(p[5], `"`), strings.Replace(p[6], ".", ",", 1))
}

// numberIn matches num as a whole number in text, either decimal mark.
func numberIn(text, num string) bool {
	re := regexp.MustCompile(`(^|[^\d.,])` + strings.Replace(regexp.QuoteMeta(num), `\.`, `[.,]`, 1) + `($|[^\d]|[.,]\D|[.,]$)`)
	return re.MatchString(text)
}

// resizeSaid reports a line of text that holds the item's day and its new
// portion count.
func resizeSaid(text, item string, german bool) bool {
	if german {
		return resizeSaidDE(text, item)
	}
	day, rest, _ := strings.Cut(item, ": cook ")
	num, _, _ := strings.Cut(rest, " ")
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(num) + `\b`)
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, day) && re.MatchString(line) {
			return true
		}
	}
	return false
}

// resizeSaidDE is resizeSaid for a German reply: the day is a weekday name
// (compounds like "Sonntags-Charge" count) or a "22. Nov" / "22.11." date, the
// portions a number with comma or point.
func resizeSaidDE(text, item string) bool {
	p := resizeParts.FindStringSubmatch(item)
	if p == nil {
		return true // not a shape we can render or detect; leave the reply alone
	}
	wd := strings.ToLower(deWeekday[p[1]][0])
	mo := deMonth[p[3]]
	date := regexp.MustCompile(`(^|\D)` + p[2] + `\.\s?(` + regexp.QuoteMeta(strings.TrimSuffix(mo.abb, ".")) + `|0?` + fmt.Sprint(mo.num) + `\.)`)
	for _, line := range strings.Split(text, "\n") {
		if (strings.Contains(strings.ToLower(line), wd) || date.MatchString(line)) && numberIn(line, p[4]) {
			return true
		}
	}
	return false
}
