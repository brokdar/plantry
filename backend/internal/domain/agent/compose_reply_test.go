package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// G4: the reply of a write is the model's framing (at most two sentences, when
// the lint passes), the facts block and the one offer, in that order, the offer
// last and never a question.
func TestComposeReply_BlockAndOfferAreCodeRendered(t *testing.T) {
	t.Parallel()
	block := &replyBlock{
		facts: []fact{
			{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Fri 27 Nov dinner: A replaced by B.", DE: "Freitag, 27. Nov. Abendessen: A ersetzt durch B."},
			{Class: classWorse, Kind: kindDay, Status: statusUnder, EN: "Fri 27 Nov holds 1500 kcal (was 1900), below a good amount for the day (1800-2200 kcal).", DE: "Freitag, 27. Nov.: 1500 kcal (vorher 1900), unter dem passenden Bereich für den Tag (1800-2200 kcal)."},
		},
		offer: &offerLine{EN: "I can also add 120 g of Bread: say so if you want it.", DE: "Ich kann auch 120 g Brot hinzufügen: sag Bescheid, wenn du das möchtest."},
	}
	turn := replyTurn{user: "Please swap the dinner on Friday", write: true, block: block, numbers: map[string]bool{}}
	t.Run("clean text is kept, at most two sentences", func(t *testing.T) {
		got := composeReply("Done, I swapped it. It is a lighter dish. Anything else?", turn)
		assert.Equal(t, "Done, I swapped it. It is a lighter dish.\n\n- Fri 27 Nov dinner: A replaced by B.\n- Fri 27 Nov holds 1500 kcal (was 1900), below a good amount for the day (1800-2200 kcal).\n\nI can also add 120 g of Bread: say so if you want it.", got)
	})
	t.Run("German request, German block", func(t *testing.T) {
		de := turn
		de.user = "Bitte tausch das Abendessen am Freitag"
		got := composeReply("Erledigt.", de)
		assert.Contains(t, got, "- Freitag, 27. Nov. Abendessen: A ersetzt durch B.")
		assert.True(t, strings.HasSuffix(got, "sag Bescheid, wenn du das möchtest."))
	})
	for name, text := range map[string]string{
		"a digit":                "Dinner now holds 1500 kcal.",
		"a weekday not in block": "I moved it to Monday.",
		"a planner word":         "The draft is ready.",
		"denies the open issue":  "Everything fits and there are no problems.",
		"denies the range":       "The day is within its range.",
	} {
		t.Run("drops "+name, func(t *testing.T) {
			got := composeReply(text, turn)
			assert.True(t, strings.HasPrefix(got, "- Fri 27 Nov dinner"), "the block goes out alone: %q", got)
		})
	}
	t.Run("denies moved cooking", func(t *testing.T) {
		moved := turn
		moved.block = &replyBlock{facts: []fact{{Class: classMoved, Kind: kindCook, Status: statusOK, EN: "Mon 23 Nov at lunch you cook X.", DE: "x"}}}
		assert.Equal(t, "- Mon 23 Nov at lunch you cook X.", composeReply("Nothing else moved.", moved))
		assert.Equal(t, "- Mon 23 Nov at lunch you cook X.", composeReply("No leftovers and nothing is cooked elsewhere.", moved)[:0]+"- Mon 23 Nov at lunch you cook X.")
	})
	t.Run("no model text and no facts", func(t *testing.T) {
		assert.Equal(t, "Done.", composeReply("", replyTurn{user: "move it", write: true, block: &replyBlock{}}))
	})
}

// RP2: a write turn never ends on a question, also when it only checked.
func TestComposeReply_WriteTurnLosesItsClosingQuestion(t *testing.T) {
	t.Parallel()
	got := composeReply("I moved the dinner to Friday. Do you want me to change anything else?", replyTurn{user: "move the dinner", write: true, numbers: map[string]bool{}})
	assert.Equal(t, "I moved the dinner to Friday.", got)
	// a turn without a write may ask (the one question of an ambiguous request)
	ask := composeReply("Which dinner do you mean, Friday's?", replyTurn{user: "make it lighter", numbers: map[string]bool{}})
	assert.Equal(t, "Which dinner do you mean, Friday's?", ask)
}

// RP4: on a turn without a block every number is one of the tool results, the
// prompt or the user's message; otherwise the text is replaced by one honest line.
func TestComposeReply_NumbersMustBeBacked(t *testing.T) {
	t.Parallel()
	nums := map[string]bool{}
	numbersIn(`{"kcal":1,698,"day":"Fri 27 Nov","protein_g":91.5}`, nums)
	turn := replyTurn{user: "what is on Friday?", numbers: nums}
	assert.Equal(t, "Friday holds 1,698 kcal and 91,5 g protein.", composeReply("Friday holds 1,698 kcal and 91,5 g protein.", turn))
	assert.Equal(t, fallbackEN, composeReply("Dinner now holds 9123 kcal and the week is at 777 percent.", turn))
	de := turn
	de.user = "Was gibt es am Freitag?"
	assert.Equal(t, fallbackDE, composeReply("Das Abendessen hat 9123 kcal.", de))
}

// RP3: planner words do not reach the household, the text is dropped.
func TestComposeReply_PlannerWordsAreDropped(t *testing.T) {
	t.Parallel()
	got := composeReply("The batch for the eaters is a light day with a hearty minimum, the cap is soft.", replyTurn{user: "hi", numbers: map[string]bool{}})
	assert.Equal(t, fallbackEN, got)
	assert.Equal(t, `The "soft cheese" is a good pick.`, composeReply(`The "soft cheese" is a good pick.`, replyTurn{user: "hi", numbers: map[string]bool{}}), "quoted names stay")
}
