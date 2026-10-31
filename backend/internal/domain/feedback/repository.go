package feedback

import (
	"context"

	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// Repository is the port adapters must implement for plate-feedback persistence.
type Repository interface {
	Upsert(ctx context.Context, f *PlateFeedback) error
	Get(ctx context.Context, plateID int64) (*PlateFeedback, error)
	Delete(ctx context.Context, plateID int64) error
}

// TagReader reads a food's tags.
type TagReader interface {
	ListTags(ctx context.Context, foodID int64) ([]string, error)
}

// ProfileStore reads and writes the user profile.
type ProfileStore interface {
	Get(ctx context.Context) (*profile.Profile, error)
	Update(ctx context.Context, p *profile.Profile) (*profile.Profile, error)
}
