package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
)

// SC2: nothing is added that was not asked for. An offer (a checked option, a
// day's fix, a compound way) is applied only when the user says yes, in a
// later turn. offeredMemo remembers, per conversation, the add ops the tools
// offered in the conversation's latest turn; apply_changes refuses an add that
// is such an offer, unless the user named its food or its meal (B-4).
//
// The memo keeps one turn per conversation: recording in a later turn drops
// the earlier one, so it never grows past the open conversations.
type offeredMemo struct {
	mu sync.Mutex
	m  map[int64]*turnOffers
	// cell resolves a plate to the cell it sits on; unresolved plates are
	// keyed by id.
	cell func(ctx context.Context, plateID int64) (date string, slotID int64, ok bool)
}

type turnOffers struct {
	seq              uint64
	offers, requests map[string]bool
}

// maxMemoConversations bounds the memo when conversations are never closed.
const maxMemoConversations = 512

// addKey identifies an add by its food and the cell it lands on, whether the
// op gives the cell as plate_id or as date and slot_id.
func (o *offeredMemo) addKey(ctx context.Context, op changeOp) (key, cell string, ok bool) {
	if op.Op != "add" || op.FoodID == 0 {
		return "", "", false
	}
	date, slot := op.Date, op.SlotID
	if op.PlateID != 0 && o.cell != nil {
		if d, s, found := o.cell(ctx, op.PlateID); found {
			date, slot = d, s
		}
	}
	if date != "" && slot != 0 {
		cell = fmt.Sprintf("%s|%d", date, slot)
		return fmt.Sprintf("%d|%s", op.FoodID, cell), cell, true
	}
	return fmt.Sprintf("%d|plate%d", op.FoodID, op.PlateID), "", true
}

// offerOps are, per tool result, the ops that are offers and the ops that are
// the request itself. Only the result shapes of these tools are read.
func offerOps(tool string, output json.RawMessage) (offers, requested []changeOp) {
	var r struct {
		After struct {
			Issues struct {
				New     []issueOptions `json:"new"`
				DayFix  []issueOptions `json:"day_fix"`
				Worsend []issueOptions `json:"worsened"`
			} `json:"issues"`
		} `json:"after"`
		Offers []opsHolder `json:"offers"`
		Offer  []opsHolder `json:"offer"`
		Ways   []opsHolder `json:"ways"`
		Alts   []opsHolder `json:"alternatives"`
	}
	if json.Unmarshal(output, &r) != nil {
		return nil, nil
	}
	switch tool {
	case applyChangesName:
		for _, list := range [][]issueOptions{r.After.Issues.New, r.After.Issues.DayFix, r.After.Issues.Worsend} {
			for _, is := range list {
				for _, o := range is.Options {
					offers = append(offers, o.all()...)
				}
			}
		}
		for _, h := range r.Offers {
			offers = append(offers, h.all()...)
		}
	case "find_ways":
		for _, h := range r.Offer {
			offers = append(offers, h.all()...)
		}
		for _, h := range r.Ways {
			requested = append(requested, h.Ops...)
			if h.Fix != nil {
				offers = append(offers, h.Fix.Ops...)
			}
		}
	case "find_alternatives":
		for _, h := range r.Alts {
			requested = append(requested, h.Ops...)
		}
	}
	return offers, requested
}

type opsHolder struct {
	Ops []changeOp `json:"ops"`
	Fix *struct {
		Ops []changeOp `json:"ops"`
	} `json:"fix"`
}

func (h opsHolder) all() []changeOp {
	out := append([]changeOp{}, h.Ops...)
	if h.Fix != nil {
		out = append(out, h.Fix.Ops...)
	}
	return out
}

type issueOptions struct {
	Options []optionOps `json:"options"`
}

type optionOps struct {
	Op  changeOp   `json:"op"`
	Ops []changeOp `json:"ops"`
}

func (o optionOps) all() []changeOp {
	if len(o.Ops) > 0 {
		return o.Ops
	}
	return []changeOp{o.Op}
}

// record notes the offers (and request ops) of a tool result of ctx's turn.
func (o *offeredMemo) record(ctx context.Context, tool string, output json.RawMessage) {
	t, ok := turnFrom(ctx)
	if o == nil {
		panic("agent: offeredMemo not wired (NewToolSet sets Services.Offered)")
	}
	if !ok {
		return
	}
	offers, requested := offerOps(tool, output)
	if len(offers) == 0 && len(requested) == 0 {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.m == nil || len(o.m) >= maxMemoConversations {
		o.m = map[int64]*turnOffers{}
	}
	cur := o.m[t.conversationID]
	if cur == nil || cur.seq != t.seq {
		cur = &turnOffers{seq: t.seq, offers: map[string]bool{}, requests: map[string]bool{}}
		o.m[t.conversationID] = cur
	}
	for _, op := range offers {
		if k, _, ok := o.addKey(ctx, op); ok {
			cur.offers[k] = true
		}
	}
	for _, op := range requested {
		if k, _, ok := o.addKey(ctx, op); ok {
			cur.requests[k] = true
		}
	}
}

// offeredNow reports whether op is an add offered in this very turn that the
// user did not ask for: its food and its meal are not named in the message.
func (o *offeredMemo) offeredNow(ctx context.Context, op changeOp) bool {
	if o == nil {
		panic("agent: offeredMemo not wired (NewToolSet sets Services.Offered)")
	}
	t, ok := turnFrom(ctx)
	key, cell, isAdd := o.addKey(ctx, op)
	if !ok || !isAdd || t.named[op.FoodID] || (cell != "" && t.cells[cell]) {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	cur := o.m[t.conversationID]
	return cur != nil && cur.seq == t.seq && cur.offers[key] && !cur.requests[key]
}
