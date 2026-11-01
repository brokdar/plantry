package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/xeipuuv/gojsonschema"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// ToolEffect classifies what a tool's output means to the agent loop besides
// feeding the model. Plan writes are not an effect: a writing tool notes its
// change (noteChange) and Execute records it (D13).
type ToolEffect int

const (
	// ToolEffectNone: the output only goes back to the model.
	ToolEffectNone ToolEffect = iota
	// ToolEffectUnlockRequest means the call previewed an unlock: its output
	// holds the llm.UnlockRequestPayload rows the loop streams (D6).
	ToolEffectUnlockRequest
)

// ToolHandler executes a single tool against its raw JSON input and returns
// a raw JSON output to feed back to the model.
type ToolHandler func(ctx context.Context, input json.RawMessage) (output json.RawMessage, effect ToolEffect, err error)

// Tool is one callable capability exposed to the model.
type Tool struct {
	Name        string
	Description string
	Schema      json.RawMessage
	Handler     ToolHandler
}

// ErrToolNotFound is returned when the model emits a tool call for a name the
// registry doesn't know.
var ErrToolNotFound = errors.New("tool not found")

// ToolSet holds the registered tools and their compiled input schemas.
type ToolSet struct {
	tools   []Tool
	byName  map[string]*Tool
	schemas map[string]*gojsonschema.Schema
	changes ChangeStore // nil: changes are not recorded
}

// Services aggregates the domain services the default tool set depends on.
type Services struct {
	Foods             *food.Service
	NutritionResolver *food.NutritionResolver
	Plates            *plate.Service
	Profile           *profile.Service
	Slots             *slot.Service
	Presets           PresetService
	// Availability may be nil: get_plates_range then lists no cells.
	Availability AvailabilityResolver
	// Settings resolves plan.default_keep_days for the foods' effective
	// keep_days.
	Settings plate.SettingsReader
	// PlanView may be nil: get_plan is then not registered.
	PlanView PlanReader
	// Planner may be nil: regenerate is then not registered.
	Planner Planner
	// Cells may be nil: set_availability is then not registered.
	Cells CellWriter
	// Changes may be nil: plan writes are then not recorded as changes.
	Changes ChangeStore
}

// NewToolSet builds the default tool set wired to the given services.
func NewToolSet(svc Services) (*ToolSet, error) {
	ts := &ToolSet{
		byName:  map[string]*Tool{},
		schemas: map[string]*gojsonschema.Schema{},
		changes: svc.Changes,
	}
	for _, t := range defaultTools(svc) {
		if err := ts.register(t); err != nil {
			return nil, fmt.Errorf("register %s: %w", t.Name, err)
		}
	}
	return ts, nil
}

func (s *ToolSet) register(t Tool) error {
	if _, exists := s.byName[t.Name]; exists {
		return fmt.Errorf("duplicate tool %q", t.Name)
	}
	schema, err := gojsonschema.NewSchema(gojsonschema.NewBytesLoader(t.Schema))
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	s.tools = append(s.tools, t)
	s.byName[t.Name] = &s.tools[len(s.tools)-1]
	s.schemas[t.Name] = schema
	return nil
}

// Describe returns the tool metadata as the LLM port expects it.
func (s *ToolSet) Describe() []llm.Tool {
	out := make([]llm.Tool, len(s.tools))
	for i, t := range s.tools {
		out[i] = llm.Tool{Name: t.Name, Description: t.Description, Schema: t.Schema}
	}
	return out
}

// Execute validates input against the tool's schema then dispatches to its
// handler. Schema violations surface as domain.ErrInvalidInput so the model
// sees a structured tool_result and can retry with corrected arguments.
// Each call is recorded as a tool_call step of the AI run in ctx, if any, and
// its plan changes are made as the agent of that run (D20). A plan-writing
// call of a run is recorded as a change (D13), which Execute returns; nil for
// any other call. A failed call can record one too (a regenerate whose caller
// went away while its run rewrote the plan), returned together with the
// error. A successful call's result object carries "change_id" (recorded) or
// "warning": "change_not_recorded"; the loop adds the same to a failed
// call's result through withChange.
func (s *ToolSet) Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, ToolEffect, *Change, error) {
	start := time.Now()
	run := airun.FromContext(ctx)
	ctx = actor.With(ctx, actor.Agent, run.ID())
	ctx, note := withChangeNote(ctx)
	mark := s.watermark(ctx, run.ID())
	output, effect, err := s.execute(ctx, name, input)
	result := output
	if err != nil {
		result = mustMarshal(map[string]string{"error": err.Error()})
	}
	seq := run.ToolStep(ctx, name, input, result, err != nil, time.Since(start))
	if !note.set {
		return output, effect, nil, err
	}
	if mark < 0 {
		if s.changes != nil && run.ID() != 0 {
			// The watermark failed (logged there): the write has no change.
			change := unrecordedChange(run.ID(), seq, name, note)
			return withChange(output, change), effect, change, err
		}
		return output, effect, nil, err
	}
	// A call that failed after writing (regenerate whose caller went away)
	// notes its change anyway. Record on a ctx that outlives the caller.
	change := s.recordChange(context.WithoutCancel(ctx), run.ID(), seq, name, note, mark)
	return withChange(output, change), effect, change, err
}

// notRecordedWarning is the key tool results carry when the call's change
// could not be saved.
const notRecordedWarning = "change_not_recorded"

// withChange adds the call's change to a JSON object result: "change_id" when
// it was recorded, {"warning": "change_not_recorded"} when it was made but
// not saved. The frontend matches persisted changes to tool results by
// change_id. Anything else (no change, no object) is returned as it was.
func withChange(output json.RawMessage, c *Change) json.RawMessage {
	if c == nil {
		return output
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(output, &m) != nil || m == nil {
		return output
	}
	if c.Unrecorded {
		m["warning"] = mustMarshal(notRecordedWarning)
	} else {
		m["change_id"] = mustMarshal(c.ID)
	}
	return mustMarshal(m)
}

func (s *ToolSet) execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
	t, ok := s.byName[name]
	if !ok {
		return nil, ToolEffectNone, fmt.Errorf("%w: %s", ErrToolNotFound, name)
	}
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	result, err := s.schemas[name].Validate(gojsonschema.NewBytesLoader(input))
	if err != nil {
		return nil, ToolEffectNone, fmt.Errorf("%w: schema validation: %v", domain.ErrInvalidInput, err)
	}
	if !result.Valid() {
		return nil, ToolEffectNone, fmt.Errorf("%w: %s", domain.ErrInvalidInput, schemaErrors(result))
	}
	return t.Handler(ctx, input)
}

func schemaErrors(r *gojsonschema.Result) string {
	msgs := make([]string, 0, len(r.Errors()))
	for _, e := range r.Errors() {
		msgs = append(msgs, e.String())
	}
	return strings.Join(msgs, "; ")
}

// ---------------------------------------------------------------------------
// default tool definitions
// ---------------------------------------------------------------------------

func defaultTools(svc Services) []Tool {
	tools := []Tool{
		toolListFoods(svc),
		toolGetFood(svc),
		toolListSlots(svc),
		toolGetPlatesRange(svc),
		toolGetProfile(svc),
		toolCreatePlate(svc),
		toolAddFoodToPlate(svc),
		toolSwapFood(svc),
		toolUpdatePlateComponent(svc),
		toolRemovePlateComponent(svc),
		toolDeletePlate(svc),
		toolSetPlateEaters(svc),
		toolSetSessionPortions(svc),
		toolConfirm(svc),
		toolUnlock(svc),
		toolRecordPreference(svc),
	}
	if svc.Presets != nil {
		tools = append(tools, presetTools(svc)...)
	}
	if svc.PlanView != nil {
		tools = append(tools, toolGetPlan(svc))
	}
	if svc.Cells != nil {
		tools = append(tools, toolSetAvailability(svc))
	}
	if svc.Planner != nil {
		tools = append(tools, toolRegenerate(svc))
	}
	return tools
}

// --- read tools ---

func toolListFoods(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "properties":{
        "search":{"type":"string"},
        "kind":{"type":"string","enum":["leaf","composed"]},
        "role":{"type":"string","enum":["main","side_starch","side_veg","side_protein","sauce","drink","dessert","standalone"]},
        "tag":{"type":"string"},
        "limit":{"type":"integer","minimum":1,"maximum":200},
        "offset":{"type":"integer","minimum":0},
        "sort_by":{"type":"string","enum":["name","created_at","last_eaten"]},
        "sort_desc":{"type":"boolean"}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "list_foods",
		Description: "List foods in the library. A food is either a LEAF (single edible item with direct nutrition — apple, rice, chicken) or COMPOSED (built from child foods — curry, schnitzel). Filter by kind, role, tag, or search substring. Use this to find a food by name before referencing it by id. Each item carries favorite and diet_class (vegan, vegetarian, pescetarian, omnivore or unknown); composed foods also carry last_eaten (YYYY-MM-DD of the latest eaten plate up to today, or null), times_cooked (cook sessions up to today), keep_days (days leftovers keep, the plan default when the recipe sets none) and freezable (null = unknown). sort_by last_eaten lists never-eaten recipes first, then the longest uneaten. Each item also carries per_portion macros {kcal, protein, fat, carbs} with basis ('per_portion' for composed foods, 'per_100g' for leaves), nutrition_complete, and missing_nutrition (names of leaves without nutrition data) when incomplete.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Search   string `json:"search"`
				Kind     string `json:"kind"`
				Role     string `json:"role"`
				Tag      string `json:"tag"`
				Limit    int    `json:"limit"`
				Offset   int    `json:"offset"`
				SortBy   string `json:"sort_by"`
				SortDesc bool   `json:"sort_desc"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			res, err := svc.Foods.List(ctx, food.ListQuery{
				Today:    todayFrom(ctx),
				Kind:     food.Kind(in.Kind),
				Search:   in.Search,
				Role:     in.Role,
				Tag:      in.Tag,
				Limit:    in.Limit,
				Offset:   in.Offset,
				SortBy:   in.SortBy,
				SortDesc: in.SortDesc,
			})
			if err != nil {
				return nil, ToolEffectNone, err
			}
			foods := make([]*food.Food, len(res.Items))
			for i := range res.Items {
				foods[i] = &res.Items[i]
			}
			if err := svc.Foods.Derive(ctx, todayFrom(ctx), foods...); err != nil {
				return nil, ToolEffectNone, err
			}
			keep, err := defaultKeepDays(ctx, svc)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			items := make([]map[string]any, 0, len(res.Items))
			for i := range res.Items {
				f := &res.Items[i]
				item := foodSummary(f, keep)
				// A failed macro lookup for one food must not sink the whole
				// page: that item simply goes without per_portion data.
				if err := addPerPortion(ctx, svc, item, f); err != nil {
					if ctxErr := ctx.Err(); ctxErr != nil {
						return nil, ToolEffectNone, ctxErr
					}
					slog.WarnContext(ctx, "list_foods: per-portion lookup failed", "food_id", f.ID, "error", err)
				}
				items = append(items, item)
			}
			return mustJSON(map[string]any{"items": items, "total": res.Total}), ToolEffectNone, nil
		},
	}
}

func toolGetFood(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["food_id"],
      "properties":{"food_id":{"type":"integer","minimum":1}},
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "get_food",
		Description: "Fetch a food by id. Composed foods include children (child food + grams), instructions, and tags; leaf foods include direct nutrition and portion overrides. Also returns favorite and diet_class; composed foods also return last_eaten (YYYY-MM-DD or null), times_cooked, keep_days (the plan default when the recipe sets none) and freezable. Also returns per_portion macros {kcal, protein, fat, carbs} with basis ('per_portion' for composed foods, 'per_100g' for leaves), nutrition_complete, and missing_nutrition (names of leaves without nutrition data) when incomplete.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				FoodID int64 `json:"food_id"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			f, err := svc.Foods.Get(ctx, in.FoodID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			if err := svc.Foods.Derive(ctx, todayFrom(ctx), f); err != nil {
				return nil, ToolEffectNone, err
			}
			keep, err := defaultKeepDays(ctx, svc)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			out := foodDetail(f, keep)
			if err := addPerPortion(ctx, svc, out, f); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("get_food nutrition: %w", err)
			}
			return mustJSON(out), ToolEffectNone, nil
		},
	}
}

func toolListSlots(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "properties":{"active_only":{"type":"boolean"}},
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "list_slots",
		Description: "List the user-defined meal time slots (breakfast, lunch, dinner, etc.). Required before creating plates — the slot_id must come from this list.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				ActiveOnly bool `json:"active_only"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			slots, err := svc.Slots.List(ctx, in.ActiveOnly)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			items := make([]map[string]any, 0, len(slots))
			for _, s := range slots {
				items = append(items, map[string]any{
					"id": s.ID, "name_key": s.NameKey, "icon": s.Icon,
					"sort_order": s.SortOrder, "active": s.Active,
				})
			}
			return mustJSON(map[string]any{"items": items}), ToolEffectNone, nil
		},
	}
}

func toolGetPlatesRange(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["from","to"],
      "properties":{
        "from":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
        "to":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "get_plates_range",
		Description: "Get all plates for a date range (from and to inclusive, format YYYY-MM-DD). Use this instead of get_week to read plates by calendar date. Each plate carries state (draft or confirmed: you may change drafts only), ai_run_id and reason. The sessions array lists the cook sessions the plates eat from (id, food_id, cook_date, cook_slot_id, portions, frozen_portions, state). The availability array lists every cell in the range that is not plan: mode none (No plate: the user eats elsewhere, never create a plate there) or routine (covered by preset_name).",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				From string `json:"from"`
				To   string `json:"to"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			from, err := time.Parse("2006-01-02", in.From)
			if err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: from: %v", domain.ErrInvalidInput, err)
			}
			to, err := time.Parse("2006-01-02", in.To)
			if err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: to: %v", domain.ErrInvalidInput, err)
			}
			plates, err := svc.Plates.Range(ctx, from, to)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			items := make([]map[string]any, 0, len(plates))
			for i := range plates {
				items = append(items, plateSummary(&plates[i]))
			}
			sessions, err := linkedSessionList(ctx, svc, plates)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			cells := []map[string]any{}
			if svc.Availability != nil {
				resolved, err := svc.Availability.Resolve(ctx, from, to)
				if err != nil {
					return nil, ToolEffectNone, err
				}
				for _, c := range resolved {
					if c.Mode == availability.ModePlan {
						continue
					}
					cell := map[string]any{"date": c.Date.Format("2006-01-02"), "slot_id": c.SlotID, "mode": string(c.Mode)}
					if c.PresetName != nil {
						cell["preset_name"] = *c.PresetName
					}
					cells = append(cells, cell)
				}
			}
			return mustJSON(map[string]any{"plates": items, "sessions": sessions, "availability": cells}), ToolEffectNone, nil
		},
	}
}

func toolGetProfile(svc Services) Tool {
	schema := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	return Tool{
		Name:        "get_profile",
		Description: "Read the user's profile: kcal/macro targets, dietary restrictions, free-form preferences, and any custom system prompt.",
		Schema:      schema,
		Handler: func(ctx context.Context, _ json.RawMessage) (json.RawMessage, ToolEffect, error) {
			p, err := svc.Profile.Get(ctx)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(profileMap(p)), ToolEffectNone, nil
		},
	}
}

// --- write tools ---

// quantitySchema is the JSON schema of a per-eater component quantity:
// portions for a composed food, amount + unit for a leaf food.
const quantitySchema = `
        "portions":{"type":"number","exclusiveMinimum":0},
        "amount":{"type":"number","exclusiveMinimum":0},
        "unit":{"type":"string","minLength":1}`

// quantityHelp tells the model how to fill quantitySchema.
const quantityHelp = "Quantities are per eater: a composed food (recipe) takes portions (REAL, usually 1); a leaf food (ingredient) takes amount + unit (e.g. 150 + \"g\", 1 + \"piece\") and rejects portions."

// quantityInput is the per-eater quantity the plate tools accept.
type quantityInput struct {
	Portions *float64 `json:"portions"`
	Amount   *float64 `json:"amount"`
	Unit     *string  `json:"unit"`
}

func (q quantityInput) empty() bool { return q.Portions == nil && q.Amount == nil && q.Unit == nil }

// component turns the quantity into a plate component. The plate service
// checks it against the food's kind.
func (q quantityInput) component(foodID int64) plate.PlateComponent {
	return plate.PlateComponent{FoodID: foodID, Portions: q.Portions, Amount: q.Amount, Unit: q.Unit}
}

func toolCreatePlate(svc Services) Tool {
	schema := json.RawMessage(fmt.Sprintf(`{
      "type":"object",
      "required":["date","slot_id"],
      "properties":{
        "date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
        "slot_id":{"type":"integer","minimum":1},
        "eaters":{"type":"number","exclusiveMinimum":0,"maximum":%g},
        "note":{"type":"string"}
      },
      "additionalProperties":false
    }`, plate.MaxEaters))
	return Tool{
		Name:        "create_plate",
		Description: "Create an empty plate at a given date (YYYY-MM-DD) and slot. eaters is how many people it serves (defaults to the household's portions); component quantities are per eater. Returns the new plate's id. A cell holds at most one plate; creating into an occupied cell or a No-plate cell fails.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Date   string   `json:"date"`
				SlotID int64    `json:"slot_id"`
				Eaters *float64 `json:"eaters"`
				Note   *string  `json:"note"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			d, err := time.Parse("2006-01-02", in.Date)
			if err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: date: %v", domain.ErrInvalidInput, err)
			}
			if err := guardDate(ctx, d); err != nil {
				return nil, ToolEffectNone, err
			}
			p, err := svc.Plates.Create(ctx, plate.PlateInput{Date: d, SlotID: in.SlotID, Eaters: in.Eaters, Note: in.Note})
			if err != nil {
				return nil, ToolEffectNone, err
			}
			slotKey, err := slotName(ctx, svc, in.SlotID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			noteChange(ctx, "create_plate", map[string]any{"date": in.Date, "slot": slotKey})
			return mustJSON(plateSummary(p)), ToolEffectNone, nil
		},
	}
}

func toolAddFoodToPlate(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["plate_id","food_id"],
      "properties":{
        "plate_id":{"type":"integer","minimum":1},
        "food_id":{"type":"integer","minimum":1},` + quantitySchema + `
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "add_food_to_plate",
		Description: "Append a food (leaf or composed) to a plate. " + quantityHelp + " A composed food without portions gets 1 per eater and its own fresh cook session.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PlateID int64 `json:"plate_id"`
				FoodID  int64 `json:"food_id"`
				quantityInput
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			if err := guardPlateDate(ctx, svc, in.PlateID); err != nil {
				return nil, ToolEffectNone, err
			}
			p, err := svc.Plates.AddComponent(ctx, in.PlateID, in.component(in.FoodID))
			if err != nil {
				return nil, ToolEffectNone, err
			}
			added := lastComponent(p)
			noteChange(ctx, "add_food", map[string]any{"name": added.FoodName, "date": p.DateString()})
			return mustJSON(plateComponentMap(added)), ToolEffectNone, nil
		},
	}
}

func toolSwapFood(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["plate_component_id","new_food_id"],
      "properties":{
        "plate_component_id":{"type":"integer","minimum":1},
        "new_food_id":{"type":"integer","minimum":1},` + quantitySchema + `
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "swap_food",
		Description: "Replace the food on an existing plate_component row, preserving sort order. Without a quantity the current one is kept, which must fit the new food's kind. " + quantityHelp,
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PlateComponentID int64 `json:"plate_component_id"`
				NewFoodID        int64 `json:"new_food_id"`
				quantityInput
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			_, old, err := componentOnPlate(ctx, svc, in.PlateComponentID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			var quantity *plate.PlateComponent
			if !in.empty() {
				q := in.component(in.NewFoodID)
				quantity = &q
			}
			p, err := svc.Plates.SwapComponent(ctx, in.PlateComponentID, in.NewFoodID, quantity)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			pc, err := componentByID(p, in.PlateComponentID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			noteChange(ctx, "swap_food", map[string]any{"from": old.FoodName, "name": pc.FoodName, "date": p.DateString()})
			return mustJSON(plateComponentMap(pc)), ToolEffectNone, nil
		},
	}
}

func toolUpdatePlateComponent(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["plate_component_id"],
      "properties":{
        "plate_component_id":{"type":"integer","minimum":1},` + quantitySchema + `
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "update_plate_component",
		Description: "Change the quantity of an existing plate_component row without swapping the food; portions or amount + unit is required. " + quantityHelp,
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PlateComponentID int64 `json:"plate_component_id"`
				quantityInput
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			if in.empty() {
				return nil, ToolEffectNone, fmt.Errorf("%w: portions or amount + unit required", domain.ErrInvalidInput)
			}
			if err := guardComponentDate(ctx, svc, in.PlateComponentID); err != nil {
				return nil, ToolEffectNone, err
			}
			p, err := svc.Plates.UpdateComponentQuantity(ctx, in.PlateComponentID, in.component(0))
			if err != nil {
				return nil, ToolEffectNone, err
			}
			pc, err := componentByID(p, in.PlateComponentID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			noteChange(ctx, "update_component", map[string]any{"name": pc.FoodName, "date": p.DateString()})
			return mustJSON(plateComponentMap(pc)), ToolEffectNone, nil
		},
	}
}

func toolRemovePlateComponent(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["plate_component_id"],
      "properties":{"plate_component_id":{"type":"integer","minimum":1}},
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "remove_plate_component",
		Description: "Remove a food from a plate without deleting the plate itself.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PlateComponentID int64 `json:"plate_component_id"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			p, pc, err := componentOnPlate(ctx, svc, in.PlateComponentID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			if _, err := svc.Plates.RemoveComponent(ctx, in.PlateComponentID); err != nil {
				return nil, ToolEffectNone, err
			}
			noteChange(ctx, "remove_component", map[string]any{"name": pc.FoodName, "date": p.DateString()})
			return mustJSON(map[string]any{"removed": true}), ToolEffectNone, nil
		},
	}
}

func toolDeletePlate(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["plate_id"],
      "properties":{"plate_id":{"type":"integer","minimum":1}},
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "delete_plate",
		Description: "Delete a plate (cascades to its components).",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PlateID int64 `json:"plate_id"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			p, err := svc.Plates.Get(ctx, in.PlateID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			if err := guardDate(ctx, p.Date); err != nil {
				return nil, ToolEffectNone, err
			}
			slotKey, err := slotName(ctx, svc, p.SlotID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			if _, err := svc.Plates.Delete(ctx, in.PlateID); err != nil {
				return nil, ToolEffectNone, err
			}
			noteChange(ctx, "delete_plate", map[string]any{"name": plateNames(p), "date": p.DateString(), "slot": slotKey})
			return mustJSON(map[string]any{"deleted": true}), ToolEffectNone, nil
		},
	}
}

func toolRecordPreference(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["key","value"],
      "properties":{
        "key":{"type":"string","minLength":1,"maxLength":64,"pattern":"^[a-zA-Z0-9_.-]+$"},
        "value":{}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "record_preference",
		Description: "Persist a single preference key/value into the user's profile (e.g. 'likes_spicy'=true, 'prefers_quick_meals'=true). Overwrites existing key.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Key   string          `json:"key"`
				Value json.RawMessage `json:"value"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			if reservedPreference(in.Key) {
				return nil, ToolEffectNone, fmt.Errorf("%w: %s is kept by plate feedback", domain.ErrAIPreferenceReserved, in.Key)
			}
			p, err := svc.Profile.Get(ctx)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			if p.Preferences == nil {
				p.Preferences = map[string]any{}
			}
			var v any
			if err := json.Unmarshal(in.Value, &v); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: value: %v", domain.ErrInvalidInput, err)
			}
			p.Preferences[in.Key] = v
			if _, err := svc.Profile.Update(ctx, p); err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(map[string]any{"recorded": true, "key": in.Key}), ToolEffectNone, nil
		},
	}
}

// reservedPreference reports whether key is likes or dislikes (or below
// them), which plate feedback owns (D9).
func reservedPreference(key string) bool {
	for _, r := range []string{"likes", "dislikes"} {
		if key == r || strings.HasPrefix(key, r+".") {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// helpers shared by handlers
// ---------------------------------------------------------------------------

// guardDate fails with domain.ErrAIPastDate when d is before the chat's
// today: past days are read-only for the agent (D7). Every plan-writing
// tool checks the dates it writes.
func guardDate(ctx context.Context, d time.Time) error {
	today := todayFrom(ctx)
	if d.Before(today) {
		return fmt.Errorf("%w: %s is before today (%s)", domain.ErrAIPastDate, d.Format(time.DateOnly), today.Format(time.DateOnly))
	}
	return nil
}

// guardPlateDate is guardDate on the date of plate id.
func guardPlateDate(ctx context.Context, svc Services, id int64) error {
	p, err := svc.Plates.Get(ctx, id)
	if err != nil {
		return err
	}
	return guardDate(ctx, p.Date)
}

// componentOnPlate returns the component id with its plate (food names
// filled) after guardDate on the plate's date.
func componentOnPlate(ctx context.Context, svc Services, id int64) (*plate.Plate, *plate.PlateComponent, error) {
	c, err := svc.Plates.GetComponent(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	p, err := svc.Plates.Get(ctx, c.PlateID)
	if err != nil {
		return nil, nil, err
	}
	if err := guardDate(ctx, p.Date); err != nil {
		return nil, nil, err
	}
	pc, err := componentByID(p, id)
	return p, pc, err
}

// slotName is the slot's i18n name key, the slot's display name in a change
// line.
func slotName(ctx context.Context, svc Services, id int64) (string, error) {
	t, err := svc.Slots.Get(ctx, id)
	if err != nil {
		return "", err
	}
	return t.NameKey, nil
}

// guardComponentDate is guardDate on the date of the component's plate.
func guardComponentDate(ctx context.Context, svc Services, id int64) error {
	pc, err := svc.Plates.GetComponent(ctx, id)
	if err != nil {
		return err
	}
	return guardPlateDate(ctx, svc, pc.PlateID)
}

func foodSummary(f *food.Food, defaultKeep int) map[string]any {
	role := ""
	if f.Role != nil {
		role = string(*f.Role)
	}
	ref := float64(0)
	if f.ReferencePortions != nil {
		ref = *f.ReferencePortions
	}
	out := map[string]any{
		"id":                 f.ID,
		"name":               f.Name,
		"kind":               string(f.Kind),
		"role":               role,
		"reference_portions": ref,
		"tags":               f.Tags,
		"favorite":           f.Favorite,
		"diet_class":         string(f.DietClass),
	}
	addHistory(out, f, defaultKeep)
	return out
}

func foodDetail(f *food.Food, defaultKeep int) map[string]any {
	children := make([]map[string]any, len(f.Children))
	for i, ch := range f.Children {
		children[i] = map[string]any{
			"child_id":   ch.ChildID,
			"child_name": ch.ChildName,
			"child_kind": string(ch.ChildKind),
			"amount":     ch.Amount,
			"unit":       ch.Unit,
			"grams":      ch.Grams,
		}
	}
	instrs := make([]map[string]any, len(f.Instructions))
	for i, inst := range f.Instructions {
		instrs[i] = map[string]any{"step_number": inst.StepNumber, "text": inst.Text}
	}
	out := map[string]any{
		"id":         f.ID,
		"name":       f.Name,
		"kind":       string(f.Kind),
		"tags":       f.Tags,
		"favorite":   f.Favorite,
		"diet_class": string(f.DietClass),
	}
	addHistory(out, f, defaultKeep)
	if f.Role != nil {
		out["role"] = string(*f.Role)
	}
	if f.ReferencePortions != nil {
		out["reference_portions"] = *f.ReferencePortions
	}
	if f.PrepMinutes != nil {
		out["prep_minutes"] = *f.PrepMinutes
	}
	if f.CookMinutes != nil {
		out["cook_minutes"] = *f.CookMinutes
	}
	if f.Notes != nil {
		out["notes"] = *f.Notes
	}
	if len(children) > 0 {
		out["children"] = children
	}
	if len(instrs) > 0 {
		out["instructions"] = instrs
	}
	if f.Kind == food.KindLeaf {
		out["kcal_100g"] = derefF(f.Kcal100g)
		out["protein_100g"] = derefF(f.Protein100g)
		out["fat_100g"] = derefF(f.Fat100g)
		out["carbs_100g"] = derefF(f.Carbs100g)
		out["fiber_100g"] = derefF(f.Fiber100g)
		out["sodium_100g"] = derefF(f.Sodium100g)
	}
	return out
}

// addHistory writes a composed food's last_eaten (YYYY-MM-DD or nil),
// times_cooked, effective keep_days (defaultKeep when the recipe sets none)
// and freezable (nil when unset) into out.
func addHistory(out map[string]any, f *food.Food, defaultKeep int) {
	if f.Kind != food.KindComposed {
		return
	}
	var lastEaten any
	if f.History.LastEaten != nil {
		lastEaten = f.History.LastEaten.Format(time.DateOnly)
	}
	out["last_eaten"] = lastEaten
	out["times_cooked"] = f.History.TimesCooked
	out["keep_days"] = plate.BatchSettings{DefaultKeepDays: defaultKeep}.KeepDays(f.KeepDays)
	out["freezable"] = boolOrNil(f.Freezable)
}

// defaultKeepDays reads plan.default_keep_days.
func defaultKeepDays(ctx context.Context, svc Services) (int, error) {
	return plate.IntSetting(ctx, svc.Settings, settings.KeyPlanDefaultKeepDays)
}

func boolOrNil(p *bool) any {
	if p == nil {
		return nil
	}
	return *p
}

// addPerPortion writes per_portion, basis, nutrition_complete and (when
// incomplete) missing_nutrition into out. On error out is left untouched.
// Leaf macros are per 100 g; composed macros are per serving.
func addPerPortion(ctx context.Context, svc Services, out map[string]any, f *food.Food) error {
	d, err := svc.NutritionResolver.PerPortionDetailed(ctx, f.ID)
	if err != nil {
		return err
	}
	out["per_portion"] = map[string]any{
		"kcal":    round1(d.Macros.Kcal),
		"protein": round1(d.Macros.Protein),
		"fat":     round1(d.Macros.Fat),
		"carbs":   round1(d.Macros.Carbs),
	}
	out["basis"] = "per_portion"
	if f.Kind == food.KindLeaf {
		out["basis"] = "per_100g"
	}
	out["nutrition_complete"] = d.Complete
	if !d.Complete {
		names := make([]string, len(d.Missing))
		for i, m := range d.Missing {
			names[i] = m.Name
		}
		out["missing_nutrition"] = names
	}
	return nil
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

func derefF(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

func plateSummary(p *plate.Plate) map[string]any {
	comps := make([]map[string]any, len(p.Components))
	for i := range p.Components {
		comps[i] = plateComponentMap(&p.Components[i])
	}
	return map[string]any{
		"id": p.ID, "date": p.DateString(), "slot_id": p.SlotID, "eaters": p.Eaters,
		"note": p.Note, "state": string(p.State), "ai_run_id": p.AIRunID, "reason": p.Reason,
		"components": comps,
	}
}

// linkedSessionList renders the cook sessions the plates' components eat
// from, ascending by id.
func linkedSessionList(ctx context.Context, svc Services, plates []plate.Plate) ([]map[string]any, error) {
	var ids []int64
	for _, p := range plates {
		for _, c := range p.Components {
			if c.CookSessionID != nil {
				ids = append(ids, *c.CookSessionID)
			}
		}
	}
	slices.Sort(ids)
	sessions, err := svc.Plates.CookSessions(ctx, slices.Compact(ids))
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, map[string]any{
			"id": sess.ID, "food_id": sess.FoodID, "cook_date": sess.CookDate.Format(time.DateOnly),
			"cook_slot_id": sess.CookSlotID, "portions": sess.Portions, "frozen_portions": sess.FrozenPortions,
			"state": string(sess.State),
		})
	}
	return out, nil
}

// plateComponentMap renders a component. Quantities are per eater.
func plateComponentMap(pc *plate.PlateComponent) map[string]any {
	out := map[string]any{
		"id": pc.ID, "plate_id": pc.PlateID, "food_id": pc.FoodID,
		"sort_order": pc.SortOrder,
	}
	if pc.Portions != nil {
		out["portions"] = *pc.Portions
	}
	if pc.Amount != nil {
		out["amount"] = *pc.Amount
	}
	if pc.Unit != nil {
		out["unit"] = *pc.Unit
	}
	if pc.Grams != nil {
		out["grams"] = *pc.Grams
	}
	if pc.GramsSource != nil {
		out["grams_source"] = *pc.GramsSource
	}
	if pc.CookSessionID != nil {
		out["cook_session_id"] = *pc.CookSessionID
	}
	return out
}

// lastComponent returns the plate's component with the highest sort order:
// the one AddComponent appended.
func lastComponent(p *plate.Plate) *plate.PlateComponent {
	var last *plate.PlateComponent
	for i := range p.Components {
		if last == nil || p.Components[i].SortOrder >= last.SortOrder {
			last = &p.Components[i]
		}
	}
	return last
}

// componentByID returns the plate's component id, or an error wrapping
// domain.ErrNotFound when the plate does not hold it.
func componentByID(p *plate.Plate, id int64) (*plate.PlateComponent, error) {
	for i := range p.Components {
		if p.Components[i].ID == id {
			return &p.Components[i], nil
		}
	}
	return nil, fmt.Errorf("%w: plate_component %d on plate %d", domain.ErrNotFound, id, p.ID)
}

func profileMap(p *profile.Profile) map[string]any {
	return map[string]any{
		"kcal_target":          p.KcalTarget,
		"protein_pct":          p.ProteinPct,
		"fat_pct":              p.FatPct,
		"carbs_pct":            p.CarbsPct,
		"dietary_restrictions": p.DietaryRestrictions,
		"preferences":          p.Preferences,
		"system_prompt":        p.SystemPrompt,
		"locale":               p.Locale,
		"household_portions":   p.HouseholdPortions,
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("agent tools: json.Marshal: %v", err))
	}
	return b
}
