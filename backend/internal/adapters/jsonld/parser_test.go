package jsonld_test

import (
	"errors"
	"fmt"
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

func TestExtractRecipe_YieldShapes(t *testing.T) {
	cases := []struct {
		name       string
		yield      string
		wantYield  string
		wantNumber int
	}{
		{"number", `4`, "4", 4},
		{"float number", `2.0`, "2", 2},
		{"array of strings", `["4", "4 Portionen"]`, "4", 4},
		{"array skips unparsable", `["Portionen", "6 Stück"]`, "6 Stück", 6},
		{"array of numbers", `[3]`, "3", 3},
		{"string", `"5 Portionen"`, "5 Portionen", 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, err := jsonld.ExtractRecipe(recipeHTML(`"recipeYield": ` + tc.yield))
			require.NoError(t, err)
			require.Equal(t, tc.wantYield, rec.RecipeYield)
			require.Equal(t, tc.wantNumber, rec.RecipeYieldNumber)
		})
	}
}

func TestExtractRecipe_NutritionCalories(t *testing.T) {
	kcal := func(v float64) *float64 { return &v }
	cases := []struct {
		name      string
		nutrition string
		want      *float64
	}{
		{"kcal string", `{"@type": "NutritionInformation", "calories": "520 kcal"}`, kcal(520)},
		{"decimal comma", `{"calories": "412,5 kcal"}`, kcal(412.5)},
		{"number", `{"calories": 300}`, kcal(300)},
		{"thousands dot", `{"calories": "2.000 kcal"}`, kcal(2000)},
		{"thousands dot at end", `{"calories": "1.250"}`, kcal(1250)},
		{"decimal dot", `{"calories": "520.5 kcal"}`, kcal(520.5)},
		{"kJ only", `{"calories": "2176 kJ"}`, nil},
		{"kJ and kcal", `{"calories": "520 kcal / 2176 kJ"}`, kcal(520)},
		{"no calories", `{"proteinContent": "20 g"}`, nil},
		{"garbage", `{"calories": "viel"}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, err := jsonld.ExtractRecipe(recipeHTML(`"nutrition": ` + tc.nutrition))
			require.NoError(t, err)
			require.Equal(t, tc.want, rec.SourceKcalPerServing)
		})
	}

	t.Run("absent", func(t *testing.T) {
		rec, err := jsonld.ExtractRecipe(recipeHTML(`"recipeYield": "4"`))
		require.NoError(t, err)
		require.Nil(t, rec.SourceKcalPerServing)
	})
}

// recipeHTML wraps one extra JSON field in a minimal Recipe JSON-LD page.
func recipeHTML(field string) string {
	return `<html><head><script type="application/ld+json">{"@type": "Recipe", "name": "Test", "recipeIngredient": ["100 g Mehl"], ` +
		field + `}</script></head></html>`
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

func TestExtractRecipe_OGImageFallback(t *testing.T) {
	page := func(meta, image string) string {
		return `<html><head>` + meta + `<script type="application/ld+json">{"@type":"Recipe","name":"X",` + image +
			`"recipeIngredient":["1 Ei"]}</script></head></html>`
	}
	tests := []struct {
		name string
		html string
		want []string
	}{
		{"og:image when JSON-LD has none", page(`<meta property="og:image" content="https://x.test/a.jpg">`, ""), []string{"https://x.test/a.jpg"}},
		{"content before property, single quotes, entities", page(`<meta content='https://x.test/a.jpg?w=1&amp;h=2' property='og:image'>`, ""), []string{"https://x.test/a.jpg?w=1&h=2"}},
		{"JSON-LD image wins", page(`<meta property="og:image" content="https://x.test/og.jpg">`, `"image":"https://x.test/ld.jpg",`), []string{"https://x.test/ld.jpg"}},
		{"neither", page(`<meta property="og:title" content="t">`, ""), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := jsonld.ExtractRecipe(tt.html)
			require.NoError(t, err)
			require.Equal(t, tt.want, rec.Image)
		})
	}
}

// FIND-9 on the recorded find pages: rating mapped onto 1..5 by the site's
// best and worst rating, counts and publication date.
func TestExtractRecipe_FindPageRatings(t *testing.T) {
	tests := []struct {
		file             string
		rating           float64
		ratings, reviews int
		published        string
	}{
		{"en_chicken_curry_1.html", 4.8, 2310, 412, "2021-03-14"},
		{"en_chicken_curry_2.html", 4.5, 880, 120, "2022-06-02"}, // explicit best 5, worst 1; RFC 3339 date
		{"en_chicken_curry_3.html", 4.68, 37, 0, "2019-11-20"},   // 92 of 0..100
		{"en_banana_bread_3.html", 4.44, 61, 9, "2018-09-30"},    // 8.6 of 0..10
		{"de_linsensuppe_1.html", 4.7, 1840, 260, "2020-10-05"},  // decimal comma
		{"de_gulasch_3.html", 4.52, 150, 0, "2016-05-17"},        // 88 of 0..100; date-time without zone
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			rec, err := jsonld.ExtractRecipe(loadFixture(t, filepath.Join("finds", tt.file)))
			require.NoError(t, err)
			require.NotNil(t, rec.Rating)
			require.InDelta(t, tt.rating, *rec.Rating, 1e-9)
			require.Equal(t, tt.ratings, rec.RatingCount)
			require.Equal(t, tt.reviews, rec.ReviewCount)
			require.NotNil(t, rec.DatePublished)
			require.Equal(t, tt.published, rec.DatePublished.Format("2006-01-02"))
			require.NotEmpty(t, rec.AuthorName)
		})
	}
}

func TestExtractRecipe_NoRating(t *testing.T) {
	rec, err := jsonld.ExtractRecipe(loadFixture(t, "chefkoch_knoblauchbrot.html"))
	require.NoError(t, err)
	require.Nil(t, rec.Rating)
	require.Zero(t, rec.RatingCount)
	require.Nil(t, rec.DatePublished)
}

// Every dish has three versions per language that extract as recipes.
func TestFindPages_ThreeVersionsPerDish(t *testing.T) {
	for _, dish := range []string{"en_chicken_curry", "en_banana_bread", "de_linsensuppe", "de_gulasch"} {
		names := map[string]bool{}
		for i := 1; i <= 3; i++ {
			rec, err := jsonld.ExtractRecipe(loadFixture(t, filepath.Join("finds", fmt.Sprintf("%s_%d.html", dish, i))))
			require.NoError(t, err)
			require.NotEmpty(t, rec.RecipeIngredient)
			names[rec.AuthorName] = true
		}
		require.Len(t, names, 3, dish)
	}
}
