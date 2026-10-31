package food

import (
	"context"
	"time"
)

// History is a composed food's eating history relative to a date (D16). It
// is derived from plates and cook sessions, never stored.
type History struct {
	// LastEaten is the latest plate on or before the date that holds the
	// food and is not marked not_eaten; nil when there is none.
	LastEaten *time.Time
	// TimesCooked counts the food's cook sessions on or before the date that
	// have at least one linked plate not marked not_eaten.
	TimesCooked int
}

// HistoryReader is the port for the derived per-food read data.
type HistoryReader interface {
	// History returns the history of each given food as of today. Foods
	// without any history may be missing from the map.
	History(ctx context.Context, ids []int64, today time.Time) (map[int64]History, error)
	// UsedInCounts returns, per leaf, the number of recipes using it. Leaves
	// used nowhere may be missing from the map.
	UsedInCounts(ctx context.Context, ids []int64) (map[int64]int, error)
}

// Derive sets the derived fields as of today: History on composed foods and
// UsedInCount on leaves, with one query per kind.
func (s *Service) Derive(ctx context.Context, today time.Time, foods ...*Food) error {
	var composed, leaves []int64
	for _, f := range foods {
		if f.Kind == KindComposed {
			composed = append(composed, f.ID)
		} else {
			leaves = append(leaves, f.ID)
		}
	}
	var hist map[int64]History
	if len(composed) > 0 {
		var err error
		if hist, err = s.repo.History(ctx, composed, today); err != nil {
			return err
		}
	}
	var used map[int64]int
	if len(leaves) > 0 {
		var err error
		if used, err = s.repo.UsedInCounts(ctx, leaves); err != nil {
			return err
		}
	}
	for _, f := range foods {
		if f.Kind == KindComposed {
			f.History = hist[f.ID]
		} else {
			f.UsedInCount = used[f.ID]
		}
	}
	return nil
}
