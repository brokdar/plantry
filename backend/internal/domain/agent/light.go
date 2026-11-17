package agent

import (
	"context"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// lightSchema is the `light` property of get_plan, apply_changes and
// regenerate: the user's light requests as data (see planning.LightIntent).
const lightSchema = `"light":{
      "type":"array","maxItems":14,
      "description":"The user's light requests, structured, one entry per day; omit when there is none. scope meal: that lunch or dinner is lighter (at most 75% of its normal share) and the day keeps its target. scope day: ONLY when the user explicitly asks to eat less that day; the day's target becomes 85%. Repeat the list in every call of this conversation turn that evaluates the plan.",
      "items":{
        "type":"object","required":["date","scope"],
        "properties":{
          "date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
          "scope":{"type":"string","enum":["meal","day"]},
          "slot":{"type":"string","enum":["lunch","dinner"],"description":"Required for scope meal."}
        },
        "additionalProperties":false
      }
    }`

type lightKey struct{}

// withLight carries the light requests of one tool call to the plan
// evaluation (evalWindow), so every figure the call returns, and the run log
// step it records, is made under them.
func withLight(ctx context.Context, light []planning.LightIntent) context.Context {
	return context.WithValue(ctx, lightKey{}, light)
}

func lightFrom(ctx context.Context) []planning.LightIntent {
	l, _ := ctx.Value(lightKey{}).([]planning.LightIntent)
	return l
}

// parseLight checks a tool's `light` input.
func parseLight(raw []planning.LightIntent) ([]planning.LightIntent, error) {
	light, err := planning.ValidateLight(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidInput, err)
	}
	return light, nil
}
