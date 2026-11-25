package planning

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
)

// fillerState is a library of the mains and fillers the CP1 judges named.
func fillerState(plateFit bool) State {
	main := func(id int64, name string, g FoodGroups, protein float64) FoodFacts {
		return FoodFacts{
			ID: id, Name: name, Kind: food.KindComposed, Role: food.RoleMain, Groups: g,
			Macros: nutrition.Macros{Kcal: 500, Protein: protein},
		}
	}
	leaf := func(id int64, name string, g FoodGroups) FoodFacts {
		return FoodFacts{
			ID: id, Name: name, Kind: food.KindLeaf, NoCook: true, NutritionComplete: true, Groups: g,
			Macros: nutrition.Macros{Kcal: 100, Protein: 5},
		}
	}
	s := State{Foods: map[int64]FoodFacts{}}
	s.Settings.PlateFit = plateFit
	for _, f := range []FoodFacts{
		main(1, "Pasta with pesto and peas", FoodGroups{}, 15),
		main(2, "Tuna pasta with tomatoes", FoodGroups{FishG: 60}, 30),
		main(3, "Quick tomato soup", FoodGroups{}, 8),
		main(4, "Pork schnitzel", FoodGroups{MeatG: 150}, 40),
		main(5, "Chicken curry", FoodGroups{MeatG: 120}, 35),
		main(6, "Vegetable stir-fry", FoodGroups{}, 10),
		main(7, "Green salad bowl", FoodGroups{}, 8),
		main(8, "Chili sin carne", FoodGroups{LegumeG: 100}, 22),
		main(9, "Hasselback potatoes", FoodGroups{}, 8),
		main(10, "Lentil soup", FoodGroups{LegumeG: 20}, 10),
		leaf(20, "Smoked salmon", FoodGroups{FishG: 100}),
		leaf(21, "Tofu (firm)", FoodGroups{LegumeG: 100}),
		leaf(22, "Muesli (unsweetened)", FoodGroups{}),
		leaf(23, "Banana", FoodGroups{}),
		leaf(24, "Natural yogurt", FoodGroups{}),
		leaf(25, "Low-fat quark", FoodGroups{}),
		leaf(26, "Ciabatta", FoodGroups{}),
		leaf(27, "Cucumber", FoodGroups{}),
		leaf(28, "Hummus", FoodGroups{LegumeG: 60}),
		leaf(29, "Milk", FoodGroups{}),
		leaf(30, "Strawberry jam", FoodGroups{}),
		leaf(31, "Egg", FoodGroups{}),
		leaf(32, "Parmesan", FoodGroups{}),
		leaf(33, "Feta", FoodGroups{}),
		leaf(34, "Cashew nuts", FoodGroups{}),
		main(11, "Salmon with broccoli", FoodGroups{FishG: 150}, 35),
		main(12, "Mushroom risotto", FoodGroups{}, 10),
		main(13, "Cheese spaetzle", FoodGroups{}, 20),
		main(14, "Shakshuka", FoodGroups{}, 20),
		main(15, "Potato gratin", FoodGroups{}, 12),
		main(16, "Greek salad", FoodGroups{}, 12),
		{ID: 41, Name: "Steamed rice", Kind: food.KindComposed, Role: food.RoleSideStarch},
		{ID: 42, Name: "Carrot salad", Kind: food.KindComposed, Role: food.RoleSideVeg},
		{ID: 40, Name: "Fruit salad", Kind: food.KindComposed, Role: food.RoleDessert, NoCook: true},
	} {
		s.Foods[f.ID] = f
	}
	return s
}

func fillerPlate(ids ...int64) Plate {
	p := Plate{}
	for _, id := range ids {
		p.Components = append(p.Components, Component{FoodID: id})
	}
	return p
}

func TestFillerFits(t *testing.T) {
	s := fillerState(true)
	tests := []struct {
		name   string
		filler int64
		plate  Plate
		slot   string
		want   bool
	}{
		// Rule 1: breakfast, sweet and drink foods stay at breakfast; dairy only as a dollop.
		{"muesli beside a schnitzel", 22, fillerPlate(4), "dinner", false},
		{"muesli at breakfast", 22, fillerPlate(), "breakfast", true},
		{"jam beside a soup", 30, fillerPlate(3), "lunch", false},
		{"banana beside a curry", 23, fillerPlate(5), "lunch", false},
		{"fruit salad beside a curry", 40, fillerPlate(5), "dinner", false},
		{"milk beside a soup", 29, fillerPlate(3), "dinner", false},
		{"yogurt dollop on a chili", 24, fillerPlate(8), "dinner", true},
		{"yogurt on a pesto pasta", 24, fillerPlate(1), "dinner", false},
		{"quark on potatoes", 25, fillerPlate(9), "dinner", true},
		{"quark on top of yogurt", 25, fillerPlate(8, 24), "dinner", false},
		// Rule 2: no second protein of another kind.
		{"smoked salmon on a tuna pasta", 20, fillerPlate(2), "lunch", false},
		{"tofu beside a meat curry", 21, fillerPlate(5), "dinner", false},
		{"tofu beside a chili with beans", 21, fillerPlate(8), "dinner", false},
		{"hummus beside a protein side", 28, fillerPlate(3, 31), "lunch", false},
		// Rule 3: a protein filler must fit the dish.
		{"smoked salmon on a hot tomato soup", 20, fillerPlate(3), "lunch", false},
		{"smoked salmon on a pesto pasta", 20, fillerPlate(1), "lunch", false},
		{"smoked salmon on a salad bowl", 20, fillerPlate(7), "lunch", true},
		{"tofu on a tomato soup", 21, fillerPlate(3), "dinner", false},
		{"tofu on a vegetable stir-fry", 21, fillerPlate(6), "dinner", true},
		{"egg on a salad bowl", 31, fillerPlate(7), "lunch", true},
		{"hummus with a lentil-light soup", 28, fillerPlate(10), "lunch", true},
		// Dairy as a cold sauce: dill yogurt with fish.
		{"yogurt sauce with salmon", 24, fillerPlate(11), "dinner", true},
		// Cheese and nuts top a dish that takes one (toppingFits).
		{"parmesan on a risotto", 32, fillerPlate(12), "dinner", true},
		{"parmesan on a pesto pasta", 32, fillerPlate(1), "dinner", true},
		{"feta on a shakshuka", 33, fillerPlate(14), "lunch", true},
		{"feta on a salad bowl", 33, fillerPlate(7), "lunch", true},
		{"parmesan on a tuna pasta (fish)", 32, fillerPlate(2), "dinner", false},
		{"feta beside salmon", 33, fillerPlate(11), "dinner", false},
		{"parmesan on a curry", 32, fillerPlate(5), "dinner", false},
		{"parmesan on a tomato soup", 32, fillerPlate(3), "dinner", false},
		{"parmesan on a potato gratin (cheese-baked)", 32, fillerPlate(15), "dinner", false},
		{"feta on a Greek salad (holds feta)", 33, fillerPlate(16), "lunch", false},
		{"quark on a Greek salad", 25, fillerPlate(16), "lunch", false},
		{"feta on cheese spaetzle", 33, fillerPlate(13), "dinner", false},
		{"feta beside parmesan", 33, fillerPlate(12, 32), "dinner", false},
		{"cashews on a curry", 34, fillerPlate(5), "dinner", true},
		{"cashews on a stir-fry", 34, fillerPlate(6), "dinner", true},
		{"cashews on a pesto pasta", 34, fillerPlate(1), "dinner", false},
		// A plate holds its main and at most two sides or add-ons.
		{"cucumber as the second side", 27, fillerPlate(4, 42), "dinner", true},
		{"cucumber as the third side", 27, fillerPlate(4, 41, 42), "dinner", false},
		{"muesli at breakfast beside three", 22, fillerPlate(24, 23, 29), "breakfast", true},
		// Sides that fit as before.
		{"bread with a soup", 26, fillerPlate(3), "lunch", true},
		{"cucumber beside a schnitzel", 27, fillerPlate(4), "dinner", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, s.fillerFits(tt.plate, s.Foods[tt.filler], tt.slot))
		})
	}
}

func TestFillerFitsPlateFitOff(t *testing.T) {
	// Switch off = no change: every filler the planner had is still a candidate.
	s := fillerState(false)
	assert.True(t, s.fillerFits(fillerPlate(2), s.Foods[20], "lunch"))
	assert.True(t, s.fillerFits(fillerPlate(4), s.Foods[22], "dinner"))
}

func TestFillerFitsCuisine(t *testing.T) {
	// A yogurt dollop goes on a soup, but not on a Japanese one (the cuisine table).
	s := fillerState(true)
	assert.True(t, s.fillerFits(fillerPlate(3), s.Foods[24], "dinner"))
	soup := s.Foods[3]
	soup.Attrs = flavor.Attributes{Cuisine: "japanese"}
	s.Foods[3] = soup
	assert.False(t, s.fillerFits(fillerPlate(3), s.Foods[24], "dinner"))
}

func TestDropMisfitAdds(t *testing.T) {
	day := time.Date(2031, 3, 10, 0, 0, 0, 0, time.UTC)
	prop := func() Proposal {
		return Proposal{Plates: []ProposedPlate{{Date: day, SlotID: 3, Eaters: 1, Components: []ProposedComponent{
			{FoodID: 3, Portions: 1, SessionKey: "s1"}, // a tomato soup that replaced the old main
			{FoodID: 20, Grams: 100},                   // smoked salmon picked for the old main
			{FoodID: 26, Grams: 80},                    // bread still fits the soup
		}}}, Sessions: []ProposedSession{{Key: "s1", FoodID: 3, CookDate: day, CookSlotID: 3, Portions: 1}}}
	}
	s := fillerState(true)
	s.SlotNames = map[int64]string{3: "slot.dinner"}
	got := dropMisfitAdds(s, Target{}, prop())
	require.Len(t, got.Plates[0].Components, 2)
	assert.Equal(t, int64(3), got.Plates[0].Components[0].FoodID)
	assert.Equal(t, int64(26), got.Plates[0].Components[1].FoodID)

	off := fillerState(false)
	off.SlotNames = s.SlotNames
	assert.Len(t, dropMisfitAdds(off, Target{}, prop()).Plates[0].Components, 3, "plate fit off: the proposal as it is")
}
