// Package preset holds the aggregate, repository port, and service for
// reusable, calendar-agnostic bundles of plates.
//
// A Preset is a named, tagged bundle of 1..N PresetPlates. Each PresetPlate
// is bound to a slot (FK to time_slots, mirroring the spec's "slot type" —
// Plantry has no separate slot_types table; time_slots is the taxonomy).
// Each PresetPlate owns 1..N PresetComponents using the same kind-aware
// quantity model as plate.PlateComponent.
//
// Presets are NOT calendar-aware. They carry no date, no day-offset, no
// week anchor. Applying a preset is a one-way copy onto a target date.
package preset

import (
	"strings"
	"time"
)

// Preset is a named, tagged, reusable bundle of plates.
type Preset struct {
	ID         int64
	Name       string
	Tags       []string // lowercase, trimmed, dedup'd
	Plates     []Plate
	CreatedAt  time.Time
	UpdatedAt  time.Time
	LastUsedAt *time.Time
}

// Plate is a single plate composition inside a preset, bound to a slot type.
type Plate struct {
	ID         int64
	PresetID   int64
	SlotID     int64
	SortOrder  int
	Components []Component
}

// Component is a single food entry inside a preset plate. Mirrors the
// kind-aware quantity model of plate.PlateComponent:
//   - composed food (recipe): Portions is an integer count of servings;
//     Amount/Unit/Grams/GramsSource are nil.
//   - leaf food (ingredient): Amount + Unit are user-entered; Grams is
//     resolved server-side via food.ResolveGrams. Portions is nil.
//
// Exactly one of (Portions) or (Amount + Unit + Grams) is set on a valid
// component — the DB enforces this with a CHECK constraint and the service
// re-validates before write.
type Component struct {
	ID            int64
	PresetPlateID int64
	FoodID        int64
	Portions      *int     // composed only
	Amount        *float64 // leaf only
	Unit          *string  // leaf only — canonical unit key
	Grams         *float64 // leaf only — resolved by service before persist
	GramsSource   *string  // leaf only — surfaced for UI confidence badge
	Note          *string
	SortOrder     int
}

// IsLeaf reports whether the component carries a leaf-style quantity.
func (c Component) IsLeaf() bool { return c.Amount != nil }

// IsComposed reports whether the component carries a composed-style quantity.
func (c Component) IsComposed() bool { return c.Portions != nil }

// NormalizeTag returns tag in canonical form (lowercased, trimmed).
// Empty result means the tag was blank.
func NormalizeTag(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}
