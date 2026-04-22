package importer

import "errors"

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
}
