package agent_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// E036: the reply is in the user's language (the language of the message),
// the report's English sentences are content to translate, and a reply ends
// with the offer or with nothing, never with a bare question.
func TestPrompt_ReplyLanguageAndClosing(t *testing.T) {
	kcal := 2000.0
	for _, locale := range []string{"de", "en", ""} {
		prompt, _ := agent.ComposePrompt(&profile.Profile{KcalTarget: &kcal, HouseholdPortions: 1, Locale: locale}, "", "", time.Time{})
		for _, want := range []string{
			"in the language of the user's message",
			"report.tell is content, not wording",
			"weekday and month names included",
			`("I can also ...: say so if you want it")`,
			"no closing question",
			"name the one you would take in the same sentence",
		} {
			assert.Contains(t, prompt, want, "locale %q", locale)
		}
		assert.False(t, strings.Contains(prompt, "as a question"), "no instruction to end on a question")
	}
}
