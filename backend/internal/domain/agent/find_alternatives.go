package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

const findAlternativesName = "find_alternatives"

const findAlternativesSchema = `{
  "type":"object",
  "required":["plate_id","what"],
  "properties":{
    "plate_id":{"type":"integer","minimum":1},
    "what":{"type":"string","enum":["meal","main","side","add"]},
    "plate_component_id":{"type":"integer","minimum":1,"description":"main or side: the component to replace (else the main, or the only side)"},
    "constraints":{
      "type":"object",
      "properties":{
        "max_minutes":{"type":"integer","minimum":1},
        "diet":{"type":"string","enum":["vegan","vegetarian","pescetarian","omnivore"]},
        "roles":{"type":"array","items":{"type":"string"}},
        "search":{"type":"string","maxLength":100},
        "tags":{"type":"array","items":{"type":"string"}},
        "min_protein_g":{"type":"number","minimum":0},
        "max_kcal":{"type":"number","minimum":0},
        "no_cook":{"type":"boolean"}
      },
      "additionalProperties":false
    },
    "light":{"type":"string","enum":["lighter","heartier"],"description":"lunch or dinner plate: judge the meal as a light (hearty) one and list only candidates whose plate meets its cap (minimum)"},
    "limit":{"type":"integer","minimum":1,"maximum":10}
  },
  "additionalProperties":false
}`

const findAlternativesDescription = `Find verified replacements for one draft plate of the plan window in one call. what: main (a new main, the sides stay), meal (a new main and every other component removed: the result's hint says what it drops), side (plate_component_id, or the plate's only side) or add (a no-cook side, dessert or drink to add; a leaf that needs cooking is never offered). constraints (all optional): max_minutes (fresh cooking time, prep + cook), diet (vegan|vegetarian|pescetarian|omnivore), roles, search (name and tags, as find_foods), tags, min_protein_g and max_kcal (the new component, per eater), no_cook. light lighter|heartier (lunch or dinner plate; needs a kcal target): the meal is judged as light (hearty) and only candidates whose plate meets the light meal cap (hearty meal minimum) are listed. Foods already on the plate are excluded. Each candidate is applied to a copy of the plan and evaluated like an apply_changes check (dry_run true), under the stored light/hearty requests; a candidate that adds a hard issue is dropped. A recipe is offered eaten from an existing batch (session; a batch that exactly covers its meals grows), cooked together with a cell that already cooks (cook_at: the same day or an earlier day within keep days, so the plate's day gets no new cooking occasion), or cooked fresh at the plate. Ranked by fewest new soft issues, no new cooking occasion, (a replaced side: a recipe side of the same role first), session before cook_at before fresh, dish repeats, kcal fit, longest since eaten; each food once. limit default 5, max 10. Saves nothing. The best-ranked alternative is marked recommended: true with recommended_because: present it as your recommendation in one phrase; a food the user removed in this conversation, and an extra that does not fit the dish (cheese, nuts on a curry, tofu on a meat plate), are never listed.
Returns compact JSON (a field that is 0, false or empty is left out) {current?:{food_id, name, role, total_minutes?, food_kcal, plate_kcal, day_kcal, intent? light|hearty, max_kcal? (the most a light meal holds), min_kcal? (the least a filling meal holds)} (the food being replaced, per eater), hint? (what=meal drops the other components), alternatives:[{food_id, name, role, source fresh|session|cook_at|leaf, session_id?, cook_at? {date, slot_id} (where a cook_at source cooks), total_minutes? (fresh or cook_at cooking minutes; cite it for "quicker"), food_kcal, food_protein_g (the new food alone), plate_kcal, plate_protein_g (the whole plate after the change), day_kcal, day_pct, week_pct, cook_occasions_day, new_issues?:[issue], ops:[apply_changes ops]}], empty_reason?: {candidates, dropped?: {"rule label": count}, why?: [plain sentences with the numbers: the floor and the day it would reach, the cap and the plate it would reach]}}. To take one, pass its ops as they are to apply_changes (with your other ops in the same call). An empty list comes with empty_reason: candidates 0 means no food fits the position and constraints (loosen one); otherwise candidates counts tries, not foods (a recipe is tried fresh, as cook_at and once per batch), and dropped counts the rule that blocked each of them (e.g. "cooking occasions per day", "dish variety", "day energy minimum", "light meal"): why says it in words, relay it to the user.`

// findAlternativesHint explains a schema refusal: limit belongs next to
// plate_id, not inside constraints.
func findAlternativesHint(input json.RawMessage) string {
	var in struct {
		Constraints map[string]json.RawMessage `json:"constraints"`
	}
	if json.Unmarshal(input, &in) == nil {
		if _, ok := in.Constraints["limit"]; ok {
			return "put limit at the top level of the call, next to plate_id and what, not inside constraints"
		}
	}
	return ""
}

func toolFindAlternatives(svc Services) Tool {
	return Tool{
		Name:        findAlternativesName,
		Hint:        findAlternativesHint,
		Description: findAlternativesDescription,
		Schema:      json.RawMessage(findAlternativesSchema),
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				PlateID          int64  `json:"plate_id"`
				What             string `json:"what"`
				PlateComponentID int64  `json:"plate_component_id"`
				Constraints      struct {
					MaxMinutes  int      `json:"max_minutes"`
					Diet        string   `json:"diet"`
					Roles       []string `json:"roles"`
					Search      string   `json:"search"`
					Tags        []string `json:"tags"`
					MinProteinG float64  `json:"min_protein_g"`
					MaxKcal     float64  `json:"max_kcal"`
					NoCook      bool     `json:"no_cook"`
				} `json:"constraints"`
				Limit int    `json:"limit"`
				Light string `json:"light"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			if svc.State == nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: find_alternatives is not available (no plan reader)", domain.ErrInvalidInput)
			}
			w, ok := windowFrom(ctx)
			if !ok {
				return nil, ToolEffectNone, fmt.Errorf("%w: no plan window in this conversation turn", domain.ErrInvalidInput)
			}
			c := in.Constraints
			roles := make([]food.Role, len(c.Roles))
			for i, r := range c.Roles {
				roles[i] = food.Role(r)
			}
			req := planning.AltRequest{
				PlateID: in.PlateID, What: in.What, ComponentID: in.PlateComponentID, Limit: in.Limit, Light: in.Light,
				Constraints: planning.AltConstraints{
					MaxMinutes: c.MaxMinutes, Diet: c.Diet, Roles: roles, Search: c.Search, Tags: c.Tags,
					MinProteinG: c.MinProteinG, MaxKcal: c.MaxKcal, NoCook: c.NoCook,
				},
			}
			// The state as apply_changes' dry run sees it: the chat window with
			// the stored light and hearty requests.
			st, err := svc.State(ctx, w, todayFrom(ctx))
			if err != nil {
				return nil, ToolEffectNone, fmt.Errorf("read the plan: %w", err)
			}
			st.Light = planning.MergeLight(st.Light, lightFrom(ctx))
			st.Avoid, st.PairAddOns = svc.Removed.avoid(ctx), true
			res, err := planning.Alternatives(st, planning.WindowTarget(st), req)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			alts := res.Alternatives
			if alts == nil {
				alts = []planning.Alternative{}
			}
			out := map[string]any{"alternatives": planning.Compact(alts)}
			if res.Current != nil {
				out["current"] = planning.Compact(res.Current)
			}
			if res.Hint != "" {
				out["hint"] = res.Hint
			}
			if res.EmptyReason != nil {
				out["empty_reason"] = res.EmptyReason
			}
			return mustJSON(out), ToolEffectNone, nil
		},
	}
}
