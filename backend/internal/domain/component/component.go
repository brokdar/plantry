package component

import "time"

// Role categorises a component for meal planning.
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

// ValidRole reports whether r is a known component role.
func ValidRole(r Role) bool { return validRoles[r] }

// Component is the aggregate root for a dish/recipe.
type Component struct {
	ID                int64
	Name              string
	Role              Role
	VariantGroupID    *int64
	ReferencePortions float64
	PrepMinutes       *int
	CookMinutes       *int
	ImagePath         *string
	Notes             *string
	LastCookedAt      *time.Time
	CookCount         int
	Favorite          bool
	Ingredients       []ComponentIngredient
	Instructions      []Instruction
	Tags              []string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ComponentIngredient links an ingredient to a component with a quantity.
// IngredientName is populated when loaded through the repo; it is not persisted
// back on write.
//
// GramsSource explains how Grams was resolved for the current in-memory value.
// It is set by the resolver at write time and is not persisted.
type ComponentIngredient struct {
	ID             int64
	ComponentID    int64
	IngredientID   int64
	IngredientName string
	Amount         float64
	Unit           string
	Grams          float64
	GramsSource    string
	SortOrder      int
}

// Grams-resolution sources surfaced by the service to help clients show
// confidence badges. These are not persisted to the database.
const (
	GramsSourceDirect   = "direct"   // unit is a mass (g/kg/mg/oz/lb), exact
	GramsSourcePortion  = "portion"  // matched an ingredient-specific portion
	GramsSourceDefault  = "default"  // universal mass default (e.g., "oz")
	GramsSourceFallback = "fallback" // universal volume default (water-density)
	GramsSourceManual   = "manual"   // user-supplied grams; unit could not be auto-resolved
)

// Instruction is a numbered cooking step.
type Instruction struct {
	ID          int64
	ComponentID int64
	StepNumber  int
	Text        string
}

// ListQuery holds filtering, pagination, and sorting for component listing.
type ListQuery struct {
	Search       string
	Role         string
	Tag          string
	FavoriteOnly bool
	Limit        int
	Offset       int
	SortBy       string
	SortDesc     bool
}

// ListResult wraps a page of components with the total count.
type ListResult struct {
	Items []Component
	Total int
}

// InsightsQuery parameterises the rotation-insights lookup. Zero values are
// replaced with defaults by Service.Insights.
type InsightsQuery struct {
	ForgottenWeeks  int
	ForgottenLimit  int
	MostCookedLimit int
}

// Insights bundles the two rotation signals used by the component library
// badges and archive view.
type Insights struct {
	Forgotten  []Component
	MostCooked []Component
}
