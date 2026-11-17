package planning_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// weekOf fills every plan cell of s with one fresh Curry (500 kcal) plate,
// the days named in skip stay empty, and evaluates the merged plan.
func weekOf(s planning.State, skip ...string) (planning.Evaluation, []planning.Issue) {
	tgt := windowTarget(s)
	var prop planning.Proposal
	for _, c := range s.Cells {
		d := c.Date.Format("2006-01-02")
		skipped := false
		for _, k := range skip {
			skipped = skipped || k == d
		}
		if skipped || c.Mode != availability.ModePlan {
			continue
		}
		prop.Plates = append(prop.Plates, pplate(d, c.SlotID, fresh(curry)))
	}
	m := planning.Merge(s, tgt, prop)
	return planning.Evaluate(m, tgt), planning.Check(m, tgt)
}

func codeDates(issues []planning.Issue, rule, code string) []string {
	var out []string
	for _, is := range issues {
		if is.Rule == rule && is.Code == code {
			out = append(out, is.Date)
		}
	}
	return out
}

func TestCheck_KcalFloor(t *testing.T) {
	t.Run("a day below 75 % of its target is a hard violation", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(2400) // floor 1800, the days hold 1500
		_, issues := weekOf(s)
		got := codeDates(issues, "H9", planning.CodeKcalBelowFloor)
		assert.Len(t, got, 7)
		for _, is := range issues {
			if is.Rule == "H9" {
				assert.Equal(t, planning.SeverityViolation, is.Severity)
				assert.InDelta(t, 1800, is.Params["floor_kcal"], 0.01)
			}
		}
	})
	t.Run("at the floor is fine", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(2000) // floor 1500
		_, issues := weekOf(s)
		assert.Empty(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor))
	})
	t.Run("the absolute floor binds when sex is known", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(1600) // 75 % = 1200, male floor 1500: the days hold 1500
		s.Profile.Sex = sexp(profile.SexMale)
		_, issues := weekOf(s)
		assert.Empty(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor))
		s.Profile.KcalTarget = f64(1600)
		s.Foods[curry] = func() planning.FoodFacts { f := s.Foods[curry]; f.Macros.Kcal = 490; return f }()
		_, issues = weekOf(s)
		assert.Len(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor), 7)
	})
	t.Run("a short day scales the floor", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(1800)
		s.Profile.Sex = sexp(profile.SexMale)
		setMode(&s, cell(mon, lunch), availability.ModeNone)
		// Monday: two plates, 1000 kcal = the male floor of 2 of 3 meals.
		_, issues := weekOf(s)
		assert.NotContains(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor), mon)
	})
	t.Run("an incomplete day or no target is not judged", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(5000)
		_, issues := weekOf(s, sat, sun, mon, tue, wed, thu)
		assert.Equal(t, []string{fri}, codeDates(issues, "H9", planning.CodeKcalBelowFloor))
		s.Profile.KcalTarget = nil
		_, issues = weekOf(s)
		assert.Empty(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor))
	})
}

func TestEvaluate_WeekLedger(t *testing.T) {
	cases := []struct {
		name     string
		target   float64
		goal     profile.Goal
		skip     []string
		wantCode string
		wantPct  float64
	}{
		{"on target", 1500, profile.GoalMaintain, nil, "", 100},
		{"just inside the band", 1450, profile.GoalMaintain, nil, "", 103.4},
		{"below", 1700, profile.GoalMaintain, nil, planning.CodeWeekKcalBelowBand, 88.2},
		{"above", 1300, profile.GoalMaintain, nil, planning.CodeWeekKcalAboveBand, 115.4},
		{"lose is stricter above", 1450, profile.GoalLose, nil, planning.CodeWeekKcalAboveBand, 103.4},
		{"too few complete days: no verdict", 1000, profile.GoalMaintain, []string{sat, sun, mon, tue, wed}, "", 150},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := baseState()
			s.Profile.KcalTarget = f64(tc.target)
			s.Profile.Goal = tc.goal
			ev, _ := weekOf(s, tc.skip...)
			require.NotNil(t, ev.Week)
			assert.InDelta(t, tc.wantPct, ev.Week.Pct, 0.1)
			assert.InDelta(t, 1500, ev.Week.AvgKcal, 0.01)
			assert.InDelta(t, tc.target, ev.Week.TargetKcal, 0.01)
			assert.Equal(t, 7-len(tc.skip), ev.Week.Days)
			got := ""
			for _, is := range ev.Issues {
				if is.Rule == "S4" {
					got = is.Code
					assert.Equal(t, planning.SeveritySoft, is.Severity)
					assert.Empty(t, is.Date)
				}
			}
			assert.Equal(t, tc.wantCode, got)
		})
	}

	t.Run("no ledger without a target or a complete day", func(t *testing.T) {
		s := baseState()
		ev, _ := weekOf(s)
		assert.Nil(t, ev.Week)
		s.Profile.KcalTarget = f64(2000)
		ev, _ = weekOf(s, sat, sun, mon, tue, wed, thu, fri)
		assert.Nil(t, ev.Week)
	})

	t.Run("the compact form keeps the ledger", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(1700)
		ev, _ := weekOf(s)
		m := ev.Compact(planning.View{Dates: map[string]bool{sat: true}})
		week, ok := m["week"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "1500", fmt.Sprint(week["avg_kcal"]))
		assert.Len(t, m["days"], 1)
	})
}
