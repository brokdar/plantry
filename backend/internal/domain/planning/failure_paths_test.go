package planning_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// E015: what a generation leaves behind when the provider fails, is cut off
// or a second one starts. The requirement for every case: the plan is
// unchanged or fully changed, the run ends with a translatable error key, and
// the run log holds the status, the key and the steps that happened.

type genAPIErr struct{ status int }

func (e genAPIErr) Error() string {
	return fmt.Sprintf("anthropic: %d some_error: raw provider text req_0123", e.status)
}
func (e genAPIErr) ProviderStatus() int     { return e.status }
func (genAPIErr) ProviderErrorType() string { return "some_error" }
func (e genAPIErr) ProviderInfo() llm.ProviderErrorInfo {
	return llm.ProviderErrorInfo{Status: e.status, RequestID: "req_0123"}
}

// genFailOn fails the n-th Stream call (1-based) with err.
type genFailOn struct {
	llm.Client
	n     int
	err   error
	calls int
}

func (c *genFailOn) Stream(ctx context.Context, req llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	if c.calls++; c.calls == c.n {
		close(out)
		return nil, c.err
	}
	return c.Client.Stream(ctx, req, out)
}

var genProviderFailures = []struct {
	name string
	err  error
	key  string
}{
	{"generic", errors.New("read tcp 10.0.0.2:5555: connection reset by peer"), "error.ai.provider_error"},
	{"rate limit", genAPIErr{429}, "error.ai.rate_limited"},
	{"timeout", fmt.Errorf("Post \"https://api.example/v1/messages\": %w", context.DeadlineExceeded), "error.plan.timeout"},
	{"key rejected", genAPIErr{401}, "error.ai.key_rejected"},
}

// translated reports whether the frontend has text for key in both languages.
func translated(t *testing.T, key string) bool {
	t.Helper()
	for _, lang := range []string{"en", "de"} {
		raw, err := os.ReadFile("../../../../frontend/src/lib/i18n/" + lang + ".json")
		require.NoError(t, err)
		var node any
		require.NoError(t, json.Unmarshal(raw, &node))
		for _, part := range strings.Split(key, ".") {
			m, ok := node.(map[string]any)
			if node, ok = m[part]; !ok {
				return false
			}
		}
		if _, ok := node.(string); !ok {
			return false
		}
	}
	return true
}

// failedRun asserts run id ended in error with wantKey, as the run API shows
// it, left the window free and wrote no plate, and returns its logged steps.
func (e *llmEnv) failedRun(t *testing.T, id int64, wantKey string) []loggedStep {
	t.Helper()
	e.svc.Wait()
	r := e.run(id)
	assert.Equal(t, "error", r.Status)
	assert.Equal(t, wantKey, r.ErrorKey)
	assert.True(t, translated(t, r.ErrorKey), "the frontend translates %s", r.ErrorKey)
	v, err := e.svc.Run(e.ctx, id)
	require.NoError(t, err)
	assert.Equal(t, airun.StatusError, v.Status)
	assert.Equal(t, wantKey, v.ErrorKey)
	plates, sessions := e.rows(id)
	assert.Zero(t, plates+sessions, "the plan is unchanged")
	assert.Empty(t, e.window())
	_, running := e.svc.Running(e.start)
	assert.False(t, running, "the window is free")
	steps := e.toolSteps(id)
	closing, _ := closing(t, steps)
	assert.Equal(t, planning.PhaseFinalProposal, closing.Phase, "the run's last plan is recorded")
	assert.Contains(t, []string{"error", "timeout"}, closing.Status)
	return steps
}

// Case 1: the provider fails on the first call.
func TestGenerationFailure_ProviderErrorOnFirstCall(t *testing.T) {
	for _, tc := range genProviderFailures {
		t.Run(tc.name, func(t *testing.T) {
			e := newLLMEnvOver(t, func(c llm.Client) llm.Client { return &genFailOn{Client: c, n: 1, err: tc.err} })

			steps := e.failedRun(t, e.gen(submit(t, "tu_1", e.validPlan())), tc.key)

			assert.Equal(t, []string{planning.ContextStep, "llm_call", planning.EvaluationStep}, names(steps), "context, the failed call, the closing plan")
		})
	}
}

// gate holds every provider call until release is closed and tells entered
// when the first one arrived.
type gate struct {
	llm.Client
	entered, release chan struct{}
	once             sync.Once
}

func (g *gate) Stream(ctx context.Context, req llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	g.once.Do(func() { close(g.entered) })
	<-g.release
	return g.Client.Stream(ctx, req, out)
}

// Case 2: the provider fails after the model's first turn: once after a dry
// run (evaluate_plan), once after a submitted plan that needs a repair. The
// plan stays as it was and the log has the steps up to the failure.
func TestGenerationFailure_ProviderFailsAfterTheFirstTurn(t *testing.T) {
	for name, tc := range map[string]struct {
		first func(e *llmEnv) fake.Turn
		want  []string
	}{
		"after a dry run": {func(e *llmEnv) fake.Turn {
			raw, err := json.Marshal(e.validPlan())
			require.NoError(t, err)
			return fake.Turn{StopReason: "tool_use", Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{
				{Type: "tool_use", ToolUseID: "tu_eval", ToolName: "evaluate_plan", ToolInput: raw}}}}
		}, []string{planning.ContextStep, "llm_call", planning.EvaluationStep, "llm_call", planning.EvaluationStep}},
		"after a plan to repair": {func(e *llmEnv) fake.Turn { return submit(t, "tu_1", e.h2Plan()) },
			[]string{planning.ContextStep, "llm_call", "plan.attempt", planning.EvaluationStep, "llm_call", planning.EvaluationStep}},
	} {
		t.Run(name, func(t *testing.T) {
			e := newLLMEnvOver(t, func(c llm.Client) llm.Client { return &genFailOn{Client: c, n: 2, err: genAPIErr{500}} })

			steps := e.failedRun(t, e.gen(tc.first(e), submit(t, "tu_2", e.validPlan())), "error.ai.provider_error")

			assert.Equal(t, tc.want, names(steps))
		})
	}
}

// Case 4 (final text): replies cut off at the output limit that hold no plan
// are bounded: the run ends with the unreadable-plan key and writes nothing.
func TestGenerationFailure_CutOffInTheFinalText(t *testing.T) {
	e := newLLMEnv(t)
	cut := fake.Turn{StopReason: "max_tokens", Message: fake.ScriptedMessage{Content: []fake.ScriptedBlock{{Type: "text", Text: "Here is the plan: Saturday"}}}}

	e.failedRun(t, e.gen(cut), "error.ai.invalid_plan")

	assert.LessOrEqual(t, len(e.client.Requests()), 5, "bounded")
}

// Case 7: a second generation for a window that has one running is refused
// before it starts a run, and the first finishes undisturbed.
func TestGenerationFailure_SecondGenerationWhileOneRuns(t *testing.T) {
	g := &gate{entered: make(chan struct{}), release: make(chan struct{})}
	e := newLLMEnvOver(t, func(c llm.Client) llm.Client { g.Client = c; return g })
	e.deps.Go = nil
	e.svc = planning.NewService(e.deps)
	e.script.Turns = []fake.Turn{submit(t, "tu_1", e.validPlan())}
	id, err := e.svc.Generate(e.ctx, e.req())
	require.NoError(t, err)
	<-g.entered

	_, err = e.svc.Generate(e.ctx, e.req())
	require.ErrorIs(t, err, domain.ErrGenerationRunning)
	_, err = e.svc.Regenerate(e.ctx, planning.Request{Start: e.start, Today: e.req().Today, Scope: planning.Scope{Kind: planning.ScopeEmptyCells}})
	require.ErrorIs(t, err, domain.ErrGenerationRunning)
	var runs int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM ai_runs`).Scan(&runs))
	assert.Equal(t, 1, runs, "the refused calls left no run behind")

	close(g.release)
	e.svc.Wait()
	assert.Equal(t, "ok", e.run(id).Status)
	plates, _ := e.rows(id)
	assert.Positive(t, plates)
}

// Case 8: no provider is configured. A generation without a chosen chooser
// plans with the heuristic; one that asks for the LLM is refused with the
// provider-missing error, starts no run and leaves the window free.
func TestGenerationFailure_NoProviderConfigured(t *testing.T) {
	e := newLLMEnv(t)
	e.deps.Choosers[airun.ChooserLLM] = planning.NewLLMChooser(llm.StaticResolver(nil, ""))
	e.svc = planning.NewService(e.deps)

	req := e.req()
	req.Chooser = airun.ChooserLLM
	_, err := e.svc.Generate(e.ctx, req)
	require.ErrorIs(t, err, domain.ErrAIProviderMissing)
	_, running := e.svc.Running(e.start)
	assert.False(t, running)
	var runs int
	require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM ai_runs`).Scan(&runs))
	assert.Zero(t, runs)

	id := e.generate(e.req())
	assert.Equal(t, string(airun.ChooserHeuristic), e.run(id).Chooser)
	assert.Equal(t, "ok", e.run(id).Status)
}
