package agent

import (
	"regexp"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// plainReply is the last line of defence for the words a household reads: the
// planner's terms that are unambiguous (several words, or clearly technical)
// are rewritten in the final reply, whatever the model wrote. Single common
// words (band, floor, cap) are only rewritten where a number or an energy word
// makes the meaning certain.
var replyWords = []struct {
	re *regexp.Regexp
	to string
}{
	{regexp.MustCompile(`\s*\((?:(?:food|plate|component|batch|session)_)?id[ :=]*\d+\)`), ""},
	{regexp.MustCompile(`\b(?:food|plate|component|plate_component|session)_id[ :=]*\d+\b`), "it"},
	{regexp.MustCompile(`(?i)\b(energy|kcal|calorie|calories|day|daily|week|weekly|healthy|target) band\b`), "$1 range"},
	{regexp.MustCompile(`(?i)(\d\s*(?:%|kcal)?)\s+band\b`), "$1 range"},
	{regexp.MustCompile(`(?i)\b(within|inside|outside|above|below|over|under|in|out of) (its|the|that|this|their) band\b`), "$1 $2 range"},
	{regexp.MustCompile(`(?i)\b(energy|kcal|calorie|calories|day|daily|minimum-range|range) floor\b`), "minimum"},
	{regexp.MustCompile(`(?i)(\d[\d.,]*\s*(?:kcal)?)\s+floor\b`), "$1 limit"},
	{regexp.MustCompile(`(?i)\bhearty minimum\b`), "minimum for a filling meal"},
	{regexp.MustCompile(`(?i)\blight caps?\b`), "limit for a light meal"},
	{regexp.MustCompile(`(?i)\bcooking occasions\b`), "cooking times"},
	{regexp.MustCompile(`(?i)\bcooking occasion\b`), "cooking time"},
	{regexp.MustCompile(`(?i)\badd-on limit\b`), "limit for extras"},
	{regexp.MustCompile(`(?i)\badd-ons\b`), "extras"},
	{regexp.MustCompile(`(?i)\badd-on\b`), "extra"},
	{regexp.MustCompile(`(?i)\bsoft (issues|items|warnings)\b`), "warnings"},
	{regexp.MustCompile(`(?i)\bsoft (issue|item|warning)\b`), "warning"},
	{regexp.MustCompile(`(?i)\bhard (issues|rules)\b`), "problems"},
	{regexp.MustCompile(`(?i)\bhard (issue|rule)\b`), "problem"},
	{regexp.MustCompile(`\bthe tool (only |also |still |does not |doesn't |can only |says |shows )`), "it $1"},
	{regexp.MustCompile(`\bthe tool\b`), "the planner"},
	{regexp.MustCompile(`\bZusatzgrenze\b`), "Grenze für Extras"},
	{regexp.MustCompile(`\bZusatz-Warnungen\b`), "Warnungen zu Extras"},
}

// plainReply rewrites a reply text; ISO dates become "Wed 25 Nov".
func plainReply(s string) string {
	s = plainDates(s)
	for _, w := range replyWords {
		s = w.re.ReplaceAllString(s, w.to)
	}
	return s
}

// plainReplyMessage rewrites the text blocks of the model's final message.
func plainReplyMessage(m *llm.Message) {
	for i := range m.Content {
		if m.Content[i].Type == llm.ContentTypeText {
			m.Content[i].Text = plainReply(m.Content[i].Text)
		}
	}
}
