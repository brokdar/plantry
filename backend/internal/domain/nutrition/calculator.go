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
