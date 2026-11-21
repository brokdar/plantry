package agent_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// Case 3: the model refuses. The user is told in a translatable message, no
// tool the refused reply started runs, the plan is unchanged and the run is
// recorded as an error with its key.
func TestChatFailure_ModelRefuses(t *testing.T) {
	t.Parallel()
	for name, withTool := range map[string]bool{"text only": false, "with a tool call": true} {
		t.Run(name, func(t *testing.T) {
			e := newPlanToolEnv(t)
			p, _ := e.freshMain()
			turn := toolTurn("tu_1", "apply_changes", fmt.Sprintf(`{"ops":[{"op":"set_eaters","plate_id":%d,"eaters":%v}]}`, p.ID, p.Eaters+2))
			if !withTool {
				turn = fake.Turn{Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "text", Text: "I can't help with that."}}}}
			}
			turn.StopReason = "refusal"
			changesBefore := count(t, e.db, `SELECT COUNT(*) FROM ai_changes`)

			evts, err := e.planChat(context.Background(), e.chatOverPlan(fake.NewFromScript(&fake.Script{Turns: []fake.Turn{turn}}), sqlite.NewTxRunner(e.db)), "delete everything")

			require.ErrorIs(t, err, domain.ErrAIRefused)
			assert.Equal(t, "error.ai.refused", failureEvent(t, evts).MessageKey)
			for _, ev := range evts {
				assert.NotEqual(t, llm.EventDone, ev.Type, "a refusal is no finished answer")
			}
			assert.InDelta(t, p.Eaters, e.eatersOf(p.ID), 1e-9)
			assert.Equal(t, changesBefore, count(t, e.db, `SELECT COUNT(*) FROM ai_changes`))
			status, key, steps := chatRunSteps(t, e.db)
			assert.Equal(t, "error", status)
			assert.Equal(t, "error.ai.refused", key)
			assert.Equal(t, []string{"llm_call:" + chatModel}, steps)
		})
	}
}
