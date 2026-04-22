package feedback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// PlateReader loads a plate (with its components) by ID. Implemented by
// plate.Repository; a narrow interface here keeps the Service port-agnostic.
type PlateReader interface {
	Get(ctx context.Context, id int64) (*plate.Plate, error)
}

// ComponentReader loads a component (with its tags) by ID. Implemented by
// component.Repository. Reads happen outside the tx — we only need tags to
// feed the preference heuristic.
type ComponentReader interface {
	Get(ctx context.Context, id int64) (*component.Component, error)
}

// Service records and deletes per-plate feedback, cascading its side effects
// (cook-count updates on components; preference tag accumulation on the user
// profile) atomically inside a single transaction.
type Service struct {
	tx         TxRunner
	plates     PlateReader
	components ComponentReader
	now        func() time.Time
}

// NewService constructs a feedback service.
func NewService(tx TxRunner, plates PlateReader, components ComponentReader) *Service {
	return &Service{tx: tx, plates: plates, components: components, now: func() time.Time { return time.Now().UTC() }}
}

// RecordFeedback upserts feedback for a plate and cascades side effects:
//
//   - Transitioning INTO a status that implies the user cooked the meal
//     (cooked, loved) increments cook_count and sets last_cooked_at on every
//     component of the plate — but ONLY on transition. Re-marking a plate
//     cooked (cooked→cooked, cooked→loved, loved→cooked) is a no-op for
//     cook_count to keep the counter stable on edits.
//   - Transitioning INTO loved/disliked appends the plate's component tags
//     to the profile's preferences map (see profile.ApplyFeedback). The
//     heuristic is append-only; changing from loved→disliked later does NOT
//     retract the earlier likes.
//   - Transitioning OUT of cooked/loved (e.g. user re-rates as skipped) does
//     not decrement cook_count or retract preferences. The cook happened.
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

	err = s.tx.RunInFeedbackTx(ctx, func(fbRepo Repository, cmpRepo component.Repository, prfRepo profile.Repository) error {
		prior, err := fbRepo.Get(ctx, plateID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return err
		}

		fb := &PlateFeedback{PlateID: plateID, Status: status, Note: note}
		if err := fbRepo.Upsert(ctx, fb); err != nil {
			return err
		}
		result = fb

		priorCooks := prior != nil && prior.Status.IncrementsCookCount()
		if status.IncrementsCookCount() && !priorCooks {
			for _, pc := range p.Components {
				if err := cmpRepo.MarkCooked(ctx, pc.ComponentID, now); err != nil {
					return err
				}
			}
		}

		if status.TouchesPreferences() && len(tags) > 0 {
			prof, err := prfRepo.Get(ctx)
			if err != nil {
				return err
			}
			prof.Preferences = profile.ApplyFeedback(prof.Preferences, string(status), tags)
			if _, err := prfRepo.Update(ctx, prof); err != nil {
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
// increments on components are NOT reversed (cook_count is a count of
// cooking events, not of current feedback state).
func (s *Service) DeleteFeedback(ctx context.Context, plateID int64) error {
	return s.tx.RunInFeedbackTx(ctx, func(fbRepo Repository, _ component.Repository, _ profile.Repository) error {
		return fbRepo.Delete(ctx, plateID)
	})
}

// collectTags returns the deduped union of tags across every component on the
// plate. Used only to drive the preference heuristic, so we keep the set
// flat.
func (s *Service) collectTags(ctx context.Context, p *plate.Plate) ([]string, error) {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, pc := range p.Components {
		c, err := s.components.Get(ctx, pc.ComponentID)
		if err != nil {
			return nil, err
		}
		for _, tag := range c.Tags {
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}
	return out, nil
}
