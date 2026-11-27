package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

type fakeRequester struct{ dish string }

func (f *fakeRequester) Request(_ context.Context, dish string, trigger finds.Trigger) (*finds.Find, error) {
	f.dish = dish + "/" + string(trigger)
	return &finds.Find{ID: 9}, nil
}

// find_recipe starts a chat find only with the Chef on, and its definition
// stays small (it is sent with every chat turn).
func TestTool_FindRecipe(t *testing.T) {
	t.Parallel()
	req := &fakeRequester{}
	on := false
	ts, err := agent.NewToolSet(agent.Services{Finds: agent.FindTools{Requester: req, ChefOn: func(context.Context) (bool, error) { return on, nil }}})
	require.NoError(t, err)
	tool, ok := describe(ts, "find_recipe")
	require.True(t, ok)
	require.LessOrEqual(t, len(tool.Description)+len(tool.Schema), 400)

	out, _, _, err := ts.Execute(context.Background(), "find_recipe", json.RawMessage(`{"dish":"pho"}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"started":false,"reason":"the Chef is off in Settings"}`, string(out))
	require.Empty(t, req.dish)

	on = true
	out, _, _, err = ts.Execute(context.Background(), "find_recipe", json.RawMessage(`{"dish":"pho"}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"started":true,"find_id":9}`, string(out))
	require.Equal(t, "pho/chat", req.dish)

	ts, err = agent.NewToolSet(agent.Services{})
	require.NoError(t, err)
	_, ok = describe(ts, "find_recipe")
	require.False(t, ok)
}

func describe(ts *agent.ToolSet, name string) (llm.Tool, bool) {
	for _, t := range ts.Describe() {
		if t.Name == name {
			return t, true
		}
	}
	return llm.Tool{}, false
}
