package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// readTurn is a read-only turn whose plan state holds the given figures and
// dates (the days the code wrote in the prompt or a tool result).
func readTurn(user string, figures []string, dates ...dateKey) replyTurn {
	t := replyTurn{year: 2026, user: user, numbers: map[string]bool{}, state: map[string]bool{}, dates: map[dateKey]bool{}}
	for _, f := range figures {
		t.state[f] = true
	}
	for _, d := range dates {
		t.dates[d] = true
	}
	return t
}

func nov(d int) dateKey { return dateKey{month: time.November, day: d} }

// audit V58 (M-LINTDATE): the day and month of a date the code itself wrote
// are no figures. Stored drops: test1 h-readonly-00 (the rewriter's own
// "Sun 22 Nov"), g-question-07 and h-readonly-01 ("30.11."), dev3 g-question-08.
func TestLint_DatesTheCodeWroteAreNoFigures(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, user, model, want string
		figures                 []string
		date                    dateKey
	}{
		{
			"ISO rewritten to Sun 22 Nov", "kcal on Sunday?", "Sunday 2026-11-22 comes to 2137 kcal per person, inside the day's range of 1980 to 2420.",
			"Sunday Sun 22 Nov comes to 2137 kcal per person, inside the day's range of 1980 to 2420.",
			[]string{"2137", "1980", "2420"},
			nov(22),
		},
		{
			"German numeric date in parentheses", "ist Montag im Bereich?", "Ja, Montag (30.11.) liegt im Plan.",
			"Ja, Montag (30.11.) liegt im Plan.", nil, nov(30),
		},
		{
			"weekday and short date", "how many calories do i have on Wednesday in total", "On Wednesday, Wed 25 Nov, you have 2198 kcal, which is within your day range of 1980 to 2420.",
			"On Wednesday, Wed 25 Nov, you have 2198 kcal, which is within your day range of 1980 to 2420.",
			[]string{"2198", "1980", "2420"},
			nov(25),
		},
		{
			"German long date", "kcal am Mittwoch?", "Am Mittwoch, dem 25. November, sind es 1707 kcal.",
			"Am Mittwoch, dem 25. November, sind es 1707 kcal.",
			[]string{"1707"},
			nov(25),
		},
		{
			"English month first", "kcal on Wednesday?", "Wednesday, November 25 comes to 1707 kcal.",
			"Wednesday, November 25 comes to 1707 kcal.",
			[]string{"1707"},
			nov(25),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, composeReply(tc.model, readTurn(tc.user, tc.figures, tc.date)))
		})
	}
}

// A date no code wrote is still a number the lint checks: a wrong day stays out.
func TestLint_ADateNoCodeWroteIsStillChecked(t *testing.T) {
	t.Parallel()
	turn := readTurn("kcal on Saturday?", []string{"2137"}, nov(28))
	got := composeReply("Saturday 29 November comes to 2137 kcal. That is fine.", turn)
	assert.Equal(t, "That is fine.", got)
	assert.Equal(t, "Am 3. Dezember sind es 2137 kcal.", composeReply("Am 3. Dezember sind es 2137 kcal.", readTurn("kcal?", []string{"2137"}, dateKey{time.December, 3})))
	assert.Equal(t, fallbackDE, composeReply("Am 3.12. sind es 2137 kcal.", readTurn("kcal am Donnerstag?", []string{"2137"}, nov(3))), "3.12. is December, not 3 Nov")
}

// audit V58 (A4): the one disambiguating question names the candidate's date;
// it is not dropped for the date. Stored drops: dev3 g-ask-00, g-ask-03, g-ask-06,
// test1 g-ask-12.
func TestLint_TheAskQuestionKeepsItsDate(t *testing.T) {
	t.Parallel()
	for _, model := range []string{
		"Do you mean Saturday 28 November dinner (the tuna pasta with tomatoes)?",
		"Do you mean Saturday Sat 28 Nov dinner, the lasagne?",
		"Ich würde das Abendessen am Samstag, 28.11., ändern.",
		"Ich würde mit dem nächsten Abendessen anfangen, also Samstag, 28.11.",
	} {
		assert.Equal(t, model, composeReply(model, readTurn("change the dinner, i'm bored of it", nil, nov(28))))
	}
}

// audit V58 (M-ROUND): a figure that rounds a backed figure to fewer decimals
// is backed; a coarser rounding ("about 2200" for 2196) is not. Stored drops:
// dev3 g-question-01 (67.9 as "about 68 g"), h-readonly-06 (78.08, 46.6),
// test1 g-question-05 (25.8 as "rund 26 %").
func TestLint_RoundedBackedFiguresAreBacked(t *testing.T) {
	t.Parallel()
	en := readTurn("protein on Wednesday?", []string{"67.9", "20.8"}, dateKey{time.December, 2})
	assert.Equal(t, "Wednesday 2 December comes to about 68 g of protein, which is 20.8% of its energy.",
		composeReply("Wednesday 2 December comes to about 68 g of protein, which is 20.8% of its energy.", en))
	sat := readTurn("protein on Saturday?", []string{"78.08", "21.4", "46.6"})
	assert.Equal(t, "Saturday comes to about 78 g of protein, roughly 21% of the day's energy, well above the day's protein target of about 47 g.",
		composeReply("Saturday comes to about 78 g of protein, roughly 21% of the day's energy, well above the day's protein target of about 47 g.", sat))
	de := readTurn("Wie viel Eiweiß bekomme ich am Freitag, bitte?", []string{"80.56", "25.8"})
	assert.Equal(t, "Am Freitag kommst du auf etwa 80,6 g Eiweiß, also rund 26 % deiner Kalorien.",
		composeReply("Am Freitag kommst du auf etwa 80,6 g Eiweiß, also rund 26 % deiner Kalorien.", de))

	coarse := readTurn("kcal on Thursday?", []string{"2196"})
	assert.Equal(t, fallbackEN, composeReply("Thursday comes to about 2200 kcal.", coarse), "2200 rounds 2196 to hundreds: not backed")
	finer := readTurn("protein?", []string{"78"})
	assert.Equal(t, fallbackEN, composeReply("It comes to 78.1 g of protein.", finer), "more decimals than the backed figure: not backed")
}

// On a write turn the same holds for the model's text next to the block.
func TestLint_WriteTurnDatesAndRoundingFollowTheSameRule(t *testing.T) {
	t.Parallel()
	block := &replyBlock{facts: []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "x", DE: "Samstag, 21. Nov. Abendessen: A ersetzt durch B (812 → 640 kcal)."}}}
	turn := replyTurn{user: "Samstag Abendessen tauschen", write: true, block: block, numbers: map[string]bool{"639.6": true}, state: map[string]bool{}, dates: map[dateKey]bool{nov(21): true}}
	got := composeReply("Das Abendessen am 21.11. hat jetzt rund 640 kcal.", turn)
	assert.Contains(t, got, "Das Abendessen am 21.11. hat jetzt rund 640 kcal.")
}

// A German numeric date in a sentence ends no sentence (s-cascade-03:
// "nach dem 21.11. ist, kann ich ...").
func TestSentences_GermanNumericDateIsNoSentenceEnd(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"Nach dem 21.11. ist alles frei.", "Fertig."}, sentences("Nach dem 21.11. ist alles frei. Fertig."))
	assert.Equal(t, []string{"Das ist am 21.11.", "Fertig."}, sentences("Das ist am 21.11. Fertig."), "before an uppercase word it ends")
}

// PR #176 review (G4 lint, RP4): a number after a date is no year; it is a
// quantity the lint checks.
func TestLint_ANumberAfterADateIsNotItsYear(t *testing.T) {
	t.Parallel()
	got := composeReply("Sunday 22 Nov 2196 kcal is fine. That is it.", readTurn("kcal on Sunday?", []string{"2137"}, nov(22)))
	assert.Equal(t, "That is it.", got)
	got = composeReply("On Nov 22, 2196 kcal is the total. That is it.", readTurn("kcal on Sunday?", []string{"2137"}, nov(22)))
	assert.Equal(t, "That is it.", got)
}

// PR #176 review: a decimal is no date.
func TestLint_DecimalsAreNoDates(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"The day has 12.5 g.", "3 meals remain."}, sentences("The day has 12.5 g. 3 meals remain."))
	assert.Equal(t, []string{"Ich habe 12.5.", "3 Mahlzeiten gefunden."}, sentences("Ich habe 12.5. 3 Mahlzeiten gefunden."))
	assert.Equal(t, []string{"Es sind 40.13.", "Fertig."}, sentences("Es sind 40.13. Fertig."))
	// 12.5. with May 12 known and no backing figure is a quantity: it is checked.
	got := composeReply("Das sind 12.5. Das ist gut.", readTurn("Wie viel Eiweiß?", nil, dateKey{time.May, 12}))
	assert.Equal(t, "Das ist gut.", got)
	assert.Equal(t, "Nach dem 21.11. ist alles frei.", composeReply("Nach dem 21.11. ist alles frei.", readTurn("frei?", nil, nov(21))))
}

// PR #176 review: the weekday of a known date is still checked.
func TestLint_WeekdayOfAKnownDateIsChecked(t *testing.T) {
	t.Parallel()
	got := composeReply("Saturday 29 November comes to 2137 kcal. That is fine.", readTurn("kcal on Sunday?", []string{"2137"}, nov(29)))
	assert.Equal(t, "That is fine.", got)
}
