package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/chefhelp"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// OutcomeRepo keeps the outcome signals the report reads: help verdicts and
// import-check outcomes.
type OutcomeRepo struct {
	conn
}

// NewOutcomeRepo creates a SQLite-backed outcome store.
func NewOutcomeRepo(db *sql.DB) *OutcomeRepo { return &OutcomeRepo{conn: newConn(db)} }

var (
	_ chefhelp.VerdictStore = (*OutcomeRepo)(nil)
	_ importer.OutcomeStore = (*OutcomeRepo)(nil)
)

// SetVerdict stores (or changes) the verdict of a chef_help run.
func (r *OutcomeRepo) SetVerdict(ctx context.Context, runID int64, verdict string, now time.Time) error {
	n, err := r.queriesFor(ctx).UpsertHelpVerdict(ctx, sqlcgen.UpsertHelpVerdictParams{
		Verdict: verdict, Now: formatTime(now), RunID: runID,
	})
	if err != nil {
		return fmt.Errorf("set help verdict: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("%w: help run %d", domain.ErrNotFound, runID)
	}
	return nil
}

// SaveCheckOutcome stores the outcome of one import check.
func (r *OutcomeRepo) SaveCheckOutcome(ctx context.Context, o importer.CheckOutcome, now time.Time) error {
	kept, err := json.Marshal(nonNil(o.Kept))
	if err != nil {
		return fmt.Errorf("encode kept: %w", err)
	}
	undone, err := json.Marshal(nonNil(o.Undone))
	if err != nil {
		return fmt.Errorf("encode undone: %w", err)
	}
	err = r.queriesFor(ctx).UpsertImportCheckOutcome(ctx, sqlcgen.UpsertImportCheckOutcomeParams{
		RunID: o.RunID, FoodID: sql.NullInt64{Int64: o.FoodID, Valid: o.FoodID > 0},
		Kept: string(kept), Undone: string(undone), Touch: o.Touch, Flags: int64(o.Flags), CreatedAt: formatTime(now),
	})
	if err != nil {
		return fmt.Errorf("save import check outcome: %w", err)
	}
	return nil
}
