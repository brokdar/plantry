package agent_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// The chat prompt's plan part (summary and facts) on the heuristic week: each
// component shows its position and a recipe its batch role, each recipe its
// minutes. Run with -v for the byte count.
func TestChatPrompt_PlanPartSize(t *testing.T) {
	e := newPlanToolEnv(t)
	e.sql(`UPDATE foods SET prep_minutes = 10, cook_minutes = 25 WHERE kind = 'composed'`)
	st, err := testsvc.PlanState(e.db)(context.Background(), window.Window{Start: planStart, End: planStart.AddDate(0, 0, 6)}, planToday)
	require.NoError(t, err)
	text := planning.RenderPlanSummary(st, agent.ChatIssues(st)) + agent.ChatFacts(st)
	t.Logf("chat prompt plan part: %d bytes", len(text))
	for _, want := range []string{", main, ", ", side, ", " cook)", " leftover)", "| main | 35 min |"} {
		assert.True(t, strings.Contains(text, want), "the plan part shows %q", want)
	}
}
