package importer

import (
	"errors"
	"time"
)

// ErrNoRecipe indicates the extractor could not locate a Recipe node in the HTML.
var ErrNoRecipe = errors.New("no recipe found")

// RawRecipe is the normalized shape an extractor produces before the importer
// service turns it into a Draft. It intentionally mirrors schema.org Recipe
// fields so both the JSON-LD extractor and the LLM fallback can target it.
type RawRecipe struct {
	Name               string
	Description        string
	Image              []string
	RecipeYield        string
	RecipeYieldNumber  int
	TotalMinutes       int
	PrepMinutes        int
	CookMinutes        int
	RecipeIngredient   []string
	RecipeInstructions []string
	AuthorName         string
	Keywords           []string
	URL                string
	// SourceKcalPerServing is the site's own calorie claim (nutrition.calories).
	// Display/comparison only — never a source for macros.
	SourceKcalPerServing *float64
	// Rating is aggregateRating.ratingValue mapped onto 1..5 by the site's
	// worst and best rating (FIND-9); nil when the page has none.
	Rating *float64
	// RatingCount and ReviewCount are the site's vote and review counts.
	RatingCount int
	ReviewCount int
	// DatePublished is the recipe's publication date, nil when absent or unreadable.
	DatePublished *time.Time
}
