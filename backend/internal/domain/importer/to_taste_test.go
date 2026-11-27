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
