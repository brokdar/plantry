// Package template holds the aggregate, repository port, and service for
// saved plate compositions (templates).
package template

import "time"

// Scope is how a template is meant to be applied: a single slot, a whole day,
// or a whole week.
type Scope string

const (
	ScopeSlot Scope = "slot"
	ScopeDay  Scope = "day"
	ScopeWeek Scope = "week"
)

// IsValid reports whether s is one of the recognised scopes.
func (s Scope) IsValid() bool {
	switch s {
	case ScopeSlot, ScopeDay, ScopeWeek:
		return true
	}
	return false
}

// Template is a named, reusable plate composition.
type Template struct {
	ID        int64
	Name      string
	Scope     Scope
	Entries   []TemplateEntry
	CreatedAt time.Time
}

// TemplateEntry is one food referenced by a template. For slot-scope
// templates SlotID is nil and DayOffset is 0; day-scope entries set SlotID and
// keep DayOffset = 0; week-scope entries set both SlotID and DayOffset.
type TemplateEntry struct {
	ID         int64
	TemplateID int64
	FoodID     int64
	Portions   float64
	SortOrder  int
	DayOffset  int
	SlotID     *int64
	Note       *string
}
