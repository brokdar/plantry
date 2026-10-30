package feedback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// PlateReader loads a plate (with its components) by ID.
type PlateReader interface {
	Get(ctx context.Context, id int64) (*plate.Plate, error)
}

// Service records and deletes per-plate feedback, cascading its side effects
// (cook-count updates on composed foods; preference tag accumulation on the
// user profile) atomically inside a single transaction.
type Service struct {
	tx       domain.Transactor
	repo     Repository
	plates   PlateReader
	foods    FoodStore
	profiles ProfileStore
	now      func() time.Time
}

// NewService constructs a feedback service.
func NewService(tx domain.Transactor, repo Repository, plates PlateReader, foods FoodStore, profiles ProfileStore) *Service {
	return &Service{tx: tx, repo: repo, plates: plates, foods: foods, profiles: profiles, now: func() time.Time { return time.Now().UTC() }}
}

// RecordFeedback upserts feedback for a plate and cascades side effects:
//
//   - Transitioning INTO a status that implies the user cooked the meal
//     (cooked, loved) increments cook_count and sets last_cooked_at on every
//     food of the plate — but ONLY on transition. Re-marking a plate cooked
//     (cooked→cooked, cooked→loved, loved→cooked) is a no-op for cook_count
//     to keep the counter stable on edits.
//   - Transitioning INTO loved/disliked appends the plate's food tags to the
//     profile's preferences map. The heuristic is append-only.
//   - Transitioning OUT of cooked/loved does not decrement cook_count or
//     retract preferences. The cook happened.
//
// All writes run inside a single tx; partial failure leaves no state visible.
func (s *Service) RecordFeedback(ctx context.Context, plateID int64, status Status, note *string) (*PlateFeedback, error) {
	if !status.Valid() {
		return nil, fmt.Errorf("%w: %q", domain.ErrInvalidFeedbackStatus, status)
	}

	p, err := s.plates.Get(ctx, plateID)
	if err != nil {
		return nil, err
	}

	tags, err := s.collectTags(ctx, p)
	if err != nil {
		return nil, err
	}

	now := s.now()
	var result *PlateFeedback

	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		prior, err := s.repo.Get(ctx, plateID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}

		fb := &PlateFeedback{PlateID: plateID, Status: status, Note: note}
		if err := s.repo.Upsert(ctx, fb); err != nil {
			return err
		}
		result = fb

		priorCooks := prior != nil && prior.Status.IncrementsCookCount()
		if status.IncrementsCookCount() && !priorCooks {
			for _, pc := range p.Components {
				if err := s.foods.MarkCooked(ctx, pc.FoodID, now); err != nil {
					return err
				}
			}
		}

		if status.TouchesPreferences() && len(tags) > 0 {
			prof, err := s.profiles.Get(ctx)
			if err != nil {
				return err
			}
			prof.Preferences = profile.ApplyFeedback(prof.Preferences, string(status), tags)
			if _, err := s.profiles.Update(ctx, prof); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// DeleteFeedback removes the feedback row for a plate. Existing cook-count
// increments on foods are NOT reversed.
func (s *Service) DeleteFeedback(ctx context.Context, plateID int64) error {
	return s.repo.Delete(ctx, plateID)
}

// collectTags returns the deduped union of tags across every food on the
// plate. Leaf foods don't have tags; composed foods do.
func (s *Service) collectTags(ctx context.Context, p *plate.Plate) ([]string, error) {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, pc := range p.Components {
		f, err := s.foods.Get(ctx, pc.FoodID)
		if err != nil {
			return nil, err
		}
		for _, tag := range f.Tags {
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}
	return out, nil
}
