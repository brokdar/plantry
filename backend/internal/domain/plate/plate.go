package plate

import "time"

// Plate is a meal scheduled at a specific date+slot. It composes one or more
// foods (a main, sides, etc.) for Eaters people.
type Plate struct {
	ID     int64
	Date   time.Time // time-of-day always 00:00 UTC; YYYY-MM-DD
	SlotID int64
	// Eaters is how many eater portions the plate serves (D10). Component
	// quantities are per eater.
	Eaters     float64
	Note       *string
	Components []PlateComponent
	// Feedback is the plate's rating, nil when unrated.
	Feedback  *Feedback
	CreatedAt time.Time
	UpdatedAt time.Time
}

// DateString returns the plate's date formatted as YYYY-MM-DD.
func (p Plate) DateString() string { return p.Date.Format(dateLayout) }

// Cell returns the plate's (date, slot) cell.
func (p Plate) Cell() CellKey { return CellKey{Date: p.Date, SlotID: p.SlotID} }

// Feedback is a plate's rating as the plate aggregate carries it. The
// feedback package owns the status rules; the plate only carries the row so
// snapshots restore it.
type Feedback struct {
	Status  string
	Note    *string
	RatedAt time.Time
}

// PlateComponent links a food onto a plate. Quantities are per eater.
//
// The quantity is interpreted by food kind:
//   - Composed food (a recipe): Portions is the servings per eater (REAL,
//     usually 1) and CookSessionID names the session it is eaten from (D8).
//     Amount/Unit/Grams/GramsSource are nil.
//   - Leaf food (an ingredient): Amount + Unit are user-entered; Grams is
//     resolved server-side via the same chain recipes use (food.Portion →
//     units default → manual). Portions and CookSessionID are nil.
//     GramsSource records which branch of the resolver matched.
//
// The DB enforces the two shapes with a CHECK constraint and the service
// re-validates before write.
type PlateComponent struct {
	ID            int64
	PlateID       int64
	FoodID        int64
	Portions      *float64 // composed only — per eater
	Amount        *float64 // leaf only — per eater
	Unit          *string  // leaf only — canonical unit key
	Grams         *float64 // leaf only — resolved by service before persist
	GramsSource   *string  // leaf only — surfaced for UI confidence badge
	CookSessionID *int64   // composed only
	SortOrder     int

	// Read-only food facts filled when a plate is loaded (never written).
	FoodName string
	FoodKind string
	FoodRole *string
}

// IsLeaf reports whether the component carries a leaf-style quantity
// (amount + unit + grams). A zero PlateComponent reports false.
func (pc PlateComponent) IsLeaf() bool { return pc.Amount != nil }

// IsComposed reports whether the component carries a composed-style quantity.
// A zero PlateComponent reports false.
func (pc PlateComponent) IsComposed() bool { return pc.Portions != nil }

// Multiplier returns the scalar to apply to per-portion (composed) or
// per-100g (leaf) macros for this component, for one eater. Composed
// components return their portions; leaf components return grams / 100;
// invalid (neither shape set) returns 0.
func (pc PlateComponent) Multiplier() float64 {
	if pc.Portions != nil {
		return *pc.Portions
	}
	if pc.Grams != nil {
		return *pc.Grams / 100
	}
	return 0
}

const dateLayout = "2006-01-02"
