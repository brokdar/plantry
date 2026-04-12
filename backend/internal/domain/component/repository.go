package component

import "context"

// Repository is the port that adapters must implement for component persistence.
type Repository interface {
	Create(ctx context.Context, c *Component) error
	Get(ctx context.Context, id int64) (*Component, error)
	Update(ctx context.Context, c *Component) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) (*ListResult, error)
}
