package importer

import (
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// Confidence levels for a parsed ingredient line.
const (
	ConfidenceParsed      = "parsed"      // exact unit (g, kg, mg, ml, l, cl, dl).
	ConfidenceApproximate = "approximate" // conventional or count unit (EL, Prise, Zehe, Stück…).
	ConfidenceUnparsed    = "unparsed"    // could not determine amount or unit.
)

// DraftIngredient is one row in the review step of the import wizard.
type DraftIngredient struct {
	RawText      string  `json:"raw_text"`
	Amount       float64 `json:"amount"`
	Unit         string  `json:"unit"`          // "g", "ml", a canonical count unit ("piece", "clove"), or "" when unparsed.
	OriginalUnit string  `json:"original_unit"` // user-visible original (e.g. "EL", "Zehe").
	Name         string  `json:"name"`
	Note         string  `json:"note"`
	Confidence   string  `json:"confidence"`
	// PortionGrams is the estimated grams per count unit from
	// units.EstimateCountGrams ("1 clove ≈ 3 g"); nil for mass, volume and
	// unparsed lines. A leaf's own portion overrides it on Finalize.
	PortionGrams *float64 `json:"portion_grams"`
	// NeedsWeight marks a count line nobody can estimate ("1 piece of
	// Fantasiefrucht"): PortionGrams is nil and the review needs a weight
	// before Save.
	NeedsWeight bool `json:"needs_weight"`
	// Warning is a code the review translates ("suspicious_unit"), or "".
	Warning string `json:"warning"`
	// DietClass is the line's diet class from its name (diet.Classify), shown
	// read-only on review (D30).
	DietClass diet.Class `json:"diet_class"`
	// Alternatives are the other foods the line offers ("oder Butterschmalz").
	Alternatives []string `json:"alternatives"`
	// ToTaste marks a seasoning line without an amount ("Salz", "nach
	// Geschmack"; MATCH-6: salt, pepper, dried spices, fresh herbs).
	ToTaste bool `json:"to_taste"`
	// Proposed: the line had no amount and Amount/Unit are the rule-based
	// proposal (MATCH-6), shown as proposed in the review.
	Proposed bool `json:"proposed"`
	// Match is the line's match (MATCH-2): status, reason, what was tried,
	// candidates and the pick. nil when the draft was not matched.
	Match *food.LineMatch `json:"match"`
}

// Draft is the whole recipe as extracted from a page, before the user resolves it.
type Draft struct {
	Name              string            `json:"name"`
	Description       string            `json:"description"`
	SourceURL         string            `json:"source_url"`
	ImageURL          string            `json:"image_url"`
	PrepMinutes       *int              `json:"prep_minutes"`
	CookMinutes       *int              `json:"cook_minutes"`
	TotalMinutes      *int              `json:"total_minutes"`
	ReferencePortions float64           `json:"reference_portions"`
	Instructions      []string          `json:"instructions"`
	Ingredients       []DraftIngredient `json:"ingredients"`
	Tags              []string          `json:"tags"`
	Language          string            `json:"language"`       // "de" | "en" | "unknown".
	ExtractMethod     string            `json:"extract_method"` // "jsonld" | "llm".
	Warnings          []string          `json:"warnings"`
	// SourceKcalPerServing is the source page's calorie claim, for display and
	// comparison only — never used for macros.
	SourceKcalPerServing *float64 `json:"source_kcal_per_serving"`
}
