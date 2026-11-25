package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/flavor"
)

// FoodAttributeRepo implements flavor.Store backed by SQLite.
type FoodAttributeRepo struct {
	conn
}

// NewFoodAttributeRepo creates a SQLite-backed attribute store.
func NewFoodAttributeRepo(db *sql.DB) *FoodAttributeRepo {
	return &FoodAttributeRepo{conn: newConn(db)}
}

var _ flavor.Store = (*FoodAttributeRepo)(nil)

// Get returns food foodID's stored record; an empty one when none.
func (r *FoodAttributeRepo) Get(ctx context.Context, foodID int64) (flavor.Record, error) {
	row, err := r.queriesFor(ctx).GetFoodAttributes(ctx, foodID)
	if errors.Is(err, sql.ErrNoRows) {
		return flavor.Record{FoodID: foodID}, nil
	}
	if err != nil {
		return flavor.Record{}, fmt.Errorf("get food attributes %d: %w", foodID, err)
	}
	return attributeRecord(row)
}

// Many returns the stored records of the foods that have one.
func (r *FoodAttributeRepo) Many(ctx context.Context, foodIDs []int64) (map[int64]flavor.Record, error) {
	out := make(map[int64]flavor.Record, len(foodIDs))
	if len(foodIDs) == 0 {
		return out, nil
	}
	rows, err := r.queriesFor(ctx).ListFoodAttributes(ctx, foodIDs)
	if err != nil {
		return nil, fmt.Errorf("list food attributes: %w", err)
	}
	for _, row := range rows {
		rec, err := attributeRecord(row)
		if err != nil {
			return nil, err
		}
		out[rec.FoodID] = rec
	}
	return out, nil
}

// Put stores rec in place of the food's record.
func (r *FoodAttributeRepo) Put(ctx context.Context, rec flavor.Record) error {
	values, err := json.Marshal(rec.Values)
	if err != nil {
		return fmt.Errorf("food attributes %d: %w", rec.FoodID, err)
	}
	sources, err := json.Marshal(rec.Sources)
	if err != nil {
		return fmt.Errorf("food attribute sources %d: %w", rec.FoodID, err)
	}
	err = r.queriesFor(ctx).UpsertFoodAttributes(ctx, sqlcgen.UpsertFoodAttributesParams{
		FoodID: rec.FoodID, Version: int64(rec.Version), ValuesJson: string(values), SourcesJson: string(sources),
	})
	if err != nil {
		return fmt.Errorf("put food attributes %d: %w", rec.FoodID, err)
	}
	return nil
}

// Delete removes the food's record.
func (r *FoodAttributeRepo) Delete(ctx context.Context, foodID int64) error {
	if err := r.queriesFor(ctx).DeleteFoodAttributes(ctx, foodID); err != nil {
		return fmt.Errorf("delete food attributes %d: %w", foodID, err)
	}
	return nil
}

// UnlabelledRecipes returns the composed foods with ingredients that hold no
// assistant value yet, by id.
func (r *FoodAttributeRepo) UnlabelledRecipes(ctx context.Context) ([]int64, error) {
	ids, err := r.queriesFor(ctx).ListUnlabelledRecipeIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list unlabelled recipes: %w", err)
	}
	return ids, nil
}

func attributeRecord(row sqlcgen.FoodAttribute) (flavor.Record, error) {
	rec := flavor.Record{FoodID: row.FoodID, Version: int(row.Version)}
	if err := json.Unmarshal([]byte(row.ValuesJson), &rec.Values); err != nil {
		return flavor.Record{}, fmt.Errorf("food attributes %d: %w", row.FoodID, err)
	}
	if err := json.Unmarshal([]byte(row.SourcesJson), &rec.Sources); err != nil {
		return flavor.Record{}, fmt.Errorf("food attribute sources %d: %w", row.FoodID, err)
	}
	return rec, nil
}
