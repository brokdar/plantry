package jsonld_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
)

func TestExtractRecipe_ChefkochSpaghettiCarbonara(t *testing.T) {
	r := loadFixture(t, "chefkoch_spaghetti_carbonara.html")

	rec, err := jsonld.ExtractRecipe(r)
	require.NoError(t, err)

	require.Equal(t, "Spaghetti Carbonara", rec.Name)
	require.Equal(t, "4 Portionen", rec.RecipeYield)
	require.Equal(t, 4, rec.RecipeYieldNumber)
	require.Equal(t, 15, rec.PrepMinutes)
	require.Equal(t, 20, rec.CookMinutes)
	require.Equal(t, 35, rec.TotalMinutes)
	// recipeCategory + recipeCuisine + keywords merged, deduped while preserving order.
	require.Equal(t, []string{"Hauptspeise", "Italienisch", "Pasta", "Nudeln", "Speck"}, rec.Keywords)
	require.Equal(t, "HobbykochXY", rec.AuthorName)
	require.Equal(t, []string{"https://img.chefkoch-cdn.de/rezepte/1070041210498204/bilder/1451029/crop-600x400/spaghetti-carbonara.jpg"}, rec.Image)
	require.Equal(t, []string{
		"400 g Spaghetti",
		"200 g Pancetta oder durchwachsener Speck",
		"4 Eier",
		"100 g Parmesan, gerieben",
		"2 Zehen Knoblauch, fein gehackt",
		"1 Prise Salz",
		"nach Geschmack Pfeffer",
	}, rec.RecipeIngredient)
	require.Len(t, rec.RecipeInstructions, 5)
	require.Contains(t, rec.RecipeInstructions[0], "Wasser in einem großen Topf")
}

func TestExtractRecipe_ChefkochKnoblauchbrot_TypeArray(t *testing.T) {
	// @type is ["Recipe","NewsArticle"] — must still match.
	// recipeInstructions is a single string with blank-line paragraphs.
	r := loadFixture(t, "chefkoch_knoblauchbrot.html")

	rec, err := jsonld.ExtractRecipe(r)
	require.NoError(t, err)

	require.Equal(t, "Knoblauchbrot", rec.Name)
	require.Equal(t, 6, rec.RecipeYieldNumber)
	require.Equal(t, []string{"Backen", "Italienisch", "Beilage"}, rec.Keywords)
	require.Equal(t, "Chefkoch-Team", rec.AuthorName)
	require.Len(t, rec.RecipeIngredient, 5)
	require.Equal(t, "½ Bund Petersilie", rec.RecipeIngredient[3])
	require.Len(t, rec.RecipeInstructions, 4)
	require.Contains(t, rec.RecipeInstructions[0], "Ofen auf 200")
}

func TestExtractRecipe_ChefkochRisotto_GraphWrapped(t *testing.T) {
	// Recipe lives inside @graph array alongside WebPage and BreadcrumbList.
	r := loadFixture(t, "chefkoch_risotto.html")

	rec, err := jsonld.ExtractRecipe(r)
	require.NoError(t, err)

	require.Equal(t, "Risotto alla Milanese", rec.Name)
	require.Equal(t, 4, rec.RecipeYieldNumber)
	require.Equal(t, 10, rec.PrepMinutes)
	require.Equal(t, 25, rec.CookMinutes)
	require.Len(t, rec.Image, 1)
	require.Contains(t, rec.Image[0], "risotto.jpg")
	require.Equal(t, "ItalianoCook", rec.AuthorName)
	require.Len(t, rec.RecipeIngredient, 9)
	require.Len(t, rec.RecipeInstructions, 5)
}

func TestExtractRecipe_KitchenStoriesGeneric(t *testing.T) {
	r := loadFixture(t, "kitchenstories_generic.html")

	rec, err := jsonld.ExtractRecipe(r)
	require.NoError(t, err)

	require.Equal(t, "Chocolate Cake", rec.Name)
	require.Equal(t, 8, rec.RecipeYieldNumber)
	require.Equal(t, 60, rec.TotalMinutes)
	require.Len(t, rec.RecipeInstructions, 3)
}

func TestExtractRecipe_EatThis_HowToSection(t *testing.T) {
	r := loadFixture(t, "eat_this_recipe.html")

	rec, err := jsonld.ExtractRecipe(r)
	require.NoError(t, err)

	require.Equal(t, "Gemüsepfanne", rec.Name)
	// HowToSection should flatten into 4 individual steps.
	require.Equal(t, []string{
		"Gemüse waschen.",
		"Alles in gleichmäßige Stücke schneiden.",
		"Öl in der Pfanne erhitzen.",
		"Gemüse braten und abschmecken.",
	}, rec.RecipeInstructions)
}

func TestExtractRecipe_NoJSONLD(t *testing.T) {
	r := loadFixture(t, "no_jsonld.html")

	_, err := jsonld.ExtractRecipe(r)
	require.True(t, errors.Is(err, jsonld.ErrNoRecipe), "want ErrNoRecipe, got %v", err)
}

func TestParseISODuration(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"PT30M", 30, true},
		{"PT1H", 60, true},
		{"PT1H30M", 90, true},
		{"PT2H15M", 135, true},
		{"PT45S", 0, true},
		// Chefkoch-specific: verbose form with D prefix.
		{"P0DT0H10M", 10, true},
		{"P0DT1H30M", 90, true},
		{"P1D", 24 * 60, true},
		{"", 0, false},
		{"invalid", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := jsonld.ParseISODuration(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok: got %v want %v", ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestParseYield(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"4 Portionen", 4},
		{"für 4 Personen", 4},
		{"8 slices", 8},
		{"4", 4},
		{"", 1},
		{"nichts", 1},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := jsonld.ParseYield(tc.in); got != tc.want {
				t.Errorf("got %d want %d", got, tc.want)
			}
		})
	}
}

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return string(b)
}
