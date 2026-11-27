package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/theme"
)

// ThemeTools wire set_week_theme (THEME-2); registered when Setter is set.
type ThemeTools struct {
	Setter interface {
		Set(ctx context.Context, start time.Time, t theme.Theme) (theme.Result, error)
	}
	ChefOn func(ctx context.Context) (bool, error)
}

func toolSetWeekTheme(tt ThemeTools) Tool {
	return Tool{
		Name:        "set_week_theme",
		Description: "Set a theme for this or next week's plan (max 2). whole_week only if the user says so. Returns {library, finding}: with library 0 say e.g. \"No Japanese recipes yet. I'll find 2.\"",
		Schema:      json.RawMessage(`{"type":"object","required":["kind","value"],"properties":{"kind":{"enum":["cuisine","ingredient","mood"]},"value":{"type":"string","maxLength":40},"next_week":{"type":"boolean"},"whole_week":{"type":"boolean"}},"additionalProperties":false}`),
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Kind      theme.Kind `json:"kind"`
				Value     string     `json:"value"`
				NextWeek  bool       `json:"next_week"`
				WholeWeek bool       `json:"whole_week"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			on, err := tt.ChefOn(ctx)
			if err != nil {
				return nil, ToolEffectNone, err
			}
			if !on {
				return json.RawMessage(`{"set":false,"reason":"the Chef is off in Settings"}`), ToolEffectNone, nil
			}
			w, ok := windowFrom(ctx)
			if !ok {
				return nil, ToolEffectNone, fmt.Errorf("%w: no plan window", domain.ErrInvalidInput)
			}
			start := w.Start
			if in.NextWeek {
				start = start.AddDate(0, 0, 7)
			}
			res, err := tt.Setter.Set(ctx, start, theme.Theme{Kind: in.Kind, Value: in.Value, Source: theme.SourceChat, WholeWeek: in.WholeWeek})
			if err != nil {
				return nil, ToolEffectNone, err
			}
			return mustJSON(map[string]any{
				"set": true, "week_start": start.Format(time.DateOnly), "theme": res.Theme.Value,
				"library": res.Library, "finding": res.Finding,
			}), ToolEffectNone, nil
		},
	}
}
