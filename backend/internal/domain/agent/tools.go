package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/xeipuuv/gojsonschema"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
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
	// CodeCallable: the model may also call the tool from code it runs
	// (programmatic tool calling). Only read and dry-run tools are.
	CodeCallable bool
}

// codeCallableTools are the tools code may call: the reads and apply_changes,
// which from code must be a dry run.
var codeCallableTools = map[string]bool{"find_foods": true, "get_plan": true, applyChangesName: true}

// ErrCodeCallRefused is returned when code the model ran calls a tool it may
// not call (allowed_callers is guidance, not a security boundary).
var ErrCodeCallRefused = errors.New("tool cannot be called from code")

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
	// State reads the planning state of a window (planning.Builder.Build over
	// the window) and Tx is the unit of work: apply_changes evaluates the
	// plan with them and rolls a dry run back.
	State func(ctx context.Context, w window.Window, today time.Time) (planning.State, error)
	Tx    domain.Transactor
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
		if t.CodeCallable {
			out[i].AllowedCallers = []string{llm.CallerDirect, llm.CallerCodeExecution}
		}
	}
	return out
}

// withoutCallers returns tools without their allowed callers (programmatic
// tool calling is off).
func withoutCallers(tools []llm.Tool) []llm.Tool {
	out := make([]llm.Tool, len(tools))
	for i, t := range tools {
		t.AllowedCallers = nil
		out[i] = t
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
	if llm.CallerFrom(ctx).FromCode() && !t.CodeCallable {
		return nil, ToolEffectNone, fmt.Errorf("%w: %s (it changes the plan or the conversation: call it directly)", ErrCodeCallRefused, name)
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
		toolFindFoods(svc),
		toolListSlots(svc),
		toolGetProfile(svc),
		toolApplyChanges(svc),
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
	if svc.Planner != nil {
		tools = append(tools, toolRegenerate(svc))
	}
	for i := range tools {
		tools[i].CodeCallable = codeCallableTools[tools[i].Name]
		if effectTools[tools[i].Name] {
			tools[i].Handler = withIssueBaseline(svc, tools[i].Handler)
		}
	}
	return tools
}

// --- read tools ---

func toolListSlots(svc Services) Tool {
	schema := json.RawMessage(`{
      "type":"object",
      "properties":{"active_only":{"type":"boolean"}},
      "additionalProperties":false
    }`)
	return Tool{
		Name:        "list_slots",
		Description: "List the user-defined meal time slots (breakfast, lunch, dinner, etc.). Required before creating plates — the slot_id must come from this list. Returns JSON {items:[{id, name_key, icon, sort_order, active}]}.",
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

func toolGetProfile(svc Services) Tool {
	schema := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	return Tool{
		Name:        "get_profile",
		Description: "Read the user's profile: kcal/macro targets, dietary restrictions, free-form preferences, and any custom system prompt. Returns JSON {kcal_target, protein_pct, fat_pct, carbs_pct, protein_g_per_kg, weight_kg (numbers or null), dietary_restrictions:[string], preferences:{key:value}, system_prompt, locale, household_portions}.",
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
		Description: "Persist a single preference key/value into the user's profile (e.g. 'likes_spicy'=true, 'prefers_quick_meals'=true). Overwrites existing key. Returns JSON {recorded:true, key}.",
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

// defaultKeepDays reads plan.default_keep_days.
func defaultKeepDays(ctx context.Context, svc Services) (int, error) {
	return plate.IntSetting(ctx, svc.Settings, settings.KeyPlanDefaultKeepDays)
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

// plateComponentMap renders a component. Quantities are per eater.
func plateComponentMap(pc *plate.PlateComponent) map[string]any {
	out := map[string]any{
		"id": pc.ID, "plate_id": pc.PlateID, "food_id": pc.FoodID,
		"sort_order": pc.SortOrder, "name": pc.FoodName,
	}
	if pc.FoodRole != nil && *pc.FoodRole != "" {
		out["role"] = *pc.FoodRole
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
		out["session_id"] = *pc.CookSessionID
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
		"protein_g_per_kg":     p.ProteinGPerKg,
		"weight_kg":            p.WeightKg,
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

// searchPage keeps the foods of res whose name or tags match every word of
// search (folded, synonyms expanded: diet.SearchAlternatives) and cuts the
// page. The order of res is kept.
func searchPage(res *food.ListResult, search string, limit, offset int) *food.ListResult {
	var words [][]diet.Alt
	for _, w := range strings.Fields(search) {
		words = append(words, diet.SearchAlternatives(w))
	}
	var hits []food.Food
	for _, f := range res.Items {
		hay := diet.Fold(f.Name + " " + strings.Join(f.Tags, " "))
		ok := true
		for _, alts := range words {
			if !diet.MatchesWord(hay, alts) {
				ok = false
				break
			}
		}
		if ok {
			hits = append(hits, f)
		}
	}
	total := len(hits)
	if limit <= 0 {
		limit = 50
	}
	if offset > len(hits) {
		offset = len(hits)
	}
	hits = hits[offset:]
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return &food.ListResult{Items: hits, Total: total}
}

// sessionIDOf lists the cook session a component eats from (none for a leaf).
func sessionIDOf(pc *plate.PlateComponent) []int64 {
	if pc == nil || pc.CookSessionID == nil {
		return nil
	}
	return []int64{*pc.CookSessionID}
}

// effectTools are the tools whose result carries an `after` block; they get
// the window's issues snapshotted before they run.
var effectTools = map[string]bool{
	"confirm": true, "unlock": true, "regenerate": true, "apply_preset": true,
}
