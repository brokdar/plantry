package planner

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Service holds business logic for weeks and orchestrates plates inside them.
type Service struct {
	weeks  WeekRepository
	plates plate.Repository
	tx     TxRunner
}

// NewService creates a planner service.
func NewService(weeks WeekRepository, plates plate.Repository, tx TxRunner) *Service {
	return &Service{weeks: weeks, plates: plates, tx: tx}
}

// Current returns the week containing now (ISO year + week), creating it if missing.
func (s *Service) Current(ctx context.Context, now time.Time) (*Week, error) {
	year, week := now.ISOWeek()
	return s.getOrCreate(ctx, year, week)
}

// ByDate returns the week with the given ISO year + week number, creating it if missing.
func (s *Service) ByDate(ctx context.Context, year, weekNumber int) (*Week, error) {
	if weekNumber < 1 || weekNumber > 53 {
		return nil, fmt.Errorf("%w: week_number out of range", domain.ErrInvalidInput)
	}
	return s.getOrCreate(ctx, year, weekNumber)
}

func (s *Service) getOrCreate(ctx context.Context, year, weekNumber int) (*Week, error) {
	w, err := s.weeks.GetByYearAndNumber(ctx, year, weekNumber)
	if err == nil {
		return s.loadPlates(ctx, w)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	w = &Week{Year: year, WeekNumber: weekNumber}
	if err := s.weeks.Create(ctx, w); err != nil {
		return nil, err
	}
	w.Plates = []plate.Plate{}
	return w, nil
}

// Get returns a week with all plates + components loaded.
func (s *Service) Get(ctx context.Context, id int64) (*Week, error) {
	w, err := s.weeks.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.loadPlates(ctx, w)
}

// List returns paginated weeks (no plates loaded — for the archive view).
func (s *Service) List(ctx context.Context, limit, offset int) ([]Week, int64, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.weeks.List(ctx, limit, offset)
}

func (s *Service) loadPlates(ctx context.Context, w *Week) (*Week, error) {
	plates, err := s.plates.ListByWeek(ctx, w.ID)
	if err != nil {
		return nil, fmt.Errorf("load plates: %w", err)
	}
	if plates == nil {
		plates = []plate.Plate{}
	}
	w.Plates = plates
	return w, nil
}

// Copy deep-clones every plate (and its components) from sourceID into the
// week identified by targetYear/targetWeek, creating the target if missing.
// The whole operation is atomic.
func (s *Service) Copy(ctx context.Context, sourceID int64, targetYear, targetWeek int) (*Week, error) {
	if targetWeek < 1 || targetWeek > 53 {
		return nil, fmt.Errorf("%w: target_week out of range", domain.ErrInvalidInput)
	}
	source, err := s.Get(ctx, sourceID)
	if err != nil {
		return nil, err
	}

	var target *Week
	err = s.tx.RunInTx(ctx, func(weeks WeekRepository, plates plate.Repository) error {
		w, err := weeks.GetByYearAndNumber(ctx, targetYear, targetWeek)
		if err != nil {
			if !errors.Is(err, domain.ErrNotFound) {
				return err
			}
			w = &Week{Year: targetYear, WeekNumber: targetWeek}
			if err := weeks.Create(ctx, w); err != nil {
				return err
			}
		}
		target = w

		for _, src := range source.Plates {
			cloned := plate.Plate{
				WeekID: target.ID,
				Day:    src.Day,
				SlotID: src.SlotID,
				Note:   src.Note,
			}
			for i, pc := range src.Components {
				cloned.Components = append(cloned.Components, plate.PlateComponent{
					ComponentID: pc.ComponentID,
					Portions:    pc.Portions,
					SortOrder:   i,
				})
			}
			if err := plates.Create(ctx, &cloned); err != nil {
				return fmt.Errorf("clone plate %d: %w", src.ID, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, target.ID)
}
