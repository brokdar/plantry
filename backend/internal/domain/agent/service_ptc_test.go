package agent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// D4 (audit F10): the chat agent never offers code execution or allowed
// callers, whatever ai.programmatic_tools says (the planner keeps it).
func TestChat_NeverUsesProgrammaticToolCalling(t *testing.T) {
	t.Parallel()
	for _, setting := range []string{"true", "false"} {
		script := &fake.Script{Turns: []fake.Turn{endTurn(fake.ScriptedUsage{InputTokens: 1, OutputTokens: 1})}}
		client := fake.NewFromScript(script)
		svc, db := newChatServiceWithClient(t, client)
		require.NoError(t, testsvc.Settings(db).Set(context.Background(), settings.KeyAIProgrammaticTools, setting))
		_, err := chat(context.Background(), svc, "hi", nil)
		require.NoError(t, err)

		reqs := client.Requests()
		require.Len(t, reqs, 1)
		assert.False(t, reqs[0].CodeExecution, setting)
		assert.Empty(t, reqs[0].Container)
		for _, tl := range reqs[0].Tools {
			assert.Empty(t, tl.AllowedCallers, "%s: %s", setting, tl.Name)
		}
	}
}
