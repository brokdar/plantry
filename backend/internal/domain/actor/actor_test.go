package actor_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
)

func TestFrom_DefaultsToUser(t *testing.T) {
	assert.Equal(t, actor.Actor{Kind: actor.User}, actor.From(context.Background()))
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
