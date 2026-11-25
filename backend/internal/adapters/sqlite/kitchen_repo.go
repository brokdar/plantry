package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/kitchen"
)

// KitchenRepo implements kitchen.Repository backed by SQLite.
type KitchenRepo struct {
	conn
}

// NewKitchenRepo creates a SQLite-backed kitchen repository.
func NewKitchenRepo(db *sql.DB) *KitchenRepo {
	return &KitchenRepo{conn: newConn(db)}
}

var _ kitchen.Repository = (*KitchenRepo)(nil)

func (r *KitchenRepo) Basics(ctx context.Context) ([]kitchen.Food, error) {
	rows, err := r.queriesFor(ctx).ListBasicsAtHome(ctx)
	if err != nil {
		return nil, fmt.Errorf("list basics at home: %w", err)
	}
	out := make([]kitchen.Food, len(rows))
	for i, row := range rows {
		out[i] = kitchen.Food{ID: row.ID, Name: row.Name}
	}
	return out, nil
}

func (r *KitchenRepo) AddBasic(ctx context.Context, foodID int64) error {
	return fkAsNotFound(r.queriesFor(ctx).AddBasicAtHome(ctx, foodID))
}

func (r *KitchenRepo) RemoveBasic(ctx context.Context, foodID int64) error {
	return r.queriesFor(ctx).RemoveBasicAtHome(ctx, foodID)
}

func (r *KitchenRepo) NeverItems(ctx context.Context) ([]kitchen.Food, error) {
	rows, err := r.queriesFor(ctx).ListNeverItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("list never items: %w", err)
	}
	out := make([]kitchen.Food, len(rows))
	for i, row := range rows {
		out[i] = kitchen.Food{ID: row.ID, Name: row.Name}
	}
	return out, nil
}

func (r *KitchenRepo) AddNeverItem(ctx context.Context, foodID int64) error {
	return fkAsNotFound(r.queriesFor(ctx).AddNeverItem(ctx, foodID))
}

func (r *KitchenRepo) RemoveNeverItem(ctx context.Context, foodID int64) error {
	return r.queriesFor(ctx).RemoveNeverItem(ctx, foodID)
}

// fkAsNotFound maps a missing food to domain.ErrNotFound.
func fkAsNotFound(err error) error {
	if isForeignKeyViolation(err) {
		return domain.ErrNotFound
	}
	return err
}
