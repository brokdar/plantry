// Package sse is a thin helper for emitting Server-Sent Events over an
// http.ResponseWriter. Designed for single-writer use: the caller
// guarantees one goroutine drives Send/Comment.
package sse

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WriteTimeout bounds a single SSE frame write. Exceeding it typically means
// the client has stalled or disconnected.
const WriteTimeout = 30 * time.Second

// Writer emits SSE frames on an http.ResponseWriter. It must be created via
// New, which sets the mandatory headers and flushes them so clients see the
// connection open immediately.
type Writer struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

// New sets SSE response headers, flushes them, and returns a Writer ready
// for Send/Comment calls.
func New(w http.ResponseWriter) *Writer {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	// Disable buffering behind nginx or similar proxies.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	_ = rc.Flush()
	return &Writer{w: w, rc: rc}
}

// Send emits `event: <evt>` + `data: <json>` and flushes. The payload is
// JSON-encoded as a single line — json.Marshal escapes any embedded newlines
// in strings, so a single-line data frame is always framing-safe.
func (w *Writer) Send(evt string, payload any) error {
	_ = w.rc.SetWriteDeadline(time.Now().Add(WriteTimeout))

	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("sse: marshal payload: %w", err)
	}
	if _, err := fmt.Fprintf(w.w, "event: %s\ndata: %s\n\n", evt, b); err != nil {
		return err
	}
	return w.rc.Flush()
}

// Comment emits an SSE comment frame. Common uses: heartbeats (`: ping`) to
// keep intermediaries from closing an idle connection.
func (w *Writer) Comment(text string) error {
	_ = w.rc.SetWriteDeadline(time.Now().Add(WriteTimeout))
	if _, err := fmt.Fprintf(w.w, ": %s\n\n", text); err != nil {
		return err
	}
	return w.rc.Flush()
}
