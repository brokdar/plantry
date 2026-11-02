package airun_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// searchClient answers every call with resp and reports web search support.
type searchClient struct {
	resp      *llm.Response
	supported bool
	err       error
	asked     []string
}

func (c *searchClient) Stream(_ context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	close(out)
	return c.resp, nil
}

func (c *searchClient) Complete(context.Context, llm.Request) (string, error) { return "", nil }

func (c *searchClient) WebSearch(_ context.Context, model string) (bool, error) {
	c.asked = append(c.asked, model)
	return c.supported, c.err
}

func TestMeter_ForwardsCapabilities(t *testing.T) {
	rec, _ := newRecorder(t, newMemStore())

	inner := &searchClient{supported: true}
	caps, ok := rec.Meter(inner, "anthropic").(llm.Capabilities)
	require.True(t, ok, "metered client must forward llm.Capabilities")
	got, err := caps.WebSearch(context.Background(), "claude-opus-5-5")
	require.NoError(t, err)
	assert.True(t, got)
	assert.Equal(t, []string{"claude-opus-5-5"}, inner.asked)

	boom := errors.New("boom")
	got, err = rec.Meter(&searchClient{err: boom}, "anthropic").(llm.Capabilities).WebSearch(context.Background(), "m")
	assert.False(t, got)
	assert.ErrorIs(t, err, boom)

	_, ok = rec.Meter(errClient{}, "openai").(llm.Capabilities)
	assert.False(t, ok, "a client without capabilities stays without them")
}

func TestMeter_RecordsWebSearchRequests(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	server := json.RawMessage(`{"type":"server_tool_use","id":"srvtoolu_1","name":"web_search","input":{"query":"curry"}}`)
	inner := &searchClient{resp: &llm.Response{
		Message: llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
			{Type: llm.ContentTypeServer, Raw: server},
			{Type: llm.ContentTypeText, Text: "ok", Citations: []string{"https://example.com/a"}},
		}},
		StopReason: llm.StopReasonEndTurn,
		Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5, WebSearchRequests: 2},
	}}

	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindUntracked})
	_, err := drainStream(ctx, rec.Meter(inner, "anthropic"), llm.Request{
		Model:     "claude-opus-5-5",
		Messages:  []llm.Message{textMsg(llm.RoleUser, "find")},
		WebSearch: &llm.WebSearch{MaxUses: 5},
	})
	require.NoError(t, err)
	run.Finish(ctx, airun.StatusOK, "")

	steps := stepsBySeq(store)
	require.Len(t, steps, 1)
	var resp struct {
		Content           []llm.ContentBlock `json:"content"`
		WebSearchRequests int                `json:"web_search_requests"`
	}
	require.NoError(t, json.Unmarshal([]byte(steps[0].ResponseJSON), &resp))
	assert.Equal(t, 2, resp.WebSearchRequests)
	require.Len(t, resp.Content, 2)
	assert.JSONEq(t, string(server), string(resp.Content[0].Raw))

	var req struct {
		WebSearch *llm.WebSearch `json:"web_search"`
	}
	require.NoError(t, json.Unmarshal([]byte(steps[0].RequestJSON), &req))
	require.NotNil(t, req.WebSearch)
	assert.Equal(t, 5, req.WebSearch.MaxUses)
}

func TestMeter_NoSearchLeavesStepJSONUnchanged(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	inner := &searchClient{resp: &llm.Response{
		Message:    llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "ok"}}},
		StopReason: llm.StopReasonEndTurn,
	}}
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindUntracked})
	_, err := drainStream(ctx, rec.Meter(inner, "anthropic"), llm.Request{Model: "m", Messages: []llm.Message{textMsg(llm.RoleUser, "x")}})
	require.NoError(t, err)
	run.Finish(ctx, airun.StatusOK, "")

	steps := stepsBySeq(store)
	require.Len(t, steps, 1)
	assert.NotContains(t, steps[0].ResponseJSON, "web_search")
	assert.NotContains(t, steps[0].RequestJSON, "web_search")
}

func TestMeter_LargeServerBlockTruncated(t *testing.T) {
	store := newMemStore()
	rec, _ := newRecorder(t, store)
	big := `{"type":"web_search_tool_result","content":[{"encrypted_content":"` + strings.Repeat("A", 300<<10) + `"}]}`
	inner := &searchClient{resp: &llm.Response{
		Message:    llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{Type: llm.ContentTypeServer, Raw: json.RawMessage(big)}}},
		StopReason: llm.StopReasonEndTurn,
	}}
	ctx, run := rec.Start(context.Background(), airun.Start{Kind: airun.KindUntracked})
	_, err := drainStream(ctx, rec.Meter(inner, "anthropic"), llm.Request{Model: "m"})
	require.NoError(t, err)
	run.Finish(ctx, airun.StatusOK, "")

	steps := stepsBySeq(store)
	require.Len(t, steps, 1)
	assert.LessOrEqual(t, len(steps[0].ResponseJSON), 256<<10)
	assert.True(t, decodeResponse(t, steps[0]).Truncated)
}
