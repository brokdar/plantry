package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

func restoreText(text string, facts ...mustFact) string {
	m := llm.Message{Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: text}}}
	restoreMust(&m, facts)
	return m.Content[0].Text
}

func dayFact(day string, kcal float64, side string, lo, hi float64) mustFact {
	return mustFact{Kind: factDay, Day: day, Kcal: kcal, Side: side, Lo: lo, Hi: hi}
}

const noYogurtReply = `I removed the Greek yogurt from six of the seven dinners. I left it in on Wed 25 Nov, because taking it out would drop that day to 1513 kcal, under its minimum of 1650 kcal.

Day energy:
- Sat 21 Nov is 1328 kcal, a good amount for the day (1320–1613 kcal).
- Sun 22 Nov is 1943 kcal, below a good amount for the day (1980–2420 kcal).
- Mon 23 Nov is 1971 kcal, a good amount for the day (1815–2218 kcal).
- Tue 24 Nov is 2026 kcal, a good amount for the day (1815–2218 kcal).

I can also swap Wed 25 Nov's yogurt for Hasselback potatoes. Say so if you want it.`

var noYogurtFacts = []mustFact{
	dayFact("Sun 22 Nov", 1943, "below", 1980, 2420),
	dayFact("Fri 27 Nov", 1794, "below", 1815, 2218),
	{Kind: factEnergy, N: 92, Was: 100, Lo: 95, Hi: 105},
}

// E047: the no-yogurt reply stopped at Tuesday and never said the week.
func TestRestoreMust_NoYogurt(t *testing.T) {
	got := restoreText(noYogurtReply, noYogurtFacts...)
	closing := "I can also swap Wed 25 Nov's yogurt for Hasselback potatoes. Say so if you want it."
	want := noYogurtReply[:len(noYogurtReply)-len(closing)] +
		"- Fri 27 Nov: 1794 kcal, below a good amount for the day (1815–2218 kcal).\n" +
		"- Week: average energy is 92% of the target (was 100%); the week's range is 95–105%.\n\n" + closing
	assert.Equal(t, want, got)
	assert.Equal(t, got, restoreText(got, noYogurtFacts...), "nothing is added twice")
}

func TestRestoreMust_AlreadySaid(t *testing.T) {
	r := noYogurtReply + "\n\nFriday ends at 1794 kcal, below its range. The week averages 92% of the target."
	assert.Equal(t, r, restoreText(r, noYogurtFacts...))
}

// E047: "321 g, same as before" hid that the shorter week is 64 g over; the fat
// share rose.
func TestRestoreMust_AwaySun(t *testing.T) {
	facts := []mustFact{
		{Kind: factAwayMeat, N: 321, Limit: 257},
		{Kind: factShare, Share: "fat", N: 41, Lo: 25, Hi: 35},
	}
	reply := "Sunday 22 Nov is now away.\n\nThe week is counted over 6 days. The meat limit for those days is 257 g, and the plan holds 321 g, the same as before."
	got := restoreText(reply, facts...)
	assert.Contains(t, got, "- Week: meat is 321 g against a limit of 257 g, 64 g over.")
	assert.Contains(t, got, "- Week: fat is 41% of the energy (range 25–35%), further out than before.")

	ok := reply + " That is 64 g over. The week's fat share is 41%, above its range."
	assert.Equal(t, ok, restoreText(ok, facts...))
}

var cookThuFacts = []mustFact{
	{Kind: factMoved, Dish: "Chili con carne", Day: "Thu 26 Nov", Meal: "dinner", Day2: "Fri 27 Nov", Meal2: "dinner"},
	{Kind: factNoCook, Day: "Fri 27 Nov"},
}

// E047: cook-thu left Friday's lost cooking unsaid.
func TestRestoreMust_CookThu(t *testing.T) {
	reply := "The chili is now cooked on Thursday at dinner.\n\nThursday's cooking is about 3 h 45 min in all."
	got := restoreText(reply, cookThuFacts...)
	assert.Contains(t, got, "- The \"Chili con carne\" is now cooked on Thu 26 Nov at dinner instead of Fri 27 Nov at dinner.")
	assert.Contains(t, got, "- Fri 27 Nov: nothing is cooked any more.")

	ok := "The chili is now cooked on Thursday instead of Friday. Friday has no cooking any more."
	assert.Equal(t, ok, restoreText(ok, cookThuFacts...))
}

func TestRestoreMust_German(t *testing.T) {
	facts := append([]mustFact{
		{Kind: factAwayMeat, N: 321, Limit: 257},
		cookThuFacts[0],
	}, noYogurtFacts[1:]...)
	reply := "Das Abendessen ist jetzt ohne Joghurt und der Sonntag ist nicht mehr dabei.\n\n- Sonntag: 1943 kcal.\n\nSag Bescheid, wenn du das willst."
	got := restoreText(reply, facts...)
	assert.Contains(t, got, "- Woche: Fleisch 321 g bei höchstens 257 g, also 64 g darüber.")
	assert.Contains(t, got, "- „Chili con carne“ wird jetzt am Donnerstag, 26. Nov. (zum Abendessen) gekocht, nicht mehr am Freitag, 27. Nov. (zum Abendessen).")
	assert.Contains(t, got, "- Freitag, 27. Nov.: 1794 kcal, unter dem passenden Bereich für den Tag (1815–2218 kcal).")
	assert.Contains(t, got, "- Woche: Durchschnitt 92 % des Ziels (vorher 100 %); der Bereich liegt bei 95–105 %.")
	assert.Equal(t, got, restoreText(got, facts...))
	assert.Less(t, indexOf(got, "Woche: Fleisch"), indexOf(got, "Sag Bescheid"), "the lines come before the closing offer")
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

var shorterFact = mustFact{
	Kind: factShorter, Day: "Fri 27 Nov", Meal: "dinner", Dishes: []string{"Hasselback potatoes"},
	Gone: []goneDish{{Dish: "Chili con carne", Leftovers: true}}, Time: "1 h 15 min", WasTime: "2 h 35 min",
}

// E051: Friday's cooking that shrinks to the potatoes, and the guest at the small lunch.
func TestRestoreMust_ShorterCooking(t *testing.T) {
	reply := "The chili is now cooked on Thursday at dinner.\n\nThursday's cooking is about 3 h 45 min in all."
	assert.Contains(t, restoreText(reply, shorterFact), `- Fri 27 Nov at dinner you now cook only "Hasselback potatoes": about 1 h 15 min in all (was 2 h 35 min); the "Chili con carne" is now eaten as leftovers.`)
	got := restoreText("Das Chili wird jetzt am Donnerstag gekocht und der Tag ist damit anders.", shorterFact)
	assert.Contains(t, got, "- Freitag, 27. Nov.: nur noch „Hasselback potatoes“ kochen, etwa 1 h 15 min statt 2 h 35 min; „Chili con carne“ gibt es als Reste.")
	ok := "The chili is now cooked on Thursday. On Friday you only cook the potatoes and eat the chili as leftovers."
	assert.Equal(t, ok, restoreText(ok, shorterFact))
}

func TestRestoreMust_SmallLunchGuest(t *testing.T) {
	f := mustFact{Kind: factSmall, Day: "Mon 23 Nov", Meal: "lunch", N: 2, Kcal: 331}
	reply := "Your friend is added to Monday's lunch.\n\nThe curry is now cooked for 4 portions."
	assert.Contains(t, restoreText(reply, f), "- Mon 23 Nov lunch: now for 2 people; it is the small lunch (331 kcal each).")
	got := restoreText("Dein Freund ist jetzt beim Mittagessen am Montag dabei und die Charge ist größer.", f)
	assert.Contains(t, got, "- Montag, 23. Nov. zum Mittagessen: jetzt für 2 Personen; es ist das kleine Mittagessen (331 kcal pro Person).")
	ok := "Your friend is added to Monday's lunch, which is the small lunch of that day (331 kcal)."
	assert.Equal(t, ok, restoreText(ok, f))
}

// The old matcher took "salmon " for Monday and "followed " for Wednesday.
func TestDayIn_WordBoundaries(t *testing.T) {
	assert.False(t, dayIn("The salmon is cooked for 2 portions", "Mon 23 Nov"))
	assert.False(t, dayIn("It followed the plan", "Wed 25 Nov"))
	assert.False(t, dayIn("The sun showed up", "Sun 22 Nov"))
	for _, l := range []string{"On Monday", "Mon 23 Nov", "on Mon.", "23 Nov", "23rd Nov", "Nov 23", "am Montag", "Montags", "Montagabend", "am 23. Nov.", "am 23.11.", "Mo. gibt es"} {
		assert.True(t, dayIn(l, "Mon 23 Nov"), l)
	}
	assert.False(t, dayIn("So wird es gemacht", "Sun 22 Nov"), "the German word so is no Sunday")
	assert.True(t, dayIn("am So. gibt es", "Sun 22 Nov"))
}

func TestNumberIn_Words(t *testing.T) {
	assert.True(t, numberIn("it keeps three days", "3"))
	assert.True(t, numberIn("hält drei Tage", "3"))
	assert.True(t, numberIn("3,5 Portionen", "3.5"))
	assert.False(t, numberIn("35 portions", "3.5"))
	assert.False(t, numberIn("one cooking", "1"), "1 is a digit only")
}

func TestCountDays(t *testing.T) {
	assert.Equal(t, "1 day", countDays(1, false))
	assert.Equal(t, "4 days", countDays(4, false))
	assert.Equal(t, "1 Tag", countDays(1, true))
	assert.Equal(t, "4 Tage", countDays(4, true))
	one := keepItem(mustFact{Kind: factKeep, Day: "Wed 25 Nov", Dish: "Rice", Keep: 1, Last: "Thu 26 Nov"})
	assert.Contains(t, one.en, "keeps 1 day in the fridge")
	assert.Contains(t, one.de, "ist 1 Tag im Kühlschrank haltbar, letzter Tag ist Donnerstag, 26. Nov.")
	assert.NotContains(t, one.de, "..")
}

func TestInsertBeforeClosing(t *testing.T) {
	for _, tc := range []struct{ name, text, wantEnd string }{
		{"en offer", "Done.\n\nSay so if you want it.", "Say so if you want it."},
		{"en question", "Done.\n\nShall I do the same for Friday?", "Shall I do the same for Friday?"},
		{"de question", "Erledigt.\n\nSoll ich das auch für Freitag machen?", "Soll ich das auch für Freitag machen?"},
		{"de offer", "Erledigt.\n\nSag Bescheid, wenn du das auch willst.", "Sag Bescheid, wenn du das auch willst."},
		{"two closing", "Done.\n\nI can also move it.\n\nWould you like that?", "Would you like that?"},
	} {
		got := insertBeforeClosing(tc.text, "- X")
		assert.Truef(t, len(got) > len(tc.wantEnd) && got[len(got)-len(tc.wantEnd):] == tc.wantEnd, "%s: %q", tc.name, got)
		assert.Less(t, indexOf(got, "- X"), indexOf(got, tc.wantEnd), tc.name)
	}
	assert.Equal(t, "Done.\n\n- a\n- b\n\n- X", insertBeforeClosing("Done.\n\n- a\n- b", "- X"), "a list is not a closing")
}
