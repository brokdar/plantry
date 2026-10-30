package preset_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
)

var plannerTargetDate = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func plannerPresetPlate(slotID int64, sortOrder int) preset.Plate {
	return preset.Plate{SlotID: slotID, SortOrder: sortOrder, Components: []preset.Component{{FoodID: 1, Portions: ptr(1.0)}}}
}

func activeSlotSet(ids ...int64) map[int64]struct{} {
	m := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		m[id] = struct{}{}
	}
	return m
}

func snapshotCells(snap plate.PlanSnapshot) []string {
	out := []string{}
	for _, p := range snap.Plates {
		out = append(out, fmt.Sprintf("%s/%d", p.Date, p.SlotID))
	}
	return out
}

func TestPresetSnapshot(t *testing.T) {
	tests := []struct {
		name          string
		plates        []preset.Plate
		active        map[int64]struct{}
		wantCells     []string
		wantNoSlot    []plate.CellKey
		wantDuplicate []plate.CellKey
	}{
		{
			name:      "every plate placed in sort order",
			plates:    []preset.Plate{plannerPresetPlate(2, 1), plannerPresetPlate(1, 0)},
			wantCells: []string{"2026-06-01/1", "2026-06-01/2"},
		},
		{
			name:       "inactive slot reported as no slot",
			plates:     []preset.Plate{plannerPresetPlate(1, 0), plannerPresetPlate(2, 1)},
			active:     activeSlotSet(1),
			wantCells:  []string{"2026-06-01/1"},
			wantNoSlot: []plate.CellKey{{Date: plannerTargetDate, SlotID: 2}},
		},
		{
			name:          "a second plate for a slot is a duplicate",
			plates:        []preset.Plate{plannerPresetPlate(1, 0), plannerPresetPlate(1, 1)},
			wantCells:     []string{"2026-06-01/1"},
			wantDuplicate: []plate.CellKey{{Date: plannerTargetDate, SlotID: 1}},
		},
		{
			name:      "empty preset",
			wantCells: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap, noSlot, dup := preset.PresetSnapshot(tt.plates, plannerTargetDate, tt.active)
			assert.Equal(t, tt.wantCells, snapshotCells(snap))
			assert.Equal(t, tt.wantNoSlot, noSlot)
			assert.Equal(t, tt.wantDuplicate, dup)
			assert.Empty(t, snap.Sessions, "preset plates get fresh sessions")
		})
	}
}

func TestPresetSnapshot_CopiesComponents(t *testing.T) {
	amount, unit, grams := 150.0, "g", 150.0
	pp := preset.Plate{SlotID: 1, Components: []preset.Component{
		{FoodID: 7, Portions: ptr(1.5)},
		{FoodID: 9, Amount: &amount, Unit: &unit, Grams: &grams},
	}}
	snap, _, _ := preset.PresetSnapshot([]preset.Plate{pp}, plannerTargetDate, nil)
	require.Len(t, snap.Plates, 1)
	p := snap.Plates[0]
	assert.Zero(t, p.Eaters, "eaters default to the household on apply")
	require.Len(t, p.Components, 2)
	assert.Equal(t, plate.SnapshotComponent{FoodID: 7, Portions: ptr(1.5)}, p.Components[0])
	assert.Equal(t, plate.SnapshotComponent{FoodID: 9, Amount: &amount, Unit: &unit, Grams: &grams, SortOrder: 1}, p.Components[1])
}

func TestCopySnapshot(t *testing.T) {
	src := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC) // Mon
	d := func(offset int) time.Time { return src.AddDate(0, 0, offset) }
	sid := func(v int64) *int64 { return &v }
	note := "double"
	cooked := time.Date(2026, 11, 2, 19, 0, 0, 0, time.UTC)
	sessions := map[int64]plate.CookSession{
		// Mon dinner batch: cooked in the window.
		7: {ID: 7, FoodID: 42, CookDate: d(0), CookSlotID: 2, Portions: 6, FrozenPortions: 2, CookedAt: &cooked, Note: &note},
		// Cooked the week before.
		8: {ID: 8, FoodID: 43, CookDate: d(-2), CookSlotID: 2, Portions: 4},
	}
	sources := []plate.Plate{
		{Date: d(2), SlotID: 2, Eaters: 1.5, Note: &note, Components: []plate.PlateComponent{{FoodID: 42, Portions: ptr(1.0), CookSessionID: sid(7)}}},
		{Date: d(0), SlotID: 2, Eaters: 2, Components: []plate.PlateComponent{{FoodID: 42, Portions: ptr(1.0), CookSessionID: sid(7)}}},
		{Date: d(0), SlotID: 1, Eaters: 2, Components: []plate.PlateComponent{{FoodID: 43, Portions: ptr(1.0), CookSessionID: sid(8)}}},
		{Date: d(1), SlotID: 3, Eaters: 1, Components: []plate.PlateComponent{{FoodID: 42, Portions: ptr(1.0), CookSessionID: sid(7)}}},
	}

	snap, noSlot := preset.CopySnapshot(sources, sessions, src, 7, activeSlotSet(1, 2))

	assert.Equal(t, []plate.CellKey{{Date: d(8), SlotID: 3}}, noSlot)
	require.Len(t, snap.Plates, 3)
	assert.Equal(t, []string{"2026-11-09/1", "2026-11-09/2", "2026-11-11/2"}, snapshotCells(snap))
	assert.Nil(t, snap.Plates[0].Components[0].Session, "a session cooked outside the window becomes fresh")
	assert.Equal(t, 1.5, snap.Plates[2].Eaters)
	assert.Equal(t, &note, snap.Plates[2].Note)
	for _, p := range snap.Plates[1:] {
		assert.Equal(t, &plate.SessionLink{Ref: "s1"}, p.Components[0].Session)
	}
	assert.Equal(t, []plate.SnapshotSession{{
		Ref: "s1", FoodID: 42, CookDate: "2026-11-09", CookSlotID: 2, Portions: 6, Note: &note,
	}}, snap.Sessions, "copied shifted, same portions, not frozen, not cooked")
}
