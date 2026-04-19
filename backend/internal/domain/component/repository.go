package component

import (
	"context"
	"time"
)

// Repository is the port that adapters must implement for component persistence.
type Repository interface {
	Create(ctx context.Context, c *Component) error
	Get(ctx context.Context, id int64) (*Component, error)
	Update(ctx context.Context, c *Component) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, q ListQuery) (*ListResult, error)
	CreateVariantGroup(ctx context.Context, name string) (int64, error)
	Siblings(ctx context.Context, variantGroupID int64, excludeID int64) ([]Component, error)
	MarkCooked(ctx context.Context, id int64, at time.Time) error
	Insights(ctx context.Context, cutoff time.Time, forgottenLimit, mostCookedLimit int) (Insights, error)
	SetFavorite(ctx context.Context, id int64, favorite bool) (*Component, error)
}
