package agent

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

func rd(d int) time.Time { return time.Date(2026, 11, 20+d, 0, 0, 0, 0, time.UTC) } // d 1 = Sat 21 Nov

func f64(v float64) *float64 { return &v }

// reportState: Fri dinner (slot 2) cooks Salmon (batch 1) eaten Sat and Sun
// dinner. portions is what the batch was cooked for, eat what each plate eats.
func reportState(portions, eatFri, eatSat float64) planning.State {
	comp := func(p float64) []planning.Component {
		return []planning.Component{{ID: 1, FoodID: 1, Portions: f64(p), SessionID: 1, Position: plate.PositionMain}}
	}
	return planning.State{
		Window:    window.Window{Start: rd(1), End: rd(7)},
		SlotOrder: plate.SlotOrder{1: 0, 2: 1}, SlotNames: map[int64]string{1: "slot.lunch", 2: "slot.dinner"},
		Foods: map[int64]planning.FoodFacts{1: {ID: 1, Name: "Salmon"}, 2: {ID: 2, Name: "Chili"}},
		Plates: []planning.Plate{
			{ID: 10, Date: rd(1), SlotID: 2, Eaters: 1, Components: comp(eatFri)},
			{ID: 11, Date: rd(2), SlotID: 2, Eaters: 1, Components: comp(eatSat)},
		},
		Sessions: []planning.Session{{CookSession: plate.CookSession{ID: 1, FoodID: 1, CookDate: rd(1), CookSlotID: 2, Portions: portions}}},
	}
}

var (
	reportRuleToken = regexp.MustCompile(`\b[HS]\d+b?\b`)
	reportISODate   = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
)

func TestBuildReport_Items(t *testing.T) {
	before := reportState(2, 1, 1)
	bev := planning.Evaluation{
		Plates: []planning.PlateEval{{PlateID: 10, Kcal: 800}, {PlateID: 11, Kcal: 800}},
		Days:   []planning.DayEval{{Date: "2026-11-21", Kcal: 2000, BandLow: 1800, BandHigh: 2200, CookOccasions: 1}},
		Week:   &planning.WeekEval{Days: 7, MeatG: 321, Pct: 100},
	}
	meat := planning.Issue{Rule: "S7", Code: planning.CodeWeekMeatAboveDGE, Severity: planning.SeveritySoft, Params: map[string]any{"meat_g": 400.0, "max_g": 300.0}}
	tests := []struct {
		name  string
		after planning.State
		aev   planning.Evaluation
		check func(t *testing.T, r changeReport)
	}{
		{
			name:  "changed meal and its day: kcal before and after, range and status",
			after: reportState(1.5, 0.5, 1),
			aev: planning.Evaluation{
				Plates: []planning.PlateEval{{PlateID: 10, Kcal: 400}, {PlateID: 11, Kcal: 800}},
				Days:   []planning.DayEval{{Date: "2026-11-21", Kcal: 1600, BandLow: 1800, BandHigh: 2200, FloorKcal: 1500, CookOccasions: 1}},
				Week:   bev.Week,
			},
			check: func(t *testing.T, r changeReport) {
				require.Len(t, r.Changed, 1)
				m := r.Changed[0]
				assert.Equal(t, "Sat 21 Nov", m.Day)
				assert.Equal(t, "dinner", m.Meal)
				assert.Equal(t, []string{"Salmon 1 portion"}, m.Before)
				assert.Equal(t, []string{"Salmon 0.5 portions"}, m.After)
				assert.Equal(t, 800, *m.KcalFrom)
				assert.Equal(t, 400, *m.KcalTo)
				require.Len(t, r.Days, 1)
				assert.Equal(t, reportDay{Day: "Sat 21 Nov", KcalBefore: 2000, KcalAfter: 1600, Range: "1800-2200 kcal", Status: "under its range"}, r.Days[0])
			},
		},
		{
			name:  "cooking: the day of the changed meal, with what is cooked for which meals and how many times",
			after: reportState(1.5, 0.5, 1),
			aev:   planning.Evaluation{Days: []planning.DayEval{{Date: "2026-11-21", Kcal: 1600, CookOccasions: 1}}},
			check: func(t *testing.T, r changeReport) {
				require.Len(t, r.Cooking, 1)
				c := r.Cooking[0]
				assert.Equal(t, "Sat 21 Nov", c.Day)
				assert.Equal(t, 1, c.CookingTimes)
				assert.Equal(t, []reportDish{{At: "dinner", Dish: "Salmon", Eaten: []string{"Sat 21 Nov dinner", "Sun 22 Nov dinner"}}}, c.Cooked)
			},
		},
		{
			name:  "leftovers: a half portion is left and the batch was not resized",
			after: reportState(2, 0.5, 1),
			check: func(t *testing.T, r changeReport) {
				require.Len(t, r.Leftovers, 1)
				assert.Contains(t, r.Leftovers[0], `0.5 portions of "Salmon" cooked Sat 21 Nov`)
				assert.Contains(t, r.Leftovers[0], "The batch was not resized.")
			},
		},
		{
			name:  "leftovers: a batch resized to the meals leaves nothing over and says so",
			after: reportState(1.5, 0.5, 1),
			check: func(t *testing.T, r changeReport) {
				require.Len(t, r.Leftovers, 1)
				assert.Contains(t, r.Leftovers[0], "resized from 2 portions to 1.5 portions; nothing is left over")
			},
		},
		{
			name:  "leftovers: meals that need more than the batch holds",
			after: reportState(2, 1.5, 1),
			check: func(t *testing.T, r changeReport) {
				require.Len(t, r.Leftovers, 1)
				assert.Contains(t, r.Leftovers[0], `need 0.5 portions more of "Salmon"`)
			},
		},
		{
			name: "leftovers: a batch nobody eats any more",
			after: func() planning.State {
				s := reportState(2, 1, 1)
				s.Plates, s.Sessions = nil, nil
				return s
			}(),
			check: func(t *testing.T, r changeReport) {
				require.Len(t, r.Leftovers, 1)
				assert.Contains(t, r.Leftovers[0], "is no longer needed")
			},
		},
		{
			name:  "week: meat and a dish that is now the main of two meals, with numbers",
			after: reportState(2, 1, 1),
			aev:   planning.Evaluation{Week: &planning.WeekEval{Days: 7, MeatG: 546, Pct: 100}},
			check: func(t *testing.T, r changeReport) {
				assert.Contains(t, r.Week, "Meat this week: 546 g, was 321 g (limit 300 g), over the limit.")
			},
		},
		{
			name:  "new warnings are sentences with the meal and the numbers; earlier ones are only counted",
			after: reportState(2, 1, 1),
			aev: planning.Evaluation{Issues: []planning.Issue{
				meat,
				{Rule: "H9", Code: planning.CodeKcalBelowFloor, Severity: planning.SeverityViolation, Date: "2026-11-22", Params: map[string]any{"kcal": 1100.0, "floor_kcal": 1500.0}},
				{Rule: "S2", Code: planning.CodeKcalBelowBand, Severity: planning.SeveritySoft, Date: "2026-11-21", Params: map[string]any{"kcal": 1600.0, "band_low_kcal": 1800.0, "band_high_kcal": 2200.0}},
			}},
			check: func(t *testing.T, r changeReport) {
				assert.Equal(t, []string{
					"Sat 21 Nov holds 1600 kcal, under its range of 1800-2200 kcal.",
					"Sun 22 Nov holds only 1100 kcal, under the day's minimum of 1500 kcal.",
				}, r.NewWarnings)
				assert.Equal(t, 1, r.EarlierWarnings)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := bev
			if tt.name[:3] == "new" {
				b.Issues = []planning.Issue{meat} // open before and unchanged: counted, not listed
			}
			r := buildReport(reportInput{beforeSt: before, afterSt: tt.after, before: b, after: tt.aev})
			tt.check(t, r)
			raw, err := json.Marshal(r)
			require.NoError(t, err)
			assert.NotRegexp(t, reportRuleToken, string(raw), "no rule code")
			assert.NotRegexp(t, reportISODate, string(raw), "no ISO date")
			assert.NotContains(t, string(raw), "_id\"", "no ids")
		})
	}
}

func TestBuildReport_EarlierWarningsAreOnlyCounted(t *testing.T) {
	open := planning.Issue{Rule: "S2", Code: planning.CodeKcalBelowBand, Severity: planning.SeveritySoft, Date: "2026-11-23", Params: map[string]any{"kcal": 1700.0, "band_low_kcal": 1800.0, "band_high_kcal": 2200.0}}
	st := reportState(2, 1, 1)
	ev := planning.Evaluation{Issues: []planning.Issue{open}}
	r := buildReport(reportInput{beforeSt: st, afterSt: st, before: ev, after: ev})
	assert.Empty(t, r.NewWarnings)
	assert.Equal(t, 1, r.EarlierWarnings)
}

func TestBuildReport_AlsoChangedSaysMovedCooksAndAutomaticFixes(t *testing.T) {
	st := reportState(2, 1, 1)
	applied := []map[string]any{
		{"op": "away", "also_changed": []any{map[string]any{"what": "cook_moved", "session_id": json.Number("1"), "date": "2026-11-22", "slot_id": json.Number("2"), "from_date": "2026-11-21", "from_slot_id": json.Number("2")}}},
		{"op": "move_cook", "auto_fix": "x", "session_id": 1, "cook_date": "2026-11-22", "cook_slot_id": 2, "from_date": "2026-11-21", "from_slot_id": 2},
	}
	r := buildReport(reportInput{beforeSt: st, afterSt: st, applied: applied})
	require.Len(t, r.AlsoChanged, 2)
	assert.Equal(t, `The "Salmon" batch is now cooked at Sun 22 Nov dinner (it was Sat 21 Nov dinner).`, r.AlsoChanged[0])
	assert.Equal(t, `Automatically moved the "Salmon" batch to be cooked at Sun 22 Nov dinner (it was Sat 21 Nov dinner), so that no day gets a second cooking time.`, r.AlsoChanged[1])
}
