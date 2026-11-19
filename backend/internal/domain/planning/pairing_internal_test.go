package planning

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

func TestAddOnFits(t *testing.T) {
	leaf := func(id int64, name string) FoodFacts {
		return FoodFacts{ID: id, Name: name, Kind: food.KindLeaf, NoCook: true, NutritionComplete: true}
	}
	meat := FoodFacts{ID: 1, Name: "Beef ragù", Kind: food.KindComposed, Role: food.RoleMain, Groups: FoodGroups{MeatG: 80}}
	curry := FoodFacts{ID: 2, Name: "Lentil curry", Kind: food.KindComposed, Role: food.RoleMain, Groups: FoodGroups{LegumeG: 90}}
	salad := FoodFacts{ID: 3, Name: "Tomato salad", Kind: food.KindComposed, Role: food.RoleSideVeg}
	yogurt := leaf(4, "Greek yogurt")
	fish := leaf(5, "Smoked salmon")
	fish.Groups = FoodGroups{FishG: 100}
	saladMain := FoodFacts{ID: 6, Name: "Greek salad with chickpeas", Kind: food.KindComposed, Role: food.RoleMain}
	bread := func(id int64) FoodFacts {
		f := leaf(id, "Wholegrain bread")
		f.LeafRole = food.RoleSideStarch
		return f
	}
	pasta := FoodFacts{ID: 30, Name: "Tuna pasta", Kind: food.KindComposed, Role: food.RoleMain, Groups: FoodGroups{FishG: 60}}
	soup := FoodFacts{ID: 31, Name: "Tomato soup", Kind: food.KindComposed, Role: food.RoleMain}
	starchSide := func(id int64, name string) FoodFacts {
		return FoodFacts{ID: id, Name: name, Kind: food.KindComposed, Role: food.RoleSideStarch}
	}
	dessert := func(id int64, name string) FoodFacts {
		return FoodFacts{ID: id, Name: name, Kind: food.KindComposed, Role: food.RoleDessert}
	}
	s := State{PairAddOns: true, Foods: map[int64]FoodFacts{1: meat, 2: curry, 3: salad, 4: yogurt, 6: saladMain, 30: pasta, 31: soup, 41: starchSide(41, "Steamed rice")}}
	plate := func(ids ...int64) Plate {
		p := Plate{}
		for _, id := range ids {
			p.Components = append(p.Components, Component{FoodID: id})
		}
		return p
	}
	recipe := func(id int64, name string, g FoodGroups) FoodFacts {
		return FoodFacts{ID: id, Name: name, Kind: food.KindComposed, Role: food.RoleSideVeg, Groups: g}
	}
	tests := []struct {
		name  string
		food  FoodFacts
		plate Plate
		slot  string
		want  bool
	}{
		{"bread on a curry", leaf(10, "Ciabatta"), plate(2), "lunch", true},
		{"fruit on a ragù", leaf(11, "Banana"), plate(1), "dinner", true},
		{"plain dairy on a curry", leaf(12, "Skyr (natural)"), plate(2), "lunch", true},
		{"mozzarella never", leaf(13, "Mozzarella"), plate(3), "lunch", false},
		{"cheddar never", leaf(14, "Cheddar"), plate(2), "dinner", false},
		{"nuts on a curry", leaf(15, "Mixed nuts"), plate(2), "lunch", false},
		{"nuts at breakfast", leaf(15, "Mixed nuts"), plate(2), "breakfast", true},
		{"nuts on a salad main", leaf(15, "Mixed nuts"), plate(6), "lunch", true},
		{"nuts on a ragù with a side salad", leaf(15, "Mixed nuts"), plate(1, 3), "dinner", false},
		{"nuts with yogurt on the plate", leaf(15, "Mixed nuts"), plate(2, 4), "dinner", true},
		{"coconut yogurt is no nut", leaf(16, "Coconut yogurt"), plate(2), "lunch", true},
		{"tofu on a ragù", leaf(17, "Tofu (firm)"), plate(1), "dinner", false},
		{"tofu on a curry", leaf(17, "Tofu (firm)"), plate(2), "dinner", true},
		{"egg on a ragù", leaf(18, "Egg"), plate(1), "dinner", false},
		{"smoked salmon on a ragù", fish, plate(1), "dinner", false},
		{"smoked salmon on a curry", fish, plate(2), "dinner", true},
		{"a tofu recipe as a side of a ragù", recipe(20, "Marinated tofu", FoodGroups{}), plate(1, 3), "dinner", false},
		{"a tofu recipe as a side of a curry", recipe(20, "Marinated tofu", FoodGroups{}), plate(2), "dinner", true},
		{"an egg recipe on a ragù", recipe(21, "Egg fried rice", FoodGroups{}), plate(1), "dinner", false},
		{"a meat recipe as a side of a ragù", recipe(22, "Chicken salad", FoodGroups{MeatG: 60}), plate(1), "lunch", false},
		{"a vegetable recipe on a ragù", recipe(23, "Roasted carrots", FoodGroups{}), plate(1), "dinner", true},
		{"nuts beside a pasta even with yogurt", leaf(15, "Peanuts (salted)"), plate(30, 4), "dinner", false},
		{"bread beside a pasta is a second starch", bread(10), plate(30), "dinner", false},
		{"bread with a soup", bread(10), plate(31), "lunch", true},
		{"a second starch side on a plate that has one", starchSide(40, "Boiled potatoes"), plate(31, 41), "dinner", false},
		{"a starch side beside a ragù", starchSide(40, "Boiled potatoes"), plate(1), "dinner", true},
		{"muesli at dinner", leaf(60, "Muesli (unsweetened)"), plate(31), "dinner", false},
		{"muesli at breakfast", leaf(60, "Muesli (unsweetened)"), plate(31), "breakfast", true},
		{"a fruit salad beside a pasta", dessert(50, "Fruit salad"), plate(30), "dinner", false},
		{"a fruit salad at breakfast", dessert(50, "Fruit salad"), plate(30), "breakfast", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, s.addOnFits(tt.plate, tt.food, tt.slot))
		})
	}
	t.Run("a removed food never comes back", func(t *testing.T) {
		s := s
		s.Avoid = map[int64]bool{12: true}
		assert.False(t, s.addOnFits(plate(2), leaf(12, "Skyr (natural)"), "lunch"))
	})
	t.Run("off: the planner's own top-ups keep their candidates", func(t *testing.T) {
		s := s
		s.PairAddOns = false
		assert.True(t, s.addOnFits(plate(1), leaf(17, "Tofu (firm)"), "dinner"))
	})
}

func TestDemoteUnclean(t *testing.T) {
	warn := Issue{Rule: "S9", Code: CodeAddOnExcess, Severity: SeveritySoft}
	clean := Way{What: "clean"}
	dirty := Way{What: "dirty", NewIssues: []Issue{warn}}
	assert.Equal(t, []Way{clean}, DemoteUnclean([]Way{dirty, clean}), "a clean way drops the unclean ones")
	got := DemoteUnclean([]Way{dirty})
	assert.Len(t, got, 1, "without a clean one the unclean stays")
	assert.Contains(t, got[0].Why, "creates a new warning", "and says so")
}

// Breakfast food is not food for lunch or dinner; sweet leaves only where the
// plan itself eats one at a meal.
func TestMealFits(t *testing.T) {
	muesli := FoodFacts{ID: 20, Name: "Muesli with yogurt and berries", Kind: food.KindComposed, Role: food.RoleStandalone, Tags: []string{"Breakfast"}}
	oats := FoodFacts{ID: 21, Name: "Rolled oats", Kind: food.KindLeaf}
	plain := FoodFacts{ID: 22, Name: "Granola", Kind: food.KindLeaf}
	honey := FoodFacts{ID: 23, Name: "Honey", Kind: food.KindLeaf, Sweet: true}
	jam := FoodFacts{ID: 24, Name: "Jam", Kind: food.KindLeaf, Sweet: true, LeafRole: food.RoleDessert}
	ragu := FoodFacts{ID: 1, Name: "Beef ragù", Kind: food.KindComposed, Role: food.RoleMain}
	s := State{PairAddOns: true, Foods: map[int64]FoodFacts{20: muesli, 21: oats, 22: plain, 23: honey, 24: jam, 1: ragu}}
	for _, f := range []FoodFacts{muesli, oats, plain} {
		assert.True(t, s.MealFits(f, "breakfast"), f.Name)
		assert.False(t, s.MealFits(f, "dinner"), f.Name)
		assert.False(t, s.MealFits(f, "lunch"), f.Name)
	}
	assert.False(t, s.MealFits(honey, "dinner"), "a sweet leaf is not a dinner side")
	assert.True(t, s.MealFits(jam, "dinner"), "a dessert may be sweet")
	assert.True(t, s.MealFits(ragu, "dinner"))
	// The plan itself eats muesli and honey at dinner: then they fit there.
	s.SlotNames = map[int64]string{3: "slot.dinner"}
	s.Plates = []Plate{{ID: 5, SlotID: 3, Components: []Component{{FoodID: 20}, {FoodID: 23}}}}
	assert.True(t, s.MealFits(muesli, "dinner"))
	assert.True(t, s.MealFits(honey, "dinner"))
	// As a add-on a standalone recipe is never offered.
	assert.False(t, s.addOnFits(Plate{}, muesli, "breakfast"), "a whole meal is no side")
}

// A second food of the same kind never goes on a plate; the one there is raised.
func TestSameKindAndMisfitSides(t *testing.T) {
	yogurt := FoodFacts{ID: 1, Name: "Greek yogurt", Kind: food.KindLeaf}
	skyr := FoodFacts{ID: 2, Name: "Skyr (natural)", Kind: food.KindLeaf}
	pasta := FoodFacts{ID: 3, Name: "Pasta al pomodoro", Kind: food.KindComposed, Role: food.RoleMain}
	chili := FoodFacts{ID: 4, Name: "Chili", Kind: food.KindComposed, Role: food.RoleMain}
	potatoes := FoodFacts{ID: 5, Name: "Roast potatoes", Kind: food.KindComposed, Role: food.RoleSideStarch}
	bread := FoodFacts{ID: 6, Name: "Ciabatta", Kind: food.KindLeaf, LeafRole: food.RoleSideStarch}
	s := State{PairAddOns: true, Foods: map[int64]FoodFacts{1: yogurt, 2: skyr, 3: pasta, 4: chili, 5: potatoes, 6: bread}}
	plate := func(ids ...int64) Plate {
		p := Plate{}
		for i, id := range ids {
			p.Components = append(p.Components, Component{ID: int64(10 + i), FoodID: id, Position: map[bool]plate.Position{true: plate.PositionMain, false: plate.PositionSide}[i == 0]})
		}
		return p
	}
	_, kind, ok := s.SameKind(plate(4, 1), skyr)
	assert.True(t, ok, "skyr on yogurt")
	assert.Equal(t, "cultured dairy", kind)
	assert.False(t, s.addOnFits(plate(4, 1), skyr, "dinner"), "an add-on of the same kind does not fit")
	_, _, ok = s.SameKind(plate(4), skyr)
	assert.False(t, ok, "the first one is fine")
	ex, kind, ok := s.SameKind(plate(4, 5), bread)
	assert.True(t, ok, "bread beside potatoes: %s", kind)
	assert.Positive(t, ex.ID, "the potatoes can be raised instead")
	// A pasta main takes no potatoes; a chili main does.
	assert.Equal(t, []int64{11}, s.MisfitSides(plate(3, 5)))
	assert.Empty(t, s.MisfitSides(plate(4, 5)))
}
