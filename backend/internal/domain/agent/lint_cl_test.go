package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func clTurn() replyTurn {
	block := &replyBlock{facts: []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Dinner changed.", DE: "Abendessen geändert."}}}
	return replyTurn{user: "make it lighter", write: true, block: block}
}

// CL3/G4: on a write turn the model's text makes no cooking or leftover claim;
// the block states them. The sentences are the real offending ones of the loop.
func TestLint_CL3NoCookingOrLeftoverClaim(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"You cook once that day: the couscous and the bean soup at dinner.",
		"Am Sonntag wird weiterhin nur einmal gekocht, und zwar abends.",
		"Sunday still cooks one dish at dinner, and the gratin is no longer cooked.",
		"Sunday still has one cooking time, at dinner, with the soup cooked together.",
		"Wednesday still has one cooking time, the beef ragu and tomato salad at dinner.",
		"The potatoes are still cooked on Sunday dinner.",
		"There are leftovers for Monday.",
		"Es bleiben Reste für Montag.",
		"Two portions are left.",
		"Die Kochzeit bleibt gleich.",
	} {
		assert.Equal(t, "cooking_claim", lintModelText(text, clTurn(), langEN), text)
	}
}

// CL4/G3: the offer is code-rendered; the model's text proposes or conditions no option.
func TestLint_CL4NoInventedOption(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"Ich kann die Umstellung nur machen, wenn du die Vielfalt-Regel lockerst.",
		"I can only do that if you loosen the variety rule.",
		"You could move it to another day.",
		"Want me to swap it?",
		"Falls du magst, tausche ich es.",
		"Möchtest du das Abendessen verschieben?",
		"Ich kann es stattdessen verschieben.",
	} {
		assert.Equal(t, "option", lintModelText(text, clTurn(), langEN), text)
	}
}

func TestLint_KnownGoodSentencesStillPass(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"Done, the dinner is lighter now.",
		"Erledigt, das Abendessen ist jetzt leichter.",
		"Done, I moved the dinner as you asked.",
	} {
		assert.Equal(t, "", lintModelText(text, clTurn(), langEN), text)
	}
}
