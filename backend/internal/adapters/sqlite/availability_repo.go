package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
)

// AvailabilityRepo implements availability.Repository backed by SQLite.
type AvailabilityRepo struct {
	q *sqlcgen.Queries
}

// NewAvailabilityRepo creates a SQLite-backed availability repository.
func NewAvailabilityRepo(db *sql.DB) *AvailabilityRepo {
	return &AvailabilityRepo{q: sqlcgen.New(db)}
}

// newAvailabilityRepoTx returns an AvailabilityRepo bound to a transaction.
func newAvailabilityRepoTx(tx *sql.Tx) *AvailabilityRepo {
	return &AvailabilityRepo{q: sqlcgen.New(tx)}
}

func (r *AvailabilityRepo) ActiveSlotIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.q.ListActiveTimeSlots(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	return ids, nil
}

func (r *AvailabilityRepo) SlotState(ctx context.Context, slotID int64) (exists, active bool, err error) {
	row, err := r.q.GetTimeSlot(ctx, slotID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return true, row.Active != 0, nil
}

func (r *AvailabilityRepo) ListDefaults(ctx context.Context) ([]availability.Default, error) {
	rows, err := r.q.ListAvailabilityDefaults(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]availability.Default, len(rows))
	for i, row := range rows {
		out[i] = availability.Default{
			Weekday:    int(row.Weekday),
			SlotID:     row.SlotID,
			Mode:       availability.Mode(row.Mode),
			PresetID:   fromNullInt64(row.PresetID),
			PresetName: fromNullString(row.PresetName),
		}
	}
	return out, nil
}

func (r *AvailabilityRepo) GetDefault(ctx context.Context, weekday int, slotID int64) (*availability.Default, error) {
	row, err := r.q.GetAvailabilityDefault(ctx, sqlcgen.GetAvailabilityDefaultParams{Weekday: int64(weekday), SlotID: slotID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &availability.Default{
		Weekday:    int(row.Weekday),
		SlotID:     row.SlotID,
		Mode:       availability.Mode(row.Mode),
		PresetID:   fromNullInt64(row.PresetID),
		PresetName: fromNullString(row.PresetName),
	}, nil
}

// ReplaceActiveDefaults must run inside a transaction (TxRunner) to be atomic.
func (r *AvailabilityRepo) ReplaceActiveDefaults(ctx context.Context, rows []availability.Default) error {
	if err := r.q.DeleteActiveSlotAvailabilityDefaults(ctx); err != nil {
		return fmt.Errorf("delete defaults: %w", err)
	}
	for _, d := range rows {
		if err := r.q.InsertAvailabilityDefault(ctx, sqlcgen.InsertAvailabilityDefaultParams{
			Weekday:  int64(d.Weekday),
			SlotID:   d.SlotID,
			Mode:     string(d.Mode),
			PresetID: toNullInt64(d.PresetID),
		}); err != nil {
			if isForeignKeyViolation(err) {
				// The slot or preset was deleted after validation.
				return r.missingRef(ctx, d.SlotID, domain.ErrAvailabilitySlotUnknown)
			}
			return fmt.Errorf("insert default weekday %d slot %d: %w", d.Weekday, d.SlotID, err)
		}
	}
	return nil
}

func (r *AvailabilityRepo) ListOverrides(ctx context.Context, from, to time.Time) ([]availability.Override, error) {
	rows, err := r.q.ListAvailabilityOverridesByRange(ctx, sqlcgen.ListAvailabilityOverridesByRangeParams{
		FromDate: from.Format(dateLayout),
		ToDate:   to.Format(dateLayout),
	})
	if err != nil {
		return nil, err
	}
	out := make([]availability.Override, len(rows))
	for i, row := range rows {
		o, err := mapOverride(row.Date, row.SlotID, row.Mode, row.PresetID, row.Note, row.PresetName)
		if err != nil {
			return nil, err
		}
		out[i] = o
	}
	return out, nil
}

func (r *AvailabilityRepo) GetOverride(ctx context.Context, date time.Time, slotID int64) (*availability.Override, error) {
	row, err := r.q.GetAvailabilityOverride(ctx, sqlcgen.GetAvailabilityOverrideParams{Date: date.Format(dateLayout), SlotID: slotID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o, err := mapOverride(row.Date, row.SlotID, row.Mode, row.PresetID, row.Note, row.PresetName)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *AvailabilityRepo) UpsertOverride(ctx context.Context, o availability.Override) error {
	err := r.q.UpsertAvailabilityOverride(ctx, sqlcgen.UpsertAvailabilityOverrideParams{
		Date:     o.Date.Format(dateLayout),
		SlotID:   o.SlotID,
		Mode:     string(o.Mode),
		PresetID: toNullInt64(o.PresetID),
		Note:     toNullString(o.Note),
	})
	if isForeignKeyViolation(err) {
		// The slot or preset was deleted after validation. The slot is the
		// URL resource, so a missing one is not found.
		return r.missingRef(ctx, o.SlotID, domain.ErrNotFound)
	}
	return err
}

// missingRef names the row behind a foreign key failure on slotID or its
// preset: unknownSlot when the slot is gone, else the preset.
func (r *AvailabilityRepo) missingRef(ctx context.Context, slotID int64, unknownSlot error) error {
	if _, err := r.q.GetTimeSlot(ctx, slotID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: slot %d", unknownSlot, slotID)
		}
		return fmt.Errorf("check slot %d: %w", slotID, err)
	}
	return fmt.Errorf("%w: preset of slot %d", domain.ErrAvailabilityPresetUnknown, slotID)
}

func (r *AvailabilityRepo) DeleteOverride(ctx context.Context, date time.Time, slotID int64) error {
	return r.q.DeleteAvailabilityOverride(ctx, sqlcgen.DeleteAvailabilityOverrideParams{Date: date.Format(dateLayout), SlotID: slotID})
}

func mapOverride(date string, slotID int64, mode string, presetID sql.NullInt64, note, presetName sql.NullString) (availability.Override, error) {
	d, err := time.Parse(dateLayout, date)
	if err != nil {
		return availability.Override{}, fmt.Errorf("parse override date %q: %w", date, err)
	}
	return availability.Override{
		Date:       d,
		SlotID:     slotID,
		Mode:       availability.Mode(mode),
		PresetID:   fromNullInt64(presetID),
		Note:       fromNullString(note),
		PresetName: fromNullString(presetName),
	}, nil
}
