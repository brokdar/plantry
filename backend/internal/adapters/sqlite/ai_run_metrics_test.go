package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper"
)

// Run duration (ms) and per-step metrics survive the round trip; a run made
// before the columns existed reads back as null / zero.
func TestAIRunRepo_Metrics(t *testing.T) {
	ctx := context.Background()
	db := testhelper.NewTestDB(t)
	repo := sqlite.NewAIRunRepo(db)

	started := time.Date(2026, 3, 2, 9, 30, 0, 0, time.UTC)
	id, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindChat, StartedAt: started, PromptVersion: "v1"})
	require.NoError(t, err)

	stepStart := started.Add(1500 * time.Millisecond).Add(123 * time.Millisecond)
	ttft := int64(420)
	require.NoError(t, repo.AddStep(ctx, airun.Step{
		RunID: id, Seq: 1, Type: airun.StepLLMCall, Name: "claude-asked",
		DurationMs: 900, InputTokens: 100, OutputTokens: 50,
		StartedAt: stepStart, TTFTMs: &ttft,
		CacheReadTokens: 900, CacheWriteTokens: 30, ReasoningTokens: 20, ThinkingChars: 144,
		ServedModel: "claude-served-1",
	}))
	require.NoError(t, repo.AddStep(ctx, airun.Step{
		RunID: id, Seq: 2, Type: airun.StepToolCall, Name: "get_plan", DurationMs: 3,
	}))
	require.NoError(t, repo.FinishRun(ctx, airun.RunEnd{
		ID: id, Status: airun.StatusOK, FinishedAt: started.Add(2 * time.Second),
		Provider: "anthropic", Model: "claude-asked", Iterations: 1,
		InputTokens: 100, OutputTokens: 50, ReasoningTokens: 20, DurationMs: 1987,
	}))

	run, err := repo.GetRun(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, run.DurationMs)
	assert.EqualValues(t, 1987, *run.DurationMs)
	assert.EqualValues(t, 20, run.Tokens.Reasoning)

	steps, err := repo.ListSteps(ctx, id)
	require.NoError(t, err)
	require.Len(t, steps, 2)
	llm, tool := steps[0], steps[1]
	require.NotNil(t, llm.StartedAt)
	assert.True(t, stepStart.Equal(*llm.StartedAt), "ms precision kept: %v", llm.StartedAt)
	require.NotNil(t, llm.TTFTMs)
	assert.EqualValues(t, 420, *llm.TTFTMs)
	assert.EqualValues(t, 900, llm.CacheReadTokens)
	assert.EqualValues(t, 30, llm.CacheWriteTokens)
	assert.EqualValues(t, 20, llm.ReasoningTokens)
	assert.EqualValues(t, 144, llm.ThinkingChars)
	assert.Equal(t, "claude-served-1", llm.ServedModel)
	assert.Nil(t, tool.StartedAt)
	assert.Nil(t, tool.TTFTMs)
	assert.Empty(t, tool.ServedModel)

	// A run still running has no duration.
	id2, err := repo.CreateRun(ctx, airun.RunStart{Kind: airun.KindChat, StartedAt: started, PromptVersion: "v1"})
	require.NoError(t, err)
	run2, err := repo.GetRun(ctx, id2)
	require.NoError(t, err)
	assert.Nil(t, run2.DurationMs)
}
