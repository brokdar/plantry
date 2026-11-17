package agent

import (
	"context"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// lightSchema is the `light` property of get_plan, apply_changes and
// regenerate: the user's light requests as data (see planning.LightIntent),
// with scope none to clear a date's stored request.
var lightSchema = planning.LightSchema(true)

type lightKey struct{}

// withLight carries the light requests of one tool call to the plan
// evaluation (evalWindow), which applies them over the stored ones, so every
// figure the call returns, and the run log step it records, is made under
// them.
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

// checkLight hands intents back to the model that cannot apply to the
// window: a date outside it, a slot the plan lacks, or more light days than
// the cap allows next to the stored ones.
func checkLight(ctx context.Context, svc Services, w window.Window, light []planning.LightIntent) error {
	if len(light) == 0 || svc.State == nil {
		return nil
	}
	st, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return err
	}
	if err := planning.ResolveLight(st, light); err != nil {
		return fmt.Errorf("%w: %w", domain.ErrInvalidInput, err)
	}
	return nil
}

// saveLight keeps the light of a successful apply_changes call (joining the
// unit of work in ctx).
func saveLight(ctx context.Context, svc Services, light []planning.LightIntent) error {
	if len(light) == 0 || svc.Light == nil {
		return nil
	}
	return svc.Light.Replace(ctx, todayFrom(ctx), planning.LightDates(light), planning.MergeLight(nil, light))
}
