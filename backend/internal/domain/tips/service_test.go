package tips_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/tips"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// memStore is an in-memory tips.Store.
type memStore struct {
	mu        sync.Mutex
	tips      []tips.Tip
	dismissed []tips.Dismissal
}

func (m *memStore) Create(_ context.Context, t *tips.Tip) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t.ID = int64(len(m.tips) + 1)
	m.tips = append(m.tips, *t)
	return nil
}

func (m *memStore) Get(_ context.Context, id int64) (tips.Tip, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tips {
		if t.ID == id {
			return t, nil
		}
	}
	return tips.Tip{}, domain.ErrNotFound
}

func (m *memStore) Window(_ context.Context, start time.Time) ([]tips.Tip, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []tips.Tip
	for _, t := range m.tips {
		if t.WindowStart.Equal(start) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *memStore) SetState(_ context.Context, id int64, st tips.State, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.tips {
		if m.tips[i].ID == id {
			m.tips[i].State, m.tips[i].UpdatedAt = st, at
			if st == tips.StateDismissed {
				m.dismissed = append(m.dismissed, tips.Dismissal{Kind: m.tips[i].Kind, At: at})
			}
		}
	}
	return nil
}

func (m *memStore) DismissedSince(_ context.Context, since time.Time) ([]tips.Dismissal, error) {
	var out []tips.Dismissal
	for _, d := range m.dismissed {
		if !d.At.Before(since) {
			out = append(out, d)
		}
	}
	return out, nil
}

func (m *memStore) CountByState(context.Context, time.Time, time.Time) (map[tips.State]int, error) {
	out := map[tips.State]int{}
	for _, t := range m.tips {
		out[t.State]++
	}
	return out, nil
}

// fakePlates records the Apply calls.
type fakePlates struct {
	mu      sync.Mutex // Recorded serializes like a transaction
	added   []plate.PlateComponent
	swapped [][2]int64
}

func (f *fakePlates) Get(_ context.Context, id int64) (*plate.Plate, error) {
	return &plate.Plate{ID: id, State: plate.StateConfirmed, Components: []plate.PlateComponent{{ID: 11}, {ID: 12}}}, nil
}

func (f *fakePlates) Recorded(ctx context.Context, fn func(ctx context.Context) error) (plate.Undo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := fn(ctx); err != nil {
		return plate.Undo{}, err
	}
	return plate.Undo{FirstEventID: 7, LastEventID: 8}, nil
}

func (f *fakePlates) AddComponent(_ context.Context, _ int64, pc plate.PlateComponent) (*plate.Plate, error) {
	f.added = append(f.added, pc)
	return &plate.Plate{}, nil
}

func (f *fakePlates) SwapComponent(_ context.Context, componentID, newFoodID int64, _ *plate.PlateComponent) (*plate.Plate, error) {
	f.swapped = append(f.swapped, [2]int64{componentID, newFoodID})
	return &plate.Plate{}, nil
}

var start = time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC) // a Saturday

const (
	lemonID = 900
	zesteID = 901 // a second acid, for the plates the lemon's repeat cap leaves
)

// fixture is a window of four dinner plates, each a different rich main
// (taste balance raised) with no side, and a lemon garnish that settles it.
type fixture struct {
	st     planning.State
	store  *memStore
	plates *fakePlates
	on     bool
	never  map[int64]bool
}

func newFixture() *fixture {
	rich := func(id int64, name string) planning.FoodFacts {
		return planning.FoodFacts{
			ID: id, Name: name, Kind: food.KindComposed, Role: food.RoleMain, DietClass: diet.Vegetarian,
			Macros: nutrition.Macros{Kcal: 600}, NutritionComplete: true, Minutes: 20,
			Attrs: flavor.Attributes{
				Cuisine: "italian", Texture: "creamy", Temperature: "hot", Colour: "beige",
				Richness: ptr(3), Saucy: ptr(false), Taste: &flavor.Taste{Salt: 2, Umami: 2},
			},
		}
	}
	f := &fixture{store: &memStore{}, plates: &fakePlates{}, on: true, never: map[int64]bool{}}
	f.st = planning.State{
		Window: window.Window{Start: start, End: start.AddDate(0, 0, 6)}, Today: start,
		SlotOrder: plate.SlotOrder{1: 1},
		Foods: map[int64]planning.FoodFacts{
			lemonID: {
				ID: lemonID, Name: "Zitronenspalten", Kind: food.KindLeaf, DietClass: diet.Vegan,
				Macros: nutrition.Macros{Kcal: 30}, NutritionComplete: true, NoCook: true,
				Attrs: flavor.Attributes{Texture: "soft", Colour: "beige", Taste: &flavor.Taste{Acid: 3}},
			},
			zesteID: {
				ID: zesteID, Name: "Zitronenzeste", Kind: food.KindLeaf, DietClass: diet.Vegan,
				Macros: nutrition.Macros{Kcal: 30}, NutritionComplete: true, NoCook: true,
				Attrs: flavor.Attributes{Texture: "soft", Colour: "beige", Taste: &flavor.Taste{Acid: 3}},
			},
		},
		NoCook:  []int64{lemonID, zesteID},
		Profile: planning.Profile{DietType: profile.DietVegetarian, HouseholdPortions: 1},
	}
	for i := range 4 {
		id := int64(100 + i)
		f.st.Foods[id] = rich(id, "Pasta "+string(rune('A'+i)))
		f.st.Plates = append(f.st.Plates, planning.Plate{
			ID: int64(i + 1), Date: start.AddDate(0, 0, i), SlotID: 1, Eaters: 1,
			Components: []planning.Component{{ID: int64(10 + i), FoodID: id, Portions: ptr(1.0), SessionID: int64(50 + i), Position: plate.PositionMain}},
		})
	}
	return f
}

func (f *fixture) svc() *tips.Service {
	return tips.NewService(tips.Deps{
		Store: f.store, Plates: f.plates,
		State:       func(context.Context, window.Window, time.Time) (planning.State, error) { return f.st, nil },
		ChefOn:      func(context.Context) (bool, error) { return f.on, nil },
		ShoppingDay: func(context.Context) (int, error) { return 5, nil }, // Saturday
		Never:       func(context.Context) (map[int64]bool, error) { return f.never, nil },
		Equipment:   func(context.Context) ([]string, error) { return []string{"oven", "stovetop"}, nil },
		Now:         func() time.Time { return start.Add(9 * time.Hour) },
	})
}

func plateIDs(ts []tips.Tip) []int64 {
	var out []int64
	for _, t := range ts {
		out = append(out, *t.PlateID)
	}
	return out
}

func TestSync_LimitsPerWindow(t *testing.T) {
	t.Parallel()
	f := newFixture()
	v, err := f.svc().Sync(context.Background(), start.AddDate(0, 0, 2))
	if err != nil {
		t.Fatal(err)
	}
	// TIP-6: at most three, the earliest plates first; PRO-4: within the
	// noise budget.
	if got := plateIDs(v.Tips); !slices.Equal(got, []int64{1, 2, 3}) {
		t.Fatalf("tips on plates %v, want [1 2 3]", got)
	}
	if len(v.Tips) > tips.NoiseBudget {
		t.Fatalf("%d unasked lines, budget %d", len(v.Tips), tips.NoiseBudget)
	}
	tp := v.Tips[0]
	if tp.Kind != tips.KindAcid || tp.Op != (tips.Op{Action: "add", FoodID: lemonID, Grams: 10}) || tp.FoodName != "Zitronenspalten" {
		t.Fatalf("tip = %+v", tp)
	}
	if len(v.Fit) != 4 || !slices.Contains(v.Fit[0].Raised, "taste_balance") {
		t.Fatalf("fit = %+v", v.Fit)
	}
	// A second read keeps the same tips and writes no new ones.
	again, err := f.svc().Sync(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.store.tips) != 3 || !slices.Equal(plateIDs(again.Tips), []int64{1, 2, 3}) {
		t.Fatalf("second sync: stored %d, tips %v", len(f.store.tips), plateIDs(again.Tips))
	}
}

func TestSync_OnePerBatch(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.st.Plates[1].Components[0].SessionID = f.st.Plates[0].Components[0].SessionID
	v, err := f.svc().Sync(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	if got := plateIDs(v.Tips); !slices.Equal(got, []int64{1, 3, 4}) {
		t.Fatalf("tips on plates %v, want [1 3 4]", got)
	}
}

func TestSync_ChefOffComputesNothing(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.on = false
	v, err := f.svc().Sync(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	if v.On || len(v.Tips) != 0 || len(f.store.tips) != 0 {
		t.Fatalf("chef off: view %+v, stored %d", v, len(f.store.tips))
	}
}

func TestSync_ChecksBeforeShowing(t *testing.T) {
	t.Parallel()
	t.Run("never item", func(t *testing.T) {
		t.Parallel()
		f := newFixture()
		f.never[lemonID], f.never[zesteID] = true, true
		v, _ := f.svc().Sync(context.Background(), start)
		if len(v.Tips) != 0 {
			t.Fatalf("never item tipped: %+v", v.Tips)
		}
	})
	t.Run("diet", func(t *testing.T) {
		t.Parallel()
		f := newFixture()
		for _, id := range []int64{lemonID, zesteID} {
			l := f.st.Foods[id]
			l.DietClass = diet.Omnivore
			f.st.Foods[id] = l
		}
		v, _ := f.svc().Sync(context.Background(), start)
		if len(v.Tips) != 0 {
			t.Fatalf("diet violation tipped: %+v", v.Tips)
		}
	})
	t.Run("past plates", func(t *testing.T) {
		t.Parallel()
		f := newFixture()
		svc := tips.NewService(tips.Deps{
			Store: f.store, Plates: f.plates,
			State:       func(context.Context, window.Window, time.Time) (planning.State, error) { return f.st, nil },
			ChefOn:      func(context.Context) (bool, error) { return true, nil },
			ShoppingDay: func(context.Context) (int, error) { return 5, nil },
			Never:       func(context.Context) (map[int64]bool, error) { return nil, nil },
			Equipment:   func(context.Context) ([]string, error) { return nil, nil },
			Now:         func() time.Time { return start.AddDate(0, 0, 2) },
		})
		v, _ := svc.Sync(context.Background(), start)
		if got := plateIDs(v.Tips); !slices.Equal(got, []int64{3, 4}) {
			t.Fatalf("tips on %v, want [3 4]", got)
		}
	})
}

func TestDismiss_CoolsDownAndSilences(t *testing.T) {
	t.Parallel()
	f := newFixture()
	ctx := context.Background()
	svc := f.svc()
	v, _ := svc.Sync(ctx, start)
	if err := svc.Dismiss(ctx, v.Tips[0].ID); err != nil {
		t.Fatal(err)
	}
	// The dismissed recipe cools down for the window; plate 4 takes its
	// place.
	v, _ = svc.Sync(ctx, start)
	if got := plateIDs(v.Tips); !slices.Equal(got, []int64{2, 3, 4}) {
		t.Fatalf("after dismiss: tips on %v, want [2 3 4]", got)
	}
	if err := svc.Dismiss(ctx, v.Tips[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Dismiss(ctx, v.Tips[1].ID); err != nil {
		t.Fatal(err)
	}
	// Three dismissals of one kind silence it.
	if err := svc.Dismiss(ctx, v.Tips[0].ID); err == nil {
		t.Fatal("dismissing a dismissed tip should fail")
	}
	v, _ = svc.Sync(ctx, start)
	if got := plateIDs(v.Tips); !slices.Equal(got, []int64{4}) {
		t.Fatalf("after three dismissals: tips on %v, want only the live [4]", got)
	}
	next := newFixture()
	next.store = f.store
	for i := range next.st.Plates {
		next.st.Plates[i].ID += 10
	}
	v, _ = next.svc().Sync(ctx, start)
	if len(v.Tips) != 0 {
		t.Fatalf("silenced kind: %d tips on new plates", len(v.Tips))
	}
}

// TestApply_UndoShowsTipAgain applies a tip on a confirmed plate (the fake's
// plates are confirmed): Apply runs as the user's own edit, and the unlock
// rules gate only the agent's writes (plate guard), so it goes through
// (TIP-7).
func TestApply_UndoShowsTipAgain(t *testing.T) {
	t.Parallel()
	f := newFixture()
	ctx := context.Background()
	svc := f.svc()
	v, _ := svc.Sync(ctx, start)
	undo, err := svc.Apply(ctx, v.Tips[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if undo.FirstEventID != 7 || len(f.plates.added) != 1 || *f.plates.added[0].Amount != 10 || *f.plates.added[0].Unit != "g" {
		t.Fatalf("apply: undo %+v, added %+v", undo, f.plates.added)
	}
	if _, err := svc.Apply(ctx, v.Tips[0].ID); err == nil {
		t.Fatal("applying twice should fail")
	}
	// The garnish is on the plate: the tip stays applied and live.
	f.st.Plates[0].Components = append(f.st.Plates[0].Components, planning.Component{ID: 99, FoodID: lemonID, Grams: ptr(10.0)})
	v, _ = svc.Sync(ctx, start)
	if v.Tips[0].State != tips.StateApplied {
		t.Fatalf("tip after apply: %+v", v.Tips[0])
	}
	// Undo took it off again: the tip is shown again.
	f.st.Plates[0].Components = f.st.Plates[0].Components[:1]
	v, _ = svc.Sync(ctx, start)
	if v.Tips[0].State != tips.StateShown {
		t.Fatalf("tip after undo: %+v", v.Tips[0])
	}
}

func TestSync_ConcurrentCallsTipEachPlateOnce(t *testing.T) {
	t.Parallel()
	f := newFixture()
	svc := f.svc()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := svc.Sync(context.Background(), start); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	seen := map[int64]int{}
	for _, tp := range f.store.tips {
		seen[*tp.PlateID]++
	}
	if len(f.store.tips) != tips.MaxPerWindow || len(seen) != tips.MaxPerWindow {
		t.Fatalf("%d tips on plates %v, want one each on %d plates", len(f.store.tips), seen, tips.MaxPerWindow)
	}
}

func TestApply_ConcurrentDoubleApplyAddsOnce(t *testing.T) {
	t.Parallel()
	f := newFixture()
	ctx := context.Background()
	svc := f.svc()
	v, _ := svc.Sync(ctx, start)
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for range 2 {
		wg.Go(func() {
			if _, err := svc.Apply(ctx, v.Tips[0].ID); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			} else if !errors.Is(err, tips.ErrStale) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if ok != 1 || len(f.plates.added) != 1 {
		t.Fatalf("%d applies succeeded, garnish added %d times, want 1 and 1", ok, len(f.plates.added))
	}
}

func TestSync_WithdrawsTipThePlateNoLongerNeeds(t *testing.T) {
	t.Parallel()
	f := newFixture()
	ctx := context.Background()
	v, _ := f.svc().Sync(ctx, start)
	id := v.Tips[0].ID
	// The user added the lemon by hand: the tip is withdrawn.
	f.st.Plates[0].Components = append(f.st.Plates[0].Components, planning.Component{ID: 99, FoodID: lemonID, Grams: ptr(10.0)})
	v, _ = f.svc().Sync(ctx, start)
	if slices.ContainsFunc(v.Tips, func(t tips.Tip) bool { return t.ID == id }) {
		t.Fatalf("tip %d still live", id)
	}
	if !slices.ContainsFunc(v.Ended, func(t tips.Tip) bool { return t.ID == id && t.State == tips.StateWithdrawn }) {
		t.Fatalf("tip %d not withdrawn: %+v", id, v.Ended)
	}
}

// TestSync_OnlyMainMeals: plate fit's dimensions are dinner rules, so a light
// slot, a breakfast or snack slot and a plate without a main get no tip
// (CP1: 22 of 28 tips were on breakfast oats).
func TestSync_OnlyMainMeals(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.st.SlotOrder = plate.SlotOrder{1: 1, 2: 0, 3: 2}
	f.st.SlotKind = map[int64]slot.Kind{2: slot.KindLight}
	f.st.SlotNames = map[int64]string{1: "slot.breakfast", 2: "slot.coffee", 3: "slot.dinner"}
	oats := f.st.Foods[103]
	oats.Role = food.RoleStandalone
	f.st.Foods[103] = oats
	f.st.Plates[0].SlotID = 1 // breakfast, a meal-kind slot
	f.st.Plates[1].SlotID = 2 // light
	f.st.Plates[2].SlotID = 3 // dinner with a main
	f.st.Plates[3].SlotID = 3 // dinner without a main
	v, err := f.svc().Sync(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	if got := plateIDs(v.Tips); !slices.Equal(got, []int64{3}) {
		t.Fatalf("tips on plates %v, want only the dinner main [3]", got)
	}
	// A shown tip whose plate stops being a main meal is withdrawn.
	f.st.SlotKind[3] = slot.KindLight
	v, _ = f.svc().Sync(context.Background(), start)
	if len(v.Tips) != 0 || len(v.Ended) != 1 || v.Ended[0].State != tips.StateWithdrawn {
		t.Fatalf("after the slot turned light: tips %+v, ended %+v", v.Tips, v.Ended)
	}
}

// TestSync_RanksByLift: when more plates qualify than the window allows, a
// dinner wins over a lunch and the nearest plate over a later one, whatever
// the date order.
func TestSync_RanksByLift(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.st.SlotOrder = plate.SlotOrder{1: 2, 2: 1}
	f.st.SlotNames = map[int64]string{1: "slot.dinner", 2: "slot.lunch"}
	f.st.Plates[0].SlotID = 2
	f.st.Plates[1].SlotID = 2
	v, err := f.svc().Sync(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	if got := plateIDs(v.Tips); !slices.Equal(got, []int64{3, 4, 1}) {
		t.Fatalf("tips on plates %v, want the dinners [3 4] then the nearest lunch [1]", got)
	}
}

// TestSync_ClashBeforeGap: a plate with a clash (two starches) takes a tip
// before earlier plates that only miss something.
func TestSync_ClashBeforeGap(t *testing.T) {
	t.Parallel()
	f := newFixture()
	side := func(id int64, name string, role food.Role, starch bool, temp, colour, texture string, taste flavor.Taste) planning.FoodFacts {
		return planning.FoodFacts{
			ID: id, Name: name, Kind: food.KindComposed, Role: role, DietClass: diet.Vegan,
			Macros: nutrition.Macros{Kcal: 150}, NutritionComplete: true, Minutes: 10, Starch: ptr(starch), Fresh: ptr(false),
			Attrs: flavor.Attributes{Cuisine: "italian", Texture: texture, Temperature: temp, Colour: colour, Richness: ptr(0), Saucy: ptr(false), Taste: &taste},
		}
	}
	f.st.Foods[200] = side(200, "Polenta", food.RoleSideStarch, true, "hot", "beige", "soft", flavor.Taste{})
	f.st.Foods[201] = side(201, "Ciabatta", food.RoleSideStarch, true, "hot", "beige", "crisp", flavor.Taste{})
	f.st.Foods[202] = side(202, "Insalata mista", food.RoleSideVeg, false, "cold", "green", "crisp", flavor.Taste{Acid: 2})
	for id := range f.st.Foods {
		if fd := f.st.Foods[id]; fd.Role == food.RoleMain {
			fd.Starch = ptr(false)
			f.st.Foods[id] = fd
		}
	}
	f.st.Plates[3].Components = append(f.st.Plates[3].Components,
		planning.Component{ID: 30, FoodID: 200, Portions: ptr(1.0), Position: plate.PositionSide},
		planning.Component{ID: 31, FoodID: 201, Portions: ptr(1.0), Position: plate.PositionSide})
	v, err := f.svc().Sync(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	if got := plateIDs(v.Tips); !slices.Equal(got, []int64{4, 1, 2}) {
		t.Fatalf("tips on plates %v, want the clash plate [4] first, then [1 2]", got)
	}
	if op := v.Tips[0].Op; op.Action != "swap" || op.FoodID != 202 {
		t.Fatalf("clash tip %+v, want the salad swapped in", v.Tips[0])
	}
	// A side the household would rather not get is never swapped in.
	f2 := newFixture()
	f2.st.Foods, f2.st.Plates = f.st.Foods, f.st.Plates
	f2.st.RatherNot = map[int64]bool{202: true}
	v, err = f2.svc().Sync(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	for _, tp := range v.Tips {
		if tp.Op.FoodID == 202 {
			t.Fatalf("rather-not salad suggested: %+v", tp)
		}
	}
}

// TestSync_HerbGarnishFromGarnishList: a fresh herb that is not a no-cook
// food (State.Garnish) is offered as fresh herbs, 4 g per eater.
func TestSync_HerbGarnishFromGarnishList(t *testing.T) {
	t.Parallel()
	const parsleyID = 901
	f := newFixture()
	f.st.NoCook = nil
	f.st.Garnish = []int64{parsleyID}
	f.st.Foods[parsleyID] = planning.FoodFacts{
		ID: parsleyID, Name: "Petersilie", Kind: food.KindLeaf, DietClass: diet.Vegan,
		Macros: nutrition.Macros{Kcal: 36}, NutritionComplete: true,
		Fresh: ptr(true), Attrs: flavor.Attributes{Texture: "soft", Colour: "green", Taste: &flavor.Taste{}},
	}
	for id, m := range f.st.Foods {
		if id != parsleyID {
			m.Fresh, m.Attrs.Colour, m.Attrs.Texture, m.Attrs.Taste = ptr(false), "red", "crunchy", &flavor.Taste{Salt: 2, Umami: 2, Acid: 1}
			f.st.Foods[id] = m
		}
	}
	v, err := f.svc().Sync(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Tips) == 0 || v.Tips[0].Dimension != "fresh" || v.Tips[0].Kind != tips.KindHerbs || v.Tips[0].Op != (tips.Op{Action: "add", FoodID: parsleyID, Grams: 4}) {
		t.Fatalf("tips = %+v", v.Tips)
	}
}

// TestSync_SideSwapRespectsWeekRules: a swap that would push the week's fat
// share over its range is not offered; the same swap with a lean side is.
func TestSync_SideSwapRespectsWeekRules(t *testing.T) {
	t.Parallel()
	run := func(t *testing.T, saladFatG float64) *tips.Tip {
		f := newFixture()
		kcal := 600.0
		f.st.Profile.KcalTarget = &kcal
		for _, p := range f.st.Plates {
			f.st.Cells = append(f.st.Cells, planning.Cell{Date: p.Date, SlotID: p.SlotID})
		}
		side := func(id int64, name string, role food.Role, starch bool, temp, colour, texture string, fat float64, taste flavor.Taste) planning.FoodFacts {
			return planning.FoodFacts{
				ID: id, Name: name, Kind: food.KindComposed, Role: role, DietClass: diet.Vegan,
				Macros: nutrition.Macros{Kcal: 150, Fat: fat}, NutritionComplete: true, Minutes: 10, Starch: ptr(starch), Fresh: ptr(false),
				Attrs: flavor.Attributes{Cuisine: "italian", Texture: texture, Temperature: temp, Colour: colour, Richness: ptr(0), Saucy: ptr(false), Taste: &taste},
			}
		}
		f.st.Foods[200] = side(200, "Polenta", food.RoleSideStarch, true, "hot", "beige", "soft", 0, flavor.Taste{})
		f.st.Foods[201] = side(201, "Ciabatta", food.RoleSideStarch, true, "hot", "beige", "crisp", 0, flavor.Taste{})
		f.st.Foods[202] = side(202, "Insalata mista", food.RoleSideVeg, false, "cold", "green", "crisp", saladFatG, flavor.Taste{Acid: 2})
		for id, fd := range f.st.Foods {
			if fd.Role == food.RoleMain {
				fd.Starch, fd.Macros.Fat = ptr(false), 23 // 207 of 600 kcal: week fat share 31% before the swap
				f.st.Foods[id] = fd
			}
		}
		f.st.Plates[3].Components = append(f.st.Plates[3].Components,
			planning.Component{ID: 30, FoodID: 200, Portions: ptr(1.0), Position: plate.PositionSide},
			planning.Component{ID: 31, FoodID: 201, Portions: ptr(1.0), Position: plate.PositionSide})
		v, err := f.svc().Sync(context.Background(), start)
		if err != nil {
			t.Fatal(err)
		}
		for _, tp := range v.Tips {
			if tp.Kind == tips.KindSideSwap && *tp.PlateID == 4 {
				return &tp
			}
		}
		return nil
	}
	t.Run("lean side offered", func(t *testing.T) {
		t.Parallel()
		if run(t, 0) == nil {
			t.Fatal("lean swap not offered")
		}
	})
	t.Run("fat side over range not offered", func(t *testing.T) {
		t.Parallel()
		if tp := run(t, 16.7); tp != nil { // +150 kcal of fat: week fat 36% > 35%
			t.Fatalf("offered %+v", tp)
		}
	})
}

// TestSync_SideSwapKeepsBatchCookDay: swapping a side out of a batch that
// plate 4 cooks and plate 5 eats the next day moves the batch's cook to
// plate 5 (D11), a second cook occasion on its day (H2 over 1). The check
// sees the move: the shared polenta is not swapped, the ciabatta is.
func TestSync_SideSwapKeepsBatchCookDay(t *testing.T) {
	t.Parallel()
	run := func(t *testing.T, shared bool) tips.Op {
		f := newFixture()
		f.st.Settings.MaxCookOccasionsPerDay = 1
		side := func(id int64, name string, texture string) planning.FoodFacts {
			return planning.FoodFacts{
				ID: id, Name: name, Kind: food.KindComposed, Role: food.RoleSideStarch, DietClass: diet.Vegan,
				Macros: nutrition.Macros{Kcal: 150}, NutritionComplete: true, Minutes: 10, Starch: ptr(true), Fresh: ptr(false),
				Attrs: flavor.Attributes{Cuisine: "italian", Texture: texture, Temperature: "hot", Colour: "beige", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{}},
			}
		}
		f.st.Foods[200] = side(200, "Polenta", "soft")
		f.st.Foods[201] = side(201, "Ciabatta", "crisp")
		f.st.Foods[202] = planning.FoodFacts{
			ID: 202, Name: "Insalata mista", Kind: food.KindComposed, Role: food.RoleSideVeg, DietClass: diet.Vegan,
			Macros: nutrition.Macros{Kcal: 150}, NutritionComplete: true, Minutes: 10, Starch: ptr(false), Fresh: ptr(false),
			Attrs: flavor.Attributes{Cuisine: "italian", Texture: "crisp", Temperature: "cold", Colour: "green", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{Acid: 2}},
		}
		f.st.Foods[104] = f.st.Foods[103]
		for id, fd := range f.st.Foods {
			if fd.Role == food.RoleMain {
				fd.ID, fd.Starch = id, ptr(false)
				f.st.Foods[id] = fd
			}
		}
		f.st.Plates = append(f.st.Plates, planning.Plate{
			ID: 5, Date: start.AddDate(0, 0, 4), SlotID: 1, Eaters: 1,
			Components: []planning.Component{{ID: 14, FoodID: 104, Portions: ptr(1.0), SessionID: 54, Position: plate.PositionMain}},
		})
		for i, p := range f.st.Plates {
			f.st.Sessions = append(f.st.Sessions, planning.Session{CookSession: plate.CookSession{ID: int64(50 + i), FoodID: p.Components[0].FoodID, CookDate: p.Date, CookSlotID: 1, Portions: 1}})
		}
		// Plate 5's pasta is cooked at an earlier slot of its day: the
		// polenta's cook at plate 5 would be a second occasion.
		f.st.SlotOrder[2] = 0
		f.st.Sessions[4].CookSlotID = 2
		for _, p := range f.st.Plates {
			f.st.Cells = append(f.st.Cells, planning.Cell{Date: p.Date, SlotID: p.SlotID})
		}
		f.st.Sessions = append(f.st.Sessions, planning.Session{CookSession: plate.CookSession{ID: 60, FoodID: 200, CookDate: f.st.Plates[3].Date, CookSlotID: 1, Portions: 1}})
		f.st.Plates[3].Components = append(f.st.Plates[3].Components,
			planning.Component{ID: 30, FoodID: 200, Portions: ptr(1.0), SessionID: 60, Position: plate.PositionSide},
			planning.Component{ID: 31, FoodID: 201, Portions: ptr(1.0), Position: plate.PositionSide})
		if shared {
			f.st.Sessions[len(f.st.Sessions)-1].Portions = 2
			f.st.Plates[4].Components = append(f.st.Plates[4].Components, planning.Component{ID: 32, FoodID: 200, Portions: ptr(1.0), SessionID: 60, Position: plate.PositionSide})
		}
		v, err := f.svc().Sync(context.Background(), start)
		if err != nil {
			t.Fatal(err)
		}
		for _, tp := range v.Tips {
			if *tp.PlateID == 4 {
				return tp.Op
			}
		}
		t.Fatalf("no tip on plate 4: %+v", v.Tips)
		return tips.Op{}
	}
	t.Run("own batch: the polenta is swapped", func(t *testing.T) {
		t.Parallel()
		if op := run(t, false); op.Action != "swap" || op.ComponentID != 30 {
			t.Fatalf("op %+v, want the polenta (component 30) swapped", op)
		}
	})
	t.Run("shared batch: the ciabatta is swapped", func(t *testing.T) {
		t.Parallel()
		if op := run(t, true); op.Action != "swap" || op.ComponentID != 31 {
			t.Fatalf("op %+v, want the ciabatta (component 31) swapped", op)
		}
	})
}

// TestSync_SameFoodAtMostTwicePerKind: with one lemon in the library, four
// plates that all want it get two tips (rule 3).
func TestSync_SameFoodAtMostTwicePerKind(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.never[zesteID] = true
	v, err := f.svc().Sync(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Tips) != 2 {
		t.Fatalf("%d tips, want 2: %+v", len(v.Tips), v.Tips)
	}
}

// TestSync_RatherNotMeansNoSideSwap: a household with refused sides gets no
// side-swap tip, garnish only (rule 1).
func TestSync_RatherNotMeansNoSideSwap(t *testing.T) {
	t.Parallel()
	build := func(ratherNot bool) []tips.Tip {
		f := newFixture()
		side := func(id int64, name string, role food.Role, cuisine string, starch bool) planning.FoodFacts {
			return planning.FoodFacts{
				ID: id, Name: name, Kind: food.KindComposed, Role: role, DietClass: diet.Vegan, Starch: ptr(starch),
				Macros: nutrition.Macros{Kcal: 150}, NutritionComplete: true, Minutes: 10, Fresh: ptr(false),
				Attrs: flavor.Attributes{Cuisine: cuisine, Texture: "soft", Temperature: "hot", Colour: "white", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{}},
			}
		}
		f.st.Foods[700] = side(700, "Thai-Curryreis", food.RoleSideStarch, "thai", true)
		f.st.Foods[701] = side(701, "Gemüse", food.RoleSideVeg, "italian", false)
		f.st.NoCook = nil
		f.st.Plates = f.st.Plates[:1]
		f.st.Plates[0].Components = append(f.st.Plates[0].Components, planning.Component{ID: 20, FoodID: 700, Position: plate.PositionSide})
		if ratherNot {
			f.st.RatherNot = map[int64]bool{999: true}
		}
		v, err := f.svc().Sync(context.Background(), start)
		if err != nil {
			t.Fatal(err)
		}
		return v.Tips
	}
	if got := build(false); len(got) != 1 || got[0].Op.Action != "swap" {
		t.Fatalf("fixture raises no swappable clash: %+v", got)
	}
	if got := build(true); len(got) != 0 {
		t.Fatalf("side swap tipped despite refusals: %+v", got)
	}
}

// TestSync_SwapPrefersEatenSide: of two sides that fix a clash, the one the
// household has eaten before is suggested.
func TestSync_SwapPrefersEatenSide(t *testing.T) {
	t.Parallel()
	for _, eaten := range []int64{701, 702} {
		f := newFixture()
		side := func(id int64, name string, role food.Role, starch bool) planning.FoodFacts {
			return planning.FoodFacts{
				ID: id, Name: name, Kind: food.KindComposed, Role: role, DietClass: diet.Vegan, Starch: ptr(starch),
				Macros: nutrition.Macros{Kcal: 150}, NutritionComplete: true, Minutes: 10, Fresh: ptr(false),
				Attrs: flavor.Attributes{Cuisine: "italian", Texture: "soft", Temperature: "hot", Colour: "white", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{}},
			}
		}
		f.st.Foods[700] = side(700, "Thai-Curryreis", food.RoleSideStarch, true)
		f.st.Foods[700] = func() planning.FoodFacts { x := f.st.Foods[700]; x.Attrs.Cuisine = "thai"; return x }()
		f.st.Foods[701] = side(701, "Gemüse A", food.RoleSideVeg, false)
		f.st.Foods[702] = side(702, "Gemüse B", food.RoleSideVeg, false)
		f.st.NoCook = nil
		f.st.Plates = f.st.Plates[:1]
		f.st.Plates[0].Components = append(f.st.Plates[0].Components, planning.Component{ID: 20, FoodID: 700, Position: plate.PositionSide})
		f.st.History = []planning.HistoryPlate{{FoodIDs: []int64{eaten}}}
		v, err := f.svc().Sync(context.Background(), start)
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Tips) != 1 || v.Tips[0].Op.FoodID != eaten {
			t.Errorf("eaten %d: got %+v", eaten, v.Tips)
		}
	}
}

// TestSync_TipSkipsFoodServedElsewhereThatDay: the swap that would put a
// side on one plate is not offered when another plate of the same day
// already serves it; the other side is.
func TestSync_TipSkipsFoodServedElsewhereThatDay(t *testing.T) {
	t.Parallel()
	f := newFixture()
	side := func(id int64, name string) planning.FoodFacts {
		return planning.FoodFacts{
			ID: id, Name: name, Kind: food.KindComposed, Role: food.RoleSideVeg, DietClass: diet.Vegan, Starch: ptr(false),
			Macros: nutrition.Macros{Kcal: 150}, NutritionComplete: true, Minutes: 10, Fresh: ptr(false),
			Attrs: flavor.Attributes{Cuisine: "italian", Texture: "soft", Temperature: "hot", Colour: "white", Richness: ptr(0), Saucy: ptr(false), Taste: &flavor.Taste{}},
		}
	}
	curry := side(700, "Thai-Curryreis")
	curry.Role, curry.Starch = food.RoleSideStarch, ptr(true)
	curry.Attrs.Cuisine = "thai"
	f.st.Foods[700], f.st.Foods[701], f.st.Foods[702] = curry, side(701, "Gemüse A"), side(702, "Gemüse B")
	f.st.NoCook = nil
	f.st.Plates = f.st.Plates[:2]
	f.st.Plates[0].Components = append(f.st.Plates[0].Components, planning.Component{ID: 20, FoodID: 700, Position: plate.PositionSide})
	f.st.Plates[1].Date = start // same day
	f.st.Plates[1].Components = append(f.st.Plates[1].Components, planning.Component{ID: 21, FoodID: 701, Position: plate.PositionSide})
	f.st.History = []planning.HistoryPlate{{FoodIDs: []int64{701}}} // 701 is the preferred side
	v, err := f.svc().Sync(context.Background(), start)
	if err != nil {
		t.Fatal(err)
	}
	for _, tp := range v.Tips {
		if *tp.PlateID == 1 && tp.Op.FoodID != 702 {
			t.Errorf("plate 1 tip repeats the day's food: %+v", tp)
		}
	}
	if !slices.ContainsFunc(v.Tips, func(tp tips.Tip) bool { return *tp.PlateID == 1 && tp.Op.FoodID == 702 }) {
		t.Errorf("no swap to 702 on plate 1: %+v", v.Tips)
	}
}
