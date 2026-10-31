package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/shopping"
)

// ShoppingRepo implements shopping.Source: one query for the sessions and
// one for the leaf components of a range.
type ShoppingRepo struct {
	conn
}

// NewShoppingRepo constructs a ShoppingRepo.
func NewShoppingRepo(db *sql.DB) *ShoppingRepo {
	return &ShoppingRepo{conn: conn{db: db}}
}

// SessionsCooking returns the sessions with cook_date in [from, to].
func (r *ShoppingRepo) SessionsCooking(ctx context.Context, from, to time.Time) ([]plate.CookSession, error) {
	rows, err := r.queriesFor(ctx).ListCookSessionsByCookDate(ctx, sqlcgen.ListCookSessionsByCookDateParams{
		FromDate: from.Format(dateLayout), ToDate: to.Format(dateLayout),
	})
	if err != nil {
		return nil, err
	}
	out := make([]plate.CookSession, len(rows))
	for i := range rows {
		if err := mapCookSessionToDomain(&rows[i], &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// LeafUses returns the leaf components on plates dated in [from, to].
func (r *ShoppingRepo) LeafUses(ctx context.Context, from, to time.Time) ([]shopping.LeafUse, error) {
	rows, err := r.queriesFor(ctx).ListLeafPlateComponentsInRange(ctx, sqlcgen.ListLeafPlateComponentsInRangeParams{
		FromDate: from.Format(dateLayout), ToDate: to.Format(dateLayout),
	})
	if err != nil {
		return nil, err
	}
	out := make([]shopping.LeafUse, len(rows))
	for i, row := range rows {
		date, _ := time.Parse(dateLayout, row.Date) //nolint:errcheck // layout controlled by the service
		out[i] = shopping.LeafUse{
			FoodID: row.FoodID,
			Date:   date,
			SlotID: row.SlotID,
			Eaters: row.Eaters,
			Amount: row.Amount.Float64,
			Unit:   row.Unit.String,
			Grams:  row.Grams.Float64,
		}
	}
	return out, nil
}
