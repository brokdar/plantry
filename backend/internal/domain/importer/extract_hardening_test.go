package importer_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

type stubFetcher struct {
	final string
	err   error
}

func (f stubFetcher) Fetch(_ context.Context, u string) (string, string, error) {
	if f.err != nil {
		return "", "", f.err
	}
	if f.final == "" {
		return "<html></html>", u, nil
	}
	return "<html></html>", f.final, nil
}

// noProvider is a resolver as main wires it: present, but with no provider
// configured.
type noProvider struct{}

func (noProvider) Current(context.Context) (llm.Client, string, error) {
	return nil, "", llm.ErrProviderMissing
}

type noRecipeExtractor struct{}

func (noRecipeExtractor) ExtractRecipe(string) (*importer.RawRecipe, error) {
	return nil, importer.ErrNoRecipe
}

// F7: a page without a recipe and no assistant is a page problem ("paste the
// text"), not "AI provider is not configured".
func TestExtract_NoRecipeWithoutAI(t *testing.T) {
	for name, in := range map[string]importer.ExtractInput{
		"url":  {URL: "https://www.lecker.de/rezepte"},
		"html": {HTML: "<html><body>hi</body></html>"},
	} {
		t.Run(name, func(t *testing.T) {
			svc := importer.NewService(stubFetcher{}, noRecipeExtractor{}, noProvider{}, nil, nil)
			_, err := svc.Extract(context.Background(), in)
			require.ErrorIs(t, err, domain.ErrImportNoRecipeNoAI)
			var xe *importer.ExtractError
			require.True(t, errors.As(err, &xe))
			assert.Equal(t, importer.StageParse, xe.Stage)
		})
	}
}

func TestExtract_FetchErrorsCarryHostAndStage(t *testing.T) {
	cases := []struct {
		name, url string
		fetchErr  error
		want      error
		host      string
	}{
		{"invalid url", "hello", nil, domain.ErrImportInvalidURL, ""},
		{"ftp", "ftp://x.y/z", nil, domain.ErrImportInvalidURL, ""},
		{"not found", "https://www.example.com/x", domain.ErrImportNotFound, domain.ErrImportNotFound, "www.example.com"},
		{"blocked", "https://chefkoch.de/x", domain.ErrImportBlocked, domain.ErrImportBlocked, "chefkoch.de"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := importer.NewService(stubFetcher{err: tc.fetchErr}, noRecipeExtractor{}, noProvider{}, nil, nil)
			_, err := svc.Extract(context.Background(), importer.ExtractInput{URL: tc.url})
			require.ErrorIs(t, err, tc.want)
			var xe *importer.ExtractError
			require.True(t, errors.As(err, &xe))
			assert.Equal(t, importer.StageFetch, xe.Stage)
			assert.Equal(t, tc.host, xe.Host)
		})
	}
}

// F7: gutekueche redirects a wrong slug with a valid id to another recipe.
func TestExtract_WarnsWhenRedirectedToAnotherRecipe(t *testing.T) {
	rec := &importer.RawRecipe{Name: "Dinkel-Muffins", RecipeIngredient: []string{"200 g Dinkelmehl"}}
	cases := []struct {
		name, final string
		warn        bool
	}{
		{"other slug", "https://www.gutekueche.at/dinkel-muffins-rezept-2050", true},
		{"same page", "https://www.gutekueche.at/spaghetti-carbonara-rezept-2050", false},
		{"trailing slash", "https://www.gutekueche.at/spaghetti-carbonara-rezept-2050/", false},
		{"similar slug", "https://www.gutekueche.at/spaghetti-carbonara-original-rezept-2050", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := importer.NewService(stubFetcher{final: tc.final}, stubExtractor{rec: rec}, nil, nil, nil)
			d, err := svc.Extract(context.Background(),
				importer.ExtractInput{URL: "https://www.gutekueche.at/spaghetti-carbonara-rezept-2050"})
			require.NoError(t, err)
			assert.Equal(t, tc.warn, slices.Contains(d.Warnings, importer.WarningRedirectedOtherRecipe))
			assert.Equal(t, tc.final, d.SourceURL)
		})
	}
}

// F5, F14, F15: names, tags, markup and missing times.
func TestExtract_CleansTheDraft(t *testing.T) {
	rec := &importer.RawRecipe{
		Name:             "Zitronenlachs Rezept",
		Description:      "Schnell &amp; <em>gut</em>",
		AuthorName:       "Anna Glover",
		Keywords:         []string{"Lachs", "Anna Glover", "1 of 5-a-day", "Chili con carne Rezepte"},
		RecipeIngredient: []string{"200 g Lachs"},
		RecipeInstructions: []string{
			"Für die <strong>Dinkelmuffins</strong> zuerst die Margarine schmelzen.<br>Dann 20 Min backen.",
			"Weitere 6-7 Stunden ruhen lassen.",
		},
	}
	svc := importer.NewService(nil, stubExtractor{rec: rec}, nil, nil, nil)
	d, err := svc.Extract(context.Background(), importer.ExtractInput{HTML: "<html></html>"})
	require.NoError(t, err)
	assert.Equal(t, "Zitronenlachs", d.Name)
	assert.Equal(t, "Schnell & gut", d.Description)
	assert.Equal(t, []string{"Lachs"}, d.Tags)
	assert.Equal(t, "Für die Dinkelmuffins zuerst die Margarine schmelzen. Dann 20 Min backen.", d.Instructions[0])
	require.NotNil(t, d.CookMinutes)
	assert.Equal(t, 20+7*60, *d.CookMinutes)
	assert.Contains(t, d.Warnings, importer.WarningTimesFromSteps)
}

func TestExtract_RecipeNameCredits(t *testing.T) {
	cases := map[string]string{
		"Tomatensalat mit Rauke und Parmesan Rezept":                        "Tomatensalat mit Rauke und Parmesan",
		"Der perfekte Pfannkuchen - gelingt einfach immer von zwergenmuomi": "Der perfekte Pfannkuchen",
		"Spaghetti Carbonara": "Spaghetti Carbonara",
	}
	for in, want := range cases {
		svc := importer.NewService(nil, stubExtractor{rec: &importer.RawRecipe{Name: in, RecipeIngredient: []string{"1 Ei"}}}, nil, nil, nil)
		d, err := svc.Extract(context.Background(), importer.ExtractInput{HTML: "<html></html>"})
		require.NoError(t, err)
		assert.Equal(t, want, d.Name)
	}
}

func TestExtract_KeepsJSONLDTimes(t *testing.T) {
	rec := &importer.RawRecipe{
		Name: "X", RecipeIngredient: []string{"1 Ei"}, CookMinutes: 30,
		RecipeInstructions: []string{"Backen, 99 Min."},
	}
	svc := importer.NewService(nil, stubExtractor{rec: rec}, nil, nil, nil)
	d, err := svc.Extract(context.Background(), importer.ExtractInput{HTML: "<html></html>"})
	require.NoError(t, err)
	assert.Equal(t, 30, *d.CookMinutes)
	assert.Empty(t, d.Warnings)
}
