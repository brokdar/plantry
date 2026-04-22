package planner

import (
	"context"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// WeekRepository is the port adapters must implement for week persistence.
// It deals with the week row only; plates are loaded via plate.Repository.
type WeekRepository interface {
	Create(ctx context.Context, w *Week) error
	Get(ctx context.Context, id int64) (*Week, error)
	GetByYearAndNumber(ctx context.Context, year, weekNumber int) (*Week, error)
	List(ctx context.Context, limit, offset int) ([]Week, int64, error)
}

// TxRunner runs fn inside a single transaction. The closure receives
// transactional copies of the week and plate repositories so multi-aggregate
// operations (copy-week) are atomic without leaking *sql.DB into the domain.
type TxRunner interface {
	RunInTx(ctx context.Context, fn func(weeks WeekRepository, plates plate.Repository) error) error
}
