package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Round 12b (audit 2026-11-24, group D): true sentences the lint dropped.
// Each case has a false twin that the loosening must not let through.

// D1 (new cook_elsewhere-23, new2 cook_elsewhere-13): the refused dishes were
// read from the EN and the DE line, so each counted twice and a loosely named
// dish never passed uniqueDishFor.
func TestLint_R12b_RefusalDishCountedOnce(t *testing.T) {
	t.Parallel()
	line := refusedLine(
		`Left out: cooking "Easy chicken curry" on Tue 1 Dec dinner (Wed 2 Dec lunch and Wed 2 Dec dinner eat it before then).`,
		`Weggelassen: „Easy chicken curry“ am Dienstag, 1. Dez. Abendessen kochen (Mittwoch, 2. Dez. Mittagessen und Mittwoch, 2. Dez. Abendessen essen schon vorher davon).`)
	foods := []string{"Easy chicken curry", "Thai green curry"}
	turn := replyTurn{
		user: "Kannst du das Curry am Dienstag zum Abendessen kochen?", write: true, foods: foods,
		block: &replyBlock{nothingSaved: true, facts: []fact{line}},
	}
	assert.Empty(t, lintModelText("Das Curry kann ich nicht am Dienstag kochen, weil es am Mittwoch schon gegessen wird.", turn, langDE))

	// Twin: two different dishes in the refused line that both fit the loose
	// name stay ambiguous.
	two := refusedLine(
		`Left out: replacing "Easy chicken curry" with "Thai green curry" at Tue 1 Dec dinner (Wed 2 Dec lunch and Wed 2 Dec dinner eat it before then).`,
		`Weggelassen: „Easy chicken curry“ bei Dienstag, 1. Dez. Abendessen durch „Thai green curry“ ersetzen (Mittwoch, 2. Dez. Mittagessen und Mittwoch, 2. Dez. Abendessen essen schon vorher davon).`)
	turn.block = &replyBlock{nothingSaved: true, facts: []fact{two}}
	assert.Equal(t, "cooking_claim", lintModelText("Das Curry kann ich nicht am Dienstag kochen, weil es am Mittwoch schon gegessen wird.", turn, langDE))
}

func r12bSaved(foods ...string) replyTurn {
	return replyTurn{
		user: "Füge Vollkornbrot zum Sonntag Mittagessen hinzu", write: true, numbers: map[string]bool{}, foods: foods,
		block: &replyBlock{
			facts: []fact{{
				Class: classChanged, Kind: kindMeal, Status: statusOK,
				EN: `Sun 6 Dec lunch: added "Vollkornbrot".`, DE: `Sonntag, 6. Dez. Mittagessen: hinzugefügt: „Vollkornbrot“.`,
			}},
			kept: map[string][]string{"sun-lunch": {"Steamed rice"}},
		},
	}
}

// D2 (new2 add-06): two sentences that each pass alone failed together on
// "dish", because a dish action in one sentence made the kept dish of the
// other count as unnamed. Both sentences were dropped.
func TestLint_R12b_JointCheckJudgesDishPerSentence(t *testing.T) {
	t.Parallel()
	turn := r12bSaved("Steamed rice", "Vollkornbrot")
	s1 := "Das Vollkornbrot wurde am Sonntagmittag hinzugefügt."
	s2 := "Der Steamed rice bleibt auf dem Teller."
	assert.Empty(t, lintModelText(s1, turn, langDE))
	assert.Empty(t, lintModelText(s2, turn, langDE))
	got := composeReply(s1+" "+s2, turn)
	assert.Contains(t, got, s1+" "+s2)

	// Twin: a kept dish in a sentence of its own that adds, removes or moves a
	// dish is still dropped, and the true sentence still stays.
	bad := "Der Steamed rice wurde entfernt."
	got = composeReply(s1+" "+bad, turn)
	assert.Contains(t, got, s1)
	assert.NotContains(t, got, bad)

	// Twin: a dish the block never names, in a sentence of its own, is dropped.
	turn.foods = append(turn.foods, "Hummus")
	got = composeReply(s1+" Der Hummus passt gut dazu.", turn)
	assert.NotContains(t, got, "Hummus passt")
}

// D3 (new2 away-13): "nothing is cooked on Thursday" is true when the block
// says so; the cookDenial check dropped any denial.
func TestLint_R12b_BackedNegatedCookingSentence(t *testing.T) {
	t.Parallel()
	away := fact{
		Class: classMoved, Kind: kindAway, Status: statusOK,
		EN: "Thu 10 Dec: dinner out (about 1243 kcal assumed), nothing is cooked.",
		DE: "Donnerstag, 10. Dez.: Abendessen außer Haus (etwa 1243 kcal angenommen), es wird nichts gekocht.",
	}
	friday := fact{
		Class: classMoved, Kind: kindCook, Status: statusOK,
		EN: `Fri 11 Dec at lunch you cook "Tofu stir-fry" for Fri 11 Dec lunch; the longest recipe takes about 20 min.`,
		DE: `Freitag, 11. Dez. (Mittagessen) kochst du „Tofu stir-fry“ für Freitag, 11. Dez. Mittagessen; das längste Rezept braucht etwa 20 Min.`,
	}
	turn := replyTurn{
		user: "Donnerstag Abendessen auswärts", write: true, foods: []string{"Tofu stir-fry"},
		block: &replyBlock{facts: []fact{away, friday}},
	}
	assert.Empty(t, lintModelText("Am Donnerstag wird nichts gekocht.", turn, langDE))
	assert.Empty(t, lintModelText("Nothing is cooked on Thursday.", turn, langEN))
	// Twins: a day that does cook, a dish, a day the block never names, no
	// day, the past tense.
	assert.Equal(t, "cooking_claim", lintModelText("Am Freitag wird nichts gekocht.", turn, langDE))
	assert.Equal(t, "cooking_claim", lintModelText("Am Donnerstag und am Freitag wird nichts gekocht.", turn, langDE))
	assert.Equal(t, "cooking_claim", lintModelText("Am Samstag wird nichts gekocht.", turn, langDE))
	assert.Equal(t, "cooking_claim", lintModelText("Es wird nichts gekocht.", turn, langDE))
	assert.Equal(t, "cooking_claim", lintModelText("Der Tofu stir-fry wird am Donnerstag nicht gekocht.", turn, langDE))
	assert.Equal(t, "cooking_claim", lintModelText("Am Donnerstag wurde nichts gekocht.", turn, langDE))
}

// D4 (new2 away-13): "instead" inside a statement of what was done is no
// offer; an offer with "instead" still is.
func TestLint_R12b_InsteadInAStatementIsNoOffer(t *testing.T) {
	t.Parallel()
	turn := replyTurn{
		user: "Donnerstag Abendessen auswärts", write: true, foods: []string{"Tofu stir-fry"},
		block: &replyBlock{facts: []fact{{
			Class: classMoved, Kind: kindCook, Status: statusOK,
			EN: `Fri 11 Dec at lunch you cook "Tofu stir-fry" for Fri 11 Dec lunch; the longest recipe takes about 20 min.`,
			DE: `Freitag, 11. Dez. (Mittagessen) kochst du „Tofu stir-fry“ für Freitag, 11. Dez. Mittagessen; das längste Rezept braucht etwa 20 Min.`,
		}}},
	}
	assert.Empty(t, lintModelText("Der Tofu stir-fry wird stattdessen für Freitagmittag gekocht.", turn, langDE))
	assert.Empty(t, lintModelText("The Tofu stir-fry is cooked for Friday lunch instead.", turn, langEN))
	for _, s := range []string{
		"Ich kann stattdessen den Tofu stir-fry für Freitagmittag kochen.",
		"Soll ich den Tofu stir-fry stattdessen für Freitagmittag kochen?",
		"I can cook the Tofu stir-fry for Friday lunch instead.",
		"Wenn du willst, wird der Tofu stir-fry stattdessen für Freitagmittag gekocht.",
		"Stattdessen kochen wir den Tofu stir-fry am Freitagmittag.",
	} {
		assert.Equal(t, "option", lintModelText(s, turn, langDE), s)
	}
}

// D5 (new2 g2-question-07): the year of the turn after a known German date is
// part of the date; any other year, or a figure with a unit, stays a number.
func TestLint_R12b_YearAfterAKnownDate(t *testing.T) {
	t.Parallel()
	turn := readTurn("ist Sonntag im Bereich?", nil, nov(29))
	assert.Empty(t, lintModelText("Ja, Sonntag, der 29. November 2026, liegt im Planungszeitraum.", turn, langDE))
	assert.Equal(t, "number", lintModelText("Ja, Sonntag, der 29. November 2027, liegt im Planungszeitraum.", turn, langDE))
	assert.Equal(t, "number", lintModelText("Ja, Sonntag, der 29. November 2026 kcal, liegt im Planungszeitraum.", turn, langDE))
	assert.Equal(t, "number", lintModelText("Ja, Sonntag, der 29. November hat 2026 Kalorien.", turn, langDE))
}
