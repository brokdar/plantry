package component

import "context"

// Repository is the port that adapters must implement for component persistence.
//
// Deferred methods (added when their phase lands):
//   - Siblings(ctx, variantGroupID int64) ([]Component, error) — Phase 4
//   - MarkCooked(ctx, id int64, at time.Time) error            — Phase 5
type Repository interface {
	Create(ctx context.Context, c *Component) error
	Get(ctx context.Context, id int64) (*Component, error)
	Update(ctx context.Context, c *Component) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) (*ListResult, error)
}
