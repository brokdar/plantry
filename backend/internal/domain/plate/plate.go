package plate

import "time"

// Plate is a meal scheduled at a specific day+slot in a week.
// It composes one or more foods (a main, sides, etc.). A plate component can
// reference either a leaf food (e.g., a standalone apple) or a composed food.
type Plate struct {
	ID         int64
	WeekID     int64
	Day        int // 0=Mon ... 6=Sun
	SlotID     int64
	Note       *string
	Skipped    bool
	Components []PlateComponent
	CreatedAt  time.Time
}

// PlateComponent links a food onto a plate with a portion count + ordering.
type PlateComponent struct {
	ID        int64
	PlateID   int64
	FoodID    int64
	Portions  float64
	SortOrder int
}

// ValidDay reports whether d is a valid day-of-week index (0..6).
func ValidDay(d int) bool { return d >= 0 && d <= 6 }
