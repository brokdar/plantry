package ingredient

import "time"

// Source indicates how an ingredient was created.
type Source = string

const (
	SourceManual Source = "manual"
	SourceOFF    Source = "off"
	SourceFDC    Source = "fdc"
)

// Ingredient is the aggregate root for the ingredient catalogue.
type Ingredient struct {
	ID          int64
	Name        string
	Source      Source
	Barcode     *string
	OffID       *string
	FdcID       *string
	ImagePath   *string
	Kcal100g    float64
	Protein100g float64
	Fat100g     float64
	Carbs100g   float64
	Fiber100g   float64
	Sodium100g  float64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ListQuery holds filtering, pagination, and sorting parameters for listing ingredients.
type ListQuery struct {
	Search   string
	Limit    int
	Offset   int
	SortBy   string
	SortDesc bool
}

// ListResult wraps a page of ingredients with the total count.
type ListResult struct {
	Items []Ingredient
	Total int
}

// Portion represents a serving size unit for an ingredient.
type Portion struct {
	IngredientID int64
	Unit         string
	Grams        float64
}
