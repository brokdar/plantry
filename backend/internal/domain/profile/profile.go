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
	UpdatedAt           time.Time
}
