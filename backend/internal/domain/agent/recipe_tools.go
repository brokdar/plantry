package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/recipechange"
)

// RecipeChanger saves recipe changes (recipechange.Service).
type RecipeChanger interface {
	Variant(ctx context.Context, baseID int64, p recipechange.Proposal) (recipechange.Result, error)
	PreviewEdit(ctx context.Context, id int64, p recipechange.Proposal) (recipechange.Preview, error)
	EditPreviewed(ctx context.Context, id int64, p recipechange.Proposal, pv recipechange.Preview) (recipechange.Result, error)
}

// AttributeSetter sets the assistant's attribute values (flavor.Service).
type AttributeSetter interface {
	SetAuto(ctx context.Context, id int64, p flavor.Patch) (flavor.View, foodevent.Undo, []flavor.Field, error)
}

// RecipeTools are the recipe abilities of the chat (VAR-4, HELP-5). The
// assistant never deletes a recipe; archive stays the user's action.
type RecipeTools struct {
	Changes    RecipeChanger
	Attributes AttributeSetter
}

// Recipe tool names.
const (
	createVariantName = "create_variant"
	editRecipeName    = "edit_recipe"
	setAttributesName = "set_recipe_attributes"
)

const recipeSchema = `"recipe":{"type":"object","required":["name","ingredients","steps","changes"],"properties":{` +
	`"name":{"type":"string","minLength":1},` +
	`"ingredients":{"type":"array","minItems":1,"items":{"type":"string"}},` +
	`"steps":{"type":"array","minItems":1,"items":{"type":"string"}},` +
	`"changes":{"type":"array","minItems":1,"items":{"type":"string"}}},"additionalProperties":false}`

type recipeInput struct {
	RecipeID  int64                 `json:"recipe_id"`
	Recipe    recipechange.Proposal `json:"recipe"`
	Confirmed bool                  `json:"confirmed"`
}

func recipeTools(svc Services) []Tool {
	var out []Tool
	if svc.Recipes.Changes != nil {
		out = append(out, toolCreateVariant(svc.Recipes.Changes), toolEditRecipe(svc.Recipes.Changes))
	}
	if svc.Recipes.Attributes != nil {
		out = append(out, toolSetAttributes(svc.Recipes.Attributes))
	}
	return out
}

func decodeRecipeInput(input json.RawMessage) (recipeInput, error) {
	var in recipeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return in, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	}
	return in, nil
}

// changeOutput is a save's tool output: saved or blocked, with the check.
func changeOutput(res recipechange.Result, extra map[string]any) json.RawMessage {
	out := map[string]any{"saved": res.FoodID != 0, "check": res.Check}
	if res.FoodID != 0 {
		out["recipe_id"], out["changes"], out["undo"] = res.FoodID, res.Changes, res.Undo
	}
	for k, v := range extra {
		out[k] = v
	}
	return mustJSON(out)
}

func toolCreateVariant(rc RecipeChanger) Tool {
	return Tool{
		Name: createVariantName,
		Description: "Save a changed recipe as a variant (\"spicier\"); the original stays. " +
			"Give the whole recipe: lines as \"amount unit food\", steps, changes (one sentence each: what and why). " +
			"saved false: check.blocks says why; tell the user.",
		Schema: json.RawMessage(`{"type":"object","required":["recipe_id","recipe"],"properties":{"recipe_id":{"type":"integer"},` +
			recipeSchema + `},"additionalProperties":false}`),
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			in, err := decodeRecipeInput(input)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			res, err := rc.Variant(ctx, in.RecipeID, in.Recipe)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return changeOutput(res, nil), ToolEffectNone, nil
		},
	}
}

func toolEditRecipe(rc RecipeChanger) Tool {
	return Tool{
		Name: editRecipeName,
		Description: "Change a recipe itself, only when the user explicitly asks; otherwise " + createVariantName + ". " +
			"Same fields. Returns before and after: show them. " +
			"needs_confirmation: confirmed plates hold the recipe; tell the user and call again with confirmed true once they agree.",
		Schema: json.RawMessage(`{"type":"object","required":["recipe_id","recipe"],"properties":{"recipe_id":{"type":"integer"},` +
			recipeSchema + `,"confirmed":{"type":"boolean"}},"additionalProperties":false}`),
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			in, err := decodeRecipeInput(input)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			pv, err := rc.PreviewEdit(ctx, in.RecipeID, in.Recipe)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			diff := map[string]any{"before": pv.Before, "after": pv.After}
			if pv.Check.Blocked() {
				return changeOutput(recipechange.Result{Check: pv.Check}, diff), ToolEffectNone, nil
			}
			if len(pv.ConfirmedDates) > 0 && !in.Confirmed {
				diff["needs_confirmation"], diff["confirmed_dates"] = true, pv.ConfirmedDates
				return changeOutput(recipechange.Result{Check: pv.Check}, diff), ToolEffectNone, nil
			}
			res, err := rc.EditPreviewed(ctx, in.RecipeID, in.Recipe, pv)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return changeOutput(res, diff), ToolEffectNone, nil
		},
	}
}

func toolSetAttributes(as AttributeSetter) Tool {
	return Tool{
		Name: setAttributesName,
		Description: "Set or correct recipe attributes (cuisine, kind, heroes, texture, temperature, saucy, richness, colour, taste, technique, format). " +
			"User-set values stay; kept lists them.",
		Schema: json.RawMessage(`{"type":"object","required":["recipe_id","attributes"],"properties":{"recipe_id":{"type":"integer"},` +
			`"attributes":{"type":"object","minProperties":1}},"additionalProperties":false}`),
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				RecipeID   int64           `json:"recipe_id"`
				Attributes json.RawMessage `json:"attributes"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			p, ok := flavor.DecodePatch(in.Attributes)
			if !ok || len(p.Clear) > 0 {
				return nil, ToolEffectNone, fmt.Errorf("%w: unknown attribute or value", domain.ErrInvalidInput)
			}
			view, undo, kept, err := as.SetAuto(ctx, in.RecipeID, p)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			if kept == nil {
				kept = []flavor.Field{}
			}
			return mustJSON(map[string]any{"saved": true, "attributes": view.Resolved, "kept": kept, "undo": undo}), ToolEffectNone, nil
		},
	}
}
