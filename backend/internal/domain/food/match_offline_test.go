package food_test

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
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// TestMatch_OfflineCatalogueWithoutFDCKey: without an FDC key the common
// foods the CP3 trials and the C2 canary left unmatched (P3) match a generic
// FDC food with nutrition from the embedded catalogue; nonsense stays
// unmatched.
func TestMatch_OfflineCatalogueWithoutFDCKey(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	svc := settings.NewService(sqlite.NewAppSettingsRepo(db), settings.NewEnvSnapshot(nil), crypto.NilCipher{})
	provider := fdc.NewDynamicProvider(svc).WithFallback(fdclocal.New())
	m := food.NewMatcher(repo, provider, nil)

	tests := []struct {
		lang   string
		line   food.MatchLine
		fdcID  int // 0: unmatched
		reason string
	}{
		{"en", food.MatchLine{Name: "apple cider vinegar", Amount: 2, Unit: "tbsp"}, 173469, ""},
		{"en", food.MatchLine{Name: "Worcestershire sauce", Amount: 1, Unit: "tbsp"}, 171610, ""},
		{"en", food.MatchLine{Name: "French bread", Amount: 1, Unit: "piece"}, 172675, ""},
		{"en", food.MatchLine{Name: "cayenne pepper", Amount: 0.5, Unit: "tsp"}, 170932, ""},
		{"en", food.MatchLine{Name: "rice vinegar", Amount: 1, Unit: "tbsp"}, 172237, ""},
		{"en", food.MatchLine{Name: "red wine vinegar", Amount: 1, Unit: "tbsp"}, 172240, ""},
		{"en", food.MatchLine{Name: "ground pork", Amount: 500, Unit: "g"}, 2514745, ""},
		{"en", food.MatchLine{Name: "oyster sauce", Amount: 2, Unit: "tbsp"}, 174529, ""},
		{"en", food.MatchLine{Name: "black beans", Amount: 400, Unit: "g"}, 175188, ""},
		{"en", food.MatchLine{Name: "brown lentils", Amount: 200, Unit: "g"}, 2644283, ""},
		// The Japanese lines of the CP3b theme week (mirin has no FDC food).
		{"en", food.MatchLine{Name: "dashi stock", Amount: 1, Unit: "cup"}, 172885, ""},
		{"en", food.MatchLine{Name: "nori sheets", Amount: 2, Unit: "piece"}, 168458, ""},
		{"en", food.MatchLine{Name: "kombu", Amount: 1, Unit: "piece"}, 168457, ""},
		{"en", food.MatchLine{Name: "wakame", Amount: 1, Unit: "tbsp"}, 170496, ""},
		{"en", food.MatchLine{Name: "sriracha", Amount: 1, Unit: "tbsp"}, 171186, ""},
		{"en", food.MatchLine{Name: "gyoza or wonton wrappers", Amount: 50, Unit: "piece"}, 172802, ""},
		{"en", food.MatchLine{Name: "daikon", Amount: 200, Unit: "g"}, 168451, ""},
		{"en", food.MatchLine{Name: "shiitake mushrooms", Amount: 100, Unit: "g"}, 169242, ""},
		{"de", food.MatchLine{Name: "Lauch", Amount: 1, Unit: "Stück"}, 169246, ""},
		{"de", food.MatchLine{Name: "Kokosmilch", Amount: 400, Unit: "ml"}, 170173, ""},
		{"de", food.MatchLine{Name: "Knollensellerie", Amount: 300, Unit: "g"}, 170400, ""},
		{"de", food.MatchLine{Name: "Kurkumapulver", Amount: 1, Unit: "TL"}, 172231, ""},
		// R4-fixes: lines the CP3 verdict left unmatched. Mirin and adobo
		// seasoning have no USDA food: the closest real one stands in.
		// CP4 lines: garam masala has no USDA food; curry powder, a ground
		// spice blend, is the closest real one.
		{"en", food.MatchLine{Name: "garam masala", Amount: 1, Unit: "tsp"}, 170924, ""},
		{"en", food.MatchLine{Name: "Mirin", Amount: 1, Unit: "tbsp"}, 167723, ""},
		{"en", food.MatchLine{Name: "portobello mushroom caps", Amount: 200, Unit: "g"}, 169255, ""},
		{"en", food.MatchLine{Name: "green chiles", Amount: 4, Unit: "oz"}, 168577, ""},
		{"en", food.MatchLine{Name: "mirin", Amount: 2, Unit: "tbsp"}, 167723, ""},
		{"en", food.MatchLine{Name: "adobo seasoning", Amount: 1, Unit: "tsp"}, 171328, ""},
		{"en", food.MatchLine{Name: "adobo sauce", Amount: 1, Unit: "tbsp"}, 171604, ""},
		{"en", food.MatchLine{Name: "chipotle peppers in adobo sauce", Amount: 30, Unit: "g"}, 171604, ""},
		{"en", food.MatchLine{Name: "diced green chiles", Amount: 4, Unit: "oz"}, 168577, ""},
		{"en", food.MatchLine{Name: "canned green chilies", Amount: 1, Unit: "tbsp"}, 168577, ""},
		{"en", food.MatchLine{Name: "glorbnax fizzwhip", Amount: 100, Unit: "g"}, 0, food.ReasonNoCandidate},
	}
	for _, tt := range tests {
		t.Run(tt.line.Name, func(t *testing.T) {
			t.Parallel()
			res, err := m.Match(ctx, food.MatchRequest{Lang: tt.lang, Lines: []food.MatchLine{tt.line}})
			require.NoError(t, err)
			l := res.Lines[0]
			if tt.fdcID == 0 {
				assert.Equal(t, food.MatchUnmatched, l.Status)
				assert.Equal(t, tt.reason, l.Reason)
				return
			}
			require.Equal(t, food.MatchMatched, l.Status, "reason %s, tried %v, candidates %v", l.Reason, l.Tried, l.Candidates)
			c := l.Candidates[l.Pick]
			assert.Equal(t, tt.fdcID, c.FdcID, c.Name)
			assert.Equal(t, food.SourceFDC, c.Source)
			require.NotNil(t, c.Kcal100g)
			assert.NotNil(t, c.Protein100g)
		})
	}
}

// TestEnsureLeaf_OfflineCandidateReusesLeafWithSameFdcID: a candidate from
// the offline catalogue is the leaf that already carries its FDC id.
func TestEnsureLeaf_OfflineCandidateReusesLeafWithSameFdcID(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewFoodRepo(db)
	origins := sqlite.NewFoodOriginRepo(db)
	src, ref := food.SourceFDC, "171610"
	existing := &food.Food{Name: "Worcester (Lea & Perrins)", Kind: food.KindLeaf, Source: &src, FdcID: &ref, Kcal100g: kcal(78)}
	require.NoError(t, repo.Create(ctx, existing))

	c, err := fdclocal.New().GetByID(ctx, 171610)
	require.NoError(t, err)
	id, created, err := food.EnsureLeaf(ctx, repo, origins, c, food.Origin{Kind: food.OriginImport, Ref: "https://example.org/r"})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, existing.ID, id)
}

// TestMatch_StrictEntryNeedsOwnWords: a strict synonym entry whose FDC term
// names another food (ras el hanout -> "spices cumin seed") is a review
// candidate, not a confident match.
func TestMatch_StrictEntryNeedsOwnWords(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	provider := fdc.NewDynamicProvider(settings.NewService(sqlite.NewAppSettingsRepo(db), settings.NewEnvSnapshot(nil), crypto.NilCipher{})).WithFallback(fdclocal.New())
	m := food.NewMatcher(sqlite.NewFoodRepo(db), provider, nil)

	res, err := m.Match(ctx, food.MatchRequest{Lang: "en", Lines: []food.MatchLine{{Name: "ras el hanout", Amount: 1, Unit: "tsp"}}})
	require.NoError(t, err)
	assert.NotEqual(t, food.MatchMatched, res.Lines[0].Status, "%v", res.Lines[0].Candidates)
}
