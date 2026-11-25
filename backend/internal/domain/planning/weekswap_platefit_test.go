package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// meatyPlateWeek is meatyWeek with plate fit on: the meat mains (and the
// fish) carry no starch, the meat-free mains are pasta (a starch), and the
// sides are plateFitCooker's rice, salad and roast veg. meatAll gives every
// main meat, so the picks overshoot the budget.
func meatyPlateWeek(meatAll bool) planning.State {
	s := meatyWeek()
	s.Settings.PlateFit = true
	for id, f := range s.Foods {
		switch f.Role {
		case food.RoleMain:
			pasta := f.Groups.MeatG == 0 && f.Groups.FishG == 0
			f.Starch, f.Fresh = ptrB(pasta), ptrB(false)
			f.Attrs = flavor.Attributes{Colour: "red"}
			if meatAll && pasta {
				f.Groups.MeatG = 150
			}
			s.Foods[id] = f
		case food.RoleSideVeg:
			delete(s.Foods, id)
		}
	}
	sides := plateFitCooker(true)
	for _, id := range []int64{sideRice, sideSalad, sideRoast} {
		s.Foods[id] = sides.Foods[id]
	}
	return s
}

// starchTwice lists the plates of p that hold a starch main beside the rice.
func starchTwice(s planning.State, p planning.Proposal) []string {
	var out []string
	for _, pl := range planning.Merge(s, windowTarget(s), p).Plates {
		starch, rice := false, false
		for _, c := range pl.Components {
			f := s.Foods[c.FoodID]
			starch = starch || (f.Role == food.RoleMain && f.Starch != nil && *f.Starch)
			rice = rice || c.FoodID == sideRice
		}
		if starch && rice {
			out = append(out, pl.Date.Format("Mon")+" "+s.Foods[pl.Components[0].FoodID].Name)
		}
	}
	return out
}

// PLATE-5 in the week guard: a meat main swapped for a pasta gets its side
// re-picked, so the rice that suited the meat never stays beside the pasta.
func TestGuardWeek_RepicksTheSwappedMainsSide(t *testing.T) {
	over := meatyPlateWeek(true)
	prop := choose(t, planning.ChooseInput{State: over, Target: windowTarget(over)})
	require.Greater(t, meatOf(over, prop), 300.0, "the picks overshoot")
	require.Contains(t, sessionFoods(prop), sideRice, "rice suits the meat mains")

	s := meatyPlateWeek(false)
	in := planning.ChooseInput{State: s, Target: windowTarget(s)}
	got, notes := planning.GuardWeekRepick(in, prop)
	require.NotEmpty(t, notes, "the guard swapped mains")
	assert.Less(t, meatOf(s, got), meatOf(s, prop))
	assert.Empty(t, starchTwice(s, got), "no pasta beside rice after the swaps")
}

// A light meal keeps its side: the guard's picker re-picks only full meals.
func TestSidePickerFor_LightMealKeepsItsSide(t *testing.T) {
	s := meatyPlateWeek(false)
	s.SlotNames = map[int64]string{lunch: "slot.lunch", dinner: "slot.dinner", breakfast: "slot.breakfast"}
	s.Light = []planning.LightIntent{{Date: mon, Scope: planning.LightScopeMeal, Slot: "dinner"}}
	in := planning.ChooseInput{State: s, Target: windowTarget(s)}
	pick := planning.SidePickerFor(in, planning.Proposal{})
	require.NotNil(t, pick)

	side, ok := pick(cell(mon, lunch), 108, nil)
	assert.True(t, ok, "a full meal gets a side")
	assert.NotEqual(t, sideRice, side, "never a second starch beside a pasta")
	_, ok = pick(cell(mon, dinner), 108, nil)
	assert.False(t, ok, "a light meal keeps its side")

	s.Settings.PlateFit = false
	assert.Nil(t, planning.SidePickerFor(planning.ChooseInput{State: s, Target: windowTarget(s)}, planning.Proposal{}), "plate fit off: no re-pick")
}

func sessionFoods(p planning.Proposal) []int64 {
	var out []int64
	for _, ss := range p.Sessions {
		out = append(out, ss.FoodID)
	}
	return out
}
