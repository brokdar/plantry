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
			"at most two short sentences",
			"\"In range\" is the day's kcal range: answer with its kcal and range", // R4, g2-question-07
			"a number only as the block or the results give it, a day only as the block or the user names it",
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

// Round 7 R1 (tune-v2c g2-add-23, cook_elsewhere-07): the model wrote English
// over a German block when the German message held an English dish name. The
// turn's language is computed in code (dish names left out) and is the
// prompt's last line, so the model writes in the block's language.
func TestMessageLang_DishNamesDecideNothing(t *testing.T) {
	t.Parallel()
	foods := []string{"Low-fat quark", "Greek salad with chickpeas", "Steamed rice", "rice", "Chicken and rice with peas"}
	for _, c := range []struct {
		text, locale string
		want         string
	}{
		{"Füge Low-fat quark zum Freitag hinzu", "", "de"},
		{"Ersetze Chicken and rice with peas am Montag", "", "de"}, // the dish's "and", "with" are no English
		{"Greek salad with chickpeas am Freitag statt Reis", "", "de"},
		{"put Steamed rice with the dinner on Friday", "de", "en"},
		{"ok", "de", "de"},
		{"ok", "", "en"},
	} {
		assert.Equal(t, c.want, agent.MessageLangCode([]string{c.text}, foods, c.locale), c.text)
	}
	assert.Equal(t, "\nReply in German, every sentence (dish names stay as they are).\n", agent.LanguageLine("de"))
	assert.Equal(t, "\nReply in English, every sentence.\n", agent.LanguageLine("en"))
}
