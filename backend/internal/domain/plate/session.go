package plate

import "time"

// CookSession means "food X is cooked at (CookDate, CookSlotID), producing
// Portions portions" (D9). Plate components eat from it: on its own cell
// (fresh), later within keep days (leftover) or later still (frozen).
type CookSession struct {
	ID             int64
	FoodID         int64
	CookDate       time.Time
	CookSlotID     int64
	Portions       float64
	FrozenPortions float64
	// CookedAt is set by "Done cooking"; nil while not cooked.
	CookedAt *time.Time
	Note     *string
	// State, AIRunID and Reason as on Plate (D1).
	State     State
	AIRunID   *int64
	Reason    *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CookCell returns the session's cook cell.
func (s CookSession) CookCell() CellKey { return CellKey{Date: s.CookDate, SlotID: s.CookSlotID} }

// Link is one composed component eating from a session.
type Link struct {
	PlateID     int64
	ComponentID int64
	FoodID      int64
	Cell        CellKey
	// PerEater is the component's portions; Eaters the plate's eaters.
	PerEater float64
	Eaters   float64
	// Portions = PerEater × Eaters: what the link consumes from the session.
	Portions float64
	Kind     LinkKind
	// Position is the link's 1-based rank in cell order (D12).
	Position int
}

// Warning codes of the read model (D13).
const (
	WarnLeftoverBeforeCook = "leftover_before_cook"
	WarnPastKeepDays       = "past_keep_days"
	WarnDeficit            = "deficit"
	WarnFrozenDeficit      = "frozen_deficit"
)

// Warning is an inconsistency a session's links leave behind. PlateID names
// the plate the warning is about; nil for session-wide warnings.
type Warning struct {
	Code    string
	PlateID *int64
}

// SessionView is a session with its links in cell order, its consumption and
// its warnings: the shape the session endpoints and read models return.
type SessionView struct {
	CookSession
	FoodName       string
	KeepDays       int
	UseBy          time.Time
	Consumed       float64
	FrozenConsumed float64
	// Surplus is Portions − Consumed, never below 0.
	Surplus  float64
	Links    []Link
	Meals    int
	Warnings []Warning
}

// BuildView assembles a SessionView from a session and its raw links.
func BuildView(s CookSession, foodName string, keepDays int, links []Link, set BatchSettings, order SlotOrder) SessionView {
	ordered := OrderLinks(s, links, keepDays, order)
	use := Consumption(s, ordered, keepDays)
	surplus := s.Portions - use.Total
	if surplus < 0 {
		surplus = 0
	}
	return SessionView{
		CookSession:    s,
		FoodName:       foodName,
		KeepDays:       keepDays,
		UseBy:          UseBy(s, keepDays),
		Consumed:       use.Total,
		FrozenConsumed: use.Frozen,
		Surplus:        surplus,
		Links:          ordered,
		Meals:          len(ordered),
		Warnings:       Warnings(s, ordered, keepDays, set, order),
	}
}
