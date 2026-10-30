package availability

import (
	"context"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Repository is the persistence port for availability.
type Repository interface {
	// ActiveSlotIDs returns the active time slots, ordered by sort_order, id.
	ActiveSlotIDs(ctx context.Context) ([]int64, error)
	// SlotState reports whether the slot exists and whether it is active.
	SlotState(ctx context.Context, slotID int64) (exists, active bool, err error)

	// ListDefaults returns every stored default row.
	ListDefaults(ctx context.Context) ([]Default, error)
	// GetDefault returns the stored default of the cell, or nil when none.
	GetDefault(ctx context.Context, weekday int, slotID int64) (*Default, error)
	// ReplaceActiveDefaults deletes the default rows of every active slot and
	// inserts rows. Rows of inactive slots are kept.
	ReplaceActiveDefaults(ctx context.Context, rows []Default) error

	// ListOverrides returns the overrides whose date is in [from, to].
	ListOverrides(ctx context.Context, from, to time.Time) ([]Override, error)
	// GetOverride returns the override of the cell, or nil when none.
	GetOverride(ctx context.Context, date time.Time, slotID int64) (*Override, error)
	// UpsertOverride inserts or replaces the override of o's cell.
	UpsertOverride(ctx context.Context, o Override) error
	// DeleteOverride removes the override of the cell; a missing row is no error.
	DeleteOverride(ctx context.Context, date time.Time, slotID int64) error
}

// CellPlates finds and removes plates through the plate service, so a
// removed plate's cook sessions stay consistent. It joins the caller's unit
// of work.
type CellPlates interface {
	Range(ctx context.Context, from, to time.Time) ([]plate.Plate, error)
	Delete(ctx context.Context, id int64) (plate.PlanSnapshot, error)
}

// PresetLookup resolves a preset's name. ok is false when it doesn't exist.
type PresetLookup interface {
	PresetName(ctx context.Context, id int64) (name string, ok bool, err error)
}
