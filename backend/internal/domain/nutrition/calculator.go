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

// PlateComponentInput is one component on a plate with its per-portion macros
// and the number of portions plated.
type PlateComponentInput struct {
	Macros   Macros
	Portions float64
}

// PlateInput holds all components on a single plate.
type PlateInput struct {
	Components []PlateComponentInput
}

// PlateTotal sums the nutritional contribution of every component on a plate.
// Formula: for each component, Macros × Portions; sum across all components.
func PlateTotal(p PlateInput) Macros {
	var m Macros
	for _, c := range p.Components {
		m.Kcal += c.Macros.Kcal * c.Portions
		m.Protein += c.Macros.Protein * c.Portions
		m.Fat += c.Macros.Fat * c.Portions
		m.Carbs += c.Macros.Carbs * c.Portions
		m.Fiber += c.Macros.Fiber * c.Portions
		m.Sodium += c.Macros.Sodium * c.Portions
	}
	return m
}

// DayPlate associates a plate (as resolved macros) with a day-of-week index (0=Mon).
type DayPlate struct {
	Day   int
	Plate PlateInput
}

// WeekTotalsResult holds per-day and whole-week macro sums.
type WeekTotalsResult struct {
	Days map[int]Macros
	Week Macros
}

// WeekTotals aggregates plate totals across all days of a week.
func WeekTotals(plates []DayPlate) WeekTotalsResult {
	days := make(map[int]Macros)
	var week Macros
	for _, dp := range plates {
		pt := PlateTotal(dp.Plate)
		d := days[dp.Day]
		d.Kcal += pt.Kcal
		d.Protein += pt.Protein
		d.Fat += pt.Fat
		d.Carbs += pt.Carbs
		d.Fiber += pt.Fiber
		d.Sodium += pt.Sodium
		days[dp.Day] = d
		week.Kcal += pt.Kcal
		week.Protein += pt.Protein
		week.Fat += pt.Fat
		week.Carbs += pt.Carbs
		week.Fiber += pt.Fiber
		week.Sodium += pt.Sodium
	}
	return WeekTotalsResult{Days: days, Week: week}
}
