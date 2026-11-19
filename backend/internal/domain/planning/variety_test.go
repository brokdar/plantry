package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

func TestWeekEffectOf(t *testing.T) {
	s := meatyWeek()
	s.SlotNames = map[int64]string{breakfast: "slot.breakfast", lunch: "slot.lunch", dinner: "slot.dinner"}
	s.Foods[111] = recipe(111, "Turkey chili", food.RoleMain, "omnivore", 600)
	s.Foods[112] = recipe(112, "Chili con carne", food.RoleMain, "omnivore", 600)
	withGroups(s, 111, planning.FoodGroups{MeatG: 100})
	withGroups(s, 112, planning.FoodGroups{MeatG: 150})
	plate := func(id int64, date string, slot, foodID int64) planning.Plate {
		return planning.Plate{ID: id, Date: day(date), SlotID: slot, Eaters: 1, Components: []planning.Component{{FoodID: foodID, Portions: f64(1), Position: "main"}}}
	}
	s.Plates = append(s.Plates,
		plate(901, tue, lunch, 111), plate(902, tue, dinner, 111), // the same dish twice in a day
		plate(903, "2026-11-11", lunch, 112)) // Wednesday: another chili next to Tuesday's
	got := planning.WeekEffectOf(s)
	assert.InDelta(t, 100+100+150, got.MeatG, 1e-9)
	assert.InDelta(t, 300, got.LimitG, 1e-9)
	require.Len(t, got.Repeats, 3, "%v", got.Repeats)
	assert.Contains(t, got.Repeats[0], "Tuesday lunch and Tuesday dinner")
}

func TestDishFamilies(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"Turkey chili", "Chili con carne", true},
		{"Chili sin carne", "Chili con carne", true},
		{"Beef goulash", "Beef ragù", false}, // short words (beef, tofu) make no family
		{"Tofu stir-fry", "Baked cod with lemon potatoes", false},
		{"Tomato soup", "Tomato salad", true},
		{"Easy chicken curry", "Speedy lentil coconut curry", true},
		{"Shakshuka", "Egg fried rice", false},
	}
	for _, tt := range tests {
		t.Run(tt.a+" / "+tt.b, func(t *testing.T) {
			assert.Equal(t, tt.want, planning.SameFamily(tt.a, tt.b))
		})
	}
}
