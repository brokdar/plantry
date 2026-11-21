package agent_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// Case 4 (final text): a reply cut off at the output limit with no tool call
// is not shown as a finished answer. Its text stays in the transcript, the
// user is told what happened and what to do, and the run is an error with its
// key.
func TestChatFailure_FinalTextCutOff(t *testing.T) {
	t.Parallel()
	cut := fake.Turn{StopReason: "max_tokens", Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "text", Text: "Here is your week: on Monday"}}}}
	svc, db := newChatService(t, &fake.Script{Turns: []fake.Turn{cut}})

	evts, err := chat(context.Background(), svc, "plan my week", nil)

	require.ErrorIs(t, err, agent.ErrCutOff)
	assert.Equal(t, "error.ai.cut_off", failureEvent(t, evts).MessageKey)
	for _, e := range evts {
		assert.NotEqual(t, llm.EventDone, e.Type)
	}
	assert.Equal(t, 1, count(t, db, `SELECT COUNT(*) FROM ai_messages WHERE role = 'assistant' AND content LIKE '%on Monday%'`), "the cut text is kept")
	status, key, steps := chatRunSteps(t, db)
	assert.Equal(t, "error", status)
	assert.Equal(t, "error.ai.cut_off", key)
	assert.Equal(t, []string{"llm_call:" + chatModel}, steps)
}
