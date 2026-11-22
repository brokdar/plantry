package planning

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// DayPreview is what a day regenerate would do before it runs: the draft
// plates it replaces, the batches their removal moves (R1, the meal
// operations' AlsoChanged) and the confirmed plates among those moves, which
// become their batch's cooking meal.
type DayPreview struct {
	Replaced          []int64
	AlsoChanged       []plate.AlsoChanged
	ConfirmedAffected []int64
}

// PreviewDay previews a day regenerate of date in the window starting on
// start. It runs the run's own removal (RemoveMeals) in a unit of work that
// is rolled back, so the preview is what the run's save then does. date must
// be in the window and not before today (domain.ErrInvalidScope).
func (s *Service) PreviewDay(ctx context.Context, start, today, date time.Time) (*DayPreview, error) {
	sd, err := plate.IntSetting(ctx, s.d.Settings, settings.KeyPlanShoppingDay)
	if err != nil {
		return nil, err
	}
	start = dateOnly(start)
	if !window.IsStart(start, sd) {
		return nil, fmt.Errorf("%w: %s", domain.ErrInvalidPlanStart, start.Format(time.DateOnly))
	}
	d, err := dayInWindow(window.Containing(start, sd), dateOnly(today), date)
	if err != nil {
		return nil, err
	}
	out := &DayPreview{Replaced: []int64{}, AlsoChanged: []plate.AlsoChanged{}, ConfirmedAffected: []int64{}}
	err = s.dryRun(ctx, func(ctx context.Context) error {
		plates, err := s.d.PlateRows.ListByDateRange(ctx, d, d)
		if err != nil {
			return err
		}
		for _, p := range dayDrafts(plates, d) {
			out.Replaced = append(out.Replaced, p.ID)
		}
		res, err := s.removeDay(ctx, out.Replaced, window.Containing(start, sd), dateOnly(today))
		if err != nil {
			return err
		}
		for _, m := range res.AlsoChanged {
			out.AlsoChanged = append(out.AlsoChanged, m)
			if m.PlateID == 0 || slices.Contains(out.ConfirmedAffected, m.PlateID) {
				continue
			}
			p, err := s.d.Plates.Get(ctx, m.PlateID)
			if err != nil {
				return err
			}
			if p.State == plate.StateConfirmed {
				out.ConfirmedAffected = append(out.ConfirmedAffected, p.ID)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
