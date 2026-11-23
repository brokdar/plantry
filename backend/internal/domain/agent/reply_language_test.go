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
		{"ok", "", ""}, // nothing says: no language is asserted (round 8 P1)
	} {
		assert.Equal(t, c.want, agent.MessageLangCode([]string{c.text}, foods, c.locale), c.text)
	}
	assert.Equal(t, "\nReply in German, every sentence (dish names stay as they are).\n", agent.LanguageLine("de"))
	assert.Equal(t, "\nReply in English, every sentence.\n", agent.LanguageLine("en"))
}

// Round 8 P1 (l1r7-tune g2-question-06, g2-injection-05): "wie lange dauert
// <English dish>" got an English reply. With the dish left out no marker word
// was left, so the earlier English message ("Reply with just the word OK.")
// or the household's "en" locale decided. A short question decides by its
// question word and verb, in both languages; an ambiguous message falls back
// to the earlier messages, then the locale, and else asserts nothing.
func TestMessageLang_ShortQuestionsDecide(t *testing.T) {
	t.Parallel()
	foods := []string{"Overnight oats", "Tomato lentil stew", "Red lentil soup"}
	earlier := "Reply with just the word OK."
	for _, c := range []struct {
		texts  []string
		locale string
		want   string
	}{
		{[]string{"wie lange dauert Overnight oats", earlier}, "en", "de"},
		{[]string{"wie lange dauert Tomato lentil stew", earlier}, "en", "de"},
		{[]string{"was kostet Red lentil soup?"}, "en", "de"},
		{[]string{"gibt es Red lentil soup am Sonntag"}, "en", "de"},
		{[]string{"wieviel Eiweiß hat Overnight oats"}, "en", "de"},
		{[]string{"how long does Overnight oats take", "Wie sieht mein Plan aus?"}, "de", "en"},
		{[]string{"how long is Tomato lentil stew"}, "de", "en"},
		{[]string{"what about Red lentil soup"}, "de", "en"},
		// ambiguous: the earlier message, then the locale, else nothing
		{[]string{"Overnight oats?", "wie sieht mein Plan aus"}, "en", "de"},
		{[]string{"Overnight oats?"}, "de", "de"},
		{[]string{"Overnight oats?"}, "", ""},
	} {
		assert.Equal(t, c.want, agent.MessageLangCode(c.texts, foods, c.locale), c.texts[0])
	}
}
