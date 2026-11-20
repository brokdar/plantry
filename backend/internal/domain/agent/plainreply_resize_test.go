package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var resizeItems = []mustFact{
	{Kind: factResize, Day: "Sun 22 Nov", Dish: "Speedy lentil coconut curry", N: 3.5, Was: 3},
	{Kind: factResize, Day: "Mon 23 Nov", Dish: "Lachs im Blätterteig", N: 1.5, Was: 1},
	{Kind: factResize, Day: "Mon 23 Nov", Dish: "Tomato salad", N: 2, Was: 1},
}

func restored(text string) string { return restoreText(text, resizeItems...) }

// E044: a reply that names the resized batches without their days gets the
// day sentences back; a reply that has them is left alone.
func TestRestoreResizes(t *testing.T) {
	t.Parallel()
	r3 := "Tuesday dinner is now a light dinner.\n\n- **Cooking:** The salmon, tomato salad and lentil curry batches are resized to cover the new portions, with nothing left over.\n- **Day:** Tue 24 Nov now holds 1713 kcal.\n\nI can also add Ciabatta at lunch: say so if you want it."
	got := restored(r3)
	for _, f := range resizeItems {
		it, _ := f.item()
		assert.Contains(t, got, "- "+it.en)
	}
	assert.True(t, strings.HasSuffix(got, "if you want it."), "the offer stays last")

	r1 := "Done.\n\n- **Cooking:** The salmon cooked on Mon 23 Nov is now 1.5 portions, the lentil curry cooked on Sun 22 Nov is 3.5 portions, and the tomato salad cooked on Mon 23 Nov is 2 portions."
	assert.Equal(t, r1, restored(r1), "no duplication")
	// A weekday name and a number word say the same.
	r4 := "Done. Sunday's lentil curry becomes 3.5 portions, and on Monday the salmon pastry is 1.5 portions and the tomato salad is two portions."
	assert.Equal(t, r4, restored(r4), "weekday names and number words count")

	fr := "Le dîner est maintenant léger. Les currys ont été ajustés."
	assert.Equal(t, fr, restored(fr), "another language is left as written")
}

// E044b: a German reply that drops the other-day resize gets it back in German.
func TestRestoreResizesGerman(t *testing.T) {
	t.Parallel()
	run := func(text string) string { return restoreText(text, resizeItems[0]) }
	failing := "Das Abendessen am Mittwoch ist jetzt leicht und die Portionen sind angepasst.\n\n- Dienstag: Das Gericht wird mit 2 Portionen gekocht.\n- Mittwoch: 1713 kcal."
	want := failing + "\n\n- Sonntag, 22. Nov.: 3,5 Portionen „Speedy lentil coconut curry“ kochen (statt 3)."
	assert.Equal(t, want, run(failing))

	for _, ok := range []string{
		"Das ist jetzt fertig und die Sonntags-Charge wird für 3,5 Portionen gekocht.",
		"Das ist jetzt fertig: Am 22. Nov. wird der Curry mit 3,5 Portionen gekocht.",
		"Das ist jetzt fertig: Am 22.11. wird der Curry mit 3,5 Portionen gekocht.",
		"Das ist jetzt fertig: Am So. wird der Curry mit 3,5 Portionen gekocht.",
	} {
		assert.Equal(t, ok, run(ok))
	}
	wrong := "Das ist jetzt fertig und die Sonntags-Charge wird für 35 Portionen gekocht."
	assert.NotEqual(t, wrong, run(wrong), "other portions do not count")
}

func TestMustTold(t *testing.T) {
	t.Parallel()
	b, _ := json.Marshal(map[string]any{"report": map[string]any{"tell": []string{"x"}, "must": resizeItems}})
	items, saved := mustTold(b)
	assert.True(t, saved)
	assert.Equal(t, resizeItems, items)

	_, saved = mustTold([]byte(`{"report_if_applied":{"tell":["x"],"must":[{"kind":"resized"}]}}`))
	assert.False(t, saved, "a dry run is not a saved change")
}

func TestWithoutMust(t *testing.T) {
	t.Parallel()
	b, _ := json.Marshal(map[string]any{"saved": true, "report": map[string]any{"tell": []string{"x"}, "must": resizeItems}})
	got := string(withoutMust(b))
	assert.NotContains(t, got, "must")
	assert.Contains(t, got, `"tell":["x"]`)
	assert.Equal(t, `{"a":1}`, string(withoutMust([]byte(`{"a":1}`))))
}
