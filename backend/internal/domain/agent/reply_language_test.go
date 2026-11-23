package agent_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// G4/G5: the program puts the facts and the offer after the model's text in the
// language of the request; the model writes at most two sentences without
// digits, and no prompt sentence has it say every item or end on a question.
func TestPrompt_ReplyContract(t *testing.T) {
	t.Parallel()
	kcal := 2000.0
	for _, locale := range []string{"de", "en", ""} {
		prompt, _ := agent.ComposePrompt(&profile.Profile{KcalTarget: &kcal, HouseholdPortions: 1, Locale: locale}, "", "", time.Time{})
		for _, want := range []string{
			"in the language of the user's message",
			"at most two short sentences",
			"a number or day only as the results give it, no dish name",
			"list, question or offer",
			"names the one you would take",
			"Out of scope: decline in one sentence",
			"the Recipes page", "the Shopping page", "the Settings page", "the Planner", "the Plan board",
		} {
			assert.Contains(t, prompt, want, "locale %q", locale)
		}
		for _, gone := range []string{"Say every item of report.tell", "report.tell is content, not wording", "as a question", "I can also ...: say so if you want it", "no digit"} {
			assert.NotContains(t, prompt, gone, "locale %q", locale)
		}
	}
}
