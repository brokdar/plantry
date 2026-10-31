package planning_test

import (
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// The window runs Sat 2026-11-07 … Fri 2026-11-13. Slots: breakfast (id 3,
// sort 0), lunch (id 1, sort 1), dinner (id 2, sort 2). Default keep days 3,
// one cooking occasion per day, variety 3 weeks, household portions 1.

const (
	sat = "2026-11-07"
	sun = "2026-11-08"
	mon = "2026-11-09"
	tue = "2026-11-10"
	wed = "2026-11-11"
	fri = "2026-11-13"
)

const (
	lunch     int64 = 1
	dinner    int64 = 2
	breakfast int64 = 3
)

// Foods.
const (
	chili int64 = 1 // main, omnivore
	curry int64 = 2 // main, vegan
	rice  int64 = 3 // side_starch, vegan
	soup  int64 = 4 // main, vegetarian
	oats  int64 = 5 // standalone, vegan (routine breakfast)
	stew  int64 = 6 // main, unknown diet class
	egg   int64 = 10
	bacon int64 = 11
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func f64(v float64) *float64 { return &v }
func intp(v int) *int        { return &v }

func cell(date string, slot int64) plate.CellKey { return plate.CellKey{Date: day(date), SlotID: slot} }

func recipe(id int64, name string, role food.Role, class string, kcal float64) planning.FoodFacts {
	return planning.FoodFacts{
		ID: id, Name: name, Kind: food.KindComposed, Role: role, DietClass: class,
		Macros: nutrition.Macros{Kcal: kcal, Protein: kcal / 20, Fat: kcal / 45, Carbs: kcal / 8}, NutritionComplete: true,
		ReferencePortions: 4,
	}
}

func baseState() planning.State {
	w := window.Window{Start: day(sat), End: day(fri)}
	s := planning.State{
		Window:    w,
		Today:     day("2026-11-06"),
		SlotOrder: plate.SlotOrder{breakfast: 0, lunch: 1, dinner: 2},
		Foods: map[int64]planning.FoodFacts{
			chili: recipe(chili, "Chili", food.RoleMain, "omnivore", 600),
			curry: recipe(curry, "Curry", food.RoleMain, "vegan", 500),
			rice:  recipe(rice, "Rice", food.RoleSideStarch, "vegan", 200),
			soup:  recipe(soup, "Soup", food.RoleMain, "vegetarian", 400),
			oats:  recipe(oats, "Oats", food.RoleStandalone, "vegan", 350),
			stew:  recipe(stew, "Stew", food.RoleMain, "unknown", 500),
			egg:   {ID: egg, Name: "Egg", Kind: food.KindLeaf, DietClass: "vegetarian", NutritionComplete: true},
			bacon: {ID: bacon, Name: "Bacon", Kind: food.KindLeaf, DietClass: "omnivore", NutritionComplete: true},
		},
		Settings: planning.Settings{
			BatchSettings:          plate.BatchSettings{DefaultKeepDays: 3, FreezerKeepDays: 90},
			MaxCookOccasionsPerDay: 1,
			VarietyWeeks:           3,
		},
		Profile: planning.Profile{DietType: profile.DietOmnivore, HouseholdPortions: 1},
	}
	for _, d := range w.Days() {
		for _, slot := range []int64{breakfast, lunch, dinner} {
			s.Cells = append(s.Cells, planning.Cell{Date: d, SlotID: slot, Mode: availability.ModePlan})
		}
	}
	return s
}

// setMode changes one cell's mode.
func setMode(s *planning.State, c plate.CellKey, m availability.Mode) {
	for i := range s.Cells {
		if s.Cells[i].Key().Equal(c) {
			s.Cells[i].Mode = m
		}
	}
}

// targetCells targets the given cells only.
func targetCells(cells ...plate.CellKey) planning.Target { return planning.Target{Cells: cells} }

// windowTarget targets every cell of s.
func windowTarget(s planning.State) planning.Target {
	t := planning.Target{}
	for _, c := range s.Cells {
		t.Cells = append(t.Cells, c.Key())
	}
	return t
}

func session(key string, foodID int64, date string, slot int64, portions float64) planning.ProposedSession {
	return planning.ProposedSession{Key: key, FoodID: foodID, CookDate: day(date), CookSlotID: slot, Portions: portions}
}

func onKey(foodID int64, key string) planning.ProposedComponent {
	return planning.ProposedComponent{FoodID: foodID, Portions: 1, SessionKey: key}
}

func onID(foodID int64, id int64) planning.ProposedComponent {
	return planning.ProposedComponent{FoodID: foodID, Portions: 1, SessionID: id}
}

func fresh(foodID int64) planning.ProposedComponent {
	return planning.ProposedComponent{FoodID: foodID, Portions: 1}
}

func pplate(date string, slot int64, comps ...planning.ProposedComponent) planning.ProposedPlate {
	return planning.ProposedPlate{Date: day(date), SlotID: slot, Components: comps}
}

// persisted is a persisted plate of one composed component eating from
// session sid.
func persisted(id int64, date string, slot int64, confirmed bool, foodID, sid int64) planning.Plate {
	return planning.Plate{
		ID: id, Date: day(date), SlotID: slot, Eaters: 1, Confirmed: confirmed,
		Components: []planning.Component{{FoodID: foodID, Portions: f64(1), SessionID: sid}},
	}
}

func persistedSession(id, foodID int64, date string, slot int64, portions float64, confirmed bool) planning.Session {
	return planning.Session{
		CookSession: plate.CookSession{ID: id, FoodID: foodID, CookDate: day(date), CookSlotID: slot, Portions: portions},
		Confirmed:   confirmed,
	}
}

// check merges p into s and checks the target.
func check(s planning.State, t planning.Target, p planning.Proposal) []planning.Issue {
	return planning.Check(planning.Merge(s, t, p), t)
}

// codes lists the codes of rule's issues.
func codes(issues []planning.Issue, rule string) []string {
	out := []string{}
	for _, is := range issues {
		if is.Rule == rule {
			out = append(out, is.Code)
		}
	}
	return out
}

func only(issues []planning.Issue, rule string) []planning.Issue {
	var out []planning.Issue
	for _, is := range issues {
		if is.Rule == rule {
			out = append(out, is)
		}
	}
	return out
}

type plateCellKey = plate.CellKey
