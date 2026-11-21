package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"unicode"

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
	if b.offer == nil && (len(b.facts) > 0 || rep != nil) {
		b.offer = offerOf(obj)
	}
	if b.empty() {
		return nil, false
	}
	return b, false
}

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
	user    string          // the user's message, for the language and its numbers
	earlier []string        // the earlier user messages, newest first: the language when this one has no marker word
	locale  string          // the profile's locale, the last word on the language
	numbers map[string]bool // numbers the turn's tool results hold (see toolNumbers)
	foods   []string        // the names of the foods of the plan: a write's text names none the block does not
	write   bool            // a plan-writing tool was called (also as a check)
	block   *replyBlock     // the facts and the offer of the turn's writes
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
	out := &replyBlock{facts: selectBlock(append(slices.Clone(a.facts), b.facts...)), offer: a.offer}
	if b.offer != nil {
		out.offer = b.offer
	}
	return out
}

var writeTools = map[string]bool{"apply_changes": true, "undo_last_change": true, "regenerate": true, "apply_preset": true}

const (
	fallbackEN = "I could not put that into words I can check, so I left my own remarks out. Ask again and name the day or meal."
	fallbackDE = "Das konnte ich nicht in Worten fassen, die ich prüfen kann, deshalb habe ich meine Anmerkungen weggelassen. Frag bitte noch einmal und nenne den Tag oder die Mahlzeit."
)

var (
	numberRe    = regexp.MustCompile(`\d[\d.,]*\d|\d`)
	digitRe     = regexp.MustCompile(`\p{N}`)
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
	return strings.Replace(s, ",", ".", 1)
}

// numbersIn collects the numbers of a text into set.
func numbersIn(text string, set map[string]bool) {
	for _, m := range numberRe.FindAllString(text, -1) {
		set[normNumber(m)] = true
	}
}

// sentences splits a text into its sentences (a sentence ends on . ! or ? and a
// space or the end; a decimal point is no end).
func sentences(s string) []string {
	var out []string
	var cur []rune
	rs := []rune(strings.TrimSpace(s))
	for i, r := range rs {
		cur = append(cur, r)
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

// contradicts reports a model text that denies what the block states.
func contradicts(text string, facts []fact) bool {
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

// lintModelText reports why the model's text cannot go out next to the block, or
// "" when it can: a planner word, a number the block or the tool results do not
// back (on a write turn: any digit at all), a weekday the block does not name, or
// a claim the block contradicts.
func lintModelText(text string, t replyTurn, lg lang) string {
	if text == "" {
		return ""
	}
	withBlock := t.write && t.block != nil
	if withBlock {
		if banned.MatchString(text) {
			return "planner_word"
		}
		if digitRe.MatchString(text) {
			return "digit"
		}
		blockText := ""
		for _, f := range t.block.facts {
			blockText += " " + f.line(lg)
		}
		if t.block.offer != nil {
			blockText += " " + t.block.offer.EN + " " + t.block.offer.DE
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
			if k := weekdayKey(w); k == "" || !namesWeekday(blockText, k) {
				return "weekday"
			}
		}
		if contradicts(text, t.block.facts) {
			return "contradiction"
		}
		return ""
	}
	if banned.MatchString(quotedRe.ReplaceAllString(text, "")) {
		return "planner_word"
	}
	have := map[string]bool{}
	numbersIn(t.user, have)
	for _, m := range numberRe.FindAllString(text, -1) {
		n := normNumber(m)
		if !t.numbers[n] && !have[n] {
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
	if t.write {
		// A write turn ends on the block or the offer, never on a question.
		ss := sentences(text)
		kept := ss
		for len(kept) > 0 && strings.HasSuffix(kept[len(kept)-1], "?") {
			kept = kept[:len(kept)-1]
		}
		if len(kept) > 0 || t.block != nil {
			text = strings.Join(kept, " ")
		}
	}
	if t.block != nil && t.write {
		if ss := sentences(text); len(ss) > 2 {
			text = strings.Join(ss[:2], " ")
		}
	}
	if lintModelText(text, t, lg) != "" {
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
// the numbers inside its sentences and day labels, but not the ids and not ISO
// dates, which are no figures anyone told.
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
			if key == "id" || strings.HasSuffix(key, "_id") || strings.HasSuffix(key, "_ids") {
				return
			}
			set[normNumber(x.String())] = true
		case string:
			if key == "id" || strings.HasSuffix(key, "_id") {
				return
			}
			numbersIn(isoAnywhere.ReplaceAllString(x, " "), set)
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
// what a write's reply may say: a text that breaks the rules (a digit, a weekday
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
		en, de = "Undone; the plan is as it was before that change.", "Rückgängig gemacht; der Plan ist wie vor dieser Änderung."
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
		en, de = "The plan was made again.", "Der Plan wurde neu gemacht."
	}
	return &replyBlock{facts: []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: en, DE: de}}}
}
