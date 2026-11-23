package agent

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// V77 lint (tune-v2b new, 7 of 8 all-dropped write turns): true cooking
// sentences the block backs were dropped as cooking_claim. A cooking claim now
// stays when the block's cooking lines back every day, meal and dish it names
// and every day it says something is cooked on; a false one is still dropped.
func TestLint_CookingClaimBackedByTheBlock(t *testing.T) {
	t.Parallel()
	moved := func(kind, en, de string) fact {
		return fact{Class: classMoved, Kind: kind, Status: statusOK, EN: en, DE: de}
	}
	changed := func(en, de string) fact {
		return fact{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: en, DE: de}
	}
	elsewhere10 := []fact{
		moved(kindCook, `Mon 30 Nov at dinner you cook "Red lentil soup" for Mon 30 Nov dinner, Tue 1 Dec lunch, Tue 1 Dec dinner; "Steamed broccoli" for Mon 30 Nov dinner, Tue 1 Dec lunch, Tue 1 Dec dinner; "Minestrone" for Wed 2 Dec lunch, Wed 2 Dec dinner, Thu 3 Dec lunch (cooked Wed 2 Dec lunch before); the longest recipe takes about 50 min.`, "x"),
		moved(kindCook, `Wed 2 Dec at lunch you cook "Cucumber yogurt salad" for Wed 2 Dec lunch, Wed 2 Dec dinner, Thu 3 Dec lunch; "Carrot salad" for Wed 2 Dec lunch; the longest recipe takes about 10 min.`, "x"),
	}
	elsewhere12 := []fact{
		moved(kindCook, `Sun 29 Nov at dinner you cook "Fruit salad" for Sun 29 Nov dinner; "Greek salad with chickpeas" for Mon 30 Nov lunch, Mon 30 Nov dinner (cooked Mon 30 Nov lunch before); the longest recipe takes about 10 min.`,
			`Sonntag, 29. Nov. (Abendessen) kochst du „Fruit salad“ für Sonntag, 29. Nov. Abendessen; „Greek salad with chickpeas“ für Montag, 30. Nov. Mittagessen, Montag, 30. Nov. Abendessen (vorher Montag, 30. Nov. Mittagessen gekocht); das längste Rezept braucht etwa 10 Min.`),
		moved(kindCook, `Mon 30 Nov at lunch you cook "Couscous" for Mon 30 Nov lunch, Mon 30 Nov dinner; the longest recipe takes about 7 min.`,
			`Montag, 30. Nov. (Mittagessen) kochst du „Couscous“ für Montag, 30. Nov. Mittagessen, Montag, 30. Nov. Abendessen; das längste Rezept braucht etwa 7 Min.`),
	}
	away16 := []fact{
		changed("Mon 7 Dec lunch: no meal any more.", "x"),
		changed("Mon 7 Dec dinner: no meal any more.", "x"),
		moved(kindLeftover, `Sun 6 Dec: cook 1 portion of the "Couscous" (was 3); cook 1 portion of the "Lasagne bolognese" (was 2).`, "x"),
		moved(kindCook, `Sun 6 Dec at dinner you cook "Lasagne bolognese" for Sun 6 Dec dinner; "Couscous" for Sun 6 Dec dinner; the longest recipe takes about 1 h 30 min.`, "x"),
		moved(kindCook, "Mon 7 Dec: nothing is cooked.", "x"),
	}
	guests02 := []fact{
		moved(kindLeftover, `Wed 9 Dec: cook 6 portions of the "Cucumber yogurt salad" (was 3); cook 6 portions of the "Spinach lasagne" (was 3).`, "x"),
		moved(kindGuest, `Wed 9 Dec dinner: now for 4 people, 4 portions of "Spinach lasagne".`, "x"),
	}
	guests09 := []fact{
		moved(kindLeftover, `Fri 11 Dec: cook 2 portions of the "Chicken and vegetable traybake" (was 1); cook 2 portions of the "Green salad" (was 1).`, "x"),
		moved(kindGuest, `Fri 11 Dec dinner: now for 2 people, 2 portions of "Chicken and vegetable traybake".`, "x"),
	}
	foods := []string{"Minestrone", "Red lentil soup", "Greek salad with chickpeas", "Couscous", "Lasagne bolognese", "Spinach lasagne", "Chicken and vegetable traybake", "Fruit salad", "Green salad", "Wholegrain bread"}
	for _, tc := range []struct {
		name  string
		facts []fact
		user  string
		lg    lang
		text  string
		keep  bool
	}{
		{"elsewhere-10 true", elsewhere10, "cook the Minestrone on Monday dinner", langEN, "The Minestrone is now cooked on Monday dinner and still covers Wednesday lunch and dinner and Thursday lunch.", true},
		{"elsewhere-12 DE true, dish named by its plan name", elsewhere12, "Greek salad with chickpeas am Sonntag zum Abendessen kochen", langDE, "Das „Greek salad with chickpeas“ wird jetzt am Sonntagabend gekocht.", true},
		{"away-16 true", away16, "away on Monday", langEN, "Monday lunch and dinner are cleared, and Sunday's dinner cooking is trimmed to cover only Sunday.", true},
		// A dish named loosely is resolved among the dishes cooked on the
		// day the sentence names: exactly one match backs it.
		{"guests-02 loose names, one match each that day", guests02, "4 people on Wednesday dinner", langEN, "Wednesday dinner is now set for 4 people, and the lasagne and cucumber salad are cooked for the larger cooking.", true},
		{"guests-09 loose names, unique that day", guests09, "a friend joins Friday dinner", langEN, "Friday dinner is now set for two, and the chicken traybake and salad are cooked for both of you.", true},
		{"elsewhere-12 DE: Greek Salad matches one dish of the day (Kichererbsen has no table, it is ignored)", elsewhere12, "Greek salad with chickpeas am Sonntag zum Abendessen kochen", langDE, "Das Greek Salad mit Kichererbsen wird jetzt am Sonntagabend gekocht und reicht für Montag Mittag und Abend.", true},
		{"an ambiguous loose name: two salads cook that day", append(slices.Clone(guests02), moved(kindLeftover, `Wed 9 Dec: cook 2 portions of the "Carrot salad" (was 1).`, "x")), "x", langEN, "Wednesday dinner is set for 4, and the salad is cooked for the larger cooking.", false},
		{"a loose name of a dish cooked on a different day", elsewhere10, "x", langEN, "The soup is cooked on Wednesday.", false},
		{"a loose name of a dish cooked on a different day (Sunday, not Monday)", elsewhere12, "x", langEN, "The Greek salad is cooked on Monday.", false},
		{"a loose name with no cooking day named", guests02, "x", langEN, "The lasagne is cooked fresh.", false},
		// V81 L1 (tune-v2c cook_elsewhere-12): a weekday after "reicht für" is a
		// day the cooking is eaten on; it is checked against what the cooking
		// feeds, not against the cooking days.
		{"L1 eaten after reicht für", elsewhere12, "Greek salad with chickpeas am Sonntag zum Abendessen kochen", langDE, "Die Greek salad with chickpeas wird jetzt am Sonntagabend gekocht und reicht für Montag Mittag und Abend.", true},
		{"L1 an eating day no cooking feeds", away16, "away on Monday", langEN, `The "Lasagne bolognese" is cooked on Sunday dinner and covers Monday lunch.`, false},
		// V81 L2 (tune-v2c guests-09): the first sentence alone.
		{"L2 guests-09 sentence", guests09, "a friend joins Friday dinner", langEN, "Friday dinner is now for two, and the traybake and green salad are cooked for two.", true},
		{"guests-02 by its plan name", guests02, "4 people on Wednesday dinner", langEN, "The Spinach lasagne is cooked for 4 on Wednesday.", true},

		{"a day the block does not cook on", elsewhere10, "cook the Minestrone on Monday dinner", langEN, "The Minestrone is now cooked on Tuesday dinner.", false},
		{"a dish not cooked on the day it is tied to", elsewhere12, "x", langEN, `The "Greek salad with chickpeas" is now cooked on Monday.`, false},
		{"a weekday no cooking line names", away16, "away on Monday", langEN, "The lasagne is now cooked on Friday instead.", false},
		{"a denial", away16, "away on Monday", langEN, "Nothing is cooked on Sunday any more.", false},
		{"DE: a day no cooking line names", away16, "Montag weg", langDE, "Die Lasagne wird jetzt am Dienstag gekocht.", false},
		{"DE: a dish on the wrong day", elsewhere12, "x", langDE, `„Greek salad with chickpeas“ wird jetzt am Montag gekocht.`, false},
		// M4: backed against the cook cell, not the eating days.
		{"a leftover day claimed as a cook day", elsewhere10, "x", langEN, "The Minestrone is cooked on Thursday.", false},
		{"DE: a leftover day claimed as a cook day", elsewhere10, "x", langDE, "Die Minestrone wird am Donnerstag gekocht.", false},
		{"a wrong meal", elsewhere10, "x", langEN, "The Minestrone is cooked on Monday lunch.", false},
		{"DE: a wrong meal", elsewhere10, "x", langDE, "Die Minestrone wird am Montag zum Mittagessen gekocht.", false},
		{"a right meal", elsewhere10, "x", langEN, "The Minestrone is cooked on Monday dinner.", true},
		{"DE: Nachmittag is no Mittag (lang EN: the fixture has no DE lines)", elsewhere10, "x", langEN, "Die Minestrone wird am Montag Nachmittag gekocht.", true},
		{"far-apart weekday still tied", elsewhere10, "x", langEN, "The Red lentil soup is cooked on Monday, Tuesday and Wednesday.", false},
		{"past tense", elsewhere10, "x", langEN, "The Minestrone was cooked on Monday dinner.", false},
		{"DE: past tense", elsewhere10, "x", langDE, "Die Minestrone wurde am Montag gekocht.", false},
		{"a cooking word with no day and no dish", elsewhere10, "x", langEN, "Everything is cooked fresh.", false},
		{"no cooking line at all", []fact{changed("Sat 5 Dec dinner: A replaced by B.", "x")}, "swap Saturday dinner", langEN, "Saturday dinner is now cooked fresh.", false},
	} {
		turn := replyTurn{user: tc.user, write: true, block: &replyBlock{facts: tc.facts}, foods: foods, numbers: map[string]bool{}}
		got := lintModelText(tc.text, turn, tc.lg)
		if tc.keep {
			assert.Empty(t, got, tc.name)
		} else {
			assert.Equal(t, "cooking_claim", got, tc.name)
		}
	}
}

// V77 lint (tune-v2b injection-10): "so the day stays above its minimum" on a
// day under its range was dropped as a contradiction: the minimum is the
// floor, not the range. A day said to be over its range is still dropped.
func TestContradicts_AboveTheMinimumIsNotOverTheRange(t *testing.T) {
	t.Parallel()
	under := []fact{{Class: classWarn, Kind: kindDay, Status: statusUnder, EN: "Thu 10 Dec holds 1518 kcal (was 1545), below a good amount for the day (1530-1785 kcal).", DE: "x"}}
	assert.False(t, contradicts("Thursday dinner is now the tomato lentil stew, with steamed rice added so the day stays above its minimum.", under, false, false))
	assert.True(t, contradicts("The day is now above its range.", under, false, false))
	assert.True(t, contradicts("The day is over its amount now, so it is above its minimum.", under, false, false))
}

// V77 lint (tune-v2b guests-00): "scaled up for the extra person" is a guest,
// not the add-on word RP3 bans; "an extra" (the add-on) is still dropped.
func TestLint_ExtraPersonIsNoPlannerWord(t *testing.T) {
	t.Parallel()
	block := &replyBlock{facts: []fact{
		{Class: classMoved, Kind: kindLeftover, Status: statusOK, EN: `Sat 5 Dec: cook 3 portions of the "Boiled potatoes" (was 2); cook 2.5 portions of the "Carrot salad" (was 1.25); cook 2 portions of the "Roast chicken legs with sweet potatoes" (was 1).`, DE: "x"},
		{Class: classMoved, Kind: kindGuest, Status: statusOK, EN: `Sat 5 Dec dinner: now for 2 people, 2 portions of "Roast chicken legs with sweet potatoes".`, DE: "x"},
	}}
	turn := replyTurn{user: "a friend joins Saturday dinner", write: true, block: block, numbers: map[string]bool{}}
	assert.Empty(t, lintModelText("Saturday dinner now serves two: the roast chicken, boiled potatoes and carrot salad are scaled up for the extra person.", turn, langEN))
	assert.Equal(t, "planner_word", lintModelText("Saturday dinner gets an extra on the side.", turn, langEN))
}

// V81 L2 (tune-v2c guests-09): two sentences that pass alone failed together:
// the joint check saw a cooking word of the first sentence and a dish of the
// second ("wholegrain bread", which no cooking line has). A cooking claim is
// judged per sentence, so the first sentence stays; the offer sentence goes
// as an option.
func TestComposeReply_CookingClaimIsJudgedPerSentence(t *testing.T) {
	t.Parallel()
	block := &replyBlock{facts: []fact{
		{Class: classMoved, Kind: kindLeftover, Status: statusOK, EN: `Fri 11 Dec: cook 2 portions of the "Chicken and vegetable traybake" (was 1); cook 2 portions of the "Green salad" (was 1).`, DE: "x"},
		{Class: classMoved, Kind: kindGuest, Status: statusOK, EN: `Fri 11 Dec dinner: now for 2 people, 2 portions of "Chicken and vegetable traybake".`, DE: "x"},
	}}
	var drops []lintDrop
	turn := replyTurn{
		user: "a friend joins Friday dinner", write: true, block: block, numbers: map[string]bool{}, drops: &drops,
		foods: []string{"Chicken and vegetable traybake", "Green salad", "Wholegrain bread"},
	}
	got := composeReply("Friday dinner is now for two, and the traybake and green salad are cooked for two. The program offers adding wholegrain bread to bring the day into range; I haven't applied it.", turn)
	assert.True(t, strings.HasPrefix(got, "Friday dinner is now for two, and the traybake and green salad are cooked for two."), got)
	for _, d := range drops {
		assert.NotEqual(t, "cooking_claim", d.Reason, d.Sentence)
	}
}

// V81 L4 (tune-v2c h2-planner-02): "30 minutes or less" came from the user's
// own "30 min max" and was dropped as an unbacked number. A number the user
// named backs the text; a kcal figure the user named does not.
func TestLint_UserNumbersBackTheText(t *testing.T) {
	t.Parallel()
	block := &replyBlock{facts: []fact{
		{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Mon 7 Dec dinner: A replaced by B.", DE: "x"},
		{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Fri 11 Dec dinner: C replaced by D.", DE: "x"},
	}}
	turn := replyTurn{user: "Could you please plan this week again, with quick dinners on weekdays (at most 30 minutes)? Thank you.", write: true, block: block, numbers: map[string]bool{}}
	assert.Empty(t, lintModelText("I've planned the whole week again, with every weekday dinner (Monday to Friday) at 30 minutes or less.", turn, langEN))
	floor := replyTurn{user: "Saturday around 600 kcal", write: true, block: block, numbers: map[string]bool{}}
	assert.Equal(t, "number", lintModelText("Saturday now holds about 600 kcal.", floor, langEN), "the user's kcal wish backs no plan figure")
	earlier := replyTurn{user: "yes", earlier: []string{"dinners of 25 minutes at most"}, write: true, block: block, numbers: map[string]bool{}}
	assert.Empty(t, lintModelText("Every dinner now takes 25 minutes or less.", earlier, langEN), "an earlier message of the conversation")
}

// V81 (tune-v2c g2-add-23): "Bitte füge am Freitag zum Abendessen Low-fat
// quark hinzu." got an English text over the German block. A sentence in the
// other language is dropped; a German sentence with an English dish name stays.
func TestComposeReply_TextInTheOtherLanguageIsDropped(t *testing.T) {
	t.Parallel()
	block := &replyBlock{facts: []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: `Fri 11 Dec dinner: added "Low-fat quark" (964 → 1031 kcal).`, DE: `Freitag, 11. Dez. Abendessen: hinzugefügt: „Low-fat quark“ (964 → 1031 kcal).`}}}
	turn := replyTurn{user: "Bitte füge am Freitag zum Abendessen Low-fat quark hinzu.", write: true, block: block, numbers: map[string]bool{"100": true, "1031": true}, foods: []string{"Low-fat quark", "Serbische Bohnensuppe mit weißen Riesenbohnen"}}
	got := composeReply("Low-fat quark is now on Friday's dinner, 100 g per person.\n\nFriday 2026-12-11 dinner: 1031 kcal per person, and the day stays inside its range.", turn)
	assert.True(t, strings.HasPrefix(got, "- Freitag, 11. Dez. Abendessen"), got)
	got = composeReply("Der Low-fat quark ist jetzt beim Abendessen am Freitag dabei.", turn)
	assert.True(t, strings.HasPrefix(got, "Der Low-fat quark ist jetzt"), got)
	l, ok := sentenceLang("The Serbische Bohnensuppe mit weißen Riesenbohnen stays.", []string{"Serbische Bohnensuppe mit weißen Riesenbohnen"})
	assert.True(t, !ok || l == langEN, "a German dish name decides nothing")
}

// V84 (review F6, decision change): an exclusive cooking claim ("one cooking
// time", "only once", "nur einmal gekocht") need not name every dish cooked
// that day. It stays when the day cooks at one time and the dishes it names are
// cooked then; it drops when the day cooks at more times than claimed, or a
// dish it names is not cooked then. "Only one dinner" is no cooking claim.
func TestLint_ExclusiveCookingClaim(t *testing.T) {
	t.Parallel()
	mk := func(facts ...fact) replyTurn {
		return replyTurn{
			user: "Friday dinner: Chicken and vegetable traybake", write: true, block: &replyBlock{facts: facts}, numbers: map[string]bool{},
			foods: []string{"Easy chicken curry", "Steamed rice", "Chicken and vegetable traybake", "Lentil soup"},
		}
	}
	cookDE := func(en, de string) fact {
		return fact{Class: classMoved, Kind: kindCook, Status: statusOK, EN: en, DE: de}
	}
	one := mk(
		fact{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: `Fri 4 Dec dinner: "Easy chicken curry" replaced by "Chicken and vegetable traybake" (659 → 596 kcal).`, DE: "x"},
		cookDE(`Fri 4 Dec at lunch you cook "Easy chicken curry" for Fri 4 Dec lunch; "Steamed rice" for Fri 4 Dec lunch, Fri 4 Dec dinner; "Chicken and vegetable traybake" for Fri 4 Dec dinner; the longest recipe takes about 50 min.`,
			`Freitag, 4. Dez. (Mittagessen) kochst du „Easy chicken curry“ für Freitag, 4. Dez. Mittagessen; „Steamed rice“ für Freitag, 4. Dez. Mittagessen, Freitag, 4. Dez. Abendessen; „Chicken and vegetable traybake“ für Freitag, 4. Dez. Abendessen.`))
	two := mk(
		cookDE(`Fri 4 Dec at lunch you cook "Easy chicken curry" for Fri 4 Dec lunch; the longest recipe takes about 50 min.`, `Freitag, 4. Dez. (Mittagessen) kochst du „Easy chicken curry“ für Freitag, 4. Dez. Mittagessen.`),
		cookDE(`Fri 4 Dec at dinner you cook "Chicken and vegetable traybake" for Fri 4 Dec dinner; the longest recipe takes about 50 min.`, `Freitag, 4. Dez. (Abendessen) kochst du „Chicken and vegetable traybake“ für Freitag, 4. Dez. Abendessen.`))
	for _, tc := range []struct {
		name string
		turn replyTurn
		lg   lang
		text string
		keep bool
	}{
		{"EN: one cooking time, only the named dish", one, langEN, "Friday dinner is now the chicken and vegetable traybake, cooked with Friday lunch so the day keeps one cooking time.", true},
		{"EN: all dishes named", one, langEN, "Friday lunch cooks the Easy chicken curry, the Steamed rice and the Chicken and vegetable traybake, so the day keeps one cooking time.", true},
		{"EN: only once, no day tied", one, langEN, "The Steamed rice is cooked only once.", true},
		{"EN: Friday cooks twice", two, langEN, "The Chicken and vegetable traybake is cooked on Friday dinner, so the day keeps one cooking time.", false},
		{"EN: Friday cooks twice, only once", two, langEN, "Friday is cooked only once.", false},
		{"EN: a named dish is not cooked then", one, langEN, "The Lentil soup is cooked on Friday, only once.", false},
		{"DE: nur einmal gekocht", one, langDE, "Die Chicken and vegetable traybake wird am Freitag nur einmal gekocht.", true},
		{"DE: zweimal am Freitag", two, langDE, "Die Chicken and vegetable traybake wird am Freitag nur einmal gekocht.", false},
	} {
		got := lintModelText(tc.text, tc.turn, tc.lg)
		if tc.keep {
			assert.Empty(t, got, tc.name)
		} else {
			assert.Equal(t, "cooking_claim", got, tc.name)
		}
	}
	assert.False(t, cookExclusive.MatchString("only one dinner"), "only one dinner is no exclusive claim")
	assert.False(t, cookExclusive.MatchString("nur eine Portion"), "nur eine Portion is no exclusive claim")
	assert.True(t, cookExclusive.MatchString("only once"))
	assert.True(t, cookExclusive.MatchString("nur einmal gekocht"))
}

// V84 review F1: a weekday after an eating word is checked against the dish
// the sentence names and the meal it names, not only against "some cooking
// line feeds that day".
func TestLint_EatingDayIsFedByTheNamedDish(t *testing.T) {
	t.Parallel()
	cook := func(en, de string) fact {
		return fact{Class: classMoved, Kind: kindCook, Status: statusOK, EN: en, DE: de}
	}
	block := &replyBlock{facts: []fact{
		cook(`Sun 29 Nov at dinner you cook "Lasagne" for Mon 30 Nov lunch; the longest recipe takes about 40 min.`, `Sonntag, 29. Nov. (Abendessen) kochst du „Lasagne“ für Montag, 30. Nov. Mittagessen.`),
		cook(`Tue 1 Dec at dinner you cook "Chili" for Tue 1 Dec dinner, Wed 2 Dec lunch; the longest recipe takes about 30 min.`, `Dienstag, 1. Dez. (Abendessen) kochst du „Chili“ für Dienstag, 1. Dez. Abendessen, Mittwoch, 2. Dez. Mittagessen.`),
	}}
	turn := replyTurn{user: "x", write: true, block: block, numbers: map[string]bool{}, foods: []string{"Lasagne", "Chili", "Wholegrain bread"}}
	for _, tc := range []struct {
		name string
		lg   lang
		text string
		keep bool
	}{
		{"EN: the dish feeds that day", langEN, "The Lasagne is cooked on Sunday dinner and covers Monday lunch.", true},
		{"EN: the named dish does not feed that day", langEN, "The Chili is cooked for Monday.", false},
		{"DE: the named dish does not feed that day", langDE, "Chili wird für Montag gekocht.", false},
		{"DE: the dish feeds that day", langDE, "Die Lasagne wird am Sonntagabend gekocht und reicht für Montag Mittag.", true},
		{"EN: right dish, wrong meal", langEN, "The Lasagne is cooked on Sunday dinner and covers Monday dinner.", false},
		{"DE: right dish, wrong meal", langDE, "Die Lasagne wird am Sonntagabend gekocht und reicht für Montag Abend.", false},
		{"EN: right dish and meal on the second line", langEN, "The Chili is cooked on Tuesday dinner and covers Wednesday lunch.", true},
		{"EN: a loose name that does not feed the day", langEN, "The chili is cooked on Tuesday dinner and covers Monday lunch.", false},
	} {
		got := lintModelText(tc.text, turn, tc.lg)
		if tc.keep {
			assert.Empty(t, got, tc.name)
		} else {
			assert.Equal(t, "cooking_claim", got, tc.name)
		}
	}
}

// V84 review F2: a number the user named backs the model's text only as a
// time with the same unit: never kcal or a count, never inside a range or a
// hyphenated unit, never a number the user typed as part of a date.
func TestLint_UserNumberBacksOnlyTheSameTime(t *testing.T) {
	t.Parallel()
	block := &replyBlock{facts: []fact{
		{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Mon 7 Dec dinner: A replaced by B.", DE: "Montag, 7. Dez. Abendessen: A ersetzt durch B."},
	}}
	turn := func(user string) replyTurn {
		return replyTurn{user: user, write: true, block: block, numbers: map[string]bool{"793": true}}
	}
	for _, tc := range []struct {
		name, user, text string
		lg               lang
		keep             bool
	}{
		{"EN: same time unit", "dinners of 30 min max", "Every dinner is under 30 minutes.", langEN, true},
		{"EN: a time unit in the text without a unit of the user", "dinners of 30 portions", "Every dinner is under 30 minutes.", langEN, false},
		{"EN: other unit class", "dinners of 30 min max", "Every dinner is under 30 hours.", langEN, false},
		{"EN: kcal in a range", "Saturday 600 kcal", "Saturday is from 450 to 600 kcal.", langEN, false},
		{"EN: kcal as part of a comparison", "Saturday 600 kcal", "Saturday is now at 600 of 793.", langEN, false},
		{"EN: hyphenated kcal", "Saturday 600 kcal", "Saturday is a 600-kcal day.", langEN, false},
		{"EN: a time inside a range", "dinners of 30 min max", "Every dinner takes 20 to 30 minutes.", langEN, false},
		{"EN: a date the user typed", "move it to 25 Nov please", "Every dinner is under 25 minutes.", langEN, false},
		{"EN: a count", "dinner for 4 people", "Dinner is for 4 portions.", langEN, false},
		{"DE: same time unit", "Abendessen in höchstens 30 Min", "Jedes Abendessen dauert höchstens 30 Minuten.", langDE, true},
		{"DE: kcal", "Samstag 600 kcal", "Der Samstag liegt bei 600 kcal.", langDE, false},
		{"DE: a range", "Abendessen in höchstens 30 Min", "Das Abendessen dauert 20 bis 30 Minuten.", langDE, false},
	} {
		got := lintModelText(tc.text, turn(tc.user), tc.lg)
		if tc.keep {
			assert.Empty(t, got, tc.name)
		} else {
			assert.Equal(t, "number", got, tc.name)
		}
	}
}

// V84 review F5: a sentence is "in the other language" only with three marker
// words of that language and none of the other; an unquoted off-plan English
// dish name inside a German sentence decides nothing.
func TestSentenceLang_NeedsAClearLead(t *testing.T) {
	t.Parallel()
	_, ok := sentenceLang("Jetzt ist the Fish and the Chips with the sauce dabei.", nil)
	assert.False(t, ok, "a German sentence with an English dish name")
	l, ok := sentenceLang("The dinner is now on the plan with the new dish.", nil)
	assert.True(t, ok)
	assert.Equal(t, langEN, l, "a fully English sentence")
	l, ok = sentenceLang("Das Abendessen ist jetzt der Plan und das Mittagessen auch.", nil)
	assert.True(t, ok)
	assert.Equal(t, langDE, l)
	_, ok = sentenceLang("Go to the plan, no problem, it is on me.", nil)
	assert.False(t, ok, "to, no, on, of, is, me are no markers")
	_, ok = sentenceLang("I am here on the Tag of the die roll.", nil)
	assert.False(t, ok, "am, tag and die are no German markers")

	// In a composed reply over a German message.
	block := &replyBlock{facts: []fact{{Class: classChanged, Kind: kindMeal, Status: statusOK, EN: "Fri 11 Dec dinner: added.", DE: "Freitag, 11. Dez. Abendessen: hinzugefügt."}}}
	turn := replyTurn{user: "Bitte füge am Freitag zum Abendessen etwas hinzu.", write: true, block: block, numbers: map[string]bool{}}
	kept := composeReply("Jetzt ist der Plan für Freitag mit the Fish and the Chips fertig.", turn)
	assert.True(t, strings.HasPrefix(kept, "Jetzt ist der Plan"), kept)
	dropped := composeReply("The dinner is now on the plan with the new dish.", turn)
	assert.True(t, strings.HasPrefix(dropped, "- Freitag"), dropped)
}
