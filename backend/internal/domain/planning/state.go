// Package planning is the plan generator's core: the state a chooser sees,
// the proposal it returns, the hard-rule checker (spec §6.4) and the quality
// score (spec §6.5). Everything in state.go, proposal.go, check.go and
// score.go is pure.
package planning

import (
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// State is everything the chooser, the checker and the score read about one
// plan window. The caller (the context builder, or a read model) fills it
// with bounded queries; nothing here does I/O.
type State struct {
	// MaxMinutes is a wish's time limit by cell (planner runs only, V72): a
	// main cooked in such a cell takes at most that many minutes (prep +
	// cook); unknown minutes do not meet it. A leftover eaten there costs no
	// time and is free.
	MaxMinutes map[cellID]int
	// Avoid are foods the user removed earlier in the conversation: no way,
	// option or alternative adds them again (set by the chat tools).
	Avoid map[int64]bool
	// Named are the foods the user's message names (set by the chat tools from
	// the request text, never from a model argument): a variety repeat of one
	// is the warning variety_repeat_named, not a violation (SF3).
	Named map[int64]bool
	// PairAddOns turns on the add-on pairing rules (addOnFits) for the chat
	// tools; the planner's own top-ups keep their candidates.
	PairAddOns bool
	// CookPile: an away call re-anchors the cooks of a day under CookTogether,
	// so a move may bring any number of dishes onto one cooking time.
	CookPile bool

	Window window.Window
	Today  time.Time
	// Cells are the window's resolved cells in (date, slot order) order.
	Cells []Cell
	// SlotOrder maps slot ids to their sort_order (D12).
	SlotOrder plate.SlotOrder
	// SlotNames maps slot ids to their name key (the LLM prompt's labels).
	SlotNames map[int64]string
	// SlotKind maps slot ids to their kind (light or meal); a slot without
	// an entry is a full meal.
	SlotKind map[int64]slot.Kind
	// SlotShare is each active slot's expected share of the day (SB2: the band
	// midpoint over the sum of midpoints); empty means an even split.
	SlotShare map[int64]float64
	// SlotBand is each active slot's effective energy band (SB1).
	SlotBand map[int64]slot.Band
	// Plates are the plates in and around the window: the window's own plus
	// the ones eating from sessions the window's plates eat from. A plate
	// with a negative ID is proposed (see Merge).
	Plates []Plate
	// Sessions are the sessions the plates eat from, plus fixed sessions
	// with remaining capacity from before the window.
	Sessions []Session
	// Foods holds the facts of every food the state references and every
	// candidate recipe, by id.
	Foods map[int64]FoodFacts
	// NoCook lists the ids (in Foods) of the no-cook foods the evaluation
	// may offer to close a day's gap (Evaluate options); empty where the
	// builder does not load them.
	NoCook []int64
	// Garnish lists the ids (in Foods) of fresh herb leaves that are not
	// no-cook foods: garnish candidates for tips only, never offered by the
	// planner.
	Garnish  []int64
	Settings Settings
	Profile  Profile
	// History holds the confirmed plates of the 4 weeks before the window.
	History []HistoryPlate
	// Corrections are the user's recipe swaps of the 4 weeks before the
	// window (D21): what the generator proposed and the user replaced.
	Corrections []Replacement
	// RatherNot holds the foods the household would rather not get as a
	// side or add-on (RatherNotSides); nil where the builder does not load
	// feedback.
	RatherNot map[int64]bool
	// Light are the light requests of this run (see LightIntent); nil
	// without any.
	Light []LightIntent
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
	// Position is the stored place on the plate (main, side, extra);
	// empty on a proposed component.
	Position plate.Position
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
	// FiberMissing: a leaf of the food has no fibre value (counted as 0).
	FiberMissing      bool
	Minutes           int
	ReferencePortions float64
	KeepDays          *int
	Freezable         bool
	Favorite          bool
	// Archived: hidden from planning (LIB-4). The food stays in State.Foods
	// when a plate holds it, but is never a candidate.
	Archived bool
	// VariantGroup is the recipe's variant group id (0: none). A group is one
	// dish for H8 (see State.dish, ShareGroupHistory).
	VariantGroup int64
	// BaseFood is the recipe a hidden substitution variant was made from (0:
	// not one). It counts as that recipe for S8 and the 8-week count.
	BaseFood int64
	// LastEaten is the date of the newest confirmed past plate with the
	// food; nil when never eaten. With a variant group: the newest of the
	// group.
	LastEaten        *time.Time
	TimesEaten8Weeks int
	// LikeScore is liked minus disliked tag hits (D19).
	LikeScore float64
	// ExploreTag is set for a recipe tagged explore by discovery.
	ExploreTag bool
	// NoCook: the food needs no cooking (see NoCook): a leaf eaten raw, or
	// a recipe whose cook minutes are explicitly 0 and that is not a main.
	NoCook bool
	// ServingG is a leaf's stored serving size in grams (0: none, see
	// ServingGrams).
	ServingG float64
	// Leaves is a recipe's leaf grams per portion by leaf food id (S9's
	// duplicate-of-the-main check).
	Leaves map[int64]float64
	// Groups are the DGE food groups in a portion (leaf: 100 g), see FoodGroups.
	Groups FoodGroups
	// Sweet: a leaf that is mostly sugar (see IsSweet); never a kcal filler.
	Sweet bool
	// Tags are the food's tags (Alternatives' search and tag filter).
	Tags []string
	// LeafRole is a leaf's effective role (shopping.EffectiveRole); Role
	// stays recipe-only. Alternatives reads it for side and add candidates.
	LeafRole food.Role
	// Attrs are the food's recipe attributes (flavor.Resolve: user, then
	// assistant, then derived values); set by the Builder only. Plate fit
	// reads them (platefit.go).
	Attrs flavor.Attributes
	// Protein is a recipe's first meat, fish or legume hero (monotony,
	// TASTE-9); "" without one. Set by the Builder.
	Protein string
	// Taste is the recipe's taste score from the household's taste items
	// (taste.Profile.Recipe, -1..1; 0 without items). Set by the Builder.
	Taste float64
	// Never: the recipe holds a food the household never eats (the
	// kitchen's never items): no auto-placing path picks it. Set by the
	// Builder.
	Never bool
	// Staple: a recipe the household has made its own (THEME-1): learned
	// from its cook sessions and signals, or pinned by the favourite star.
	// It ranks first among equals and no soft rule holds it back. Set by
	// the Builder.
	Staple bool
	// Fresh and Starch are plate fit's inputs derived from the name and
	// ingredients (freshStarch); nil is unknown.
	Fresh, Starch *bool
}

// Explore reports whether the food is an explore candidate: tagged explore
// and never eaten (D17).
func (f FoodFacts) Explore() bool { return f.ExploreTag && f.LastEaten == nil }

// Composed reports whether the food is a recipe.
func (f FoodFacts) Composed() bool { return f.Kind == food.KindComposed }

// Settings are the plan settings the rules read.
type Settings struct {
	plate.BatchSettings
	MaxCookOccasionsPerDay int
	VarietyWeeks           int
	// BatchCooking false keeps every session to its own cell.
	BatchCooking bool
	// MaxDishMealsPerWeek is plan.max_dish_meals_per_week (S8); 0 reads
	// as DefaultMaxDishMeals.
	MaxDishMealsPerWeek int
	// PlateFit is plan.plate_fit: plate fit judges sides and add-ons
	// (PLATE-4, PLATE-5); false is the planner without it.
	PlateFit bool
	// BatchVariation is chef.batch_variation: the leftover plates of a
	// batch get another side than its cook plate (PLATE-6).
	BatchVariation bool
	// ChefOn is chef.enabled: with it a recipe over a slot's time allowance
	// is ineligible for that slot (GATE-2).
	ChefOn bool
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
	// Goal, Sex and the body data feed the nutrition policy (PolicyOf).
	Goal          profile.Goal
	Sex           *profile.Sex
	WeightKg      *float64
	BirthYear     *int
	ProteinGPerKg *float64
	// MinutesWeekday and MinutesWeekend are the cooking time allowances (KIT-1);
	// 0 means no allowance (a hand-built State).
	MinutesWeekday int
	MinutesWeekend int
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
