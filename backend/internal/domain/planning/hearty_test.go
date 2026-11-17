package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

func TestValidateLight_Level(t *testing.T) {
	got, err := planning.ValidateLight([]planning.LightIntent{
		{Date: mon, Scope: "meal", Slot: "dinner", Level: "light"},
		{Date: tue, Scope: "day", Level: "hearty"},
		{Date: wed, Scope: "none", Slot: "lunch", Level: "hearty"},
	})
	require.NoError(t, err)
	assert.Equal(t, []planning.LightIntent{
		{Date: mon, Scope: "meal", Slot: "dinner"},
		{Date: tue, Scope: "day", Level: "hearty"},
		{Date: wed, Scope: "none", Slot: "lunch"},
	}, got, "light is the empty default, none keeps its slot and drops the level")

	_, err = planning.ValidateLight([]planning.LightIntent{{Date: mon, Scope: "day", Level: "huge"}})
	assert.Error(t, err)
	_, err = planning.ValidateLight([]planning.LightIntent{{Date: mon, Scope: "none", Slot: "breakfast"}})
	assert.Error(t, err)
}

// C7: a call replaces only the stored intents of the same date and slot.
func TestMergeLight_PerSlot(t *testing.T) {
	lunchL := planning.LightIntent{Date: mon, Scope: "meal", Slot: "lunch"}
	dinnerL := planning.LightIntent{Date: mon, Scope: "meal", Slot: "dinner"}
	dayL := planning.LightIntent{Date: mon, Scope: "day"}
	stored := []planning.LightIntent{lunchL, dinnerL, dayL}
	heartyDinner := planning.LightIntent{Date: mon, Scope: "meal", Slot: "dinner", Level: "hearty"}
	heartyDay := planning.LightIntent{Date: mon, Scope: "day", Level: "hearty"}

	tests := []struct {
		name string
		in   []planning.LightIntent
		want []planning.LightIntent
	}{
		{"meal replaces its slot only", []planning.LightIntent{heartyDinner}, []planning.LightIntent{lunchL, dayL, heartyDinner}},
		{"day replaces the day intent", []planning.LightIntent{heartyDay}, []planning.LightIntent{lunchL, dinnerL, heartyDay}},
		{"none with slot clears the slot", []planning.LightIntent{{Date: mon, Scope: "none", Slot: "lunch"}}, []planning.LightIntent{dinnerL, dayL}},
		{"none without slot clears the date", []planning.LightIntent{{Date: mon, Scope: "none"}}, nil},
		{"other dates stay", []planning.LightIntent{{Date: tue, Scope: "none"}}, stored},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, planning.MergeLight(stored, tt.in))
		})
	}
}

func TestResolveLight_HeartyDaysCountWithLight(t *testing.T) {
	s := lightState()
	s.Light = []planning.LightIntent{{Date: mon, Scope: "day"}, {Date: tue, Scope: "day", Level: "hearty"}}
	assert.NoError(t, planning.ResolveLight(s, []planning.LightIntent{{Date: wed, Scope: "day", Level: "hearty"}}))
	assert.Error(t, planning.ResolveLight(s, []planning.LightIntent{{Date: wed, Scope: "day"}, {Date: "2026-11-12", Scope: "day", Level: "hearty"}}))
}

// A hearty meal must hold at least 125 % of its normal share (S6b); a
// hearty day's target is 115 %.
func TestHearty_MealIssueAndDayTarget(t *testing.T) {
	s := lightState()
	s.Profile.KcalTarget = f64(1500) // 3 meals of 500: a normal share of 500
	ev := weekWith(s, planning.LightIntent{Date: mon, Scope: "meal", Slot: "dinner", Level: "hearty"})
	var got []planning.Issue
	for _, is := range ev.Issues {
		if is.Code == planning.CodeMealNotHearty {
			got = append(got, is)
		}
	}
	require.Len(t, got, 1)
	assert.Equal(t, "S6b", got[0].Rule)
	assert.Equal(t, mon, got[0].Date)
	assert.EqualValues(t, 625, got[0].Params["min_kcal"])
	assert.EqualValues(t, 500, got[0].Params["normal_share_kcal"])
	for _, d := range ev.Days {
		if d.Date == mon {
			assert.Equal(t, "meal", d.Light)
			assert.Equal(t, "hearty", d.LightLevel)
		}
	}

	ev = weekWith(s, planning.LightIntent{Date: tue, Scope: "day", Level: "hearty"})
	for _, d := range ev.Days {
		if d.Date == tue {
			require.NotNil(t, d.TargetKcal)
			assert.InDelta(t, 1725, *d.TargetKcal, 0.01)
			assert.InDelta(t, 1500, d.NormalTargetKcal, 0.01)
			assert.Equal(t, "hearty", d.LightLevel)
		}
	}
}
