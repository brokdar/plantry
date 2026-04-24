// Package food is the unified aggregate root for what used to be ingredients +
// components: a single recursive "Food" that can be a LEAF (direct per-100g
// nutrition — apple, chicken breast, rice) or COMPOSED (built from child Foods
// with optional instructions + tags — schnitzel with potato salad, homemade
// pesto). Plates and templates reference a Food by ID regardless of kind.
package food

import "time"

// Kind discriminates between leaf foods (own direct nutrition) and composed
// foods (nutrition aggregated from child foods).
type Kind string

const (
	KindLeaf     Kind = "leaf"
	KindComposed Kind = "composed"
)

// Source indicates how a leaf food was created.
type Source = string

const (
	SourceManual Source = "manual"
	SourceOFF    Source = "off"
	SourceFDC    Source = "fdc"
)

// Role categorises a composed food for meal planning.
type Role string

const (
	RoleMain        Role = "main"
	RoleSideStarch  Role = "side_starch"
	RoleSideVeg     Role = "side_veg"
	RoleSideProtein Role = "side_protein"
	RoleSauce       Role = "sauce"
	RoleDrink       Role = "drink"
	RoleDessert     Role = "dessert"
	RoleStandalone  Role = "standalone"
)

var validRoles = map[Role]bool{
	RoleMain:        true,
	RoleSideStarch:  true,
	RoleSideVeg:     true,
	RoleSideProtein: true,
	RoleSauce:       true,
	RoleDrink:       true,
	RoleDessert:     true,
	RoleStandalone:  true,
}

// ValidRole reports whether r is a known food role.
func ValidRole(r Role) bool { return validRoles[r] }

// Food is the unified aggregate root.
type Food struct {
	ID   int64
	Kind Kind
	Name string

	// LEAF provenance (required for leaf, nil for composed).
	Source  *Source
	Barcode *string
	OffID   *string
	FdcID   *string

	// LEAF direct per-100g nutrition (nil for composed).
	Kcal100g         *float64
	Protein100g      *float64
	Fat100g          *float64
	Carbs100g        *float64
	Fiber100g        *float64
	Sodium100g       *float64
	SaturatedFat100g *float64
	TransFat100g     *float64
	Cholesterol100g  *float64 // mg
	Sugar100g        *float64
	Potassium100g    *float64 // mg
	Calcium100g      *float64 // mg
	Iron100g         *float64 // mg
	Magnesium100g    *float64 // mg
	Phosphorus100g   *float64 // mg
	Zinc100g         *float64 // mg
	VitaminA100g     *float64 // µg RAE
	VitaminC100g     *float64 // mg
	VitaminD100g     *float64 // µg
	VitaminB12100g   *float64 // µg
	VitaminB6100g    *float64 // mg
	Folate100g       *float64 // µg DFE

	// COMPOSED metadata (nil for leaf).
	Role              *Role
	VariantGroupID    *int64
	ReferencePortions *float64
	PrepMinutes       *int
	CookMinutes       *int
	Notes             *string

	// Shared.
	ImagePath    *string
	Favorite     bool
	LastCookedAt *time.Time
	CookCount    int

	// Composed children and associated data. Nil/empty for leaf.
	Children     []FoodComponent
	Instructions []Instruction
	Tags         []string

	// Leaf-only unit→grams overrides. Nil/empty for composed.
	Portions []Portion

	CreatedAt time.Time
	UpdatedAt time.Time
}

// FoodComponent links a child food into a composed parent with a quantity.
// ChildName + ChildKind are populated on read for UI display; not persisted
// back on write. GramsSource is computed by the grams resolver and not
// persisted.
type FoodComponent struct {
	ID          int64
	ParentID    int64
	ChildID     int64
	ChildName   string
	ChildKind   Kind
	Amount      float64
	Unit        string
	Grams       float64
	GramsSource string
	SortOrder   int
}

// Grams-resolution sources surfaced by the service for UI confidence badges.
// Not persisted.
const (
	GramsSourceDirect   = "direct"   // unit is a mass (g/kg/mg/oz/lb), exact
	GramsSourcePortion  = "portion"  // matched a food-specific portion
	GramsSourceDefault  = "default"  // universal mass default (e.g., "oz")
	GramsSourceFallback = "fallback" // universal volume default (water-density)
	GramsSourceManual   = "manual"   // user-supplied grams; unit unresolved
)

// Instruction is a numbered cooking step on a composed food.
type Instruction struct {
	ID         int64
	FoodID     int64
	StepNumber int
	Text       string
}

// Portion is a unit→grams override on a leaf food (e.g., "1 cup rice = 158 g").
type Portion struct {
	FoodID int64
	Unit   string
	Grams  float64
}

// ListQuery filters, paginates, and sorts a food listing.
type ListQuery struct {
	Kind         Kind // "" means both kinds
	Search       string
	Role         string
	Tag          string
	FavoriteOnly bool
	Limit        int
	Offset       int
	SortBy       string
	SortDesc     bool
}

// ListResult wraps a page of foods with the total count.
type ListResult struct {
	Items []Food
	Total int
}

// InsightsQuery parameterises the rotation-insights lookup. Zero values are
// replaced with defaults by Service.Insights.
type InsightsQuery struct {
	ForgottenWeeks  int
	ForgottenLimit  int
	MostCookedLimit int
}

// Insights bundles the two rotation signals used by the library badges and
// archive view. Both slices contain composed foods only.
type Insights struct {
	Forgotten  []Food
	MostCooked []Food
}
