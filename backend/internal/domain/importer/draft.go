package importer

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
	// units.CountGrams ("1 piece ≈ 120 g"); nil for mass, volume and
	// unparsed lines. A leaf's own portion overrides it on Finalize.
	PortionGrams *float64 `json:"portion_grams"`
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
