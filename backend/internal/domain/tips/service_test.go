package tips_test

import (
	"context"
	"slices"
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
	"github.com/jaltszeimer/plantry/backend/internal/domain/tips"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// memStore is an in-memory tips.Store.
type memStore struct {
	tips      []tips.Tip
	dismissed []tips.Dismissal
}

func (m *memStore) Create(_ context.Context, t *tips.Tip) error {
	t.ID = int64(len(m.tips) + 1)
	m.tips = append(m.tips, *t)
	return nil
}

func (m *memStore) Get(_ context.Context, id int64) (tips.Tip, error) {
	for _, t := range m.tips {
		if t.ID == id {
			return t, nil
		}
	}
	return tips.Tip{}, domain.ErrNotFound
}

func (m *memStore) Window(_ context.Context, start time.Time) ([]tips.Tip, error) {
	var out []tips.Tip
	for _, t := range m.tips {
		if t.WindowStart.Equal(start) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *memStore) SetState(_ context.Context, id int64, st tips.State, at time.Time) error {
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
	confirmed bool
	added     []plate.PlateComponent
	swapped   [][2]int64
}

func (f *fakePlates) Get(_ context.Context, id int64) (*plate.Plate, error) {
	st := plate.StateDraft
	if f.confirmed {
		st = plate.StateConfirmed
	}
	return &plate.Plate{ID: id, State: st, Components: []plate.PlateComponent{{ID: 11}, {ID: 12}}}, nil
}

func (f *fakePlates) Recorded(ctx context.Context, fn func(ctx context.Context) error) (plate.Undo, error) {
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

const lemonID = 900

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
		},
		NoCook:  []int64{lemonID},
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
		f.never[lemonID] = true
		v, _ := f.svc().Sync(context.Background(), start)
		if len(v.Tips) != 0 {
			t.Fatalf("never item tipped: %+v", v.Tips)
		}
	})
	t.Run("diet", func(t *testing.T) {
		t.Parallel()
		f := newFixture()
		l := f.st.Foods[lemonID]
		l.DietClass = diet.Omnivore
		f.st.Foods[lemonID] = l
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

func TestApply_ConfirmedPlateRefused(t *testing.T) {
	t.Parallel()
	f := newFixture()
	f.plates.confirmed = true
	v, _ := f.svc().Sync(context.Background(), start)
	if _, err := f.svc().Apply(context.Background(), v.Tips[0].ID); err != tips.ErrConfirmed {
		t.Fatalf("err = %v, want ErrConfirmed", err)
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
