package importer

import (
	"context"
	"errors"
)

// ErrURLBlocked marks a fetch hop rejected by the ctx URL guard.
var ErrURLBlocked = errors.New("importer: url blocked by guard")

type urlGuardKey struct{}

// WithURLGuard returns a ctx whose fetches must pass fn for every URL they
// request, redirect hops included (any error blocks). User-pasted imports
// carry no guard.
func WithURLGuard(ctx context.Context, fn func(string) error) context.Context {
	return context.WithValue(ctx, urlGuardKey{}, fn)
}

// URLGuardFrom returns the guard set by WithURLGuard, or nil.
func URLGuardFrom(ctx context.Context) func(string) error {
	fn, _ := ctx.Value(urlGuardKey{}).(func(string) error)
	return fn
}
