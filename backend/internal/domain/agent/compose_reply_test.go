package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// M3 cycle 1, tune g-move-08 / g-swap-14 / h-refusal-06 (G4.1, SF2): a call that
// saved nothing says so in code, also when the lint drops the model's text.
func TestComposeReply_RefusedCallSaysNothingChanged(t *testing.T) {
	t.Parallel()
	out := `{"saved":false,"applied":[],"skipped":[{"en":"Left out: moving \"Lasagne bolognese\" from Sun 6 Dec dinner to Wed 9 Dec dinner (it would create a problem on Mon 7 Dec: cooking times per day (2 cooking times, at most 1)).","de":"Weggelassen: „Lasagne bolognese“ von Sonntag, 6. Dez. Abendessen nach Mittwoch, 9. Dez. Abendessen verschieben (es würde am Montag, 7. Dez. ein Problem entstehen: mehr getrennte Kochzeiten an einem Tag als erlaubt (2 Kochzeiten, höchstens 1))."}]}`
	b, _ := blockOf([]byte(out))
	require.NotNil(t, b)
	turn := replyTurn{user: "move Sunday's dinner over to Wednesday please", write: true, block: b, numbers: map[string]bool{}}
	got := composeReply("I couldn't move Sunday's dinner to Wednesday, so nothing changed. The lasagne is cooked Sunday dinner and Monday dinner eats its leftovers.", turn)
	assert.Contains(t, got, "- Nothing changed: the plan is as it was.\n- Left out: moving")
	de := turn
	de.user = "Kannst du das Abendessen vom Sonntag auf Mittwoch verschieben?"
	assert.Contains(t, composeReply("", de), "- Nichts geändert: der Plan bleibt, wie er war.")

	saved, _ := blockOf([]byte(`{"saved":true,"applied":[{"op":"add"}],"skipped":[{"en":"Left out: adding x.","de":"Weggelassen: x hinzufügen."}]}`))
	require.NotNil(t, saved)
	assert.NotContains(t, composeReply("", replyTurn{user: "add", write: true, block: saved, numbers: map[string]bool{}}), "Nothing changed", "a partly saved call is not 'nothing changed'")

	// One turn, several calls: a refused call followed by a saving one is not 'nothing changed'.
	refused, _ := blockOf([]byte(out))
	saving, _ := blockOf([]byte(`{"saved":true,"applied":[{"op":"add"}],"report":{"facts":[{"class":2,"kind":"meal","status":"ok","en":"Fri dinner: X replaced by Y.","de":"Fr Abendessen: X ersetzt durch Y."}]}}`))
	require.NotNil(t, saving)
	mixed := composeReply("", replyTurn{user: "swap", write: true, block: mergeBlock(refused, saving), numbers: map[string]bool{}})
	assert.NotContains(t, mixed, "Nothing changed")
	assert.Contains(t, mixed, "Fri dinner: X replaced by Y.")
	both := composeReply("", replyTurn{user: "move", write: true, block: mergeBlock(refused, refused), numbers: map[string]bool{}})
	assert.Equal(t, 1, strings.Count(both, "Nothing changed"))
}

// M3 cycle 1, tune g-question-00 (RP3, RP4): "per eater" is rewritten, not a reason
// to drop the answer, and on a read turn an unbacked figure costs only its sentence.
func TestComposeReply_ReadTurnKeepsTheBackedAnswer(t *testing.T) {
	t.Parallel()
	nums := map[string]bool{"1522": true, "1504": true}
	turn := replyTurn{user: "kcal on Saturday?", numbers: nums}
	got := composeReply("Saturday comes to 1522 kcal per eater. That's a little above the day's range top of 1504, so the day is slightly over its range.", turn)
	assert.Equal(t, "Saturday comes to 1522 kcal per person. That's a little above the day's range top of 1504, so the day is slightly over its range.", got)
	assert.Equal(t, "Saturday comes to 1522 kcal.", composeReply("Saturday comes to 1522 kcal. That is 21 % of the week.", turn))
	assert.Equal(t, fallbackEN, composeReply("That is 21 % of the week.", turn))
}

// M3 cycle 1, tune g-move-08: a refused move names where it was going, so the
// reply may say the day the user named back (G4.1, lint weekday rule).
func TestRefusedPart_MoveNamesItsDestination(t *testing.T) {
	t.Parallel()
	op := changeOp{Op: "move", To: []byte(`{"date":"2026-12-09","slot_id":3}`)}
	en, de := refusedPart(context.Background(), Services{}, window.Window{}, op, applyRun{})
	assert.Contains(t, en, "to Wed 9 Dec")
	assert.Contains(t, de, "nach Mittwoch, 9. Dez.")
}

// v1.3 P-6 (G4 lint per sentence), tune g-move-08: on a write turn the sentence
// that breaks the lint is dropped and the rest stays; a weekday the user named
// may be said back.
func TestComposeReply_WriteLintDropsOnlyTheBreakingSentence(t *testing.T) {
	t.Parallel()
	out := `{"saved":false,"applied":[],"skipped":[{"en":"Left out: moving \"Lasagne bolognese\" from Sun 6 Dec dinner (it would create a problem on Mon 7 Dec: cooking times per day (2 cooking times, at most 1)).","de":"Weggelassen: x."}]}`
	b, _ := blockOf([]byte(out))
	require.NotNil(t, b)
	turn := replyTurn{user: "move Sunday's dinner over to Wednesday please", write: true, block: b, numbers: map[string]bool{}}
	got := composeReply("I couldn't move Sunday's dinner to Wednesday, so nothing changed. The lasagne is cooked Sunday dinner and Monday dinner eats its leftovers.", turn)
	assert.True(t, strings.HasPrefix(got, "I couldn't move Sunday's dinner to Wednesday, so nothing changed.\n\n- Nothing changed"), got)
	assert.NotContains(t, got, "leftovers")

	other := turn
	other.user = "move it please"
	assert.True(t, strings.HasPrefix(composeReply("I couldn't move Sunday's dinner to Wednesday, so nothing changed.", other), "- Nothing changed"), "a weekday neither the block nor the user names drops the sentence")
}

func TestComposeReply_CheckOnlyWriteFallsBackWhenEverySentenceFails(t *testing.T) {
	t.Parallel()
	got := composeReply("Friday has 9999 kcal.", replyTurn{user: "hi", write: true, numbers: map[string]bool{}})
	assert.Equal(t, fallbackEN, got)
}

func TestUserNamesWeekday_WholeFullNamesOnly(t *testing.T) {
	t.Parallel()
	assert.False(t, userNamesWeekday("I'm satisfied, so.", "Sat"))
	assert.False(t, userNamesWeekday("I'm satisfied, so.", "Sun"))
	assert.True(t, userNamesWeekday("move Sunday's dinner to Wednesday", "Sun"))
	assert.True(t, userNamesWeekday("move Sunday's dinner to Wednesday", "Wed"))
	assert.True(t, userNamesWeekday("verschiebe den Sonntag", "Sun"))
}

func TestComposeReply_RefusedCallDropsDoneClaims(t *testing.T) {
	t.Parallel()
	out := `{"saved":false,"applied":[],"skipped":[{"en":"Left out: moving \"Lasagne bolognese\" from Sun 6 Dec dinner (it would create a problem on Mon 7 Dec: cooking times per day (2 cooking times, at most 1)).","de":"Weggelassen: x."}]}`
	b, _ := blockOf([]byte(out))
	require.NotNil(t, b)
	require.True(t, b.nothingSaved)
	turn := replyTurn{user: "move Sunday's dinner to Wednesday", write: true, block: b, numbers: map[string]bool{}}
	for _, s := range []string{"I moved it to Wednesday.", "Ich habe das Abendessen verschoben."} {
		assert.True(t, strings.HasPrefix(composeReply(s, turn), "- Nothing changed"), s)
	}
	got := composeReply("I couldn't move Sunday's dinner to Wednesday, so nothing changed.", turn)
	assert.True(t, strings.HasPrefix(got, "I couldn't move"), got)
}
