package importer_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// MATCH-6: only seasonings are "to taste"; another quantity-less line gets a
// proposed amount, or none when no rule knows it.
func TestParseLines_ToTasteOnlyForSeasonings(t *testing.T) {
	rows := importer.ParseLines([]string{"Salz", "Petersilie", "Olivenöl", "Zucker", "Zitronenspalten zum Servieren"}, "de")
	require.Len(t, rows, 5)
	type got struct {
		toTaste, proposed bool
		amount            float64
		unit              string
	}
	pick := func(r importer.DraftIngredient) got { return got{r.ToTaste, r.Proposed, r.Amount, r.Unit} }
	assert.Equal(t, got{toTaste: true}, pick(rows[0]), "salt")
	assert.Equal(t, got{toTaste: true}, pick(rows[1]), "fresh herb")
	assert.Equal(t, got{proposed: true, amount: 1, unit: "tbsp"}, pick(rows[2]), "oil")
	assert.Equal(t, got{proposed: true, amount: 1, unit: "tsp"}, pick(rows[3]), "sugar")
	assert.Equal(t, got{}, pick(rows[4]), "unknown: the review asks for an amount")
}

// C2b: seasonings measured in no unit become clean to-taste rows, and a
// price annotation from the recipe site never reaches a line (canary
// 2026-11-27: "a tiny pinch cayenne" matched nothing).
func TestParseLine_LooseSeasoningsAndPrices(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ raw, lang, name string }{
		{"a tiny pinch cayenne", "en", "cayenne"},
		{"a few turns of black pepper", "en", "black pepper"},
		{"a pinch of salt", "en", "salt"},
		{"eine Prise Salz", "de", "Salz"},
		{"ein paar Umdrehungen Pfeffer", "de", "Pfeffer"},
		{"Salz", "de", "Salz"},
	} {
		rows := importer.ParseLine(c.raw, c.lang)
		require.Len(t, rows, 1, c.raw)
		assert.Equal(t, c.name, rows[0].Name, c.raw)
		assert.True(t, rows[0].ToTaste, c.raw)
	}
	rows := importer.ParseLine("2 Tbsp olive oil ($0.32)", "en")
	require.Len(t, rows, 1)
	assert.Equal(t, "2 Tbsp olive oil", rows[0].RawText)
	assert.Equal(t, "olive oil", rows[0].Name)
	assert.Empty(t, rows[0].Note)
	assert.Equal(t, "Ghee", importer.StripPrice("Ghee (1,20 €)"))
	assert.Equal(t, "1 pinch of salt", importer.StripPrice("1 pinch of salt"))
}
