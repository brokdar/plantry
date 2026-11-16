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
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

const applyChangesName = "apply_changes"

// errDryRun ends a dry run's transaction so everything it wrote rolls back.
var errDryRun = errors.New("dry run")

const applyChangesSchema = `{
  "type":"object",
  "required":["ops"],
  "properties":{
    "dry_run":{"type":"boolean","description":"true: run every op, report the result and roll everything back (nothing is saved). Use it first for multi-step changes."},
    "ops":{
      "type":"array","minItems":1,"maxItems":40,
      "items":{
        "type":"object",
        "required":["op"],
        "properties":{
          "op":{"type":"string","enum":["add","swap","set_quantity","remove","create_plate","delete_plate","set_portions","set_eaters","set_availability"]},
          "plate_id":{"type":"integer","minimum":1},
          "plate_component_id":{"type":"integer","minimum":1},
          "food_id":{"type":"integer","minimum":1},
          "date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
          "slot_id":{"type":"integer","minimum":1},
          "portions":{"type":"number","exclusiveMinimum":0,"maximum":100},
          "amount":{"type":"number","exclusiveMinimum":0},
          "unit":{"type":"string","minLength":1},
          "session_id":{"type":"integer","minimum":1},
          "cook_at":{"type":"object","required":["date","slot_id"],"properties":{"date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},"slot_id":{"type":"integer","minimum":1}},"additionalProperties":false},
          "frozen_portions":{"type":"number","minimum":0,"maximum":100},
          "eaters":{"type":"number","exclusiveMinimum":0,"maximum":100},
          "mode":{"type":"string","enum":["plan","none","routine","default"]},
          "preset_id":{"type":"integer","minimum":1},
          "note":{"type":"string","maxLength":200}
        },
        "additionalProperties":false
      }
    }
  },
  "additionalProperties":false
}`

const applyChangesDescription = `Change the plan in one call: a list of ops that run in order in ONE transaction (any failing op rolls everything back and names it, numbered from 1). With dry_run true nothing is saved: you get the same result so you can check the rule effects first. Quantities are per eater: a recipe takes portions (usually 1), a leaf takes amount + unit (e.g. 100 + "g") and rejects portions.
Ops (fields):
- add: food_id, the plate by plate_id OR by date + slot_id (the plate is created when the cell is empty), a quantity (portions | amount+unit). Optional session_id: a recipe component eats from that existing cook session (leftover, no new cooking). Optional cook_at {date, slot_id}: a recipe is cooked on that other cell (a new session cooking there, e.g. with the lunch batch) and the plate eats it (portions = eaters x portions).
- swap: plate_component_id, food_id (the new food), optional quantity (else the current one is kept).
- set_quantity: plate_component_id + quantity.
- remove: plate_component_id.
- create_plate: date, slot_id, optional eaters, note.
- delete_plate: plate_id.
- set_portions: session_id, portions (total the batch cooks), optional frozen_portions. Refused below what the plates eat.
- set_eaters: plate_id, eaters (the batches it eats from are resized).
- set_availability: date, slot_id, mode plan|none|routine|default, preset_id (routine), note (none). Only one per call.
Only draft plates in the future are changeable; a confirmed one needs the user to unlock it.
Returns JSON {dry_run, applied:[{op, plate_id?, plate_component_id?, session_id?, ...}], after:{plates:[{plate_id,date,slot_id,state,eaters,components:[names],kcal,protein_g (per eater),component_details:[{plate_component_id,food_id,name,role,portions?|grams?,kcal,protein_g}]}], sessions:[{session_id,food,cook_date,portions,frozen_portions,consumed,frozen_consumed,surplus,state,cooked,warnings}], days:[{date,kcal,target_kcal,protein_g,protein_pct,fat_pct,carbs_pct,meat,meat_foods,cook_occasions,complete}] (the days the ops touched, a cook_at op's cook day included, also in other plan windows), meat_days, meat_days_max, issues_checked (false with issues_unchecked_reason when a touched date lies outside the evaluated plan windows), issues:{new:[issue],resolved:[issue],total_open,new_more?,resolved_more?}, change_id? (not on dry runs)}}. An issue is {rule, code, severity violation|warning|soft, date, slot_id?, plate_id?, session_id?, params, options?:[{action add|raise, food_id, name, date, slot_id, plate_id, portions|grams, from?, kcal_delta, protein_delta_g, day_kcal, day_protein_pct, cook_occasions, new_meat_day, op}]} where op is an apply_changes op you can pass as it is (add with plate_id or date+slot_id and portions or amount+unit, or set_quantity with plate_component_id). An S3 option raises the day's protein share; an S2 option moves the day closer to its target without leaving the band; options that bring the day into the band come first. Violations (H1-H8) are hard rules: fix them. Soft issues (S2 day kcal outside 90-110% of target, S3 protein share under target) carry options you can apply with another call. Your request is not done while an issue it caused is open, unless you tell the user why.`

// changeOp is one op of apply_changes.
type changeOp struct {
	Op               string   `json:"op"`
	PlateID          int64    `json:"plate_id"`
	PlateComponentID int64    `json:"plate_component_id"`
	FoodID           int64    `json:"food_id"`
	Date             string   `json:"date"`
	SlotID           int64    `json:"slot_id"`
	Portions         *float64 `json:"portions"`
	Amount           *float64 `json:"amount"`
	Unit             *string  `json:"unit"`
	SessionID        int64    `json:"session_id"`
	CookAt           *struct {
		Date   string `json:"date"`
		SlotID int64  `json:"slot_id"`
	} `json:"cook_at"`
	FrozenPortions *float64 `json:"frozen_portions"`
	Eaters         *float64 `json:"eaters"`
	CellMode       string   `json:"mode"`
	PresetID       *int64   `json:"preset_id"`
	Note           *string  `json:"note"`
}

func (o changeOp) quantity() quantityInput {
	return quantityInput{Portions: o.Portions, Amount: o.Amount, Unit: o.Unit}
}

// batch collects what a call's ops touched.
type batch struct {
	aff affected
	// ops is one entry per op for the change line's params (the frontend
	// writes the sentence): op, food, to, date, slot_id, slot_key, mode,
	// portions, amount, unit, eaters.
	ops []map[string]any
}

// note records op's line entry; quantity fields are added when the op sets them.
func (b *batch) note(op changeOp, kv map[string]any) {
	kv["op"] = op.Op
	if op.Portions != nil {
		kv["portions"] = *op.Portions
	}
	if op.Amount != nil {
		kv["amount"] = *op.Amount
		if op.Unit != nil {
			kv["unit"] = *op.Unit
		}
	}
	b.ops = append(b.ops, kv)
}

func (b *batch) plate(p *plate.Plate) {
	if !slices.Contains(b.aff.Plates, p.ID) {
		b.aff.Plates = append(b.aff.Plates, p.ID)
	}
	b.aff.Dates = append(b.aff.Dates, p.DateString())
}

func toolApplyChanges(svc Services) Tool {
	return Tool{
		Name:        applyChangesName,
		Description: applyChangesDescription,
		Schema:      json.RawMessage(applyChangesSchema),
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Ops    []changeOp `json:"ops"`
				DryRun bool       `json:"dry_run"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			if llm.CallerFrom(ctx).FromCode() && !in.DryRun {
				return nil, ToolEffectNone, fmt.Errorf("%w: apply_changes called from code must set dry_run: true; call it directly (not from code) to save the change", ErrCodeCallRefused)
			}
			if svc.Tx == nil || svc.State == nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: apply_changes is not available (no unit of work or plan reader)", domain.ErrInvalidInput)
			}
			if n := countOp(in.Ops, "set_availability"); n > 1 {
				return nil, ToolEffectNone, fmt.Errorf("%w: at most one set_availability op per call (undo restores one cell)", domain.ErrInvalidInput)
			}
			w, ok := windowFrom(ctx)
			if !ok {
				return nil, ToolEffectNone, fmt.Errorf("%w: no plan window in this conversation turn", domain.ErrInvalidInput)
			}
			ws := windowsFor(w, opDates(in.Ops))
			before, err := evalWindows(ctx, svc, ws)
			if err != nil {
				return nil, ToolEffectNone, fmt.Errorf("read the plan before the change: %w", err)
			}
			var (
				applied []map[string]any
				after   map[string]any
				afterEv planning.Evaluation
				b       batch
			)
			run := func(ctx context.Context) error {
				for i, op := range in.Ops {
					res, err := applyOp(ctx, svc, op, &b)
					if err != nil {
						return fmt.Errorf("op %d of %d (%s): %w", i+1, len(in.Ops), op.Op, err)
					}
					applied = append(applied, res)
				}
				var err error
				after, afterEv, err = batchAfter(ctx, svc, ws, b, before)
				return err
			}
			err = svc.Tx.InTx(ctx, func(ctx context.Context) error {
				if err := run(ctx); err != nil {
					return err
				}
				if in.DryRun {
					return errDryRun
				}
				return nil
			})
			if err != nil && !errors.Is(err, errDryRun) {
				return nil, ToolEffectNone, err
			}
			if in.DryRun {
				// Rolled back: the dry run is a step of the run log too.
				planning.RecordEvaluation(ctx, planning.PhaseDryRun, func() planning.Evaluation { return afterEv })
			} else {
				noteChange(ctx, "apply_changes", map[string]any{"count": len(in.Ops), "ops": b.ops})
			}
			return mustJSON(map[string]any{"dry_run": in.DryRun, "applied": applied, "after": after}), ToolEffectNone, nil
		},
	}
}

func countOp(ops []changeOp, name string) int {
	n := 0
	for _, o := range ops {
		if o.Op == name {
			n++
		}
	}
	return n
}

// applyOp runs one op through plate.Service (and the availability writer).
func applyOp(ctx context.Context, svc Services, op changeOp, b *batch) (map[string]any, error) {
	switch op.Op {
	case "add":
		return opAdd(ctx, svc, op, b)
	case "swap":
		if op.FoodID == 0 {
			return nil, fmt.Errorf("%w: swap needs food_id (the new food)", domain.ErrInvalidInput)
		}
		_, old, err := componentOnPlate(ctx, svc, op.PlateComponentID)
		if err != nil {
			return nil, err
		}
		var quantity *plate.PlateComponent
		if q := op.quantity(); !q.empty() {
			c := q.component(op.FoodID)
			quantity = &c
		}
		p, err := svc.Plates.SwapComponent(ctx, op.PlateComponentID, op.FoodID, quantity)
		if err != nil {
			return nil, err
		}
		pc, err := componentByID(p, op.PlateComponentID)
		if err != nil {
			return nil, err
		}
		b.plate(p)
		b.aff.Sessions = append(b.aff.Sessions, sessionIDOf(old)...)
		b.note(op, map[string]any{"food": old.FoodName, "to": pc.FoodName, "date": p.DateString()})
		return opResult(op, p, pc), nil
	case "set_quantity":
		q := op.quantity()
		if q.empty() {
			return nil, fmt.Errorf("%w: portions or amount + unit required", domain.ErrInvalidInput)
		}
		if err := guardComponentDate(ctx, svc, op.PlateComponentID); err != nil {
			return nil, err
		}
		p, err := svc.Plates.UpdateComponentQuantity(ctx, op.PlateComponentID, q.component(0))
		if err != nil {
			return nil, err
		}
		pc, err := componentByID(p, op.PlateComponentID)
		if err != nil {
			return nil, err
		}
		b.plate(p)
		b.note(op, map[string]any{"food": pc.FoodName, "date": p.DateString()})
		return opResult(op, p, pc), nil
	case "remove":
		p, pc, err := componentOnPlate(ctx, svc, op.PlateComponentID)
		if err != nil {
			return nil, err
		}
		if _, err := svc.Plates.RemoveComponent(ctx, op.PlateComponentID); err != nil {
			return nil, err
		}
		b.plate(p)
		b.aff.Sessions = append(b.aff.Sessions, sessionIDOf(pc)...)
		b.note(op, map[string]any{"food": pc.FoodName, "date": p.DateString()})
		return map[string]any{"op": op.Op, "plate_component_id": op.PlateComponentID, "removed": true}, nil
	case "create_plate":
		d, err := parseOpDate(op.Date)
		if err != nil {
			return nil, err
		}
		if err := guardDate(ctx, d); err != nil {
			return nil, err
		}
		p, err := svc.Plates.Create(ctx, plate.PlateInput{Date: d, SlotID: op.SlotID, Eaters: op.Eaters, Note: op.Note})
		if err != nil {
			return nil, err
		}
		b.plate(p)
		b.note(op, map[string]any{"date": p.DateString(), "slot_id": p.SlotID})
		return map[string]any{"op": op.Op, "plate_id": p.ID, "date": p.DateString(), "slot_id": p.SlotID, "eaters": p.Eaters}, nil
	case "delete_plate":
		p, err := svc.Plates.Get(ctx, op.PlateID)
		if err != nil {
			return nil, err
		}
		if err := guardDate(ctx, p.Date); err != nil {
			return nil, err
		}
		if _, err := svc.Plates.Delete(ctx, op.PlateID); err != nil {
			return nil, err
		}
		b.aff.Dates = append(b.aff.Dates, p.DateString())
		for i := range p.Components {
			b.aff.Sessions = append(b.aff.Sessions, sessionIDOf(&p.Components[i])...)
		}
		b.note(op, map[string]any{"food": plateNames(p), "date": p.DateString()})
		return map[string]any{"op": op.Op, "plate_id": op.PlateID, "deleted": true}, nil
	case "set_portions":
		return opSetPortions(ctx, svc, op, b)
	case "set_eaters":
		if op.Eaters == nil {
			return nil, fmt.Errorf("%w: set_eaters needs eaters", domain.ErrInvalidInput)
		}
		if err := guardPlateDate(ctx, svc, op.PlateID); err != nil {
			return nil, err
		}
		p, err := svc.Plates.Update(ctx, op.PlateID, plate.PlatePatch{Eaters: op.Eaters})
		if err != nil {
			return nil, err
		}
		b.plate(p)
		b.note(op, map[string]any{"food": plateNames(p), "date": p.DateString(), "eaters": p.Eaters})
		return map[string]any{"op": op.Op, "plate_id": p.ID, "eaters": p.Eaters}, nil
	case "set_availability":
		return opSetAvailability(ctx, svc, op, b)
	}
	return nil, fmt.Errorf("%w: unknown op %q", domain.ErrInvalidInput, op.Op)
}

func parseOpDate(s string) (time.Time, error) {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: date: %v", domain.ErrInvalidInput, err)
	}
	return d, nil
}

func opResult(op changeOp, p *plate.Plate, pc *plate.PlateComponent) map[string]any {
	out := plateComponentMap(pc)
	out["op"] = op.Op
	out["plate_id"] = p.ID
	out["plate_component_id"] = pc.ID
	return out
}

// plateAt returns the plate on (date, slot), nil when the cell is empty.
func plateAt(ctx context.Context, svc Services, d time.Time, slotID int64) (*plate.Plate, error) {
	plates, err := svc.Plates.Range(ctx, d, d)
	if err != nil {
		return nil, err
	}
	for i := range plates {
		if plates[i].SlotID == slotID {
			return &plates[i], nil
		}
	}
	return nil, nil
}

func opAdd(ctx context.Context, svc Services, op changeOp, b *batch) (map[string]any, error) {
	if op.FoodID == 0 {
		return nil, fmt.Errorf("%w: add needs food_id", domain.ErrInvalidInput)
	}
	var p *plate.Plate
	switch {
	case op.PlateID != 0:
		if err := guardPlateDate(ctx, svc, op.PlateID); err != nil {
			return nil, err
		}
		got, err := svc.Plates.Get(ctx, op.PlateID)
		if err != nil {
			return nil, err
		}
		p = got
	case op.Date != "" && op.SlotID != 0:
		d, err := parseOpDate(op.Date)
		if err != nil {
			return nil, err
		}
		if err := guardDate(ctx, d); err != nil {
			return nil, err
		}
		if p, err = plateAt(ctx, svc, d, op.SlotID); err != nil {
			return nil, err
		}
		if p == nil {
			if p, err = svc.Plates.Create(ctx, plate.PlateInput{Date: d, SlotID: op.SlotID}); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("%w: add needs plate_id or date + slot_id", domain.ErrInvalidInput)
	}
	comp := op.quantity().component(op.FoodID)
	if op.SessionID != 0 {
		sid := op.SessionID
		comp.CookSessionID = &sid
	}
	var got *plate.Plate
	var err error
	if op.CookAt != nil && op.SessionID == 0 && (op.CookAt.Date != p.DateString() || op.CookAt.SlotID != p.SlotID) {
		got, err = addCookedElsewhere(ctx, svc, p, comp, op)
	} else {
		got, err = svc.Plates.AddComponent(ctx, p.ID, comp)
	}
	if err != nil {
		return nil, err
	}
	added := lastComponent(got)
	b.plate(got)
	if op.CookAt != nil && op.SessionID == 0 {
		b.aff.Dates = append(b.aff.Dates, op.CookAt.Date)
	}
	b.aff.Sessions = append(b.aff.Sessions, sessionIDOf(added)...)
	b.note(op, map[string]any{"food": added.FoodName, "date": got.DateString()})
	return opResult(op, got, added), nil
}

// addCookedElsewhere adds a recipe to p that is cooked on another cell: one
// snapshot with a session cooking at op.CookAt and a component of p eating
// it (plate.Service.Apply merges into p). The session cooks eaters x
// portions.
func addCookedElsewhere(ctx context.Context, svc Services, p *plate.Plate, comp plate.PlateComponent, op changeOp) (*plate.Plate, error) {
	cd, err := parseOpDate(op.CookAt.Date)
	if err != nil {
		return nil, err
	}
	if err := guardDate(ctx, cd); err != nil {
		return nil, err
	}
	f, err := svc.Foods.Get(ctx, op.FoodID)
	if err != nil {
		return nil, err
	}
	if f.Kind != food.KindComposed {
		return nil, fmt.Errorf("%w: cook_at needs a recipe, %s is a leaf food (eat it without cooking)", domain.ErrInvalidInput, f.Name)
	}
	portions := 1.0
	if comp.Portions != nil {
		portions = *comp.Portions
	}
	id := p.ID
	snap := plate.PlanSnapshot{
		Sessions: []plate.SnapshotSession{{
			Ref: "c1", FoodID: op.FoodID, CookDate: cd.Format(time.DateOnly), CookSlotID: op.CookAt.SlotID, Portions: p.Eaters * portions,
		}},
		Plates: []plate.SnapshotPlate{{
			PlateID: &id, Date: p.DateString(), SlotID: p.SlotID, Eaters: p.Eaters,
			Components: []plate.SnapshotComponent{{FoodID: op.FoodID, Portions: &portions, SortOrder: len(p.Components), Session: &plate.SessionLink{Ref: "c1"}}},
		}},
	}
	if _, err := svc.Plates.Apply(ctx, snap, plate.ApplyOptions{OnConflict: plate.ConflictFail}); err != nil {
		return nil, err
	}
	return svc.Plates.Get(ctx, p.ID)
}

func opSetPortions(ctx context.Context, svc Services, op changeOp, b *batch) (map[string]any, error) {
	if op.Portions == nil {
		return nil, fmt.Errorf("%w: set_portions needs portions", domain.ErrInvalidInput)
	}
	sess, err := svc.Plates.CookSession(ctx, op.SessionID)
	if err != nil {
		return nil, err
	}
	if err := guardDate(ctx, sess.CookDate); err != nil {
		return nil, err
	}
	frozen := sess.FrozenPortions
	if op.FrozenPortions != nil {
		frozen = *op.FrozenPortions
	}
	// Deficit guard (spec §3.4, H3): a batch must cover what its plates eat.
	cur, err := svc.Plates.Session(ctx, op.SessionID)
	if err != nil {
		return nil, err
	}
	if plate.Exceeds(cur.Consumed, *op.Portions) || plate.Exceeds(cur.FrozenConsumed, frozen) {
		return nil, fmt.Errorf("%w: %s would cook %s portions (%s frozen) but its plates eat %s (%s from the freezer); "+
			"set at least %s portions (%s frozen), or reduce the plates' eaters or portions first",
			domain.ErrInvalidInput, cur.FoodName, fnum(*op.Portions), fnum(frozen), fnum(cur.Consumed), fnum(cur.FrozenConsumed), fnum(cur.Consumed), fnum(cur.FrozenConsumed))
	}
	v, err := svc.Plates.SetSessionPortions(ctx, op.SessionID, plate.SessionPortions{Portions: *op.Portions, FrozenPortions: frozen, Note: sess.Note})
	if err != nil {
		return nil, err
	}
	b.aff.Sessions = append(b.aff.Sessions, v.ID)
	b.aff.Dates = append(b.aff.Dates, v.CookDate.Format(time.DateOnly))
	b.note(op, map[string]any{"food": v.FoodName, "date": v.CookDate.Format(time.DateOnly), "portions": v.Portions})
	return map[string]any{"op": op.Op, "session_id": v.ID, "portions": v.Portions, "frozen_portions": v.FrozenPortions}, nil
}

func opSetAvailability(ctx context.Context, svc Services, op changeOp, b *batch) (map[string]any, error) {
	if svc.Cells == nil {
		return nil, fmt.Errorf("%w: set_availability is not available", domain.ErrInvalidInput)
	}
	d, err := parseOpDate(op.Date)
	if err != nil {
		return nil, err
	}
	if err := guardDate(ctx, d); err != nil {
		return nil, err
	}
	if op.CellMode == "" || op.SlotID == 0 {
		return nil, fmt.Errorf("%w: set_availability needs date, slot_id and mode", domain.ErrInvalidInput)
	}
	before, err := svc.Cells.Override(ctx, d, op.SlotID)
	if err != nil {
		return nil, err
	}
	slotKey, err := slotName(ctx, svc, op.SlotID)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"op": op.Op, "date": op.Date, "slot_id": op.SlotID, "mode": op.CellMode, "removed_plate": false}
	if op.CellMode == availabilityDefault {
		if err := svc.Cells.ClearOverride(ctx, d, op.SlotID); err != nil {
			return nil, err
		}
	} else {
		mode := availability.Mode(op.CellMode)
		_, removed, err := svc.Cells.SetOverride(ctx, availability.OverrideInput{
			Date: d, SlotID: op.SlotID, Mode: mode, PresetID: op.PresetID, Note: op.Note,
			RemovePlate: mode == availability.ModeNone,
		})
		if err != nil {
			return nil, err
		}
		out["removed_plate"] = removed != nil
	}
	b.aff.Dates = append(b.aff.Dates, op.Date)
	b.note(op, map[string]any{"slot_key": slotKey, "slot_id": op.SlotID, "mode": op.CellMode, "date": op.Date})
	after, err := svc.Cells.Override(ctx, d, op.SlotID)
	if err != nil {
		return nil, err
	}
	noteCell(ctx, d, op.SlotID, before, after)
	return out, nil
}

// evalWindow evaluates the chat window as the model and user see it.
func evalWindow(ctx context.Context, svc Services, w window.Window) (planning.Evaluation, error) {
	st, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return planning.Evaluation{}, err
	}
	return chatView(st), nil
}

// batchAfter is the combined `after` of a call: the touched plates and
// sessions, the evaluation's days for the touched dates and the issues the
// ops made new or resolved against before.
func batchAfter(ctx context.Context, svc Services, ws []window.Window, b batch, before planning.Evaluation) (map[string]any, planning.Evaluation, error) {
	now, err := evalWindows(ctx, svc, ws)
	if err != nil {
		return nil, planning.Evaluation{}, fmt.Errorf("read the plan after the change: %w", err)
	}
	reader := svc
	reader.PlanView = nil // the evaluation, not the plan read model, supplies the issues
	out := changeEffect(ctx, reader, b.aff)
	dates := map[string]bool{}
	for _, d := range b.aff.Dates {
		dates[d] = true
	}
	if plates, ok := out["plates"].([]effectPlate); ok {
		for i, p := range plates {
			dates[p.Date] = true
			for _, pe := range now.Plates {
				if pe.PlateID == p.ID {
					kcal, prot := pe.Kcal, pe.ProteinG
					plates[i].Kcal, plates[i].ProteinG, plates[i].Details = &kcal, &prot, pe.Components
				}
			}
		}
	}
	days := []planning.DayEval{}
	for _, d := range now.Days {
		if dates[d.Date] {
			days = append(days, d)
		}
	}
	out["days"] = days
	out["meat_days"], out["meat_days_max"] = now.MeatDays, now.MeatDaysMax
	out["issues_checked"] = true
	delete(out, "issues_unchecked_reason")
	// A touched date outside every evaluated window (a plate_id or session_id
	// op that reaches past the chat window) has no figures: say so.
	var outside []string
	for d := range dates {
		if !inWindows(ws, d) {
			outside = append(outside, d)
		}
	}
	if len(outside) > 0 {
		slices.Sort(outside)
		out["issues_checked"] = false
		out["issues_unchecked_reason"] = "dates outside the evaluated windows: " + strings.Join(outside, ", ")
	}
	out["issues"] = deltaOf(issueMap(before.Issues), issueMap(now.Issues))
	return out, now, nil
}

// opDates lists the dates the ops name (date, cook_at.date).
func opDates(ops []changeOp) []string {
	var out []string
	for _, o := range ops {
		if o.Date != "" {
			out = append(out, o.Date)
		}
		if o.CookAt != nil && o.CookAt.Date != "" {
			out = append(out, o.CookAt.Date)
		}
	}
	return out
}

// windowsFor is w plus the 7-day windows (same shopping day) that contain
// the given dates, each once.
func windowsFor(w window.Window, dates []string) []window.Window {
	ws := []window.Window{w}
	for _, s := range dates {
		d, err := time.Parse(time.DateOnly, s)
		if err != nil || w.Contains(d) {
			continue
		}
		days := int(d.Sub(w.Start).Hours() / 24)
		k := days / 7
		if days < 0 && days%7 != 0 {
			k--
		}
		start := w.Start.AddDate(0, 0, 7*k)
		if !slices.ContainsFunc(ws, func(x window.Window) bool { return x.Start.Equal(start) }) {
			ws = append(ws, window.Window{Start: start, End: start.AddDate(0, 0, 6)})
		}
	}
	return ws
}

func inWindows(ws []window.Window, date string) bool {
	d, err := time.Parse(time.DateOnly, date)
	return err == nil && slices.ContainsFunc(ws, func(w window.Window) bool { return w.Contains(d) })
}

// evalWindows evaluates the chat window and every extra window: days and
// issues are the union, the meat figures are the chat window's.
func evalWindows(ctx context.Context, svc Services, ws []window.Window) (planning.Evaluation, error) {
	ev, err := evalWindow(ctx, svc, ws[0])
	if err != nil {
		return ev, err
	}
	seen := issueMap(ev.Issues)
	for _, w := range ws[1:] {
		more, err := evalWindow(ctx, svc, w)
		if err != nil {
			return ev, err
		}
		ev.Days = append(ev.Days, more.Days...)
		ev.Plates = append(ev.Plates, more.Plates...)
		for _, is := range more.Issues {
			if _, dup := seen[issueKey(is)]; !dup {
				ev.Issues = append(ev.Issues, is)
				seen[issueKey(is)] = is
			}
		}
	}
	return ev, nil
}

func issueMap(issues []planning.Issue) map[string]planning.Issue {
	m := make(map[string]planning.Issue, len(issues))
	for _, is := range issues {
		m[issueKey(is)] = is
	}
	return m
}
