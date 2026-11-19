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

var applyChangesSchema = `{
  "type":"object",
  "properties":{
    "goal":{"type":"object","required":["kind"],"description":"the relative request this call serves (lighter, heartier, more_protein) with its target; take it as find_ways returned it","properties":{"kind":{"type":"string","enum":["lighter","heartier","more_protein","lower_day","raise_day"]},"plate_id":{"type":"integer","minimum":1},"date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"}},"additionalProperties":false},
    "dry_run":{"type":"boolean","description":"true: run every op, report the result and roll everything back (nothing is saved). Only for a what-if question: an explicit request is saved at once (saved says whether it was)."},
    ` + lightSchema + `,
    "ops":{
      "type":"array","maxItems":40,
      "items":{
        "type":"object",
        "required":["op"],
        "properties":{
          "op":{"type":"string","enum":["add","swap","set_quantity","remove","create_plate","delete_plate","set_portions","set_eaters","set_availability","move","replace","away","move_cook","copy"]},
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
          "eaters":{"type":"number","exclusiveMinimum":0,"maximum":100,"description":"the total number of people eating the plate, guests included"},
          "mode":{"type":"string","enum":["plan","none","routine","default"]},
          "preset_id":{"type":"integer","minimum":1},
          "note":{"type":"string","maxLength":200},
          "position":{"type":"string","enum":["main","side","extra"]},
          "to":{"type":["object","array"],"description":"move: one cell {date, slot_id}; copy: a list of cells [{date, slot_id}]"},
          "occupant":{"type":["string","object"],"description":"move onto a taken cell: \"swap\", \"drop\" or {date, slot_id}"},
          "cells":{"type":"array","minItems":1,"maxItems":21,"items":{"type":"object","required":["date","slot_id"],"properties":{"date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},"slot_id":{"type":"integer","minimum":1}},"additionalProperties":false}},
          "as":{"type":"string","enum":["leftover","fresh"]}
        },
        "additionalProperties":false
      }
    }
  },
  "additionalProperties":false
}`

const applyChangesDescription = `Change the plan in one call: a list of ops that run in order in ONE transaction (any failing op rolls everything back and names it, numbered from 1). With dry_run true nothing is saved: you get the same result (saved is false): use it only for a what-if question; an explicit request is saved at once with dry_run false, no check first. Rows a checked change creates do not exist afterwards: their ids are left out everywhere (applied, after, issues and an option's op) and marked "provisional": true, an option whose op uses one is marked too, so never use them in a later call; to apply, send the full op list again with dry_run false (the result then has the real ids; a checked change says so in dry_run_note). A field an op does not take is refused (the error names the fields it takes), except plate_id, plate_component_id, food_id, date, slot_id, note and eaters: those are ignored and listed in that op's applied entry as ignored_fields (the op did not use them). Quantities are per eater: a recipe takes portions (usually 1), a leaf takes amount + unit (e.g. 100 + "g") and rejects portions.
Ops (fields):
- add: food_id, the plate by plate_id OR by date + slot_id (the plate is created when the cell is empty), portions | amount+unit. Optional position main|side|extra (else derived). Optional session_id: a recipe component eats from that existing cook session (leftover, no new cooking). Optional cook_at {date, slot_id}: a recipe is cooked on that other cell (a new session cooking there, e.g. with the lunch batch) and the plate eats it (portions = eaters x portions).
- swap: plate_component_id, food_id (the new food), optional portions | amount+unit (else the current one is kept), optional session_id (eat from that batch) or cook_at {date, slot_id} (then it is a replace).
- set_quantity: plate_component_id + portions | amount+unit.
- remove: plate_component_id.
- create_plate: date, slot_id, optional eaters, note.
- delete_plate: plate_id.
- set_portions: session_id, portions (total the batch cooks), optional frozen_portions. Refused below what the plates eat.
- set_eaters: plate_id, eaters = how many people eat that plate in total, guests included (a friend joins a plate of 1: eaters 2, do it at once). Each person's portion stays the same; the batches it eats from are resized to cover them, and nothing else changes.
- set_availability: date, slot_id, mode plan|none|routine|default, preset_id (routine), note (none). none removes the cell's plate and moves a cook off it.
- move: plate_id, to {date, slot_id}. A taken target needs occupant: "swap" (the other plate takes this plate's cell), "drop" (the other plate is removed) or {date, slot_id} (it moves on to that empty cell); without one the op fails naming the plate there. Batches follow (the cook moves with its meal or to the earliest meal still eating it).
- replace: plate_component_id (the plate is known from it), or plate_id + position main|side, and food_id (the new food); optional session_id (eat from that batch), cook_at {date, slot_id} (cook it there), portions | amount+unit (else the current quantity is kept). Keeps the position.
- away: cells [{date, slot_id}] OR date (every planned meal of that day) OR date + slot_id (that one cell), optional note. Marks the cells "no plate", removes their plates and moves cooks off them. A day away is not judged for calories.
- move_cook: session_id, cook_at {date, slot_id}: the batch cooks there instead (on or before every meal eating it, within keep days).
- copy: plate_id, to [{date, slot_id}], as leftover (eat from the same batches) | fresh (cook again on each cell).
Only draft plates in the future are changeable; a confirmed one needs the user to unlock it.
Returns compact JSON (numbers rounded: kcal whole, other figures one decimal; a field that is 0, false or empty is left out, so absent means 0 / false / none; complete shows only when false) {dry_run, dry_run_note? (checks only), applied:[{op, plate_id?, plate_component_id?, session_id?, ..., also_changed?:[{what cook_moved|became_cooking_meal|cooks_fresh|batch_resized, session_id, plate_id, date, slot_id, from_date, from_slot_id, why?}]}] (move: date, slot_id, from_date, from_slot_id, occupant?, occupant_plate_id?; away: cells, removed_plates; move_cook: cook_date, cook_slot_id; copy: copies [{plate_id, date, slot_id}]; also_changed lists what else the op changed, tell the user in plain words), after:{plates:[{plate_id,date,slot_id,state,eaters,components:[names],kcal,protein_g (per eater),intent? (light|hearty) with max_kcal? (the most a light meal holds) or min_kcal? (the least a filling meal holds),component_details (only these touched plates):[{plate_component_id,name,portions?|grams?,kcal,protein_g,nutrition_complete? (only false)}]}], sessions:[{session_id,food,cook_date,portions,frozen_portions,consumed,frozen_consumed,surplus,state,cooked,warnings}], days:[{date,kcal,target_kcal,band_low,band_high (the day band, kcal),floor_kcal (the day energy minimum),room_kcal (kcal left to the band top),protein_g,protein_pct,fat_pct,carbs_pct,meat,meat_foods,cook_occasions,light_meals? (e.g. "light dinner"),complete}] (the days the ops touched, a cook_at op's cook day included, also in other plan windows), meat_days, meat_days_max, issues_checked (false with issues_unchecked_reason when a touched date lies outside the evaluated plan windows), issues:{new:[issue],resolved:[issue],worsened:[{label,message,severity,date?,slot_id?,plate_id?,before:{params},after:{params}}] (open before and after, the numbers got worse, e.g. meat 321 to 471 g),total_open,new_more?,resolved_more?,worsened_more?}, change_id? (not when dry_run is true)}}. An issue is {label (the rule in plain words), message (one plain sentence), severity violation|warning|soft, date, slot_id?, plate_id?, session_id?, params, options?:[{action add|raise, food_id, name, date, slot_id, plate_id, portions|grams, from?, kcal_delta, protein_delta_g, day_kcal, day_protein_pct, cook_occasions, new_meat_day, op}]} where op is an apply_changes op you can pass as it is (add with plate_id or date+slot_id and portions or amount+unit, or set_quantity with plate_component_id). A "day protein" option raises the day's protein; a "day energy" option moves the day closer to its target without leaving the band; options that bring the day into the band come first. Violations (the "day energy minimum" is a day below its kcal safety floor) are hard rules: fix them. Soft issues (label "day energy": day kcal outside the day band; "day protein" and "weekly protein": protein under 90% of the day target (g/kg) or the share under target or the weekly average under target; "weekly energy": weekly average outside the week band; "weekly carbs" and "weekly fat": the share outside its range; "light meal": a light meal over its cap (max_kcal); "hearty meal": a hearty meal under its minimum (min_kcal); "weekly meat" (over 300 g), "weekly fish" and "weekly legumes": a DGE food rule missed; "dish as main too often": a dish as main of too many meals) never block saving. Options (computed, verified, each with a ready op; ops when it needs several): "day energy" below band and "day protein" add or raise food (action add|raise); "light meal" and "hearty meal" portions (a smaller or larger portion, 0.75-1.5), remove (an add-on, light meal only) or swap (a lighter or heartier main or side); "cooking occasions per day" move_cook (cook the batch on another cell) or swap (a leftover or no-cook food). A light meal or hearty meal option may leave the day under its band (a light or hearty request ranks above the day and week bands) but never under the day energy minimum. Pass light (the user's light requests, meal unless they explicitly ask to eat less for the day) to judge the result under them; stored light requests apply without passing them again. A call with dry_run false that succeeds saves the light you pass (a date replaces that date's request, scope none clears it); a call with dry_run true saves nothing: days then show light and normal_target_kcal. Your request is not done while a hard issue (violation) it caused is open, unless you tell the user why; a soft issue you report and offer the fix for, without applying it. An away call that would leave a day with a second cooking occasion (cooking occasions per day) moves cooks itself: each such move is an applied entry with auto_fix (op move_cook, cook_date, cook_slot_id, from_date, from_slot_id); tell the user which dish now cooks where. An issue with no_fix_found true has no verified fix (no_fix_reason says why): tell the user, do not guess one.
A relative request ("lighter", "heartier", "more protein"): take the ops, light and goal of a way from find_ways and pass them as returned; goal {kind lighter|heartier|more_protein|lower_day|raise_day, plate_id (a meal) | date (the day: more_protein, lower_day, raise_day)}. The result then has request:{goal, target, before, after (meal_kcal, meal_protein_g, day_kcal, day_protein_g; a day request only the day's), max_kcal? (the most a light meal holds) or min_kcal? (the least a filling meal holds), satisfied, reached_light (lighter), reached_hearty (heartier) or reached_band (lower_day, raise_day), why?}: satisfied is true when the food changed in the asked direction by a meaningful amount (at least 10% of the meal or 80 kcal, the smaller; protein at least 10 g); reached_light/reached_hearty say whether the meal also is at its cap or minimum (false: lighter, but not a light meal yet). A meal that already meets its cap or minimum still has to change. A light or hearty entry for a meal that misses its cap (minimum) after the ops is refused with the numbers, and so is one passed with a goal when the food did not change; a goal needs ops, a call that only sets light may leave ops out.
Every list of options carries one marked recommended: true: present it as your recommendation. Alternatives for a requested change are the request: apply the recommended one at once and name one other; an offer (a step beyond the request) is applied only when the user says yes. A call without a goal whose ops made issues new, left a day under its minimum or made a dish repeat has next (report what is new, offer the recommended fix as a question) and, for a repeat, offers (a verified replacement, recommended).`

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
	Position       string   `json:"position"`
	// To is one cell (move) or a list of cells (copy).
	To       json.RawMessage `json:"to"`
	Occupant json.RawMessage `json:"occupant"`
	Cells    []cellRef       `json:"cells"`
	As       string          `json:"as"`
}

func (o changeOp) quantity() quantityInput {
	return quantityInput{Portions: o.Portions, Amount: o.Amount, Unit: o.Unit}
}

// batch collects what a call's ops touched.
type batch struct {
	aff affected
	// removed are the foods the ops took off a plate.
	removed []int64
	// made are the plate components the ops added or replaced in: an offer
	// never takes back what the user just asked for.
	made []int64
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

// pulseCell is one board cell a change line lists under "pulse": the cells the
// board flashes after the change (a plate or session in the change's rows
// flashes too; this is for cells that hold none, such as a move's source).
func pulseCell(date string, slotID int64) map[string]any {
	return map[string]any{"date": date, "slot_id": slotID}
}

func pulseCells(cs ...map[string]any) []map[string]any { return cs }

func (b *batch) plate(p *plate.Plate) {
	if !slices.Contains(b.aff.Plates, p.ID) {
		b.aff.Plates = append(b.aff.Plates, p.ID)
	}
	b.aff.Dates = append(b.aff.Dates, p.DateString())
}

// applyGoal is the relative request an apply_changes call serves (lighter,
// heartier or more_protein) with its target: a plate (a meal) or a date (the
// day, more_protein only).
type applyGoal struct {
	Kind    string `json:"kind"`
	PlateID int64  `json:"plate_id,omitempty"`
	Date    string `json:"date,omitempty"`
}

// applyInput is one apply_changes call.
type applyInput struct {
	Ops    []changeOp
	DryRun bool
	Light  []planning.LightIntent
	Goal   *applyGoal
	// quiet leaves the run log out: find_ways checks its ways with it.
	quiet bool
}

// applyRun is what a call computed: the result the model reads and the
// evaluations before and after.
type applyRun struct {
	result map[string]any
	before planning.Evaluation
	after  planning.Evaluation
}

func toolApplyChanges(svc Services) Tool {
	return Tool{
		Name:        applyChangesName,
		Description: applyChangesDescription,
		Schema:      json.RawMessage(applyChangesSchema),
		Hint:        applyChangesHint,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Ops    []changeOp             `json:"ops"`
				DryRun bool                   `json:"dry_run"`
				Light  []planning.LightIntent `json:"light"`
				Goal   *applyGoal             `json:"goal"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			light, err := parseLight(in.Light)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			ctx = withLight(ctx, light)
			if llm.CallerFrom(ctx).FromCode() && !in.DryRun {
				return nil, ToolEffectNone, fmt.Errorf("%w: apply_changes called from code must set dry_run: true; call it directly (not from code) to save the change", ErrCodeCallRefused)
			}
			if svc.Tx == nil || svc.State == nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: apply_changes is not available (no unit of work or plan reader)", domain.ErrInvalidInput)
			}
			if hint := applyChangesHint(input); hint != "" {
				return nil, ToolEffectNone, fmt.Errorf("%w: %s", domain.ErrInvalidInput, hint)
			}
			if len(in.Ops) == 0 && (len(light) == 0 || in.Goal != nil) {
				return nil, ToolEffectNone, fmt.Errorf("%w: ops is empty: a change needs at least one op (only a call that sets light without a goal may leave ops out); for a relative request take the ops of a way from find_ways", domain.ErrInvalidInput)
			}
			w, ok := windowFrom(ctx)
			if !ok {
				return nil, ToolEffectNone, fmt.Errorf("%w: no plan window in this conversation turn", domain.ErrInvalidInput)
			}
			// A replace that names its component needs no plate_id: the component knows it.
			for i, op := range in.Ops {
				// A swap that names its source (a batch to eat from, a cell to cook in)
				// is a replace: the models send it that way.
				if op.Op == "swap" && (op.SessionID != 0 || op.CookAt != nil) && op.PlateComponentID != 0 {
					in.Ops[i].Op = "replace"
					op.Op = "replace"
				}
				if op.Op == "replace" && op.PlateID == 0 && op.PlateComponentID != 0 {
					if pc, err := svc.Plates.GetComponent(ctx, op.PlateComponentID); err == nil {
						in.Ops[i].PlateID = pc.PlateID
					}
				}
			}
			run, err := runGuarded(ctx, svc, w, applyInput{Ops: in.Ops, DryRun: in.DryRun, Light: light, Goal: in.Goal}, ignoredFields(input))
			if err != nil {
				return nil, ToolEffectNone, err
			}
			run.result["saved"], run.result["status"] = saveStatus(run.result)
			return mustJSON(run.result), ToolEffectNone, nil
		},
	}
}

// saveStatus says whether anything was saved and in one sentence what that
// means: the first two fields of the result the model reads (dryRunView).
func saveStatus(res map[string]any) (bool, string) {
	switch {
	case res["dry_run"] == true:
		return false, "Nothing changed: this was only a check, say what would change and never that it was added, changed or removed."
	case res["saved"] == false:
		return false, "Nothing changed: refused, the plan is unchanged."
	}
	return true, "Saved: the change is in the plan (the user can undo it)."
}

// runApply runs the ops of one apply_changes call in one transaction (rolled
// back for a dry run), checks the marks and the goal, and builds the result.
func runApply(ctx context.Context, svc Services, w window.Window, in applyInput, ignored map[int][]string) (applyRun, error) {
	light := in.Light
	if err := checkLight(ctx, svc, w, light); err != nil {
		return applyRun{}, err
	}
	ws := windowsFor(w, opDates(in.Ops))
	// The baseline stands under the stored light only: an intent this
	// call adds must show its own S6/S6b as new (audit B2).
	before, err := evalWindows(withLight(ctx, nil), svc, ws, false, nil)
	if err != nil {
		return applyRun{}, fmt.Errorf("read the plan before the change: %w", err)
	}
	// The plan as it is before the ops, for the change report.
	beforeSt, beforeErr := svc.State(withLight(ctx, nil), w, todayFrom(ctx))
	var (
		applied []map[string]any
		after   map[string]any
		afterEv planning.Evaluation
		request map[string]any
		b       batch
		rep     *changeReport
		// cookFacts says where the away call put the day's cooks.
		cookFacts map[string]any
	)
	run := func(ctx context.Context) error {
		for i, op := range in.Ops {
			res, err := applyOp(ctx, svc, op, &b)
			if err != nil {
				return fmt.Errorf("op %d of %d (%s): %w", i+1, len(in.Ops), op.Op, err)
			}
			if id, ok := res["plate_component_id"].(int64); ok && (op.Op == "add" || op.Op == "replace") {
				b.made = append(b.made, id)
			}
			if ign := ignored[i]; len(ign) > 0 {
				res["ignored_fields"] = ign
			}
			applied = append(applied, res)
		}
		fixed, err := settleCooks(ctx, svc, ws, in.Ops, &b, before)
		if err != nil {
			return err
		}
		applied = append(applied, fixed...)
		cookFacts = cookingMoved(ctx, svc, w, fixed, in.Ops)
		b.aff.Dates = append(b.aff.Dates, planning.LightDates(light)...)
		after, afterEv, err = batchAfter(withExtraAvoid(ctx, b.removed), svc, ws, b, before)
		if err != nil {
			return err
		}
		if len(light) > 0 {
			st, err := svc.State(ctx, w, todayFrom(ctx))
			if err != nil {
				return err
			}
			if err := checkMarks(st, light, afterEv); err != nil {
				return err
			}
		}
		rep = reportOf(ctx, svc, w, beforeSt, beforeErr, before, afterEv, applied)
		if in.Goal != nil {
			if request, err = judgeGoal(ctx, svc, w, *in.Goal, light, before, afterEv); err != nil {
				return err
			}
		}
		return nil
	}
	err = svc.Tx.InTx(ctx, func(ctx context.Context) error {
		if err := run(ctx); err != nil {
			return err
		}
		if in.DryRun {
			return errDryRun
		}
		return saveLight(ctx, svc, light)
	})
	if err != nil && !errors.Is(err, errDryRun) {
		return applyRun{}, err
	}
	switch {
	case in.quiet:
	case in.DryRun:
		// Rolled back: the dry run is a step of the run log too.
		planning.RecordEvaluation(ctx, planning.PhaseDryRun, func() planning.Evaluation { return afterEv })
	default:
		noteChange(ctx, "apply_changes", map[string]any{"count": len(in.Ops), "ops": b.ops})
		noteDates(ctx, b.aff.Dates)
	}
	result := map[string]any{"dry_run": in.DryRun, "applied": applied, "after": after}
	if request != nil {
		result["request"] = request
	}
	if !in.quiet && !in.DryRun {
		svc.Removed.add(ctx, b.removed)
	}
	if in.Goal == nil && !in.quiet {
		if next, offers := adviceFor(ctx, svc, w, before, afterEv, in.DryRun, applied); next != "" {
			result["next"] = next
			if len(offers) > 0 {
				result["offers"] = offers
			}
		}
	}
	if cookFacts != nil {
		result["cooking_moved"] = cookFacts
	}
	if rep != nil {
		result["report"] = rep
	}
	if in.DryRun {
		// The rollback is done: a row that is gone now was created by the dry run.
		markProvisional(applied, after, rowGone(ctx, svc))
		result["dry_run_note"] = dryRunNote
	}
	return applyRun{result: result, before: before, after: afterEv}, nil
}

// maxSettleRounds bounds the cook moves settleCooks makes after one call.
const maxSettleRounds = 3

// settleCooks keeps an away from creating a second cooking occasion: marking
// cells away hands a cook to the first meal that still eats it, which can push
// that day over the H2 limit. For each H2 issue the ops made new, it applies
// the first verified move_cook option (the least disruptive: one cook, or all
// of the day's cooks into one cell) and reports each move as an applied
// entry with auto_fix, so the model tells the user. An issue without a
// verified option stays, marked no_fix_found.
// cookPileKey marks the context of the away guard's re-anchoring.
type cookPileKey struct{}

func settleCooks(ctx context.Context, svc Services, ws []window.Window, ops []changeOp, b *batch, before planning.Evaluation) ([]map[string]any, error) {
	if !slices.ContainsFunc(ops, func(op changeOp) bool { return op.Op == "away" }) || planning.CookPolicy == planning.CookTwoTimes {
		return nil, nil
	}
	was := issueMap(before.Issues)
	isNew := func(is planning.Issue) bool {
		_, ok := was[issueKey(is)]
		return !ok && is.Rule == "H2" && is.Code == planning.CodeTooManyCookOccasions
	}
	var out []map[string]any
	for range maxSettleRounds {
		ev, err := evalWindows(context.WithValue(ctx, cookPileKey{}, true), svc, ws, true, isNew)
		if err != nil {
			return nil, fmt.Errorf("read the plan after the away: %w", err)
		}
		i := slices.IndexFunc(ev.Issues, func(is planning.Issue) bool {
			return isNew(is) && len(is.Options) > 0 && is.Options[0].Op.Op == "move_cook"
		})
		if i < 0 {
			return out, nil
		}
		is, opt := ev.Issues[i], ev.Issues[i].Options[0]
		opOps := opt.Ops
		if len(opOps) == 0 {
			opOps = []planning.OptionOp{opt.Op}
		}
		for _, o := range opOps {
			raw, err := json.Marshal(o)
			if err != nil {
				return nil, err
			}
			var op changeOp
			if err := json.Unmarshal(raw, &op); err != nil {
				return nil, err
			}
			res, err := applyOp(ctx, svc, op, b)
			if err != nil {
				return nil, fmt.Errorf("move the cook after the away (%s): %w", food.Quote(opt.Name), err)
			}
			res["auto_fix"] = fmt.Sprintf("moved so that %s does not get a second cooking time from the away", dayLabelStr(is.Date))
			if o.Op != "move_cook" {
				res["auto_fix"] = fmt.Sprintf("so that %s keeps one cooking time with at most two dishes: %s", dayLabelStr(is.Date), opt.Name)
			}
			out = append(out, res)
		}
	}
	return out, nil
}

// dryRunNote closes a dry run's result: its new rows never existed.
const dryRunNote = "Nothing was saved. Ids marked provisional (plates, components, sessions; in issues and in an option's op too) do not exist after the check: do not use them in a later call. An option whose op uses one is marked provisional too. To apply, send the full op list again with dry_run false."

// markProvisional replaces, anywhere in a rolled-back dry run's result (the
// applied list, after's plates, sessions and issue deltas with their options'
// ops), the id of every plate, component or session that no longer exists by
// "provisional": true; an option whose op holds such an id is marked too.
// Called after the rollback, so what is gone is what the dry run created.
func markProvisional(applied []map[string]any, after map[string]any, gone func(kind string, id int64) bool) {
	r := &rolledBack{gone: gone, cache: map[string]bool{}}
	for _, m := range applied {
		r.walk(m)
	}
	r.walk(after)
}

// rowGone looks a row up by its id key ("plate_id", "plate_component_id" or
// "session_id"); a failing lookup counts as present.
func rowGone(ctx context.Context, svc Services) func(kind string, id int64) bool {
	return func(kind string, id int64) bool {
		var err error
		switch kind {
		case "plate_id":
			_, err = svc.Plates.Get(ctx, id)
		case "plate_component_id":
			_, err = svc.Plates.GetComponent(ctx, id)
		default:
			_, err = svc.Plates.CookSession(ctx, id)
		}
		return errors.Is(err, domain.ErrNotFound)
	}
}

type rolledBack struct {
	gone  func(kind string, id int64) bool
	cache map[string]bool // "kind:id" -> the row does not exist
}

var idKinds = []string{"plate_id", "plate_component_id", "session_id"}

// missing reports whether the row is gone, asking once per row.
func (r *rolledBack) missing(kind string, id int64) bool {
	key := fmt.Sprintf("%s:%d", kind, id)
	g, ok := r.cache[key]
	if !ok {
		g = r.gone(kind, id)
		r.cache[key] = g
	}
	return g
}

// walk marks every map and list below v; it reports whether v itself was marked.
func (r *rolledBack) walk(v any) bool {
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			r.walk(e)
		}
	case []map[string]any: // the ops build copies and cells this way
		for _, e := range x {
			r.walk(e)
		}
	case map[string]any:
		marked := false
		for k, e := range x {
			switch e.(type) {
			case map[string]any, []any, []map[string]any:
				if r.walk(e) && k == "op" {
					marked = true
				}
			}
		}
		for _, kind := range idKinds {
			if id, ok := idOf(x[kind]); ok && r.missing(kind, id) {
				delete(x, kind)
				x["provisional"] = true
				marked = true
			}
		}
		if _, isOption := x["action"]; isOption && marked {
			x["provisional"] = true
		}
		return x["provisional"] == true
	}
	return false
}

func idOf(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case json.Number:
		id, err := n.Int64()
		return id, err == nil
	}
	return 0, false
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
		b.note(op, map[string]any{"food": food.Data(old.FoodName), "to": food.Data(pc.FoodName), "date": p.DateString()})
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
		b.note(op, map[string]any{"food": food.Data(pc.FoodName), "date": p.DateString()})
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
		b.removed = append(b.removed, pc.FoodID)
		b.aff.Sessions = append(b.aff.Sessions, sessionIDOf(pc)...)
		b.note(op, map[string]any{"food": food.Data(pc.FoodName), "date": p.DateString()})
		return map[string]any{"op": op.Op, "plate_component_id": op.PlateComponentID, "removed": true, "food": pc.FoodName, "date": p.DateString()}, nil
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
		b.note(op, map[string]any{"food": plateNames(p), "date": p.DateString(), "pulse": pulseCells(pulseCell(p.DateString(), p.SlotID))})
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
	case "move":
		return opMove(ctx, svc, op, b)
	case "replace":
		return opReplace(ctx, svc, op, b)
	case "away":
		return opAway(ctx, svc, op, b)
	case "move_cook":
		return opMoveCook(ctx, svc, op, b)
	case "copy":
		return opCopy(ctx, svc, op, b)
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
	comp.Position = plate.Position(op.Position)
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
	b.note(op, map[string]any{"food": food.Data(added.FoodName), "date": got.DateString()})
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
	if err := checkCookCell(ctx, svc, plate.CellKey{Date: cd, SlotID: op.CookAt.SlotID}); err != nil {
		return nil, err
	}
	f, err := svc.Foods.Get(ctx, op.FoodID)
	if err != nil {
		return nil, err
	}
	if f.Kind != food.KindComposed {
		return nil, fmt.Errorf("%w: cook_at needs a recipe, %s is a leaf food (eat it without cooking)", domain.ErrInvalidInput, food.Quote(f.Name))
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
			domain.ErrInvalidInput, food.Quote(cur.FoodName), fnum(*op.Portions), fnum(frozen), fnum(cur.Consumed), fnum(cur.FrozenConsumed), fnum(cur.Consumed), fnum(cur.FrozenConsumed))
	}
	v, err := svc.Plates.SetSessionPortions(ctx, op.SessionID, plate.SessionPortions{Portions: *op.Portions, FrozenPortions: frozen, Note: sess.Note})
	if err != nil {
		return nil, err
	}
	b.aff.Sessions = append(b.aff.Sessions, v.ID)
	b.aff.Dates = append(b.aff.Dates, v.CookDate.Format(time.DateOnly))
	b.note(op, map[string]any{"food": food.Data(v.FoodName), "date": v.CookDate.Format(time.DateOnly), "portions": v.Portions})
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
		_, res, err := svc.Cells.SetOverrideMeals(ctx, availability.OverrideInput{
			Date: d, SlotID: op.SlotID, Mode: mode, PresetID: op.PresetID, Note: op.Note,
			RemovePlate: mode == availability.ModeNone,
		})
		if err != nil {
			return nil, err
		}
		out["removed_plate"] = res != nil && len(res.Removed.Plates) > 0
		if res != nil {
			mealResult(out, *res, b)
		}
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
func evalWindow(ctx context.Context, svc Services, w window.Window, withOptions bool, keep func(planning.Issue) bool) (planning.Evaluation, error) {
	st, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return planning.Evaluation{}, err
	}
	st.Light = planning.MergeLight(st.Light, lightFrom(ctx))
	st.Avoid, st.PairAddOns = svc.Removed.avoid(ctx), true
	st.CookPile, _ = ctx.Value(cookPileKey{}).(bool)
	ev := chatView(st)
	if withOptions {
		ev.Issues = planning.IssueOptions(st, planning.WindowTarget(st), ev, ev.Issues, keep)
	}
	return ev, nil
}

// batchAfter is the combined `after` of a call: the touched plates and
// sessions, the evaluation's days for the touched dates and the issues the
// ops made new or resolved against before.
func batchAfter(ctx context.Context, svc Services, ws []window.Window, b batch, before planning.Evaluation) (map[string]any, planning.Evaluation, error) {
	// Options only for the issues this call made new or worse.
	was := issueMap(before.Issues)
	changed := func(is planning.Issue) bool {
		b, ok := was[issueKey(is)]
		return !ok || worsened(b, is)
	}
	now, err := evalWindows(ctx, svc, ws, true, changed)
	if err != nil {
		return nil, planning.Evaluation{}, fmt.Errorf("read the plan after the change: %w", err)
	}
	now.Issues = withoutUndoOptions(now.Issues, b.made)
	now.Issues = lowerInstead(ctx, svc, ws[0], now.Issues, b.made)
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
					plates[i].Kcal, plates[i].ProteinG, plates[i].Details = &kcal, &prot, planning.SlimComponents(pe.Components)
				}
			}
		}
	}
	days := []planning.DayEval{}
	for _, d := range now.Days {
		if dates[d.Date] {
			days = append(days, awayDay(d))
		}
	}
	out["days"] = planning.Compact(days)
	out["meat_days"], out["meat_days_max"] = now.MeatDays, now.MeatDaysMax
	if now.Week != nil {
		out["week"] = planning.Compact(now.Week)
	}
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
	delta := deltaOf(issueMap(before.Issues), issueMap(now.Issues))
	for _, k := range []string{"new", "resolved", "worsened"} {
		if _, ok := delta[k]; ok {
			delta[k] = planning.Compact(delta[k])
		}
	}
	out["issues"] = delta
	if plates, ok := out["plates"].([]effectPlate); ok {
		out["plates"] = planning.Compact(plates)
	}
	if sessions, ok := out["sessions"].([]effectSession); ok {
		out["sessions"] = planning.Compact(sessions)
	}
	return out, now, nil
}

// opDates lists the dates the ops name (date, cook_at.date, to, cells).
func opDates(ops []changeOp) []string {
	var out []string
	for _, o := range ops {
		if o.Date != "" {
			out = append(out, o.Date)
		}
		if o.CookAt != nil && o.CookAt.Date != "" {
			out = append(out, o.CookAt.Date)
		}
		if c, err := o.toCell(); err == nil {
			out = append(out, c.Date)
		}
		cs, _ := o.toCells()
		for _, c := range append(cs, o.Cells...) {
			out = append(out, c.Date)
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
func evalWindows(ctx context.Context, svc Services, ws []window.Window, withOptions bool, keep func(planning.Issue) bool) (planning.Evaluation, error) {
	ev, err := evalWindow(ctx, svc, ws[0], withOptions, keep)
	if err != nil {
		return ev, err
	}
	seen := issueMap(ev.Issues)
	for _, w := range ws[1:] {
		more, err := evalWindow(ctx, svc, w, withOptions, keep)
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

// withoutUndoOptions drops the options that would change a component the call
// just made (remove the bread that was asked for): the day over its range
// after an explicit add is brought back by something else that fits. The first
// option left is the recommended one.
func withoutUndoOptions(issues []planning.Issue, made []int64) []planning.Issue {
	if len(made) == 0 {
		return issues
	}
	out := slices.Clone(issues)
	for i, is := range out {
		if len(is.Options) == 0 {
			continue
		}
		var keep []planning.Option
		for _, o := range is.Options {
			ops := o.Ops
			if len(ops) == 0 {
				ops = []planning.OptionOp{o.Op}
			}
			if !slices.ContainsFunc(ops, func(op planning.OptionOp) bool {
				return op.PlateComponentID != nil && slices.Contains(made, *op.PlateComponentID)
			}) {
				keep = append(keep, o)
			}
		}
		if len(keep) == len(is.Options) {
			continue
		}
		recommended := slices.ContainsFunc(keep, func(o planning.Option) bool { return o.Recommended })
		if len(keep) > 0 && !recommended {
			keep[0].Recommended, keep[0].Because = true, "the best-ranked option that was checked: no new problem"
		}
		out[i].Options = keep
	}
	return out
}

// lowerInstead gives a day pushed over its range by an explicit add the one
// option that remains once undoing the add is off the table: a verified way to
// lower the day inside its range by changing something else.
func lowerInstead(ctx context.Context, svc Services, w window.Window, issues []planning.Issue, made []int64) []planning.Issue {
	if len(made) == 0 || svc.State == nil {
		return issues
	}
	out := slices.Clone(issues)
	for i, is := range out {
		if is.Code != planning.CodeKcalAboveBand || len(is.Options) > 0 {
			continue
		}
		st, err := svc.State(ctx, w, todayFrom(ctx))
		if err != nil {
			return out
		}
		st.Avoid = svc.Removed.avoid(ctx)
		res, err := planning.Ways(st, planning.WindowTarget(st), planning.WayRequest{Date: is.Date, Goal: planning.GoalLowerDay, Limit: 8})
		if err != nil {
			continue
		}
		for _, way := range res.Ways {
			if len(way.Ops) == 0 || slices.ContainsFunc(way.Ops, func(op planning.OptionOp) bool {
				return op.PlateComponentID != nil && slices.Contains(made, *op.PlateComponentID)
			}) {
				continue
			}
			out[i].Options = []planning.Option{{
				Action: "lower", Name: way.What, Date: is.Date, Op: way.Ops[0], Ops: way.Ops, DayKcal: way.After.DayKcal,
				Recommended: true, Because: "brings the day back into its range by changing something else, the added food stays",
			}}
			break
		}
	}
	return out
}

// cookingMoved are the facts of the cooks an away call re-anchored, for the
// reply: per day and cooking time the dishes cooked there and the meals each
// feeds, and whether nothing else changed. nil when no cook was moved.
func cookingMoved(ctx context.Context, svc Services, w window.Window, fixed []map[string]any, ops []changeOp) map[string]any {
	var moved []int64
	cell := map[int64][2]any{}
	onlyMoves := true
	for _, f := range fixed {
		if f["op"] != "move_cook" {
			onlyMoves = false
			continue
		}
		if id, ok := f["session_id"].(int64); ok {
			moved = append(moved, id)
			cell[id] = [2]any{f["cook_date"], f["cook_slot_id"]}
		}
	}
	if len(moved) == 0 || svc.State == nil {
		return nil
	}
	st, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return nil
	}
	type at struct {
		date string
		slot int64
	}
	var order []at
	seen := map[at]bool{}
	for _, id := range moved {
		k := at{fmt.Sprint(cell[id][0]), 0}
		switch v := cell[id][1].(type) {
		case int64:
			k.slot = v
		case int:
			k.slot = int64(v)
		}
		if !seen[k] {
			seen[k] = true
			order = append(order, k)
		}
	}
	// Every dish cooked there now, the ones the away handed over included.
	groups := map[at][]map[string]any{}
	for _, k := range order {
		for _, ss := range st.Sessions {
			if ss.CookDate.Format(time.DateOnly) != k.date || ss.CookSlotID != k.slot {
				continue
			}
			var feeds []string
			for _, p := range st.Plates {
				for _, c := range p.Components {
					if c.SessionID == ss.ID {
						feeds = append(feeds, mealName(st, p))
					}
				}
			}
			if len(feeds) > 0 {
				groups[k] = append(groups[k], map[string]any{"dish": food.Quote(st.Foods[ss.FoodID].Name), "feeds": feeds})
			}
		}
	}
	onlyAway := !slices.ContainsFunc(ops, func(o changeOp) bool { return o.Op != "away" })
	var out []map[string]any
	for _, k := range order {
		day := dayLabelStr(k.date)
		out = append(out, map[string]any{"day": day, "cooking_time": strings.TrimPrefix(st.SlotNames[k.slot], "slot."), "dishes": groups[k]})
	}
	facts := map[string]any{"cooked_now": out}
	if onlyMoves && onlyAway {
		facts["nothing_else_changed"] = true
	}
	return facts
}
