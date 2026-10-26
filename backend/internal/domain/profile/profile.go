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
	UpdatedAt           time.Time
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
