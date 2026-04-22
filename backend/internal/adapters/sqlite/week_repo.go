package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
)

// WeekRepo implements planner.WeekRepository backed by SQLite.
type WeekRepo struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

// NewWeekRepo creates a SQLite-backed week repository.
func NewWeekRepo(db *sql.DB) *WeekRepo {
	return &WeekRepo{db: db, q: sqlcgen.New(db)}
}

// newWeekRepoTx returns a WeekRepo bound to a transaction. Used by TxRunner.
func newWeekRepoTx(tx *sql.Tx) *WeekRepo {
	return &WeekRepo{db: nil, q: sqlcgen.New(tx)}
}

func (r *WeekRepo) Create(ctx context.Context, w *planner.Week) error {
	row, err := r.q.CreateWeek(ctx, sqlcgen.CreateWeekParams{
		Year:       int64(w.Year),
		WeekNumber: int64(w.WeekNumber),
	})
	if err != nil {
		if isUniqueViolation(err, "weeks") {
			return fmt.Errorf("%w: week %d/%d already exists", domain.ErrDuplicateName, w.Year, w.WeekNumber)
		}
		return err
	}
	mapWeekToDomain(&row, w)
	return nil
}

func (r *WeekRepo) Get(ctx context.Context, id int64) (*planner.Week, error) {
	row, err := r.q.GetWeek(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: week %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var w planner.Week
	mapWeekToDomain(&row, &w)
	return &w, nil
}

func (r *WeekRepo) GetByYearAndNumber(ctx context.Context, year, weekNumber int) (*planner.Week, error) {
	row, err := r.q.GetWeekByYearAndNumber(ctx, sqlcgen.GetWeekByYearAndNumberParams{
		Year:       int64(year),
		WeekNumber: int64(weekNumber),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: week %d/%d", domain.ErrNotFound, year, weekNumber)
		}
		return nil, err
	}
	var w planner.Week
	mapWeekToDomain(&row, &w)
	return &w, nil
}

func (r *WeekRepo) List(ctx context.Context, limit, offset int) ([]planner.Week, int64, error) {
	total, err := r.q.CountWeeks(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.q.ListWeeks(ctx, sqlcgen.ListWeeksParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]planner.Week, len(rows))
	for i := range rows {
		mapWeekToDomain(&rows[i], &out[i])
	}
	return out, total, nil
}

func mapWeekToDomain(row *sqlcgen.Week, w *planner.Week) {
	w.ID = row.ID
	w.Year = int(row.Year)
	w.WeekNumber = int(row.WeekNumber)
	w.CreatedAt, _ = time.Parse(timeLayout, row.CreatedAt)
}
