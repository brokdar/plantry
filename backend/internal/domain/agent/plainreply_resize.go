package agent

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// resizeTold returns the resized-batch items of a result's report.tell
// ("Sun 22 Nov: cook 3.5 portions of the ... (was 3)."), in order, and whether
// the result is a saved change's report (a dry run's is renamed away).
func resizeTold(view json.RawMessage) (items []string, saved bool) {
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
		if resizeLine.MatchString(t) {
			out = append(out, t)
		}
	}
	return out, true
}

var englishWord = regexp.MustCompile(`(?i)\b(the|and|is|are|now|with)\b`)

// restoreResizes keeps a batch cooked on another day from dropping out of the
// final reply: the cook must know the day and the new portions. A resize the
// reply names with neither its day nor its portions in one line is added as
// the tell item itself, before the closing offer. English replies only (the
// item is English; a translated reply is left as the model wrote it).
func restoreResizes(m *llm.Message, items []string) {
	if len(items) == 0 {
		return
	}
	for i := range m.Content {
		b := &m.Content[i]
		if b.Type != llm.ContentTypeText || len(englishWord.FindAllString(b.Text, -1)) < 3 {
			continue
		}
		var missing []string
		for _, it := range items {
			if !resizeSaid(b.Text, it) {
				missing = append(missing, "- "+it)
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

// resizeSaid reports a line of text that holds the item's day and its new
// portion count.
func resizeSaid(text, item string) bool {
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
