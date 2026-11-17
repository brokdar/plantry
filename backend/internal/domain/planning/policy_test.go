package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

func sexp(s profile.Sex) *profile.Sex { return &s }

func TestPolicyOf_Bands(t *testing.T) {
	cases := []struct {
		name                            string
		goal                            profile.Goal
		dayLow, dayHigh, weekLow, weekH float64
	}{
		{"unset goal is maintain", "", 0.9, 1.1, 0.95, 1.05},
		{"maintain", profile.GoalMaintain, 0.9, 1.1, 0.95, 1.05},
		{"lose", profile.GoalLose, 0.9, 1.05, 0.95, 1.03},
		{"gain", profile.GoalGain, 0.95, 1.1, 0.97, 1.05},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := planning.PolicyOf(planning.Profile{Goal: tc.goal})
			assert.InDelta(t, tc.dayLow, p.DayLow, 1e-9)
			assert.InDelta(t, tc.dayHigh, p.DayHigh, 1e-9)
			assert.InDelta(t, tc.weekLow, p.WeekLow, 1e-9)
			assert.InDelta(t, tc.weekH, p.WeekHigh, 1e-9)
		})
	}
}

func TestPolicy_Floor(t *testing.T) {
	cases := []struct {
		name   string
		sex    *profile.Sex
		target float64 // full-day target
		share  float64 // planned meals / meals
		want   float64
	}{
		{"no sex: 75 % of the target only", nil, 2000, 1, 1500},
		{"female: share floor higher than absolute", sexp(profile.SexFemale), 2000, 1, 1500},
		{"female: absolute floor wins", sexp(profile.SexFemale), 1500, 1, 1200},
		{"male: absolute floor wins", sexp(profile.SexMale), 1800, 1, 1500},
		{"partial day scales the absolute floor", sexp(profile.SexMale), 1800, 2.0 / 3.0, 1000},
		{"partial day scales the share floor", nil, 2400, 0.5, 900},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := planning.PolicyOf(planning.Profile{Sex: tc.sex})
			assert.InDelta(t, tc.want, p.Floor(tc.target, tc.share), 1e-6)
		})
	}
}

func TestPolicy_WeekCode(t *testing.T) {
	p := planning.PolicyOf(planning.Profile{Goal: profile.GoalMaintain})
	assert.Equal(t, "", p.WeekCode(1.0))
	assert.Equal(t, "", p.WeekCode(0.95))
	assert.Equal(t, "", p.WeekCode(1.05))
	assert.Equal(t, planning.CodeWeekKcalBelowBand, p.WeekCode(0.94))
	assert.Equal(t, planning.CodeWeekKcalAboveBand, p.WeekCode(1.06))
}
