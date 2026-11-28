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
		{"frozen spinach goes straight in", recipe([]string{"Frozen spinach"}), ""},
		{"TK-Erbsen go straight in", recipe([]string{"TK-Erbsen", "Pasta"}), ""},
		{"frozen berries", recipe([]string{"Frozen berries"}), ""},
		{"frozen chicken", recipe([]string{"Frozen chicken breast"}), ReminderThaw},
		{"TK-Lachsfilet", recipe([]string{"TK-Lachsfilet"}), ReminderThaw},
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

func TestPrepAheadOf(t *testing.T) {
	tests := []struct {
		name  string
		ing   []string
		steps []string
		want  *PrepAhead
	}{
		{"none", []string{"rice"}, []string{"Cook the rice."}, nil},
		{"marinate no time", nil, []string{"Marinate the chicken in miso."}, &PrepAhead{Kind: "marinate"}},
		{"marinate 24 hours", nil, []string{"Marinate for 24 hours."}, &PrepAhead{Kind: "marinate"}},
		{"marinate overnight", nil, []string{"Marinate overnight in the fridge."}, &PrepAhead{Kind: "marinate"}},
		{"marinate at least 4 hours", nil, []string{"Marinate for at least 4 hours."}, &PrepAhead{Kind: "marinate", Hours: 4}},
		{"marinate range takes the long end", nil, []string{"Marinate 2-3 hours."}, &PrepAhead{Kind: "marinate", Hours: 3}},
		{"de 24 Stunden", nil, []string{"24 Stunden marinieren."}, &PrepAhead{Kind: "marinate"}},
		{"de 2 Stunden", nil, []string{"Das Fleisch 2 Stunden marinieren."}, &PrepAhead{Kind: "marinate", Hours: 2}},
		{"marinate 15 minutes needs no mark", nil, []string{"Marinate for 15 minutes."}, nil},
		{"soak dried chickpeas", []string{"dried chickpeas"}, []string{"Soak the chickpeas."}, &PrepAhead{Kind: "soak"}},
		{"soak 6 hours", []string{"dried chickpeas"}, []string{"Soak for 6 hours."}, &PrepAhead{Kind: "soak", Hours: 6}},
		{"soak 30 minutes needs no mark", []string{"dried chickpeas"}, []string{"Soak for 30 minutes."}, nil},
		{"thaw", []string{"frozen salmon fillet"}, []string{"Cook."}, &PrepAhead{Kind: "thaw"}},
		{"frozen peas need no thaw", []string{"frozen peas"}, []string{"Cook."}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, PrepAheadOf(tt.ing, tt.steps))
		})
	}
}
