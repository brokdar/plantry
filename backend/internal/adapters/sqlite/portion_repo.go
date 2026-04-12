package sqlite

import (
	"context"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
)

func (r *IngredientRepo) ListPortions(ctx context.Context, ingredientID int64) ([]ingredient.Portion, error) {
	rows, err := r.q.ListPortions(ctx, ingredientID)
	if err != nil {
		return nil, err
	}
	portions := make([]ingredient.Portion, len(rows))
	for i, row := range rows {
		portions[i] = ingredient.Portion{
			IngredientID: row.IngredientID,
			Unit:         row.Unit,
			Grams:        row.Grams,
		}
	}
	return portions, nil
}

func (r *IngredientRepo) UpsertPortion(ctx context.Context, p *ingredient.Portion) error {
	err := r.q.UpsertPortion(ctx, sqlcgen.UpsertPortionParams{
		IngredientID: p.IngredientID,
		Unit:         p.Unit,
		Grams:        p.Grams,
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: ingredient id %d", domain.ErrNotFound, p.IngredientID)
		}
		return err
	}
	return nil
}

func (r *IngredientRepo) DeletePortion(ctx context.Context, ingredientID int64, unit string) error {
	res, err := r.q.DeletePortion(ctx, sqlcgen.DeletePortionParams{
		IngredientID: ingredientID,
		Unit:         unit,
	})
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: portion %d/%s", domain.ErrNotFound, ingredientID, unit)
	}
	return nil
}
