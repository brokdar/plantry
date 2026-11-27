package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/recipechange"
)

// RecipeChangeRepo implements recipechange.Store backed by SQLite.
type RecipeChangeRepo struct {
	conn
}

// NewRecipeChangeRepo creates a SQLite-backed recipe change store.
func NewRecipeChangeRepo(db *sql.DB) *RecipeChangeRepo {
	return &RecipeChangeRepo{conn: newConn(db)}
}

var _ recipechange.Store = (*RecipeChangeRepo)(nil)

func (r *RecipeChangeRepo) Changes(ctx context.Context, foodID int64) ([]string, error) {
	raw, err := r.queriesFor(ctx).GetFoodChanges(ctx, foodID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("changes of food %d: %w", foodID, err)
	}
	return out, nil
}

func (r *RecipeChangeRepo) PutChanges(ctx context.Context, foodID int64, changes []string) error {
	if len(changes) == 0 {
		return r.queriesFor(ctx).DeleteFoodChanges(ctx, foodID)
	}
	b, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	return r.queriesFor(ctx).PutFoodChanges(ctx, sqlcgen.PutFoodChangesParams{FoodID: foodID, ChangesJson: string(b)})
}

func (r *RecipeChangeRepo) ConfirmedDates(ctx context.Context, foodID int64, today time.Time) ([]string, error) {
	return r.queriesFor(ctx).ConfirmedPlateDatesOf(ctx, sqlcgen.ConfirmedPlateDatesOfParams{FoodID: foodID, Today: today.Format("2006-01-02")})
}
