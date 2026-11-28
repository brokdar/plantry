package planning_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

func floorCodes(issues []planning.Issue) []string {
	var out []string
	for _, is := range issues {
		if is.Rule == "H9" {
			out = append(out, is.Date)
		}
	}
	return out
}

// A day under the floor is lifted to it: only mains and carb sides grow,
// never by more than 1.5x, and batch sessions stay consistent (no hard
// violation, every session covers its plates).
func TestHeuristic_FloorTopUpLiftsADay(t *testing.T) {
	s := batchCooker()
	s.Profile.KcalTarget = f64(3000) // floor 2250; a weekday holds ~1850

	tgt := windowTarget(s)
	p := choose(t, planning.ChooseInput{State: s, Target: tgt})
	m := planning.Merge(s, tgt, p)

	assert.Empty(t, floorCodes(planning.Check(m, tgt)), "every day reaches the floor")
	assert.Empty(t, violations(planning.Check(m, tgt)), "sessions still cover their plates")

	eaten := map[string]float64{}
	for _, pl := range p.Plates {
		for _, c := range pl.Components {
			assert.LessOrEqual(t, c.Portions, 1.5+1e-9, "capped at 1.5x")
			f := s.Foods[c.FoodID]
			if f.Role == food.RoleSideVeg {
				assert.InDelta(t, 1, c.Portions, 1e-9, "sides do not grow")
			}
			if c.SessionKey != "" {
				eaten[c.SessionKey] += c.Portions
			}
		}
	}
	for _, ss := range p.Sessions {
		assert.GreaterOrEqual(t, ss.Portions+1e-9, eaten[ss.Key], "session %s covers its plates (shopping buys the session)", ss.Key)
	}
}

// A floor the plan cannot reach within the cap ends as an explicit H9 and a
// note to the user, not an endless loop.
func TestHeuristic_UnreachableFloorStaysAnH9WithANote(t *testing.T) {
	s := batchCooker()
	s.Profile.KcalTarget = f64(5000) // floor 3750, far above 1.5x of a weekday

	tgt := windowTarget(s)
	p := choose(t, planning.ChooseInput{State: s, Target: tgt})
	m := planning.Merge(s, tgt, p)

	assert.NotEmpty(t, floorCodes(planning.Check(m, tgt)))
	assert.Contains(t, p.Notes, "daily minimum")
	assert.Contains(t, p.Notes, "Below target", "the band is out of reach too: said, not silent")
}

// A normal target is not only lifted over the floor: no day stays under its
// band and the week lands in its band, within the 1.5x cap.
func TestHeuristic_KcalBandDaysAndWeek(t *testing.T) {
	for _, kcal := range []float64{2000, 2400, 2800} {
		s := proteinState(profile.DietOmnivore) // batchCooker plus no-cook foods
		s.Profile.WeightKg = nil
		s.Profile.KcalTarget = f64(kcal)
		tgt := windowTarget(s)
		p := choose(t, planning.ChooseInput{State: s, Target: tgt})
		m := planning.Merge(s, tgt, p)
		ev := planning.Evaluate(m, tgt)

		assert.Empty(t, violations(planning.Check(m, tgt)), "%.0f kcal", kcal)
		for _, d := range ev.Days {
			// Days the recipes already overfill stay as they are.
			assert.GreaterOrEqual(t, d.Kcal, kcal*0.9-1e-6, "%.0f kcal: %s not under the day band", kcal, d.Date)
		}
		if assert.NotNil(t, ev.Week) {
			assert.InDelta(t, 100, ev.Week.Pct, 5, "%.0f kcal: week inside its band", kcal)
		}
		for _, pl := range p.Plates {
			for _, c := range pl.Components {
				assert.LessOrEqual(t, c.Portions, 1.5+1e-9, "capped at 1.5x over every pass")
			}
		}
	}
}

// A routine cell with no plate has no kcal to count: it does not scale the
// floor, so the day is not a false H9.
func TestCheck_FloorIgnoresRoutineCellWithoutPlate(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(1600) // 75 % = 1200; two of three meals: 800
	setMode(&s, cell(mon, breakfast), availability.ModeRoutine)

	_, issues := weekOf(s)

	assert.NotContains(t, codeDates(issues, "H9", planning.CodeKcalBelowFloor), mon)
}

func TestBoardIssues_FloorOnlyForDaysThatCanChange(t *testing.T) {
	s := baseState()
	s.Today = day(mon)
	s.Profile.KcalTarget = f64(2400) // floor 1800, a day of three curries holds 1500
	id := int64(1)
	for _, d := range []struct {
		date      string
		confirmed bool
	}{{sat, true}, {tue, false}, {mon, true}} {
		for _, slot := range []int64{breakfast, lunch, dinner} {
			s.Plates = append(s.Plates, persisted(id, d.date, slot, d.confirmed, curry, 0))
			id++
		}
	}

	got := map[string]planning.Severity{}
	for _, is := range planning.BoardIssues(s) {
		if is.Rule == "H9" {
			got[is.Date] = is.Severity
		}
	}

	assert.NotContains(t, got, sat, "past days are history")
	assert.Equal(t, planning.SeverityViolation, got[tue], "a draft day can still be fixed")
	assert.Equal(t, planning.SeverityWarning, got[mon], "a confirmed day is at most a warning")
}

// R6 (planner-10, -22): with plate fit on the lift rounds every grown
// component up to a quarter portion; a day just under its band must not end
// above the band top because both its dishes took a quarter more. A quarter
// less on one of them lands inside (lose: band 90-105 %).
func TestHeuristic_RoundedUpLiftDoesNotCarryADayPastTheBandTop(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(2000)
	s.Profile.Goal = profile.GoalLose
	s.Settings.PlateFit = true
	a := recipe(chili, "Main", food.RoleMain, "omnivore", 200)
	a.ReferencePortions = 1
	b := recipe(rice, "Starch", food.RoleSideStarch, "vegan", 280)
	b.ReferencePortions = 1
	s.Foods = map[int64]planning.FoodFacts{chili: a, rice: b}
	setMode(&s, cell(mon, breakfast), "none")
	setMode(&s, cell(mon, lunch), "none")
	tgt := targetCells(cell(mon, dinner))

	p := choose(t, planning.ChooseInput{State: s, Target: tgt})

	ev := planning.Evaluate(planning.Merge(s, tgt, p), tgt)
	i := slices.IndexFunc(ev.Days, func(d planning.DayEval) bool { return d.Date == mon })
	if assert.GreaterOrEqual(t, i, 0) && assert.NotNil(t, ev.Days[i].TargetKcal) {
		d := ev.Days[i]
		assert.LessOrEqual(t, d.Kcal, *d.TargetKcal*1.05+1, "the day stays at or under the band top")
		assert.GreaterOrEqual(t, d.Kcal, *d.TargetKcal*0.9-1, "and does not fall under its bottom")
	}
}

// dinnerOnly is a one-dinner Monday of a main and a starch, both lifted by
// the lifts of topUpFloors (target 2400 kcal: the dinner's band is 720-880,
// its floor 600).
func dinnerOnly(mainKcal, starchKcal float64) (planning.State, planning.Target) {
	s := baseState()
	s.Profile.KcalTarget = f64(2400)
	s.Settings.PlateFit = true
	a := recipe(chili, "Main", food.RoleMain, "omnivore", mainKcal)
	a.ReferencePortions = 1
	b := recipe(rice, "Starch", food.RoleSideStarch, "vegan", starchKcal)
	b.ReferencePortions = 1
	s.Foods = map[int64]planning.FoodFacts{chili: a, rice: b}
	setMode(&s, cell(mon, breakfast), "none")
	setMode(&s, cell(mon, lunch), "none")
	return s, targetCells(cell(mon, dinner))
}

func monday(t *testing.T, s planning.State, tgt planning.Target, p planning.Proposal) planning.DayEval {
	t.Helper()
	ev := planning.Evaluate(planning.Merge(s, tgt, p), tgt)
	i := slices.IndexFunc(ev.Days, func(d planning.DayEval) bool { return d.Date == mon })
	require.GreaterOrEqual(t, i, 0)
	return ev.Days[i]
}

// R6 review: a light day's band bottom (0.9 of 85 %) can sit under the H9
// floor, which follows the normal target; the trim must not take the day
// under it (here the day is 450 kcal over a 437 band top, one quarter step
// lands on 375 < floor 400).
func TestHeuristic_TrimKeepsTheH9FloorOnALightDay(t *testing.T) {
	s := baseState()
	s.Profile.KcalTarget = f64(1400)
	s.Settings.PlateFit = true
	a := recipe(chili, "Main", food.RoleMain, "omnivore", 300)
	a.ReferencePortions = 1
	b := recipe(rice, "Starch", food.RoleSideStarch, "vegan", 60)
	b.ReferencePortions = 1
	s.Foods = map[int64]planning.FoodFacts{chili: a, rice: b}
	setMode(&s, cell(mon, breakfast), "none")
	setMode(&s, cell(mon, lunch), "none")
	s.Light = []planning.LightIntent{{Date: mon, Scope: planning.LightScopeDay}}
	tgt := targetCells(cell(mon, dinner))

	d := monday(t, s, tgt, choose(t, planning.ChooseInput{State: s, Target: tgt}))

	assert.GreaterOrEqual(t, d.Kcal, d.FloorKcal-1e-6, "the trim never takes a day under the H9 floor")
}

// R6 review: a quarter step that would take the day under its protein
// target (it meets it before the trim) stays; the trim may only use a step
// that keeps the protein.
func TestHeuristic_TrimKeepsTheDayProteinTarget(t *testing.T) {
	s, tgt := dinnerOnly(500, 100)
	s.Profile.WeightKg = f64(126) // 1 g/kg over a third of the day: 42 g

	d := monday(t, s, tgt, choose(t, planning.ChooseInput{State: s, Target: tgt}))

	require.NotZero(t, d.ProteinTargetG)
	assert.GreaterOrEqual(t, d.ProteinG, d.ProteinTargetG-1e-6, "the trim keeps the protein target")
	assert.LessOrEqual(t, d.Kcal, *d.TargetKcal*1.1+1, "and the day still ends inside the band")
}

// R6 review: a trimmed component takes its share out of the batch it eats
// from, so a session's portions stay the sum over its plates.
func TestHeuristic_TrimKeepsBatchPortionsEqualToWhatIsEaten(t *testing.T) {
	s, _ := dinnerOnly(500, 100)
	s.Settings.BatchCooking = true
	setMode(&s, cell(tue, breakfast), "none")
	setMode(&s, cell(tue, lunch), "none")
	tgt := targetCells(cell(mon, dinner), cell(tue, dinner))

	p := choose(t, planning.ChooseInput{State: s, Target: tgt})

	eaten := map[string]float64{}
	for _, pl := range p.Plates {
		e := pl.Eaters
		if e == 0 {
			e = s.Profile.HouseholdPortions
		}
		for _, c := range pl.Components {
			if c.SessionKey != "" {
				eaten[c.SessionKey] += e * c.Portions
			}
		}
	}
	require.NotEmpty(t, p.Sessions)
	for _, ss := range p.Sessions {
		assert.InDelta(t, eaten[ss.Key], ss.Portions, 1e-6, "session %s", ss.Key)
	}
}
