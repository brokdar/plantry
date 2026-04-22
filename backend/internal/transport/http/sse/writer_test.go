package sse_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/transport/http/sse"
)

type frame struct {
	Event string
	Data  string
}

// readFrames parses an SSE response body into frame records. Blank lines
// terminate frames; `: ` lines are collected as comments (returned with
// Event == "").
func readFrames(t *testing.T, body io.Reader, want int) []frame {
	t.Helper()
	var frames []frame
	scanner := bufio.NewScanner(body)
	var cur frame
	hasData := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if hasData || cur.Event != "" {
				frames = append(frames, cur)
				cur = frame{}
				hasData = false
				if len(frames) == want {
					return frames
				}
			}
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			cur.Event = strings.TrimPrefix(line, "event: ")
		}
		if strings.HasPrefix(line, "data: ") {
			if cur.Data != "" {
				cur.Data += "\n"
			}
			cur.Data += strings.TrimPrefix(line, "data: ")
			hasData = true
		}
		if strings.HasPrefix(line, ": ") {
			frames = append(frames, frame{Event: "", Data: strings.TrimPrefix(line, ": ")})
			if len(frames) == want {
				return frames
			}
		}
	}
	return frames
}

func TestWriter_SendSequence(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wr := sse.New(w)
		require.NoError(t, wr.Send("message_start", map[string]string{"model": "test"}))
		require.NoError(t, wr.Send("assistant_delta", map[string]string{"text": "hello"}))
		require.NoError(t, wr.Send("done", map[string]any{}))
	}))
	defer ts.Close()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, ts.URL, nil)
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, "text/event-stream; charset=utf-8", resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
	assert.Equal(t, "no", resp.Header.Get("X-Accel-Buffering"))

	frames := readFrames(t, resp.Body, 3)
	require.Len(t, frames, 3)
	assert.Equal(t, "message_start", frames[0].Event)
	assert.Equal(t, "assistant_delta", frames[1].Event)
	assert.Equal(t, "done", frames[2].Event)

	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(frames[1].Data), &payload))
	assert.Equal(t, "hello", payload["text"])
}

func TestWriter_PayloadWithNewlinesStaysSingleLine(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wr := sse.New(w)
		require.NoError(t, wr.Send("note", map[string]string{"text": "line1\nline2\nline3"}))
	}))
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	frames := readFrames(t, resp.Body, 1)
	require.Len(t, frames, 1)
	// JSON-escaped newlines preserve the single-line data frame.
	assert.Contains(t, frames[0].Data, `\n`)
	var payload map[string]string
	require.NoError(t, json.Unmarshal([]byte(frames[0].Data), &payload))
	assert.Equal(t, "line1\nline2\nline3", payload["text"])
}

func TestWriter_Comment(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wr := sse.New(w)
		require.NoError(t, wr.Comment("ping"))
		require.NoError(t, wr.Send("done", map[string]any{}))
	}))
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(raw), ": ping\n\n")
	assert.Contains(t, string(raw), "event: done\n")
}

func TestWriter_HeartbeatLoop(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wr := sse.New(w)
		// Emit a heartbeat, then the final event.
		require.NoError(t, wr.Comment("ping"))
		time.Sleep(10 * time.Millisecond)
		require.NoError(t, wr.Comment("ping"))
		require.NoError(t, wr.Send("done", struct{}{}))
	}))
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(resp.Body)
	// Two heartbeats + one final frame.
	assert.Equal(t, 2, strings.Count(string(raw), ": ping\n\n"))
	assert.Contains(t, string(raw), "event: done\n")
}
