package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

var resizeItems = []string{
	`Sun 22 Nov: cook 3.5 portions of the "Speedy lentil coconut curry" (was 3).`,
	`Mon 23 Nov: cook 1.5 portions of the "Lachs im Blätterteig" (was 1).`,
	`Mon 23 Nov: cook 2 portions of the "Tomato salad" (was 1).`,
}

func restored(text string) string {
	m := llm.Message{Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: text}}}
	restoreResizes(&m, resizeItems)
	return m.Content[0].Text
}

// E044: a reply that names the resized batches without their days gets the
// day sentences back; a reply that has them is left alone.
func TestRestoreResizes(t *testing.T) {
	r3 := "Tuesday dinner is now a light dinner.\n\n- **Cooking:** The salmon, tomato salad and lentil curry batches are resized to cover the new portions, with nothing left over.\n- **Day:** Tue 24 Nov now holds 1713 kcal.\n\nI can also add Ciabatta at lunch: say so if you want it."
	got := restored(r3)
	for _, it := range resizeItems {
		assert.Contains(t, got, "- "+it)
	}
	assert.True(t, strings.HasSuffix(got, "if you want it."), "the offer stays last")

	r1 := "Done.\n\n- **Cooking:** The salmon cooked on Mon 23 Nov is now 1.5 portions, the lentil curry cooked on Sun 22 Nov is 3.5 portions, and the tomato salad cooked on Mon 23 Nov is 2 portions."
	assert.Equal(t, r1, restored(r1), "no duplication")

	fr := "Le dîner est maintenant léger. Les currys ont été ajustés."
	assert.Equal(t, fr, restored(fr), "another language is left as written")
}

// E044b: a German reply that drops the other-day resize gets it back in German.
func TestRestoreResizesGerman(t *testing.T) {
	one := []string{resizeItems[0]}
	run := func(text string) string {
		m := llm.Message{Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: text}}}
		restoreResizes(&m, one)
		return m.Content[0].Text
	}
	failing := "Das Abendessen am Mittwoch ist jetzt leicht und die Portionen sind angepasst.\n\n- Dienstag: Das Gericht wird mit 2 Portionen gekocht.\n- Mittwoch: 1713 kcal."
	want := failing + "\n\n- So., 22. Nov.: 3,5 Portionen „Speedy lentil coconut curry“ kochen (statt 3)."
	assert.Equal(t, want, run(failing))

	for _, ok := range []string{
		"Das ist jetzt fertig und die Sonntags-Charge wird für 3,5 Portionen gekocht.",
		"Das ist jetzt fertig: Am 22. Nov. wird der Curry mit 3,5 Portionen gekocht.",
		"Das ist jetzt fertig: Am 22.11. wird der Curry mit 3,5 Portionen gekocht.",
	} {
		assert.Equal(t, ok, run(ok))
	}
	wrong := "Das ist jetzt fertig und die Sonntags-Charge wird für 35 Portionen gekocht."
	assert.NotEqual(t, wrong, run(wrong), "other portions do not count")
}

func TestResizeTold(t *testing.T) {
	tell := append([]string{"Tue 24 Nov dinner: x."}, resizeItems...)
	b, _ := json.Marshal(map[string]any{"report": map[string]any{"tell": tell}})
	items, saved := resizeTold(b)
	assert.True(t, saved)
	assert.Equal(t, resizeItems, items)

	_, saved = resizeTold([]byte(`{"report_if_applied":{"tell":["x"]}}`))
	assert.False(t, saved, "a dry run is not a saved change")
}
