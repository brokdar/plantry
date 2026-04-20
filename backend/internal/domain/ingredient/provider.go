package ingredient

import "context"

// Candidate represents a food item from any source (local, OFF, FDC).
type Candidate struct {
	Name       string `json:"name"`
	SourceName string `json:"source_name,omitempty"` // name exactly as returned by the upstream source
	Brand      string `json:"brand,omitempty"`
	Source     Source `json:"source"`
	Barcode    string `json:"barcode,omitempty"`
	FdcID      int    `json:"fdc_id,omitempty"`
	ImageURL   string `json:"image_url,omitempty"`
	ExistingID *int64 `json:"existing_id,omitempty"`

	Kcal100g    *float64 `json:"kcal_100g"`
	Protein100g *float64 `json:"protein_100g"`
	Fat100g     *float64 `json:"fat_100g"`
	Carbs100g   *float64 `json:"carbs_100g"`
	Fiber100g   *float64 `json:"fiber_100g,omitempty"`
	Sodium100g  *float64 `json:"sodium_100g,omitempty"`

	// Extended nutrients — nullable; omitted from JSON when no upstream data.
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

// FoodProvider can search foods by name.
type FoodProvider interface {
	SearchByName(ctx context.Context, query string, limit int) ([]Candidate, error)
}

// BarcodeProvider can also look up foods by barcode.
type BarcodeProvider interface {
	FoodProvider
	LookupBarcode(ctx context.Context, barcode string) ([]Candidate, error)
}
