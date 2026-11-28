package importer_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
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

// Budget Bytes puts bare price tags on names: "onion $0.47".
func TestStripPrice_BareTags(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"1 onion $0.47":                  "1 onion",
		"8 oz cheddar cheese $1.72":      "8 oz cheddar cheese",
		"1 green bell pepper $0.78":      "1 green bell pepper",
		"1 cheddar $1.72, grated":        "1 cheddar, grated",
		"2 Tbsp Ghee 1,20 €":             "2 Tbsp Ghee",
		"1 onion €0.50 diced":            "1 onion diced",
		"1 tomato (€0.50)":               "1 tomato",
		"8 oz cheddar cheese\u00a0$1.72": "8 oz cheddar cheese",
		"8 oz cheddar cheese $1.72*":     "8 oz cheddar cheese",
		"8 oz cheddar cheese $1.72/oz":   "8 oz cheddar cheese",
		"2 cups rice":                    "2 cups rice",
		"1 can beans, 15 oz":             "1 can beans, 15 oz",
	} {
		assert.Equal(t, want, importer.StripPrice(in), in)
	}
}

func TestSettleQuantityless(t *testing.T) {
	t.Parallel()
	line := func(leaf string, mod func(*importer.DraftIngredient)) importer.DraftIngredient {
		ing := importer.DraftIngredient{Name: "nonstick cooking spray", Match: &food.LineMatch{
			Status: food.MatchMatched, Candidates: []food.Candidate{{Name: leaf}},
		}}
		if mod != nil {
			mod(&ing)
		}
		return ing
	}
	spray := line("cooking spray", nil)
	assert.True(t, importer.SettleQuantityless(&spray))
	assert.True(t, spray.ToTaste, "an unknown-group food is to taste")
	oil := line("olive oil", nil)
	assert.True(t, importer.SettleQuantityless(&oil))
	assert.True(t, oil.Proposed && oil.Amount == 1 && oil.Unit == "tbsp" && !oil.ToTaste, "a fat gets the proposed amount")
	chicken := line("chicken breast", nil)
	assert.False(t, importer.SettleQuantityless(&chicken), "a main food without amount stays blocked")
	ok := line("chicken breast", func(i *importer.DraftIngredient) { i.Amount, i.Unit = 2, "g" })
	assert.True(t, importer.SettleQuantityless(&ok))
}

// F3b: "grape or cherry tomatoes" is two tomatoes, not a food called grape.
func TestParseLine_OrAlternativeSharesHeadNoun(t *testing.T) {
	t.Parallel()
	rows := importer.ParseLine("2 cups grape or cherry tomatoes", "en")
	require.Len(t, rows, 1)
	assert.Equal(t, "grape tomatoes", rows[0].Name)
	assert.Equal(t, []string{"cherry tomatoes"}, rows[0].Alternatives)
	assert.Equal(t, "cheddar cheese", importer.ParseLine("8 oz cheddar cheese\u00a0$1.72", "en")[0].Name)
}

// F3b: "10-15 dashes hot sauce" is 12.5 pinch; that is no amount.
func TestSettlePinches(t *testing.T) {
	t.Parallel()
	hot := importer.ParseLine("10-15 dashes mild hot sauce", "en")[0]
	importer.SettlePinches(&hot)
	assert.True(t, hot.ToTaste, "%+v", hot)
	one := importer.ParseLine("1 pinch of salt", "en")[0]
	importer.SettlePinches(&one)
	assert.False(t, one.ToTaste)
}
