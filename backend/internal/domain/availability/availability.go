// Package availability resolves whether a planner cell (date, slot) needs a
// plate. A per-date override wins, otherwise the weekday × slot default
// applies, otherwise the cell is "plan".
package availability

import (
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
)

// Mode is a cell's availability.
type Mode string

const (
	// ModePlan means a plate is needed and should be planned.
	ModePlan Mode = "plan"
	// ModeRoutine means a plate is needed, filled from a bound preset. In
	// Phase 1 it is a label only; nothing applies the preset.
	ModeRoutine Mode = "routine"
	// ModeNone means no plate is needed; none may exist there.
	ModeNone Mode = "none"
)

// Valid reports whether m is a known mode.
func (m Mode) Valid() bool {
	switch m {
	case ModePlan, ModeRoutine, ModeNone:
		return true
	}
	return false
}

// Source tells which level a resolved cell's mode came from.
type Source string

const (
	SourceDefault  Source = "default"
	SourceOverride Source = "override"
	SourceFallback Source = "fallback"
)

// MaxNoteLen is the maximum length of an override note, in characters.
const MaxNoteLen = 200

// Default is the weekday × slot default. Weekday 0 is Monday.
type Default struct {
	Weekday  int
	SlotID   int64
	Mode     Mode
	PresetID *int64
	// PresetName is filled by the repository on read; it is ignored on write.
	PresetName *string
}

// Override is a per-date mode for one cell.
type Override struct {
	Date     time.Time
	SlotID   int64
	Mode     Mode
	PresetID *int64
	Note     *string
	// PresetName is filled by the repository on read; it is ignored on write.
	PresetName *string
}

// Cell is the resolved availability of (date, slot).
type Cell struct {
	Date       time.Time
	SlotID     int64
	Mode       Mode
	Source     Source
	PresetID   *int64
	PresetName *string
	Note       *string
}

// OverrideInput is the input of Service.SetOverride.
type OverrideInput struct {
	Date        time.Time
	SlotID      int64
	Mode        Mode
	PresetID    *int64
	Note        *string
	RemovePlate bool
}

// WeekdayOf returns the weekday of t with Monday = 0.
func WeekdayOf(t time.Time) int {
	return (int(t.Weekday()) + 6) % 7
}

func validateMode(m Mode) error {
	if !m.Valid() {
		return fmt.Errorf("%w: unknown mode %q", domain.ErrInvalidInput, m)
	}
	return nil
}
