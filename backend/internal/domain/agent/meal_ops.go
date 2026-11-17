package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// cellRef is a (date, slot) cell as the ops name it.
type cellRef struct {
	Date   string `json:"date"`
	SlotID int64  `json:"slot_id"`
}

// key parses the cell and fails with error.ai.past_date before today.
func (c cellRef) key(ctx context.Context) (plate.CellKey, error) {
	if c.Date == "" || c.SlotID == 0 {
		return plate.CellKey{}, fmt.Errorf("%w: a cell needs date and slot_id", domain.ErrInvalidInput)
	}
	d, err := parseOpDate(c.Date)
	if err != nil {
		return plate.CellKey{}, err
	}
	if err := guardDate(ctx, d); err != nil {
		return plate.CellKey{}, err
	}
	return plate.CellKey{Date: d, SlotID: c.SlotID}, nil
}

// toCell reads `to` as one cell (move).
func (o changeOp) toCell() (cellRef, error) {
	var c cellRef
	if len(o.To) == 0 || o.To[0] != '{' || json.Unmarshal(o.To, &c) != nil {
		return c, fmt.Errorf("%w: %s needs to {date, slot_id}", domain.ErrInvalidInput, o.Op)
	}
	return c, nil
}

// toCells reads `to` as a list of cells (copy).
func (o changeOp) toCells() ([]cellRef, error) {
	var cs []cellRef
	if len(o.To) == 0 || o.To[0] != '[' || json.Unmarshal(o.To, &cs) != nil || len(cs) == 0 {
		return nil, fmt.Errorf("%w: %s needs to [{date, slot_id}, ...]", domain.ErrInvalidInput, o.Op)
	}
	return cs, nil
}

// occupant reads `occupant`: "swap", "drop" or {date, slot_id}.
func (o changeOp) occupant(ctx context.Context) (plate.Occupant, error) {
	if len(o.Occupant) == 0 {
		return plate.Occupant{}, nil
	}
	var kind string
	if json.Unmarshal(o.Occupant, &kind) == nil {
		switch plate.OccupantKind(kind) {
		case plate.OccupantSwap, plate.OccupantDrop:
			return plate.Occupant{Kind: plate.OccupantKind(kind)}, nil
		}
		return plate.Occupant{}, fmt.Errorf("%w: occupant is \"swap\", \"drop\" or {date, slot_id}, not %q", domain.ErrInvalidInput, kind)
	}
	var c cellRef
	if err := json.Unmarshal(o.Occupant, &c); err != nil {
		return plate.Occupant{}, fmt.Errorf("%w: occupant is \"swap\", \"drop\" or {date, slot_id}", domain.ErrInvalidInput)
	}
	k, err := c.key(ctx)
	if err != nil {
		return plate.Occupant{}, err
	}
	return plate.Occupant{Kind: plate.OccupantTo, To: k}, nil
}

// mealResult adds what every meal op's applied entry carries: the
// operation's also_changed list, compact (absent: nothing else changed).
func mealResult(out map[string]any, res plate.MealResult, b *batch) map[string]any {
	for _, a := range res.AlsoChanged {
		b.aff.Sessions = append(b.aff.Sessions, a.SessionID)
		b.aff.Dates = append(b.aff.Dates, a.Date, a.FromDate)
		if a.PlateID != 0 && !slices.Contains(b.aff.Plates, a.PlateID) {
			b.aff.Plates = append(b.aff.Plates, a.PlateID)
		}
	}
	if len(res.AlsoChanged) > 0 {
		out["also_changed"] = planning.Compact(res.AlsoChanged)
	}
	return out
}

func opMove(ctx context.Context, svc Services, op changeOp, b *batch) (map[string]any, error) {
	if op.PlateID == 0 {
		return nil, fmt.Errorf("%w: move needs plate_id", domain.ErrInvalidInput)
	}
	to, err := op.toCell()
	if err != nil {
		return nil, err
	}
	src, err := svc.Plates.Get(ctx, op.PlateID)
	if err != nil {
		return nil, err
	}
	if err := guardDate(ctx, src.Date); err != nil {
		return nil, err
	}
	target, err := to.key(ctx)
	if err != nil {
		return nil, err
	}
	occ, err := op.occupant(ctx)
	if err != nil {
		return nil, err
	}
	occupant, err := plateAt(ctx, svc, target.Date, target.SlotID)
	if err != nil {
		return nil, err
	}
	res, err := svc.Plates.MoveMeal(ctx, op.PlateID, target, occ)
	var oe *plate.OccupiedError
	if errors.As(err, &oe) && occupant != nil {
		return nil, fmt.Errorf("%w: %s slot %d holds plate %d (%s); pass occupant \"swap\" (it takes this plate's cell), \"drop\" (it is removed) or {date, slot_id} (it moves on to that empty cell)",
			err, to.Date, to.SlotID, oe.PlateID, plateNames(occupant))
	}
	if err != nil {
		return nil, err
	}
	for i := range res.Plates {
		b.plate(&res.Plates[i])
	}
	b.aff.Dates = append(b.aff.Dates, src.DateString(), to.Date)
	for i := range src.Components {
		b.aff.Sessions = append(b.aff.Sessions, sessionIDOf(&src.Components[i])...)
	}
	fromKey, err := slotName(ctx, svc, src.SlotID)
	if err != nil {
		return nil, err
	}
	toKey, err := slotName(ctx, svc, to.SlotID)
	if err != nil {
		return nil, err
	}
	line := map[string]any{"food": plateNames(src), "date": src.DateString(), "slot_key": fromKey, "to_date": to.Date, "to_slot_key": toKey}
	out := map[string]any{"op": op.Op, "plate_id": op.PlateID, "date": to.Date, "slot_id": to.SlotID, "from_date": src.DateString(), "from_slot_id": src.SlotID}
	if occupant != nil && occ.Kind != plate.OccupantNone {
		line["occupant"], line["other"] = string(occ.Kind), plateNames(occupant)
		out["occupant"], out["occupant_plate_id"] = string(occ.Kind), occupant.ID
		if occ.Kind == plate.OccupantTo {
			d := occ.To.Date.Format(time.DateOnly)
			out["occupant_to"] = map[string]any{"date": d, "slot_id": occ.To.SlotID}
			b.aff.Dates = append(b.aff.Dates, d)
		}
		for i := range occupant.Components {
			b.aff.Sessions = append(b.aff.Sessions, sessionIDOf(&occupant.Components[i])...)
		}
		if occ.Kind != plate.OccupantDrop && !slices.Contains(b.aff.Plates, occupant.ID) {
			b.aff.Plates = append(b.aff.Plates, occupant.ID)
		}
	}
	b.note(op, line)
	return mealResult(out, res, b), nil
}

func opReplace(ctx context.Context, svc Services, op changeOp, b *batch) (map[string]any, error) {
	if op.PlateID == 0 || op.FoodID == 0 {
		return nil, fmt.Errorf("%w: replace needs plate_id and food_id", domain.ErrInvalidInput)
	}
	if (op.Position == "") == (op.PlateComponentID == 0) {
		return nil, fmt.Errorf("%w: replace needs exactly one of position (main|side) or plate_component_id", domain.ErrInvalidInput)
	}
	p, err := svc.Plates.Get(ctx, op.PlateID)
	if err != nil {
		return nil, err
	}
	if err := guardDate(ctx, p.Date); err != nil {
		return nil, err
	}
	var src plate.Source
	src.SessionID = op.SessionID
	if op.CookAt != nil {
		k, err := cellRef{Date: op.CookAt.Date, SlotID: op.CookAt.SlotID}.key(ctx)
		if err != nil {
			return nil, err
		}
		src.CookAt = &k
	}
	var qty *plate.PlateComponent
	if q := op.quantity(); !q.empty() {
		c := q.component(op.FoodID)
		qty = &c
	}
	old := selectedComponent(p, op)
	res, err := svc.Plates.ReplaceComponent(ctx, op.PlateID, plate.Selector{Position: plate.Position(op.Position), ComponentID: op.PlateComponentID}, op.FoodID, src, qty)
	if err != nil {
		return nil, err
	}
	if len(res.Plates) == 0 {
		return nil, fmt.Errorf("replace: plate %d not returned", op.PlateID)
	}
	after := &res.Plates[0]
	pc := replacedComponent(after, op, old)
	if pc == nil {
		return nil, fmt.Errorf("replace: food %d not on plate %d", op.FoodID, op.PlateID)
	}
	b.plate(after)
	b.aff.Sessions = append(b.aff.Sessions, sessionIDOf(pc)...)
	oldName := ""
	if old != nil {
		oldName = old.FoodName
		b.aff.Sessions = append(b.aff.Sessions, sessionIDOf(old)...)
	}
	if src.CookAt != nil {
		b.aff.Dates = append(b.aff.Dates, op.CookAt.Date)
	}
	b.note(op, map[string]any{"food": oldName, "to": pc.FoodName, "date": after.DateString()})
	out := opResult(op, after, pc)
	out["position"] = string(pc.Position)
	return mealResult(out, res, b), nil
}

// selectedComponent is the component replace names (nil when the selector
// does not resolve; the plate service then says why).
func selectedComponent(p *plate.Plate, op changeOp) *plate.PlateComponent {
	var hit *plate.PlateComponent
	for i := range p.Components {
		c := &p.Components[i]
		switch {
		case op.PlateComponentID != 0 && c.ID == op.PlateComponentID:
			return c
		case op.PlateComponentID == 0 && string(c.Position) == op.Position:
			if hit != nil {
				return nil
			}
			hit = c
		}
	}
	return hit
}

// replacedComponent finds the new component: same id or same position as the
// old one, else the one with the new food.
func replacedComponent(p *plate.Plate, op changeOp, old *plate.PlateComponent) *plate.PlateComponent {
	for i := range p.Components {
		c := &p.Components[i]
		if c.FoodID == op.FoodID && (old == nil || c.ID == old.ID || c.Position == old.Position) {
			return c
		}
	}
	return nil
}

func opAway(ctx context.Context, svc Services, op changeOp, b *batch) (map[string]any, error) {
	if svc.Cells == nil {
		return nil, fmt.Errorf("%w: away is not available", domain.ErrInvalidInput)
	}
	in := availability.AwayInput{Note: op.Note, Today: todayFrom(ctx)}
	switch {
	case len(op.Cells) > 0 && op.Date == "":
		for _, c := range op.Cells {
			k, err := c.key(ctx)
			if err != nil {
				return nil, err
			}
			in.Cells = append(in.Cells, k)
		}
	case len(op.Cells) == 0 && op.Date != "":
		d, err := parseOpDate(op.Date)
		if err != nil {
			return nil, err
		}
		if err := guardDate(ctx, d); err != nil {
			return nil, err
		}
		in.Date = &d
	default:
		return nil, fmt.Errorf("%w: away needs cells [{date, slot_id}] or date (the whole day), not both", domain.ErrInvalidInput)
	}
	res, err := svc.Cells.SetAway(ctx, in)
	if err != nil {
		return nil, err
	}
	cells := make([]map[string]any, 0, len(res.Before))
	line := make([]map[string]any, 0, len(res.Before))
	for _, c := range res.Before {
		after, err := svc.Cells.Override(ctx, c.Date, c.SlotID)
		if err != nil {
			return nil, err
		}
		noteCell(ctx, c.Date, c.SlotID, c.Override, after)
		d := c.Date.Format(time.DateOnly)
		key, err := slotName(ctx, svc, c.SlotID)
		if err != nil {
			return nil, err
		}
		b.aff.Dates = append(b.aff.Dates, d)
		cells = append(cells, map[string]any{"date": d, "slot_id": c.SlotID})
		line = append(line, map[string]any{"date": d, "slot_key": key})
	}
	sort.SliceStable(line, func(i, j int) bool { return line[i]["date"].(string) < line[j]["date"].(string) })
	b.note(op, map[string]any{"cells": line, "date": firstDate(line)})
	out := map[string]any{"op": op.Op, "cells": cells, "removed_plates": len(res.Removed.Plates)}
	return mealResult(out, plate.MealResult{AlsoChanged: res.AlsoChanged}, b), nil
}

func firstDate(line []map[string]any) string {
	if len(line) == 0 {
		return ""
	}
	return line[0]["date"].(string)
}

func opMoveCook(ctx context.Context, svc Services, op changeOp, b *batch) (map[string]any, error) {
	if op.SessionID == 0 || op.CookAt == nil {
		return nil, fmt.Errorf("%w: move_cook needs session_id and cook_at {date, slot_id}", domain.ErrInvalidInput)
	}
	sess, err := svc.Plates.CookSession(ctx, op.SessionID)
	if err != nil {
		return nil, err
	}
	if err := guardDate(ctx, sess.CookDate); err != nil {
		return nil, err
	}
	k, err := cellRef{Date: op.CookAt.Date, SlotID: op.CookAt.SlotID}.key(ctx)
	if err != nil {
		return nil, err
	}
	res, err := svc.Plates.MoveCook(ctx, op.SessionID, k)
	if err != nil {
		return nil, err
	}
	from := sess.CookDate.Format(time.DateOnly)
	b.aff.Sessions = append(b.aff.Sessions, op.SessionID)
	b.aff.Dates = append(b.aff.Dates, from, op.CookAt.Date)
	name := ""
	if v, err := svc.Plates.Session(ctx, op.SessionID); err == nil {
		name = v.FoodName
	}
	b.note(op, map[string]any{"food": name, "date": op.CookAt.Date, "from_date": from})
	out := map[string]any{"op": op.Op, "session_id": op.SessionID, "cook_date": op.CookAt.Date, "cook_slot_id": op.CookAt.SlotID, "from_date": from, "from_slot_id": sess.CookSlotID}
	return mealResult(out, res, b), nil
}

func opCopy(ctx context.Context, svc Services, op changeOp, b *batch) (map[string]any, error) {
	if op.PlateID == 0 {
		return nil, fmt.Errorf("%w: copy needs plate_id", domain.ErrInvalidInput)
	}
	if op.As != string(plate.CopyLeftover) && op.As != string(plate.CopyFresh) {
		return nil, fmt.Errorf("%w: copy needs as \"leftover\" (eat from the same batch) or \"fresh\" (cook again)", domain.ErrInvalidInput)
	}
	refs, err := op.toCells()
	if err != nil {
		return nil, err
	}
	src, err := svc.Plates.Get(ctx, op.PlateID)
	if err != nil {
		return nil, err
	}
	cells := make([]plate.CellKey, len(refs))
	dates := make([]string, len(refs))
	for i, c := range refs {
		if cells[i], err = c.key(ctx); err != nil {
			return nil, err
		}
		dates[i] = c.Date
	}
	res, err := svc.Plates.CopyMeal(ctx, op.PlateID, cells, plate.CopyAs(op.As))
	if err != nil {
		return nil, err
	}
	copies := make([]map[string]any, 0, len(res.Plates))
	for i := range res.Plates {
		p := &res.Plates[i]
		if p.ID == src.ID {
			continue
		}
		b.plate(p)
		for j := range p.Components {
			b.aff.Sessions = append(b.aff.Sessions, sessionIDOf(&p.Components[j])...)
		}
		copies = append(copies, map[string]any{"plate_id": p.ID, "date": p.DateString(), "slot_id": p.SlotID})
	}
	b.note(op, map[string]any{"food": plateNames(src), "date": src.DateString(), "dates": dates, "as": op.As})
	out := map[string]any{"op": op.Op, "plate_id": op.PlateID, "as": op.As, "copies": copies}
	return mealResult(out, res, b), nil
}

// opFields are the fields each apply_changes op takes, for the error that
// names a field the op does not know.
var opFields = map[string]string{
	"add":              "food_id, plate_id or date + slot_id, portions or amount + unit, optional session_id, cook_at {date, slot_id}, position (main|side|extra)",
	"swap":             "plate_component_id, food_id, optional portions or amount + unit",
	"set_quantity":     "plate_component_id, portions or amount + unit",
	"remove":           "plate_component_id",
	"create_plate":     "date, slot_id, optional eaters, note",
	"delete_plate":     "plate_id",
	"set_portions":     "session_id, portions, optional frozen_portions",
	"set_eaters":       "plate_id, eaters",
	"set_availability": "date, slot_id, mode, optional preset_id, note",
	"move":             "plate_id, to {date, slot_id}, optional occupant (\"swap\" | \"drop\" | {date, slot_id})",
	"replace":          "plate_id, position (main|side) or plate_component_id, food_id, optional session_id, cook_at {date, slot_id}, portions or amount + unit",
	"away":             "cells [{date, slot_id}] or date, optional note",
	"move_cook":        "session_id, cook_at {date, slot_id}",
	"copy":             "plate_id, to [{date, slot_id}], as (leftover|fresh)",
}

// opKeys are the JSON keys of each op (besides "op").
var opKeys = map[string][]string{
	"add":              {"food_id", "plate_id", "date", "slot_id", "portions", "amount", "unit", "session_id", "cook_at", "position"},
	"swap":             {"plate_component_id", "food_id", "portions", "amount", "unit"},
	"set_quantity":     {"plate_component_id", "portions", "amount", "unit"},
	"remove":           {"plate_component_id"},
	"create_plate":     {"date", "slot_id", "eaters", "note"},
	"delete_plate":     {"plate_id"},
	"set_portions":     {"session_id", "portions", "frozen_portions"},
	"set_eaters":       {"plate_id", "eaters"},
	"set_availability": {"date", "slot_id", "mode", "preset_id", "note"},
	"move":             {"plate_id", "to", "occupant"},
	"replace":          {"plate_id", "position", "plate_component_id", "food_id", "session_id", "cook_at", "portions", "amount", "unit"},
	"away":             {"cells", "date", "note"},
	"move_cook":        {"session_id", "cook_at"},
	"copy":             {"plate_id", "to", "as"},
}

// applyChangesHint explains a schema refusal per op: each field an op does
// not take, with the fields it does take, so the model can retry once.
func applyChangesHint(input json.RawMessage) string {
	var in struct {
		Ops []map[string]json.RawMessage `json:"ops"`
	}
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	var hints []string
	for i, o := range in.Ops {
		var name string
		_ = json.Unmarshal(o["op"], &name)
		keys, ok := opKeys[name]
		if !ok {
			continue
		}
		var bad []string
		for k := range o {
			if k != "op" && !slices.Contains(keys, k) {
				bad = append(bad, k)
			}
		}
		if len(bad) == 0 {
			continue
		}
		slices.Sort(bad)
		hints = append(hints, fmt.Sprintf("op %d (%s): unknown field %s; %s takes %s", i+1, name, quoteAll(bad), name, opFields[name]))
	}
	return strings.Join(hints, "; ")
}

func quoteAll(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(q, ", ")
}
