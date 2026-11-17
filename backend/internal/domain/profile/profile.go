package profile

import "time"

// Profile holds the user's nutrition targets and preferences. Singleton (id=1).
type Profile struct {
	KcalTarget          *float64
	ProteinPct          *float64
	FatPct              *float64
	CarbsPct            *float64
	DietaryRestrictions []string
	Preferences         map[string]any
	SystemPrompt        *string
	Locale              string
	DietType            DietType
	MeatDaysPerWeekMax  *int
	HouseholdPortions   float64
	// Goal is the nutrition goal mode; Service normalises "" to maintain.
	Goal Goal
	// Optional body data. They only feed the kcal-target estimate in the
	// UI and the sex-based safety floor; nothing requires them. BirthYear
	// rather than age: a stored age goes stale.
	WeightKg      *float64
	HeightCm      *float64
	BirthYear     *int
	Sex           *Sex
	ActivityLevel *float64 // PAL: 1.4, 1.6 or 1.8
	UpdatedAt     time.Time
}

// DietType is the household's base diet.
type DietType string

const (
	DietVegan       DietType = "vegan"
	DietVegetarian  DietType = "vegetarian"
	DietPescetarian DietType = "pescetarian"
	DietOmnivore    DietType = "omnivore"
)

// Valid reports whether d is a known diet type.
func (d DietType) Valid() bool {
	switch d {
	case DietVegan, DietVegetarian, DietPescetarian, DietOmnivore:
		return true
	}
	return false
}

// Goal is the user's nutrition goal. The kcal target is always the user's
// own number; the goal decides which deviations around it are tolerated.
type Goal string

const (
	GoalMaintain Goal = "maintain"
	GoalLose     Goal = "lose"
	GoalGain     Goal = "gain"
)

// Valid reports whether g is a known goal.
func (g Goal) Valid() bool {
	switch g {
	case GoalMaintain, GoalLose, GoalGain:
		return true
	}
	return false
}

// Sex selects the absolute kcal floor of the safety rule.
type Sex string

const (
	SexFemale Sex = "female"
	SexMale   Sex = "male"
)

// Valid reports whether s is a known sex.
func (s Sex) Valid() bool { return s == SexFemale || s == SexMale }

// MinDailyKcal is the lowest daily kcal an unsupervised plan may hold per
// person: 1200 for women, 1500 for men, 1200 (the lower figure) when the sex is unknown. Source:
// Academy of Nutrition and Dietetics and NICE CG189 summaries (secondary
// sources, not checked against the primary pages); below these figures only
// under medical supervision.
func MinDailyKcal(s *Sex) float64 {
	if s != nil && *s == SexMale {
		return 1500
	}
	return 1200
}

// ActivityLevels are the allowed PAL values (DGE: mostly sitting, standing,
// physically active).
var ActivityLevels = []float64{1.4, 1.6, 1.8}
