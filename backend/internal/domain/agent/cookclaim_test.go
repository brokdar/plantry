package agent

import (
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
	foods := []string{"Minestrone", "Red lentil soup", "Greek salad with chickpeas", "Couscous", "Lasagne bolognese", "Spinach lasagne", "Chicken and vegetable traybake", "Fruit salad"}
	for _, tc := range []struct {
		name  string
		facts []fact
		user  string
		lg    lang
		text  string
		keep  bool
	}{
		{"elsewhere-10 true", elsewhere10, "cook the Minestrone on Monday dinner", langEN, "The Minestrone is now cooked on Monday dinner and still covers Wednesday lunch and dinner and Thursday lunch.", true},
		{"elsewhere-12 true", elsewhere12, "Greek salad with chickpeas am Sonntag zum Abendessen kochen", langDE, "Das Greek Salad mit Kichererbsen wird jetzt am Sonntagabend gekocht und reicht für Montag Mittag und Abend.", true},
		{"away-16 true", away16, "away on Monday", langEN, "Monday lunch and dinner are cleared, and Sunday's dinner cooking is trimmed to cover only Sunday.", true},
		{"guests-02 true", guests02, "4 people on Wednesday dinner", langEN, "Wednesday dinner is now set for 4 people, and the lasagne and cucumber salad are cooked for the larger cooking.", true},
		{"guests-09 true", guests09, "a friend joins Friday dinner", langEN, "Friday dinner is now set for two, and the chicken traybake and salad are cooked for both of you.", true},

		{"a day the block does not cook on", elsewhere10, "cook the Minestrone on Monday dinner", langEN, "The Minestrone is now cooked on Tuesday dinner.", false},
		{"a dish not cooked on the day it is tied to", elsewhere12, "x", langEN, `The "Greek salad with chickpeas" is now cooked on Monday.`, false},
		{"a weekday no cooking line names", away16, "away on Monday", langEN, "The lasagne is now cooked on Friday instead.", false},
		{"a denial", away16, "away on Monday", langEN, "Nothing is cooked on Sunday any more.", false},
		{"DE: a day no cooking line names", away16, "Montag weg", langDE, "Die Lasagne wird jetzt am Dienstag gekocht.", false},
		{"DE: a dish on the wrong day", elsewhere12, "x", langDE, `„Greek salad with chickpeas“ wird jetzt am Montag gekocht.`, false},
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
	assert.False(t, contradicts("Thursday dinner is now the tomato lentil stew, with steamed rice added so the day stays above its minimum.", under, false))
	assert.True(t, contradicts("The day is now above its range.", under, false))
	assert.True(t, contradicts("The day is over its amount now, so it is above its minimum.", under, false))
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
