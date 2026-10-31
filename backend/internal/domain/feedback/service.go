package feedback

import (
	"context"
	"fmt"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// PlateReader loads a plate (with its components) by ID.
type PlateReader interface {
	Get(ctx context.Context, id int64) (*plate.Plate, error)
}

// Service records and deletes per-plate feedback. Feedback never touches
// foods: eating history is derived from plates (D16, D17).
type Service struct {
	tx       domain.Transactor
	repo     Repository
	plates   PlateReader
	tags     TagReader
	profiles ProfileStore
}

// NewService constructs a feedback service.
func NewService(tx domain.Transactor, repo Repository, plates PlateReader, tags TagReader, profiles ProfileStore) *Service {
	return &Service{tx: tx, repo: repo, plates: plates, tags: tags, profiles: profiles}
}

// RecordFeedback upserts feedback for a plate. Loved and disliked append the
// tags of the plate's foods to the profile's likes or dislikes; the heuristic
// is append-only and never retracted. Both writes share one transaction.
func (s *Service) RecordFeedback(ctx context.Context, plateID int64, status Status, note *string) (*PlateFeedback, error) {
	if !status.Valid() {
		return nil, fmt.Errorf("%w: %q", domain.ErrInvalidFeedbackStatus, status)
	}

	p, err := s.plates.Get(ctx, plateID)
	if err != nil {
		return nil, err
	}

	var tags []string
	if status.TouchesPreferences() {
		if tags, err = s.collectTags(ctx, p); err != nil {
			return nil, err
		}
	}

	fb := &PlateFeedback{PlateID: plateID, Status: status, Note: note}
	err = s.tx.InTx(ctx, func(ctx context.Context) error {
		if err := s.repo.Upsert(ctx, fb); err != nil {
			return err
		}
		if len(tags) == 0 {
			return nil
		}
		prof, err := s.profiles.Get(ctx)
		if err != nil {
			return err
		}
		prof.Preferences = profile.ApplyFeedback(prof.Preferences, string(status), tags)
		_, err = s.profiles.Update(ctx, prof)
		return err
	})
	if err != nil {
		return nil, err
	}
	return fb, nil
}

// DeleteFeedback removes the feedback row for a plate.
func (s *Service) DeleteFeedback(ctx context.Context, plateID int64) error {
	return s.repo.Delete(ctx, plateID)
}

// collectTags returns the deduped union of tags across every food on the
// plate. Leaf foods don't have tags; composed foods do.
func (s *Service) collectTags(ctx context.Context, p *plate.Plate) ([]string, error) {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, pc := range p.Components {
		tags, err := s.tags.ListTags(ctx, pc.FoodID)
		if err != nil {
			return nil, err
		}
		for _, tag := range tags {
			if _, ok := seen[tag]; ok {
				continue
			}
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}
	return out, nil
}
