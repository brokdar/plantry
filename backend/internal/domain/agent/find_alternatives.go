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
    "limit":{"type":"integer","minimum":1,"maximum":10}
  },
  "additionalProperties":false
}`

const findAlternativesDescription = `Find verified replacements for one draft plate of the plan window in one call. what: meal (a new main, every other component removed), main, side (plate_component_id, or the plate's only side) or add (a side, dessert, drink or no-cook food to add). constraints (all optional): max_minutes (fresh cooking time, prep + cook), diet (vegan|vegetarian|pescetarian|omnivore), roles, search (name and tags, as find_foods), tags, min_protein_g and max_kcal (the new component, per eater), no_cook. Foods already on the plate are excluded. Each candidate is applied to a copy of the plan and evaluated like an apply_changes dry run, under the stored light/hearty requests; a candidate that adds a hard issue is dropped. Ranked by fewest new soft issues, no new cooking occasion, eating an existing batch before cooking fresh, dish repeats, kcal fit, longest since eaten. limit default 5, max 10. Saves nothing.
Returns compact JSON (a field that is 0, false or empty is left out) {alternatives:[{food_id, name, source fresh|session|leaf, session_id?, total_minutes? (fresh cooking minutes; cite it for "quicker"), kcal, protein_g (the plate after the change, per eater), day_kcal, day_pct, week_pct, cook_occasions_day, new_issues?:[issue], ops:[apply_changes ops]}]}. To take one, pass its ops as they are to apply_changes (with your other ops in the same call). An empty list means nothing fits: say so, or loosen a constraint.`

func toolFindAlternatives(svc Services) Tool {
	return Tool{
		Name:        findAlternativesName,
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
				Limit int `json:"limit"`
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
				PlateID: in.PlateID, What: in.What, ComponentID: in.PlateComponentID, Limit: in.Limit,
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
			alts, err := planning.Alternatives(st, planning.WindowTarget(st), req)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			if alts == nil {
				alts = []planning.Alternative{}
			}
			return mustJSON(map[string]any{"alternatives": planning.Compact(alts)}), ToolEffectNone, nil
		},
	}
}
