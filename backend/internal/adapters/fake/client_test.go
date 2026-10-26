package fake

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

func endTurnScript(usage ScriptedUsage) *Script {
	return &Script{Turns: []Turn{{
		StopReason: "end_turn",
		Message:    ScriptedMessage{Content: []ScriptedBlock{{Type: "text", Text: "ok"}}},
		Usage:      usage,
	}}}
}

func stream(t *testing.T, c *Client, req llm.Request) *llm.Response {
	t.Helper()
	out := make(chan llm.Event, 16)
	resp, err := c.Stream(context.Background(), req, out)
	require.NoError(t, err)
	return resp
}

func TestClient_RecordsRequests(t *testing.T) {
	t.Parallel()

	c := NewFromScript(endTurnScript(ScriptedUsage{}))

	stream(t, c, llm.Request{Model: "m1", System: "sys one"})
	_, err := c.Complete(context.Background(), llm.Request{Model: "m2", System: "sys two"})
	require.NoError(t, err)

	reqs := c.Requests()
	require.Len(t, reqs, 2)
	assert.Equal(t, "m1", reqs[0].Model)
	assert.Equal(t, "sys one", reqs[0].System)
	assert.Equal(t, "m2", reqs[1].Model)
	assert.Equal(t, "sys two", reqs[1].System)

	// The returned slice is a copy: mutating it must not affect the client.
	reqs[0].Model = "mutated"
	assert.Equal(t, "m1", c.Requests()[0].Model)
}

func TestClient_RecordsConcurrentRequests(t *testing.T) {
	t.Parallel()

	c := NewFromScript(endTurnScript(ScriptedUsage{}))

	const n = 20
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Complete(context.Background(), llm.Request{Model: "m"})
			_ = c.Requests()
		}()
	}
	wg.Wait()

	assert.Len(t, c.Requests(), n)
}

func TestClient_UsageIncludesCacheTokens(t *testing.T) {
	t.Parallel()

	var s Script
	require.NoError(t, json.Unmarshal([]byte(`{"turns":[{
		"stop_reason":"end_turn",
		"message":{"content":[{"type":"text","text":"ok"}]},
		"usage":{"input_tokens":10,"output_tokens":20,"cache_read_tokens":30,"cache_write_tokens":40}
	}]}`), &s))
	c := NewFromScript(&s)

	resp := stream(t, c, llm.Request{})

	assert.Equal(t, llm.Usage{
		InputTokens: 10, OutputTokens: 20, CacheReadTokens: 30, CacheWriteTokens: 40,
	}, resp.Usage)
}
