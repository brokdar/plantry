package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plainwords"
)

// The reply a household reads is put together by code: the facts block and the
// one offer are rendered from the typed outcome of the write, and the model's own
// text is at most two sentences of framing. A model text that breaks the rules
// below is dropped and the block goes out alone (spec G4, RP2-RP6).

// plainReply is the last line of defence for the words a household reads (see
// plainwords.Reply, shared with the planner's saved note).
func plainReply(s string) string { return plainwords.Reply(s) }

// replyBlock is what code owns in the reply of a turn: the refused part (class
// 1), the facts of the newest saved change and its one offer.
type replyBlock struct {
	facts []fact // class 1 first, then the block
	offer *offerLine
	// nothingSaved: the call (or, merged, every call) of the turn saved nothing.
	nothingSaved bool
}

func (b *replyBlock) empty() bool { return b == nil || (len(b.facts) == 0 && b.offer == nil) }

// blockOf reads the reply block of a tool result: the report's facts and offer
// of a saved change, and what the guard left out. Nil for a result that is no
// write or only a check.
func blockOf(output json.RawMessage) (b *replyBlock, check bool) {
	var obj map[string]any
	if json.Unmarshal(output, &obj) != nil || obj == nil {
		return nil, false
	}
	if obj["dry_run"] == true {
		return nil, true
	}
	b = &replyBlock{}
	if skipped, _ := obj["skipped"].([]any); len(skipped) > 0 {
		for _, e := range skipped {
			m, _ := e.(map[string]any)
			en, _ := m["en"].(string)
			de, _ := m["de"].(string)
			if en != "" && de != "" && !slices.ContainsFunc(b.facts, func(f fact) bool { return f.EN == en && f.Class == classRefused }) {
				b.facts = append(b.facts, fact{Class: classRefused, Kind: kindRefused, Status: statusOK, EN: en, DE: de})
			}
		}
	}
	// Nothing of the call was saved: composeReply says so first, in code (G4.1, SF2),
	// so the reply is honest even when the lint drops the model's text.
	b.nothingSaved = obj["saved"] == false && len(b.facts) > 0
	rep, _ := obj["report"].(map[string]any)
	if rep != nil {
		raw, _ := json.Marshal(rep["facts"])
		var fs []fact
		if err := json.Unmarshal(raw, &fs); err != nil {
			slog.Warn("agent.reply.facts unreadable", "error", err)
		}
		b.facts = append(b.facts, fs...)
		if o, ok := rep["offer"].(map[string]any); ok {
			en, _ := o["en"].(string)
			de, _ := o["de"].(string)
			if en != "" {
				b.offer = &offerLine{EN: en, DE: de}
			}
		}
	}
	if stop, ok := undoStop(obj); ok && rep != nil {
		b.facts = append(b.facts, stop) // undone some, then stopped
	}
	if b.offer == nil && (len(b.facts) > 0 || rep != nil) {
		b.offer = offerOf(obj)
	}
	if b.empty() {
		return nil, false
	}
	return b, false
}

// The first line of a reply whose calls all saved nothing.
const (
	nothingChangedEN = "Nothing changed: the plan is as it was."
	nothingChangedDE = "Nichts geändert: der Plan bleibt, wie er war."
)

// withoutFacts removes the facts and the offer from a result's reports: the
// model reads the tell to frame its sentence, the code puts the block into the
// reply.
func withoutFacts(view json.RawMessage) json.RawMessage {
	if !strings.Contains(string(view), `"facts"`) && !strings.Contains(string(view), `"offer"`) && !strings.Contains(string(view), `"check"`) {
		return view
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(view, &obj) != nil {
		return view
	}
	changed := false
	for _, key := range []string{"report", "report_if_applied"} {
		var rep map[string]json.RawMessage
		if json.Unmarshal(obj[key], &rep) != nil || rep == nil {
			continue
		}
		for _, k := range []string{"facts", "offer", "check"} {
			if _, ok := rep[k]; ok {
				delete(rep, k)
				changed = true
			}
		}
		b, _ := json.Marshal(rep)
		obj[key] = b
	}
	if !changed {
		return view
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return view
	}
	return b
}

// replyTurn is what the reply of a turn is judged against.
type replyTurn struct {
	user    string           // the user's message, for the language and its numbers
	earlier []string         // the earlier user messages, newest first: the language when this one has no marker word
	locale  string           // the profile's locale, the last word on the language
	numbers map[string]bool  // numbers the turn's tool results hold (see toolNumbers)
	state   map[string]bool  // numbers of the prompt's plan state: back a read-only answer, never a write's text (v1.4 P-7)
	dates   map[dateKey]bool // the days the code wrote (plan state, tool results) or the user named: a date naming one is no figure
	year    int              // the year of the turn's ISO dates, 0 when none: a weekday before a date is checked against it
	foods   []string         // the names of the foods of the plan: a write's text names none the block does not
	write   bool             // a plan-writing tool was called (also as a check)
	block   *replyBlock      // the facts and the offer of the turn's writes
	// drops, when set, collects every sentence of the model's final text the
	// reply leaves out, with the reason (F6: agent.reply.lint).
	drops *[]lintDrop
	// ways is the block of the turn's last find_ways when it found nothing to
	// apply on its own (waysBlock): told only when no write of the turn has a block.
	ways *replyBlock
}

// final is the turn as its closing reply is composed: a turn that saved nothing
// after a find_ways that found nothing says so in code, with the reason and its
// numbers and the search's one offer (G4.1, SF2, C-11).
func (t replyTurn) final() replyTurn {
	if t.ways != nil && t.block == nil {
		t.write, t.block = true, t.ways
	}
	return t
}

// lang is the language of the reply: the user's message, else the earlier
// ones, else the profile's locale, else the model's own text, else English.
func (t replyTurn) lang(model string) lang {
	texts := append([]string{t.user}, t.earlier...)
	if l, ok := replyLangOf(texts...); ok {
		return l
	}
	switch strings.ToLower(strings.TrimSpace(t.locale)) {
	case "de":
		return langDE
	case "en":
		return langEN
	}
	l, _ := replyLangOf(model)
	return l
}

// mergeBlock adds the block of a later write to the earlier ones: every write's
// facts reach the reply (the block is cut again to its cap), the newest offer stays.
func mergeBlock(a, b *replyBlock) *replyBlock {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}
	// A later write's cooking of a day is the turn's last word on it: the
	// earlier write's cooking lines of that day are no longer true (V72).
	later := map[string]bool{}
	for _, f := range b.facts {
		if d := cookDay(f); d != "" {
			later[d] = true
		}
	}
	earlier := slices.DeleteFunc(slices.Clone(a.facts), func(f fact) bool { return later[cookDay(f)] })
	out := &replyBlock{facts: selectBlock(append(earlier, b.facts...)), offer: a.offer, nothingSaved: a.nothingSaved && b.nothingSaved}
	if b.offer != nil {
		out.offer = b.offer
	}
	return out
}

// cookDay is the day a cooking line tells ("Thu 26 Nov at lunch you cook ...",
// "Thu 26 Nov: nothing is cooked."), "" for any other line.
func cookDay(f fact) string {
	if f.Class != classMoved || f.Kind != kindCook {
		return ""
	}
	colon := strings.Index(f.EN, ":")
	if i := strings.Index(f.EN, " at "); i > 0 && (colon < 0 || i < colon) {
		return f.EN[:i]
	}
	if colon > 0 {
		return f.EN[:colon]
	}
	return ""
}

var writeTools = map[string]bool{"apply_changes": true, "undo_last_change": true, "regenerate": true, "apply_preset": true}

const (
	fallbackEN = "I could not put that into words I can check, so I left my own remarks out. Ask again and name the day or meal."
	fallbackDE = "Das konnte ich nicht in Worten fassen, die ich prüfen kann, deshalb habe ich meine Anmerkungen weggelassen. Frag bitte noch einmal und nenne den Tag oder die Mahlzeit."
)

var (
	numberRe    = regexp.MustCompile(`\d[\d.,]*\d|\d`)
	isoAnywhere = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)
	thousand    = regexp.MustCompile(`^(\d{1,3})([.,]\d{3})+$`)
	banned      = regexp.MustCompile(`(?i)\b(batch(es)?|eaters?|light days?|hearty minimum|cap|caps|band|bands|floor|soft|drafts?|extras?)\b|\b[HS]\d{1,2}\b|\b\d{4}-\d{2}-\d{2}\b|the tool only allows`)
	quotedRe    = regexp.MustCompile(`"[^"\n]*"|„[^“\n]*“|“[^”\n]*”`)
	weekdayRe   = regexp.MustCompile(`(?i)\b(mon|tues?|wed(nes)?|thu(rs)?|fri|sat(ur)?|sun)(day)?\b|\b(montag|dienstag|mittwoch|donnerstag|freitag|samstag|sonntag)\b`)
)

// normNumber is a number in one form: no thousands separator, a dot for the decimal mark.
func normNumber(s string) string {
	s = strings.TrimRight(s, ".,")
	if thousand.MatchString(s) {
		s = strings.NewReplacer(".", "", ",", "").Replace(s)
	}
	s = strings.Replace(s, ",", ".", 1)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

// deDecimalComma is a German number whose comma has three digits after it
// ("1,250 kg"): there the comma is the decimal mark, not a thousands separator.
var deDecimalComma = regexp.MustCompile(`^\d+,\d{3}$`)

// normModelNumber is normNumber for a number of the model's text in language lg.
func normModelNumber(s string, lg lang) string {
	if lg == langDE && deDecimalComma.MatchString(s) {
		return strings.TrimRight(strings.TrimRight(strings.Replace(s, ",", ".", 1), "0"), ".")
	}
	return normNumber(s)
}

// dateKey is a calendar day without its year, as a date in a sentence names it.
type dateKey struct {
	month time.Month
	day   int
}

// dateInText is a date written in a sentence ("Wed 25 Nov", "25. Dezember"):
// its day number is no figure a tool result tells.
var dateInText = regexp.MustCompile(`(?i)\b\d{1,2}\.?\s*(jan(uar|uary)?|feb(ruar|ruary)?|m[äa]r(z|ch)?|apr(il)?|mai|may|jun[ie]?|jul[iy]?|aug(ust)?|sept?(ember)?|o[ck]t(ober)?|nov(ember)?|de[cz](ember)?)\b`)

// The dates a sentence may write, with their day and month captured: ISO
// ("2026-11-22"), day before month ("Sun 22 Nov", "22. November", "22. Nov.",
// a year after it), English month before day ("November 22") and the German
// numeric date ("30.11.", "30.11.2026").
var (
	isoDate    = regexp.MustCompile(`\b\d{4}-(\d{2})-(\d{2})\b`)
	dayMonth   = regexp.MustCompile(`(?i)\b(\d{1,2})\.?\s*(jan(?:uar|uary)?|feb(?:ruar|ruary)?|m[äa]r(?:z|ch)?|apr(?:il)?|mai|may|jun[ie]?|jul[iy]?|aug(?:ust)?|sept?(?:ember)?|o[ck]t(?:ober)?|nov(?:ember)?|de[cz](?:ember)?)\b\.?`)
	monthDay   = regexp.MustCompile(`\b(January|February|March|April|May|June|July|August|September|October|November|December|Jan|Feb|Mar|Apr|Jun|Jul|Aug|Sept?|Oct|Nov|Dec)\.?\s+(\d{1,2})(?:st|nd|rd|th)?\b`)
	numericDay = regexp.MustCompile(`\b(\d{1,2})\.(\d{1,2})\.(?:\d{4}\b)?`)
)

// monthPrefix maps the first three letters of a month word, English or
// German, to its month.
var monthPrefix = map[string]time.Month{
	"jan": time.January, "feb": time.February, "mar": time.March, "mär": time.March, "apr": time.April,
	"may": time.May, "mai": time.May, "jun": time.June, "jul": time.July, "aug": time.August, "sep": time.September,
	"oct": time.October, "okt": time.October, "nov": time.November, "dec": time.December, "dez": time.December,
}

// monthOf is the month a month word names, in either language; 0 for none.
func monthOf(word string) time.Month {
	if w := []rune(strings.ToLower(word)); len(w) >= 3 {
		return monthPrefix[string(w[:3])]
	}
	return 0
}

// monthNumber is the month of a number text, 0 for none.
func monthNumber(s string) time.Month {
	n, _ := strconv.Atoi(s)
	return time.Month(n)
}

// dateOf is the day a date match names (day and month as captured), ok false
// for no calendar day.
func dateOf(day string, month time.Month) (dateKey, bool) {
	d, err := strconv.Atoi(day)
	if err != nil || month < time.January || month > time.December || d < 1 || d > 31 {
		return dateKey{}, false
	}
	return dateKey{month: month, day: d}, true
}

// dateMatchers are the date forms with the day and month of a match.
var dateMatchers = []struct {
	re  *regexp.Regexp
	key func(m []string) (dateKey, bool)
}{
	{isoDate, func(m []string) (dateKey, bool) { mo, _ := strconv.Atoi(m[1]); return dateOf(m[2], time.Month(mo)) }},
	{dayMonth, func(m []string) (dateKey, bool) { return dateOf(m[1], monthOf(m[2])) }},
	{monthDay, func(m []string) (dateKey, bool) { return dateOf(m[2], monthOf(m[1])) }},
	{numericDay, func(m []string) (dateKey, bool) { mo, _ := strconv.Atoi(m[2]); return dateOf(m[1], time.Month(mo)) }},
}

// datesIn collects into set the days the code writes in a text: ISO dates and
// "25 Nov" (a tool result, the plan state, the facts block). With loose (the
// user's own message) the other forms count too.
func datesIn(text string, set map[dateKey]bool, loose bool) {
	for i, dm := range dateMatchers {
		if !loose && i > 1 {
			break
		}
		for _, m := range dm.re.FindAllStringSubmatch(text, -1) {
			if k, ok := dm.key(m); ok {
				set[k] = true
			}
		}
	}
}

// withoutKnownDates blanks every date of text that names a day of known, so its
// day and month numbers are no figures (audit V58); any other date stays, and
// the lint checks its numbers as before. A year after a date is no part of it:
// "22 Nov 2196 kcal" keeps 2196 as a number. A weekday word before a date of
// year that is not the date's weekday leaves the date checked; a numeric date
// that reads as a decimal ("12.5." ending a sentence) stays a number too.
func withoutKnownDates(text string, known map[dateKey]bool, year int) string {
	for _, dm := range dateMatchers {
		var b strings.Builder
		last := 0
		for _, ix := range dm.re.FindAllStringSubmatchIndex(text, -1) {
			m := make([]string, len(ix)/2)
			for g := range m {
				if ix[2*g] >= 0 {
					m[g] = text[ix[2*g]:ix[2*g+1]]
				}
			}
			k, ok := dm.key(m)
			if !ok || !known[k] || (dm.re == numericDay && readsAsDecimal(text, ix[1], m)) || weekdayMismatch(text[:ix[0]], k, year) {
				continue
			}
			b.WriteString(text[last:ix[0]])
			b.WriteString(" ")
			last = ix[1]
		}
		b.WriteString(text[last:])
		text = b.String()
	}
	return text
}

// isoYear is the year of the first ISO date of text, 0 for none.
func isoYear(text string) int {
	if m := isoAnywhere.FindString(text); m != "" {
		y, _ := strconv.Atoi(m[:4])
		return y
	}
	return 0
}

// readsAsDecimal reports whether a numeric date "12.5." (m its match, end the
// index after it) is a decimal that ends a sentence rather than a date: a
// one-digit month and no lowercase word after it.
func readsAsDecimal(text string, end int, m []string) bool {
	if len(m[2]) != 1 || strings.HasSuffix(m[0], ")") {
		return false
	}
	rest := strings.TrimLeft(text[end:], " \t\n")
	r, _ := utf8.DecodeRuneInString(rest)
	return rest == "" || !unicode.IsLower(r)
}

// weekdayMismatch reports whether the text before a date ends on a weekday word
// that is not the weekday of k in year (0 for no year known: no check).
func weekdayMismatch(before string, k dateKey, year int) bool {
	if year == 0 {
		return false
	}
	fields := strings.Fields(strings.TrimRight(before, " ,("))
	if len(fields) == 0 {
		return false
	}
	w := strings.Trim(fields[len(fields)-1], ".,")
	if !weekdayRe.MatchString(w) || weekdayRe.FindString(w) != w {
		return false
	}
	want := time.Date(year, k.month, k.day, 0, 0, 0, 0, time.UTC).Weekday().String()[:3]
	return weekdayKey(w) != want
}

// backedBy reports whether the model's number n is a figure of one of sets, or
// such a figure rounded to n's decimals (78 for 78.08, 80.6 for 80.56). A
// coarser rounding is not backed: 2200 rounds no figure 2196.
func backedBy(n string, sets ...map[string]bool) bool {
	for _, s := range sets {
		if s[n] {
			return true
		}
	}
	v, err := strconv.ParseFloat(n, 64)
	if err != nil {
		return false
	}
	k := decimals(n)
	p := math.Pow10(k)
	for _, s := range sets {
		for x := range s {
			if decimals(x) <= k {
				continue
			}
			if f, err := strconv.ParseFloat(x, 64); err == nil && math.Abs(math.Round(f*p)/p-v) < 1e-9 {
				return true
			}
		}
	}
	return false
}

// decimals is the number of digits after the decimal point of a normalised number.
func decimals(n string) int {
	if i := strings.IndexByte(n, '.'); i >= 0 {
		return len(n) - i - 1
	}
	return 0
}

// numbersIn collects the numbers of a text into set.
func numbersIn(text string, set map[string]bool) {
	for _, m := range numberRe.FindAllString(text, -1) {
		set[normNumber(m)] = true
	}
}

// sentences splits a text into its sentences (a sentence ends on . ! or ? and a
// space or the end; a decimal point is no end, and neither is the dot of a
// German date, "8. Dezember", "25. Nov. Abendessen": RP7).
func sentences(s string) []string {
	var out []string
	var cur []rune
	rs := []rune(strings.TrimSpace(s))
	for i, r := range rs {
		cur = append(cur, r)
		if r == '.' && i < len(rs)-1 && (dateDot(rs, i) || abbrevDot(rs, i)) {
			continue
		}
		if (r == '.' || r == '!' || r == '?') && (i == len(rs)-1 || unicode.IsSpace(rs[i+1])) {
			out = append(out, strings.TrimSpace(string(cur)))
			cur = nil
		}
	}
	if t := strings.TrimSpace(string(cur)); t != "" {
		out = append(out, t)
	}
	return out
}

// abbreviations end with a dot that ends no sentence ("ca.", "z. B.", "bzw.").
var abbreviations = map[string]bool{"ca": true, "bzw": true, "ggf": true, "evtl": true, "inkl": true, "vgl": true, "z.b": true, "u.a": true, "e.g": true, "i.e": true}

// abbrevDot reports whether the dot at rs[i] closes an abbreviation: one of
// abbreviations, or the second letter of "z. B." and "u. a.".
func abbrevDot(rs []rune, i int) bool {
	start := i
	for start > 0 && !unicode.IsSpace(rs[start-1]) {
		start--
	}
	word := strings.ToLower(strings.TrimLeft(string(rs[start:i]), "("))
	if abbreviations[word] {
		return true
	}
	// The first dot of "z. B." / "u. a.": the next word is the other half.
	if (word == "z" && followedBy(rs, i, 'b')) || (word == "u" && followedBy(rs, i, 'a')) {
		return true
	}
	if word != "b" && word != "a" {
		return false
	}
	p := start
	for p > 0 && unicode.IsSpace(rs[p-1]) {
		p--
	}
	q := p
	for q > 0 && !unicode.IsSpace(rs[q-1]) {
		q--
	}
	prev := strings.ToLower(strings.TrimLeft(string(rs[q:p]), "("))
	return (word == "b" && prev == "z.") || (word == "a" && prev == "u.")
}

// followedBy reports whether the dot at rs[i] is followed by a space and the
// letter c (any case) and a dot.
func followedBy(rs []rune, i int, c rune) bool {
	j := i + 1
	for j < len(rs) && unicode.IsSpace(rs[j]) {
		j++
	}
	return j > i+1 && j+1 < len(rs) && unicode.ToLower(rs[j]) == c && rs[j+1] == '.'
}

// monthWord is a month name or its abbreviation, German or English.
var monthWord = regexp.MustCompile(`(?i)^(jan(uar|uary)?|feb(ruar|ruary)?|m[äa]r(z|ch)?|apr(il)?|mai|may|jun[ie]?|jul[iy]?|aug(ust)?|sept?(ember)?|o[ck]t(ober)?|nov(ember)?|de[cz](ember)?)$`)

// numericDate is a German numeric date before its last dot ("21.11", "(30.11").
var numericDate = regexp.MustCompile(`^\(?(\d{1,2})\.(\d{1,2})$`)

// monthFull is a month's own word ("Mai", "Juni"): its dot ends a sentence.
var monthFull = regexp.MustCompile(`(?i)^(mai|may|jun[ie]?|jul[iy]?|m[äa]rz|march|apr(il)?)$`)

// dateDot reports whether the dot at rs[i] belongs to a date: the dot of a 1-2
// digit day number before a month word ("8. Dezember"), or of a month's
// abbreviation ("Dez.") that a date goes on after: a day number before it
// ("7. Dez. Abendessen") or a lowercase letter, a digit or a comma after it.
// The dot of a full month word ("im Mai. Der Tag") ends the sentence.
func dateDot(rs []rune, i int) bool {
	start := i
	for start > 0 && !unicode.IsSpace(rs[start-1]) {
		start--
	}
	word := string(rs[start:i])
	if m := numericDate.FindStringSubmatch(word); m != nil {
		if _, ok := dateOf(m[1], monthNumber(m[2])); !ok {
			return false
		}
		// "nach dem 21.11. ist": the sentence goes on in lowercase.
		n := i + 1
		for n < len(rs) && unicode.IsSpace(rs[n]) {
			n++
		}
		return n < len(rs) && unicode.IsLower(rs[n])
	}
	if monthWord.MatchString(word) && len([]rune(word)) <= 4 && !monthFull.MatchString(word) {
		p := start
		for p > 0 && unicode.IsSpace(rs[p-1]) {
			p--
		}
		q := p
		for q > 0 && !unicode.IsSpace(rs[q-1]) {
			q--
		}
		if prev := strings.TrimSuffix(string(rs[q:p]), "."); p > q && len(prev) <= 2 && prev != "" && strings.Trim(prev, "0123456789") == "" {
			return true // "7. Dez."
		}
		n := i + 1
		for n < len(rs) && unicode.IsSpace(rs[n]) {
			n++
		}
		return n < len(rs) && (unicode.IsLower(rs[n]) || unicode.IsDigit(rs[n]) || rs[n] == ',')
	}
	if n := len([]rune(word)); n == 0 || n > 2 || strings.Trim(word, "0123456789") != "" {
		return false
	}
	end := i + 1
	for end < len(rs) && unicode.IsSpace(rs[end]) {
		end++
	}
	next := end
	for next < len(rs) && unicode.IsLetter(rs[next]) {
		next++
	}
	return end > i+1 && monthWord.MatchString(string(rs[end:next]))
}

// Claims the model text may make about the four classes of fact; a claim the
// block contradicts rejects the text (RP6).
var (
	cookMovedNo = regexp.MustCompile(`(?i)(nothing (else )?(moves|moved|changes|changed)|no (other )?(meal|cooking|dish)\w* (moves|moved|changes|changed)|cooking (stays|remains|is unchanged|does not change|doesn't change)|nichts (anderes |weiter )?(verschoben|geändert|ändert sich)|kochen (bleibt|ändert sich nicht)|keine (weitere|andere) (mahlzeit|änderung))`)
	leftoverNo  = regexp.MustCompile(`(?i)(no leftovers?|without leftovers?|nothing (is )?left ?over|keine reste|ohne reste|nichts bleibt übrig)`)
	issuesNo    = regexp.MustCompile(`(?i)(no (new )?(problems?|warnings?|issues?)|nothing (is )?(wrong|off)|everything (still )?(fits|is fine|works)|all (is )?fine|alles (passt|in ordnung|ok)|keine (neuen )?(probleme|warnungen))`)
	rangeIn     = regexp.MustCompile(`(?i)(within|inside|in) (its|the|a good) (range|amount)|(im|innerhalb des) (passenden )?bereich|passt in den bereich`)
	dayOver     = regexp.MustCompile(`(?i)\b(day|tag)\b[^.]*\b(over|above|too much|über|darüber|zu viel)\b|\b(over|above) (its|the) (range|amount)`)
	dayUnder    = regexp.MustCompile(`(?i)\b(day|tag)\b[^.]*\b(under|below|short|too little|unter|darunter|zu wenig)\b|\b(under|below) (its|the) (range|amount|minimum)`)
)

// Word classes the model's text on a write turn may not use: the block states
// the cooking and the leftovers, and the one offer is code-rendered (G4, CL3,
// CL4). A narrow vocabulary check in English and German, not a parser.
var (
	modelCookingWords = regexp.MustCompile(`(?i)\b(cook\w*|batch(es)?|leftovers?|left ?overs?|fresh(ly)?|portions? (are |is )?left|kochen|koch\w*|gekocht\w*|kochzeit\w*|reste[n]?|übrig\w*|frisch\w*)\b`)
	// "I can't", "unless another meal grows", "weil der Tag sonst ..." and
	// "instead of" (withoutComparisons) are refusals or reasons, no option.
	modelOptionWords = regexp.MustCompile(`(?i)\b(if you|only if|unless you|i could|i'?d (suggest|recommend)|want me to|would you like|shall i|should i|alternatively|instead|otherwise|falls du|wenn du|nur wenn|ich kann|ich könnte|du kannst|du könntest|möchtest du|willst du|soll ich|stattdessen|alternativ)\b|\b(i can|you (could|can|may|might)) \w|\b(sonst )?(kann|könnte|soll) ich\b|(^|[.!?:;]\s+)sonst\b`)
	// insteadOf is a done comparison: between quantities ("100 g instead of
	// 200 g", "3 statt 4 Portionen") or after a statement ("is now ... instead
	// of ..."). "Instead of the pasta, try rice" carries a proposal and stays.
	insteadOf = regexp.MustCompile(`(?i)(\d[^.!?]*?\b(instead of|statt)\b[^.!?]*?\d|\b(is|are|was|were|now|ist|sind|jetzt|habe|have|has|hat)\b[^.!?]*?\b(instead of|statt)\b)`)
)

// A done-claim next to a refused call (nothing saved) says what did not happen.
var (
	doneClaim = regexp.MustCompile(`(?i)\b(moved|swapped|replaced|added|removed|changed|(is|are) now|verschoben|getauscht|ersetzt|hinzugefügt|entfernt|geändert|ist jetzt)\b`)
	negation  = regexp.MustCompile(`(?i)(\bnot\b|n't|\bcannot\b|\bno\b|\bnothing\b|\bnicht\b|\bkeine?\b|\bnichts\b)`)
)

// contradicts reports a model text that denies what the block states.
func contradicts(text string, facts []fact, nothingSaved bool) bool {
	if nothingSaved && doneClaim.MatchString(text) && !negation.MatchString(text) {
		return true
	}
	has := func(kind string) bool {
		for _, f := range facts {
			if f.Kind == kind {
				return true
			}
		}
		return false
	}
	status := func(st string) bool {
		for _, f := range facts {
			if f.Kind == kindDay && f.Status == st {
				return true
			}
		}
		return false
	}
	open := false
	for _, f := range facts {
		if f.Class == classWorse || f.Class == classWarn || f.Class == classRefused {
			open = true
		}
	}
	switch {
	case cookMovedNo.MatchString(text) && (has(kindCook) || has(kindLeftover) || has(kindAway) || has(kindGuest)):
		return true
	case leftoverNo.MatchString(text) && has(kindLeftover):
		return true
	case issuesNo.MatchString(text) && open:
		return true
	case rangeIn.MatchString(text) && (status(statusUnder) || status(statusOver)):
		return true
	case dayOver.MatchString(text) && status(statusUnder) && !status(statusOver):
		return true
	case dayUnder.MatchString(text) && status(statusOver) && !status(statusUnder):
		return true
	}
	return false
}

// withoutComparisons drops a done comparison ("100 g instead of 200 g") before
// the option check; an "instead of" that proposes something stays.
func withoutComparisons(text string) string { return insteadOf.ReplaceAllString(text, " ") }

// lintModelText reports why the model's text cannot go out next to the block, or
// "" when it can: a planner word, a number the block or the tool results do not
// back (v1.4 P-7; on a turn without a block the user's message backs one too), a
// weekday the block does not name, or a claim the block contradicts.
func lintModelText(text string, t replyTurn, lg lang) string {
	if text == "" {
		return ""
	}
	withBlock := t.write && t.block != nil
	if withBlock {
		if banned.MatchString(text) {
			return "planner_word"
		}
		blockText := ""
		for _, f := range t.block.facts {
			blockText += " " + f.line(lg)
		}
		if t.block.offer != nil {
			blockText += " " + t.block.offer.EN + " " + t.block.offer.DE
		}
		// v1.4 P-7 (G4/RP4): a number of the model's text equals one of the
		// facts block or of the turn's tool results; any other is dropped.
		for _, r := range text {
			if unicode.IsNumber(r) && (r < '0' || r > '9') {
				return "number" // ½, ², a full-width digit: no tool result writes one
			}
		}
		backed := map[string]bool{}
		numbersIn(isoAnywhere.ReplaceAllString(blockText, " "), backed)
		known := maps.Clone(t.dates)
		if known == nil {
			known = map[dateKey]bool{}
		}
		datesIn(blockText, known, false)
		for _, m := range numberRe.FindAllString(withoutKnownDates(text, known, t.year), -1) {
			if n := normModelNumber(m, lg); !backedBy(n, backed, t.numbers) {
				return "number"
			}
		}
		if modelCookingWords.MatchString(text) {
			return "cooking_claim"
		}
		if modelOptionWords.MatchString(withoutComparisons(text)) {
			return "option"
		}
		lower := strings.ToLower(blockText)
		// A dish is named in the block or not at all: a quoted span, or the
		// name of a food of the plan.
		for _, m := range quotedRe.FindAllString(text, -1) {
			if !strings.Contains(lower, strings.ToLower(strings.Trim(m, `"„“”`))) {
				return "dish"
			}
		}
		for _, name := range t.foods {
			if namesFood(text, name) && !namesFood(blockText, name) {
				return "dish"
			}
		}
		for _, w := range weekdayRe.FindAllString(text, -1) {
			// A day the block names, or one the user named (v1.3, P-6).
			if k := weekdayKey(w); k == "" || (!namesWeekday(blockText, k) && !userNamesWeekday(t.user, k)) {
				return "weekday"
			}
		}
		if contradicts(text, t.block.facts, t.block.nothingSaved) {
			return "contradiction"
		}
		return ""
	}
	if banned.MatchString(quotedRe.ReplaceAllString(text, "")) {
		return "planner_word"
	}
	have := map[string]bool{}
	numbersIn(t.user, have)
	for _, m := range numberRe.FindAllString(withoutKnownDates(text, t.dates, t.year), -1) {
		if n := normModelNumber(m, lg); !backedBy(n, t.numbers, t.state, have) {
			return "number"
		}
	}
	return ""
}

// composeReply is the reply a household reads: the model's text (at most two
// sentences, when the lint passes) followed by the facts block and the one offer,
// both rendered by code in the language of the request.
func composeReply(model string, t replyTurn) string {
	text := strings.TrimSpace(plainReply(model))
	lg := t.lang(text)
	drop := func(sentence, reason string) {
		if t.drops != nil {
			*t.drops = append(*t.drops, lintDrop{Sentence: sentence, Reason: reason})
		}
	}
	if t.write {
		// A write turn ends on the block or the offer, never on a question.
		ss := sentences(text)
		kept := ss
		for len(kept) > 0 && strings.HasSuffix(kept[len(kept)-1], "?") {
			kept = kept[:len(kept)-1]
		}
		if len(kept) > 0 || t.block != nil {
			for _, q := range ss[len(kept):] {
				drop(q, "question")
			}
			text = strings.Join(kept, " ")
		}
	}
	if t.block != nil && t.write {
		if ss := sentences(text); len(ss) > 2 {
			for _, x := range ss[2:] {
				drop(x, "third_sentence")
			}
			text = strings.Join(ss[:2], " ")
		}
	}
	if lintModelText(text, t, lg) != "" {
		// The text keeps every sentence the lint passes: a sentence that breaks it
		// is dropped, not the whole text (G4 lint per sentence, v1.3 P-6; RP4).
		var kept []string
		for _, s := range sentences(text) {
			if why := lintModelText(s, t, lg); why != "" {
				drop(s, why)
			} else {
				kept = append(kept, s)
			}
		}
		text = strings.Join(kept, " ")
	}
	if text == "" && strings.TrimSpace(model) != "" && t.block == nil {
		if lg == langDE {
			return fallbackDE
		}
		return fallbackEN
	}
	if why := lintModelText(text, t, lg); why != "" {
		// The kept sentences pass alone but not together (a weekday or a
		// number one names and another needs).
		drop(text, why)
		text = ""
		if t.block == nil {
			if lg == langDE {
				return fallbackDE
			}
			return fallbackEN
		}
	}
	if t.block == nil || !t.write {
		return text
	}
	var parts []string
	if text != "" {
		parts = append(parts, text)
	}
	var lines []string
	if t.block.nothingSaved {
		if lg == langDE {
			lines = append(lines, "- "+nothingChangedDE)
		} else {
			lines = append(lines, "- "+nothingChangedEN)
		}
	}
	for _, f := range t.block.facts {
		lines = append(lines, "- "+f.line(lg))
	}
	if len(lines) > 0 {
		parts = append(parts, strings.Join(lines, "\n"))
	}
	if o := t.block.offer; o != nil {
		if lg == langDE {
			parts = append(parts, o.DE)
		} else {
			parts = append(parts, o.EN)
		}
	}
	if len(parts) == 0 {
		if lg == langDE {
			return "Erledigt."
		}
		return "Done."
	}
	return strings.Join(parts, "\n\n")
}

// lintDrop is a sentence of the model's final text the reply left out, with
// the reason: a lint reason of lintModelText (planner_word,
// cooking_claim, option, dish, weekday, contradiction, number), "question" (a
// write turn's trailing question) or "third_sentence" (past the two a write's
// text may have).
type lintDrop struct {
	Sentence string `json:"sentence"`
	Reason   string `json:"reason"`
}

// finalizeMessage puts the composed reply into the model's final message: the
// first text block carries it, the other text blocks are dropped.
func finalizeMessage(m *llm.Message, t replyTurn) {
	var texts []string
	first := -1
	for i, c := range m.Content {
		if c.Type == llm.ContentTypeText {
			if first < 0 {
				first = i
			}
			texts = append(texts, c.Text)
		}
	}
	if first < 0 && (t.block == nil || !t.write) {
		return
	}
	reply := composeReply(strings.Join(texts, "\n\n"), t)
	out := m.Content[:0:0]
	placed := false
	for i, c := range m.Content {
		switch {
		case c.Type != llm.ContentTypeText:
			out = append(out, c)
		case i == first:
			c.Text = reply
			out = append(out, c)
			placed = true
		}
	}
	if !placed {
		out = append(out, llm.ContentBlock{Type: llm.ContentTypeText, Text: reply})
	}
	m.Content = out
}

// weekdayKey is the English short name of a weekday word in either language.
func weekdayKey(w string) string {
	w = strings.ToLower(w)
	for _, d := range []struct{ key, prefixes string }{
		{"Mon", "mon"}, {"Tue", "tue di"}, {"Wed", "wed mi"}, {"Thu", "thu do"}, {"Fri", "fri fr"}, {"Sat", "sat sa"}, {"Sun", "sun so"},
	} {
		for _, p := range strings.Fields(d.prefixes) {
			if strings.HasPrefix(w, p) {
				return d.key
			}
		}
	}
	return ""
}

// namesWeekday reports whether the block text names a weekday, as "Fri", "Friday", "Freitag" or "Fr.".
func namesWeekday(text, key string) bool {
	en := map[string]string{"Mon": "mon", "Tue": "tue", "Wed": "wed", "Thu": "thu", "Fri": "fri", "Sat": "sat", "Sun": "sun"}[key]
	de := strings.ToLower(deWeekday[key][0])
	re := regexp.MustCompile(`(?i)\b` + en + `\w*|\b` + de + `\b|\b` + strings.TrimSuffix(strings.ToLower(deWeekday[key][1]), ".") + `\.`)
	return re.MatchString(text)
}

// userNamesWeekday reports whether the user's text names the weekday in full, as
// a whole word ("Sunday", "Sonntag", "Sunday's"); "so" or "satisfied" do not count.
func userNamesWeekday(text, key string) bool {
	en := map[string]string{"Mon": "monday", "Tue": "tuesday", "Wed": "wednesday", "Thu": "thursday", "Fri": "friday", "Sat": "saturday", "Sun": "sunday"}[key]
	de := strings.ToLower(deWeekday[key][0])
	return regexp.MustCompile(`(?i)\b(` + en + `|` + de + `)(['’]s)?\b`).MatchString(text)
}

// namesFood reports a text that names the food (a name of four letters or more,
// as a whole word run, case aside).
func namesFood(text, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if len([]rune(name)) < 4 {
		return false
	}
	t := strings.ToLower(text)
	for from := 0; ; {
		i := strings.Index(t[from:], name)
		if i < 0 {
			return false
		}
		i += from
		end := i + len(name)
		before := i == 0 || !isLetter(t[:i])
		after := end == len(t) || !startsWithLetter(t[end:])
		if before && after {
			return true
		}
		from = i + 1
	}
}

func isLetter(prefix string) bool {
	r := []rune(prefix)
	return unicode.IsLetter(r[len(r)-1]) || unicode.IsDigit(r[len(r)-1])
}

func startsWithLetter(s string) bool {
	for _, r := range s {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	return false
}

// toolNumbers collects the figures a tool result holds into set: its numbers and
// the numbers inside its sentences, but not the ids and not dates (ISO or
// "Wed 25 Nov"), which are no figures anyone told.
func toolNumbers(raw []byte, set map[string]bool) {
	var v any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if dec.Decode(&v) != nil {
		return
	}
	var walk func(v any, key string)
	walk = func(v any, key string) {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				walk(e, k)
			}
		case []any:
			for _, e := range x {
				walk(e, key)
			}
		case json.Number:
			if key == "id" || key == "ids" || strings.HasSuffix(key, "_id") || strings.HasSuffix(key, "_ids") {
				return
			}
			set[normNumber(x.String())] = true
		case string:
			if key == "id" || strings.HasSuffix(key, "_id") {
				return
			}
			numbersIn(dateInText.ReplaceAllString(isoAnywhere.ReplaceAllString(x, " "), " "), set)
		}
	}
	walk(v, "")
}

// messageText is the text blocks of a message, joined.
func messageText(m llm.Message) string {
	var parts []string
	for _, c := range m.Content {
		if c.Type == llm.ContentTypeText && strings.TrimSpace(c.Text) != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// earlierUserTexts are the user's messages before this turn's, newest first.
func earlierUserTexts(history []llm.Message) []string {
	var out []string
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role != llm.RoleUser {
			continue
		}
		if t := messageText(history[i]); t != "" {
			out = append(out, t)
		}
	}
	if len(out) > 0 {
		out = out[1:] // the newest is this turn's own message
	}
	return out
}

// sanitizeToolMessage keeps, of the text a model writes before a tool call, only
// what a write's reply may say: a text that breaks the rules (an unbacked number, a weekday
// or a dish the block does not name, a planner word) is dropped, never stored
// nor shown.
func sanitizeToolMessage(m *llm.Message, t replyTurn) {
	strict := t
	strict.write, strict.block = true, &replyBlock{}
	out := m.Content[:0:0]
	for _, c := range m.Content {
		if c.Type == llm.ContentTypeText {
			c.Text = plainReply(c.Text)
			if strings.TrimSpace(c.Text) == "" || lintModelText(c.Text, strict, t.lang(c.Text)) != "" {
				continue
			}
		}
		out = append(out, c)
	}
	m.Content = out
}

// savedBlock is the block of a write whose result carries no report of its own
// (a routine applied, an undo or a regenerate that changed no meal, a plan that
// could not be read): every write is told, with what is known, in both languages.
func savedBlock(tool string, output json.RawMessage) *replyBlock {
	var obj map[string]any
	_ = json.Unmarshal(output, &obj)
	rep, _ := obj["report"].(map[string]any)
	if nc, _ := rep["not_covered"].([]any); len(nc) > 0 {
		return &replyBlock{facts: []fact{{
			Class: classChanged, Kind: kindMeal, Status: statusUnknown,
			EN: "Saved, but I could not read the plan before or after the change, so I cannot list what else moved.",
			DE: "Gespeichert, aber ich konnte den Plan vor oder nach der Änderung nicht lesen und kann deshalb nicht sagen, was sich sonst verschoben hat.",
		}}}
	}
	en, de := "Saved; no other meal changed.", "Gespeichert; keine andere Mahlzeit hat sich geändert."
	switch tool {
	case "undo_last_change":
		if stop, ok := undoStop(obj); ok && num(obj["undone"]) == 0 {
			return &replyBlock{facts: []fact{stop}, nothingSaved: true}
		}
		en, de = "Undone; the plan is as it was before that change.", "Rückgängig gemacht; der Plan ist wie vor dieser Änderung."
		if stop, ok := undoStop(obj); ok {
			return &replyBlock{facts: []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: en, DE: de}, stop}}
		}
	case "apply_preset":
		en, de = "The routine is applied.", "Die Routine ist angewendet."
		var skipped []string
		var skippedDE []string
		for _, k := range []struct{ key, en, de string }{
			{"skipped_occupied", "already had a meal", "hatten schon ein Essen"},
			{"skipped_unavailable", "were marked as away", "waren als abwesend markiert"},
			{"skipped_no_slot", "had no meal slot", "hatten keine Mahlzeit"},
		} {
			if l, _ := obj[k.key].([]any); len(l) > 0 {
				skipped = append(skipped, fmt.Sprintf("%d %s", len(l), k.en))
				skippedDE = append(skippedDE, fmt.Sprintf("%d %s", len(l), k.de))
			}
		}
		if len(skipped) > 0 {
			en += " Skipped cells: " + strings.Join(skipped, ", ") + "."
			de += " Übersprungene Zellen: " + strings.Join(skippedDE, ", ") + "."
		}
	case "regenerate":
		if obj["status"] == "error" {
			return regenerateRefused(obj)
		}
		en, de = "The plan was made again.", "Der Plan wurde neu gemacht."
	}
	return &replyBlock{facts: []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: en, DE: de}}}
}

// undoReasons are the code-rendered reasons of a refused undo, by kind (G4.1).
var undoReasons = map[string][2]string{
	"":            {"no change of this conversation is left to undo.", "in diesem Gespräch ist keine Änderung mehr rückgängig zu machen."},
	undoStale:     {"the plan was changed since that change, so putting it back exactly would overwrite that edit.", "der Plan wurde seit dieser Änderung geändert, und sie genau zurückzusetzen würde diese Änderung überschreiben."},
	undoConfirmed: {"a meal of that change is confirmed now.", "eine Mahlzeit dieser Änderung ist jetzt bestätigt."},
	undoAlready:   {"that change was undone already on the board.", "diese Änderung wurde schon auf dem Plan rückgängig gemacht."},
}

// undoStop is the refused part of an undo_last_change result (G4.1, SF2): with
// nothing undone "Nothing was undone: <why>", after some "The next change was
// not undone: <why>"; ok false when the undo was not refused. A result with
// nothing undone and no refusal had nothing left to undo.
func undoStop(obj map[string]any) (fact, bool) {
	ref, _ := obj["refused"].(map[string]any)
	if _, isUndo := obj["undone"]; !isUndo || (ref == nil && num(obj["undone"]) > 0) {
		return fact{}, false
	}
	kind, _ := ref["kind"].(string)
	if ref != nil && kind == "" {
		kind = undoStale // a stored refusal without its kind: the plan changed since
	}
	why, ok := undoReasons[kind]
	if !ok {
		why = undoReasons[undoStale]
	}
	en, de := "Nothing was undone: ", "Nichts wurde rückgängig gemacht: "
	if num(obj["undone"]) > 0 {
		en, de = "The next change was not undone: ", "Die nächste Änderung wurde nicht rückgängig gemacht: "
	}
	return fact{Class: classRefused, Kind: kindRefused, Status: statusOK, EN: en + why[0], DE: de + why[1]}, true
}

// regenerateRefused is the block of a regenerate run that ended in error and
// saved nothing (KR7: a day replan that would break a rule saves nothing and
// says so): "nothing changed" and the reason, never "the plan was made again".
func regenerateRefused(obj map[string]any) *replyBlock {
	en := "Nothing was planned again: the run could not finish, so nothing was saved."
	de := "Nichts wurde neu geplant: die Planung konnte nicht abgeschlossen werden, deshalb wurde nichts gespeichert."
	switch key, _ := obj["error_key"].(string); key {
	case "error.plan.hard_rule_open":
		en = "Nothing was planned again: the new plan would still break a rule of the plan, so nothing was saved."
		de = "Nichts wurde neu geplant: der neue Plan hätte noch eine Regel des Plans verletzt, deshalb wurde nichts gespeichert."
		notes, _ := obj["notes"].(string)
		if _, rules, ok := strings.Cut(notes, "nothing was saved: "); ok {
			if rules = strings.TrimSuffix(strings.TrimSpace(isoInParens.ReplaceAllString(rules, "")), "."); rules != "" {
				en = fmt.Sprintf("Nothing was planned again: the new plan would still break a rule of the plan (%s), so nothing was saved.", rules)
			}
		}
	case "error.plan.day_same_dish":
		en = "Nothing was planned again: the same main dish would be at both meals of the day and no other fits, so nothing was saved."
		de = "Nichts wurde neu geplant: dasselbe Hauptgericht wäre bei beiden Mahlzeiten des Tages und kein anderes passt, deshalb wurde nichts gespeichert."
	}
	return &replyBlock{facts: []fact{{Class: classRefused, Kind: kindRefused, Status: statusOK, EN: en, DE: de}}, nothingSaved: true}
}

// isoInParens is a parenthesised list of ISO dates in a run note ("(2026-12-07)").
var isoInParens = regexp.MustCompile(`\s*\(\d{4}-\d{2}-\d{2}(?:, \d{4}-\d{2}-\d{2})*\)`)

var (
	// cellID is the slot id in a cell line ("Fri 2026-11-27 | 3 dinner | ...").
	cellID = regexp.MustCompile(`(?m)^(\w+ \d{4}-\d{2}-\d{2} \| )\d+ `)
	// idRef is an id in the plan state: "plate_id 30", "| id 12", "slot_id 3".
	idRef = regexp.MustCompile(`(?i)\b(?:[a-z]+_)?ids?[ :=]*\d+`)
)

// stateNumbers collects the figures of the prompt's plan state (kcal, portions,
// minutes, counts) into set: the plan state is the program's computed view of the
// stored plan, as a get_plan result would be. The rule text around it is not
// state, and ids and ISO dates are no figures.
func stateNumbers(prompt string, set map[string]bool) {
	state := planState(prompt)
	state = cellID.ReplaceAllString(state, "$1")
	state = idRef.ReplaceAllString(state, " ")
	numbersIn(isoAnywhere.ReplaceAllString(state, " "), set)
}

// planState is the plan state section of the prompt; "" without one.
func planState(prompt string) string {
	i := strings.Index(prompt, planStateStart)
	if i < 0 {
		return ""
	}
	state := prompt[i+len(planStateStart):]
	if j := strings.Index(state, planStateEnd); j >= 0 {
		state = state[:j]
	} else if j := strings.Index(state, "\nTool usage rules:"); j >= 0 {
		state = state[:j]
	}
	return state
}
