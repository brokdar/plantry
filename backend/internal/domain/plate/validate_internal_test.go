package plate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateSnapshot_ReturnsParsedDates: Apply uses the dates validation
// parsed instead of re-parsing and discarding the error.
func TestValidateSnapshot_ReturnsParsedDates(t *testing.T) {
	snap := PlanSnapshot{
		Sessions: []SnapshotSession{{Ref: "s1", FoodID: 1, CookDate: "2026-11-01", CookSlotID: 3, Portions: 2}},
		Plates: []SnapshotPlate{
			{Date: "2026-11-02", SlotID: 3},
			{Date: "2026-11-03", SlotID: 2},
		},
	}
	sessions, cells, err := validateSnapshot(snap)
	require.NoError(t, err)

	day := func(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }
	assert.Equal(t, []CellKey{{Date: day(2), SlotID: 3}, {Date: day(3), SlotID: 2}}, cells)
	require.Contains(t, sessions, "s1")
	assert.Equal(t, day(1), sessions["s1"].cookDate)
	assert.Equal(t, int64(1), sessions["s1"].FoodID)
}
