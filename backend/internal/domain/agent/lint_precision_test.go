package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// audit V58 A5: honest sentences the option and cooking words dropped. Each
// is a stored dev3/test1 drop; the claims they guard stay caught.
func TestLint_OptionWordsDoNotMatchRefusalsOrExplanations(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"I can't plan Monday at 750 kcal: that is below the day's minimum of 1095 kcal, so I haven't changed anything.", // dev3 g-daynumber-09
		"I can't take this dinner any lower without another change.",                                                    // dev3 g-lighter-09
		"I cannot make it lighter.",
		"Die 2100 kcal habe ich nicht ganz erreicht, weil der Tag sonst unter sein Minimum gerutscht wäre.",            // dev3 g-daynumber-15
		"Mehr ging an diesem Abend nicht, weil der Tag sonst über seine Obergrenze gekommen wäre.",                     // test1 g-heartier-13
		"It's not yet a light meal, because the limit would take the day under its minimum unless another meal grows.", // test1 g-lighter-01
		"I've lightened Monday: the dinner tofu is now 100 g instead of 200 g.",                                        // dev3 g-daynumber-10
		"Done, Monday dinner is now the pasta with pesto and peas instead of the lemon salmon.",                        // test1 g-replace-03
	} {
		assert.False(t, modelOptionWords.MatchString(withoutComparisons(s)), s)
	}
	for _, s := range []string{
		"I can make that change if you want it.",
		"Monday is now under its energy range, and I can suggest a fix for that.",
		"You can add a side of bread.",
		"Unless you want it lighter, it stays.",
		"I would take the soup instead.",
		"Möchtest du das?",
		"Sonst kann ich dir eine Beilage vorschlagen.",
	} {
		assert.True(t, modelOptionWords.MatchString(withoutComparisons(s)), s)
	}
}

func TestLint_RestIsNoLeftoverClaim(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"The rest of the day stays as planned.",                                                     // dev3 g-ask-00
		"The rest of the day stays within its range.",                                               // test1 g-ask-02
		"Den Reis am Mittag habe ich gestrichen, den Rest des Tages habe ich nicht angefasst.",      // test1 g-daynumber-13
		"Die Kartoffeln sind vom Donnerstagabend gestrichen, der Rest des Plans bleibt wie er war.", // dev3 g-undo-14
	} {
		assert.False(t, modelCookingWords.MatchString(s), s)
	}
	for _, s := range []string{"Die Reste reichen bis Dienstag.", "The leftovers last until Tuesday.", "Das wird frisch gekocht."} {
		assert.True(t, modelCookingWords.MatchString(s), s)
	}
}

// The splitter keeps common abbreviations inside the sentence (test1
// h-readonly-01 "(ca." / "23 g mehr Eiweiß)."; dev3 g-replan-15 "z." / "B.").
func TestSentences_AbbreviationsAreNoSentenceEnd(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"Etwa Magerquark am Abendessen (ca. 23 g mehr Eiweiß).",
		"Eine Beilage, z. B. Brot, passt dazu.",
		"Eine Beilage, z.B. Brot, passt dazu.",
		"Brot bzw. Reis passt dazu.",
		"Das geht ggf. auch am Dienstag.",
		"Es gibt u. a. Linsen.",
	} {
		assert.Equal(t, []string{s}, sentences(s))
	}
	assert.Equal(t, []string{"Das sind ca. 2 Portionen.", "Fertig."}, sentences("Das sind ca. 2 Portionen. Fertig."))
	assert.Equal(t, []string{"Plan A.", "Fertig."}, sentences("Plan A. Fertig."), "a lone capital letter ends a sentence")
}

// End to end on a write turn: the refusal stays next to the block.
func TestReply_HonestRefusalStaysNextToTheBlock(t *testing.T) {
	t.Parallel()
	block := &replyBlock{facts: []fact{{Class: classRefused, Kind: kindRefused, Status: statusOK, EN: "Mon 30 Nov: 750 kcal is under the minimum of 1095 kcal.", DE: "x"}}, nothingSaved: true}
	turn := replyTurn{user: "plan Monday at 750 kcal", write: true, block: block, numbers: map[string]bool{}, state: map[string]bool{}}
	got := composeReply("I can't plan Monday at 750 kcal: that is below the day's minimum of 1095 kcal, so I haven't changed anything.", turn)
	assert.Contains(t, got, "I can't plan Monday at 750 kcal")
}

// PR #177 review (G4 lint, P-7): offers hiding behind "sonst", "kann ich" and a
// leading "instead of" are dropped; a reason and a quantity comparison pass.
func TestLint_ReviewOfferGaps(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"Der Tag liegt drunter, sonst kann ich dir Obst ergänzen.",
		"Das geht, sonst könnte ich dir Obst ergänzen.",
		"Das geht, sonst soll ich Obst ergänzen.",
		"Kann ich dir eine Beilage vorschlagen? Das kann ich tun.",
		"Instead of the pasta, try rice.",
		"Instead of the pasta I'd go for rice.",
		"Try the rice instead of the pasta.",
	} {
		assert.True(t, modelOptionWords.MatchString(withoutComparisons(s)), s)
	}
	for _, s := range []string{
		"Es blieb, weil der Tag sonst unter sein Minimum gerutscht wäre.",
		"The tofu is now 100 g instead of 200 g.",
		"Es sind jetzt 3 Portionen statt 4 Portionen.",
	} {
		assert.False(t, modelOptionWords.MatchString(withoutComparisons(s)), s)
	}
}

func TestSentences_LoneZAndUSplit(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"Plan z.", "Danach geht es weiter."}, sentences("Plan z. Danach geht es weiter."))
	assert.Equal(t, []string{"Plan u.", "Danach."}, sentences("Plan u. Danach."))
	s := "Eine Beilage, z. B. Brot, und u. a. Linsen."
	assert.Equal(t, []string{s}, sentences(s))
}

// PR #177 review: the clash text names the side dish, which the lint drops from
// the model's sentence; the program tells it, so next never asks the model to.
func TestClashNext_ProgramTellsTheClash(t *testing.T) {
	t.Parallel()
	offer := map[string]any{"clash": "Rice and Pasta are two starches on one plate", "what": "drop the rice"}
	saved := clashNext("", offer, false)
	assert.NotContains(t, saved, "Rice and Pasta")
	assert.Contains(t, saved, "the program tells")
	dry := clashNext("", offer, true)
	assert.NotContains(t, dry, "Rice and Pasta")
	assert.Contains(t, dry, "without naming the dish")
}
