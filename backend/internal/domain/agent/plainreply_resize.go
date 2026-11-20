package agent

import (
	"regexp"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

var (
	englishWord = regexp.MustCompile(`(?i)\b(the|and|is|are|now|with)\b`)
	germanWord  = regexp.MustCompile(`(?i)\b(der|die|das|und|ist|wird|werden|mit|nicht|für|jetzt|auf|dem|den)\b`)
	// closingWord marks a closing offer or question, English and German.
	closingWord = regexp.MustCompile(`(?i)(if you want|say so|let me know|do you want|would you like|shall i|should i|i can also|soll ich|möchtest du|willst du|sag bescheid|sag einfach|wenn du willst|wenn du möchtest|ich kann auch|kann ich)`)
	listLine    = regexp.MustCompile(`^\s*([-*•]|\d+[.)])\s`)
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
// A fact the reply does not carry is added as a line in the reply's language,
// before the closing question or offer. English and German replies only; any
// other language is left as the model wrote it.
func restoreMust(m *llm.Message, facts []mustFact) {
	var items []mustItem
	for _, f := range facts {
		if it, ok := f.item(); ok {
			items = append(items, it)
		}
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
				if l := "- " + line; !slices.Contains(missing, l) {
					missing = append(missing, l) // two facts can say one line
				}
			}
		}
		if len(missing) > 0 {
			b.Text = insertBeforeClosing(b.Text, strings.Join(missing, "\n"))
		}
		return
	}
}

// insertBeforeClosing puts block before the reply's closing paragraphs: the
// trailing plain paragraphs that ask a question or make an offer.
func insertBeforeClosing(text, block string) string {
	paras := strings.Split(strings.TrimRight(text, "\n"), "\n\n")
	at := len(paras)
	for at > 1 && closing(paras[at-1]) {
		at--
	}
	out := append(append(append([]string{}, paras[:at]...), block), paras[at:]...)
	return strings.Join(out, "\n\n")
}

// closing reports a paragraph that is no list and asks or offers.
func closing(p string) bool {
	for _, l := range strings.Split(p, "\n") {
		if listLine.MatchString(l) {
			return false
		}
	}
	return strings.HasSuffix(strings.TrimSpace(p), "?") || closingWord.MatchString(p)
}
