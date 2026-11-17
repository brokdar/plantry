package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// A routine cell without a plate asks nothing of the day, like a No-plate
// cell: its share leaves the day's target everywhere the target is used.
func TestRoutineCellWithoutPlate_LeavesTheDayTarget(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(1500)
	for _, d := range []string{sat, sun, mon, tue, wed, thu, fri} {
		setMode(&s, cell(d, breakfast), availability.ModeRoutine)
	}

	ev, issues := weekOf(s)

	require.NotEmpty(t, ev.Days)
	for _, d := range ev.Days {
		require.NotNil(t, d.TargetKcal)
		assert.InDelta(t, 1000, *d.TargetKcal, 1e-6, d.Date)
	}
	assert.Empty(t, codeDates(issues, "S2", planning.CodeKcalBelowBand))
	for _, dk := range planning.DayKcals(s, windowTarget(s)) {
		assert.InDelta(t, 1000, dk.Scaled, 1e-6)
	}
	if assert.NotNil(t, ev.Week) {
		assert.InDelta(t, 100, ev.Week.Pct, 1e-6, "the week is in band")
	}
}
