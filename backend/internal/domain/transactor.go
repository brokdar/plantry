package domain

import (
	"context"
	"errors"
)

// Transactor is the unit-of-work port (Phase 2 D21). InTx runs fn in one
// transaction carried by fn's context: every repository called with that
// context joins it. A nested InTx joins the outer transaction: there are no
// savepoints, so a failed nested call leaves its partial writes in the outer
// transaction and its caller must return the error.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// errDryRun rolls back a DryRun's unit of work.
var errDryRun = errors.New("dry run")

// DryRun runs fn in a unit of work that is always rolled back, so the caller
// sees what fn would write without keeping it; fn's own error is returned.
// Never call it inside another InTx: its rollback would end the outer one.
func DryRun(ctx context.Context, tx Transactor, fn func(ctx context.Context) error) error {
	err := tx.InTx(ctx, func(ctx context.Context) error {
		if err := fn(ctx); err != nil {
			return err
		}
		return errDryRun
	})
	if errors.Is(err, errDryRun) {
		return nil
	}
	return err
}
