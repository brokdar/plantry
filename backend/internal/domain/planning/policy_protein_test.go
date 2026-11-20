package planning_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

func TestPolicyOf_ProteinPerKg(t *testing.T) {
	now := time.Now().Year()
	cases := []struct {
		name     string
		weight   *float64
		goal     profile.Goal
		birth    *int
		override *float64
		want     float64 // g per kg, 0 = share fallback
		wantG    float64 // full-day target in g
	}{
		{"no weight falls back to the share", nil, profile.GoalLose, nil, nil, 0, 0},
		{"maintain 1.0", f64(70), profile.GoalMaintain, nil, nil, 1.0, 70},
		{"unset goal is maintain", f64(70), "", nil, nil, 1.0, 70},
		{"lose 1.6", f64(70), profile.GoalLose, nil, nil, 1.6, 112},
		{"gain 1.6", f64(80), profile.GoalGain, nil, nil, 1.6, 128},
		{"65+ maintain 1.0 (DGE)", f64(70), profile.GoalMaintain, intp(now - 65), nil, 1.0, 70},
		{"65+ keeps the higher lose default", f64(70), profile.GoalLose, intp(now - 70), nil, 1.6, 112},
		{"65+ raises an override below 1.0 to 1.0", f64(70), profile.GoalMaintain, intp(now - 66), f64(0.9), 1.0, 70},
		{"under 65 keeps the low override", f64(70), profile.GoalMaintain, intp(now - 64), f64(0.9), 0.9, 63},
		{"65+ keeps an override above 1.0", f64(70), profile.GoalMaintain, intp(now - 66), f64(1.4), 1.4, 98},
		{"override wins", f64(70), profile.GoalMaintain, nil, f64(1.8), 1.8, 126},
		{"override at the upper bound", f64(70), profile.GoalLose, nil, f64(2.2), 2.2, 154},
		{"never below the DGE floor", f64(70), profile.GoalMaintain, nil, f64(0.5), 0.8, 56},
		{"override without weight is inert", nil, profile.GoalMaintain, nil, f64(1.8), 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := planning.PolicyOf(planning.Profile{WeightKg: tc.weight, Goal: tc.goal, BirthYear: tc.birth, ProteinGPerKg: tc.override})
			assert.InDelta(t, tc.want, p.ProteinGPerKg, 1e-9)
			assert.InDelta(t, tc.wantG, p.ProteinTargetG(1), 1e-9)
		})
	}
	t.Run("a short day scales the target", func(t *testing.T) {
		p := planning.PolicyOf(planning.Profile{WeightKg: f64(90)})
		assert.InDelta(t, 60, p.ProteinTargetG(2.0/3.0), 1e-9)
	})
}

// CC7: the order is the spec's (2.5): carb and fat ranges rank above preferences.
func TestPolicy_DescribeRanksS5Lowest(t *testing.T) {
	d := planning.PolicyOf(planning.Profile{}).Describe()
	assert.Contains(t, d, "6 carb and fat ranges")
	assert.Contains(t, d, "7 preferences and effort")
	assert.Contains(t, d, "never at the cost of kcal or protein")
}

func TestPolicyOf_MacroRanges(t *testing.T) {
	cases := []struct {
		name                   string
		carbs, fat             *float64
		cLow, cHigh, fLow, fHi float64
	}{
		{"AMDR by default", nil, nil, 45, 65, 20, 35},
		{"the user's share +-5 points", f64(50), f64(30), 45, 55, 25, 35},
		{"only fat set", nil, f64(40), 45, 65, 35, 45},
		{"low shares stop at zero", f64(3), f64(2), 0, 8, 0, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := planning.PolicyOf(planning.Profile{CarbsPct: tc.carbs, FatPct: tc.fat})
			assert.InDelta(t, tc.cLow, p.CarbsLow, 1e-9)
			assert.InDelta(t, tc.cHigh, p.CarbsHigh, 1e-9)
			assert.InDelta(t, tc.fLow, p.FatLow, 1e-9)
			assert.InDelta(t, tc.fHi, p.FatHigh, 1e-9)
		})
	}
}

// With a weight the g/kg target replaces the share margin: a day under 90 %
// of it is S3, the week under 100 % too; carbs and fat are judged weekly.
func TestEvaluate_ProteinPerKgAndMacroRanges(t *testing.T) {
	// weekOf: every meal a Curry, 25 g protein, 20 % fat, 50 % carbs: 75 g a day.
	cases := []struct {
		name        string
		weight      float64
		carbs, fat  *float64
		skip        []string
		wantDayS3   int
		wantWeekS3  bool
		wantCarbsS5 bool
		wantFatS5   bool
	}{
		{"on target (70 kg x 1.0 = 70 g, day 75 g)", 70, nil, nil, nil, 0, false, false, false},
		{"day under 90 %, week under 100 %", 90, nil, nil, nil, 7, true, false, false}, // 90 g: 75/90 = 83 %
		{"a day at 94 % passes the day check but the week fails", 80, nil, nil, nil, 0, true, false, false},
		{"the user's carb share narrows the range", 70, f64(40), nil, nil, 0, false, true, false}, // 35-45 vs 50
		{"the user's fat share narrows the range", 70, nil, f64(30), nil, 0, false, false, true},  // 25-35 vs 20
		{"too few days: no week verdict", 90, nil, nil, []string{sat, sun, mon, tue, wed}, 2, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := baseState()
			s.Profile.KcalTarget = f64(1500)
			s.Profile.WeightKg = f64(tc.weight)
			s.Profile.CarbsPct, s.Profile.FatPct = tc.carbs, tc.fat
			ev, _ := weekOf(s, tc.skip...)
			dayS3, weekS3, carbs, fat := 0, false, false, false
			for _, is := range ev.Issues {
				switch is.Code {
				case planning.CodeProteinLow:
					dayS3++
					assert.NotEmpty(t, is.Date)
					assert.Contains(t, is.Params, "target_g")
				case planning.CodeWeekProteinLow:
					weekS3 = true
					assert.Empty(t, is.Date)
				case planning.CodeWeekCarbsRange:
					carbs = true
				case planning.CodeWeekFatRange:
					fat = true
				}
			}
			assert.Equal(t, tc.wantDayS3, dayS3)
			assert.Equal(t, tc.wantWeekS3, weekS3)
			assert.Equal(t, tc.wantCarbsS5, carbs)
			assert.Equal(t, tc.wantFatS5, fat)
		})
	}

	t.Run("without a weight the share logic is unchanged", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(1500)
		s.Profile.ProteinPct = f64(40) // days hold 20 %
		ev, _ := weekOf(s)
		n := 0
		for _, is := range ev.Issues {
			if is.Code == planning.CodeProteinLow {
				n++
				assert.Contains(t, is.Params, "target_pct")
			}
			assert.NotEqual(t, planning.CodeWeekProteinLow, is.Code)
		}
		assert.Equal(t, 7, n)
	})

	t.Run("without a weight S5 still applies to carbs and fat, protein stays on the share", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(1500)
		s.Profile.FatPct = f64(40) // range 35-45 %: the days hold 20 %
		s.Profile.ProteinPct = f64(40)
		ev, _ := weekOf(s)
		s5 := map[string]map[string]any{}
		protein := 0
		for _, is := range ev.Issues {
			if is.Rule == "S5" {
				s5[is.Code] = is.Params
			}
			if is.Code == planning.CodeProteinLow {
				protein++
				assert.Contains(t, is.Params, "target_pct")
			}
			assert.NotEqual(t, planning.CodeWeekProteinLow, is.Code)
		}
		require.Contains(t, s5, planning.CodeWeekFatRange)
		assert.InDelta(t, 20, s5[planning.CodeWeekFatRange]["pct"], 0.01)
		assert.InDelta(t, 35, s5[planning.CodeWeekFatRange]["range_low_pct"], 1e-9)
		assert.InDelta(t, 45, s5[planning.CodeWeekFatRange]["range_high_pct"], 1e-9)
		assert.NotContains(t, s5, planning.CodeWeekCarbsRange, "carbs 50 % sit inside the default 45-65 %")
		assert.Equal(t, 7, protein, "the share logic is unchanged")
	})

	t.Run("a short day scales its protein target", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(1500)
		s.Profile.WeightKg = f64(90) // 90 g a full day, 60 g with two of three meals
		setMode(&s, cell(sat, breakfast), "none")
		ev, _ := weekOf(s)
		for _, d := range ev.Days {
			if d.Date == sat {
				assert.InDelta(t, 60, d.ProteinTargetG, 1e-9)
				assert.InDelta(t, 50, d.ProteinG, 1e-9) // 50 / 60 = 83 %
			}
		}
	})

	t.Run("the week ledger shows protein, macros and fibre only with data", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(1500)
		s.Profile.WeightKg = f64(70)
		ev, _ := weekOf(s)
		assert.InDelta(t, 75, ev.Week.AvgProteinG, 1e-6)
		assert.InDelta(t, 70, ev.Week.TargetProteinG, 1e-6)
		assert.InDelta(t, 107.1, ev.Week.ProteinPct, 0.1)
		assert.InDelta(t, 50, ev.Week.CarbsPct, 0.01)
		assert.InDelta(t, 20, ev.Week.FatPct, 0.01)
		assert.Zero(t, ev.Week.FiberG, "no fibre data in the library: not shown")

		c := s.Foods[curry]
		c.Macros.Fiber = 10
		s.Foods[curry] = c
		ev, _ = weekOf(s)
		assert.InDelta(t, 30, ev.Week.FiberG, 1e-6)
		m := ev.Compact(planning.View{Dates: map[string]bool{sat: true}})
		assert.Contains(t, m["week"], "fiber_g")
		for _, is := range ev.Issues {
			assert.NotContains(t, is.Code, "fiber")
		}
	})

	t.Run("a food without fibre data hides the weekly fibre average", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(1500)
		for _, id := range []int64{curry, chili} {
			f := s.Foods[id]
			f.Macros.Fiber = 10
			s.Foods[id] = f
		}
		build := func(missing bool) planning.Evaluation {
			f := s.Foods[chili]
			f.FiberMissing = missing
			s.Foods[chili] = f
			tgt := windowTarget(s)
			var prop planning.Proposal
			for i, c := range s.Cells {
				if c.Mode != availability.ModePlan {
					continue
				}
				id := curry
				if i == 0 {
					id = chili
				}
				prop.Plates = append(prop.Plates, pplate(c.Date.Format("2006-01-02"), c.SlotID, fresh(id)))
			}
			return planning.Evaluate(planning.Merge(s, tgt, prop), tgt)
		}
		assert.Positive(t, build(false).Week.FiberG, "all foods have fibre: shown")
		ev := build(true)
		assert.Zero(t, ev.Week.FiberG, "one food lacks fibre: no partial average")
		m := ev.Compact(planning.View{Dates: map[string]bool{sat: true}})
		assert.NotContains(t, m["week"], "fiber_g")
	})
}
