package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// SlotRepo implements slot.Repository backed by SQLite.
type SlotRepo struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

// NewSlotRepo creates a SQLite-backed time slot repository.
func NewSlotRepo(db *sql.DB) *SlotRepo {
	return &SlotRepo{db: db, q: sqlcgen.New(db)}
}

func (r *SlotRepo) Create(ctx context.Context, s *slot.TimeSlot) error {
	row, err := r.q.CreateTimeSlot(ctx, sqlcgen.CreateTimeSlotParams{
		NameKey:   s.NameKey,
		Icon:      s.Icon,
		SortOrder: int64(s.SortOrder),
		Active:    boolToInt(s.Active),
	})
	if err != nil {
		return err
	}
	mapTimeSlotToDomain(&row, s)
	return nil
}

func (r *SlotRepo) Get(ctx context.Context, id int64) (*slot.TimeSlot, error) {
	row, err := r.q.GetTimeSlot(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: id %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var s slot.TimeSlot
	mapTimeSlotToDomain(&row, &s)
	return &s, nil
}

func (r *SlotRepo) Update(ctx context.Context, s *slot.TimeSlot) error {
	row, err := r.q.UpdateTimeSlot(ctx, sqlcgen.UpdateTimeSlotParams{
		ID:        s.ID,
		NameKey:   s.NameKey,
		Icon:      s.Icon,
		SortOrder: int64(s.SortOrder),
		Active:    boolToInt(s.Active),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: id %d", domain.ErrNotFound, s.ID)
		}
		return err
	}
	mapTimeSlotToDomain(&row, s)
	return nil
}

func (r *SlotRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.q.DeleteTimeSlot(ctx, id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: id %d", domain.ErrInUse, id)
		}
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: id %d", domain.ErrNotFound, id)
	}
	return nil
}

func (r *SlotRepo) List(ctx context.Context, activeOnly bool) ([]slot.TimeSlot, error) {
	var rows []sqlcgen.TimeSlot
	var err error
	if activeOnly {
		rows, err = r.q.ListActiveTimeSlots(ctx)
	} else {
		rows, err = r.q.ListTimeSlots(ctx)
	}
	if err != nil {
		return nil, err
	}
	out := make([]slot.TimeSlot, len(rows))
	for i := range rows {
		mapTimeSlotToDomain(&rows[i], &out[i])
	}
	return out, nil
}

func (r *SlotRepo) CountPlatesUsing(ctx context.Context, slotID int64) (int64, error) {
	return r.q.CountPlatesUsingTimeSlot(ctx, slotID)
}

// Exists reports whether a time slot with the given ID exists.
// Implements plate.SlotChecker.
func (r *SlotRepo) Exists(ctx context.Context, id int64) (bool, error) {
	_, err := r.q.GetTimeSlot(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func mapTimeSlotToDomain(row *sqlcgen.TimeSlot, s *slot.TimeSlot) {
	s.ID = row.ID
	s.NameKey = row.NameKey
	s.Icon = row.Icon
	s.SortOrder = int(row.SortOrder)
	s.Active = row.Active != 0
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
