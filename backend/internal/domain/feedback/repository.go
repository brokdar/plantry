package feedback

import (
	"context"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// Repository is the port adapters must implement for plate-feedback persistence.
type Repository interface {
	Upsert(ctx context.Context, f *PlateFeedback) error
	Get(ctx context.Context, plateID int64) (*PlateFeedback, error)
	Delete(ctx context.Context, plateID int64) error
}

// FoodStore loads a food (with its tags) and records that it was cooked.
type FoodStore interface {
	Get(ctx context.Context, id int64) (*food.Food, error)
	MarkCooked(ctx context.Context, id int64, at time.Time) error
}

// ProfileStore reads and writes the user profile.
type ProfileStore interface {
	Get(ctx context.Context) (*profile.Profile, error)
	Update(ctx context.Context, p *profile.Profile) (*profile.Profile, error)
}
