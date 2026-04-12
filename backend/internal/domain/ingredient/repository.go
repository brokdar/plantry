package ingredient

import "context"

// Repository is the port that adapters must implement for ingredient persistence.
type Repository interface {
	Create(ctx context.Context, i *Ingredient) error
	Get(ctx context.Context, id int64) (*Ingredient, error)
	Update(ctx context.Context, i *Ingredient) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) (*ListResult, error)
	ListPortions(ctx context.Context, ingredientID int64) ([]Portion, error)
	UpsertPortion(ctx context.Context, p *Portion) error
	DeletePortion(ctx context.Context, ingredientID int64, unit string) error
}
