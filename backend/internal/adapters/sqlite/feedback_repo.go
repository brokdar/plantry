package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
)

// FeedbackRepo implements feedback.Repository backed by SQLite.
type FeedbackRepo struct {
	q *sqlcgen.Queries
}

// NewFeedbackRepo creates a SQLite-backed feedback repository.
func NewFeedbackRepo(db *sql.DB) *FeedbackRepo {
	return &FeedbackRepo{q: sqlcgen.New(db)}
}

// newFeedbackRepoTx binds a FeedbackRepo to an open transaction.
func newFeedbackRepoTx(tx *sql.Tx) *FeedbackRepo {
	return &FeedbackRepo{q: sqlcgen.New(tx)}
}

func (r *FeedbackRepo) Upsert(ctx context.Context, f *feedback.PlateFeedback) error {
	row, err := r.q.UpsertPlateFeedback(ctx, sqlcgen.UpsertPlateFeedbackParams{
		PlateID: f.PlateID,
		Status:  string(f.Status),
		Note:    toNullString(f.Note),
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: plate %d", domain.ErrNotFound, f.PlateID)
		}
		return err
	}
	f.Status = feedback.Status(row.Status)
	f.Note = fromNullString(row.Note)
	f.RatedAt, _ = time.Parse(timeLayout, row.RatedAt) //nolint:errcheck // layout controlled by migration
	return nil
}

func (r *FeedbackRepo) Get(ctx context.Context, plateID int64) (*feedback.PlateFeedback, error) {
	row, err := r.q.GetPlateFeedback(ctx, plateID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: feedback for plate %d", domain.ErrNotFound, plateID)
		}
		return nil, err
	}
	f := &feedback.PlateFeedback{
		PlateID: row.PlateID,
		Status:  feedback.Status(row.Status),
		Note:    fromNullString(row.Note),
	}
	f.RatedAt, _ = time.Parse(timeLayout, row.RatedAt) //nolint:errcheck
	return f, nil
}

func (r *FeedbackRepo) Delete(ctx context.Context, plateID int64) error {
	res, err := r.q.DeletePlateFeedback(ctx, plateID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: feedback for plate %d", domain.ErrNotFound, plateID)
	}
	return nil
}

func (r *FeedbackRepo) ListByWeek(ctx context.Context, weekID int64) ([]feedback.PlateFeedback, error) {
	rows, err := r.q.ListPlateFeedbackByWeek(ctx, weekID)
	if err != nil {
		return nil, err
	}
	items := make([]feedback.PlateFeedback, len(rows))
	for i, row := range rows {
		items[i] = feedback.PlateFeedback{
			PlateID: row.PlateID,
			Status:  feedback.Status(row.Status),
			Note:    fromNullString(row.Note),
		}
		items[i].RatedAt, _ = time.Parse(timeLayout, row.RatedAt) //nolint:errcheck
	}
	return items, nil
}
