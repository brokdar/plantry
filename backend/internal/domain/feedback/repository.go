package feedback

import (
	"context"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// Repository is the port adapters must implement for plate-feedback persistence.
type Repository interface {
	Upsert(ctx context.Context, f *PlateFeedback) error
	Get(ctx context.Context, plateID int64) (*PlateFeedback, error)
	Delete(ctx context.Context, plateID int64) error
	ListByWeek(ctx context.Context, weekID int64) ([]PlateFeedback, error)
}

// TxRunner runs fn inside a single transaction. The closure receives
// transactional copies of the feedback, food, and profile repositories so
// recording a feedback event atomically updates feedback rows, cook-count on
// each food of the plate, and the profile's preferences JSON.
type TxRunner interface {
	RunInFeedbackTx(ctx context.Context, fn func(Repository, food.Repository, profile.Repository) error) error
}
