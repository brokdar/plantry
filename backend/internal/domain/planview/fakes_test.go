package planview

import (
	"context"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// store is an in-memory plan: plates, sessions and foods. Session links are
// derived from the plates' components, so the fake cannot disagree with
// itself.
type store struct {
	plates    []plate.Plate
	sessions  map[int64]plate.CookSession
	foods     map[int64]*food.Food
	nutrition map[int64]food.Detailed
	slots     []slot.TimeSlot
	overrides map[cellKey]availability.Mode
	recipes   int64
	runs      map[int64]RunNote

	recipeListCalls int

	plateCalls     [][2]time.Time
	sessionCalls   int
	nutritionCalls map[int64]int
	// readsOutsideTx counts reads whose ctx carries no fakeTx transaction.
	readsOutsideTx int
}

type txMarker struct{}

// fakeTx is a domain.Transactor that marks fn's ctx and counts calls.
type fakeTx struct{ calls int }

func (t *fakeTx) InTx(ctx context.Context, fn func(ctx context.Context) error) error {
	t.calls++
	return fn(context.WithValue(ctx, txMarker{}, true))
}

func (s *store) read(ctx context.Context) {
	if ctx.Value(txMarker{}) == nil {
		s.readsOutsideTx++
	}
}

func newStore() *store {
	return &store{
		sessions:       map[int64]plate.CookSession{},
		foods:          map[int64]*food.Food{},
		nutrition:      map[int64]food.Detailed{},
		overrides:      map[cellKey]availability.Mode{},
		nutritionCalls: map[int64]int{},
		runs:           map[int64]RunNote{},
	}
}

func inRange(d, from, to time.Time) bool { return !d.Before(from) && !d.After(to) }

func (s *store) ListByDateRange(ctx context.Context, from, to time.Time) ([]plate.Plate, error) {
	s.read(ctx)
	s.plateCalls = append(s.plateCalls, [2]time.Time{from, to})
	out := []plate.Plate{}
	for _, p := range s.plates {
		if inRange(p.Date, from, to) {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *store) ListInRange(ctx context.Context, from, to time.Time) ([]plate.CookSession, map[int64][]plate.Link, error) {
	s.read(ctx)
	s.sessionCalls++
	links := map[int64][]plate.Link{}
	touched := map[int64]bool{}
	for _, p := range s.plates {
		for _, pc := range p.Components {
			if pc.CookSessionID == nil {
				continue
			}
			sid := *pc.CookSessionID
			links[sid] = append(links[sid], plate.Link{
				PlateID: p.ID, ComponentID: pc.ID, FoodID: pc.FoodID, Cell: p.Cell(),
				PerEater: *pc.Portions, Eaters: p.Eaters,
			})
			if inRange(p.Date, from, to) {
				touched[sid] = true
			}
		}
	}
	out := []plate.CookSession{}
	for id, cs := range s.sessions {
		if touched[id] || inRange(cs.CookDate, from, to) {
			out = append(out, cs)
		}
	}
	return out, links, nil
}

func (s *store) LookupByIDs(ctx context.Context, ids []int64) (map[int64]*food.Food, error) {
	s.read(ctx)
	out := map[int64]*food.Food{}
	for _, id := range ids {
		if f, ok := s.foods[id]; ok {
			out[id] = f
		}
	}
	return out, nil
}

func (s *store) PerPortionDetailed(ctx context.Context, foodID int64) (food.Detailed, error) {
	s.read(ctx)
	s.nutritionCalls[foodID]++
	d, ok := s.nutrition[foodID]
	if !ok {
		return food.Detailed{}, fmt.Errorf("%w: food %d", domain.ErrNotFound, foodID)
	}
	return d, nil
}

func (s *store) Resolve(ctx context.Context, from, to time.Time) ([]availability.Cell, error) {
	s.read(ctx)
	out := []availability.Cell{}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		for _, sl := range s.slots {
			if !sl.Active {
				continue
			}
			mode, src := availability.ModePlan, availability.SourceFallback
			if m, ok := s.overrides[cellID(d, sl.ID)]; ok {
				mode, src = m, availability.SourceOverride
			}
			out = append(out, availability.Cell{Date: d, SlotID: sl.ID, Mode: mode, Source: src})
		}
	}
	return out, nil
}

func (s *store) List(ctx context.Context, activeOnly bool) ([]slot.TimeSlot, error) {
	s.read(ctx)
	out := []slot.TimeSlot{}
	for _, sl := range s.slots {
		if !activeOnly || sl.Active {
			out = append(out, sl)
		}
	}
	return out, nil
}

func (s *store) CountComposedFoods(ctx context.Context) (int64, error) {
	s.read(ctx)
	return s.recipes, nil
}

func (s *store) ComposedFoodIDs(ctx context.Context) ([]int64, error) {
	s.read(ctx)
	s.recipeListCalls++
	out := []int64{}
	for id, f := range s.foods {
		if f.Kind == food.KindComposed {
			out = append(out, id)
		}
	}
	return out, nil
}

// History is the newest confirmed plate on or before today per food, as the
// FoodHistory query reads it.
func (s *store) History(ctx context.Context, ids []int64, today time.Time) (map[int64]food.History, error) {
	s.read(ctx)
	want := map[int64]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := map[int64]food.History{}
	for _, p := range s.plates {
		if p.State != plate.StateConfirmed || p.Date.After(today) {
			continue
		}
		for _, pc := range p.Components {
			if !want[pc.FoodID] {
				continue
			}
			h := out[pc.FoodID]
			if h.LastEaten == nil || p.Date.After(*h.LastEaten) {
				d := p.Date
				h.LastEaten = &d
			}
			out[pc.FoodID] = h
		}
	}
	return out, nil
}

func (s *store) RunNote(ctx context.Context, id int64) (RunNote, error) {
	s.read(ctx)
	n, ok := s.runs[id]
	if !ok {
		return RunNote{}, fmt.Errorf("%w: run %d", domain.ErrNotFound, id)
	}
	return n, nil
}

// fakeDiets is the diet-class port over a fixed table.
type fakeDiets map[int64]string

func (f fakeDiets) DietClasses(_ context.Context, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	for _, id := range ids {
		if c, ok := f[id]; ok {
			out[id] = c
		} else {
			out[id] = "unknown"
		}
	}
	return out, nil
}

// fakeProgress reports one running generation per window start.
type fakeProgress map[string]int64

func (f fakeProgress) Running(start time.Time) (int64, bool) {
	id, ok := f[start.Format(time.DateOnly)]
	return id, ok
}

type fakeSettings map[string]string

func (f fakeSettings) Get(_ context.Context, key string) (settings.Value, error) {
	v, ok := f[key]
	if !ok {
		return settings.Value{}, fmt.Errorf("unknown setting %s", key)
	}
	return settings.Value{Key: key, Raw: v}, nil
}

type fakeProfile struct{ p profile.Profile }

func (f *fakeProfile) Get(context.Context) (*profile.Profile, error) {
	p := f.p
	return &p, nil
}

// ── Scenario: AC-1 Sunday batch ─────────────────────────────────────────

const (
	breakfast int64 = 1
	lunch     int64 = 2
	dinner    int64 = 3

	chiliID   int64 = 42
	omeletID  int64 = 43
	saladID   int64 = 44
	appleID   int64 = 5
	mysteryID int64 = 6

	chiliKcal = 500.0
)

func date(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr[T any](v T) *T { return &v }

// fixture is a store plus the settings and profile around it.
type fixture struct {
	*store
	settings fakeSettings
	profile  *fakeProfile
	tx       *fakeTx
	nextID   int64
	diets    DietClasses
	progress Progress
}

func newFixture() *fixture {
	s := newStore()
	s.slots = []slot.TimeSlot{
		{ID: breakfast, NameKey: "slot.breakfast", Icon: "coffee", SortOrder: 0, Active: true},
		{ID: lunch, NameKey: "slot.lunch", Icon: "sun", SortOrder: 1, Active: true},
		{ID: dinner, NameKey: "slot.dinner", Icon: "moon", SortOrder: 2, Active: true},
		{ID: 9, NameKey: "slot.snack", Icon: "cookie", SortOrder: 3, Active: false},
	}
	main, side := food.RoleMain, food.RoleSideVeg
	s.foods[chiliID] = &food.Food{ID: chiliID, Name: "Chili", Kind: food.KindComposed, Role: &main, PrepMinutes: ptr(15), CookMinutes: ptr(45)}
	s.foods[omeletID] = &food.Food{ID: omeletID, Name: "Omelette", Kind: food.KindComposed, Role: &main, PrepMinutes: ptr(5), CookMinutes: ptr(10)}
	s.foods[saladID] = &food.Food{ID: saladID, Name: "Salad", Kind: food.KindComposed, Role: &side}
	s.nutrition[chiliID] = food.Detailed{Macros: nutrition.Macros{Kcal: chiliKcal, Protein: 30, Fat: 20, Carbs: 50}, Complete: true}
	s.nutrition[omeletID] = food.Detailed{Macros: nutrition.Macros{Kcal: 300}, Complete: true}
	s.nutrition[saladID] = food.Detailed{Macros: nutrition.Macros{Kcal: 100}, Complete: true}
	s.nutrition[appleID] = food.Detailed{Macros: nutrition.Macros{Kcal: 52}, Complete: true}
	s.nutrition[mysteryID] = food.Detailed{
		Macros: nutrition.Macros{Kcal: 200}, Complete: false,
		Missing: []food.MissingLeaf{{ID: mysteryID, Name: "Mystery sauce"}},
	}
	s.recipes = 3
	return &fixture{
		store: s,
		settings: fakeSettings{
			settings.KeyPlanShoppingDay:            "5", // Saturday
			settings.KeyPlanAnchor:                 "next_shopping_day",
			settings.KeyPlanDefaultKeepDays:        "3",
			settings.KeyPlanFreezingEnabled:        "false",
			settings.KeyPlanFreezerKeepDays:        "90",
			settings.KeyPlanMaxCookOccasionsPerDay: "1",
			settings.KeyPlanVarietyWeeks:           "3",
		},
		profile: &fakeProfile{p: profile.Profile{HouseholdPortions: 2}},
		tx:      &fakeTx{},
		nextID:  100,
	}
}

func (f *fixture) id() int64 {
	f.nextID++
	return f.nextID
}

func (f *fixture) service() *Service {
	return NewService(Deps{
		Plates: f.store, Sessions: f.store, Foods: f.store, Nutrition: f.store,
		Availability: f.store, Slots: f.store, Settings: f.settings, Profile: f.profile, Recipes: f.store, Tx: f.tx,
		Runs: f.store, Diets: f.diets, Progress: f.progress,
	})
}

// session adds a session of foodID cooking on (day, slotID).
func (f *fixture) session(foodID int64, day string, slotID int64, portions float64) int64 {
	id := f.id()
	f.sessions[id] = plate.CookSession{ID: id, FoodID: foodID, CookDate: date(day), CookSlotID: slotID, Portions: portions, State: plate.StateConfirmed}
	return id
}

// draftSession adds a draft session written by run.
func (f *fixture) draftSession(foodID int64, day string, slotID int64, portions float64, run int64) int64 {
	id := f.session(foodID, day, slotID, portions)
	cs := f.sessions[id]
	cs.State, cs.AIRunID = plate.StateDraft, &run
	f.sessions[id] = cs
	return id
}

// draft marks plate id as a draft of run.
func (f *fixture) draft(id, run int64) {
	for i := range f.plates {
		if f.plates[i].ID == id {
			f.plates[i].State, f.plates[i].AIRunID = plate.StateDraft, &run
		}
	}
}

// composed adds a plate for 2 eaters with one portion per eater of foodID,
// eaten from session sid.
func (f *fixture) composed(day string, slotID, foodID, sid int64) int64 {
	fd := f.foods[foodID]
	role := (*string)(fd.Role)
	return f.plate(day, slotID, plate.PlateComponent{
		ID: f.id(), FoodID: foodID, Portions: ptr(1.0), CookSessionID: &sid,
		FoodName: fd.Name, FoodKind: string(food.KindComposed), FoodRole: role,
	})
}

// leaf adds a plate holding grams of leaf foodID.
func (f *fixture) leaf(day string, slotID, foodID int64, name string, grams float64) int64 {
	return f.plate(day, slotID, plate.PlateComponent{
		ID: f.id(), FoodID: foodID, Amount: &grams, Unit: ptr("g"), Grams: &grams,
		FoodName: name, FoodKind: string(food.KindLeaf),
	})
}

func (f *fixture) plate(day string, slotID int64, comps ...plate.PlateComponent) int64 {
	id := f.id()
	for i := range comps {
		comps[i].PlateID = id
		comps[i].SortOrder = i
	}
	f.plates = append(f.plates, plate.Plate{ID: id, Date: date(day), SlotID: slotID, Eaters: 2, State: plate.StateConfirmed, Components: comps})
	return id
}

// ac1 is AC-1: Chili cooked Sun 2026-11-01 dinner for 3 meals of 2 eaters,
// eaten Sun dinner, Mon lunch and Mon dinner. It returns the session id.
func (f *fixture) ac1() int64 {
	sid := f.session(chiliID, "2026-11-01", dinner, 6)
	f.composed("2026-11-01", dinner, chiliID, sid)
	f.composed("2026-11-02", lunch, chiliID, sid)
	f.composed("2026-11-02", dinner, chiliID, sid)
	return sid
}

func (f *fixture) setFeedback(plateID int64) {
	for i := range f.plates {
		if f.plates[i].ID == plateID {
			f.plates[i].Feedback = &plate.Feedback{Status: "loved"}
		}
	}
}
