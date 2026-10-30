package plate_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// The snapshot is a wire format (D22): its JSON shape is part of the API.
func TestPlanSnapshot_JSONShape(t *testing.T) {
	one, ten, unit := 1.0, 10.0, "g"
	cooked := time.Date(2026, 11, 1, 18, 0, 0, 0, time.UTC)
	keep := int64(12)
	snap := plate.PlanSnapshot{
		Plates: []plate.SnapshotPlate{{
			PlateID: &keep, Date: "2026-11-01", SlotID: 2, Eaters: 2,
			Feedback: &plate.SnapshotFeedback{Status: "loved", RatedAt: cooked},
			Components: []plate.SnapshotComponent{
				{FoodID: 4, Portions: &one, Session: &plate.SessionLink{Ref: "s1"}},
				{FoodID: 5, Portions: &one, Session: &plate.SessionLink{ID: 9}, SortOrder: 1},
				{FoodID: 6, Amount: &ten, Unit: &unit, Grams: &ten, SortOrder: 2},
			},
		}},
		Sessions: []plate.SnapshotSession{{Ref: "s1", FoodID: 4, CookDate: "2026-11-01", CookSlotID: 2, Portions: 6, CookedAt: &cooked}},
	}
	raw, err := json.Marshal(snap)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"plates": [{
			"plate_id": 12, "date": "2026-11-01", "slot_id": 2, "eaters": 2, "note": null,
			"feedback": {"status": "loved", "note": null, "rated_at": "2026-11-01T18:00:00Z"},
			"components": [
				{"food_id": 4, "portions": 1, "sort_order": 0, "session": {"ref": "s1"}},
				{"food_id": 5, "portions": 1, "sort_order": 1, "session": {"id": 9}},
				{"food_id": 6, "amount": 10, "unit": "g", "grams": 10, "sort_order": 2}
			]
		}],
		"sessions": [{"ref": "s1", "food_id": 4, "cook_date": "2026-11-01", "cook_slot_id": 2,
			"portions": 6, "frozen_portions": 0, "cooked_at": "2026-11-01T18:00:00Z", "note": null}]
	}`, string(raw))

	var back plate.PlanSnapshot
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, snap, back)
}
