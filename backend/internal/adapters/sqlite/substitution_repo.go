package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/substitution"
)

// SubstitutionRepo implements substitution.Store backed by SQLite.
type SubstitutionRepo struct {
	conn
}

// NewSubstitutionRepo creates a SQLite-backed substitution store.
func NewSubstitutionRepo(db *sql.DB) *SubstitutionRepo {
	return &SubstitutionRepo{conn: newConn(db)}
}

var _ substitution.Store = (*SubstitutionRepo)(nil)

func (r *SubstitutionRepo) Hidden(ctx context.Context, foodID int64) (substitution.HiddenVariant, bool, error) {
	row, err := r.queriesFor(ctx).GetHiddenVariant(ctx, foodID)
	if errors.Is(err, sql.ErrNoRows) {
		return substitution.HiddenVariant{}, false, nil
	}
	if err != nil {
		return substitution.HiddenVariant{}, false, err
	}
	return substitution.HiddenVariant{FoodID: row.FoodID, BaseFoodID: row.BaseFoodID.Int64, SessionID: row.SessionID.Int64}, true, nil
}

func (r *SubstitutionRepo) PutHidden(ctx context.Context, v substitution.HiddenVariant) error {
	return r.queriesFor(ctx).UpsertHiddenVariant(ctx, sqlcgen.UpsertHiddenVariantParams{
		FoodID:     v.FoodID,
		BaseFoodID: sql.NullInt64{Int64: v.BaseFoodID, Valid: v.BaseFoodID != 0},
		SessionID:  sql.NullInt64{Int64: v.SessionID, Valid: v.SessionID != 0},
	})
}

func (r *SubstitutionRepo) ClearSession(ctx context.Context, sessionID int64) error {
	return r.queriesFor(ctx).ClearHiddenVariantSession(ctx, sql.NullInt64{Int64: sessionID, Valid: true})
}

func (r *SubstitutionRepo) UnusedHidden(ctx context.Context) ([]int64, error) {
	return r.queriesFor(ctx).ListUnusedHiddenVariantIDs(ctx)
}

// SetSessionFood points the session and its plate components at foodID and
// bumps the updated_at of their plates.
func (r *SubstitutionRepo) SetSessionFood(ctx context.Context, sessionID, foodID int64) error {
	return r.inTx(ctx, func(ctx context.Context) error {
		q := r.queriesFor(ctx)
		if err := q.SetSessionFood(ctx, sqlcgen.SetSessionFoodParams{FoodID: foodID, ID: sessionID}); err != nil {
			return err
		}
		if err := q.SetSessionComponentsFood(ctx, sqlcgen.SetSessionComponentsFoodParams{FoodID: foodID, CookSessionID: sql.NullInt64{Int64: sessionID, Valid: true}}); err != nil {
			return err
		}
		return q.TouchPlatesOfSession(ctx, sql.NullInt64{Int64: sessionID, Valid: true})
	})
}

func (r *SubstitutionRepo) DeleteHidden(ctx context.Context, foodID int64) error {
	return r.queriesFor(ctx).DeleteHiddenVariant(ctx, foodID)
}

// CountSwap counts one more swap of from for to in recipe base; it reports
// true exactly once, when the swap reaches its second time.
func (r *SubstitutionRepo) CountSwap(ctx context.Context, baseID, fromID, toID int64) (bool, error) {
	q := r.queriesFor(ctx)
	row, err := q.CountSubstitutionSwap(ctx, sqlcgen.CountSubstitutionSwapParams{BaseFoodID: baseID, FromFoodID: fromID, ToFoodID: toID})
	if err != nil {
		return false, err
	}
	if row.Swaps < 2 || row.Offered != 0 {
		return false, nil
	}
	err = q.MarkSubstitutionSwapOffered(ctx, sqlcgen.MarkSubstitutionSwapOfferedParams{BaseFoodID: baseID, FromFoodID: fromID, ToFoodID: toID})
	return err == nil, err
}
