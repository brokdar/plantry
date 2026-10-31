package actor_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
)

func TestFrom_DefaultsToUser(t *testing.T) {
	assert.Equal(t, actor.Actor{Kind: actor.User}, actor.From(context.Background()))
}

// TestFrom_FallbackIsLogged: a ctx without an actor (a write path that forgot
// actor.With) is visible at debug level, not silent.
func TestFrom_FallbackIsLogged(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	actor.From(actor.With(context.Background(), actor.Agent, 1))
	assert.Empty(t, logs.String(), "an explicit actor logs nothing")

	actor.From(context.Background())
	assert.Contains(t, logs.String(), "level=DEBUG")
	assert.Contains(t, logs.String(), "actor.fallback_user")
}

func TestWith_RoundTrips(t *testing.T) {
	tests := []struct {
		name  string
		kind  actor.Kind
		runID int64
	}{
		{"user", actor.User, 0},
		{"agent with run", actor.Agent, 42},
		{"agent without run", actor.Agent, 0},
		{"generator with run", actor.Generator, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := actor.With(context.Background(), tt.kind, tt.runID)
			assert.Equal(t, actor.Actor{Kind: tt.kind, RunID: tt.runID}, actor.From(ctx))
		})
	}
}

func TestWith_InnerWins(t *testing.T) {
	ctx := actor.With(context.Background(), actor.User, 0)
	ctx = actor.With(ctx, actor.Agent, 7)
	assert.Equal(t, actor.Actor{Kind: actor.Agent, RunID: 7}, actor.From(ctx))
}
