package agent_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
)

// F6: every sentence of the model's final text the reply leaves out is logged
// (agent.reply.lint) and stored with its reason as the run's reply.lint step,
// so an eval reads the exact reasons of a turn from its run.
func TestReplyLint_DroppedSentencesAreStoredWithTheirReason(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	got := agent.ComposeDrops(ctx, "swap the dinner",
		"Done on Tuesday. It now has 800 kcal. The leftovers stay. Want more?", true)
	assert.Equal(t, []string{
		"question: Want more?",
		"third_sentence: The leftovers stay.",
		"weekday: Done on Tuesday.",
		"number: It now has 800 kcal.",
	}, got)

	var result string
	require.NoError(t, e.db.QueryRow(`SELECT result_json FROM ai_run_steps WHERE type = 'tool_call' AND name = 'reply.lint'`).Scan(&result))
	var stored struct {
		Dropped []struct{ Sentence, Reason string } `json:"dropped"`
	}
	require.NoError(t, json.Unmarshal([]byte(result), &stored))
	require.Len(t, stored.Dropped, 4)
	assert.Equal(t, "weekday", stored.Dropped[2].Reason)
	assert.Equal(t, "Done on Tuesday.", stored.Dropped[2].Sentence)

	// A turn whose text passes stores nothing.
	ctx2, _ := e.chat()
	assert.Empty(t, agent.ComposeDrops(ctx2, "swap the dinner", "Done.", true))
}
