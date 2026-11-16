package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planview"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

type fakePlanReader struct {
	issues []planning.Issue
	err    error
}

func (f *fakePlanReader) Plan(context.Context, *time.Time, time.Time) (*planview.Plan, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &planview.Plan{Issues: f.issues}, nil
}

func issueN(n int) planning.Issue {
	return planning.Issue{Rule: "H5", Code: "c", Severity: "warn", Date: fmt.Sprintf("2026-05-%02d", n)}
}

// runWrite runs a fake write that turns the reader's issues into after and
// returns the `after` block's issue part.
func runWrite(t *testing.T, ctx context.Context, f *fakePlanReader, after []planning.Issue) map[string]any {
	t.Helper()
	svc := agent.Services{PlanView: f}
	out := map[string]any{}
	h := agent.WithIssueBaseline(svc, func(ctx context.Context, _ json.RawMessage) (json.RawMessage, agent.ToolEffect, error) {
		f.issues = after
		agent.IssueDelta(ctx, svc, out)
		return nil, agent.ToolEffectNone, nil
	})
	_, _, err := h(ctx, nil)
	require.NoError(t, err)
	return out
}

func windowCtx() context.Context {
	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	return agent.WithWindow(context.Background(), window.Window{Start: start, End: start.AddDate(0, 0, 6)})
}

func TestIssueDelta_NewResolvedAndTotal(t *testing.T) {
	f := &fakePlanReader{issues: []planning.Issue{issueN(1), issueN(2)}}
	out := runWrite(t, windowCtx(), f, []planning.Issue{issueN(2), issueN(3)})
	assert.Equal(t, true, out["issues_checked"])
	res := out["issues"].(map[string]any)
	assert.Equal(t, 2, res["total_open"])
	assert.Len(t, res["new"], 1)
	assert.Len(t, res["resolved"], 1)
	assert.NotContains(t, res, "new_more")
}

func TestIssueDelta_CapsLists(t *testing.T) {
	var many []planning.Issue
	for i := 1; i <= 25; i++ {
		many = append(many, issueN(i))
	}
	out := runWrite(t, windowCtx(), &fakePlanReader{}, many)
	res := out["issues"].(map[string]any)
	assert.Len(t, res["new"], 10)
	assert.Equal(t, 15, res["new_more"])
	assert.Equal(t, 25, res["total_open"])
}

// Not-checked must never read like a clean result.
func TestIssueDelta_UncheckedIsExplicit(t *testing.T) {
	t.Run("no window", func(t *testing.T) {
		out := runWrite(t, context.Background(), &fakePlanReader{}, nil)
		assert.Equal(t, false, out["issues_checked"])
		assert.NotContains(t, out, "issues")
		assert.Contains(t, out["issues_unchecked_reason"], "no planning window")
	})
	t.Run("plan fails", func(t *testing.T) {
		out := runWrite(t, windowCtx(), &fakePlanReader{err: errors.New("boom")}, nil)
		assert.Equal(t, false, out["issues_checked"])
		assert.NotContains(t, out, "issues")
		assert.Contains(t, out["issues_unchecked_reason"], "plan read failed")
	})
}
