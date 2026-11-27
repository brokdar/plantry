package sqlite

import (
	"context"
	"database/sql"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// FoodOriginRepo stores where matching-added leaves came from (food.OriginStore).
type FoodOriginRepo struct {
	conn
}

// NewFoodOriginRepo constructs a FoodOriginRepo.
func NewFoodOriginRepo(db *sql.DB) *FoodOriginRepo {
	return &FoodOriginRepo{conn: newConn(db)}
}

// AddOrigin records the origin of a leaf the matching path created.
func (r *FoodOriginRepo) AddOrigin(ctx context.Context, foodID int64, o food.Origin, source string) error {
	return r.queriesFor(ctx).InsertFoodOrigin(ctx, sqlcgen.InsertFoodOriginParams{
		FoodID: foodID, Kind: o.Kind, Ref: o.Ref, Source: source,
	})
}

// OriginLeaves lists the leaves created for o.
func (r *FoodOriginRepo) OriginLeaves(ctx context.Context, o food.Origin) ([]int64, error) {
	return r.queriesFor(ctx).ListFoodOriginLeaves(ctx, sqlcgen.ListFoodOriginLeavesParams{Kind: o.Kind, Ref: o.Ref})
}
