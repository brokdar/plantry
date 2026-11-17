package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

func TestGroupsOf(t *testing.T) {
	produce := food.SectionProduce
	other := food.SectionOther
	g := planning.GroupsOf([]food.LeafGrams{
		{Name: "Hähnchenbrust", Grams: 120},
		{Name: "Lachsfilet", Grams: 100},
		{Name: "Kichererbsen", Grams: 80},
		{Name: "Brokkoli", Grams: 150},
		{Name: "Kartoffeln", Grams: 200},                     // potatoes are not vegetables for the DGE
		{Name: "Spezialkraut", Section: &produce, Grams: 10}, // override wins
		{Name: "Tomate", Section: &other, Grams: 50},         // override away from produce
		{Name: "Erdnüsse", Grams: 30},
	})
	assert.InDelta(t, 120, g.MeatG, 1e-9)
	assert.InDelta(t, 100, g.FishG, 1e-9)
	assert.InDelta(t, 80, g.LegumeG, 1e-9)
	assert.InDelta(t, 160, g.ProduceG, 1e-9)
}

func dgeCodes(t *testing.T, ev planning.Evaluation) []string {
	var out []string
	for _, is := range ev.Issues {
		switch is.Code {
		case planning.CodeWeekMeatAboveDGE, planning.CodeWeekFishBelowDGE, planning.CodeWeekLegumesBelow:
			assert.Equal(t, "S7", is.Rule)
			assert.Equal(t, planning.SeveritySoft, is.Severity)
			assert.Empty(t, is.Date)
			assert.Empty(t, is.Options)
			out = append(out, is.Code)
		}
	}
	return out
}

func TestEvaluate_DGEWeek(t *testing.T) {
	cases := []struct {
		name   string
		groups planning.FoodGroups // of the one recipe on every plate, per portion
		diet   profile.DietType
		skip   []string
		want   []string
		meatG  float64
		fish   int
		legume int
		prodG  float64
	}{
		{
			"nothing: no fish, no legumes",
			planning.FoodGroups{ProduceG: 100},
			profile.DietOmnivore, nil,
			[]string{planning.CodeWeekFishBelowDGE, planning.CodeWeekLegumesBelow},
			0, 0, 0, 300,
		},
		{
			"meat above 300 g a week",
			planning.FoodGroups{MeatG: 20, FishG: 50, LegumeG: 50},
			profile.DietOmnivore, nil,
			[]string{planning.CodeWeekMeatAboveDGE},
			420, 21, 21, 0,
		},
		{
			"meat at the limit is fine",
			planning.FoodGroups{MeatG: 300.0 / 21, FishG: 50, LegumeG: 50},
			profile.DietOmnivore, nil,
			nil, 300, 21, 21, 0,
		},
		{
			"vegetarian: no meat or fish rule, legumes still",
			planning.FoodGroups{},
			profile.DietVegetarian, nil,
			[]string{planning.CodeWeekLegumesBelow},
			0, 0, 0, 0,
		},
		{
			"vegan with legumes: nothing",
			planning.FoodGroups{LegumeG: 50},
			profile.DietVegan, nil,
			nil, 0, 0, 21, 0,
		},
		{
			"pescetarian: fish rule applies",
			planning.FoodGroups{LegumeG: 50},
			profile.DietPescetarian, nil,
			[]string{planning.CodeWeekFishBelowDGE},
			0, 0, 21, 0,
		},
		{
			"a dash of fish or legumes is no meal (under 50 g)",
			planning.FoodGroups{FishG: 49.9, LegumeG: 49.9},
			profile.DietOmnivore, nil,
			[]string{planning.CodeWeekFishBelowDGE, planning.CodeWeekLegumesBelow},
			0, 0, 0, 0,
		},
		{"too few days: no verdict", planning.FoodGroups{}, profile.DietOmnivore, []string{sat, sun, mon, tue, wed}, nil, 0, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := baseState()
			s.Profile.KcalTarget = f64(1500)
			s.Profile.DietType = tc.diet
			c := s.Foods[curry]
			c.Groups = tc.groups
			s.Foods[curry] = c
			ev, _ := weekOf(s, tc.skip...)
			require.NotNil(t, ev.Week)
			assert.ElementsMatch(t, tc.want, dgeCodes(t, ev))
			if tc.skip == nil {
				assert.InDelta(t, tc.meatG, ev.Week.MeatG, 1e-6)
				assert.Equal(t, tc.fish, ev.Week.FishMeals)
				assert.Equal(t, tc.legume, ev.Week.LegumeMeals)
				assert.InDelta(t, tc.prodG, ev.Week.ProduceGPerDay, 1e-6)
			}
		})
	}

	t.Run("meat is compared to a prorated limit, shown as the real sum", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(1500)
		c := s.Foods[curry]
		c.Groups = planning.FoodGroups{MeatG: 20, FishG: 50, LegumeG: 50}
		s.Foods[curry] = c
		// five complete days (15 plates, 20 g each = 300 g) against 300 x 5/7 = 214 g
		ev, _ := weekOf(s, sat, sun)
		require.NotNil(t, ev.Week)
		assert.Equal(t, 5, ev.Week.Days)
		assert.InDelta(t, 300, ev.Week.MeatG, 1e-6)
		assert.Equal(t, []string{planning.CodeWeekMeatAboveDGE}, dgeCodes(t, ev))
	})
	t.Run("meat under the prorated limit is fine", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(1500)
		c := s.Foods[curry]
		c.Groups = planning.FoodGroups{MeatG: 13, FishG: 50, LegumeG: 50}
		s.Foods[curry] = c
		// five days: 195 g < 214 g (extrapolated to a week it would be 273 g)
		ev, _ := weekOf(s, sat, sun)
		assert.Empty(t, dgeCodes(t, ev))
	})
	t.Run("four days are no verdict, even with no fish", func(t *testing.T) {
		s := baseState()
		s.Profile.KcalTarget = f64(1500)
		ev, _ := weekOf(s, sat, sun, mon)
		require.NotNil(t, ev.Week)
		assert.Equal(t, 4, ev.Week.Days)
		assert.Empty(t, dgeCodes(t, ev))
	})
}
