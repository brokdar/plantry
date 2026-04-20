package ingredient

import (
	"context"
	"errors"
	"testing"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// scriptedClient is a canned llm.Client whose Stream() returns a single assistant
// text response (or an error). Mirrors the pattern in handlers/ai_test.go.
type scriptedClient struct {
	text string
	err  error
}

func (s *scriptedClient) Stream(_ context.Context, _ llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)
	if s.err != nil {
		return nil, s.err
	}
	return &llm.Response{
		Message: llm.Message{
			Role: llm.RoleAssistant,
			Content: []llm.ContentBlock{
				{Type: llm.ContentTypeText, Text: s.text},
			},
		},
		StopReason: llm.StopReasonEndTurn,
	}, nil
}

func TestTranslateQuery_NilClient(t *testing.T) {
	got := translateQuery(context.Background(), nil, "", "Hähnchenbrust", nil)
	if got != "Hähnchenbrust" {
		t.Fatalf("want unchanged query, got %q", got)
	}
}

func TestTranslateQuery_EmptyQuery(t *testing.T) {
	c := &scriptedClient{text: `["onion raw"]`}
	got := translateQuery(context.Background(), c, "m", "   ", nil)
	if got != "   " {
		t.Fatalf("want passthrough of empty query, got %q", got)
	}
}

func TestTranslateQuery_Success(t *testing.T) {
	c := &scriptedClient{text: `["chicken breast raw"]`}
	trace := NewLookupTrace()
	got := translateQuery(context.Background(), c, "m", "Hähnchenbrust", trace)
	if got != "chicken breast raw" {
		t.Fatalf("want translated term, got %q", got)
	}
	entries := trace.Entries()
	if len(entries) != 1 || entries[0].Step != "ai.translate" || entries[0].Level != TraceLevelSuccess {
		t.Fatalf("trace should record one success entry, got %+v", entries)
	}
}

func TestTranslateQuery_MalformedJSON_FallsBack(t *testing.T) {
	c := &scriptedClient{text: `not-json`}
	trace := NewLookupTrace()
	got := translateQuery(context.Background(), c, "m", "Zwiebel", trace)
	if got != "Zwiebel" {
		t.Fatalf("want original query on malformed response, got %q", got)
	}
	entries := trace.Entries()
	if len(entries) != 1 || entries[0].Level != TraceLevelWarning {
		t.Fatalf("expected a warning trace entry, got %+v", entries)
	}
}

func TestTranslateQuery_StripsCodeFences(t *testing.T) {
	c := &scriptedClient{text: "```json\n[\"spaghetti dry\"]\n```"}
	got := translateQuery(context.Background(), c, "m", "Spaghetti", nil)
	if got != "spaghetti dry" {
		t.Fatalf("want unwrapped term, got %q", got)
	}
}

func TestTranslateQuery_ClientError_FallsBack(t *testing.T) {
	c := &scriptedClient{err: errors.New("rate limited")}
	trace := NewLookupTrace()
	got := translateQuery(context.Background(), c, "m", "Zucker", trace)
	if got != "Zucker" {
		t.Fatalf("want passthrough on error, got %q", got)
	}
	if trace.Entries()[0].Level != TraceLevelWarning {
		t.Fatalf("expected warning, got %v", trace.Entries()[0])
	}
}

func TestPickBest_NilClient(t *testing.T) {
	cands := []Candidate{
		{Name: "a"}, {Name: "b"},
	}
	if got := pickBest(context.Background(), nil, "", "x", cands, nil); got != 0 {
		t.Fatalf("nil client should yield 0, got %d", got)
	}
}

func TestPickBest_SingleCandidate_SkipsAI(t *testing.T) {
	// Even with a scripted AI, a single candidate should always yield index 0
	// without consulting the model.
	c := &scriptedClient{text: `[2]`}
	got := pickBest(context.Background(), c, "m", "x", []Candidate{{Name: "only"}}, nil)
	if got != 0 {
		t.Fatalf("want 0 for single candidate, got %d", got)
	}
}

func TestPickBest_Success(t *testing.T) {
	c := &scriptedClient{text: `[1]`}
	trace := NewLookupTrace()
	cands := []Candidate{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	got := pickBest(context.Background(), c, "m", "x", cands, trace)
	if got != 1 {
		t.Fatalf("want index 1, got %d", got)
	}
	if trace.Entries()[0].Level != TraceLevelSuccess {
		t.Fatalf("expected success trace")
	}
}

func TestPickBest_OutOfRange_ClampsToZero(t *testing.T) {
	c := &scriptedClient{text: `[99]`}
	trace := NewLookupTrace()
	cands := []Candidate{{Name: "a"}, {Name: "b"}}
	if got := pickBest(context.Background(), c, "m", "x", cands, trace); got != 0 {
		t.Fatalf("out-of-range should clamp to 0, got %d", got)
	}
}

func TestPickBest_NegativeOne_ClampsToZero(t *testing.T) {
	c := &scriptedClient{text: `[-1]`}
	cands := []Candidate{{Name: "a"}, {Name: "b"}}
	if got := pickBest(context.Background(), c, "m", "x", cands, nil); got != 0 {
		t.Fatalf("-1 should clamp to 0, got %d", got)
	}
}

func TestPickBest_MalformedJSON_FallsBack(t *testing.T) {
	c := &scriptedClient{text: "nonsense"}
	cands := []Candidate{{Name: "a"}, {Name: "b"}}
	if got := pickBest(context.Background(), c, "m", "x", cands, nil); got != 0 {
		t.Fatalf("want 0 on malformed response, got %d", got)
	}
}

func TestLookupTrace_NilSafe(t *testing.T) {
	var tr *LookupTrace
	// No panic when nil.
	tr.Add(TraceEntry{Step: "x"})
	if got := tr.Entries(); got != nil {
		t.Fatalf("nil trace should return nil entries, got %v", got)
	}
}

func TestLookupTrace_ConcurrentAdd(t *testing.T) {
	tr := NewLookupTrace()
	done := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			for j := 0; j < 50; j++ {
				tr.Add(TraceEntry{Step: "x"})
			}
			done <- struct{}{}
		}()
	}
	<-done
	<-done
	if len(tr.Entries()) != 100 {
		t.Fatalf("want 100 entries, got %d", len(tr.Entries()))
	}
}

func TestTraceContext_Roundtrip(t *testing.T) {
	tr := NewLookupTrace()
	ctx := WithTrace(context.Background(), tr)
	if got := TraceFromContext(ctx); got != tr {
		t.Fatalf("roundtrip failed")
	}
	if got := TraceFromContext(context.Background()); got != nil {
		t.Fatalf("empty context should yield nil, got %v", got)
	}
}
