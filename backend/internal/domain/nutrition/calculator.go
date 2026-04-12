package nutrition

// Macros holds the six tracked nutritional values.
type Macros struct {
	Kcal    float64 `json:"kcal"`
	Protein float64 `json:"protein"`
	Fat     float64 `json:"fat"`
	Carbs   float64 `json:"carbs"`
	Fiber   float64 `json:"fiber"`
	Sodium  float64 `json:"sodium"`
}

// IngredientInput represents one ingredient's contribution to a recipe.
type IngredientInput struct {
	Per100g Macros  `json:"per_100g"`
	Grams   float64 `json:"grams"`
}

// ComponentInput represents all ingredients in a component plus portion info.
type ComponentInput struct {
	Ingredients       []IngredientInput `json:"ingredients"`
	ReferencePortions float64           `json:"reference_portions"`
}

// FromIngredients sums the nutritional contribution of each ingredient.
// Formula per macro: sum( per100g * grams / 100 ).
func FromIngredients(items []IngredientInput) Macros {
	var m Macros
	for _, it := range items {
		factor := it.Grams / 100
		m.Kcal += it.Per100g.Kcal * factor
		m.Protein += it.Per100g.Protein * factor
		m.Fat += it.Per100g.Fat * factor
		m.Carbs += it.Per100g.Carbs * factor
		m.Fiber += it.Per100g.Fiber * factor
		m.Sodium += it.Per100g.Sodium * factor
	}
	return m
}

// PerPortion returns the per-portion nutrition for a component.
// It divides the total nutrition by ReferencePortions.
func PerPortion(c ComponentInput) Macros {
	total := FromIngredients(c.Ingredients)
	if c.ReferencePortions <= 0 {
		return total
	}
	return Macros{
		Kcal:    total.Kcal / c.ReferencePortions,
		Protein: total.Protein / c.ReferencePortions,
		Fat:     total.Fat / c.ReferencePortions,
		Carbs:   total.Carbs / c.ReferencePortions,
		Fiber:   total.Fiber / c.ReferencePortions,
		Sodium:  total.Sodium / c.ReferencePortions,
	}
}

// PlateTotal computes total nutrition for a full plate of components.
// Deferred to Phase 5 (planner).
