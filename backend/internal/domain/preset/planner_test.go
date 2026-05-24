package preset_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
)

var plannerTargetDate = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func plannerPresetPlate(slotID int64, sortOrder int) preset.Plate {
	return preset.Plate{SlotID: slotID, SortOrder: sortOrder, Components: []preset.Component{{FoodID: 1}}}
}

func plannerExistingPlate(id, slotID int64) plate.Plate {
	return plate.Plate{ID: id, SlotID: slotID, Date: plannerTargetDate}
}

func activeSlotSet(ids ...int64) map[int64]struct{} {
	m := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

func TestPlanApply(t *testing.T) {
	tests := []struct {
		name            string
		input           preset.ApplyInput
		wantToCreateLen int
		wantToDeleteIDs []int64 // parallel with ToCreate; 0 means no deletion
		wantSkipOccLen  int
		wantSkipSlotLen int
	}{
		{
			name: "empty slots — all preset plates are placed",
			input: preset.ApplyInput{
				Plates: []preset.Plate{
					plannerPresetPlate(10, 0),
					plannerPresetPlate(20, 1),
				},
				TargetDate:     plannerTargetDate,
				ExistingPlates: nil,
				ActiveSlots:    nil, // nil = all active
				OnConflict:     preset.ConflictSkip,
			},
			wantToCreateLen: 2,
			wantToDeleteIDs: []int64{0, 0},
			wantSkipOccLen:  0,
			wantSkipSlotLen: 0,
		},
		{
			name: "fully occupied with ConflictSkip — nothing placed",
			input: preset.ApplyInput{
				Plates: []preset.Plate{
					plannerPresetPlate(10, 0),
					plannerPresetPlate(20, 1),
				},
				TargetDate: plannerTargetDate,
				ExistingPlates: []plate.Plate{
					plannerExistingPlate(100, 10),
					plannerExistingPlate(200, 20),
				},
				ActiveSlots: nil,
				OnConflict:  preset.ConflictSkip,
			},
			wantToCreateLen: 0,
			wantToDeleteIDs: nil,
			wantSkipOccLen:  2,
			wantSkipSlotLen: 0,
		},
		{
			name: "fully occupied with ConflictOverwrite — old plates in ToDeleteBeforeCreate",
			input: preset.ApplyInput{
				Plates: []preset.Plate{
					plannerPresetPlate(10, 0),
					plannerPresetPlate(20, 1),
				},
				TargetDate: plannerTargetDate,
				ExistingPlates: []plate.Plate{
					plannerExistingPlate(100, 10),
					plannerExistingPlate(200, 20),
				},
				ActiveSlots: nil,
				OnConflict:  preset.ConflictOverwrite,
			},
			wantToCreateLen: 2,
			wantToDeleteIDs: []int64{100, 200},
			wantSkipOccLen:  0,
			wantSkipSlotLen: 0,
		},
		{
			name: "partial overlap with ConflictSkip — occupied slot skipped, free slot placed",
			input: preset.ApplyInput{
				Plates: []preset.Plate{
					plannerPresetPlate(10, 0), // free
					plannerPresetPlate(20, 1), // occupied
				},
				TargetDate: plannerTargetDate,
				ExistingPlates: []plate.Plate{
					plannerExistingPlate(200, 20),
				},
				ActiveSlots: nil,
				OnConflict:  preset.ConflictSkip,
			},
			wantToCreateLen: 1,
			wantToDeleteIDs: []int64{0},
			wantSkipOccLen:  1,
			wantSkipSlotLen: 0,
		},
		{
			name: "inactive slot — skipped regardless of occupancy",
			input: preset.ApplyInput{
				Plates: []preset.Plate{
					plannerPresetPlate(10, 0), // active
					plannerPresetPlate(20, 1), // inactive
				},
				TargetDate:     plannerTargetDate,
				ExistingPlates: nil,
				ActiveSlots:    activeSlotSet(10), // only slot 10 active
				OnConflict:     preset.ConflictSkip,
			},
			wantToCreateLen: 1,
			wantToDeleteIDs: []int64{0},
			wantSkipOccLen:  0,
			wantSkipSlotLen: 1,
		},
		{
			name: "duplicate slot_id in preset — first by sort_order wins, second is treated as occupied",
			input: preset.ApplyInput{
				Plates: []preset.Plate{
					plannerPresetPlate(10, 0), // first entry for slot 10
					plannerPresetPlate(10, 1), // second entry for slot 10 (same slot)
				},
				TargetDate:     plannerTargetDate,
				ExistingPlates: nil,
				ActiveSlots:    nil,
				OnConflict:     preset.ConflictSkip,
			},
			wantToCreateLen: 1,
			wantToDeleteIDs: []int64{0},
			wantSkipOccLen:  1,
			wantSkipSlotLen: 0,
		},
		{
			name: "partial overlap with ConflictOverwrite — occupied slot replaced, free slot placed",
			input: preset.ApplyInput{
				Plates: []preset.Plate{
					plannerPresetPlate(10, 0), // free
					plannerPresetPlate(20, 1), // occupied → overwrite
				},
				TargetDate: plannerTargetDate,
				ExistingPlates: []plate.Plate{
					plannerExistingPlate(200, 20),
				},
				ActiveSlots: nil,
				OnConflict:  preset.ConflictOverwrite,
			},
			wantToCreateLen: 2,
			wantToDeleteIDs: []int64{0, 200},
			wantSkipOccLen:  0,
			wantSkipSlotLen: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan := preset.PlanApply(tc.input)

			require.Len(t, plan.ToCreate, tc.wantToCreateLen, "ToCreate len")
			require.Len(t, plan.ToDeleteBeforeCreate, tc.wantToCreateLen,
				"ToDeleteBeforeCreate must be parallel to ToCreate")

			if tc.wantToDeleteIDs != nil {
				for i, wantID := range tc.wantToDeleteIDs {
					assert.Equal(t, wantID, plan.ToDeleteBeforeCreate[i],
						"ToDeleteBeforeCreate[%d]", i)
				}
			}

			assert.Len(t, plan.SkippedOccupied, tc.wantSkipOccLen, "SkippedOccupied len")
			assert.Len(t, plan.SkippedNoSlot, tc.wantSkipSlotLen, "SkippedNoSlot len")
		})
	}
}

func TestPlanApply_SkipItems_CarryCorrectDate(t *testing.T) {
	target := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	plan := preset.PlanApply(preset.ApplyInput{
		Plates:         []preset.Plate{plannerPresetPlate(10, 0)},
		TargetDate:     target,
		ExistingPlates: []plate.Plate{plannerExistingPlate(99, 10)},
		ActiveSlots:    nil,
		OnConflict:     preset.ConflictSkip,
	})
	require.Len(t, plan.SkippedOccupied, 1)
	assert.Equal(t, target, plan.SkippedOccupied[0].Date)
	assert.Equal(t, int64(10), plan.SkippedOccupied[0].SlotID)
}

func TestPlanApply_PreservesComponents(t *testing.T) {
	unit := "g"
	amt := 200.0
	grams := 200.0
	src := "direct"
	comp := preset.Component{
		FoodID: 42, Amount: &amt, Unit: &unit, Grams: &grams, GramsSource: &src,
	}
	plan := preset.PlanApply(preset.ApplyInput{
		Plates: []preset.Plate{
			{SlotID: 10, SortOrder: 0, Components: []preset.Component{comp}},
		},
		TargetDate:     plannerTargetDate,
		ExistingPlates: nil,
		ActiveSlots:    nil,
		OnConflict:     preset.ConflictSkip,
	})
	require.Len(t, plan.ToCreate, 1)
	require.Len(t, plan.ToCreate[0].Components, 1)
	assert.Equal(t, int64(42), plan.ToCreate[0].Components[0].FoodID)
	assert.Equal(t, 200.0, *plan.ToCreate[0].Components[0].Amount)
}
