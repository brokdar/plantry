package planning_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

func ptrB(v bool) *bool { return &v }

const (
	sideRice  = int64(201) // a starch side
	sideSalad = int64(202) // a fresh side
	sideRoast = int64(203) // a cooked veg side, not fresh
)

// plateFitCooker is batchCooker with pasta mains (a starch, nothing fresh)
// and three sides the old keys rank rice, roast veg, salad (favourites and
// ids), all of the same kcal.
func plateFitCooker(on bool) planning.State {
	s := batchCooker()
	s.Settings.PlateFit = on
	for id, f := range s.Foods {
		if f.Role == food.RoleMain {
			f.Starch, f.Fresh = ptrB(true), ptrB(false)
			f.Attrs = flavor.Attributes{Colour: "red"}
			s.Foods[id] = f
		}
		if f.Role == food.RoleSideVeg {
			delete(s.Foods, id)
		}
	}
	rice := recipe(sideRice, "Basmatireis", food.RoleSideStarch, "vegan", 150)
	rice.Starch, rice.Fresh, rice.Favorite = ptrB(true), ptrB(false), true
	salad := recipe(sideSalad, "Gurkensalat", food.RoleSideVeg, "vegan", 150)
	salad.Starch, salad.Fresh = ptrB(false), ptrB(true)
	roast := recipe(sideRoast, "Ofengemüse", food.RoleSideVeg, "vegan", 150)
	roast.Starch, roast.Fresh, roast.Favorite = ptrB(false), ptrB(false), true
	s.Foods[sideRice], s.Foods[sideSalad], s.Foods[sideRoast] = rice, salad, roast
	return s
}

func firstSide(t *testing.T, s planning.State) int64 {
	t.Helper()
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	require.GreaterOrEqual(t, len(p.Sessions), 2)
	return p.Sessions[1].FoodID
}

// PLATE-5: a side that clashes (a second starch beside a pasta) is no
// candidate; PLATE-4: among the rest, the side that settles a raised gap
// (fresh) wins over the old keys' favourite.
func TestHeuristic_PlateFitPicksSides(t *testing.T) {
	assert.Equal(t, sideRice, firstSide(t, plateFitCooker(false)), "old keys: the favourite with the lowest id")
	assert.Equal(t, sideSalad, firstSide(t, plateFitCooker(true)), "no second starch; the fresh side settles the fresh gap")

	s := plateFitCooker(true)
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	for _, ps := range p.Sessions {
		assert.NotEqual(t, sideRice, ps.FoodID, "rice never beside a pasta main")
	}
}

// PLATE-9: with plan.plate_fit off the heuristic reads no plate-fit data, so
// its week is the old path's: the same plan with or without the data.
func TestHeuristic_PlateFitOffIsTheOldPath(t *testing.T) {
	with := plateFitCooker(false)
	without := plateFitCooker(false)
	for id, f := range without.Foods {
		f.Attrs, f.Fresh, f.Starch = flavor.Attributes{}, nil, nil
		without.Foods[id] = f
	}
	a := choose(t, planning.ChooseInput{State: with, Target: windowTarget(with)})
	b := choose(t, planning.ChooseInput{State: without, Target: windowTarget(without)})
	assert.Equal(t, b, a)
}

// The side swap judges the new side beside the plate's main as well.
func TestHeuristic_SideSwapAvoidsClash(t *testing.T) {
	s := plateFitCooker(true)
	p := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s)})
	m := planning.Merge(s, windowTarget(s), p)
	var pl planning.Plate
	for _, mp := range m.Plates {
		if len(mp.Components) == 2 && s.Foods[mp.Components[0].FoodID].Role == food.RoleMain {
			pl = mp
			break
		}
	}
	require.NotZero(t, pl.ID)
	old := pl.Components[1].FoodID
	s.Plates = m.Plates
	sw := planning.SideSwap{
		FoodID: old, Role: s.Foods[old].Role, CookDate: pl.Date, CookSlotID: pl.SlotID, Portions: 1,
		Plates: []planning.SidePlate{{PlateID: pl.ID, Date: pl.Date, SlotID: pl.SlotID, Eaters: 1, PerEater: 1}},
	}
	got := choose(t, planning.ChooseInput{State: s, Target: windowTarget(s), Side: &sw})
	require.Len(t, got.Sessions, 1)
	assert.NotEqual(t, sideRice, got.Sessions[0].FoodID)
	assert.NotEqual(t, old, got.Sessions[0].FoodID)
}

// A side with no known plate values settles no gap: an unattributed
// favourite never outranks the side that really closes the fresh gap.
func TestHeuristic_BlankSideNeverOutranksAFit(t *testing.T) {
	s := plateFitCooker(true)
	blank := recipe(200, "Beilage", food.RoleSideVeg, "vegan", 150)
	blank.Favorite = true
	s.Foods[200] = blank
	assert.Equal(t, sideSalad, firstSide(t, s))
}

// Nutrition before fit: beside the mains, the side whose fat stays in the
// week's range wins over the fresh side that settles the fresh gap.
func TestHeuristic_PlateFitSidePrefersTheLeanPlate(t *testing.T) {
	s := plateFitCooker(true)
	salad := s.Foods[sideSalad]
	salad.Macros.Fat = salad.Macros.Kcal / 9 // all fat kcal: a parmesan salad
	s.Foods[sideSalad] = salad
	for id, f := range s.Foods {
		if f.Role == food.RoleMain {
			f.Macros.Fat = f.Macros.Kcal * 0.35 / 9 // the plate sits at the range top
			s.Foods[id] = f
		}
	}
	assert.Equal(t, sideRoast, firstSide(t, s), "the lean side, not the fresh fatty one")
}
