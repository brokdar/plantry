package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// Every day row carries its band, floor and room, a marked meal's plate its
// intent with the cap or minimum, and the marks name their slot (audit C1).
func TestEvaluate_DayBandFloorRoomAndMealIntent(t *testing.T) {
	s := lightState()
	s.Profile.KcalTarget = f64(1500) // 3 meals of 500
	ev := weekWith(s,
		planning.LightIntent{Date: mon, Scope: "meal", Slot: "dinner"},
		planning.LightIntent{Date: mon, Scope: "meal", Slot: "lunch", Level: "hearty"},
	)

	var day planning.DayEval
	for _, d := range ev.Days {
		if d.Date == mon {
			day = d
		}
	}
	require.NotNil(t, day.TargetKcal)
	assert.InDelta(t, *day.TargetKcal*0.9, day.BandLow, 0.51)
	assert.InDelta(t, *day.TargetKcal*1.1, day.BandHigh, 0.51)
	assert.InDelta(t, 1200, day.FloorKcal, 0.51, "the absolute floor beats 75% of the normal target here")
	assert.InDelta(t, max(0, day.BandHigh-day.Kcal), day.RoomKcal, 0.51)
	assert.Equal(t, []string{"hearty lunch", "light dinner"}, day.LightMeals)

	for _, p := range ev.Plates {
		if p.Date != mon {
			continue
		}
		switch p.SlotID {
		case dinner:
			assert.Equal(t, "light", p.Intent)
			assert.InDelta(t, 375, p.MaxKcal, 0.51)
			assert.Zero(t, p.MinKcal)
		case lunch:
			assert.Equal(t, "hearty", p.Intent)
			assert.InDelta(t, 625, p.MinKcal, 0.51)
			assert.Zero(t, p.MaxKcal)
		default:
			assert.Empty(t, p.Intent)
		}
	}
}
