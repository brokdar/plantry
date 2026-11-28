package importer_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/crypto"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdc"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/fdclocal"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// TestParseAndMatch_CP3Run4Lines: the lines the paid CP3 run 4 left
// unmatched go through the real parse and match path without an FDC key.
func TestParseAndMatch_CP3Run4Lines(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	svc := settings.NewService(sqlite.NewAppSettingsRepo(db), settings.NewEnvSnapshot(nil), crypto.NilCipher{})
	m := food.NewMatcher(sqlite.NewFoodRepo(db), fdc.NewDynamicProvider(svc).WithFallback(fdclocal.New()), nil)

	tests := []struct {
		line     string
		name     string // parsed name contains
		minG     float64
		maxG     float64 // both 0: to taste
		toTaste  bool
		wantKcal bool
	}{
		{"2 cups water", "water", 470, 480, false, false},
		{"1 avocado", "avocado", 100, 300, false, true},
		{"2 tbsp tahini", "tahini", 25, 35, false, true},
		{"13oz. can coconut milk", "coconut milk", 365, 385, false, true},
		{"13.5oz can full-fat coconut milk", "coconut milk", 375, 395, false, true},
		{"1 15oz. can quartered artichoke hearts", "artichoke", 420, 430, false, true},
		{"1 (15 oz) can diced tomatoes", "tomato", 420, 430, false, true},
		{"2 lbs bone-in chicken thighs", "chicken thigh", 900, 910, false, true},
		{"1 inch fresh ginger", "ginger", 5, 20, false, true},
		{"2-inch piece ginger", "ginger", 10, 40, false, true},
		{"1 tsp italian seasoning", "italian seasoning", 0.5, 4, false, true},
		// CP4 finds that failed on these lines.
		{"1 package extra-firm tofu", "extra-firm tofu", 390, 400, false, true},
		{"2 servings steamed white rice", "steamed white rice", 310, 320, false, true},
		{"1 lb center-cut salmon fillets", "salmon fillets", 450, 460, false, true},
		{"1 tsp garam masala", "garam masala", 1, 4, false, true},
		{"2 portobello mushroom caps", "portobello", 20, 300, false, true},
		{"1 tbsp Mirin", "Mirin", 10, 25, false, true},
		{"2 tbsp green chiles", "green chiles", 20, 60, false, true},
		{"a few cranks freshly ground pepper", "pepper", 0, 0, true, true},
		{"3 cranks freshly ground pepper", "pepper", 0, 0, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			t.Parallel()
			rows := importer.ParseLine(tt.line, "en")
			require.Len(t, rows, 1, "%+v", rows)
			r := rows[0]
			assert.Contains(t, r.Name, tt.name)
			assert.Equal(t, tt.toTaste, r.ToTaste, "%+v", r)
			res, err := m.Match(ctx, food.MatchRequest{Lang: "en", Lines: []food.MatchLine{importer.MatchLineOf(r)}})
			require.NoError(t, err)
			l := res.Lines[0]
			require.Equal(t, food.MatchMatched, l.Status, "row %+v reason %s tried %v", r, l.Reason, l.Tried)
			if tt.toTaste {
				return
			}
			g := r.Amount
			switch r.Unit {
			case "g":
			case "ml", "cup", "tbsp", "tsp", "piece", "":
				if l.Grams != nil {
					g = *l.Grams
				}
			}
			t.Logf("row %+v grams %v", r, l.Grams)
			assert.GreaterOrEqual(t, g, tt.minG, "%+v", r)
			assert.LessOrEqual(t, g, tt.maxG, "%+v", r)
		})
	}
}

// A counted piece of tofu has no known weight; only a package does.
func TestParseLine_TofuPieceNeedsWeight(t *testing.T) {
	for _, line := range []string{"2 Stück Räuchertofu", "1 piece smoked tofu"} {
		got := importer.ParseLineEN(line)
		assert.True(t, got.NeedsWeight, line)
		assert.Nil(t, got.PortionGrams, line)
	}
	got := importer.ParseLineEN("1 package extra-firm tofu")
	require.NotNil(t, got.PortionGrams)
	assert.InDelta(t, 397, *got.PortionGrams, 0.01)
}
