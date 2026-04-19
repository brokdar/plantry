package llm

import (
	"context"
	"errors"
)

// ErrProviderMissing is returned by a Resolver when the current effective
// configuration does not name a provider (e.g. AI is disabled).
var ErrProviderMissing = errors.New("ai provider not configured")

// Resolver yields the LLM Client and model to use for a request. Implementations
// may be static (fixed at startup) or dynamic (derived from live settings).
//
// Current is called at the start of each Chat turn. Callers hold the returned
// Client for the duration of one request; if settings change mid-stream, the
// next Chat call sees the new client.
type Resolver interface {
	Current(ctx context.Context) (Client, string, error)
}

// StaticResolver wraps a fixed Client + model; convenient for tests and for
// the fake/dev wiring. Use DynamicResolver (in adapters/llmresolver) for
// settings-driven hot reload.
func StaticResolver(c Client, model string) Resolver {
	return &staticResolver{c: c, model: model}
}

type staticResolver struct {
	c     Client
	model string
}

func (s *staticResolver) Current(_ context.Context) (Client, string, error) {
	if s.c == nil {
		return nil, "", ErrProviderMissing
	}
	return s.c, s.model, nil
}
