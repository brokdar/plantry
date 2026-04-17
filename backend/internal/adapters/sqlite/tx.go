package sqlite

import (
	"context"
	"database/sql"

	"github.com/jaltszeimer/plantry/backend/internal/domain/planner"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/template"
)

// TxRunner implements planner.TxRunner using a SQLite *sql.DB.
type TxRunner struct {
	db *sql.DB
}

// NewTxRunner creates a TxRunner bound to db.
func NewTxRunner(db *sql.DB) *TxRunner {
	return &TxRunner{db: db}
}

// RunInTx wraps fn in a single transaction. The closure receives weeks + plate
// repositories bound to that transaction; both commit or both roll back.
func (t *TxRunner) RunInTx(ctx context.Context, fn func(planner.WeekRepository, plate.Repository) error) error {
	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	weeks := newWeekRepoTx(tx)
	plates := newPlateRepoTx(tx)

	if err := fn(weeks, plates); err != nil {
		return err
	}
	return tx.Commit()
}

// RunInTemplateTx wraps fn in a single transaction, binding template + plate
// repositories to the same tx. Both commit or both roll back.
func (t *TxRunner) RunInTemplateTx(ctx context.Context, fn func(template.Repository, plate.Repository) error) error {
	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	templates := newTemplateRepoTx(tx)
	plates := newPlateRepoTx(tx)

	if err := fn(templates, plates); err != nil {
		return err
	}
	return tx.Commit()
}
