// Package actor carries who is making a change through a context (D20): the
// user (HTTP) or the agent (a chat run).
package actor

import "context"

// Kind is who made a change.
type Kind string

// Kinds. Phase 3 adds the generator.
const (
	User  Kind = "user"
	Agent Kind = "agent"
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

// From returns the actor set by With, or the user when ctx carries none.
func From(ctx context.Context) Actor {
	if a, ok := ctx.Value(ctxKey{}).(Actor); ok {
		return a
	}
	return Actor{Kind: User}
}
