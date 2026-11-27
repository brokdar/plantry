package food_test

import (
	"context"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

func (r *fakeRepo) NeedsCheckLines(context.Context) ([]food.NeedsCheckLine, error) { return nil, nil }
