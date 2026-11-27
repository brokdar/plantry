package agent_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/theme"
)

type fakeThemes struct {
	start time.Time
	got   theme.Theme
}

func (f *fakeThemes) Set(_ context.Context, start time.Time, t theme.Theme) (theme.Result, error) {
	f.start, f.got = start, t
	return theme.Result{Theme: theme.Theme{Value: "japanese"}, Finding: 2}, nil
}

// set_week_theme sets a chat theme on this or next week's window only with
// the Chef on, and its definition stays small (sent with every chat turn).
func TestTool_SetWeekTheme(t *testing.T) {
	t.Parallel()
	f := &fakeThemes{}
	on := false
	ts, err := agent.NewToolSet(agent.Services{Themes: agent.ThemeTools{Setter: f, ChefOn: func(context.Context) (bool, error) { return on, nil }}})
	require.NoError(t, err)
	tool, ok := describe(ts, "set_week_theme")
	require.True(t, ok)
	require.LessOrEqual(t, len(tool.Description)+len(tool.Schema), 520)

	in := json.RawMessage(`{"kind":"cuisine","value":"Japanese","next_week":true}`)
	out, _, _, err := ts.Execute(context.Background(), "set_week_theme", in)
	require.NoError(t, err)
	require.JSONEq(t, `{"set":false,"reason":"the Chef is off in Settings"}`, string(out))

	on = true
	start := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	out, _, _, err = ts.Execute(agent.WithWindowForTest(context.Background(), start), "set_week_theme", in)
	require.NoError(t, err)
	require.JSONEq(t, `{"set":true,"week_start":"2026-11-28","theme":"japanese","library":0,"finding":2}`, string(out))
	require.Equal(t, theme.Theme{Kind: theme.KindCuisine, Value: "Japanese", Source: theme.SourceChat}, f.got)

	ts, err = agent.NewToolSet(agent.Services{})
	require.NoError(t, err)
	_, ok = describe(ts, "set_week_theme")
	require.False(t, ok)
}
