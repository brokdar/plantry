package planning_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// sameDishDay is a Tuesday regenerate where lunch cooks a batch of Main 4 and
// dinner eats it again (one cooking occasion a day).
func sameDishDay() (planning.State, planning.Target, planning.Proposal) {
	s := batchCooker()
	tgt := targetCells(cell(tue, lunch), cell(tue, dinner))
	p := planning.Proposal{
		Sessions: []planning.ProposedSession{session("s1", 104, tue, lunch, 2)},
		Plates:   []planning.ProposedPlate{pplate(tue, lunch, onKey(104, "s1")), pplate(tue, dinner, onKey(104, "s1"))},
	}
	return s, tgt, p
}

// coldFoods adds a bread, a protein and a raw vegetable to the state's no-cook leaves.
func coldFoods(s *planning.State) {
	add := func(id int64, name string, role food.Role, kcal, protein, serving float64) {
		s.Foods[id] = planning.FoodFacts{
			ID: id, Name: name, Kind: food.KindLeaf, LeafRole: role, DietClass: "vegan", NoCook: true, NutritionComplete: true,
			Macros: nutrition.Macros{Kcal: kcal, Protein: protein}, ServingG: serving,
		}
		s.NoCook = append(s.NoCook, id)
	}
	add(950, "Bread", food.RoleSideStarch, 216, 7, 50)
	add(951, "Tofu", "", 144, 17, 100)
	add(952, "Cucumber", food.RoleSideVeg, 15, 0.7, 100)
}

func mainAt(p planning.Proposal, date string, slot int64) planning.ProposedComponent {
	for _, pl := range p.Plates {
		if pl.Date.Equal(day(date)) && pl.SlotID == slot {
			return pl.Components[0]
		}
	}
	return planning.ProposedComponent{}
}

func TestGuardDayMains(t *testing.T) {
	t.Run("dinner eats leftovers of a batch the week already has", func(t *testing.T) {
		s, tgt, p := sameDishDay()
		s.Sessions = []planning.Session{persistedSession(7, 105, mon, dinner, 4, false)}
		got, notes := planning.GuardDayMains(s, tgt, p, nil)
		assert.EqualValues(t, 104, mainAt(got, tue, lunch).FoodID, "lunch keeps its dish")
		dinnerMain := mainAt(got, tue, dinner)
		assert.EqualValues(t, 105, dinnerMain.FoodID)
		assert.EqualValues(t, 7, dinnerMain.SessionID)
		require.Len(t, notes, 1)
		assert.Contains(t, notes[0], "Day variety")
		assert.Contains(t, notes[0], "leftovers")
		for _, v := range violations(check(s, tgt, got)) {
			assert.Equal(t, "plate_outside_target", v.Code, "only the untargeted routine plates are outside the target")
		}
		require.Len(t, got.Sessions, 1)
		assert.InDelta(t, 1, got.Sessions[0].Portions, 1e-9, "the new batch no longer feeds the dinner")
	})
	t.Run("with no other main the earlier meal becomes a cold plate and the dinner cooks on its own cell", func(t *testing.T) {
		s, tgt, p := sameDishDay()
		coldFoods(&s)
		got, notes := planning.GuardDayMains(s, tgt, p, nil)
		require.Len(t, notes, 1)
		assert.Contains(t, notes[0], "slot 1 is a cold plate")
		assert.Contains(t, notes[0], "is a cold plate")
		assert.Contains(t, notes[0], "dinner is cooked fresh: Main 4")
		var lunchPlate planning.ProposedPlate
		for _, pl := range got.Plates {
			if pl.SlotID == lunch {
				lunchPlate = pl
			}
		}
		require.GreaterOrEqual(t, len(lunchPlate.Components), 3)
		for _, c := range lunchPlate.Components {
			assert.Zero(t, c.Portions, "no recipe on a cold plate")
			assert.Positive(t, c.Grams)
			assert.Empty(t, c.SessionKey)
		}
		require.Len(t, got.Sessions, 1)
		assert.EqualValues(t, dinner, got.Sessions[0].CookSlotID, "the dish cooks at dinner now")
		assert.InDelta(t, 1, got.Sessions[0].Portions, 1e-9)
		saved := strings.Join(planning.SavedPlanNotes(planning.ChooseInput{State: s, Target: tgt}, p, got), "\n")
		assert.Contains(t, saved, "slot 1 is a cold plate, nothing to cook: bread 50 g")
		merged := planning.Merge(s, tgt, got)
		ev := planning.Evaluate(merged, tgt)
		var kinds []string
		for _, pe := range ev.Plates {
			kinds = append(kinds, pe.Kind)
		}
		assert.Contains(t, kinds, planning.ColdPlateKind)
		for _, is := range ev.Issues {
			assert.NotEqual(t, "S9", is.Rule, "a cold plate holds no add-on excess")
		}
		for _, v := range violations(ev.Issues) {
			assert.Equal(t, "plate_outside_target", v.Code)
		}
	})
	t.Run("without suitable no-cook foods the same dish stays and the note says so", func(t *testing.T) {
		s, tgt, p := sameDishDay()
		got, notes := planning.GuardDayMains(s, tgt, p, nil)
		assert.Equal(t, p, got)
		require.Len(t, notes, 1)
		assert.Contains(t, notes[0], "no suitable no-cook foods")
	})
	t.Run("a fresh main is used when the day may cook twice", func(t *testing.T) {
		s, tgt, p := sameDishDay()
		s.Settings.MaxCookOccasionsPerDay = 2
		got, notes := planning.GuardDayMains(s, tgt, p, nil)
		dinnerMain := mainAt(got, tue, dinner)
		assert.NotEqual(t, int64(104), dinnerMain.FoodID)
		assert.Empty(t, dinnerMain.SessionKey, "cooked fresh for its own plate")
		require.Len(t, notes, 1)
		assert.Contains(t, notes[0], "fresh main")
	})
	t.Run("the evaluation shows the replacement", func(t *testing.T) {
		s, tgt, p := sameDishDay()
		s.Sessions = []planning.Session{persistedSession(7, 105, mon, dinner, 4, false)}
		view, _, done := planning.PlannerViewOf(planning.ChooseInput{State: s, Target: tgt}, p)
		assert.EqualValues(t, 105, mainAt(done, tue, dinner).FoodID)
		assert.Contains(t, view["code_changes"], "Day variety: "+"Tuesday slot 2: Main 5 replaced the repeated Main 4 (leftovers of a batch the week already has).")
	})
	t.Run("without another main the dish stays and the note says why", func(t *testing.T) {
		s, tgt, p := sameDishDay()
		got, notes := planning.GuardDayMains(s, tgt, p, nil)
		assert.Equal(t, p, got)
		require.Len(t, notes, 1)
		assert.Contains(t, notes[0], "kept, no other main fits")
		assert.Contains(t, notes[0], "limited to 1")
	})
	t.Run("an excluded food is not placed", func(t *testing.T) {
		s, tgt, p := sameDishDay()
		s.Sessions = []planning.Session{persistedSession(7, 105, mon, dinner, 4, false)}
		got, notes := planning.GuardDayMains(s, tgt, p, map[int64]bool{105: true})
		assert.Equal(t, p, got)
		require.Len(t, notes, 1)
		assert.True(t, strings.Contains(notes[0], "kept"), notes[0])
	})
	t.Run("different mains are left alone", func(t *testing.T) {
		s, tgt, _ := sameDishDay()
		p := planning.Proposal{
			Sessions: []planning.ProposedSession{session("s1", 104, tue, lunch, 1), session("s2", 106, tue, dinner, 1)},
			Plates:   []planning.ProposedPlate{pplate(tue, lunch, onKey(104, "s1")), pplate(tue, dinner, onKey(106, "s2"))},
		}
		got, notes := planning.GuardDayMains(s, tgt, p, nil)
		assert.Equal(t, p, got)
		assert.Empty(t, notes)
	})
	t.Run("a whole-week run is the week guard's", func(t *testing.T) {
		s, _, p := sameDishDay()
		got, notes := planning.GuardDayMains(s, windowTarget(s), p, nil)
		assert.Equal(t, p, got)
		assert.Empty(t, notes)
	})
}
