package agent

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planview"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// PlanReader is the read model behind GET /api/plan (planview.Service).
type PlanReader interface {
	Plan(ctx context.Context, start *time.Time, today time.Time) (*planview.Plan, error)
}

// toolGetPlan returns the chat window's plan: the same cells, sessions and
// issues as GET /api/plan for that window, in a compact shape (D10).
func toolGetPlan(svc Services) Tool {
	return Tool{
		Name:        "get_plan",
		Description: "Read the current plan window: every cell (date, slot, availability mode, plate with its components), the cook sessions with consumption and warnings, the checker's issues and the draft summary. Call it to see the effect of your changes.",
		Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, ToolEffect, error) {
			w, ok := windowFrom(ctx)
			if !ok {
				return nil, ToolEffectNone, fmt.Errorf("%w: no plan window in this conversation turn", domain.ErrInvalidInput)
			}
			p, err := svc.PlanView.Plan(ctx, &w.Start, todayFrom(ctx))
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(planOutput(p)), ToolEffectNone, nil
		},
	}
}

type planOut struct {
	Window   planWindowOut    `json:"window"`
	Cells    []planCellOut    `json:"cells"`
	Sessions []planSessionOut `json:"sessions"`
	Issues   []planning.Issue `json:"issues"`
	Draft    *planDraftOut    `json:"draft"`
}

type planWindowOut struct {
	Start       string `json:"start"`
	End         string `json:"end"`
	Today       string `json:"today"`
	ShoppingDay int    `json:"shopping_day"`
}

type planCellOut struct {
	Date       string        `json:"date"`
	SlotID     int64         `json:"slot_id"`
	CellMode   string        `json:"mode"`
	PresetName *string       `json:"preset_name,omitempty"`
	Note       *string       `json:"note,omitempty"`
	Plate      *planPlateOut `json:"plate"`
}

type planPlateOut struct {
	ID         int64              `json:"id"`
	State      string             `json:"state"`
	Eaters     float64            `json:"eaters"`
	Reason     *string            `json:"reason,omitempty"`
	Components []planComponentOut `json:"components"`
}

type planComponentOut struct {
	ID        int64    `json:"id"`
	FoodID    int64    `json:"food_id"`
	Name      string   `json:"name"`
	Portions  *float64 `json:"portions,omitempty"`
	Amount    *float64 `json:"amount,omitempty"`
	Unit      *string  `json:"unit,omitempty"`
	SessionID *int64   `json:"session_id,omitempty"`
}

type planSessionOut struct {
	ID             int64            `json:"id"`
	FoodID         int64            `json:"food_id"`
	Name           string           `json:"name"`
	CookDate       string           `json:"cook_date"`
	CookSlotID     int64            `json:"cook_slot_id"`
	Portions       float64          `json:"portions"`
	FrozenPortions float64          `json:"frozen_portions"`
	State          string           `json:"state"`
	Cooked         bool             `json:"cooked"`
	Consumed       float64          `json:"consumed"`
	FrozenConsumed float64          `json:"frozen_consumed"`
	UseBy          string           `json:"use_by"`
	Links          []planLinkOut    `json:"links"`
	Warnings       []planWarningOut `json:"warnings"`
}

type planLinkOut struct {
	PlateID     int64   `json:"plate_id"`
	ComponentID int64   `json:"component_id"`
	Portions    float64 `json:"portions"`
	Kind        string  `json:"kind"`
}

type planWarningOut struct {
	Code    string `json:"code"`
	PlateID *int64 `json:"plate_id,omitempty"`
}

type planDraftOut struct {
	Plates   int     `json:"plates"`
	Sessions int     `json:"sessions"`
	RunID    *int64  `json:"run_id"`
	Notes    *string `json:"notes"`
}

func planOutput(p *planview.Plan) planOut {
	out := planOut{
		Window: planWindowOut{
			Start: p.Window.Start.Format(time.DateOnly), End: p.Window.End.Format(time.DateOnly),
			Today: p.Today.Format(time.DateOnly), ShoppingDay: p.ShoppingDay,
		},
		Cells:    make([]planCellOut, len(p.Cells)),
		Sessions: make([]planSessionOut, len(p.Sessions)),
		Issues:   p.Issues,
	}
	if out.Issues == nil {
		out.Issues = []planning.Issue{}
	}
	for i, c := range p.Cells {
		out.Cells[i] = planCellOut{
			Date: c.Date.Format(time.DateOnly), SlotID: c.SlotID, CellMode: string(c.Mode),
			PresetName: c.PresetName, Note: c.Note,
		}
		if c.Plate != nil {
			out.Cells[i].Plate = planPlate(c.Plate)
		}
	}
	for i, v := range p.Sessions {
		out.Sessions[i] = planSession(v)
	}
	if d := p.Draft; d != nil {
		out.Draft = &planDraftOut{Plates: d.Plates, Sessions: d.Sessions, RunID: d.RunID, Notes: d.Notes}
	}
	return out
}

func planSession(v plate.SessionView) planSessionOut {
	s := planSessionOut{
		ID: v.ID, FoodID: v.FoodID, Name: v.FoodName,
		CookDate: v.CookDate.Format(time.DateOnly), CookSlotID: v.CookSlotID,
		Portions: v.Portions, FrozenPortions: v.FrozenPortions, State: string(v.State),
		Cooked: v.CookedAt != nil, Consumed: v.Consumed, FrozenConsumed: v.FrozenConsumed,
		UseBy: v.UseBy.Format(time.DateOnly),
		Links: make([]planLinkOut, len(v.Links)), Warnings: make([]planWarningOut, len(v.Warnings)),
	}
	for j, l := range v.Links {
		s.Links[j] = planLinkOut{PlateID: l.PlateID, ComponentID: l.ComponentID, Portions: l.Portions, Kind: string(l.Kind)}
	}
	for j, w := range v.Warnings {
		s.Warnings[j] = planWarningOut{Code: w.Code, PlateID: w.PlateID}
	}
	return s
}

func planPlate(p *plate.Plate) *planPlateOut {
	out := &planPlateOut{
		ID: p.ID, State: string(p.State), Eaters: p.Eaters, Reason: p.Reason,
		Components: make([]planComponentOut, len(p.Components)),
	}
	for i, c := range p.Components {
		out.Components[i] = planComponentOut{
			ID: c.ID, FoodID: c.FoodID, Name: c.FoodName,
			Portions: c.Portions, Amount: c.Amount, Unit: c.Unit, SessionID: c.CookSessionID,
		}
	}
	return out
}

// Planner starts generation runs and waits for them (planning.Service).
type Planner interface {
	Generate(ctx context.Context, req planning.Request) (int64, error)
	Regenerate(ctx context.Context, req planning.Request) (int64, error)
	Await(ctx context.Context, id int64) (*planning.RunView, error)
}

// CellWriter writes the availability overrides of cells
// (availability.Service).
type CellWriter interface {
	SetOverride(ctx context.Context, in availability.OverrideInput) (availability.Cell, *plate.PlanSnapshot, error)
	ClearOverride(ctx context.Context, date time.Time, slotID int64) error
	// Override returns the cell's stored override, nil when none.
	Override(ctx context.Context, date time.Time, slotID int64) (*availability.Override, error)
}

// rowSeedSchema is the plate_id / session_id pair of confirm and unlock;
// exactly one is required, which the handler checks (no schema combinators).
const rowSeedSchema = `
        "plate_id":{"type":"integer","minimum":1,"description":"A plate of the batch closure."},
        "session_id":{"type":"integer","minimum":1,"description":"A cook session of the batch closure."}`

type rowSeed struct {
	PlateID   int64 `json:"plate_id"`
	SessionID int64 `json:"session_id"`
}

func (r rowSeed) rows() (plate.RowSet, error) {
	switch {
	case (r.PlateID == 0) == (r.SessionID == 0):
		return plate.RowSet{}, fmt.Errorf("%w: exactly one of plate_id or session_id is required", domain.ErrInvalidInput)
	case r.PlateID != 0:
		return plate.RowSet{PlateIDs: []int64{r.PlateID}}, nil
	default:
		return plate.RowSet{SessionIDs: []int64{r.SessionID}}, nil
	}
}

// guardClosure is guardDate on every row of a closure that is in state from,
// the rows a confirm or unlock changes.
func guardClosure(ctx context.Context, plates []plate.Plate, sessions []plate.CookSession, from plate.State) error {
	for _, p := range plates {
		if p.State == from {
			if err := guardDate(ctx, p.Date); err != nil {
				return err
			}
		}
	}
	for _, s := range sessions {
		if s.State == from {
			if err := guardDate(ctx, s.CookDate); err != nil {
				return err
			}
		}
	}
	return nil
}

type rowsOut struct {
	PlateIDs   []int64 `json:"plate_ids"`
	SessionIDs []int64 `json:"session_ids"`
}

func rowsOutput(r plate.RowSet) rowsOut {
	out := rowsOut{PlateIDs: r.PlateIDs, SessionIDs: r.SessionIDs}
	if out.PlateIDs == nil {
		out.PlateIDs = []int64{}
	}
	if out.SessionIDs == nil {
		out.SessionIDs = []int64{}
	}
	return out
}

func toolSetSessionPortions(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["session_id","portions"],
      "properties":{
        "session_id":{"type":"integer","minimum":1},
        "portions":{"type":"number","exclusiveMinimum":0,"maximum":100,"description":"Total portions the batch cooks."},
        "frozen_portions":{"type":"number","minimum":0,"maximum":100,"description":"Of those, portions to freeze. Omitted keeps the current value."}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "set_session_portions",
		Description: "Set how many portions a cook session (batch) makes and how many of them are frozen. Returns the session with its consumption and warnings.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				SessionID      int64    `json:"session_id"`
				Portions       float64  `json:"portions"`
				FrozenPortions *float64 `json:"frozen_portions"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			sess, err := svc.Plates.CookSession(ctx, in.SessionID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			if err := guardDate(ctx, sess.CookDate); err != nil {
				return nil, ToolEffectNone, err
			}
			frozen := sess.FrozenPortions
			if in.FrozenPortions != nil {
				frozen = *in.FrozenPortions
			}
			v, err := svc.Plates.SetSessionPortions(ctx, in.SessionID, plate.SessionPortions{
				Portions: in.Portions, FrozenPortions: frozen, Note: sess.Note,
			})
			if err != nil {
				return nil, ToolEffectNone, err
			}
			noteChange(ctx, "set_session_portions", map[string]any{
				"name": v.FoodName, "portions": v.Portions, "frozen": v.FrozenPortions, "date": v.CookDate.Format(time.DateOnly),
			})
			return mustJSON(planSession(*v)), ToolEffectNone, nil
		},
	}
}

func toolSetPlateEaters(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["plate_id","eaters"],
      "properties":{
        "plate_id":{"type":"integer","minimum":1},
        "eaters":{"type":"number","exclusiveMinimum":0,"maximum":100}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "set_plate_eaters",
		Description: "Set how many people eat a plate (e.g. guests). The batches the plate eats from are resized to match.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PlateID int64   `json:"plate_id"`
				Eaters  float64 `json:"eaters"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			if err := guardPlateDate(ctx, svc, in.PlateID); err != nil {
				return nil, ToolEffectNone, err
			}
			p, err := svc.Plates.Update(ctx, in.PlateID, plate.PlatePatch{Eaters: &in.Eaters})
			if err != nil {
				return nil, ToolEffectNone, err
			}
			noteChange(ctx, "set_plate_eaters", map[string]any{"name": plateNames(p), "date": p.DateString(), "eaters": p.Eaters})
			return mustJSON(plateSummary(p)), ToolEffectNone, nil
		},
	}
}

// availabilityDefault is set_availability's mode that clears the cell's
// override, so the weekday default applies again.
const availabilityDefault = "default"

func toolSetAvailability(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["date","slot_id","mode"],
      "properties":{
        "date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
        "slot_id":{"type":"integer","minimum":1},
        "mode":{"type":"string","enum":["plan","none","routine","default"]},
        "preset_id":{"type":"integer","minimum":1,"description":"The routine's preset; required for mode routine."},
        "note":{"type":"string","maxLength":200,"description":"Why there is no plate (mode none only)."}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "set_availability",
		Description: "Set what a cell (date, slot) is for: plan (a planned plate), none (no plate, e.g. eating out; removes a draft plate there), routine (a fixed preset) or default (back to the weekday default).",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Date     string  `json:"date"`
				SlotID   int64   `json:"slot_id"`
				CellMode string  `json:"mode"`
				PresetID *int64  `json:"preset_id"`
				Note     *string `json:"note"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			d, err := time.Parse(time.DateOnly, in.Date)
			if err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: date: %v", domain.ErrInvalidInput, err)
			}
			if err := guardDate(ctx, d); err != nil {
				return nil, ToolEffectNone, err
			}
			before, err := svc.Cells.Override(ctx, d, in.SlotID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			slotKey, err := slotName(ctx, svc, in.SlotID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			out := map[string]any{"date": in.Date, "slot_id": in.SlotID, "mode": in.CellMode, "removed_plate": false}
			if in.CellMode == availabilityDefault {
				if err := svc.Cells.ClearOverride(ctx, d, in.SlotID); err != nil {
					return nil, ToolEffectNone, err
				}
				if err := noteAvailability(ctx, svc, d, in.SlotID, before, in.CellMode, slotKey); err != nil {
					return nil, ToolEffectNone, err
				}
				return mustJSON(out), ToolEffectNone, nil
			}
			mode := availability.Mode(in.CellMode)
			_, removed, err := svc.Cells.SetOverride(ctx, availability.OverrideInput{
				Date: d, SlotID: in.SlotID, Mode: mode, PresetID: in.PresetID, Note: in.Note,
				RemovePlate: mode == availability.ModeNone,
			})
			if err != nil {
				return nil, ToolEffectNone, err
			}
			out["removed_plate"] = removed != nil
			if err := noteAvailability(ctx, svc, d, in.SlotID, before, in.CellMode, slotKey); err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(out), ToolEffectNone, nil
		},
	}
}

// noteAvailability notes a set_availability change: none, or plan for the
// modes that let the cell hold a plate again (plan, routine, default). It
// reads the override the write left, so undo can tell a newer manual edit.
func noteAvailability(ctx context.Context, svc Services, d time.Time, slotID int64, before *availability.Override, mode, slotKey string) error {
	after, err := svc.Cells.Override(ctx, d, slotID)
	if err != nil {
		return err
	}
	key := "set_availability_plan"
	if mode == string(availability.ModeNone) {
		key = "set_availability_none"
	}
	noteCell(ctx, d, slotID, before, after)
	noteChange(ctx, key, map[string]any{"date": d.Format(time.DateOnly), "slot": slotKey, "mode": mode})
	return nil
}

// regenerateDetachedWait bounds the wait for a run whose caller went away:
// the generator's own run timeout (3 minutes) plus its save budget.
const regenerateDetachedWait = 4 * time.Minute

func toolConfirm(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "properties":{` + rowSeedSchema + `
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "confirm",
		Description: "Confirm a draft plate or cook session together with its batch closure (every plate and batch linked to it). Give exactly one of plate_id or session_id. Only when the user asks for it.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in rowSeed
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			seed, err := in.rows()
			if err != nil {
				return nil, ToolEffectNone, err
			}
			_, plates, sessions, err := svc.Plates.BatchClosure(ctx, seed)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			if err := guardClosure(ctx, plates, sessions, plate.StateDraft); err != nil {
				return nil, ToolEffectNone, err
			}
			rows, err := svc.Plates.Confirm(ctx, seed)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			noteChange(ctx, "confirm", map[string]any{"count": len(rows.PlateIDs)})
			return mustJSON(rowsOutput(rows)), ToolEffectNone, nil
		},
	}
}

// unlockPreview is unlock's result without user_confirmed: the rows the
// user is asked about, in the llm.UnlockRequestPayload shape the loop
// streams.
type unlockPreview struct {
	Status string          `json:"status"`
	Rows   []llm.UnlockRow `json:"rows"`
	rowsOut
}

func toolUnlock(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "properties":{` + rowSeedSchema + `,
        "user_confirmed":{"type":"boolean","description":"true only after the user said yes to this unlock in a message after your preview."}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name: "unlock",
		Description: "Return a confirmed plate or cook session, with its batch closure, to draft so you can change it. Two steps: first call without user_confirmed; it changes nothing, " +
			"shows the user the rows and you ask for a yes. Only after the user's yes in a later message call it again with user_confirmed: true. Give exactly one of plate_id or session_id.",
		Schema: schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				rowSeed
				UserConfirmed bool `json:"user_confirmed"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			seed, err := in.rows()
			if err != nil {
				return nil, ToolEffectNone, err
			}
			turn, ok := turnFrom(ctx)
			if !ok {
				return nil, ToolEffectNone, fmt.Errorf("%w: no conversation", domain.ErrAIUnlockNeedsUser)
			}
			closure, plates, sessions, err := svc.Plates.BatchClosure(ctx, seed)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			if err := guardClosure(ctx, plates, sessions, plate.StateConfirmed); err != nil {
				return nil, ToolEffectNone, err
			}
			if !in.UserConfirmed {
				turn.unlocks.issue(turn, closure)
				return mustJSON(unlockPreview{
					Status: "needs_user_confirmation", Rows: unlockRows(plates), rowsOut: rowsOutput(closure),
				}), ToolEffectUnlockRequest, nil
			}
			if !turn.unlocks.confirmed(turn, closure) {
				return nil, ToolEffectNone, fmt.Errorf("%w: preview the unlock and wait for the user's yes", domain.ErrAIUnlockNeedsUser)
			}
			rows, err := svc.Plates.Unlock(ctx, seed)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			turn.unlocks.drop(turn, closure)
			noteChange(ctx, "unlock", map[string]any{"count": len(rows.PlateIDs)})
			return mustJSON(rowsOutput(rows)), ToolEffectNone, nil
		},
	}
}

// unlockRows lists the closure's plates in cell order with their food names.
func unlockRows(plates []plate.Plate) []llm.UnlockRow {
	sorted := slices.Clone(plates)
	slices.SortFunc(sorted, func(a, b plate.Plate) int {
		if c := a.Date.Compare(b.Date); c != 0 {
			return c
		}
		return cmp.Compare(a.SlotID, b.SlotID)
	})
	rows := make([]llm.UnlockRow, len(sorted))
	for i, p := range sorted {
		names := make([]string, len(p.Components))
		for j, c := range p.Components {
			names[j] = c.FoodName
		}
		rows[i] = llm.UnlockRow{Date: p.DateString(), SlotID: p.SlotID, Names: names}
	}
	return rows
}

func toolRegenerate(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["kind"],
      "properties":{
        "kind":{"type":"string","enum":["window","day","plate","side","empty_cells"],"description":"window: the whole draft plan; day: one day (date); plate: one plate (plate_id); side: one side dish (component_id); empty_cells: fill the open cells."},
        "date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$","description":"Required for kind day."},
        "plate_id":{"type":"integer","minimum":1,"description":"Required for kind plate."},
        "component_id":{"type":"integer","minimum":1,"description":"The side's plate component; required for kind side."},
        "instruction":{"type":"string","maxLength":500,"description":"The user's wish for the new choice, e.g. 'something lighter'."}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name: "regenerate",
		Description: "Let the planner choose again for part of the draft plan and wait for it: use it for 'different/another X' requests. Confirmed rows are kept. " +
			"Returns the run's status, counts, issues and the changed plates.",
		Schema: schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Kind        string `json:"kind"`
				Date        string `json:"date"`
				PlateID     int64  `json:"plate_id"`
				ComponentID int64  `json:"component_id"`
				Instruction string `json:"instruction"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			w, ok := windowFrom(ctx)
			if !ok {
				return nil, ToolEffectNone, fmt.Errorf("%w: no plan window in this conversation turn", domain.ErrInvalidInput)
			}
			sc, err := regenerateScope(ctx, svc, planning.ScopeKind(in.Kind), in.Date, in.PlateID, in.ComponentID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			params, before, err := regenerateParams(ctx, svc, sc)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			req := planning.Request{Start: w.Start, Today: todayFrom(ctx), Scope: sc, Instruction: in.Instruction}
			var id int64
			if sc.Kind == planning.ScopeWindow {
				id, err = svc.Planner.Generate(ctx, req)
			} else {
				id, err = svc.Planner.Regenerate(ctx, req)
			}
			if err != nil {
				return nil, ToolEffectNone, err
			}
			noteRun(ctx, id)
			v, err := svc.Planner.Await(ctx, id)
			if err != nil {
				if ctx.Err() != nil {
					// The caller is gone but the run goes on and rewrites the
					// plan. Note the change and wait for the run on a ctx that
					// outlives the caller, so Execute records it (undoable) once
					// the run finished. A failed or unknown run changed nothing.
					noteChange(ctx, "regenerate_"+string(sc.Kind), params)
					wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), regenerateDetachedWait)
					defer cancel()
					if _, werr := svc.Planner.Await(wctx, id); werr != nil {
						slog.ErrorContext(ctx, "ai.change.regenerate wait failed", "run_id", id, "error", werr)
					}
				}
				return nil, ToolEffectNone, err
			}
			out := regenerateOut{
				RunID: id, Status: string(v.Status), ErrorKey: v.ErrorKey, Notes: v.Notes, Issues: v.Issues,
				Counts:  regenerateCounts{Plates: v.Counts.Plates, Sessions: v.Counts.Sessions, CookDays: v.Counts.CookDays, KeptConfirmed: v.Counts.KeptConfirmed},
				Changed: []changedPlateOut{},
			}
			if v.Status != airun.StatusOK && v.Status != airun.StatusPartial {
				return mustJSON(out), ToolEffectNone, nil
			}
			if out.Changed, err = changedPlates(ctx, svc, w.Start, w.End, id); err != nil {
				// The run already rewrote the plan: it is still a change.
				noteChange(ctx, "regenerate_"+string(sc.Kind), params)
				return nil, ToolEffectNone, err
			}
			params["count"] = len(out.Changed)
			if sc.Kind == planning.ScopeSide {
				if name, ok := newSideName(before, out.Changed); ok {
					params["name"] = name
				}
			}
			noteChange(ctx, "regenerate_"+string(sc.Kind), params)
			return mustJSON(out), ToolEffectNone, nil
		},
	}
}

// regenerateParams are a regenerate change's display params before the run:
// the day, or the plate's or side's food names with their date (a side also
// its slot), and the plate as it was (side only) so the new side can be told
// from the old one.
func regenerateParams(ctx context.Context, svc Services, sc planning.Scope) (map[string]any, *plate.Plate, error) {
	switch sc.Kind {
	case planning.ScopeDay:
		return map[string]any{"date": sc.Date.Format(time.DateOnly)}, nil, nil
	case planning.ScopePlate:
		p, err := svc.Plates.Get(ctx, sc.PlateID)
		if err != nil {
			return nil, nil, err
		}
		return map[string]any{"name": plateNames(p), "date": p.DateString()}, nil, nil
	case planning.ScopeSide:
		p, pc, err := componentOnPlate(ctx, svc, sc.ComponentID)
		if err != nil {
			return nil, nil, err
		}
		slotKey, err := slotName(ctx, svc, p.SlotID)
		if err != nil {
			return nil, nil, err
		}
		return map[string]any{"name": pc.FoodName, "date": p.DateString(), "slot": slotKey}, p, nil
	default:
		return map[string]any{}, nil, nil
	}
}

// newSideName is the name of the food the regenerated plate now has that the
// plate before the run did not (a side swap keeps the plate, D6).
func newSideName(before *plate.Plate, changed []changedPlateOut) (string, bool) {
	if before == nil {
		return "", false
	}
	had := map[int64]bool{}
	for _, c := range before.Components {
		had[c.FoodID] = true
	}
	for _, ch := range changed {
		if ch.Plate == nil || ch.Plate.ID != before.ID {
			continue
		}
		for _, c := range ch.Plate.Components {
			if !had[c.FoodID] {
				return c.Name, true
			}
		}
	}
	return "", false
}

// regenerateScope checks the id kind needs (D8) and the past-date guard on
// its target.
func regenerateScope(ctx context.Context, svc Services, kind planning.ScopeKind, date string, plateID, componentID int64) (planning.Scope, error) {
	sc := planning.Scope{Kind: kind}
	switch kind {
	case planning.ScopeDay:
		if date == "" {
			return sc, fmt.Errorf("%w: date is required for kind day", domain.ErrInvalidInput)
		}
		d, err := time.Parse(time.DateOnly, date)
		if err != nil {
			return sc, fmt.Errorf("%w: date: %v", domain.ErrInvalidInput, err)
		}
		sc.Date = d
		return sc, guardDate(ctx, d)
	case planning.ScopePlate:
		if plateID == 0 {
			return sc, fmt.Errorf("%w: plate_id is required for kind plate", domain.ErrInvalidInput)
		}
		sc.PlateID = plateID
		return sc, guardPlateDate(ctx, svc, plateID)
	case planning.ScopeSide:
		if componentID == 0 {
			return sc, fmt.Errorf("%w: component_id is required for kind side", domain.ErrInvalidInput)
		}
		sc.ComponentID = componentID
		return sc, guardComponentDate(ctx, svc, componentID)
	case planning.ScopeWindow, planning.ScopeEmptyCells:
		return sc, nil
	default:
		return sc, fmt.Errorf("%w: kind %q", domain.ErrInvalidScope, kind)
	}
}

type regenerateOut struct {
	RunID    int64             `json:"run_id"`
	Status   string            `json:"status"`
	ErrorKey string            `json:"error_key,omitempty"`
	Counts   regenerateCounts  `json:"counts"`
	Issues   []planning.Issue  `json:"issues"`
	Notes    string            `json:"notes,omitempty"`
	Changed  []changedPlateOut `json:"changed"`
}

type regenerateCounts struct {
	Plates        int `json:"plates"`
	Sessions      int `json:"sessions"`
	CookDays      int `json:"cook_days"`
	KeptConfirmed int `json:"kept_confirmed"`
}

type changedPlateOut struct {
	Date   string        `json:"date"`
	SlotID int64         `json:"slot_id"`
	Plate  *planPlateOut `json:"plate"`
}

// changedPlates lists the plates of [from, to] that run wrote, or that eat
// from a session it wrote (a side swap keeps the plate, D6).
func changedPlates(ctx context.Context, svc Services, from, to time.Time, runID int64) ([]changedPlateOut, error) {
	plates, err := svc.Plates.Range(ctx, from, to)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, p := range plates {
		for _, c := range p.Components {
			if c.CookSessionID != nil {
				ids = append(ids, *c.CookSessionID)
			}
		}
	}
	sessions, err := svc.Plates.CookSessions(ctx, ids)
	if err != nil {
		return nil, err
	}
	byRun := map[int64]bool{}
	for _, s := range sessions {
		if s.AIRunID != nil && *s.AIRunID == runID {
			byRun[s.ID] = true
		}
	}
	out := []changedPlateOut{}
	for i := range plates {
		p := &plates[i]
		changed := p.AIRunID != nil && *p.AIRunID == runID
		for _, c := range p.Components {
			changed = changed || (c.CookSessionID != nil && byRun[*c.CookSessionID])
		}
		if changed {
			out = append(out, changedPlateOut{Date: p.DateString(), SlotID: p.SlotID, Plate: planPlate(p)})
		}
	}
	return out, nil
}
