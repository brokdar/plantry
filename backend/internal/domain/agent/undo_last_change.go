package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

const undoLastChangeName = "undo_last_change"

// undoRefusal is why an undo was refused and what the user can do.
type undoRefusal struct {
	Reason string `json:"reason"`
	WhatTo string `json:"what_to_do"`
}

// undoOut is undo_last_change's result: what was restored, in plain words.
type undoOut struct {
	Undone   int          `json:"undone"`
	Restored []string     `json:"restored"`
	Refused  *undoRefusal `json:"refused,omitempty"`
	Message  string       `json:"message"`
}

func toolUndoLastChange(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "properties":{
        "count":{"type":"integer","minimum":1,"maximum":5,"description":"How many of the newest changes to undo, one after the other (default 1; 2 for \"undo both\")."}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name: undoLastChangeName,
		Description: "Undo the newest change you made in this conversation that is not undone yet, exactly as it was: the same meals, the same cooking batches and leftovers. Use it whenever the user asks to undo, revert or take back what you just did (count 2 for \"both\"); never rebuild the old state by hand with apply_changes, that loses batch links. After it, tell the user in plain words what is back (use restored), or why it could not be undone. " +
			"Takes no ids. Returns JSON {undone:int, restored:[string] (the meals and what they are again), message, refused?:{reason, what_to_do}}; refused is set when the plan was changed since or a meal is confirmed now, and nothing of that change was touched.",
		Schema: schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Count int `json:"count"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			turn, ok := turnFrom(ctx)
			if !ok {
				return nil, ToolEffectNone, fmt.Errorf("%w: no conversation", domain.ErrInvalidInput)
			}
			list, err := svc.Undo.List(ctx, turn.conversationID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			var open []Change
			for i := len(list) - 1; i >= 0; i-- {
				if list[i].UndoneAt == nil && list[i].Undoable() {
					open = append(open, list[i])
				}
			}
			out := undoOut{Restored: []string{}}
			if len(open) == 0 {
				out.Message = "Nothing to undo: every change made in this conversation is undone already or wrote nothing."
				return mustJSON(out), ToolEffectNone, nil
			}
			// Undo is the user's action (the draft guard would block the agent).
			uctx := actor.With(ctx, actor.User, 0)
			for _, ch := range open[:min(max(in.Count, 1), len(open))] {
				rows, err := svc.Undo.Undo(uctx, ch.ID)
				switch {
				case errors.Is(err, domain.ErrAIChangeStale):
					out.Refused = &undoRefusal{
						Reason: "The plan was changed since that change (by the user on the board or by a later edit), so putting it back exactly would overwrite that edit.",
						WhatTo: "Nothing was changed. The user can ask for the meals they want back and you change them with apply_changes (say that a cooking batch may need a fresh start), or undo by hand on the board.",
					}
				case errors.Is(err, domain.ErrAIChangeConfirmed):
					out.Refused = &undoRefusal{
						Reason: "A meal or cooking batch of that change is confirmed now.",
						WhatTo: "Nothing was changed. Ask the user whether to unlock it (unlock), then undo again.",
					}
				case errors.Is(err, domain.ErrAIChangeUndone):
					out.Refused = &undoRefusal{Reason: "That change was undone already (on the board).", WhatTo: "Nothing more to do for it."}
				case err != nil:
					return nil, ToolEffectNone, err
				}
				if out.Refused != nil {
					break
				}
				lines, err := restoredLines(ctx, svc, ch, rows)
				if err != nil {
					return nil, ToolEffectNone, err
				}
				out.Undone++
				out.Restored = append(out.Restored, lines...)
			}
			switch {
			case out.Refused != nil && out.Undone == 0:
				out.Message = "Not undone: " + out.Refused.Reason
			case out.Refused != nil:
				out.Message = fmt.Sprintf("Undone %d, then stopped: %s", out.Undone, out.Refused.Reason)
			default:
				out.Message = fmt.Sprintf("Undone %d change(s); the plan is as it was before.", out.Undone)
			}
			return mustJSON(out), ToolEffectNone, nil
		},
	}
}

// restoredLines says in plain words what an undone change put back: its meals
// as they are again, the meals it had added that are gone, and its cells.
func restoredLines(ctx context.Context, svc Services, ch Change, rows plate.RowSet) ([]string, error) {
	slots := map[int64]string{}
	if list, err := svc.Slots.List(ctx, false); err == nil {
		for _, sl := range list {
			slots[sl.ID] = strings.TrimPrefix(sl.NameKey, "slot.")
		}
	}
	cell := func(d time.Time, slotID int64) string {
		return d.Format("Mon 2 Jan") + " " + slots[slotID]
	}
	plates, err := svc.Plates.PlatesByIDs(ctx, slices.Clone(rows.PlateIDs))
	if err != nil {
		return nil, err
	}
	var sessionIDs []int64
	for _, p := range plates {
		for _, c := range p.Components {
			if c.CookSessionID != nil {
				sessionIDs = append(sessionIDs, *c.CookSessionID)
			}
		}
	}
	sessionIDs = append(sessionIDs, rows.SessionIDs...)
	slices.Sort(sessionIDs)
	sessions, err := svc.Plates.CookSessions(ctx, slices.Compact(sessionIDs))
	if err != nil {
		return nil, err
	}
	byID := map[int64]plate.CookSession{}
	for _, s := range sessions {
		byID[s.ID] = s
	}
	slices.SortFunc(plates, func(a, b plate.Plate) int {
		if c := a.Date.Compare(b.Date); c != 0 {
			return c
		}
		return int(a.SlotID - b.SlotID)
	})
	var lines []string
	for _, p := range plates {
		parts := make([]string, len(p.Components))
		for i, c := range p.Components {
			parts[i] = c.FoodName
			var s plate.CookSession
			ok := false
			if c.CookSessionID != nil {
				s, ok = byID[*c.CookSessionID]
			}
			switch {
			case !ok:
			case s.CookDate.Equal(p.Date) && s.CookSlotID == p.SlotID:
				parts[i] += " (cooked for this meal)"
			default:
				parts[i] += " (leftover from the batch cooked " + cell(s.CookDate, s.CookSlotID) + ")"
			}
		}
		lines = append(lines, cell(p.Date, p.SlotID)+": "+strings.Join(parts, ", "))
	}
	if gone := len(ch.PlateIDs) - len(plates); gone > 0 {
		lines = append(lines, fmt.Sprintf("%d meal(s) the change had added are gone again", gone))
	}
	for _, cb := range ch.Cells {
		d, err := time.Parse(time.DateOnly, cb.Date)
		if err != nil {
			continue
		}
		mode := "the weekday default"
		if cb.Override != nil {
			mode = cb.Override.CellMode
		}
		lines = append(lines, cell(d, cb.SlotID)+": back to "+mode)
	}
	if ch.Light != nil {
		lines = append(lines, "the light or hearty requests of "+strings.Join(ch.Light.Dates, ", ")+" are as before")
	}
	return lines, nil
}
