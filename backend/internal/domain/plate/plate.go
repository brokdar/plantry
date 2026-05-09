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

// PlateComponent links a food onto a plate.
//
// The quantity is interpreted by food kind:
//   - Composed food (a recipe): Portions is an integer count of servings.
//     Amount/Unit/Grams/GramsSource are nil.
//   - Leaf food (an ingredient): Amount + Unit are user-entered; Grams is
//     resolved server-side via the same chain recipes use (food.Portion →
//     units default → manual). Portions is nil. GramsSource records which
//     branch of the resolver matched, for UI confidence badges.
//
// Exactly one of (Portions) or (Amount + Unit + Grams) is set on a valid
// component — the DB enforces this with a CHECK constraint and the service
// re-validates before write.
type PlateComponent struct {
	ID          int64
	PlateID     int64
	FoodID      int64
	Portions    *int     // composed only
	Amount      *float64 // leaf only
	Unit        *string  // leaf only — canonical unit key
	Grams       *float64 // leaf only — resolved by service before persist
	GramsSource *string  // leaf only — surfaced for UI confidence badge
	SortOrder   int
}

// IsLeaf reports whether the component carries a leaf-style quantity
// (amount + unit + grams). A zero PlateComponent reports false.
func (pc PlateComponent) IsLeaf() bool { return pc.Amount != nil }

// IsComposed reports whether the component carries a composed-style quantity
// (integer portions). A zero PlateComponent reports false.
func (pc PlateComponent) IsComposed() bool { return pc.Portions != nil }

// Multiplier returns the scalar to apply to per-portion (composed) or
// per-100g (leaf) macros for this component. Composed components return
// their integer portion count as a float; leaf components return grams / 100;
// invalid (neither shape set) returns 0.
func (pc PlateComponent) Multiplier() float64 {
	if pc.Portions != nil {
		return float64(*pc.Portions)
	}
	if pc.Grams != nil {
		return *pc.Grams / 100
	}
	return 0
}

// LegacyPortionsValue extracts a float "portions" representation of a
// kind-aware component for legacy callers (templates, agent JSON output)
// that have not yet been updated to the new model. Composed components
// return their integer portion count as a float; leaf components return
// grams / 100, matching the migration's backfill rule. Returns 0 when both
// shapes are absent.
//
// TODO(plate-workflow-rework): retire this when templates and agent tools
// adopt the kind-aware quantity model directly.
func (pc PlateComponent) LegacyPortionsValue() float64 {
	if pc.Portions != nil {
		return float64(*pc.Portions)
	}
	if pc.Grams != nil {
		return *pc.Grams / 100
	}
	return 0
}

// QuantityFromLegacyPortions translates a legacy float "portions" value into
// the kind-aware quantity model:
//
//   - composed kind → Portions = round-half-up(portions), clamped to ≥ 1
//   - leaf kind     → Amount = portions × 100 g, Unit = "g", Grams = same,
//     GramsSource = "direct" (mirrors the migration's backfill rule)
//
// This is used by the template apply code and the AI agent tools, both of
// which still speak the old "portions" vocabulary. New callers should build
// PlateComponent values directly with the right fields set.
//
// TODO(plate-workflow-rework, phase ?): replace the float-portions vocabulary
// in templates and agent tools with the kind-aware model so this translator
// can go away. See PLATE_WORKFLOW_REWORK.md, "Cross-cutting concerns".
func QuantityFromLegacyPortions(kind string, portions float64) PlateComponent {
	if kind == "leaf" {
		amount := portions * 100
		if amount <= 0 {
			amount = 100
		}
		grams := amount
		unit := "g"
		src := "direct"
		return PlateComponent{
			Amount:      &amount,
			Unit:        &unit,
			Grams:       &grams,
			GramsSource: &src,
		}
	}
	// Default to composed; round half-up, clamp to ≥ 1.
	n := int(portions + 0.5)
	if n < 1 {
		n = 1
	}
	return PlateComponent{Portions: &n}
}
