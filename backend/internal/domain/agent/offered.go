package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// SC2: nothing is added that was not asked for. An offer (a checked option, a
// day's fix, a compound way) is applied only when the user says yes, in a
// later turn. offeredMemo remembers, per conversation, the ops the tools
// offered and the turn that offered them; apply_changes refuses a write op
// that is an offer of its own turn, unless the user named its food.
type offeredMemo struct {
	mu sync.Mutex
	m  map[int64]map[string]uint64 // conversation -> op key -> the turn that offered it
	// r are the ops of the request itself (ways, alternatives) by turn: an op
	// that is both is the request, not an offer.
	r map[int64]map[string]uint64
}

// opKey identifies the change an op makes, the way the options print it.
func opKey(op changeOp) string {
	return fmt.Sprintf("%s|%d|%d|%d|%s|%d", op.Op, op.PlateID, op.PlateComponentID, op.FoodID, op.Date, op.SlotID)
}

// offerKeys are the keys of every op that sits in an offer of a tool result
// (under the keys offer, offers, options and fix) and of every op of the
// request itself (under ways and alternatives, outside an offer).
func offerKeys(output json.RawMessage) (offers, requested []string) {
	var v any
	if json.Unmarshal(output, &v) != nil {
		return nil, nil
	}
	var walk func(n any, inOffer, inRequest bool)
	walk = func(n any, inOffer, inRequest bool) {
		switch x := n.(type) {
		case map[string]any:
			if _, isOp := x["op"].(string); isOp && (inOffer || inRequest) {
				if raw, err := json.Marshal(x); err == nil {
					var op changeOp
					if json.Unmarshal(raw, &op) == nil {
						if inOffer {
							offers = append(offers, opKey(op))
						} else {
							requested = append(requested, opKey(op))
						}
					}
				}
			}
			for k, c := range x {
				off := inOffer || k == "offer" || k == "offers" || k == "options" || k == "fix"
				walk(c, off, inRequest || (!off && (k == "ways" || k == "alternatives")))
			}
		case []any:
			for _, c := range x {
				walk(c, inOffer, inRequest)
			}
		}
	}
	walk(v, false, false)
	return offers, requested
}

// record notes the offers of a tool result as made in ctx's turn.
func (o *offeredMemo) record(ctx context.Context, output json.RawMessage) {
	t, ok := turnFrom(ctx)
	if o == nil || !ok {
		return
	}
	offers, requested := offerKeys(output)
	if len(offers) == 0 && len(requested) == 0 {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	note := func(m *map[int64]map[string]uint64, keys []string) {
		if *m == nil {
			*m = map[int64]map[string]uint64{}
		}
		if (*m)[t.conversationID] == nil {
			(*m)[t.conversationID] = map[string]uint64{}
		}
		for _, k := range keys {
			(*m)[t.conversationID][k] = t.seq
		}
	}
	note(&o.m, offers)
	note(&o.r, requested)
}

// offeredNow reports whether op is an offer made in this very turn whose food
// the user did not name.
func (o *offeredMemo) offeredNow(ctx context.Context, op changeOp) bool {
	t, ok := turnFrom(ctx)
	if o == nil || !ok || t.named[op.FoodID] {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	key := opKey(op)
	seq, found := o.m[t.conversationID][key]
	req, isReq := o.r[t.conversationID][key]
	return found && seq == t.seq && (!isReq || req != t.seq)
}
