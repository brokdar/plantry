package planning_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// addOnFood registers a no-cook leaf (kcal and protein per 100 g).
func addOnFood(s *planning.State, id int64, name string, kcal float64) {
	s.Foods[id] = planning.FoodFacts{
		ID: id, Name: name, Kind: food.KindLeaf, DietClass: "vegan", NoCook: true, NutritionComplete: true,
		Macros: nutrition.Macros{Kcal: kcal, Protein: 1},
	}
	s.NoCook = append(s.NoCook, id)
}

// addTo puts grams of food id on plate pid.
func addTo(s *planning.State, pid, id int64, grams float64) {
	for i := range s.Plates {
		if s.Plates[i].ID == pid {
			s.Plates[i].Components = append(s.Plates[i].Components, planning.Component{FoodID: id, Grams: f64(grams)})
		}
	}
}

func s9Of(s planning.State) []planning.Issue {
	return softOf(planning.Evaluate(s, windowTarget(s)), "S9")
}

func TestS9_OverCap(t *testing.T) {
	s := softState(2200)
	addTo(&s, 2, apple, 150)
	assert.Empty(t, s9Of(s), "150 g is within two servings (default serving 100 g) and 200 g")

	addTo(&s, 3, apple, 250)
	got := s9Of(s)
	require.Len(t, got, 1)
	assert.Equal(t, planning.CodeAddOnExcess, got[0].Code)
	assert.Equal(t, planning.SeveritySoft, got[0].Severity)
	assert.Equal(t, planning.AddOnOverCap, got[0].Params["reason"])
	assert.EqualValues(t, apple, got[0].Params["food_id"])
	assert.EqualValues(t, 3, *got[0].PlateID)
	assert.EqualValues(t, 200, got[0].Params["max_grams"])
}

func TestS9_CapFollowsTheServingAndTheDayShare(t *testing.T) {
	s := softState(2200)
	a := s.Foods[apple]
	a.ServingG = 50 // cap 100 g
	s.Foods[apple] = a
	addTo(&s, 2, apple, 110)
	got := s9Of(s)
	require.Len(t, got, 1)
	assert.EqualValues(t, 100, got[0].Params["max_grams"])

	// 20 % of a 2200 kcal day is 440 kcal: 60 g of a 900 kcal food at most...
	s = softState(2200)
	addOnFood(&s, 30, "Oil crackers", 900)
	addTo(&s, 2, 30, 100)
	got = s9Of(s)
	require.Len(t, got, 1)
	assert.Equal(t, planning.AddOnOverCap, got[0].Params["reason"])
	assert.LessOrEqual(t, got[0].Params["max_grams"], 50.0)
}

func TestS9_LightMeal(t *testing.T) {
	s := softState(2200)
	s.SlotNames = map[int64]string{breakfast: "slot.breakfast", lunch: "slot.lunch", dinner: "slot.dinner"}
	s.Light = []planning.LightIntent{{Date: sat, Scope: planning.LightScopeMeal, Slot: "lunch"}}
	addTo(&s, 2, apple, 50)
	addTo(&s, 3, apple, 50)
	got := s9Of(s)
	require.Len(t, got, 1, "only the light lunch")
	assert.Equal(t, planning.AddOnLightMeal, got[0].Params["reason"])
	assert.EqualValues(t, 2, *got[0].PlateID)
}

func TestS9_DayLimit(t *testing.T) {
	s := softState(2200)
	for i, name := range []string{"Banana", "Pear", "Carrot"} {
		addOnFood(&s, int64(31+i), name, 50)
	}
	addTo(&s, 2, apple, 50)
	addTo(&s, 2, 31, 50)
	addTo(&s, 3, 32, 50)
	assert.Empty(t, s9Of(s), "three add-ons a day are fine")
	addTo(&s, 3, 33, 50)
	got := s9Of(s)
	require.Len(t, got, 1)
	assert.Equal(t, planning.AddOnDayLimit, got[0].Params["reason"])
	assert.EqualValues(t, 33, got[0].Params["food_id"])
}

func TestS9_SameAddOnOnManyPlates(t *testing.T) {
	s := softState(2200)
	// Plates on four more days (the cells need not plan them for S9).
	for i, d := range []string{sun, mon, tue, wed} {
		p := persisted(int64(10+i), d, lunch, false, chili, 1)
		s.Plates = append(s.Plates, p)
	}
	for _, pid := range []int64{2, 10, 11} {
		addTo(&s, pid, apple, 50)
	}
	assert.Empty(t, s9Of(s), "three plates are fine")
	addTo(&s, 12, apple, 50)
	got := s9Of(s)
	require.Len(t, got, 1)
	assert.Equal(t, planning.AddOnRepeated, got[0].Params["reason"])
	assert.EqualValues(t, 12, *got[0].PlateID)
}

func TestS9_DuplicatesTheMain(t *testing.T) {
	s := softState(2200)
	m := s.Foods[chili]
	m.Leaves = map[string]float64{"apple": 40, "rice dry": 10}
	s.Foods[chili] = m
	addTo(&s, 2, apple, 50)
	addOnFood(&s, 40, "Rice dry", 130)
	addTo(&s, 3, 40, 50)
	got := s9Of(s)
	require.Len(t, got, 1, "apple is 40 g of the main, rice only 10 g")
	assert.Equal(t, planning.AddOnDuplicatesMain, got[0].Params["reason"])
	assert.EqualValues(t, apple, got[0].Params["food_id"])
}

func TestS9_IgnoresConfirmedAndRecipeChildren(t *testing.T) {
	s := softState(2200)
	s.Plates[1].Confirmed = true
	addTo(&s, 2, apple, 400)
	assert.Empty(t, s9Of(s), "a confirmed plate is the user's own")
	// A leaf that is not no-cook is not an add-on.
	addTo(&s, 3, riceLeaf, 400)
	assert.Empty(t, s9Of(s))
}

func TestS9_HeuristicNeverProducesIt(t *testing.T) {
	for _, kcal := range []float64{2000, 2800, 3200} {
		s := proteinState(profile.DietOmnivore)
		s.Profile.KcalTarget = f64(kcal)
		tgt := windowTarget(s)
		p := choose(t, planning.ChooseInput{State: s, Target: tgt})
		ev := planning.Evaluate(planning.Merge(s, tgt, p), tgt)
		assert.Empty(t, softOf(ev, "S9"), fmt.Sprint(kcal))
	}
}

func TestOptions_ServingSizeNoSweetsWhy(t *testing.T) {
	s := softState(2400)
	s.SlotNames = map[int64]string{breakfast: "slot.breakfast", lunch: "slot.lunch", dinner: "slot.dinner"}
	a := s.Foods[apple]
	a.ServingG = 60
	s.Foods[apple] = a
	addOnFood(&s, 50, "Chocolate", 540)
	c := s.Foods[50]
	c.Sweet = true
	s.Foods[50] = c
	// Dinner is light: the add-on goes to lunch, and says why.
	s.Light = []planning.LightIntent{{Date: sat, Scope: planning.LightScopeMeal, Slot: "dinner"}}
	ev := planning.Evaluate(s, windowTarget(s))
	s2 := softOf(ev, "S2")
	require.Len(t, s2, 1)
	var adds int
	for _, o := range s2[0].Options {
		assert.NotEqual(t, int64(50), o.FoodID, "sweets are never options")
		if o.Action == "add" {
			adds++
			require.NotNil(t, o.Grams)
			assert.LessOrEqual(t, *o.Grams, 120.0, "at most two servings")
			assert.InDelta(t, 0, float64(int(*o.Grams)%30), 0.01, "half servings of 60 g")
			assert.Equal(t, "lunch: dinner is light", o.Why)
			assert.EqualValues(t, lunch, o.SlotID)
		}
	}
	assert.Positive(t, adds)
}

func TestH9GetsOptions(t *testing.T) {
	s := softState(3200) // floor 2400, the day holds 1800
	ev := planning.Evaluate(s, windowTarget(s))
	var h9 []planning.Issue
	for _, is := range ev.Issues {
		if is.Code == planning.CodeKcalBelowFloor {
			h9 = append(h9, is)
		}
	}
	require.Len(t, h9, 1)
	require.NotEmpty(t, h9[0].Options)
	for _, o := range h9[0].Options {
		assert.Positive(t, o.KcalDelta)
		assert.Greater(t, o.DayKcal, 1800.0)
	}
}
