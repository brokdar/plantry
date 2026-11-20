package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// SC2: an op the tools offered is not applied in the turn that offered it
// (unless it is also the request, or the user named its food); a later turn
// may apply it.
func TestOfferedMemo_AnOfferWaitsForAYes(t *testing.T) {
	t.Parallel()
	m := &offeredMemo{}
	turn := func(seq uint64, named ...int64) context.Context {
		n := map[int64]bool{}
		for _, f := range named {
			n[f] = true
		}
		return withTurn(context.Background(), chatTurn{conversationID: 1, seq: seq, named: n})
	}
	add := func(food int64) changeOp {
		return changeOp{Op: "add", PlateID: 7, FoodID: food}
	}
	result := json.RawMessage(`{
		"after":{"issues":{"day_fix":[{"options":[{"op":{"op":"add","plate_id":7,"food_id":10}}]}]}},
		"ways":[{"ops":[{"op":"add","plate_id":7,"food_id":12}]}],
		"offer":[{"ops":[{"op":"add","plate_id":7,"food_id":12},{"op":"add","plate_id":7,"food_id":11}],"fix":{"ops":[{"op":"add","plate_id":7,"food_id":13}]}}]}`)
	m.record(turn(5), result)

	assert.True(t, m.offeredNow(turn(5), add(10)), "a day fix offered this turn")
	assert.True(t, m.offeredNow(turn(5), add(11)), "a compound offer's op")
	assert.True(t, m.offeredNow(turn(5), add(13)), "an offer's fix")
	assert.False(t, m.offeredNow(turn(5), add(12)), "an op that is the request itself (a way) is no offer")
	assert.False(t, m.offeredNow(turn(5, 10), add(10)), "the user named the food")
	assert.False(t, m.offeredNow(turn(6), add(10)), "the next turn may apply it: that is the yes")
	assert.False(t, m.offeredNow(turn(5), add(99)), "an op nobody offered")
	assert.False(t, (*offeredMemo)(nil).offeredNow(turn(5), add(10)))
}
