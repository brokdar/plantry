package food

import "context"

// Candidate represents an external lookup result (OFF, FDC) that can be
// promoted into a leaf food. Only leaf foods have provenance and direct
// per-100g nutrition, so composed foods never come from a Candidate.
type Candidate struct {
	Name       string `json:"name"`
	SourceName string `json:"source_name,omitempty"`
	Brand      string `json:"brand,omitempty"`
	Source     Source `json:"source"`
	Barcode    string `json:"barcode,omitempty"`
	FdcID      int    `json:"fdc_id,omitempty"`
	ImageURL   string `json:"image_url,omitempty"`
	ExistingID *int64 `json:"existing_id,omitempty"`

	// ServingQuantityG is grams per serving as reported by the upstream source
	// (currently OFF). Used to seed a "serving" portion on leaf food create.
	ServingQuantityG *float64 `json:"serving_quantity_g,omitempty"`

	Kcal100g    *float64 `json:"kcal_100g"`
	Protein100g *float64 `json:"protein_100g"`
	Fat100g     *float64 `json:"fat_100g"`
	Carbs100g   *float64 `json:"carbs_100g"`
	Fiber100g   *float64 `json:"fiber_100g,omitempty"`
	Sodium100g  *float64 `json:"sodium_100g,omitempty"`

	SaturatedFat100g *float64 `json:"saturated_fat_100g,omitempty"`
	TransFat100g     *float64 `json:"trans_fat_100g,omitempty"`
	Cholesterol100g  *float64 `json:"cholesterol_100g,omitempty"`
	Sugar100g        *float64 `json:"sugar_100g,omitempty"`
	Potassium100g    *float64 `json:"potassium_100g,omitempty"`
	Calcium100g      *float64 `json:"calcium_100g,omitempty"`
	Iron100g         *float64 `json:"iron_100g,omitempty"`
	Magnesium100g    *float64 `json:"magnesium_100g,omitempty"`
	Phosphorus100g   *float64 `json:"phosphorus_100g,omitempty"`
	Zinc100g         *float64 `json:"zinc_100g,omitempty"`
	VitaminA100g     *float64 `json:"vitamin_a_100g,omitempty"`
	VitaminC100g     *float64 `json:"vitamin_c_100g,omitempty"`
	VitaminD100g     *float64 `json:"vitamin_d_100g,omitempty"`
	VitaminB12100g   *float64 `json:"vitamin_b12_100g,omitempty"`
	VitaminB6100g    *float64 `json:"vitamin_b6_100g,omitempty"`
	Folate100g       *float64 `json:"folate_100g,omitempty"`
}

// FoodProvider searches external food databases by name.
type FoodProvider interface {
	SearchByName(ctx context.Context, query string, limit int) ([]Candidate, error)
}

// BarcodeProvider extends FoodProvider with barcode lookup.
type BarcodeProvider interface {
	FoodProvider
	LookupBarcode(ctx context.Context, barcode string) ([]Candidate, error)
}

// PortionInfo is the provider-neutral shape of a per-unit gram weight from an
// external source (e.g., FDC's foodPortions).
type PortionInfo struct {
	RawUnit    string
	Modifier   string
	GramWeight float64
}

// PortionProvider supplies per-food unit → grams conversions used to seed
// food_portions. FDC implements this; OFF surfaces only a generic serving
// handled directly on import.
type PortionProvider interface {
	GetFoodPortions(ctx context.Context, fdcID int) ([]PortionInfo, error)
}
