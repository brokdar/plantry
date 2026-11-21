package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

func testTurn(seq uint64, named []int64, cells ...string) context.Context {
	n := map[int64]bool{}
	for _, f := range named {
		n[f] = true
	}
	c := map[string]bool{}
	for _, k := range cells {
		c[k] = true
	}
	return withTurn(context.Background(), chatTurn{conversationID: 1, seq: seq, named: n, cells: c})
}

func plateMemo() *offeredMemo {
	return &offeredMemo{cell: func(_ context.Context, id int64) (string, int64, bool) {
		if id == 7 {
			return "2026-11-24", 3, true
		}
		return "", 0, false
	}}
}

// SC2: an add the tools offered is not applied in the turn that offered it
// (unless it is also the request, or the user named its food or its meal); a
// later turn may apply it. The same add by plate_id or by date and slot is the
// same add.
func TestOfferedMemo_AnOfferWaitsForAYes(t *testing.T) {
	m := plateMemo()
	byPlate := func(food int64) changeOp { return changeOp{Op: "add", PlateID: 7, FoodID: food} }
	byCell := func(food int64) changeOp { return changeOp{Op: "add", Date: "2026-11-24", SlotID: 3, FoodID: food} }
	result := json.RawMessage(`{
		"after":{"issues":{"day_fix":[{"options":[{"op":{"op":"add","plate_id":7,"food_id":10}}]}]}},
		"ways":[{"ops":[{"op":"add","plate_id":7,"food_id":12}]}],
		"offer":[{"ops":[{"op":"add","plate_id":7,"food_id":12},{"op":"add","plate_id":7,"food_id":11}],"fix":{"ops":[{"op":"add","plate_id":7,"food_id":13}]}}]}`)
	m.record(testTurn(5, nil), "find_ways", result)
	m.record(testTurn(5, nil), "apply_changes", result)

	assert.True(t, m.offeredNow(testTurn(5, nil), byPlate(10)), "a day fix offered this turn")
	assert.True(t, m.offeredNow(testTurn(5, nil), byCell(10)), "the same add given as date and slot")
	assert.True(t, m.offeredNow(testTurn(5, nil), byPlate(11)), "a compound offer's op")
	assert.True(t, m.offeredNow(testTurn(5, nil), byPlate(13)), "an offer's fix")
	assert.False(t, m.offeredNow(testTurn(5, nil), byPlate(12)), "an op that is the request itself (a way) is no offer")
	assert.False(t, m.offeredNow(testTurn(5, []int64{10}), byPlate(10)), "the user named the food")
	assert.False(t, m.offeredNow(testTurn(5, nil, "2026-11-24|3"), byPlate(10)), "the user named the meal (B-4)")
	assert.True(t, m.offeredNow(testTurn(5, nil, "2026-11-25|3"), byPlate(10)), "another meal named")
	assert.False(t, m.offeredNow(testTurn(6, nil), byPlate(10)), "the next turn may apply it: that is the yes")
	assert.False(t, m.offeredNow(testTurn(5, nil), byPlate(99)), "an op nobody offered")
	assert.False(t, m.offeredNow(testTurn(5, nil), changeOp{Op: "set_quantity", PlateComponentID: 4}), "only adds are SC2's")
}

// The memo keeps the latest turn of a conversation only.
func TestOfferedMemo_KeepsOnlyTheLatestTurn(t *testing.T) {
	m := plateMemo()
	res := json.RawMessage(`{"offers":[{"ops":[{"op":"add","plate_id":7,"food_id":10}]}]}`)
	for seq := uint64(1); seq <= 50; seq++ {
		m.record(testTurn(seq, nil), "apply_changes", res)
	}
	assert.Len(t, m.m, 1)
	assert.Len(t, m.m[1].offers, 1)
	assert.EqualValues(t, 50, m.m[1].seq)
	assert.False(t, m.offeredNow(testTurn(3, nil), changeOp{Op: "add", PlateID: 7, FoodID: 10}), "an old turn is forgotten")
}

// Only the offer fields of the known tool results are read: an options or fix
// key anywhere else is not an offer.
func TestOfferedMemo_ReadsOnlyKnownResultShapes(t *testing.T) {
	m := plateMemo()
	m.record(testTurn(5, nil), "get_plan", json.RawMessage(`{"options":[{"op":"add","plate_id":7,"food_id":10}],"fix":{"op":"add","plate_id":7,"food_id":10}}`))
	m.record(testTurn(5, nil), "apply_changes", json.RawMessage(`{"applied":[{"op":"add","plate_id":7,"food_id":10,"options":[{"op":"add","plate_id":7,"food_id":10}]}]}`))
	assert.False(t, m.offeredNow(testTurn(5, nil), changeOp{Op: "add", PlateID: 7, FoodID: 10}))
}

// A memo that was never wired fails loudly instead of switching SC2 off.
func TestOfferedMemo_NilPanics(t *testing.T) {
	var m *offeredMemo
	assert.Panics(t, func() { m.offeredNow(testTurn(1, nil), changeOp{Op: "add", FoodID: 1}) })
	assert.Panics(t, func() { m.record(testTurn(1, nil), "apply_changes", json.RawMessage(`{}`)) })
}

func TestNamedCells(t *testing.T) {
	w := window.Window{Start: time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)}
	slots := map[int64]string{1: "slot.breakfast", 2: "slot.lunch", 3: "slot.dinner"}
	assert.Equal(t, map[string]bool{"2026-11-26|3": true}, namedCells("Make Thursday dinner lighter and bigger", w, slots))
	assert.Equal(t, map[string]bool{"2026-11-26|3": true, "2026-11-26|2": true}, namedCells("Donnerstag Mittagessen und Abendessen", w, slots))
	assert.Empty(t, namedCells("make Thursday lighter", w, slots), "a day alone names no meal")
	assert.Empty(t, namedCells("a lighter dinner", w, slots), "a meal alone names no day")
}
