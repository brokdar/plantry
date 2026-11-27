package planview

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// PRO-1: the prep rule table: dried legumes that need soaking, marinating
// steps and frozen ingredients, English and German; one kind per recipe.
func TestPrepKind(t *testing.T) {
	recipe := func(leaves []string, steps ...string) *food.Food {
		f := &food.Food{Kind: food.KindComposed}
		for _, n := range leaves {
			f.Children = append(f.Children, food.FoodComponent{ChildName: n})
		}
		for _, s := range steps {
			f.Instructions = append(f.Instructions, food.Instruction{Text: s})
		}
		return f
	}
	for _, tc := range []struct {
		name string
		f    *food.Food
		want string
	}{
		{"dried chickpeas", recipe([]string{"Dried chickpeas", "Onion"}), ReminderSoak},
		{"getrocknete Bohnen", recipe([]string{"Weiße Bohnen, getrocknet"}), ReminderSoak},
		{"canned chickpeas", recipe([]string{"Chickpeas (canned)"}), ""},
		{"dried lentils need no soak", recipe([]string{"Dried red lentils"}), ""},
		{"marinate step", recipe([]string{"Chicken"}, "Marinate the chicken overnight."), ReminderMarinate},
		{"marinieren", recipe([]string{"Tofu"}, "Den Tofu 4 Stunden marinieren."), ReminderMarinate},
		{"frozen spinach", recipe([]string{"Frozen spinach"}), ReminderThaw},
		{"TK-Erbsen", recipe([]string{"TK-Erbsen"}), ReminderThaw},
		{"soak wins over marinate", recipe([]string{"Dried black beans"}, "Marinate the pork."), ReminderSoak},
		{"nothing", recipe([]string{"Pasta", "Tomato"}, "Boil the pasta."), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, PrepKind(tc.f))
		})
	}
}

// PRO-2: at most one Chef line: the first session cooked tomorrow that needs
// a step tonight; sessions on other days or without one give none.
func TestPrepReminder_OneLine(t *testing.T) {
	tomorrow := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	view := func(id, foodID int64, cook time.Time) plate.SessionView {
		v := plate.SessionView{}
		v.ID, v.FoodID, v.FoodName, v.CookDate = id, foodID, "R", cook
		return v
	}
	beans := &food.Food{Children: []food.FoodComponent{{ChildName: "dried kidney beans"}}}
	chicken := &food.Food{Instructions: []food.Instruction{{Text: "marinate"}}}
	recipes := map[int64]*food.Food{1: beans, 2: chicken, 3: {}}

	r, ok := prepReminder([]plate.SessionView{view(10, 3, tomorrow), view(11, 2, tomorrow), view(12, 1, tomorrow)}, tomorrow, recipes)
	assert.True(t, ok)
	assert.Equal(t, ReminderMarinate, r.Kind)
	assert.Equal(t, int64(11), r.SessionID)

	_, ok = prepReminder([]plate.SessionView{view(12, 1, tomorrow.AddDate(0, 0, 1))}, tomorrow, recipes)
	assert.False(t, ok, "not cooked tomorrow")
	_, ok = prepReminder([]plate.SessionView{view(12, 1, tomorrow)}, tomorrow, nil)
	assert.False(t, ok, "Chef off: no recipes loaded")
}
