package agent

import (
	"encoding/json"
	"regexp"
	"strings"
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
	t.Parallel()
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
				tell := strings.Join(r.Tell, "\n")
				assert.Contains(t, tell, `0.5 portions of the "Salmon" cooked on Sat 21 Nov`)
				assert.Contains(t, tell, "It was not resized.")
			},
		},
		{
			name:  "leftovers: a batch resized to the meals leaves nothing over and says so",
			after: reportState(1.5, 0.5, 1),
			check: func(t *testing.T, r changeReport) {
				assert.Contains(t, strings.Join(r.Tell, "\n"), "cook 1.5 portions of the \"Salmon\" (was 2)")
			},
		},
		{
			name:  "leftovers: meals that need more than the batch holds",
			after: reportState(2, 1.5, 1),
			check: func(t *testing.T, r changeReport) {
				assert.Contains(t, strings.Join(r.Tell, "\n"), `need 0.5 portions more of the "Salmon"`)
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
				// the meals that went explain it: no meal any more, nothing cooked
				assert.Contains(t, strings.Join(r.Tell, "\n"), "no meal any more")
				assert.Contains(t, strings.Join(r.Tell, "\n"), "nothing is cooked")
			},
		},
		{
			name:  "week: meat and a dish that is now the main of two meals, with numbers",
			after: reportState(2, 1, 1),
			aev:   planning.Evaluation{Week: &planning.WeekEval{Days: 7, MeatG: 546, Pct: 100}},
			check: func(t *testing.T, r changeReport) {
				assert.Contains(t, strings.Join(r.Tell, "\n"), "meat is 546 g (was 321 g, limit 300 g)")
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
				tell := strings.Join(r.Tell, "\n")
				assert.Contains(t, tell, "New warning: Sat 21 Nov holds 1600 kcal, below a good amount for the day (1800–2200 kcal).")
				assert.Contains(t, tell, "New warning: Sun 22 Nov holds only 1100 kcal, under the day's minimum of 1500 kcal.")
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
	t.Parallel()
	open := planning.Issue{Rule: "S2", Code: planning.CodeKcalBelowBand, Severity: planning.SeveritySoft, Date: "2026-11-23", Params: map[string]any{"kcal": 1700.0, "band_low_kcal": 1800.0, "band_high_kcal": 2200.0}}
	st := reportState(2, 1, 1)
	ev := planning.Evaluation{Issues: []planning.Issue{open}}
	r := buildReport(reportInput{beforeSt: st, afterSt: st, before: ev, after: ev})
	assert.NotContains(t, strings.Join(r.Tell, "\n"), "New warning")
	assert.Equal(t, 1, r.EarlierWarnings)
}

// A cook that moved is told by one cooking-time fact (where the dish is cooked
// now, what it feeds, the longest recipe's minutes), not by the also_changed
// sentences of the ops.
func TestBuildReport_MovedCookIsOneCookingTimeFact(t *testing.T) {
	t.Parallel()
	before := reportState(2, 1, 1)
	after := reportState(2, 1, 1)
	after.Sessions[0].CookDate = rd(2)
	applied := []map[string]any{
		{"op": "away", "also_changed": []any{map[string]any{"what": "cook_moved", "session_id": json.Number("1"), "date": "2026-11-22", "slot_id": json.Number("2"), "from_date": "2026-11-21", "from_slot_id": json.Number("2")}}},
		{"op": "move_cook", "auto_fix": "x", "session_id": 1, "cook_date": "2026-11-22", "cook_slot_id": 2, "from_date": "2026-11-21", "from_slot_id": 2},
	}
	r := buildReport(reportInput{beforeSt: before, afterSt: after, applied: applied})
	var cooks int
	for _, f := range r.Facts {
		if f.Kind == kindCook && strings.Contains(f.EN, " you cook ") {
			cooks++
			assert.Contains(t, f.EN, `Sun 22 Nov at dinner you cook "Salmon"`)
			assert.Contains(t, f.EN, "cooked Sat 21 Nov dinner before")
		}
	}
	assert.Equal(t, 1, cooks, "%v", r.Tell)
	assert.NotContains(t, strings.Join(r.Tell, "\n"), "To keep one cooking time")
}

// E022c: a meal of no-cook foods is named a cold plate in the report, so the
// reply can say what lunch is.
func TestBuildReport_NamesAColdPlate(t *testing.T) {
	t.Parallel()
	before := reportState(2, 1, 1)
	after := before
	after.Foods = map[int64]planning.FoodFacts{
		1: {ID: 1, Name: "Salmon"},
		3: {ID: 3, Name: "Bread", Kind: "leaf", NoCook: true},
		4: {ID: 4, Name: "Hummus", Kind: "leaf", NoCook: true},
	}
	after.Plates = []planning.Plate{
		{ID: 10, Date: rd(1), SlotID: 1, Eaters: 1, Components: []planning.Component{{ID: 5, FoodID: 3, Grams: f64(80)}, {ID: 6, FoodID: 4, Grams: f64(60)}}},
		before.Plates[1],
	}
	r := buildReport(reportInput{beforeSt: before, afterSt: after})
	var kinds []string
	for _, m := range r.Changed {
		kinds = append(kinds, m.Kind)
	}
	assert.Contains(t, kinds, "cold plate, nothing to cook")
}

// E042: a regenerate says what it did to the cooking of other days, and why
// the new meals may hold no meat.
func TestOtherCookingDays(t *testing.T) {
	t.Parallel()
	before := reportState(2, 1, 1) // Fri dinner cooks Salmon for Fri and Sat dinner
	after := before
	after.Plates = before.Plates[:1] // Sat dinner's plate is gone: the batch feeds one meal
	got := otherCookingDays(before, after, map[string]bool{rd(2).Format(time.DateOnly): true})
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "cooking: was Salmon at dinner for")
	assert.Contains(t, got[0], "now Salmon at dinner for")
	assert.Empty(t, otherCookingDays(before, before, nil), "nothing changed, nothing said")
	assert.Empty(t, otherCookingDays(before, after, map[string]bool{rd(1).Format(time.DateOnly): true}), "the days the run wrote are not other days")
}

func TestWeekEffect_SaysWhyNoMeatFits(t *testing.T) {
	t.Parallel()
	w := weekEffect(&planning.WeekEffect{MeatG: 321, LimitG: 300}, &planning.WeekEffect{MeatG: 233, LimitG: 300})
	assert.Contains(t, w.MeatNote, "holds 233 g of meat per person against a limit of 300 g: 67 g is left")
	w = weekEffect(&planning.WeekEffect{MeatG: 100, LimitG: 300}, &planning.WeekEffect{MeatG: 120, LimitG: 300})
	assert.Empty(t, w.MeatNote)
}
