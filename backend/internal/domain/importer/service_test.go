package importer_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// -- fakes --

type fakeFetcher struct {
	called bool
	html   string
	final  string
	err    error
}

func (f *fakeFetcher) Fetch(_ context.Context, url string) (string, string, error) {
	f.called = true
	if f.err != nil {
		return "", "", f.err
	}
	final := f.final
	if final == "" {
		final = url
	}
	return f.html, final, nil
}

type fakeJSONLD struct {
	called bool
	rec    *importer.RawRecipe
	err    error
}

func (f *fakeJSONLD) ExtractRecipe(_ string) (*importer.RawRecipe, error) {
	f.called = true
	return f.rec, f.err
}

type fakeLLM struct {
	called int
	text   string
	err    error
}

func (f *fakeLLM) Stream(ctx context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	f.called++
	close(out)
	if f.err != nil {
		return nil, f.err
	}
	return &llm.Response{
		Message:    llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: f.text}}},
		StopReason: llm.StopReasonEndTurn,
	}, nil
}

type fakeResolver struct{}

func (fakeResolver) Lookup(_ context.Context, _, _, _ string, _ int) ([]ingredient.Candidate, error) {
	return nil, nil
}

// -- tests --

func TestExtract_ValidationFailsWithoutInputs(t *testing.T) {
	s := importer.NewService(&fakeFetcher{}, &fakeJSONLD{}, nil, "", fakeResolver{})
	_, err := s.Extract(context.Background(), importer.ExtractInput{})
	require.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestExtract_BothInputsIsInvalid(t *testing.T) {
	s := importer.NewService(&fakeFetcher{}, &fakeJSONLD{}, nil, "", fakeResolver{})
	_, err := s.Extract(context.Background(), importer.ExtractInput{URL: "u", HTML: "h"})
	require.True(t, errors.Is(err, domain.ErrInvalidInput))
}

func TestExtract_JSONLD_HappyPath_URLFetched(t *testing.T) {
	f := &fakeFetcher{html: "<html>…</html>", final: "https://example.com/final"}
	j := &fakeJSONLD{rec: &importer.RawRecipe{
		Name:               "Carbonara",
		RecipeYieldNumber:  4,
		RecipeIngredient:   []string{"400 g Spaghetti", "2 Zehen Knoblauch"},
		RecipeInstructions: []string{"Kochen."},
		PrepMinutes:        15,
	}}
	llmc := &fakeLLM{text: `{}`}
	s := importer.NewService(f, j, llmc, "fake-model", fakeResolver{})

	d, err := s.Extract(context.Background(), importer.ExtractInput{URL: "https://example.com/x"})
	require.NoError(t, err)
	require.True(t, f.called, "fetcher should be called for URL input")
	require.Equal(t, 0, llmc.called, "LLM must not be called when JSON-LD succeeds")
	require.Equal(t, "jsonld", d.ExtractMethod)
	require.Equal(t, "Carbonara", d.Name)
	require.Equal(t, "https://example.com/final", d.SourceURL)
	require.Equal(t, 4.0, d.ReferencePortions)
	require.Equal(t, 2, len(d.Ingredients))
	require.Equal(t, 400.0, d.Ingredients[0].Amount)
	require.Equal(t, "g", d.Ingredients[0].Unit)
}

func TestExtract_HTMLPath_SkipsFetcher(t *testing.T) {
	f := &fakeFetcher{}
	j := &fakeJSONLD{rec: &importer.RawRecipe{
		Name: "X", RecipeYieldNumber: 1,
		RecipeIngredient: []string{"100 g Mehl"},
	}}
	s := importer.NewService(f, j, nil, "", fakeResolver{})

	_, err := s.Extract(context.Background(), importer.ExtractInput{HTML: "<html/>"})
	require.NoError(t, err)
	require.False(t, f.called, "fetcher must not run when HTML is provided")
}

func TestExtract_NoRecipe_NoLLM_ReturnsNoRecipe(t *testing.T) {
	f := &fakeFetcher{html: "<html/>"}
	j := &fakeJSONLD{err: importer.ErrNoRecipe}
	s := importer.NewService(f, j, nil, "", fakeResolver{})

	_, err := s.Extract(context.Background(), importer.ExtractInput{URL: "u"})
	require.True(t, errors.Is(err, domain.ErrImportNoRecipe))
}

func TestExtract_EmptyIngredients_FallsBackToLLM(t *testing.T) {
	f := &fakeFetcher{html: "<html/>"}
	j := &fakeJSONLD{rec: &importer.RawRecipe{Name: "Empty"}}
	llmText := `{"name":"Via LLM","servings":2,"instructions":["Tu dies."],"ingredient_lines":["200 g Mehl"],"language":"de"}`
	llmc := &fakeLLM{text: llmText}
	s := importer.NewService(f, j, llmc, "fake-model", fakeResolver{})

	d, err := s.Extract(context.Background(), importer.ExtractInput{URL: "u"})
	require.NoError(t, err)
	require.Equal(t, 1, llmc.called)
	require.Equal(t, "llm", d.ExtractMethod)
	require.Equal(t, "Via LLM", d.Name)
	require.Len(t, d.Ingredients, 1)
	require.Equal(t, 200.0, d.Ingredients[0].Amount)
}

func TestExtract_NoRecipe_LLMDisabled_ReturnsAIProviderMissing(t *testing.T) {
	f := &fakeFetcher{html: "<html/>"}
	// jsonld found nothing; no LLM wired.
	j := &fakeJSONLD{rec: &importer.RawRecipe{}}
	s := importer.NewService(f, j, nil, "", fakeResolver{})

	_, err := s.Extract(context.Background(), importer.ExtractInput{URL: "u"})
	require.True(t, errors.Is(err, domain.ErrAIProviderMissing))
}

func TestExtract_LLMInvalidJSON_Retries_ThenFails(t *testing.T) {
	f := &fakeFetcher{html: "<html/>"}
	j := &fakeJSONLD{err: importer.ErrNoRecipe}
	llmc := &fakeLLM{text: "not json"}
	s := importer.NewService(f, j, llmc, "fake-model", fakeResolver{})

	_, err := s.Extract(context.Background(), importer.ExtractInput{URL: "u"})
	require.True(t, errors.Is(err, domain.ErrImportLLMFailed))
	require.Equal(t, 2, llmc.called, "should retry once")
}

func TestExtract_LLMNotARecipe(t *testing.T) {
	f := &fakeFetcher{html: "<html/>"}
	j := &fakeJSONLD{err: importer.ErrNoRecipe}
	llmc := &fakeLLM{text: `{"not_a_recipe": true}`}
	s := importer.NewService(f, j, llmc, "fake-model", fakeResolver{})

	_, err := s.Extract(context.Background(), importer.ExtractInput{URL: "u"})
	require.True(t, errors.Is(err, domain.ErrImportNoRecipe))
}

// dumpDraft prints the draft as JSON when PLANTRY_DUMP_DRAFT=1 so a human
// reviewer can eyeball the parser's output against the original page.
func dumpDraft(t *testing.T, d *importer.Draft) {
	t.Helper()
	if os.Getenv("PLANTRY_DUMP_DRAFT") == "" {
		return
	}
	b, _ := json.MarshalIndent(d, "", "  ")
	t.Logf("\n%s\n", string(b))
}

// TestExtract_Chefkoch_LasagnaReal runs a captured real chefkoch.de page
// through the full import pipeline. This is the canonical chefkoch regression
// guard — any change that degrades parsing of this page fails here.
func TestExtract_Chefkoch_LasagnaReal(t *testing.T) {
	html := loadChefkoch(t, "chefkoch_lasagna_real.html")
	f := &fakeFetcher{html: html}
	s := importer.NewService(f, jsonld.Extractor{}, nil, "", fakeResolver{})
	d, err := s.Extract(context.Background(), importer.ExtractInput{URL: "https://www.chefkoch.de/rezepte/1112181217260303/Lasagne-Bolognese.html"})
	require.NoError(t, err)

	require.Equal(t, "Lasagne Bolognese von chefkoch", d.Name)
	require.Equal(t, 4.0, d.ReferencePortions)
	require.NotNil(t, d.PrepMinutes)
	require.Equal(t, 60, *d.PrepMinutes)
	require.NotNil(t, d.CookMinutes)
	require.Equal(t, 60, *d.CookMinutes)
	// recipeCategory + recipeCuisine + keywords all merged.
	require.Contains(t, d.Tags, "Schwein")
	require.Contains(t, d.Tags, "Italien")
	require.Contains(t, d.Tags, "Pasta")
	// HowToSection nested steps flattened.
	require.GreaterOrEqual(t, len(d.Instructions), 4)
	require.Equal(t, 18, len(d.Ingredients))

	// Spot-check well-known lines.
	byRaw := map[string]importer.DraftIngredient{}
	for _, ing := range d.Ingredients {
		byRaw[ing.RawText] = ing
	}

	if ing, ok := byRaw["600 g Hackfleisch, gemischtes"]; ok {
		require.Equal(t, 600.0, ing.Amount)
		require.Equal(t, "g", ing.Unit)
		require.Equal(t, "Hackfleisch", ing.Name)
		require.Equal(t, "gemischtes", ing.Note)
	} else {
		t.Fatal("Hackfleisch line missing")
	}

	if ing, ok := byRaw["3 EL Olivenöl"]; ok {
		require.Equal(t, 45.0, ing.Amount)
		require.Equal(t, "ml", ing.Unit)
		require.Equal(t, "Olivenöl", ing.Name)
	} else {
		t.Fatal("Olivenöl line missing")
	}

	// Compound-word suffix match — "Knoblauchzehe(n)" must resolve to ~4 g per clove.
	if ing, ok := byRaw["2  Knoblauchzehe(n)"]; ok {
		require.InDelta(t, 8.0, ing.Amount, 0.01, "Knoblauchzehe(n) should be ~8g not 240g")
		require.Equal(t, "g", ing.Unit)
		require.Equal(t, "Zehe", ing.OriginalUnit)
	}

	if ing, ok := byRaw["200 ml Weißwein"]; ok {
		require.Equal(t, 200.0, ing.Amount)
		require.Equal(t, "ml", ing.Unit)
		require.Equal(t, "Weißwein", ing.Name)
	}

	// "3 EL, gestr. Mehl" — comma-before-name pattern. Name should NOT end up empty.
	if ing, ok := byRaw["3 EL, gestr. Mehl"]; ok {
		require.Equal(t, 45.0, ing.Amount)
		require.Equal(t, "ml", ing.Unit)
		require.NotEmpty(t, ing.Name, "name must not be empty after comma-note promotion")
	}

	// Pure-text lines ("Salz und Pfeffer", "Muskat", "Zucker", "Fett für die Form")
	// must still show up — unparsed is fine, empty-amount is fine.
	haveSalz := false
	for _, ing := range d.Ingredients {
		if strings.Contains(ing.Name, "Salz") {
			haveSalz = true
		}
	}
	require.True(t, haveSalz, "Salz/Pfeffer line missing from draft")

	dumpDraft(t, d)
}

// TestExtract_Chefkoch_BombayReal validates a spicier chefkoch page:
// decimal amounts with dot ("1.5 TL"), compound "Knoblauchzehe(n)",
// "2 m.-große Zwiebel(n) (gehackt)" with multiple parens, and a leading
// "etwas" qualifier on the garnish line.
func TestExtract_Chefkoch_BombayReal(t *testing.T) {
	html := loadChefkoch(t, "chefkoch_bombay_real.html")
	f := &fakeFetcher{html: html}
	s := importer.NewService(f, jsonld.Extractor{}, nil, "", fakeResolver{})
	d, err := s.Extract(context.Background(), importer.ExtractInput{URL: "https://www.chefkoch.de/rezepte/926401197908362/Bombay-Curry.html"})
	require.NoError(t, err)

	require.Equal(t, "Bombay-Curry von feuervogel", d.Name)
	require.Equal(t, 4.0, d.ReferencePortions)
	require.NotNil(t, d.PrepMinutes)
	require.Equal(t, 20, *d.PrepMinutes)
	require.NotNil(t, d.CookMinutes)
	require.Equal(t, 100, *d.CookMinutes) // 1h40m
	require.Contains(t, d.Tags, "Rind")
	require.Contains(t, d.Tags, "Asien")
	require.Equal(t, 14, len(d.Ingredients))

	byRaw := map[string]importer.DraftIngredient{}
	for _, ing := range d.Ingredients {
		byRaw[ing.RawText] = ing
	}

	// Dot decimal: "1.5 TL" → 7.5 ml
	if ing, ok := byRaw["1.5 TL Kurkumapulver"]; ok {
		require.InDelta(t, 7.5, ing.Amount, 0.001)
		require.Equal(t, "ml", ing.Unit)
		require.Equal(t, "Kurkumapulver", ing.Name)
	} else {
		t.Fatal("Kurkumapulver (1.5 TL) line missing")
	}

	// Compound unit + multiple parens
	if ing, ok := byRaw["2  Knoblauchzehe(n) (zerdrückt)"]; ok {
		require.InDelta(t, 8.0, ing.Amount, 0.01)
		require.Equal(t, "g", ing.Unit)
		require.Equal(t, "Zehe", ing.OriginalUnit)
		require.Contains(t, ing.Note, "zerdrückt")
	}

	// Dose (can) → 400g
	if ing, ok := byRaw["1 Dose Tomaten, geschälte (in Stücke geschnitten)"]; ok {
		require.Equal(t, 400.0, ing.Amount)
		require.Equal(t, "g", ing.Unit)
		require.Equal(t, "Dose", ing.OriginalUnit)
	}

	// "etwas" qualifier + paren
	haveKoriander := false
	for _, ing := range d.Ingredients {
		if strings.Contains(ing.Name, "Koriandergrün") {
			haveKoriander = true
			require.Equal(t, importer.ConfidenceUnparsed, ing.Confidence)
			require.Contains(t, ing.Note, "etwas")
		}
	}
	require.True(t, haveKoriander, "Koriandergrün line missing")

	dumpDraft(t, d)
}

// TestExtract_Chefkoch_HackauflaufReal covers the "1 Glas + long parenthetical",
// compound "Paprikaschote(n), rote (und grüne)" with comma + paren, and a
// bare-text "Salz und Pfeffer (frisch gemahlener)" row.
func TestExtract_Chefkoch_HackauflaufReal(t *testing.T) {
	html := loadChefkoch(t, "chefkoch_hackauflauf_real.html")
	f := &fakeFetcher{html: html}
	s := importer.NewService(f, jsonld.Extractor{}, nil, "", fakeResolver{})
	d, err := s.Extract(context.Background(), importer.ExtractInput{URL: "https://www.chefkoch.de/rezepte/1560161263457749/Tuerkischer-Hackfleischauflauf-mit-Schafskaese.html"})
	require.NoError(t, err)

	require.Equal(t, "Türkischer Hackfleischauflauf mit Schafskäse von nikalo", d.Name)
	require.Equal(t, 4.0, d.ReferencePortions)
	require.NotNil(t, d.PrepMinutes)
	require.Equal(t, 20, *d.PrepMinutes)
	require.Equal(t, 40, *d.CookMinutes)
	require.Contains(t, d.Tags, "Rind")
	require.Contains(t, d.Tags, "Türkei")
	require.Equal(t, 9, len(d.Ingredients))

	byRaw := map[string]importer.DraftIngredient{}
	for _, ing := range d.Ingredients {
		byRaw[ing.RawText] = ing
	}

	// 1 Glas → 200 g, long parenthetical captured as note
	if ing, ok := byRaw["1 Glas Schafskäse (Würfel in Öl, ca. 150 - 200 g)"]; ok {
		require.Equal(t, 200.0, ing.Amount)
		require.Equal(t, "g", ing.Unit)
		require.Equal(t, "Glas", ing.OriginalUnit)
		require.NotEmpty(t, ing.Note)
	}

	// 600 g + standard paren
	if ing, ok := byRaw["600 g Rinderhackfleisch (oder Lammhackfleisch)"]; ok {
		require.Equal(t, 600.0, ing.Amount)
		require.Equal(t, "g", ing.Unit)
	}

	// 500 g Champignons
	if ing, ok := byRaw["500 g Champignons (frische)"]; ok {
		require.Equal(t, 500.0, ing.Amount)
		require.Equal(t, "Champignons", ing.Name)
	}

	// Compound "Paprikaschote(n), rote (und grüne)" — doesn't match a suffix we
	// know, so it falls back to Stück default. Just assert it parsed with an
	// amount and didn't lose the name.
	if ing, ok := byRaw["2  Paprikaschote(n), rote (und grüne)"]; ok {
		require.Equal(t, 2.0*120.0, ing.Amount)
		require.Contains(t, ing.Name, "Paprikaschote")
	}

	// Compound Zehe must match
	if ing, ok := byRaw["2  Knoblauchzehe(n)"]; ok {
		require.InDelta(t, 8.0, ing.Amount, 0.01)
		require.Equal(t, "Zehe", ing.OriginalUnit)
	}

	// Bare-text with paren
	haveSalzRow := false
	for _, ing := range d.Ingredients {
		if strings.Contains(ing.Name, "Salz und Pfeffer") {
			haveSalzRow = true
			require.Equal(t, importer.ConfidenceUnparsed, ing.Confidence)
			require.Contains(t, ing.Note, "frisch gemahlener")
		}
	}
	require.True(t, haveSalzRow, "Salz und Pfeffer line missing")

	dumpDraft(t, d)
}

// Golden-file chefkoch integration: use the real jsonld extractor with a fake
// fetcher that serves the fixture HTML.
func TestExtract_Chefkoch_Fixtures(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		expect  func(t *testing.T, d *importer.Draft)
	}{
		{
			name:    "carbonara",
			fixture: "chefkoch_spaghetti_carbonara.html",
			expect: func(t *testing.T, d *importer.Draft) {
				require.Equal(t, "Spaghetti Carbonara", d.Name)
				require.Equal(t, "de", d.Language)
				require.Equal(t, 4.0, d.ReferencePortions)
				require.Len(t, d.Ingredients, 7)
				// All ingredient lines must be parsed with known unit — only the
				// "nach Geschmack Pfeffer" entry is legitimately unparsed.
				unparsed := 0
				for _, ing := range d.Ingredients {
					if ing.Confidence == importer.ConfidenceUnparsed {
						unparsed++
					}
				}
				require.Equal(t, 1, unparsed, "only 'nach Geschmack Pfeffer' should be unparsed")
				require.Equal(t, 400.0, d.Ingredients[0].Amount)
				require.Equal(t, "g", d.Ingredients[0].Unit)
			},
		},
		{
			name:    "knoblauchbrot",
			fixture: "chefkoch_knoblauchbrot.html",
			expect: func(t *testing.T, d *importer.Draft) {
				require.Equal(t, "Knoblauchbrot", d.Name)
				require.Len(t, d.Ingredients, 5)
				require.Len(t, d.Instructions, 4)
				// ½ Bund Petersilie must parse.
				require.Equal(t, 15.0, d.Ingredients[3].Amount)
				require.Equal(t, "g", d.Ingredients[3].Unit)
			},
		},
		{
			name:    "risotto",
			fixture: "chefkoch_risotto.html",
			expect: func(t *testing.T, d *importer.Draft) {
				require.Equal(t, "Risotto alla Milanese", d.Name)
				require.Len(t, d.Ingredients, 9)
				// 1 l Gemüsebrühe must become 1000 ml.
				require.Equal(t, 1000.0, d.Ingredients[1].Amount)
				require.Equal(t, "ml", d.Ingredients[1].Unit)
				// 2 EL Olivenöl must become 30 ml, approximate.
				require.Equal(t, 30.0, d.Ingredients[3].Amount)
				require.Equal(t, importer.ConfidenceApproximate, d.Ingredients[3].Confidence)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := loadChefkoch(t, tc.fixture)
			f := &fakeFetcher{html: html}
			s := importer.NewService(f, jsonld.Extractor{}, nil, "", fakeResolver{})
			d, err := s.Extract(context.Background(), importer.ExtractInput{URL: "https://chefkoch.de/r/1"})
			require.NoError(t, err)
			tc.expect(t, d)
		})
	}
}

func loadChefkoch(t *testing.T, name string) string {
	t.Helper()
	// Fixtures live under adapters/jsonld/testdata/ — reuse them.
	b, err := os.ReadFile(filepath.Join("..", "..", "adapters", "jsonld", "testdata", name))
	require.NoError(t, err)
	return string(b)
}
