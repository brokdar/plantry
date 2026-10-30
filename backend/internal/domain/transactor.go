package domain

import "context"

// Transactor is the unit-of-work port (Phase 2 D21). InTx runs fn in one
// transaction carried by fn's context: every repository called with that
// context joins it. A nested InTx joins the outer transaction.
type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context) error) error
}
