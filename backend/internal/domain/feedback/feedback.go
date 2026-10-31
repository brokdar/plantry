package feedback

import "time"

// Status is how the user rated a plate after it appeared on their plan (D17).
type Status string

const (
	StatusLoved    Status = "loved"
	StatusDisliked Status = "disliked"
	// StatusNotEaten ("Didn't eat it") removes the plate from the food's
	// derived eating history (D16).
	StatusNotEaten Status = "not_eaten"
)

// Valid reports whether s is one of the allowed feedback statuses.
func (s Status) Valid() bool {
	switch s {
	case StatusLoved, StatusDisliked, StatusNotEaten:
		return true
	}
	return false
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
