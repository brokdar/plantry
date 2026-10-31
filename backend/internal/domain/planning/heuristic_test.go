package planning_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// batchCooker is the "Batch cooker" fixture (plan §0.1) as a pure state:
// Mon–Fri breakfast routine Oats (already applied), batch cooking, keep 3
// days, 1 occasion per day, variety 3 weeks; mains 101–110 (ref 4; 101–103
// eaten in the last 2 weeks) and sides 201–204.
func batchCooker() planning.State {
	s := baseState()
	s.Settings.BatchCooking = true
	// The routine Oats were eaten yesterday, like every weekday.
	o := s.Foods[oats]
	yesterday := day(fri).AddDate(0, 0, -7)
	o.LastEaten = &yesterday
	s.Foods = map[int64]planning.FoodFacts{oats: o}
	for i := range 10 {
		id := int64(101 + i)
		f := recipe(id, fmt.Sprintf("Main %d", i+1), food.RoleMain, "omnivore", 600)
		if i < 3 {
			eaten := day(sat).AddDate(0, 0, -3-i)
			f.LastEaten = &eaten
		}
		s.Foods[id] = f
	}
	for i := range 4 {
		id := int64(201 + i)
		s.Foods[id] = recipe(id, fmt.Sprintf("Side %d", i+1), food.RoleSideVeg, "vegan", 150)
	}
	for i, d := range s.Window.Days() {
		if wd := d.Weekday(); wd >= 1 && wd <= 5 {
			c := cell(d.Format("2006-01-02"), breakfast)
			setMode(&s, c, availability.ModeRoutine)
			s.Plates = append(s.Plates, planning.Plate{ID: int64(-1000 - i), Date: c.Date, SlotID: breakfast, Eaters: 1, Components: []planning.Component{{FoodID: oats, Portions: f64(1)}}})
		}
	}
	return s
}

func choose(t *testing.T, in planning.ChooseInput) planning.Proposal {
	t.Helper()
	p, err := planning.Heuristic{}.Choose(context.Background(), in)
	require.NoError(t, err)
	return p
}

func violations(issues []planning.Issue) []planning.Issue {
	var out []planning.Issue
	for _, is := range issues {
		if is.Severity == planning.SeverityViolation {
			out = append(out, is)
		}
	}
	return out
}

// laterCells counts the plates eating from session key on cells after its
// cook cell.
func laterCells(p planning.Proposal, key string) int {
	var ps planning.ProposedSession
	for _, s := range p.Sessions {
		if s.Key == key {
			ps = s
		}
	}
	n := 0
	for _, pp := range p.Plates {
		for _, c := range pp.Components {
			if c.SessionKey == key && (!pp.Date.Equal(ps.CookDate) || pp.SlotID != ps.CookSlotID) {
				n++
			}
		}
	}
	return n
}

// B6-AC1 (pure part): the batch cooker's week is violation-free with one
// occasion per day, a batch feeding ≥ 3 later cells, none of the recent
// mains and the none cells left empty.
func TestHeuristic_BatchCooker(t *testing.T) {
	s := batchCooker()
	thuDinner, friLunch := cell("2026-11-12", dinner), cell(fri, lunch)
	setMode(&s, thuDinner, availability.ModeNone)
	setMode(&s, friLunch, availability.ModeNone)
	tgt := windowTarget(s)

	p := choose(t, planning.ChooseInput{State: s, Target: tgt})

	m := planning.Merge(s, tgt, p)
	issues := planning.Check(m, tgt)
	assert.Empty(t, violations(issues))
	sc := planning.ComputeScore(m, tgt, issues)
	assert.InDelta(t, 1.0, sc.Coverage, 1e-9)
	for d, n := range sc.CookOccasions.PerDay {
		assert.LessOrEqual(t, n, 1, d)
	}
	maxLater := 0
	for _, ps := range p.Sessions {
		maxLater = max(maxLater, laterCells(p, ps.Key))
		assert.NotContains(t, []int64{101, 102, 103}, ps.FoodID, "recently eaten mains are not cooked")
		assert.NotEmpty(t, ps.Reason)
	}
	assert.GreaterOrEqual(t, maxLater, 3)
	for _, pp := range p.Plates {
		assert.False(t, pp.Date.Equal(thuDinner.Date) && pp.SlotID == dinner, "Thu dinner is none")
		assert.False(t, pp.Date.Equal(friLunch.Date) && pp.SlotID == lunch, "Fri lunch is none")
		assert.Equal(t, 1.0, pp.Eaters)
	}
	again := choose(t, planning.ChooseInput{State: s, Target: tgt})
	assert.Equal(t, p, again, "deterministic")
}

// Spec §6.7 step 1: after the first day, sessions cook at dinner; the
// cells before it eat from the previous batch.
func TestHeuristic_CooksAtDinnerAfterTheFirstDay(t *testing.T) {
	s := batchCooker()
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	for _, ps := range p.Sessions {
		if ps.CookDate.Equal(day(sat)) {
			continue
		}
		assert.Equal(t, dinner, ps.CookSlotID, "%s cooks at dinner", ps.CookDate.Format("2006-01-02"))
	}
	m := planning.Merge(s, windowTarget(s), p)
	assert.Empty(t, violations(planning.Check(m, windowTarget(s))))
}

func TestHeuristic_Deterministic(t *testing.T) {
	in := func() planning.ChooseInput {
		s := batchCooker()
		return planning.ChooseInput{State: s, Target: windowTarget(s), EatersByDate: map[string]float64{sun: 2}, IncludeNewRecipe: true}
	}
	first := choose(t, in())
	for range 5 {
		assert.Equal(t, first, choose(t, in()), "same input, same proposal")
	}
}

func TestHeuristic_MainsGetASideCookedAtTheSameOccasion(t *testing.T) {
	s := batchCooker()
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	require.GreaterOrEqual(t, len(p.Sessions), 2)
	main, side := p.Sessions[0], p.Sessions[1]
	assert.Equal(t, food.RoleSideVeg, s.Foods[side.FoodID].Role)
	assert.Equal(t, main.CookDate, side.CookDate)
	assert.Equal(t, main.CookSlotID, side.CookSlotID)
	assert.InDelta(t, main.Portions, side.Portions, 1e-9)
}

func TestHeuristic_NoBatchCookingKeepsSessionsToTheirCell(t *testing.T) {
	s := batchCooker()
	s.Settings.BatchCooking = false
	s.Settings.MaxCookOccasionsPerDay = 3
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	for _, ps := range p.Sessions {
		assert.Zero(t, laterCells(p, ps.Key), ps.Key)
		assert.InDelta(t, 1.0, ps.Portions, 1e-9)
	}
	m := planning.Merge(s, windowTarget(s), p)
	assert.Empty(t, violations(planning.Check(m, windowTarget(s))))
}

func TestHeuristic_FixedSessionsFeedCellsFirst(t *testing.T) {
	s := batchCooker()
	// A confirmed Chili batch cooked the Friday before with 3 portions left.
	s.Foods[chili] = recipe(chili, "Chili", food.RoleMain, "omnivore", 600)
	s.Sessions = []planning.Session{persistedSession(7, chili, "2026-11-06", dinner, 4, true)}
	s.Plates = append(s.Plates, persisted(70, "2026-11-06", dinner, true, chili, 7))

	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})

	fromFixed := 0
	for _, pp := range p.Plates {
		if pp.Components[0].SessionID == 7 {
			fromFixed++
			assert.NotEmpty(t, pp.Reason)
		}
	}
	assert.Equal(t, 3, fromFixed, "the fixed batch's 3 spare portions fill the first cells")
	assert.Equal(t, sat, p.Plates[0].Date.Format("2006-01-02"))
	assert.Equal(t, int64(7), p.Plates[0].Components[0].SessionID)
	m := planning.Merge(s, windowTarget(s), p)
	assert.Empty(t, violations(planning.Check(m, windowTarget(s))))
}

func TestHeuristic_FreezesPastKeepDaysOnlyWhenAllowed(t *testing.T) {
	setup := func(freezing, freezable bool) planning.Proposal {
		s := batchCooker()
		s.Settings.DefaultKeepDays = 0
		s.Settings.FreezingEnabled = freezing
		s.Settings.MaxCookOccasionsPerDay = 3
		for id, f := range s.Foods {
			f.ReferencePortions = 6
			f.Freezable = freezable
			s.Foods[id] = f
		}
		p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
		m := planning.Merge(s, windowTarget(s), p)
		assert.Empty(t, violations(planning.Check(m, windowTarget(s))))
		return p
	}
	frozen := func(p planning.Proposal) float64 {
		n := 0.0
		for _, ps := range p.Sessions {
			n += ps.FrozenPortions
		}
		return n
	}
	assert.Positive(t, frozen(setup(true, true)), "freezing on, freezable: later cells eat frozen portions")
	assert.Zero(t, frozen(setup(true, false)), "not freezable")
	assert.Zero(t, frozen(setup(false, true)), "freezing off")
}

func TestHeuristic_Diet(t *testing.T) {
	s := batchCooker()
	for _, id := range []int64{104, 105, 106} {
		f := s.Foods[id]
		f.DietClass = "vegetarian"
		s.Foods[id] = f
	}
	s.Profile.DietType = profile.DietVegetarian
	s.Settings.VarietyWeeks = 0
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	for _, ps := range p.Sessions {
		assert.NotEqual(t, "omnivore", s.Foods[ps.FoodID].DietClass)
	}

	s = batchCooker()
	for _, id := range []int64{104, 105, 106, 107} {
		f := s.Foods[id]
		f.DietClass = "vegan"
		s.Foods[id] = f
	}
	s.Profile.MeatDaysPerWeekMax = intp(2)
	p = choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	m := planning.Merge(s, windowTarget(s), p)
	assert.Empty(t, codes(planning.Check(m, windowTarget(s)), "H5"), "at most 2 meat days")
}

// B6-AC6 (pure part).
func TestHeuristic_IncludeNewRecipePlacesExactlyOne(t *testing.T) {
	s := batchCooker()
	for i, id := range []int64{104, 105, 106, 107, 108} {
		f := s.Foods[id]
		eaten := day(sat).AddDate(0, 0, -30-i)
		f.LastEaten = &eaten
		s.Foods[id] = f
	}
	isNew := func(p planning.Proposal) int {
		n := 0
		for _, ps := range p.Sessions {
			if f := s.Foods[ps.FoodID]; f.Role == food.RoleMain && f.LastEaten == nil {
				n++
			}
		}
		return n
	}
	assert.Equal(t, 1, isNew(choose(t, planning.ChooseInput{State: s, Target: windowTarget(s), IncludeNewRecipe: true})))
	assert.Zero(t, isNew(choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})))
}

// B6-AC6 (pure part).
func TestHeuristic_EatersByDate(t *testing.T) {
	s := batchCooker()
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s), EatersByDate: map[string]float64{sun: 4}})
	byKey := map[string]float64{}
	for _, pp := range p.Plates {
		want := 1.0
		if pp.Date.Format("2006-01-02") == sun {
			want = 4
		}
		assert.InDelta(t, want, pp.Eaters, 1e-9, pp.Date)
		for _, c := range pp.Components {
			byKey[c.SessionKey] += pp.Eaters * c.Portions
		}
	}
	for _, ps := range p.Sessions {
		assert.InDelta(t, byKey[ps.Key], ps.Portions, 1e-9, "sessions are sized to their plates")
	}
	m := planning.Merge(s, windowTarget(s), p)
	assert.Empty(t, violations(planning.Check(m, windowTarget(s))))
}

func TestHeuristic_AvoidsTheReplacedRecipes(t *testing.T) {
	s := batchCooker()
	first := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	again := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s), Avoid: []int64{first.Sessions[0].FoodID}})
	assert.NotEqual(t, first.Sessions[0].FoodID, again.Sessions[0].FoodID)
}

func TestHeuristic_SideSwap(t *testing.T) {
	s := batchCooker()
	s.Plates = append(s.Plates, persisted(70, sat, dinner, false, 104, 0))
	in := planning.ChooseInput{State: s, Target: targetCells(cell(sat, dinner)), Locale: "de", Side: &planning.SideSwap{
		FoodID: 201, CookDate: day(sat), CookSlotID: dinner, Portions: 2,
		Plates: []planning.SidePlate{{PlateID: 70, Date: day(sat), SlotID: dinner, Eaters: 1, PerEater: 1}},
	}}
	p := choose(t, in)
	require.Len(t, p.Sessions, 1)
	assert.NotEqual(t, int64(201), p.Sessions[0].FoodID)
	assert.True(t, s.Foods[p.Sessions[0].FoodID].Role == food.RoleSideVeg)
	assert.InDelta(t, 2.0, p.Sessions[0].Portions, 1e-9)
	require.Len(t, p.Plates, 1)
	assert.Equal(t, int64(70), p.Plates[0].PlateID)
	assert.Equal(t, "Eine andere Beilage", p.Plates[0].Reason)

	only := batchCooker()
	only.Foods = map[int64]planning.FoodFacts{201: only.Foods[201]}
	in.State = only
	_, err := planning.Heuristic{}.Choose(context.Background(), in)
	assert.ErrorIs(t, err, domain.ErrNotEnoughRecipes)
}

func TestHeuristic_NoRecipe(t *testing.T) {
	s := baseState()
	s.Foods = map[int64]planning.FoodFacts{}
	_, err := planning.Heuristic{}.Choose(context.Background(), planning.ChooseInput{State: s, Target: windowTarget(s)})
	assert.ErrorIs(t, err, domain.ErrNotEnoughRecipes)
}

var _ = plate.CellKey{}
