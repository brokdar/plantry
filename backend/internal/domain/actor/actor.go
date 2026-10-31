// Package actor carries who is making a change through a context (D20): the
// user (HTTP), the agent (a chat run) or the plan generator (a generate run).
package actor

import (
	"context"
	"log/slog"
)

// Kind is who made a change.
type Kind string

// Kinds. Rows the user creates are confirmed; rows the agent or the
// generator creates are drafts tagged with their run (Phase 3 D2).
const (
	User      Kind = "user"
	Agent     Kind = "agent"
	Generator Kind = "generator"
)

// Actor is the change's author. RunID is the AI run that made it, 0 when
// there is none.
type Actor struct {
	Kind  Kind
	RunID int64
}

type ctxKey struct{}

// With returns ctx carrying the actor kind and the AI run id (0 for none).
func With(ctx context.Context, kind Kind, runID int64) context.Context {
	return context.WithValue(ctx, ctxKey{}, Actor{Kind: kind, RunID: runID})
}

// From returns the actor set by With, or the user when ctx carries none. The
// fallback is logged at debug level so a write path missing With shows up.
func From(ctx context.Context) Actor {
	if a, ok := ctx.Value(ctxKey{}).(Actor); ok {
		return a
	}
	slog.DebugContext(ctx, "actor.fallback_user")
	return Actor{Kind: User}
}
