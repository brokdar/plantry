package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

const (
	potatoes int64 = 40 // side_starch recipe
	broccoli int64 = 41 // side_veg recipe
	salad    int64 = 42 // leaf, side_veg by section, no-cook
	lasagne  int64 = 43 // heavy main
)

// Plate ids of altState.
const (
	altSatDinner int64 = 10 // chili, cooks session 1
	altSunLunch  int64 = 11 // chili leftover of session 1
	altSatLunch  int64 = 12 // curry (session 2, 2 portions) + rice + broccoli
)

// altState: Saturday dinner cooks 2 portions of chili, Sunday lunch eats the
// leftover; Saturday lunch cooks 2 portions of curry and eats one with rice
// and broccoli, so one curry portion is spare for Sunday.
func altState() planning.State {
	s := baseState()
	s.Settings.MaxCookOccasionsPerDay = 2
	s.SlotNames = map[int64]string{breakfast: "slot.breakfast", lunch: "slot.lunch", dinner: "slot.dinner"}
	s.Foods[potatoes] = recipe(potatoes, "Potatoes", food.RoleSideStarch, "vegan", 250)
	s.Foods[broccoli] = recipe(broccoli, "Broccoli", food.RoleSideVeg, "vegan", 80)
	s.Foods[lasagne] = recipe(lasagne, "Lasagne", food.RoleMain, "vegetarian", 950)
	s.Foods[salad] = planning.FoodFacts{
		ID: salad, Name: "Salad", Kind: food.KindLeaf, LeafRole: food.RoleSideVeg, DietClass: "vegan", NoCook: true,
		Macros: nutrition.Macros{Kcal: 20, Protein: 1}, NutritionComplete: true,
	}
	soupF := s.Foods[soup]
	soupF.Minutes = 20
	s.Foods[soup] = soupF
	curryF := s.Foods[curry]
	curryF.Minutes = 90
	s.Foods[curry] = curryF

	s.Sessions = []planning.Session{
		persistedSession(1, chili, sat, dinner, 2, false),
		persistedSession(2, curry, sat, lunch, 2, false),
		persistedSession(3, rice, sat, lunch, 1, false),
		persistedSession(4, broccoli, sat, lunch, 1, false),
	}
	main := func(p planning.Plate, id int64) planning.Plate {
		p.Components[0].ID, p.Components[0].Position = id, plate.PositionMain
		return p
	}
	lunchPlate := main(persisted(altSatLunch, sat, lunch, false, curry, 2), 120)
	lunchPlate.Components = append(lunchPlate.Components,
		planning.Component{ID: 121, FoodID: rice, Portions: f64(1), SessionID: 3, Position: plate.PositionSide},
		planning.Component{ID: 122, FoodID: broccoli, Portions: f64(1), SessionID: 4, Position: plate.PositionSide})
	s.Plates = []planning.Plate{
		main(persisted(altSatDinner, sat, dinner, false, chili, 1), 100),
		main(persisted(altSunLunch, sun, lunch, false, chili, 1), 110),
		lunchPlate,
	}
	return s
}

func alts(t *testing.T, s planning.State, req planning.AltRequest) []planning.Alternative {
	t.Helper()
	req.Limit = 10
	out, err := planning.Alternatives(s, windowTarget(s), req)
	require.NoError(t, err)
	return out
}

func altIDs(out []planning.Alternative) []int64 {
	ids := make([]int64, len(out))
	for i, a := range out {
		ids[i] = a.FoodID
	}
	return ids
}

func indexOf(out []planning.Alternative, foodID int64, source string) int {
	for i, a := range out {
		if a.FoodID == foodID && a.Source == source {
			return i
		}
	}
	return -1
}

func TestAlternatives(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*planning.State)
		req    planning.AltRequest
		assert func(t *testing.T, out []planning.Alternative)
	}{
		{
			name: "the replaced food is never returned and a leftover meal gets a different main",
			req:  planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain},
			assert: func(t *testing.T, out []planning.Alternative) {
				require.NotEmpty(t, out)
				assert.NotContains(t, altIDs(out), chili)
				for _, a := range out {
					assert.NotEqual(t, potatoes, a.FoodID, "a side is no main")
				}
			},
		},
		{
			name: "a session source that can feed the cell ranks before fresh",
			req:  planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain},
			assert: func(t *testing.T, out []planning.Alternative) {
				require.NotEmpty(t, out)
				first := out[0]
				assert.Equal(t, curry, first.FoodID)
				assert.Equal(t, planning.SourceSession, first.Source)
				require.NotNil(t, first.SessionID)
				assert.EqualValues(t, 2, *first.SessionID)
				assert.Zero(t, first.TotalMinutes)
				require.Len(t, first.Ops, 2)
				assert.Equal(t, "add", first.Ops[0].Op)
				assert.EqualValues(t, 2, *first.Ops[0].SessionID)
				assert.EqualValues(t, altSunLunch, *first.Ops[0].PlateID)
				assert.Equal(t, "remove", first.Ops[1].Op)
				assert.EqualValues(t, 110, *first.Ops[1].PlateComponentID)
				if i := indexOf(out, soup, planning.SourceFresh); assert.Positive(t, i) {
					fresh := out[i]
					assert.Equal(t, "swap", fresh.Ops[0].Op)
					assert.EqualValues(t, soup, fresh.Ops[0].FoodID)
					assert.Equal(t, 1, fresh.CookOccasionsDay)
				}
			},
		},
		{
			name: "a full batch is not offered, one with a free portion is",
			mutate: func(s *planning.State) {
				s.Sessions = append(s.Sessions, persistedSession(6, curry, sat, lunch, 1, false))
				p := persisted(14, sun, dinner, false, curry, 6)
				p.Components[0].Position = plate.PositionMain
				s.Plates = append(s.Plates, p)
			},
			req: planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain},
			assert: func(t *testing.T, out []planning.Alternative) {
				for _, a := range out {
					if a.SessionID != nil {
						assert.NotEqual(t, int64(6), *a.SessionID, "session 6 is eaten up")
					}
				}
				assert.GreaterOrEqual(t, indexOf(out, curry, planning.SourceSession), 0, "session 2 has a spare portion")
			},
		},
		{
			name: "max_minutes filters fresh cooks by minutes",
			req:  planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain, Constraints: planning.AltConstraints{MaxMinutes: 30}},
			assert: func(t *testing.T, out []planning.Alternative) {
				assert.GreaterOrEqual(t, indexOf(out, soup, planning.SourceFresh), 0)
				assert.Equal(t, -1, indexOf(out, curry, planning.SourceFresh))
				assert.Equal(t, -1, indexOf(out, lasagne, planning.SourceFresh), "no minutes known is not within 30")
			},
		},
		{
			name: "a diet constraint is respected",
			req:  planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain, Constraints: planning.AltConstraints{Diet: "vegan"}},
			assert: func(t *testing.T, out []planning.Alternative) {
				require.NotEmpty(t, out)
				for _, a := range out {
					assert.Equal(t, curry, a.FoodID)
				}
			},
		},
		{
			name: "a candidate that would add an H2 cook is dropped",
			mutate: func(s *planning.State) {
				s.Settings.MaxCookOccasionsPerDay = 1
				s.Sessions = append(s.Sessions, persistedSession(5, soup, sun, dinner, 1, false))
				p := persisted(13, sun, dinner, false, soup, 5)
				p.Components[0].Position = plate.PositionMain
				s.Plates = append(s.Plates, p)
			},
			req: planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain},
			assert: func(t *testing.T, out []planning.Alternative) {
				require.NotEmpty(t, out)
				for _, a := range out {
					assert.Equal(t, planning.SourceSession, a.Source, "%s would cook a second time on Sunday", a.Name)
				}
			},
		},
		{
			name: "a side with the role of the other side ranks last",
			req:  planning.AltRequest{PlateID: altSatLunch, What: planning.AltSide, ComponentID: 122},
			assert: func(t *testing.T, out []planning.Alternative) {
				require.GreaterOrEqual(t, len(out), 2)
				assert.Equal(t, potatoes, out[len(out)-1].FoodID)
				assert.Equal(t, salad, out[0].FoodID)
				assert.Equal(t, planning.SourceLeaf, out[0].Source)
				assert.Equal(t, "swap", out[0].Ops[0].Op)
				assert.Equal(t, "g", out[0].Ops[0].Unit)
				assert.NotContains(t, altIDs(out), rice, "foods on the plate are excluded")
			},
		},
		{
			name: "under a light intent a lighter main ranks first",
			mutate: func(s *planning.State) {
				s.Profile.KcalTarget = f64(2000)
				s.Light = []planning.LightIntent{{Date: sun, Scope: planning.LightScopeMeal, Slot: "lunch"}}
			},
			req: planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain, Constraints: planning.AltConstraints{Diet: "vegetarian"}},
			assert: func(t *testing.T, out []planning.Alternative) {
				soupAt, lasagneAt := indexOf(out, soup, planning.SourceFresh), indexOf(out, lasagne, planning.SourceFresh)
				require.GreaterOrEqual(t, soupAt, 0)
				require.GreaterOrEqual(t, lasagneAt, 0)
				assert.Less(t, soupAt, lasagneAt)
			},
		},
		{
			name: "meal swaps the main and removes the other components",
			req:  planning.AltRequest{PlateID: altSatLunch, What: planning.AltMeal},
			assert: func(t *testing.T, out []planning.Alternative) {
				i := indexOf(out, soup, planning.SourceFresh)
				require.GreaterOrEqual(t, i, 0)
				ops := out[i].Ops
				require.Len(t, ops, 3)
				assert.Equal(t, "swap", ops[0].Op)
				assert.EqualValues(t, 120, *ops[0].PlateComponentID)
				assert.Equal(t, "remove", ops[1].Op)
				assert.Equal(t, "remove", ops[2].Op)
			},
		},
		{
			name: "add offers no-cook and side foods not yet on the plate",
			req:  planning.AltRequest{PlateID: altSunLunch, What: planning.AltAdd},
			assert: func(t *testing.T, out []planning.Alternative) {
				assert.Contains(t, altIDs(out), salad)
				assert.NotContains(t, altIDs(out), curry, "a main is not an add")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := altState()
			if tt.mutate != nil {
				tt.mutate(&s)
			}
			tt.assert(t, alts(t, s, tt.req))
		})
	}
}

func TestAlternatives_Deterministic(t *testing.T) {
	s := altState()
	req := planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain}
	first := alts(t, s, req)
	for range 5 {
		assert.Equal(t, first, alts(t, altState(), req))
	}
}

func TestAlternatives_LimitAndErrors(t *testing.T) {
	s := altState()
	out, err := planning.Alternatives(s, windowTarget(s), planning.AltRequest{PlateID: altSunLunch, What: planning.AltMain, Limit: 1})
	require.NoError(t, err)
	assert.Len(t, out, 1)

	g := 100.0
	s.Plates[2].Components = append(s.Plates[2].Components, planning.Component{ID: 123, FoodID: salad, Grams: &g})
	cases := []struct {
		name string
		req  planning.AltRequest
		err  error
	}{
		{"unknown plate", planning.AltRequest{PlateID: 999, What: planning.AltMain}, domain.ErrNotFound},
		{"bad what", planning.AltRequest{PlateID: altSunLunch, What: "dessert"}, domain.ErrInvalidInput},
		{"two sides need a component", planning.AltRequest{PlateID: altSatLunch, What: planning.AltSide}, domain.ErrInvalidInput},
		{"a leaf is no main", planning.AltRequest{PlateID: altSatLunch, What: planning.AltMain, ComponentID: 123}, domain.ErrInvalidInput},
		{"unknown component", planning.AltRequest{PlateID: altSatLunch, What: planning.AltSide, ComponentID: 7}, domain.ErrNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := planning.Alternatives(s, windowTarget(s), c.req)
			assert.ErrorIs(t, err, c.err)
		})
	}
	s.Plates[0].Confirmed = true
	_, err = planning.Alternatives(s, windowTarget(s), planning.AltRequest{PlateID: altSatDinner, What: planning.AltMain})
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "a confirmed plate needs an unlock first")
}
