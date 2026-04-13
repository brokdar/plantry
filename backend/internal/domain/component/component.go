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
	Ingredients       []ComponentIngredient
	Instructions      []Instruction
	Tags              []string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ComponentIngredient links an ingredient to a component with a quantity.
type ComponentIngredient struct {
	ID           int64
	ComponentID  int64
	IngredientID int64
	Amount       float64
	Unit         string
	Grams        float64
	SortOrder    int
}

// Instruction is a numbered cooking step.
type Instruction struct {
	ID          int64
	ComponentID int64
	StepNumber  int
	Text        string
}

// ListQuery holds filtering, pagination, and sorting for component listing.
type ListQuery struct {
	Search   string
	Role     string
	Tag      string
	Limit    int
	Offset   int
	SortBy   string
	SortDesc bool
}

// ListResult wraps a page of components with the total count.
type ListResult struct {
	Items []Component
	Total int
}
