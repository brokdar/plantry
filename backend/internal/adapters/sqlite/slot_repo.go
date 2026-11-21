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
	conn
}

// NewSlotRepo creates a SQLite-backed time slot repository.
func NewSlotRepo(db *sql.DB) *SlotRepo {
	return &SlotRepo{conn: newConn(db)}
}

func (r *SlotRepo) Create(ctx context.Context, s *slot.TimeSlot) error {
	row, err := r.queriesFor(ctx).CreateTimeSlot(ctx, sqlcgen.CreateTimeSlotParams{
		NameKey:    s.NameKey,
		Icon:       s.Icon,
		SortOrder:  int64(s.SortOrder),
		Active:     boolToInt(s.Active),
		Kind:       kindOrMeal(s.Kind),
		BandMinPct: nullFloat(s.BandMinPct), BandMaxPct: nullFloat(s.BandMaxPct),
	})
	if err != nil {
		return err
	}
	mapTimeSlotToDomain(&row, s)
	return nil
}

func (r *SlotRepo) Get(ctx context.Context, id int64) (*slot.TimeSlot, error) {
	row, err := r.queriesFor(ctx).GetTimeSlot(ctx, id)
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
	row, err := r.queriesFor(ctx).UpdateTimeSlot(ctx, sqlcgen.UpdateTimeSlotParams{
		ID:         s.ID,
		NameKey:    s.NameKey,
		Icon:       s.Icon,
		SortOrder:  int64(s.SortOrder),
		Active:     boolToInt(s.Active),
		Kind:       kindOrMeal(s.Kind),
		BandMinPct: nullFloat(s.BandMinPct), BandMaxPct: nullFloat(s.BandMaxPct),
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
	res, err := r.queriesFor(ctx).DeleteTimeSlot(ctx, id)
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
		rows, err = r.queriesFor(ctx).ListActiveTimeSlots(ctx)
	} else {
		rows, err = r.queriesFor(ctx).ListTimeSlots(ctx)
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
	return r.queriesFor(ctx).CountPlatesUsingTimeSlot(ctx, slotID)
}

// Exists reports whether a time slot with the given ID exists.
// Implements plate.SlotChecker.
func (r *SlotRepo) Exists(ctx context.Context, id int64) (bool, error) {
	_, err := r.queriesFor(ctx).GetTimeSlot(ctx, id)
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
	s.Kind = slot.Kind(row.Kind)
	s.BandMinPct, s.BandMaxPct = floatPtr(row.BandMinPct), floatPtr(row.BandMaxPct)
}

func nullFloat(p *float64) sql.NullFloat64 {
	if p == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *p, Valid: true}
}

func floatPtr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	v := n.Float64
	return &v
}

// SetBand stores a slot's own band; nil values restore the default.
func (r *SlotRepo) SetBand(ctx context.Context, id int64, minPct, maxPct *float64) error {
	return r.queriesFor(ctx).SetTimeSlotBand(ctx, sqlcgen.SetTimeSlotBandParams{BandMinPct: nullFloat(minPct), BandMaxPct: nullFloat(maxPct), ID: id})
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// kindOrMeal stores an unset kind as the default, a full meal.
func kindOrMeal(k slot.Kind) string {
	if k == "" {
		return string(slot.KindMeal)
	}
	return string(k)
}
