package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

func restoreText(text string, tell ...string) string {
	m := llm.Message{Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: text}}}
	restoreMust(&m, tell)
	return m.Content[0].Text
}

const noYogurtReply = `I removed the Greek yogurt from six of the seven dinners. I left it in on Wed 25 Nov, because taking it out would drop that day to 1513 kcal, under its minimum of 1650 kcal.

Day energy:
- Sat 21 Nov is 1328 kcal, a good amount for the day (1320–1613 kcal).
- Sun 22 Nov is 1943 kcal, below a good amount for the day (1980–2420 kcal).
- Mon 23 Nov is 1971 kcal, a good amount for the day (1815–2218 kcal).
- Tue 24 Nov is 2026 kcal, a good amount for the day (1815–2218 kcal).

I can also swap Wed 25 Nov's yogurt for Hasselback potatoes. Say so if you want it.`

var noYogurtTell = []string{
	"Sun 22 Nov holds 1943 kcal (was 2193), below a good amount for the day (1980-2420 kcal).",
	"Tue 24 Nov holds 2026 kcal (was 2220), a good amount for the day (1815-2218 kcal).",
	"Fri 27 Nov holds 1794 kcal (was 1988), below a good amount for the day (1815-2218 kcal).",
	"For the week this means: average energy is 92% of the target (was 100%); the week's range is 95–105%.",
}

// E047: the no-yogurt reply stopped at Tuesday and never said the week.
func TestRestoreMust_NoYogurt(t *testing.T) {
	got := restoreText(noYogurtReply, noYogurtTell...)
	want := noYogurtReply[:len(noYogurtReply)-len("I can also swap Wed 25 Nov's yogurt for Hasselback potatoes. Say so if you want it.")] +
		"- Fri 27 Nov: 1794 kcal, below a good amount for the day (1815–2218 kcal).\n" +
		"- Week: average energy is 92% of the target (was 100%); the week's range is 95–105%.\n\n" +
		"I can also swap Wed 25 Nov's yogurt for Hasselback potatoes. Say so if you want it."
	assert.Equal(t, want, got)
	assert.Equal(t, got, restoreText(got, noYogurtTell...), "nothing is added twice")
}

func TestRestoreMust_AlreadySaid(t *testing.T) {
	r := noYogurtReply + "\n\nFriday ends at 1794 kcal, below its range. The week averages 92% of the target."
	assert.Equal(t, r, restoreText(r, noYogurtTell...))
}

// E047: "321 g, same as before" hid that the shorter week is 64 g over; the fat
// share rose.
func TestRestoreMust_AwaySun(t *testing.T) {
	tell := []string{
		"With Sun 22 Nov away the week is counted over 6 days; the meat limit for those days is 257 g and the plan holds 321 g, as before; that is 64 g over.",
		"Worse: Weekly fat: 41% of the week's energy; the range is 25-35%. It was already open and got worse: 4.7 points beyond its limit before, 6.2 now.",
	}
	reply := "Sunday 22 Nov is now away.\n\nThe week is counted over 6 days. The meat limit for those days is 257 g, and the plan holds 321 g, the same as before."
	got := restoreText(reply, tell...)
	assert.Contains(t, got, "- Week: meat is 321 g against a limit of 257 g, 64 g over.")
	assert.Contains(t, got, "- Week: fat is 41% of the energy (range 25–35%), further out than before.")

	ok := reply + " That is 64 g over. The week's fat share is 41%, above its range."
	assert.Equal(t, ok, restoreText(ok, tell...))
}

// E047: cook-thu left Friday's lost cooking unsaid.
func TestRestoreMust_CookThu(t *testing.T) {
	tell := []string{
		`The "Chili con carne" is now cooked on Thu 26 Nov at dinner instead of Fri 27 Nov at dinner.`,
		"Fri 27 Nov: nothing is cooked.",
	}
	reply := "The chili is now cooked on Thursday at dinner.\n\nThursday's cooking is about 3 h 45 min in all."
	got := restoreText(reply, tell...)
	assert.Contains(t, got, "- "+tell[0])
	assert.Contains(t, got, "- Fri 27 Nov: nothing is cooked any more.")

	ok := "The chili is now cooked on Thursday instead of Friday. Friday has no cooking any more."
	assert.Equal(t, ok, restoreText(ok, tell...))
}

func TestRestoreMust_German(t *testing.T) {
	tell := append([]string{
		"With Sun 22 Nov away the week is counted over 6 days; the meat limit for those days is 257 g and the plan holds 321 g, as before; that is 64 g over.",
		`The "Chili con carne" is now cooked on Thu 26 Nov at dinner instead of Fri 27 Nov at dinner.`,
	}, noYogurtTell[2:]...)
	reply := "Das Abendessen ist jetzt ohne Joghurt und der Sonntag ist nicht mehr dabei.\n\n- Sonntag: 1943 kcal.\n\nSag Bescheid, wenn du das willst: if you want it."
	got := restoreText(reply, tell...)
	assert.Contains(t, got, "- Woche: Fleisch 321 g bei höchstens 257 g, also 64 g darüber.")
	assert.Contains(t, got, "- „Chili con carne“ wird jetzt am Donnerstag, 26. Nov. (zum Abendessen) gekocht, nicht mehr am Freitag, 27. Nov. (zum Abendessen).")
	assert.Contains(t, got, "- Freitag, 27. Nov.: 1794 kcal, unter dem passenden Bereich für den Tag (1815–2218 kcal).")
	assert.Contains(t, got, "- Woche: Durchschnitt 92 % des Ziels (vorher 100 %); der Bereich liegt bei 95–105 %.")
	assert.Equal(t, got, restoreText(got, tell...))
}

func TestMustItems_Ignored(t *testing.T) {
	for _, s := range []string{
		"Tue 24 Nov holds 2026 kcal (was 2220), a good amount for the day (1815-2218 kcal).",
		"For the week this means: average energy is 100% of the target (was 98%); the week's range is 95–105%; meat is 200 g (was 330 g, limit 300 g).",
		"Sun 22 Nov dinner: x.",
	} {
		assert.Empty(t, mustItems(s), s)
	}
}
