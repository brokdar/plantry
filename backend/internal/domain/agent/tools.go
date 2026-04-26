package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xeipuuv/gojsonschema"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/template"
)

// ToolEffect classifies side-effects of a tool invocation. The agent loop uses
// this to decide when to emit plate_changed events to the SSE stream.
type ToolEffect int

const (
	// ToolEffectNone is a read-only tool (no cache invalidation needed).
	ToolEffectNone ToolEffect = iota
	// ToolEffectPlateChanged means the call mutated the plan (week/plate/plate_component).
	ToolEffectPlateChanged
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
}

// Services aggregates the domain services the default tool set depends on.
type Services struct {
	Foods             *food.Service
	NutritionResolver *food.NutritionResolver
	Planner           *planner.Service
	Plates            *plate.Service
	Profile           *profile.Service
	Slots             *slot.Service
	Templates         *template.Service
}

// NewToolSet builds the default tool set wired to the given services.
func NewToolSet(svc Services) (*ToolSet, error) {
	ts := &ToolSet{
		byName:  map[string]*Tool{},
		schemas: map[string]*gojsonschema.Schema{},
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
func (s *ToolSet) Execute(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
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
	return []Tool{
		toolListFoods(svc),
		toolGetFood(svc),
		toolListSlots(svc),
		toolGetPlatesRange(svc),
		toolGetWeekNutrition(svc),
		toolGetProfile(svc),
		toolCreatePlate(svc),
		toolApplyTemplate(svc),
		toolAddFoodToPlate(svc),
		toolSwapFood(svc),
		toolUpdatePlateComponent(svc),
		toolRemovePlateComponent(svc),
		toolDeletePlate(svc),
		toolClearWeek(svc),
		toolRecordPreference(svc),
	}
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
        "sort_by":{"type":"string","enum":["name","created_at","last_cooked_at"]},
        "sort_desc":{"type":"boolean"}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "list_foods",
		Description: "List foods in the library. A food is either a LEAF (single edible item with direct nutrition — apple, rice, chicken) or COMPOSED (built from child foods — curry, schnitzel). Filter by kind, role, tag, or search substring. Use this to find a food by name before referencing it by id.",
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
			items := make([]map[string]any, 0, len(res.Items))
			for i := range res.Items {
				items = append(items, foodSummary(&res.Items[i]))
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
		Description: "Fetch a food by id. Composed foods include children (child food + grams), instructions, and tags; leaf foods include direct nutrition and portion overrides.",
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
			return mustJSON(foodDetail(f)), ToolEffectNone, nil
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
		Description: "Get all plates for a date range (from and to inclusive, format YYYY-MM-DD). Use this instead of get_week to read plates by calendar date.",
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
			return mustJSON(map[string]any{"plates": items}), ToolEffectNone, nil
		},
	}
}

func toolGetWeekNutrition(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "properties":{"week_id":{"type":"integer","minimum":1}},
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "get_week_nutrition",
		Description: "Compute per-day and week-total macros (kcal, protein, fat, carbs, fiber, sodium) for a week's plates.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				WeekID int64 `json:"week_id"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			week, err := resolveWeek(ctx, svc, in.WeekID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			totals, err := weekNutritionTotals(ctx, svc, week)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			days := make([]map[string]any, 0, len(totals.Days))
			for day, m := range totals.Days {
				days = append(days, map[string]any{"day": day, "macros": macrosMap(m)})
			}
			sort.Slice(days, func(i, j int) bool {
				return days[i]["day"].(int) < days[j]["day"].(int)
			})
			return mustJSON(map[string]any{
				"week_id": week.ID,
				"days":    days,
				"week":    macrosMap(totals.Week),
			}), ToolEffectNone, nil
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

func toolCreatePlate(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["date","slot_id"],
      "properties":{
        "date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
        "slot_id":{"type":"integer","minimum":1},
        "note":{"type":"string"}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "create_plate",
		Description: "Create an empty plate at a given date (YYYY-MM-DD) and slot. Returns the new plate's id.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Date   string  `json:"date"`
				SlotID int64   `json:"slot_id"`
				Note   *string `json:"note"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			d, err := time.Parse("2006-01-02", in.Date)
			if err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: date: %v", domain.ErrInvalidInput, err)
			}
			p := &plate.Plate{Date: d, SlotID: in.SlotID, Note: in.Note}
			if err := svc.Plates.Create(ctx, p); err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(plateSummary(p)), ToolEffectPlateChanged, nil
		},
	}
}

func toolApplyTemplate(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["template_id","start_date","slot_id"],
      "properties":{
        "template_id":{"type":"integer","minimum":1},
        "start_date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
        "slot_id":{"type":"integer","minimum":1}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "apply_template",
		Description: "Apply a meal template starting from a given date. Creates one plate per day_offset entry in the template at start_date + offset, using the given slot.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				TemplateID int64  `json:"template_id"`
				StartDate  string `json:"start_date"`
				SlotID     int64  `json:"slot_id"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			startDate, err := time.Parse("2006-01-02", in.StartDate)
			if err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: start_date: %v", domain.ErrInvalidInput, err)
			}
			plates, err := svc.Templates.Apply(ctx, in.TemplateID, startDate, in.SlotID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(map[string]any{"plates_created": len(plates), "effect": "plate_changed"}), ToolEffectPlateChanged, nil
		},
	}
}

func toolAddFoodToPlate(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["plate_id","food_id"],
      "properties":{
        "plate_id":{"type":"integer","minimum":1},
        "food_id":{"type":"integer","minimum":1},
        "portions":{"type":"number","exclusiveMinimum":0}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "add_food_to_plate",
		Description: "Append a food (leaf or composed) to a plate with the given portion count (defaults to 1).",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PlateID  int64   `json:"plate_id"`
				FoodID   int64   `json:"food_id"`
				Portions float64 `json:"portions"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			pc, err := svc.Plates.AddComponent(ctx, in.PlateID, in.FoodID, in.Portions)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(plateComponentMap(pc)), ToolEffectPlateChanged, nil
		},
	}
}

func toolSwapFood(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["plate_component_id","new_food_id"],
      "properties":{
        "plate_component_id":{"type":"integer","minimum":1},
        "new_food_id":{"type":"integer","minimum":1},
        "portions":{"type":"number","exclusiveMinimum":0}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "swap_food",
		Description: "Replace the food on an existing plate_component row, preserving sort order. Optionally override portions.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PlateComponentID int64    `json:"plate_component_id"`
				NewFoodID        int64    `json:"new_food_id"`
				Portions         *float64 `json:"portions"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			pc, err := svc.Plates.SwapComponent(ctx, in.PlateComponentID, in.NewFoodID, in.Portions)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(plateComponentMap(pc)), ToolEffectPlateChanged, nil
		},
	}
}

func toolUpdatePlateComponent(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["plate_component_id","portions"],
      "properties":{
        "plate_component_id":{"type":"integer","minimum":1},
        "portions":{"type":"number","exclusiveMinimum":0}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "update_plate_component",
		Description: "Change the portion count on an existing plate_component row without swapping the food.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PlateComponentID int64   `json:"plate_component_id"`
				Portions         float64 `json:"portions"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			pc, err := svc.Plates.UpdateComponentPortions(ctx, in.PlateComponentID, in.Portions)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(plateComponentMap(pc)), ToolEffectPlateChanged, nil
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
			if err := svc.Plates.RemoveComponent(ctx, in.PlateComponentID); err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(map[string]any{"removed": true}), ToolEffectPlateChanged, nil
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
			if err := svc.Plates.Delete(ctx, in.PlateID); err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(map[string]any{"deleted": true}), ToolEffectPlateChanged, nil
		},
	}
}

func toolClearWeek(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["week_id"],
      "properties":{"week_id":{"type":"integer","minimum":1}},
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "clear_week",
		Description: "Delete every plate in a week. Use for 'replace all' flows.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				WeekID int64 `json:"week_id"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			week, err := svc.Planner.Get(ctx, in.WeekID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			var removed int
			for _, p := range week.Plates {
				if err := svc.Plates.Delete(ctx, p.ID); err != nil {
					return nil, ToolEffectPlateChanged, err
				}
				removed++
			}
			effect := ToolEffectNone
			if removed > 0 {
				effect = ToolEffectPlateChanged
			}
			return mustJSON(map[string]any{"removed": removed}), effect, nil
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
		Description: "Persist a single preference key/value into the user's profile (e.g. 'likes_spicy'=true, 'prefers_quick_meals'=true). Overwrites existing key. DO NOT use the keys 'likes' or 'dislikes' — those arrays are managed automatically by plate-feedback ratings; writing them here clobbers the accumulated tag history.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Key   string          `json:"key"`
				Value json.RawMessage `json:"value"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
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

// ---------------------------------------------------------------------------
// helpers shared by handlers
// ---------------------------------------------------------------------------

func resolveWeek(ctx context.Context, svc Services, weekID int64) (*planner.Week, error) {
	if weekID == 0 {
		return svc.Planner.Current(ctx, time.Now().UTC())
	}
	return svc.Planner.Get(ctx, weekID)
}

func foodSummary(f *food.Food) map[string]any {
	role := ""
	if f.Role != nil {
		role = string(*f.Role)
	}
	ref := float64(0)
	if f.ReferencePortions != nil {
		ref = *f.ReferencePortions
	}
	return map[string]any{
		"id":                 f.ID,
		"name":               f.Name,
		"kind":               string(f.Kind),
		"role":               role,
		"reference_portions": ref,
		"tags":               f.Tags,
	}
}

func foodDetail(f *food.Food) map[string]any {
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
		"id":   f.ID,
		"name": f.Name,
		"kind": string(f.Kind),
		"tags": f.Tags,
	}
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
		"id": p.ID, "week_id": p.WeekID, "day": p.Day, "slot_id": p.SlotID,
		"note": p.Note, "components": comps,
	}
}

func plateComponentMap(pc *plate.PlateComponent) map[string]any {
	return map[string]any{
		"id": pc.ID, "plate_id": pc.PlateID, "food_id": pc.FoodID,
		"portions": pc.Portions, "sort_order": pc.SortOrder,
	}
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
	}
}

func macrosMap(m nutrition.Macros) map[string]any {
	return map[string]any{
		"kcal": m.Kcal, "protein": m.Protein, "fat": m.Fat,
		"carbs": m.Carbs, "fiber": m.Fiber, "sodium": m.Sodium,
	}
}

// weekNutritionTotals computes per-day and week-total macros for a week's
// plates via the recursive food nutrition resolver.
func weekNutritionTotals(ctx context.Context, svc Services, w *planner.Week) (*nutrition.WeekTotalsResult, error) {
	perPortion := map[int64]nutrition.Macros{}
	for _, p := range w.Plates {
		for _, pc := range p.Components {
			if _, ok := perPortion[pc.FoodID]; ok {
				continue
			}
			m, err := svc.NutritionResolver.PerPortion(ctx, pc.FoodID)
			if err != nil {
				return nil, err
			}
			perPortion[pc.FoodID] = m
		}
	}
	dayPlates := make([]nutrition.DayPlate, 0, len(w.Plates))
	for _, p := range w.Plates {
		cis := make([]nutrition.PlateComponentInput, 0, len(p.Components))
		for _, pc := range p.Components {
			m := perPortion[pc.FoodID]
			cis = append(cis, nutrition.PlateComponentInput{Macros: m, Portions: pc.Portions})
		}
		dayPlates = append(dayPlates, nutrition.DayPlate{Day: p.Day, Plate: nutrition.PlateInput{Components: cis}})
	}
	r := nutrition.WeekTotals(dayPlates)
	return &r, nil
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("agent tools: json.Marshal: %v", err))
	}
	return b
}
