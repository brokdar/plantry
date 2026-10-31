package food

import "context"

// Repository is the port that adapters must implement for food persistence.
// A single repository covers both leaf and composed foods; the kind lives on
// the aggregate.
type Repository interface {
	HistoryReader
	Create(ctx context.Context, f *Food) error
	Get(ctx context.Context, id int64) (*Food, error)
	// LeafIDByName returns the ID of the leaf food with exactly this name, or
	// domain.ErrNotFound. Leaf names are unique.
	LeafIDByName(ctx context.Context, name string) (int64, error)
	Update(ctx context.Context, f *Food) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) (*ListResult, error)

	// Reachable returns the set of food IDs reachable from parentID via
	// food_components edges. Used for cycle detection before an Update.
	Reachable(ctx context.Context, parentID int64) (map[int64]struct{}, error)

	// LookupByIDs loads a batch of foods (header only, no children) for
	// recursive nutrition resolution and shopping aggregation.
	LookupByIDs(ctx context.Context, ids []int64) (map[int64]*Food, error)

	// ListChildrenByParents loads the child links of several composed foods
	// at once, keyed by parent, with the child's name + kind populated. Used
	// by the nutrition resolver to load food trees level by level.
	ListChildrenByParents(ctx context.Context, parentIDs []int64) (map[int64][]FoodComponent, error)

	ListPortions(ctx context.Context, foodID int64) ([]Portion, error)
	UpsertPortion(ctx context.Context, p *Portion) error
	DeletePortion(ctx context.Context, foodID int64, unit string) error

	CreateVariantGroup(ctx context.Context, name string) (int64, error)
	Siblings(ctx context.Context, variantGroupID int64, excludeID int64) ([]Food, error)

	SetFavorite(ctx context.Context, id int64, favorite bool) (*Food, error)
}

// ChildExistsChecker lets plate / template services verify a food_id refers
// to an existing food before writing.
type ChildExistsChecker interface {
	Exists(ctx context.Context, id int64) (bool, error)
}
