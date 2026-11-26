package planning

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
)

const (
	nfApple, nfCucumber = int64(41), int64(42)
)

// nutritionFillPlanner is fillerPlanner's day with plate fit on, its dishes
// grown to their 1.5x cap (the culinary passes ran) and one no-cook filler:
// an apple (never beside a savoury main) or the chickpeas (fit the curry and
// the bowl). atCap fills both meal plates to main + 2 (a cucumber on each,
// the couscous on the lunch too). The breakfast took an add already, so the
// adds go to the meal plates. The day eats 2349 kcal (2764 at the cap); kcal
// sets the target.
func nutritionFillPlanner(filler int64, atCap bool, kcal float64) *planner {
	h, _ := fillerPlanner(true, 55, true)
	apple := h.s.Foods[pfBread]
	apple.ID, apple.Name = nfApple, "Apfel"
	h.s.Foods[nfApple] = apple
	veg := func(id int64, name string) FoodFacts {
		return FoodFacts{
			ID: id, Name: name, Kind: food.KindLeaf, LeafRole: food.RoleSideVeg, NoCook: true, NutritionComplete: true,
			DietClass: "vegan", ServingG: 100, Macros: nutrition.Macros{Kcal: 20, Protein: 1, Carbs: 3},
		}
	}
	h.s.Foods[nfCucumber] = veg(nfCucumber, "Gurke")
	h.s.NoCook = []int64{filler}
	h.s.Profile.KcalTarget = &kcal
	h.in.State = h.s
	for pi := range h.prop.Plates {
		p := &h.prop.Plates[pi]
		for ci := range p.Components {
			if p.SlotID != bfBreakfast {
				h.topUpBase[[2]int{pi, ci}] = 1
				p.Components[ci].Portions = 1.5
			}
		}
		if atCap && p.SlotID != bfBreakfast {
			p.Components = append(p.Components, ProposedComponent{FoodID: nfCucumber, Grams: 100})
			if p.SlotID == bfLunch {
				p.Components = append(p.Components, ProposedComponent{FoodID: bfCouscous, Portions: 1.5, SessionKey: "c"})
				h.topUpBase[[2]int{pi, len(p.Components) - 1}] = 1
			}
		}
	}
	for i := range h.prop.Sessions {
		if s := &h.prop.Sessions[i]; s.CookSlotID != bfBreakfast {
			s.Portions = 1.5
			if atCap && s.Key == "c" {
				s.Portions = 3
			}
		}
	}
	h.addsOf(bfMon).used[bfBreakfast] = true
	return h
}

// fillChoices are the nutrition_fill choices the planner logged.
func fillChoices(h *planner) []heuristicChoice {
	return slices.DeleteFunc(slices.Clone(h.choices), func(c heuristicChoice) bool { return c.Reason != "nutrition_fill" })
}

func holds(h *planner, id int64) bool {
	return slices.ContainsFunc(h.prop.Plates, func(p ProposedPlate) bool {
		return slices.ContainsFunc(p.Components, func(c ProposedComponent) bool { return c.FoodID == id })
	})
}

// PLATE-4: the culinary pass adds only fitting fillers under the plate cap;
// the nutrition pass (pre-R1 ranking, no filter, no cap) runs only where
// the day is still under its band, and logs each add.
func TestNutritionFill_FallbackOnlyWhenShort(t *testing.T) {
	tests := []struct {
		name     string
		filler   int64
		atCap    bool
		kcal     float64 // 2850 (3300 at the cap) leaves the day under its 90 % band, 2450 leaves day and week in theirs
		culinary bool    // the culinary pass adds the filler
		fallback bool    // the nutrition pass adds it
	}{
		{name: "fitting filler with room: the culinary pass adds it", filler: pfChickpeas, kcal: 2850, culinary: true},
		{name: "misfit filler, day short: the nutrition pass adds it", filler: nfApple, kcal: 2850, fallback: true},
		{name: "fitting filler at the plate cap, day short: the nutrition pass passes the cap", filler: pfChickpeas, atCap: true, kcal: 3300, fallback: true},
		{name: "misfit filler, day in its band: nothing", filler: nfApple, kcal: 2450},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := nutritionFillPlanner(tt.filler, tt.atCap, tt.kcal)
			dk := h.dayNow(bfMon)
			require.NotNil(t, dk)
			require.InDelta(t, map[bool]float64{false: 2349, true: 2764}[tt.atCap], dk.Kcal, 1)
			limit := dk.Adjusted * 0.9
			assert.Equal(t, tt.culinary, dk.Kcal < limit && h.addNoCook(bfMon, false, limit), "culinary pass")
			n := len(h.prop.Plates[1].Components) + len(h.prop.Plates[2].Components)
			h.nutritionFill()
			assert.True(t, h.s.Settings.PlateFit, "plate fit is back on")
			assert.False(t, h.nutritionPass)
			assert.Equal(t, tt.culinary || tt.fallback, holds(h, tt.filler))
			if tt.fallback {
				require.Len(t, fillChoices(h), 1)
				assert.Equal(t, []int64{tt.filler}, fillChoices(h)[0].Foods)
				assert.Equal(t, n+1, len(h.prop.Plates[1].Components)+len(h.prop.Plates[2].Components))
			} else {
				assert.Empty(t, fillChoices(h), "no nutrition-pass add")
			}
		})
	}
}

// fatPctAfter: 7 days at 2000 kcal and 30 % fat; an add of 500 kcal with
// 450 fat kcal takes the week to (4200+450)/14500.
func TestFatPctAfter(t *testing.T) {
	w := WeekEval{Days: 7, AvgKcal: 2000, FatPct: 30}
	assert.InDelta(t, 30, fatPctAfter(w, 0, 0), 1e-9)
	assert.InDelta(t, 4650.0/14500*100, fatPctAfter(w, 450, 500), 1e-9)
}

// CP1 family: the nutrition pass put plain tofu beside a lentil curry. A
// second protein beside a main that carries its own comes after every other
// verified food; when it is the only one, nutrition still wins (PLATE-4).
func TestNutritionFill_SecondProteinLast(t *testing.T) {
	const nfTofu = int64(43)
	for _, tt := range []struct {
		name   string
		noCook []int64
		want   int64
	}{
		{name: "another food: not the tofu", noCook: []int64{nfTofu, nfApple}, want: nfApple},
		{name: "the tofu alone: nutrition first", noCook: []int64{nfTofu}, want: nfTofu},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := nutritionFillPlanner(nfApple, false, 2850)
			h.s.Foods[nfTofu] = FoodFacts{
				ID: nfTofu, Name: "Tofu (firm)", Kind: food.KindLeaf, NoCook: true, NutritionComplete: true,
				DietClass: "vegan", ServingG: 170, Macros: nutrition.Macros{Kcal: 144, Protein: 16, Fat: 8, Carbs: 2},
			}
			for _, p := range h.prop.Plates {
				if p.SlotID != bfBreakfast {
					m := h.s.Foods[p.Components[0].FoodID]
					m.Name, m.Groups.LegumeG = "Lentil curry", 150
					h.s.Foods[m.ID] = m
				}
			}
			h.s.NoCook = tt.noCook
			h.in.State = h.s
			h.nutritionFill()
			require.NotEmpty(t, fillChoices(h))
			assert.Equal(t, []int64{tt.want}, fillChoices(h)[0].Foods)
		})
	}
}
