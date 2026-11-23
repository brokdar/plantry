package agent

import (
	"slices"
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
		"can i get the Beef goulash on Sunday dinner instead": `Left out: the change to Sun 22 Nov dinner ("beef" does not fit your vegetarian diet).`,
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
	assert.Equal(t, `Weggelassen: die Änderung an Sonntag, 22. Nov. Abendessen („beef“ passt nicht zu deiner vegetarischen Ernährung).`, de.facts[0].DE)

	assert.Nil(t, unmetChange("what is on Sunday dinner?", sun, slots, st), "a question changes nothing")
	assert.Nil(t, unmetChange("can i get the Beef goulash instead", nil, slots, st), "no named meal")

	// A turn that wrote nothing tells it after the model's text; a turn with a
	// write block keeps its own.
	turn := replyTurn{user: "can i get the Beef goulash on Sunday dinner instead", unmet: unmetChange("can i get the Beef goulash on Sunday dinner instead", sun, slots, st)}
	got := composeReply("Beef goulash doesn't fit your vegetarian plan.", turn.final())
	assert.Contains(t, got, nothingChangedEN)
	assert.Contains(t, got, `Left out: the change to Sun 22 Nov dinner ("beef" does not fit your vegetarian diet).`)
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
		"Once you confirm, the new week is in place.",
	} {
		assert.True(t, contradicts(text, saved, false), text)
	}
	assert.False(t, contradicts("The week is planned again.", saved, false))
	refused := append(slices.Clone(saved), fact{Class: classRefused, Kind: kindRefused, Status: statusOK, EN: "Left out: x.", DE: "x"})
	assert.False(t, contradicts("The swap on Monday was not saved.", refused, false), "a part left out may be unsaved")
	assert.Contains(t, regenerateSaved, "nothing waits for a confirmation")
}
