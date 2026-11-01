// Package planning is the plan generator's core: the state a chooser sees,
// the proposal it returns, the hard-rule checker (spec §6.4) and the quality
// score (spec §6.5). Everything in state.go, proposal.go, check.go and
// score.go is pure.
package planning

import (
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// State is everything the chooser, the checker and the score read about one
// plan window. The caller (the context builder, or a read model) fills it
// with bounded queries; nothing here does I/O.
type State struct {
	Window window.Window
	Today  time.Time
	// Cells are the window's resolved cells in (date, slot order) order.
	Cells []Cell
	// SlotOrder maps slot ids to their sort_order (D12).
	SlotOrder plate.SlotOrder
	// SlotNames maps slot ids to their name key (the LLM prompt's labels).
	SlotNames map[int64]string
	// Plates are the plates in and around the window: the window's own plus
	// the ones eating from sessions the window's plates eat from. A plate
	// with a negative ID is proposed (see Merge).
	Plates []Plate
	// Sessions are the sessions the plates eat from, plus fixed sessions
	// with remaining capacity from before the window.
	Sessions []Session
	// Foods holds the facts of every food the state references and every
	// candidate recipe, by id.
	Foods    map[int64]FoodFacts
	Settings Settings
	Profile  Profile
	// History holds the confirmed plates of the 4 weeks before the window.
	History []HistoryPlate
}

// Cell is one resolved (date, slot) cell of the window.
type Cell struct {
	Date     time.Time
	SlotID   int64
	Mode     availability.Mode
	PresetID *int64
}

// Key returns the cell's (date, slot) key.
func (c Cell) Key() plate.CellKey { return plate.CellKey{Date: c.Date, SlotID: c.SlotID} }

// Plate is a plate as the planner sees it. Persisted plates have their ID;
// proposed plates (Merge) have a negative one.
type Plate struct {
	ID         int64
	Date       time.Time
	SlotID     int64
	Eaters     float64
	Confirmed  bool
	Components []Component
}

// Cell returns the plate's (date, slot) key.
func (p Plate) Cell() plate.CellKey { return plate.CellKey{Date: p.Date, SlotID: p.SlotID} }

// Proposed reports whether the plate comes from a proposal.
func (p Plate) Proposed() bool { return p.ID < 0 }

// Component is one food on a plate, per eater. A composed component has
// Portions and names its session by SessionID (a persisted session) or
// SessionKey (a proposed one); with neither it gets a fresh session of its
// own when applied. A leaf component has Grams.
type Component struct {
	// ID is the persisted component's id; 0 for a proposed one.
	ID         int64
	FoodID     int64
	Portions   *float64
	Grams      *float64
	SessionID  int64
	SessionKey string
	// Proposed marks a component Merge added from a proposal.
	Proposed bool
}

// Multiplier is the per-eater factor on the food's macros: portions for a
// composed food (macros per portion), grams/100 for a leaf (macros per 100 g).
func (c Component) Multiplier() float64 {
	switch {
	case c.Portions != nil:
		return *c.Portions
	case c.Grams != nil:
		return *c.Grams / 100
	}
	return 0
}

// Session is a cook session: persisted (ID set) or proposed (Key set).
type Session struct {
	plate.CookSession
	Key       string
	Confirmed bool
}

// Proposed reports whether the session comes from a proposal.
func (s Session) Proposed() bool { return s.Key != "" }

// FoodFacts are the facts about one food the planner uses. Macros are per
// portion for a composed food and per 100 g for a leaf.
type FoodFacts struct {
	ID                int64
	Name              string
	Kind              food.Kind
	Role              food.Role
	DietClass         diet.Class
	Macros            nutrition.Macros
	NutritionComplete bool
	Minutes           int
	ReferencePortions float64
	KeepDays          *int
	Freezable         bool
	Favorite          bool
	// LastEaten is the date of the newest confirmed past plate with the
	// food; nil when never eaten.
	LastEaten        *time.Time
	TimesEaten8Weeks int
	// LikeScore is liked minus disliked tag hits (D19).
	LikeScore float64
}

// Composed reports whether the food is a recipe.
func (f FoodFacts) Composed() bool { return f.Kind == food.KindComposed }

// Settings are the plan settings the rules read.
type Settings struct {
	plate.BatchSettings
	MaxCookOccasionsPerDay int
	VarietyWeeks           int
	// BatchCooking false keeps every session to its own cell.
	BatchCooking bool
}

// Profile is the part of the user profile the rules read.
type Profile struct {
	KcalTarget         *float64
	ProteinPct         *float64
	FatPct             *float64
	CarbsPct           *float64
	DietType           profile.DietType
	MeatDaysPerWeekMax *int
	HouseholdPortions  float64
	// Locale is the language of the chooser's reasons and notes.
	Locale string
	// Restrictions and Notes (the user's notes to the assistant) are free
	// text only the LLM chooser reads.
	Restrictions []string
	Notes        string
}

// HistoryPlate is a past confirmed plate: its cell and its foods.
type HistoryPlate struct {
	Date    time.Time
	SlotID  int64
	FoodIDs []int64
}

// Target is what a generation scope may change (D6): the cells it may fill
// and the draft rows it replaces. The caller expands it to batch closures
// (plate.Closure) before checking.
type Target struct {
	Cells      []plate.CellKey
	PlateIDs   []int64
	SessionIDs []int64
}
