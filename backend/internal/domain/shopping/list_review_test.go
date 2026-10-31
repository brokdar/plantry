package shopping_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/units"
)

// fakeSlots lists slots with their sort order.
type fakeSlots []slot.TimeSlot

func (f fakeSlots) List(context.Context, bool) ([]slot.TimeSlot, error) { return f, nil }

func newSvc(src shopping.Source, foods food.TreeReader) *shopping.Service {
	return shopping.NewService(shopping.Deps{Source: src, Foods: foods, Slots: fakeSlots{}})
}

func TestList_SameMetricBaseIsNotMixed(t *testing.T) {
	const milkID, flourID = 20, 21
	foods := ac1Foods()
	foods.foods[milkID] = &food.Food{ID: milkID, Kind: food.KindLeaf, Name: "Milch"}
	foods.foods[flourID] = &food.Food{ID: flourID, Kind: food.KindLeaf, Name: "Mehl"}
	foods.children[chiliID] = []food.FoodComponent{
		{ChildID: milkID, ChildKind: food.KindLeaf, Amount: 500, Unit: "ml", Grams: 500},
		{ChildID: flourID, ChildKind: food.KindLeaf, Amount: 200, Unit: "g", Grams: 200},
	}
	src := &fakeSource{
		sessions: []plate.CookSession{chiliSession(7, 4)},
		leaves: []shopping.LeafUse{
			{FoodID: milkID, Date: day("2026-11-02"), SlotID: 1, Eaters: 1, Amount: 1, Unit: "l", Grams: 1000},
			{FoodID: flourID, Date: day("2026-11-02"), SlotID: 1, Eaters: 1, Amount: 1, Unit: "kg", Grams: 1000},
		},
	}
	l, err := newSvc(src, foods).List(context.Background(), day("2026-10-31"), day("2026-11-06"))
	require.NoError(t, err)

	_, milk := findItem(t, l, milkID)
	assert.Equal(t, units.Quantity{Amount: 1.5, Unit: "l"}, milk.Display, "500 ml + 1 l")
	assert.Nil(t, milk.Buy)
	_, flour := findItem(t, l, flourID)
	assert.Equal(t, units.Quantity{Amount: 1.2, Unit: "kg"}, flour.Display, "200 g + 1 kg")
}

func TestList_UsedInFollowsCellOrder(t *testing.T) {
	const breakfast, lunch = 5, 6 // ids against sort order: lunch (id 6) sorts before breakfast (id 5)
	src := &fakeSource{leaves: []shopping.LeafUse{
		{FoodID: appleID, Date: day("2026-11-03"), SlotID: breakfast, Eaters: 1, Amount: 1, Unit: "piece", Grams: 150},
		{FoodID: appleID, Date: day("2026-11-02"), SlotID: breakfast, Eaters: 1, Amount: 1, Unit: "piece", Grams: 150},
		{FoodID: appleID, Date: day("2026-11-02"), SlotID: lunch, Eaters: 1, Amount: 1, Unit: "piece", Grams: 150},
	}}
	svc := shopping.NewService(shopping.Deps{Source: src, Foods: ac1Foods(), Slots: fakeSlots{
		{ID: breakfast, SortOrder: 2}, {ID: lunch, SortOrder: 1},
	}})
	l, err := svc.List(context.Background(), day("2026-10-31"), day("2026-11-06"))
	require.NoError(t, err)
	_, apple := findItem(t, l, appleID)
	require.Len(t, apple.UsedIn, 3)
	got := make([][2]any, len(apple.UsedIn))
	for i, u := range apple.UsedIn {
		got[i] = [2]any{u.Date.Format("2006-01-02"), u.SlotID}
	}
	assert.Equal(t, [][2]any{
		{"2026-11-02", int64(lunch)}, {"2026-11-02", int64(breakfast)}, {"2026-11-03", int64(breakfast)},
	}, got)
}
