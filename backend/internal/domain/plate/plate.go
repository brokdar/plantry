package plate

import "time"

// Plate is a meal scheduled at a specific date+slot.
// It composes one or more foods (a main, sides, etc.). A plate component can
// reference either a leaf food (e.g., a standalone apple) or a composed food.
type Plate struct {
	ID         int64
	Date       time.Time // time-of-day always 00:00 UTC; YYYY-MM-DD
	SlotID     int64
	Note       *string
	Skipped    bool
	Components []PlateComponent
	CreatedAt  time.Time
}

// DateString returns the plate's date formatted as YYYY-MM-DD.
func (p Plate) DateString() string { return p.Date.Format("2006-01-02") }

// PlateComponent links a food onto a plate with a portion count + ordering.
type PlateComponent struct {
	ID        int64
	PlateID   int64
	FoodID    int64
	Portions  float64
	SortOrder int
}
