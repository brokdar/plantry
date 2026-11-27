package feedback

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
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
	taste    Rater
	trials   TrialArchiver
}

// TrialArchiver archives the recipes among ids that were on trial on the
// given day (food.Service.ArchiveDisliked, NEW-4).
type TrialArchiver interface {
	ArchiveDisliked(ctx context.Context, ids []int64, day time.Time) ([]food.ArchivedTrial, error)
}

// WithTrials sets the trial archiver and returns s.
func (s *Service) WithTrials(t TrialArchiver) *Service {
	s.trials = t
	return s
}

// Rater learns from a plate's rating (taste.Service.Rating); old is the
// previous status ("" when unrated).
type Rater interface {
	Rating(ctx context.Context, p *plate.Plate, old, status string) error
}

// WithTaste sets the taste learner and returns s.
func (s *Service) WithTaste(r Rater) *Service {
	s.taste = r
	return s
}

// NewService constructs a feedback service.
func NewService(tx domain.Transactor, repo Repository, plates PlateReader, tags TagReader, profiles ProfileStore) *Service {
	return &Service{tx: tx, repo: repo, plates: plates, tags: tags, profiles: profiles}
}

// RecordFeedback upserts feedback for a plate. Loved and disliked move the
// specific tags of the plate's foods into the profile's likes or dislikes
// (profile.ApplyFeedback). Both writes share one transaction.
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
		if s.taste != nil {
			old := ""
			if p.Feedback != nil {
				old = p.Feedback.Status
			}
			if err := s.taste.Rating(ctx, p, old, string(status)); err != nil {
				return err
			}
		}
		// A disliked first cooking archives a trial recipe: one still on trial
		// on the plate's day (NEW-4).
		if status == StatusDisliked && s.trials != nil {
			if fb.ArchivedTrials, err = s.trials.ArchiveDisliked(ctx, recipeIDs(p), p.Date); err != nil {
				return err
			}
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

// recipeIDs lists the plate's recipes, each once.
func recipeIDs(p *plate.Plate) []int64 {
	var out []int64
	for _, c := range p.Components {
		if !c.IsLeaf() && !slices.Contains(out, c.FoodID) {
			out = append(out, c.FoodID)
		}
	}
	return out
}

// DeleteFeedback removes the feedback row for a plate.
func (s *Service) DeleteFeedback(ctx context.Context, plateID int64) error {
	return s.repo.Delete(ctx, plateID)
}

// tagExplore marks a recipe discovery added that the user has not tried yet.
const tagExplore = "explore"

// genericTag reports whether a tag says nothing about taste: a role name or
// explore (spec §6.6). Such tags never reach the preferences.
func genericTag(tag string) bool {
	t := strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(tag)), " ", "_"), "-", "_")
	return t == tagExplore || food.ValidRole(food.Role(t)) || genericDisplayTags[t]
}

// genericDisplayTags are the role names as people write them ("Side dish").
var genericDisplayTags = map[string]bool{
	"side": true, "side_dish": true, "main_dish": true, "main_course": true,
	"sides": true, "side_dishes": true, "mains": true, "starter": true,
}

// collectTags returns the deduped union of the specific tags across every
// food on the plate. Leaf foods don't have tags; composed foods do.
func (s *Service) collectTags(ctx context.Context, p *plate.Plate) ([]string, error) {
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, pc := range p.Components {
		tags, err := s.tags.ListTags(ctx, pc.FoodID)
		if err != nil {
			return nil, err
		}
		for _, tag := range tags {
			if _, ok := seen[tag]; ok || genericTag(tag) {
				continue
			}
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}
	return out, nil
}
