package airun

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Start describes a run being started.
type Start struct {
	Kind          Kind
	PromptVersion string
	WindowStart   *time.Time
	WindowEnd     *time.Time
	Instruction   *string
}

// Recorder starts runs and meters LLM clients. Store failures are logged at
// warn and never surface to the caller.
type Recorder struct {
	store  Store
	logger *slog.Logger
	now    func() time.Time
}

// NewRecorder builds a Recorder. now is injected so tests can pin time.
func NewRecorder(store Store, logger *slog.Logger, now func() time.Time) *Recorder {
	return &Recorder{store: store, logger: logger, now: now}
}

type ctxKey struct{}

// FromContext returns the run carried by ctx, or nil when there is none.
func FromContext(ctx context.Context) *Run {
	r, _ := ctx.Value(ctxKey{}).(*Run)
	return r
}

// Run is one user-visible AI action in progress. Its methods are safe for
// concurrent use and nil-safe. Steps recorded after Finish are still written
// but are not reflected in the run totals.
type Run struct {
	rec       *Recorder
	id        int64 // 0 when CreateRun failed: nothing more is written
	kind      Kind
	requestID string
	startedAt time.Time

	mu               sync.Mutex
	seq              int
	iterations       int
	inputTokens      int64
	outputTokens     int64
	cacheReadTokens  int64
	cacheWriteTokens int64
	provider         string
	model            string
	promptHash       string
	promptTried      bool
	finished         bool
}

// Start creates the run row, returns a ctx carrying the run and logs
// ai.run.start. If the row cannot be created the run is still returned (so
// metered calls do not fall back to untracked runs) but records nothing.
func (r *Recorder) Start(ctx context.Context, s Start) (context.Context, *Run) {
	run := &Run{
		rec:       r,
		kind:      s.Kind,
		requestID: middleware.GetReqID(ctx),
		startedAt: r.now(),
	}
	id, err := r.store.CreateRun(context.WithoutCancel(ctx), RunStart{
		Kind:          s.Kind,
		StartedAt:     run.startedAt,
		PromptVersion: s.PromptVersion,
		WindowStart:   s.WindowStart,
		WindowEnd:     s.WindowEnd,
		Instruction:   s.Instruction,
	})
	if err != nil {
		r.logger.WarnContext(ctx, "ai.run.create failed", "request_id", run.requestID, "kind", s.Kind, "error", err)
	} else {
		run.id = id
	}
	r.logger.InfoContext(ctx, "ai.run.start", "run_id", run.id, "request_id", run.requestID, "kind", s.Kind)
	return context.WithValue(ctx, ctxKey{}, run), run
}

// Finish writes the run totals and logs ai.run.end. Only the first call has
// an effect. The write ignores ctx cancellation so a cancelled request still
// records its status.
func (run *Run) Finish(ctx context.Context, status Status, errKey string) {
	if run == nil {
		return
	}
	run.mu.Lock()
	if run.finished {
		run.mu.Unlock()
		return
	}
	run.finished = true
	end := RunEnd{
		ID:               run.id,
		Status:           status,
		FinishedAt:       run.rec.now(),
		Provider:         run.provider,
		Model:            run.model,
		PromptHash:       run.promptHash,
		Iterations:       run.iterations,
		InputTokens:      run.inputTokens,
		OutputTokens:     run.outputTokens,
		CacheReadTokens:  run.cacheReadTokens,
		CacheWriteTokens: run.cacheWriteTokens,
		ErrorKey:         errKey,
	}
	run.mu.Unlock()

	if run.id != 0 {
		if err := run.rec.store.FinishRun(context.WithoutCancel(ctx), end); err != nil {
			run.warn(ctx, "ai.run.finish failed", err)
		}
	}
	run.rec.logger.InfoContext(ctx, "ai.run.end",
		"run_id", run.id,
		"request_id", run.requestID,
		"kind", run.kind,
		"provider", end.Provider,
		"model", end.Model,
		"iterations", end.Iterations,
		"input_tokens", end.InputTokens,
		"output_tokens", end.OutputTokens,
		"cache_read_tokens", end.CacheReadTokens,
		"cache_write_tokens", end.CacheWriteTokens,
		"status", status,
		"duration_ms", end.FinishedAt.Sub(run.startedAt).Milliseconds(),
	)
}

// ToolStep records one tool call. It is the hook used by agent tools.
func (run *Run) ToolStep(ctx context.Context, name string, args, result json.RawMessage, isErr bool, dur time.Duration) {
	if run == nil {
		return
	}
	run.mu.Lock()
	run.seq++
	seq := run.seq
	run.mu.Unlock()

	run.addStep(ctx, Step{
		Seq:        seq,
		Type:       StepToolCall,
		Name:       name,
		ArgsJSON:   string(args),
		ResultJSON: string(result),
		IsError:    isErr,
		DurationMs: dur.Milliseconds(),
	})
}

// addStep writes step for this run, ignoring ctx cancellation.
func (run *Run) addStep(ctx context.Context, step Step) {
	if run.id == 0 {
		return
	}
	step.RunID = run.id
	if err := run.rec.store.AddStep(context.WithoutCancel(ctx), step); err != nil {
		run.warn(ctx, "ai.run.step failed", err)
	}
}

func (run *Run) warn(ctx context.Context, msg string, err error) {
	run.rec.logger.WarnContext(ctx, msg, "run_id", run.id, "request_id", run.requestID, "kind", run.kind, "error", err)
}
