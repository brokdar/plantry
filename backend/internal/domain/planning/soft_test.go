package planning_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

const (
	riceLeaf int64 = 20
	apple    int64 = 21
)

// softState is Saturday only (every other cell is No plate): three plates
// of 600 kcal / 30 g protein, the dinner with 80 g of leaf rice on top.
func softState(kcalTarget float64) planning.State {
	s := baseState()
	for i := range s.Cells {
		if s.Cells[i].Date.Format("2006-01-02") != sat {
			s.Cells[i].Mode = availability.ModeNone
		}
	}
	s.Foods[riceLeaf] = planning.FoodFacts{
		ID: riceLeaf, Name: "Rice dry", Kind: food.KindLeaf, DietClass: "vegan",
		Macros: nutrition.Macros{Kcal: 130, Protein: 2.7}, NutritionComplete: true,
	}
	s.Foods[apple] = planning.FoodFacts{
		ID: apple, Name: "Apple", Kind: food.KindLeaf, DietClass: "vegan", NoCook: true,
		Macros: nutrition.Macros{Kcal: 52, Protein: 0.3}, NutritionComplete: true,
	}
	s.NoCook = []int64{apple}
	s.Profile.KcalTarget = f64(kcalTarget)
	s.Settings.MaxCookOccasionsPerDay = 3
	for i, slot := range []int64{breakfast, lunch, dinner} {
		id := int64(i + 1)
		s.Sessions = append(s.Sessions, persistedSession(id, chili, sat, slot, 1, false))
		p := persisted(id, sat, slot, false, chili, id)
		if slot == dinner {
			p.Components = append(p.Components, planning.Component{FoodID: riceLeaf, Grams: f64(80)})
		}
		s.Plates = append(s.Plates, p)
	}
	return s
}

func softOf(ev planning.Evaluation, rule string) []planning.Issue {
	var out []planning.Issue
	for _, is := range ev.Issues {
		if is.Rule == rule {
			out = append(out, is)
		}
	}
	return out
}

func TestEvaluate_S2BelowBandOffersRaiseAndNoCookSide(t *testing.T) {
	s := softState(2200)
	s.Profile.MeatDaysPerWeekMax = intp(7)
	ev := planning.Evaluate(s, windowTarget(s))

	s2 := softOf(ev, "S2")
	require.Len(t, s2, 1)
	assert.Equal(t, planning.SeveritySoft, s2[0].Severity)
	assert.Equal(t, planning.CodeKcalBelowBand, s2[0].Code)
	assert.Equal(t, sat, s2[0].Date)
	require.NotEmpty(t, s2[0].Options)
	require.LessOrEqual(t, len(s2[0].Options), 3)
	assert.Positive(t, ev.Totals.Soft)

	var raise, add bool
	for _, o := range s2[0].Options {
		assert.Positive(t, o.KcalDelta)
		assert.LessOrEqual(t, o.CookOccasions, 3, "H2 never worsened")
		if o.Action == "raise" && o.FoodID == riceLeaf {
			raise = true
			require.NotNil(t, o.Grams)
			require.NotNil(t, o.From)
			assert.InDelta(t, 80, *o.From, 0.01)
			assert.InDelta(t, 100, *o.Grams, 0.01, "rice 80 -> 100 g")
		}
		if o.Action == "add" && o.FoodID == apple {
			add = true
			assert.False(t, o.NewMeatDay)
		}
	}
	assert.True(t, raise, "options: %+v", s2[0].Options)
	assert.True(t, add, "options: %+v", s2[0].Options)
}

func TestEvaluate_LightDayInBandIsUntouched(t *testing.T) {
	s := softState(1850) // 1800 / 1850 = 97 %
	ev := planning.Evaluate(s, windowTarget(s))
	assert.Empty(t, softOf(ev, "S2"))
	assert.Empty(t, softOf(ev, "S3"))
	assert.Zero(t, ev.Totals.Soft)
}

func TestEvaluate_S3ProteinBelowTarget(t *testing.T) {
	s := softState(1850)
	s.Profile.ProteinPct = f64(30) // the day holds 20 %
	ev := planning.Evaluate(s, windowTarget(s))
	s3 := softOf(ev, "S3")
	require.Len(t, s3, 1)
	assert.Equal(t, planning.CodeProteinLow, s3[0].Code)
	assert.InDelta(t, 25, s3[0].Params["min_pct"], 0.01)

	s.Profile.ProteinPct = f64(24) // within 5 points
	assert.Empty(t, softOf(planning.Evaluate(s, windowTarget(s)), "S3"))
}

func TestEvaluate_S2AboveBandHasNoOptions(t *testing.T) {
	s := softState(1500) // 1800 / 1500 = 120 %
	ev := planning.Evaluate(s, windowTarget(s))
	s2 := softOf(ev, "S2")
	require.Len(t, s2, 1)
	assert.Equal(t, planning.CodeKcalAboveBand, s2[0].Code)
	assert.Empty(t, s2[0].Options)
}

func satDay(ev planning.Evaluation) planning.DayEval {
	for _, d := range ev.Days {
		if d.Date == sat {
			return d
		}
	}
	return planning.DayEval{}
}

// An S3 option must raise the day's protein share: raising low-protein rice
// (80 -> 100 g) would lower it and is never offered.
func TestEvaluate_S3OptionsRaiseTheProteinShare(t *testing.T) {
	s := softState(1850)
	s.Profile.ProteinPct = f64(30)
	s.Profile.MeatDaysPerWeekMax = intp(7)
	before := planning.Evaluate(s, windowTarget(s))
	s3 := softOf(before, "S3")
	require.Len(t, s3, 1)
	dayPct := satDay(before).ProteinPct
	for _, o := range s3[0].Options {
		assert.Greater(t, o.DayProteinPct, dayPct-0.05, "option %+v", o)
		assert.False(t, o.Action == "raise" && o.FoodID == riceLeaf, "rice lowers the share: %+v", o)
	}
}

// An S2 option moves the day toward its target without leaving the band,
// and options that bring the day into the band come first. Each option's op
// is a valid apply_changes op.
func TestEvaluate_S2OptionsCloseTheGapAndCarryOps(t *testing.T) {
	s := softState(2200)
	s.Profile.MeatDaysPerWeekMax = intp(7)
	ev := planning.Evaluate(s, windowTarget(s))
	s2 := softOf(ev, "S2")
	require.Len(t, s2, 1)
	opts := s2[0].Options
	require.NotEmpty(t, opts)
	dayKcal := satDay(ev).Kcal
	seenOut := false
	for _, o := range opts {
		assert.Less(t, math.Abs(2200-o.DayKcal), math.Abs(2200-dayKcal), "closer to the target: %+v", o)
		assert.LessOrEqual(t, o.DayKcal, 2200*planning.KcalBandHigh)
		inBand := o.DayKcal >= 2200*planning.KcalBandLow
		assert.False(t, seenOut && inBand, "in-band options come first")
		seenOut = seenOut || !inBand
		switch o.Op.Op {
		case "set_quantity":
			require.NotNil(t, o.Op.PlateComponentID)
			assert.Equal(t, "g", o.Op.Unit)
			require.NotNil(t, o.Op.Amount)
		case "add":
			assert.NotZero(t, o.Op.FoodID)
			assert.True(t, o.Op.PlateID != nil || (o.Op.Date != "" && o.Op.SlotID != 0))
			assert.True(t, o.Op.Portions != nil || (o.Op.Amount != nil && o.Op.Unit == "g"))
		default:
			t.Fatalf("op %q is not an apply_changes op", o.Op.Op)
		}
	}
}
