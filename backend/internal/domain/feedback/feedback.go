package feedback

import "time"

// Status is how the user rated a plate after it appeared on their week.
type Status string

const (
	StatusCooked   Status = "cooked"
	StatusSkipped  Status = "skipped"
	StatusLoved    Status = "loved"
	StatusDisliked Status = "disliked"
)

// Valid reports whether s is one of the allowed feedback statuses.
func (s Status) Valid() bool {
	switch s {
	case StatusCooked, StatusSkipped, StatusLoved, StatusDisliked:
		return true
	}
	return false
}

// IncrementsCookCount reports whether transitioning into this status should
// increment the cook_count / last_cooked_at of the components on the plate.
// Only "cooked" and "loved" imply the user actually prepared the meal.
func (s Status) IncrementsCookCount() bool {
	return s == StatusCooked || s == StatusLoved
}

// TouchesPreferences reports whether transitioning into this status should run
// the preference heuristic (appending component tags to likes/dislikes).
func (s Status) TouchesPreferences() bool {
	return s == StatusLoved || s == StatusDisliked
}

// PlateFeedback is the single feedback row for a given plate (one per plate).
type PlateFeedback struct {
	PlateID int64
	Status  Status
	Note    *string
	RatedAt time.Time
}
