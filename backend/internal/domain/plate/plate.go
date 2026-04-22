package plate

import "time"

// Plate is a meal scheduled at a specific day+slot in a week.
// It composes one or more components (a main, sides, etc.).
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

// PlateComponent is one component on a plate, with portion count and ordering.
type PlateComponent struct {
	ID          int64
	PlateID     int64
	ComponentID int64
	Portions    float64
	SortOrder   int
}

// ValidDay reports whether d is a valid day-of-week index (0..6).
func ValidDay(d int) bool { return d >= 0 && d <= 6 }
