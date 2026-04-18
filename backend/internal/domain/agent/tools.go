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
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
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
	Components *component.Service
	Planner    *planner.Service
	Plates     *plate.Service
	Profile    *profile.Service
	Slots      *slot.Service
	Ingredient ingredient.Repository
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
		toolListComponents(svc),
		toolGetComponent(svc),
		toolListSlots(svc),
		toolGetWeek(svc),
		toolGetWeekNutrition(svc),
		toolGetProfile(svc),
		toolCreatePlate(svc),
		toolAddComponentToPlate(svc),
		toolSwapComponent(svc),
		toolUpdatePlateComponent(svc),
		toolRemovePlateComponent(svc),
		toolDeletePlate(svc),
		toolClearWeek(svc),
		toolRecordPreference(svc),
	}
}

// --- read tools ---

func toolListComponents(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "properties":{
        "search":{"type":"string"},
        "role":{"type":"string","enum":["main","side_starch","side_veg","side_protein","sauce","drink","dessert","standalone"]},
        "tag":{"type":"string"},
        "limit":{"type":"integer","minimum":1,"maximum":200},
        "offset":{"type":"integer","minimum":0},
        "sort_by":{"type":"string","enum":["name","created","last_cooked"]},
        "sort_desc":{"type":"boolean"}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "list_components",
		Description: "List components (dishes) in the library. Filter by role, tag, or search substring. Use this to find a component by name before referencing it by id.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Search   string `json:"search"`
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
			res, err := svc.Components.List(ctx, component.ListQuery{
				Search: in.Search, Role: in.Role, Tag: in.Tag,
				Limit: in.Limit, Offset: in.Offset,
				SortBy: in.SortBy, SortDesc: in.SortDesc,
			})
			if err != nil {
				return nil, ToolEffectNone, err
			}
			items := make([]map[string]any, 0, len(res.Items))
			for _, c := range res.Items {
				items = append(items, componentSummary(&c))
			}
			return mustJSON(map[string]any{"items": items, "total": res.Total}), ToolEffectNone, nil
		},
	}
}

func toolGetComponent(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["component_id"],
      "properties":{"component_id":{"type":"integer","minimum":1}},
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "get_component",
		Description: "Fetch a component (dish) by id, with ingredients, instructions, role, and tags.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				ComponentID int64 `json:"component_id"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			c, err := svc.Components.Get(ctx, in.ComponentID)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(componentDetail(c)), ToolEffectNone, nil
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

func toolGetWeek(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "properties":{"week_id":{"type":"integer","minimum":1}},
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "get_week",
		Description: "Get a week with all its plates (day + slot + component list). If week_id is omitted, returns the current ISO week.",
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
			return mustJSON(weekSummary(week)), ToolEffectNone, nil
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
      "required":["week_id","day","slot_id"],
      "properties":{
        "week_id":{"type":"integer","minimum":1},
        "day":{"type":"integer","minimum":0,"maximum":6},
        "slot_id":{"type":"integer","minimum":1},
        "note":{"type":"string"}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "create_plate",
		Description: "Create an empty plate at a given day (0=Mon..6=Sun) and slot in a week. Returns the new plate's id.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				WeekID int64   `json:"week_id"`
				Day    int     `json:"day"`
				SlotID int64   `json:"slot_id"`
				Note   *string `json:"note"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			p := &plate.Plate{WeekID: in.WeekID, Day: in.Day, SlotID: in.SlotID, Note: in.Note}
			if err := svc.Plates.Create(ctx, p); err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(plateSummary(p)), ToolEffectPlateChanged, nil
		},
	}
}

func toolAddComponentToPlate(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["plate_id","component_id"],
      "properties":{
        "plate_id":{"type":"integer","minimum":1},
        "component_id":{"type":"integer","minimum":1},
        "portions":{"type":"number","exclusiveMinimum":0}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "add_component_to_plate",
		Description: "Append a component (dish) to a plate with the given portion count (defaults to 1).",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PlateID     int64   `json:"plate_id"`
				ComponentID int64   `json:"component_id"`
				Portions    float64 `json:"portions"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			pc, err := svc.Plates.AddComponent(ctx, in.PlateID, in.ComponentID, in.Portions)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(plateComponentMap(pc)), ToolEffectPlateChanged, nil
		},
	}
}

func toolSwapComponent(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "required":["plate_component_id","new_component_id"],
      "properties":{
        "plate_component_id":{"type":"integer","minimum":1},
        "new_component_id":{"type":"integer","minimum":1},
        "portions":{"type":"number","exclusiveMinimum":0}
      },
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "swap_component",
		Description: "Replace the component on an existing plate_component row, preserving sort order. Optionally override portions.",
		Schema:      schema,
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PlateComponentID int64    `json:"plate_component_id"`
				NewComponentID   int64    `json:"new_component_id"`
				Portions         *float64 `json:"portions"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			pc, err := svc.Plates.SwapComponent(ctx, in.PlateComponentID, in.NewComponentID, in.Portions)
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
		Description: "Change the portion count on an existing plate_component row without swapping the component.",
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
		Description: "Remove a component from a plate without deleting the plate itself.",
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

func componentSummary(c *component.Component) map[string]any {
	return map[string]any{
		"id":                 c.ID,
		"name":               c.Name,
		"role":               string(c.Role),
		"reference_portions": c.ReferencePortions,
		"tags":               c.Tags,
	}
}

func componentDetail(c *component.Component) map[string]any {
	ings := make([]map[string]any, len(c.Ingredients))
	for i, ci := range c.Ingredients {
		ings[i] = map[string]any{
			"ingredient_id": ci.IngredientID, "amount": ci.Amount,
			"unit": ci.Unit, "grams": ci.Grams,
		}
	}
	instrs := make([]map[string]any, len(c.Instructions))
	for i, inst := range c.Instructions {
		instrs[i] = map[string]any{"step_number": inst.StepNumber, "text": inst.Text}
	}
	return map[string]any{
		"id":                 c.ID,
		"name":               c.Name,
		"role":               string(c.Role),
		"variant_group_id":   c.VariantGroupID,
		"reference_portions": c.ReferencePortions,
		"prep_minutes":       c.PrepMinutes,
		"cook_minutes":       c.CookMinutes,
		"notes":              c.Notes,
		"tags":               c.Tags,
		"ingredients":        ings,
		"instructions":       instrs,
	}
}

func weekSummary(w *planner.Week) map[string]any {
	plates := make([]map[string]any, len(w.Plates))
	for i, p := range w.Plates {
		plates[i] = plateSummary(&p)
	}
	return map[string]any{
		"id": w.ID, "year": w.Year, "week_number": w.WeekNumber,
		"plates": plates,
	}
}

func plateSummary(p *plate.Plate) map[string]any {
	comps := make([]map[string]any, len(p.Components))
	for i, pc := range p.Components {
		comps[i] = plateComponentMap(&pc)
	}
	return map[string]any{
		"id": p.ID, "week_id": p.WeekID, "day": p.Day, "slot_id": p.SlotID,
		"note": p.Note, "components": comps,
	}
}

func plateComponentMap(pc *plate.PlateComponent) map[string]any {
	return map[string]any{
		"id": pc.ID, "plate_id": pc.PlateID, "component_id": pc.ComponentID,
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

// weekNutritionTotals computes per-day and week-total macros for a week's plates
// using the same pipeline as the HTTP /api/weeks/{id}/nutrition handler.
func weekNutritionTotals(ctx context.Context, svc Services, w *planner.Week) (*nutrition.WeekTotalsResult, error) {
	compIDs := map[int64]struct{}{}
	for _, p := range w.Plates {
		for _, pc := range p.Components {
			compIDs[pc.ComponentID] = struct{}{}
		}
	}
	comps := make(map[int64]*component.Component, len(compIDs))
	ingIDs := map[int64]struct{}{}
	for id := range compIDs {
		c, err := svc.Components.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		comps[id] = c
		for _, ci := range c.Ingredients {
			ingIDs[ci.IngredientID] = struct{}{}
		}
	}
	ids := make([]int64, 0, len(ingIDs))
	for id := range ingIDs {
		ids = append(ids, id)
	}
	ingMap, err := svc.Ingredient.LookupForNutrition(ctx, ids)
	if err != nil {
		return nil, err
	}
	perPortion := make(map[int64]nutrition.Macros, len(comps))
	for id, c := range comps {
		inputs := make([]nutrition.IngredientInput, 0, len(c.Ingredients))
		for _, ci := range c.Ingredients {
			ing, ok := ingMap[ci.IngredientID]
			if !ok {
				continue
			}
			inputs = append(inputs, nutrition.IngredientInput{
				Per100g: nutrition.Macros{
					Kcal: ing.Kcal100g, Protein: ing.Protein100g, Fat: ing.Fat100g,
					Carbs: ing.Carbs100g, Fiber: ing.Fiber100g, Sodium: ing.Sodium100g,
				},
				Grams: ci.Grams,
			})
		}
		perPortion[id] = nutrition.PerPortion(nutrition.ComponentInput{
			Ingredients: inputs, ReferencePortions: c.ReferencePortions,
		})
	}
	dayPlates := make([]nutrition.DayPlate, 0, len(w.Plates))
	for _, p := range w.Plates {
		cis := make([]nutrition.PlateComponentInput, 0, len(p.Components))
		for _, pc := range p.Components {
			m, ok := perPortion[pc.ComponentID]
			if !ok {
				continue
			}
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
		// Encoding plain maps of primitives never errors; this is a true bug if hit.
		panic(fmt.Sprintf("agent tools: json.Marshal: %v", err))
	}
	return b
}
