package planner

import (
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Week is the calendar unit for meal planning. Identified by ISO year + week number.
type Week struct {
	ID         int64
	Year       int
	WeekNumber int
	Plates     []plate.Plate
	CreatedAt  time.Time
}
