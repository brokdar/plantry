package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// The two reference cases of the paid rerun (lighter-tue, hearty-sat), with
// the saved profile's 2200 kcal target, checked without a paid call.
const (
	refB310 int64 = 9310 + iota
	refL550
	refD620
	refD550
	refL700
	refD917
	refD1100
)

func refState(t *testing.T) planning.State {
	t.Helper()
	s := lightState()
	s.Profile.KcalTarget = f64(2200)
	for id, kcal := range map[int64]float64{refB310: 310, refL550: 550, refD620: 620, refD550: 550, refL700: 700, refD917: 917, refD1100: 1100} {
		s.Foods[id] = recipe(id, "ref", food.RoleMain, "vegan", kcal)
	}
	return s
}

func refDay(t *testing.T, s planning.State, date string, light []planning.LightIntent, foods map[int64]int64) planning.Evaluation {
	t.Helper()
	tgt := windowTarget(s)
	var prop planning.Proposal
	for _, c := range s.Cells {
		d := c.Date.Format("2006-01-02")
		if f, ok := foods[c.SlotID]; ok && d == date {
			prop.Plates = append(prop.Plates, pplate(d, c.SlotID, fresh(f)))
		}
	}
	prop.Light = light
	return planning.Evaluate(planning.Merge(s, tgt, prop), tgt)
}

// Tuesday with lunch and dinner light: the shift -16.7 % is capped at -10 %,
// the target is 1980 and the floor 1485 (75 % of it).
func TestMealShift_ReferenceLighterTuesday(t *testing.T) {
	s := refState(t)
	light := []planning.LightIntent{{Date: tue, Scope: "meal", Slot: "lunch"}, {Date: tue, Scope: "meal", Slot: "dinner"}}

	ev := refDay(t, s, tue, light, map[int64]int64{breakfast: refB310, lunch: refL550, dinner: refD620})
	d := dayEval(t, ev, tue)
	assert.InDelta(t, 1980, *d.TargetKcal, 0.01)
	assert.InDelta(t, 2200, d.NormalTargetKcal, 0.01)
	// 310 + 550 + 620 = 1480, five kcal under the floor 1485 (and the
	// dinner is over its light cap of 550): not free of H9.
	got := codeDates(ev.Issues, "H9", planning.CodeKcalBelowFloor)
	assert.Equal(t, []string{tue}, got)
	for _, is := range ev.Issues {
		if is.Code == planning.CodeKcalBelowFloor {
			assert.InDelta(t, 1485, is.Params["floor_kcal"], 0.01)
			assert.InDelta(t, 1480, is.Params["kcal"], 0.01)
		}
	}
}

// Saturday as a short day (breakfast No plate, target 1467): a hearty dinner
// lifts the target to 1613 (+10 % cap) and a dinner at its minimum
// (125 % of 733 = 917) stays in the band and meets S6b.
func TestMealShift_ReferenceHeartySaturday(t *testing.T) {
	s := refState(t)
	setMode(&s, cell(sat, breakfast), availability.ModeNone)
	hearty := []planning.LightIntent{{Date: sat, Scope: "meal", Slot: "dinner", Level: "hearty"}}

	ev := refDay(t, s, sat, hearty, map[int64]int64{lunch: refL700, dinner: refD917})
	d := dayEval(t, ev, sat)
	require.NotNil(t, d.TargetKcal)
	assert.InDelta(t, 1613.3, *d.TargetKcal, 0.1)
	assert.InDelta(t, 1466.7, d.NormalTargetKcal, 0.1)
	assert.Empty(t, codeDates(ev.Issues, "S6b", planning.CodeMealNotHearty), "917 meets 125 % of 733")
	assert.Empty(t, codeDates(ev.Issues, "S2", planning.CodeKcalBelowBand))
	assert.Empty(t, codeDates(ev.Issues, "S2", planning.CodeKcalAboveBand), "1617 is inside the band 1452-1775")
	assert.Empty(t, codeDates(ev.Issues, "H9", planning.CodeKcalBelowFloor))
}
