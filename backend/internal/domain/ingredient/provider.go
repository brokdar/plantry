package ingredient

import "context"

// Candidate represents a food item from any source (local, OFF, FDC).
type Candidate struct {
	Name        string   `json:"name"`
	Brand       string   `json:"brand,omitempty"`
	Source      Source   `json:"source"`
	Barcode     string   `json:"barcode,omitempty"`
	FdcID       int      `json:"fdc_id,omitempty"`
	ImageURL    string   `json:"image_url,omitempty"`
	ExistingID  *int64   `json:"existing_id,omitempty"`
	Kcal100g    *float64 `json:"kcal_100g"`
	Protein100g *float64 `json:"protein_100g"`
	Fat100g     *float64 `json:"fat_100g"`
	Carbs100g   *float64 `json:"carbs_100g"`
	Fiber100g   *float64 `json:"fiber_100g,omitempty"`
	Sodium100g  *float64 `json:"sodium_100g,omitempty"`
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
