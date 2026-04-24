package food

import (
	"context"
	"sync"
)

// TraceLevel classifies the severity of a trace step.
type TraceLevel = string

const (
	TraceLevelInfo    TraceLevel = "info"
	TraceLevelSuccess TraceLevel = "success"
	TraceLevelWarning TraceLevel = "warning"
	TraceLevelError   TraceLevel = "error"
)

// TraceEntry is one step in a lookup pipeline trace. Detail is free-form
// JSON shaped by the step.
type TraceEntry struct {
	Step       string     `json:"step"`
	Level      TraceLevel `json:"level"`
	Summary    string     `json:"summary"`
	DurationMs int64      `json:"duration_ms,omitempty"`
	Detail     any        `json:"detail,omitempty"`
}

// LookupTrace accumulates TraceEntry items across a single lookup. Nil-safe:
// every method no-ops on a nil receiver.
type LookupTrace struct {
	mu      sync.Mutex
	entries []TraceEntry
}

// NewLookupTrace returns an empty LookupTrace ready to accept entries.
func NewLookupTrace() *LookupTrace {
	return &LookupTrace{entries: make([]TraceEntry, 0, 8)}
}

// Add appends a trace entry. Safe on a nil receiver.
func (t *LookupTrace) Add(entry TraceEntry) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.entries = append(t.entries, entry)
}

// Entries returns a snapshot of the entries accumulated so far.
func (t *LookupTrace) Entries() []TraceEntry {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TraceEntry, len(t.entries))
	copy(out, t.entries)
	return out
}

type traceCtxKey struct{}

// WithTrace returns ctx with the given trace pointer attached.
func WithTrace(ctx context.Context, trace *LookupTrace) context.Context {
	return context.WithValue(ctx, traceCtxKey{}, trace)
}

// TraceFromContext returns the trace pointer stored on ctx, or nil if none.
// Nil return values are safe to pass to Add.
func TraceFromContext(ctx context.Context) *LookupTrace {
	v := ctx.Value(traceCtxKey{})
	if v == nil {
		return nil
	}
	t, _ := v.(*LookupTrace)
	return t
}

// AITranslationDetail describes a single AI query-translation call.
type AITranslationDetail struct {
	SystemPrompt string `json:"system_prompt,omitempty"`
	InputQuery   string `json:"input_query"`
	Translated   string `json:"translated,omitempty"`
	RawResponse  string `json:"raw_response,omitempty"`
	Error        string `json:"error,omitempty"`
}

// AIPickBestDetail describes a single AI pick-best ranking call.
type AIPickBestDetail struct {
	OriginalQuery string   `json:"original_query"`
	Candidates    []string `json:"candidates"`
	RawResponse   string   `json:"raw_response,omitempty"`
	PickedIndex   int      `json:"picked_index"`
	Error         string   `json:"error,omitempty"`
}

// ExternalAPIDetail describes a call out to OFF or FDC.
type ExternalAPIDetail struct {
	Source      string `json:"source"` // "fdc" | "off"
	Query       string `json:"query,omitempty"`
	Barcode     string `json:"barcode,omitempty"`
	ResultCount int    `json:"result_count"`
	Error       string `json:"error,omitempty"`
}
