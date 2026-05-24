package preset

import (
	"sort"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// ApplyInput holds all data PlanApply needs — no repo calls required.
type ApplyInput struct {
	// Plates is the filtered, pre-validated list of preset plates to apply.
	Plates []Plate
	// TargetDate is the calendar date to materialise plates onto.
	TargetDate time.Time
	// ExistingPlates are the plates already occupying the target date.
	ExistingPlates []plate.Plate
	// ActiveSlots is the set of active slot IDs. A nil map means all slots
	// are considered active (matches the behaviour of isSlotActive).
	ActiveSlots map[int64]struct{}
	// OnConflict controls behaviour when a target slot is already occupied.
	OnConflict ConflictMode
}

// PlateToCreate describes one new plate the caller should persist.
type PlateToCreate struct {
	SlotID     int64
	Components []Component
}

// ApplyPlan is the decision the algorithm produces — no side effects.
type ApplyPlan struct {
	// ToCreate holds one entry per preset plate that should be materialised.
	// When a slot must be overwritten, the corresponding plate ID to delete
	// is in ToDeleteBeforeCreate at the same index.
	ToCreate []PlateToCreate
	// ToDeleteBeforeCreate holds the existing plate ID that must be deleted
	// before materialising ToCreate[i]. A value of 0 means no deletion
	// needed (clean slot). Len(ToDeleteBeforeCreate) == Len(ToCreate).
	ToDeleteBeforeCreate []int64
	// SkippedOccupied lists (date, slotID) cells that were skipped due to
	// conflict when OnConflict == ConflictSkip.
	SkippedOccupied []SkipItem
	// SkippedNoSlot lists (date, slotID) cells whose slot was inactive.
	SkippedNoSlot []SkipItem
}

// PlanApply is a pure function: given the current state of the world (preset
// plates + existing occupancy + active slots), it returns a plan describing
// what to create, what to delete, and what to skip. No I/O, no global state.
func PlanApply(in ApplyInput) ApplyPlan {
	// Sort by sort_order so duplicate slot_ids inside a preset land in stable
	// order (first wins; rest hit "occupied" semantics).
	plates := make([]Plate, len(in.Plates))
	copy(plates, in.Plates)
	sort.SliceStable(plates, func(i, j int) bool {
		return plates[i].SortOrder < plates[j].SortOrder
	})

	// Build occupancy map keyed by slot_id.
	occupancy := make(map[int64]plate.Plate, len(in.ExistingPlates))
	for _, ep := range in.ExistingPlates {
		occupancy[ep.SlotID] = ep
	}

	var plan ApplyPlan

	for _, pp := range plates {
		if !isSlotActive(in.ActiveSlots, pp.SlotID) {
			plan.SkippedNoSlot = append(plan.SkippedNoSlot, SkipItem{Date: in.TargetDate, SlotID: pp.SlotID})
			continue
		}

		var deleteID int64
		existingPlate, taken := occupancy[pp.SlotID]
		if taken {
			if in.OnConflict == ConflictSkip {
				plan.SkippedOccupied = append(plan.SkippedOccupied, SkipItem{Date: in.TargetDate, SlotID: pp.SlotID})
				continue
			}
			// Overwrite: record which existing plate must be deleted first.
			deleteID = existingPlate.ID
		}

		plan.ToCreate = append(plan.ToCreate, PlateToCreate{
			SlotID:     pp.SlotID,
			Components: pp.Components,
		})
		plan.ToDeleteBeforeCreate = append(plan.ToDeleteBeforeCreate, deleteID)

		// Mark slot as occupied so duplicate slot_ids in the preset hit the
		// "occupied" path on subsequent iterations.
		occupancy[pp.SlotID] = plate.Plate{SlotID: pp.SlotID}
	}

	return plan
}
