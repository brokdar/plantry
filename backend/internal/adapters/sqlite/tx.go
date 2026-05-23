package sqlite

import (
	"context"
	"database/sql"

	"github.com/jaltszeimer/plantry/backend/internal/domain/feedback"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// TxRunner provides transactional wrappers for multi-aggregate operations.
type TxRunner struct {
	db *sql.DB
}

// NewTxRunner creates a TxRunner bound to db.
func NewTxRunner(db *sql.DB) *TxRunner {
	return &TxRunner{db: db}
}

// RunInPresetTx wraps fn in a single transaction, binding preset + plate
// repositories to the same tx. Both commit or both roll back. Used by the
// apply pipeline so plates are materialised atomically with the preset's
// last_used_at update.
func (t *TxRunner) RunInPresetTx(ctx context.Context, fn func(preset.Repository, plate.Repository) error) error {
	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	presets := newPresetRepoTx(tx)
	plates := newPlateRepoTx(tx)

	if err := fn(presets, plates); err != nil {
		return err
	}
	return tx.Commit()
}

// RunInFeedbackTx wraps fn in a single transaction, binding feedback, food,
// and profile repositories to the same tx. Used by the feedback service so
// recording a plate rating atomically updates the feedback row, the
// foods' cook_count, and the profile preferences map.
func (t *TxRunner) RunInFeedbackTx(ctx context.Context, fn func(feedback.Repository, food.Repository, profile.Repository) error) error {
	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	feedbackRepo := newFeedbackRepoTx(tx)
	foodRepo := newFoodRepoTx(tx)
	profileRepo := newProfileRepoTx(tx)

	if err := fn(feedbackRepo, foodRepo, profileRepo); err != nil {
		return err
	}
	return tx.Commit()
}
