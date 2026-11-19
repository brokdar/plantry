package planning

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
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
	s := State{PairAddOns: true, Foods: map[int64]FoodFacts{1: meat, 2: curry, 3: salad, 4: yogurt, 6: saladMain}}
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
