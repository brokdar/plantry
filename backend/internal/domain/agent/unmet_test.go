package agent

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// S-B (ledger V77, safety-v2b s-diet-00-k2 / s-diet-01-k3, safety-v2
// s-diet-01-k1): a dish against the diet asked for a named meal was answered
// in prose with no write, and the reply did not always say what was left out.
// The refused part is now told in code, with the diet as the reason.
func TestUnmetChange_TellsWhatWasLeftOutInCode(t *testing.T) {
	t.Parallel()
	slots := map[int64]string{3: "slot.dinner"}
	st := planning.State{Profile: planning.Profile{DietType: "vegetarian"}, Foods: map[int64]planning.FoodFacts{
		9: {ID: 9, Name: "Pork roast", Kind: food.KindComposed, DietClass: "omnivore"},
	}}
	sun := map[string]bool{"2026-11-22|3": true}

	for text, wantEN := range map[string]string{
		"can i get the Beef goulash on Sunday dinner instead": `Left out: the change to Sun 22 Nov dinner ("Beef" does not fit your vegetarian diet).`,
		"put the Pork roast on Sunday dinner":                 `Left out: the change to Sun 22 Nov dinner ("Pork roast" does not fit your vegetarian diet).`,
		"swap Sunday dinner for something else":               `Left out: the change to Sun 22 Nov dinner (nothing was checked and saved for it).`,
	} {
		b := unmetChange(text, sun, slots, st)
		require.NotNil(t, b, text)
		assert.True(t, b.nothingSaved)
		assert.Equal(t, wantEN, b.facts[0].EN, text)
	}
	de := unmetChange("kann ich am Sonntag zum Abendessen stattdessen Beef goulash haben", sun, slots, st)
	require.NotNil(t, de)
	assert.Equal(t, `Weggelassen: die Änderung an Sonntag, 22. Nov. Abendessen („Beef“ passt nicht zu deiner vegetarischen Ernährung).`, de.facts[0].DE)

	assert.Nil(t, unmetChange("what is on Sunday dinner?", sun, slots, st), "a question changes nothing")
	assert.Nil(t, unmetChange("can i get the Beef goulash instead", nil, slots, st), "no named meal")

	// A turn that wrote nothing tells it after the model's text; a turn with a
	// write block keeps its own.
	turn := replyTurn{user: "can i get the Beef goulash on Sunday dinner instead", unmet: unmetChange("can i get the Beef goulash on Sunday dinner instead", sun, slots, st)}
	got := composeReply("Beef goulash doesn't fit your vegetarian plan.", turn.final())
	assert.True(t, strings.HasPrefix(got, "Beef goulash doesn't fit your vegetarian plan."), "the model's text stays first: %s", got)
	assert.Contains(t, got, nothingChangedEN)
	assert.Contains(t, got, `Left out: the change to Sun 22 Nov dinner ("Beef" does not fit your vegetarian diet).`)
	written := turn
	written.write, written.block = true, &replyBlock{facts: []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Sun 22 Nov dinner: A replaced by B.", DE: "x"}}}
	assert.NotContains(t, composeReply("Done.", written.final()), "Left out")
}

// S-C (ledger V77, dev-v2 g2-planner-13): after a saved replan the reply said
// "Die Änderungen sind noch nicht gespeichert, bis du sie bestätigst". A saved
// change is never told as unsaved or waiting for a confirmation: the sentence
// is dropped, and the regenerate result says it is saved.
func TestContradicts_SavedChangeIsNeverUnsaved(t *testing.T) {
	t.Parallel()
	saved := []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Sat 21 Nov dinner: A replaced by B.", DE: "x"}}
	for _, text := range []string{
		"Die Änderungen sind noch nicht gespeichert, bis du sie bestätigst.",
		"The changes are not saved until you confirm them.",
	} {
		assert.True(t, contradicts(text, saved, false, false), text)
	}
	assert.False(t, contradicts("The week is planned again.", saved, false, false))
	refused := append(slices.Clone(saved), fact{Class: classRefused, Kind: kindRefused, Status: statusOK, EN: "Left out: x.", DE: "x"})
	assert.False(t, contradicts("The swap on Monday was not saved.", refused, false, false), "a part left out may be unsaved")
	assert.NotContains(t, regenerateSaved, "nothing waits for a confirmation")
	assert.Contains(t, regenerateSaved, "drafts")
}

func unmetFixture() (map[int64]string, planning.State, map[string]bool) {
	slots := map[int64]string{3: "slot.dinner", 2: "slot.lunch"}
	st := planning.State{Profile: planning.Profile{DietType: "vegetarian"}, Foods: map[int64]planning.FoodFacts{
		9: {ID: 9, Name: "Pork roast", Kind: food.KindComposed, DietClass: "omnivore"},
		8: {ID: 8, Name: "Tofu bowl", Kind: food.KindComposed, DietClass: "vegan"},
	}}
	return slots, st, map[string]bool{"2026-11-22|3": true}
}

// H1 (review of PR 180): the unmet block fires only on a turn that ran no
// tool, or only food lookups that found nothing, and whose text does not end
// in a question; it is appended after the model's text, never a write.
func TestUnmetTold_OnlyWithoutToolsAndWithoutAQuestion(t *testing.T) {
	t.Parallel()
	slots, st, mon := unmetFixture()
	mon = map[string]bool{"2026-11-23|3": true}
	user := "can i get the Beef goulash on Monday dinner instead"
	mk := func() replyTurn { return replyTurn{user: user, unmet: unmetChange(user, mon, slots, st)} }
	require.NotNil(t, mk().unmet)

	// No tool: told, after the text, text kept.
	got := composeReply("Beef goulash doesn't fit your plan.", mk().final())
	assert.True(t, strings.HasPrefix(got, "Beef goulash doesn't fit your plan.\n\n- "+nothingChangedEN), got)
	assert.Contains(t, got, "Left out: the change to Mon 23 Nov dinner")
	// The model's own question stays and nothing is added (the G1 ask turn).
	ask := "Which one do you prefer, the lentil stew or the veggie bowl?"
	assert.Equal(t, ask, composeReply(ask, mk().final()))
	// A text that does not end in a question but has one earlier is told.
	assert.Contains(t, composeReply("Do you mean the goulash? It has beef.", mk().final()), "Left out")
	// A food lookup that found nothing: told.
	lookup := mk()
	lookup.noteTool("find_foods", json.RawMessage(`{"items":[],"total":0}`), nil)
	assert.Contains(t, composeReply("I found no such dish.", lookup.final()), "Left out")
	// A lookup that found something, find_ways, get_plan, a check, any error: not told.
	for name, out := range map[string]string{
		"find_foods":        `{"items":[{"id":1}],"total":1}`,
		"find_ways":         `{"offers":[{"x":1}]}`,
		"get_plan":          `{"days":[]}`,
		"find_alternatives": `{"items":[]}`,
		"apply_changes":     `{"dry_run":true,"saved":false}`,
	} {
		turn := mk()
		turn.noteTool(name, json.RawMessage(out), nil)
		assert.Equal(t, "Offers are in prose.", composeReply("Offers are in prose.", turn.final()), name)
	}
	failed := mk()
	failed.noteTool("find_foods", nil, errors.New("boom"))
	assert.NotContains(t, composeReply("Sorry.", failed.final()), "Left out")
	// The block never turns the turn into a write.
	assert.False(t, mk().final().write)
	// DE.
	deUser := "kann ich am Montag zum Abendessen stattdessen Beef goulash haben"
	de := replyTurn{user: deUser, unmet: unmetChange(deUser, mon, slots, st)}
	got = composeReply("Das passt nicht zu deinem Plan.", de.final())
	assert.Contains(t, got, nothingChangedDE)
	assert.Contains(t, got, "Weggelassen: die Änderung an")
	assert.Equal(t, "Welches möchtest du?", composeReply("Welches möchtest du?", de.final()))
}

func TestUnmetChange_QuestionsAreNoRequest(t *testing.T) {
	t.Parallel()
	slots, st, mon := unmetFixture()
	for _, text := range []string{
		"Why did you put pasta on Monday dinner?",
		"warum hast du Pasta am Montag zum Abendessen eingeplant?",
		"Wieso steht Nudeln am Montag zum Abendessen?",
		"is the Beef goulash on Monday dinner?",
		"ist am Montag zum Abendessen Beef goulash?",
		"Who put the pork on Monday dinner?",
		"Where did you put pasta on Monday dinner",
		"Wer hat Beef goulash am Montag zum Abendessen eingeplant?",
		"Does Monday dinner have the pork added?",
		"the pork on Monday dinner instead?",
	} {
		assert.Nil(t, unmetChange(text, mon, slots, st), text)
	}
	for _, text := range []string{
		"can i get the Beef goulash on Monday dinner instead?",
		"put something lighter on Monday dinner",
		"Put the pork on Monday dinner",
		"bitte setz Beef goulash auf Montag Abendessen",
		"kann ich am Montag zum Abendessen stattdessen Beef goulash haben?",
	} {
		assert.NotNil(t, unmetChange(text, mon, slots, st), text)
	}
}

// H2: the diet reason blames the food being added, never the one taken out;
// the user's own word is shown.
func TestUnmetChange_DietReasonBlamesTheAddedFood(t *testing.T) {
	t.Parallel()
	slots, st, mon := unmetFixture()
	mon = map[string]bool{"2026-11-23|3": true}
	reason := func(text string) string {
		b := unmetChange(text, mon, slots, st)
		require.NotNil(t, b, text)
		return b.facts[0].EN
	}
	neutral := "Left out: the change to Mon 23 Nov dinner (nothing was checked and saved for it)."
	assert.Equal(t, neutral, reason("replace the chicken on Monday dinner with tofu"))
	assert.Equal(t, neutral, reason("swap the Chicken on Monday dinner for tofu"))
	assert.Equal(t, neutral, reason("put tofu on Monday dinner instead of the chicken"))
	assert.Equal(t, neutral, reason("remove the Pork roast from Monday dinner and put the Tofu bowl there"))
	assert.Equal(t, `Left out: the change to Mon 23 Nov dinner ("Pork" does not fit your vegetarian diet).`, reason("replace the Tofu bowl on Monday dinner with Pork"))
	assert.Equal(t, `Left out: the change to Mon 23 Nov dinner ("Pork roast" does not fit your vegetarian diet).`, reason("put the Pork roast on Monday dinner instead of the Tofu bowl"))
	assert.Equal(t, `Left out: the change to Mon 23 Nov dinner ("Chicken" does not fit your vegetarian diet).`, reason("put Chicken on Monday dinner"))
	assert.Equal(t, `Left out: the change to Mon 23 Nov dinner ("BEEF" does not fit your vegetarian diet).`, reason("add BEEF to Monday dinner"))
	// DE: the user's word, not the folded token.
	de := unmetChange("ersetze das Hähnchen am Montag zum Abendessen durch Tofu", mon, slots, st)
	require.NotNil(t, de)
	assert.Contains(t, de.facts[0].DE, "dafür wurde nichts geprüft")
	de = unmetChange("setz am Montag zum Abendessen stattdessen Hähnchen", mon, slots, st)
	require.NotNil(t, de)
	assert.Contains(t, de.facts[0].DE, "„Hähnchen“ passt nicht zu deiner vegetarischen Ernährung")
	de = unmetChange("ersetze am Montag zum Abendessen das Tofu bowl durch Schweinefleisch und Lamm", mon, slots, st)
	require.NotNil(t, de)
	assert.Contains(t, de.facts[0].DE, "„Lamm“")
	// Deterministic: the first added candidate by position, on every run.
	for range 20 {
		assert.Equal(t, `Left out: the change to Mon 23 Nov dinner ("Pork roast" does not fit your vegetarian diet).`, reason("put the Pork roast and beef on Monday dinner"))
	}
}

// M3: only "not saved" claims are dropped; a true mention of confirming the
// drafts stays. A turn that ran a check, or left a part out, keeps its text.
func TestContradicts_UnsavedClaimIsNarrow(t *testing.T) {
	t.Parallel()
	saved := []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Sat 21 Nov dinner: A replaced by B.", DE: "x"}}
	for _, text := range []string{
		"The changes are not saved.", "They are not yet saved.", "Das ist noch nicht gespeichert.",
		"Die Änderungen sind noch nicht gespeichert, bis du sie bestätigst.", "It is stored only until you confirm.", "Alles bleibt offen, bis du bestätigst.",
	} {
		assert.True(t, contradicts(text, saved, false, false), text)
		assert.False(t, contradicts(text, saved, false, true), "a checked turn: "+text)
	}
	for _, text := range []string{
		"The plan is saved as drafts; you can confirm them on the board.", "Once you are happy, confirm the drafts on the board.",
		"Du kannst die Entwürfe auf dem Board bestätigen.", "Not all of it is the lentil stew, but it is saved.", "Gespeichert, nicht alles ist neu.",
	} {
		assert.False(t, contradicts(text, saved, false, false), text)
	}
	assert.Contains(t, regenerateSaved, "drafts")
	assert.NotContains(t, regenerateSaved, "nothing waits")
	assert.Contains(t, regenerateSavedPartial(3), "part of the new plan, 3 plate(s)")
	assert.Contains(t, regenerateSavedPartial(3), "the rest was not planned")
}

// H3: "above its minimum" is stripped only when no fact says the day is under
// its minimum.
func TestContradicts_AboveMinimumAgainstAMustFloorFact(t *testing.T) {
	t.Parallel()
	floor := []fact{{Class: classWorse, Kind: kindDay, Status: statusUnder, Must: true, EN: "No checked change of a planned meal brings Thu 10 Dec up to its minimum.", DE: "Keine geprüfte Änderung einer geplanten Mahlzeit bringt Donnerstag, 10. Dez. auf seinen Mindestwert."}}
	assert.True(t, contradicts("Thursday dinner is changed, so the day stays above its minimum.", floor, false, false))
	assert.True(t, contradicts("Der Tag liegt über dem Mindestwert.", floor, false, false))
	assert.False(t, contradicts("Thursday dinner is changed.", floor, false, false))
	// No such fact: the phrase is no claim about the range.
	range1 := []fact{{Class: classWarn, Kind: kindDay, Status: statusUnder, EN: "Thu 10 Dec holds 1518 kcal, below a good amount for the day (1530-1785 kcal).", DE: "x"}}
	assert.False(t, contradicts("The day stays above its minimum.", range1, false, false))
}
