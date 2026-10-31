package shopping_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

// fakeSource returns its sessions and leaf uses as given: the date filter is
// the adapter's job (tested against SQLite).
type fakeSource struct {
	sessions []plate.CookSession
	leaves   []shopping.LeafUse
}

func (f *fakeSource) SessionsCooking(context.Context, time.Time, time.Time) ([]plate.CookSession, error) {
	return f.sessions, nil
}

func (f *fakeSource) LeafUses(context.Context, time.Time, time.Time) ([]shopping.LeafUse, error) {
	return f.leaves, nil
}

// fakeFoods is a food tree in memory that counts its round trips.
type fakeFoods struct {
	foods    map[int64]*food.Food
	children map[int64][]food.FoodComponent
	lookups  int
}

func (f *fakeFoods) LookupByIDs(_ context.Context, ids []int64) (map[int64]*food.Food, error) {
	f.lookups++
	out := map[int64]*food.Food{}
	for _, id := range ids {
		if v, ok := f.foods[id]; ok {
			c := *v
			out[id] = &c
		}
	}
	return out, nil
}

func (f *fakeFoods) ListChildrenByParents(_ context.Context, ids []int64) (map[int64][]food.FoodComponent, error) {
	out := map[int64][]food.FoodComponent{}
	for _, id := range ids {
		out[id] = f.children[id]
	}
	return out, nil
}

func ptr[T any](v T) *T { return &v }

const (
	chiliID  = 1
	beansID  = 10
	onionID  = 11
	appleID  = 12
	sauceID  = 2
	tomatoID = 13
	dinner   = 3
)

// ac1Foods is the AC-1 recipe: Chili for 4 with 400 g kidney beans and
// 2 piece onions, plus an apple and a sauce recipe.
func ac1Foods() *fakeFoods {
	return &fakeFoods{
		foods: map[int64]*food.Food{
			chiliID:  {ID: chiliID, Kind: food.KindComposed, Name: "Chili", ReferencePortions: ptr(4.0)},
			sauceID:  {ID: sauceID, Kind: food.KindComposed, Name: "Tomato sauce", ReferencePortions: ptr(2.0)},
			beansID:  {ID: beansID, Kind: food.KindLeaf, Name: "Kidneybohnen (Dose)"},
			onionID:  {ID: onionID, Kind: food.KindLeaf, Name: "Zwiebel"},
			appleID:  {ID: appleID, Kind: food.KindLeaf, Name: "Apfel"},
			tomatoID: {ID: tomatoID, Kind: food.KindLeaf, Name: "Tomaten"},
		},
		children: map[int64][]food.FoodComponent{
			chiliID: {
				{ChildID: beansID, ChildKind: food.KindLeaf, Amount: 400, Unit: "g", Grams: 400},
				{ChildID: onionID, ChildKind: food.KindLeaf, Amount: 2, Unit: "piece", Grams: 240},
			},
			sauceID: {
				{ChildID: tomatoID, ChildKind: food.KindLeaf, Amount: 500, Unit: "g", Grams: 500},
				{ChildID: onionID, ChildKind: food.KindLeaf, Amount: 1, Unit: "piece", Grams: 120},
			},
		},
	}
}

func chiliSession(id int64, portions float64) plate.CookSession {
	return plate.CookSession{ID: id, FoodID: chiliID, CookDate: day("2026-11-01"), CookSlotID: dinner, Portions: portions}
}

func findItem(t *testing.T, l *shopping.List, foodID int64) (food.ShopSection, shopping.Item) {
	t.Helper()
	for _, s := range l.Sections {
		for _, it := range s.Items {
			if it.FoodID == foodID {
				return s.Section, it
			}
		}
	}
	t.Fatalf("food %d not on the list", foodID)
	return "", shopping.Item{}
}

func TestList_AC1_SessionScaledByPortions(t *testing.T) {
	src := &fakeSource{sessions: []plate.CookSession{chiliSession(7, 6)}}
	l, err := shopping.NewService(src, ac1Foods(), nil).List(context.Background(), day("2026-10-31"), day("2026-11-06"))
	require.NoError(t, err)
	assert.Equal(t, 2, l.Count)

	sec, beans := findItem(t, l, beansID)
	assert.Equal(t, food.SectionCansJars, sec)
	assert.Equal(t, "Kidneybohnen (Dose)", beans.Name)
	assert.InDelta(t, 600, beans.TotalGrams, 1e-9)
	assert.Equal(t, units.Quantity{Amount: 600, Unit: "g"}, beans.Display)
	assert.Nil(t, beans.Buy)
	require.Len(t, beans.UsedIn, 1)
	assert.Equal(t, shopping.Use{SessionID: ptr(int64(7)), FoodID: chiliID, FoodName: "Chili", Date: day("2026-11-01"), SlotID: dinner}, beans.UsedIn[0])

	sec, onion := findItem(t, l, onionID)
	assert.Equal(t, food.SectionProduce, sec)
	assert.Equal(t, units.Quantity{Amount: 3, Unit: "piece"}, onion.Display)
	require.NotNil(t, onion.Buy)
	assert.Equal(t, units.Quantity{Amount: 3, Unit: "piece"}, *onion.Buy)
	assert.InDelta(t, 360, onion.TotalGrams, 1e-9)
	require.Len(t, onion.UsedIn, 1)
	assert.Equal(t, int64(7), *onion.UsedIn[0].SessionID)
}

func TestList_AC3_LeafComponentTimesEaters(t *testing.T) {
	src := &fakeSource{leaves: []shopping.LeafUse{
		{FoodID: appleID, Date: day("2026-11-02"), SlotID: 1, Eaters: 3.5, Amount: 1, Unit: "piece", Grams: 150},
	}}
	l, err := shopping.NewService(src, ac1Foods(), nil).List(context.Background(), day("2026-10-31"), day("2026-11-06"))
	require.NoError(t, err)
	_, apple := findItem(t, l, appleID)
	assert.Equal(t, units.Quantity{Amount: 3.5, Unit: "piece"}, apple.Display)
	require.NotNil(t, apple.Buy)
	assert.Equal(t, units.Quantity{Amount: 4, Unit: "piece"}, *apple.Buy)
	assert.InDelta(t, 525, apple.TotalGrams, 1e-9)
	require.Len(t, apple.UsedIn, 1)
	assert.Equal(t, shopping.Use{FoodID: appleID, FoodName: "Apfel", Date: day("2026-11-02"), SlotID: 1}, apple.UsedIn[0])
}

func TestList_NestedComposedChildScaledByGramsShare(t *testing.T) {
	foods := ac1Foods()
	// Chili also takes 310 g of the sauce, half of its 620 g batch.
	foods.children[chiliID] = append(foods.children[chiliID],
		food.FoodComponent{ChildID: sauceID, ChildKind: food.KindComposed, Amount: 310, Unit: "g", Grams: 310})
	src := &fakeSource{sessions: []plate.CookSession{chiliSession(7, 4)}}
	l, err := shopping.NewService(src, foods, nil).List(context.Background(), day("2026-10-31"), day("2026-11-06"))
	require.NoError(t, err)

	_, tomato := findItem(t, l, tomatoID)
	assert.InDelta(t, 250, tomato.TotalGrams, 1e-9)
	assert.Equal(t, units.Quantity{Amount: 250, Unit: "g"}, tomato.Display)

	_, onion := findItem(t, l, onionID)
	assert.InDelta(t, 2.5, onion.Display.Amount, 1e-9, "2 from the chili + ½ from the sauce")
	assert.Equal(t, 3.0, onion.Buy.Amount)
	require.Len(t, onion.UsedIn, 1, "one entry per session")
	assert.Equal(t, 3, foods.lookups, "three tree levels load in three lookups")
}

func TestList_MixedUnitsShowGrams(t *testing.T) {
	src := &fakeSource{
		sessions: []plate.CookSession{chiliSession(7, 4)},
		leaves: []shopping.LeafUse{
			{FoodID: onionID, Date: day("2026-11-03"), SlotID: 1, Eaters: 2, Amount: 50, Unit: "g", Grams: 50},
		},
	}
	l, err := shopping.NewService(src, ac1Foods(), nil).List(context.Background(), day("2026-10-31"), day("2026-11-06"))
	require.NoError(t, err)
	_, onion := findItem(t, l, onionID)
	assert.InDelta(t, 340, onion.TotalGrams, 1e-9)
	assert.Equal(t, units.Quantity{Amount: 340, Unit: "g"}, onion.Display)
	assert.Nil(t, onion.Buy)
	require.Len(t, onion.UsedIn, 2)
	assert.NotNil(t, onion.UsedIn[0].SessionID, "the session cooks first")
	assert.Nil(t, onion.UsedIn[1].SessionID)
}

func TestList_SectionsInWalkOrderItemsByName(t *testing.T) {
	foods := ac1Foods()
	foods.foods[tomatoID].ShopSection = ptr(food.SectionFrozen)
	src := &fakeSource{leaves: []shopping.LeafUse{
		{FoodID: tomatoID, Date: day("2026-11-02"), SlotID: 1, Eaters: 1, Amount: 100, Unit: "g", Grams: 100},
		{FoodID: appleID, Date: day("2026-11-02"), SlotID: 1, Eaters: 1, Amount: 1, Unit: "piece", Grams: 150},
		{FoodID: onionID, Date: day("2026-11-02"), SlotID: 1, Eaters: 1, Amount: 1, Unit: "piece", Grams: 120},
		{FoodID: beansID, Date: day("2026-11-02"), SlotID: 1, Eaters: 1, Amount: 100, Unit: "g", Grams: 100},
	}}
	l, err := shopping.NewService(src, foods, nil).List(context.Background(), day("2026-10-31"), day("2026-11-06"))
	require.NoError(t, err)
	require.Len(t, l.Sections, 3)
	assert.Equal(t, food.SectionProduce, l.Sections[0].Section)
	assert.Equal(t, []string{"Apfel", "Zwiebel"}, []string{l.Sections[0].Items[0].Name, l.Sections[0].Items[1].Name})
	assert.Equal(t, food.SectionCansJars, l.Sections[1].Section)
	assert.Equal(t, food.SectionFrozen, l.Sections[2].Section, "the override wins over the name")
	assert.Equal(t, 4, l.Count)
}

func TestList_Empty(t *testing.T) {
	l, err := shopping.NewService(&fakeSource{}, ac1Foods(), nil).List(context.Background(), day("2026-10-31"), day("2026-11-06"))
	require.NoError(t, err)
	assert.Empty(t, l.Sections)
	assert.NotNil(t, l.Sections)
	assert.Zero(t, l.Count)
}
