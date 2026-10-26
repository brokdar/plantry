package airun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// Meter wraps inner so every call is recorded as an llm_call step of the run
// in ctx. A call made without a run gets its own untracked run.
func (r *Recorder) Meter(inner llm.Client, provider string) llm.Client {
	return &meteredClient{rec: r, inner: inner, provider: provider}
}

type meteredClient struct {
	rec      *Recorder
	inner    llm.Client
	provider string
}

// Stream calls inner.Stream and records the call. The response and error are
// returned unchanged.
func (m *meteredClient) Stream(ctx context.Context, req llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	run := FromContext(ctx)
	implicit := run == nil
	if implicit {
		ctx, run = m.rec.Start(ctx, Start{Kind: KindUntracked})
	}

	started := m.rec.now()
	resp, err := m.inner.Stream(ctx, req, out)
	run.llmCall(ctx, m.provider, req, resp, err, m.rec.now().Sub(started))

	if implicit {
		status := StatusOK
		switch {
		case errors.Is(err, context.Canceled):
			status = StatusCancelled
		case err != nil:
			status = StatusError
		}
		run.Finish(ctx, status, "")
	}
	return resp, err
}

// Complete goes through Stream so tokens are recorded. Every adapter's
// Complete is Stream plus concatenating the response's text blocks (see the
// llm.Client contract), so this returns the same text inner.Complete would.
func (m *meteredClient) Complete(ctx context.Context, req llm.Request) (string, error) {
	events := make(chan llm.Event, 16)
	done := make(chan struct{})
	go func() {
		for range events {
		}
		close(done)
	}()
	resp, err := m.Stream(ctx, req, events)
	<-done
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", errors.New("airun: nil response")
	}
	var sb strings.Builder
	for _, block := range resp.Message.Content {
		if block.Type == llm.ContentTypeText {
			sb.WriteString(block.Text)
		}
	}
	return sb.String(), nil
}

// llmCall folds one LLM call into the run totals and records it as a step.
// The provider, model and prompt hash of the first call are kept.
func (run *Run) llmCall(ctx context.Context, provider string, req llm.Request, resp *llm.Response, callErr error, dur time.Duration) {
	var usage llm.Usage
	if resp != nil {
		usage = resp.Usage
	}

	run.mu.Lock()
	run.seq++
	seq := run.seq
	run.iterations++
	run.inputTokens += int64(usage.InputTokens)
	run.outputTokens += int64(usage.OutputTokens)
	run.cacheReadTokens += int64(usage.CacheReadTokens)
	run.cacheWriteTokens += int64(usage.CacheWriteTokens)
	if run.provider == "" {
		run.provider = provider
		run.model = req.Model
	}
	// Only the first call with a system prompt tries to store it.
	storePrompt := !run.promptTried && req.System != "" && run.id != 0
	if storePrompt {
		run.promptTried = true
	}
	run.mu.Unlock()

	if storePrompt {
		sum := sha256.Sum256([]byte(req.System))
		hash := hex.EncodeToString(sum[:])
		if err := run.rec.store.UpsertPrompt(context.WithoutCancel(ctx), hash, req.System); err != nil {
			run.warn(ctx, "ai.run.prompt failed", err)
		} else {
			// Set only once stored, so the run never points at a missing prompt.
			run.mu.Lock()
			run.promptHash = hash
			run.mu.Unlock()
		}
	}
	run.addStep(ctx, Step{
		Seq:          seq,
		Type:         StepLLMCall,
		Name:         req.Model,
		IsError:      callErr != nil,
		DurationMs:   dur.Milliseconds(),
		InputTokens:  int64(usage.InputTokens),
		OutputTokens: int64(usage.OutputTokens),
	})
}
