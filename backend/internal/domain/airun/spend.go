package airun

import (
	"context"
	"fmt"
	"time"
)

// SpendStore sums the stored cost of runs.
type SpendStore interface {
	// SpendUSD sums cost_usd of the runs started in [from, to) whose kind is
	// in kinds (all kinds when kinds is empty).
	SpendUSD(ctx context.Context, from, to time.Time, kinds []Kind) (float64, error)
}

// Spend returns what the runs of kinds (all kinds when empty) started in the
// calendar month of month (in month's location) cost, from the cost each
// run stored when it finished. Unpriced and running runs add nothing.
func Spend(ctx context.Context, s SpendStore, month time.Time, kinds []Kind) (float64, error) {
	from := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, month.Location())
	usd, err := s.SpendUSD(ctx, from, from.AddDate(0, 1, 0), kinds)
	if err != nil {
		return 0, fmt.Errorf("spend %s: %w", from.Format("2006-01"), err)
	}
	return usd, nil
}
