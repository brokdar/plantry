// Package plainwords holds the rewriter of planner words into kitchen words
// that the chat's final reply and the planner's saved note both pass through
// (agent and planning each import it; it imports nothing of the domain).
package plainwords

import (
	"regexp"
	"strings"
	"time"
)

var isoDay = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)

// Dates replaces every ISO date in a text with the day as people say it ("Wed 25 Nov").
func Dates(s string) string {
	return isoDay.ReplaceAllStringFunc(s, func(d string) string {
		if t, err := time.Parse(time.DateOnly, d); err == nil {
			return t.Format("Mon 2 Jan")
		}
		return d
	})
}

// Reply is the last line of defence for the words a household reads: the
// planner's terms are rewritten in the final reply, whatever the model wrote.
// It is careful, since a reply is also everyday language: nothing inside a
// quoted name ("soft cheese"), and the words that mean something else outside
// the plan (band, soft, hard, occasion, tool) only on a line that talks about
// the plan (kcal, a day, the week, a warning ...).
type replyRule struct {
	re      *regexp.Regexp
	to      string
	needCtx bool
}

var replyRules = []replyRule{
	{regexp.MustCompile(`\s*\((?:(?:food|plate|component|batch|session)_)?id[ :=]*\d+\)`), "", false},
	{regexp.MustCompile(`\b(?:food|plate|component|plate_component|session)_id[ :=]*\d+\b`), "it", false},
	{regexp.MustCompile(`(?i)\b(energy|kcal|calorie|calories|weekly|healthy|target) band\b`), "$1 range", false},
	{regexp.MustCompile(`(?i)(\d\s*(?:%|kcal)?)\s+band\b`), "$1 range", false},
	{regexp.MustCompile(`(?i)\b(within|inside|outside|above|below|over|under|out of) (its|the|that|this) band\b`), "$1 $2 range", true},
	{regexp.MustCompile(`(?i)\b(energy|kcal|calorie|calories) floor\b`), "minimum", false},
	{regexp.MustCompile(`(?i)(\d[\d.,]*\s*(?:kcal)?)\s+floor\b`), "$1 limit", true},
	{regexp.MustCompile(`(?i)\bhearty minimum\b`), "minimum for a filling meal", false},
	{regexp.MustCompile(`(?i)\blight caps?\b`), "limit for a light meal", false},
	{regexp.MustCompile(`(?i)\bcooking occasions\b`), "cooking times", true},
	{regexp.MustCompile(`(?i)\bcooking occasion\b`), "cooking time", true},
	{regexp.MustCompile(`(?i)\badd-on limit\b`), "limit for sides", false},
	{regexp.MustCompile(`(?i)\badd-ons\b`), "sides", false},
	{regexp.MustCompile(`(?i)\badd-on\b`), "side", false},
	{regexp.MustCompile(`(?i)\bsoft (issues|warnings)\b`), "warnings", true},
	{regexp.MustCompile(`(?i)\bsoft (issue|warning)\b`), "warning", true},
	{regexp.MustCompile(`(?i)\bsessions\b`), "batches", true},
	{regexp.MustCompile(`(?i)\bsession\b`), "batch", true},
	{regexp.MustCompile(`(?i)\bhard issues\b`), "problems", true},
	{regexp.MustCompile(`(?i)\bhard issue\b`), "problem", true},
	{regexp.MustCompile(`(?i)\brequired[ -]rules?(?: (?:problem|breach)s?)?\b`), "must", true},
	{regexp.MustCompile(`\bthe tool (only |also |still |does not |doesn't |can only |says |shows |returned |gave |allows |refused )`), "it $1", false},
	{regexp.MustCompile(`(?i)\btool results?\b`), "result", false},
	{regexp.MustCompile(`\bZusatzgrenze\b`), "Grenze für Beilagen", false},
	{regexp.MustCompile(`\bZusatz-Warnungen\b`), "Warnungen zu Beilagen", false},
}

// planLine says a line talks about the plan.
var planLine = regexp.MustCompile(`(?i)\d|\b(kcal|calories?|energy|protein|target|range|minimum|limit|warnings?|problems?|issues?|dinner|lunch|breakfast|batch|plan|week|weekly|day|days|portions?|monday|tuesday|wednesday|thursday|friday|saturday|sunday|mon|tue|wed|thu|fri|sat|sun)\b`)

// anArticle repairs "an side" after a replacement.
var anArticle = regexp.MustCompile(`\b([Aa])n (sides?|musts?|limits?|ranges?|minimum|cooking|filling|problems?|warnings?)\b`)

var quoted = regexp.MustCompile(`"[^"\n]*"|“[^”\n]*”`)

// Reply rewrites a reply text; ISO dates become "Wed 25 Nov". Quoted
// names stay as they are.
func Reply(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		ctx := planLine.MatchString(line)
		var b strings.Builder
		last := 0
		rewrite := func(seg string) string {
			seg = Dates(seg)
			for _, r := range replyRules {
				if r.needCtx && !ctx {
					continue
				}
				seg = r.re.ReplaceAllString(seg, r.to)
			}
			return anArticle.ReplaceAllString(seg, "$1 $2")
		}
		for _, m := range quoted.FindAllStringIndex(line, -1) {
			b.WriteString(rewrite(line[last:m[0]]))
			b.WriteString(line[m[0]:m[1]])
			last = m[1]
		}
		b.WriteString(rewrite(line[last:]))
		lines[i] = b.String()
	}
	return strings.Join(lines, "\n")
}
