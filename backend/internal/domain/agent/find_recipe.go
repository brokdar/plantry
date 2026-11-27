package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
)

// FindRequester starts a requested find (finds.Service).
type FindRequester interface {
	Request(ctx context.Context, dish string, trigger finds.Trigger) (*finds.Find, error)
}

// FindTools wire find_recipe (FIND-1c); registered when Requester is set.
type FindTools struct {
	Requester FindRequester
	ChefOn    func(ctx context.Context) (bool, error)
}

func toolFindRecipe(ft FindTools) Tool {
	return Tool{
		Name:        "find_recipe",
		Description: "Look for a new recipe on the web for a dish the user asks for. Runs in the background; it shows under Finds. Returns {started, find_id} or {started:false, reason}.",
		Schema:      json.RawMessage(`{"type":"object","required":["dish"],"properties":{"dish":{"type":"string","minLength":1,"maxLength":80}},"additionalProperties":false}`),
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Dish string `json:"dish"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			on, err := ft.ChefOn(ctx)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			if !on {
				return json.RawMessage(`{"started":false,"reason":"the Chef is off in Settings"}`), ToolEffectNone, nil
			}
			f, err := ft.Requester.Request(ctx, in.Dish, finds.TriggerChat)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			out, err := json.Marshal(map[string]any{"started": true, "find_id": f.ID})
			return out, ToolEffectNone, err
		},
	}
}
