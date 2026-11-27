package food

import (
	"context"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
)

// StartTrial puts recipe id on trial with its To try badge (NEW-1): a
// recipe the assistant added to the library.
func (s *Service) StartTrial(ctx context.Context, id int64) error {
	return s.repo.StartTrial(ctx, id)
}

// MarkTrialSeen clears recipe id's To try badge (NEW-3); its trial runs on.
// A recipe not on trial is left alone.
func (s *Service) MarkTrialSeen(ctx context.Context, id int64) error {
	return s.repo.SetTrialSeen(ctx, id)
}

// ArchivedTrial is a trial recipe a disliked first cooking archived
// (NEW-4), with the Undo token of the archive.
type ArchivedTrial struct {
	FoodID int64
	Name   string
	Undo   foodevent.Undo
}

// ArchiveDisliked archives every recipe among ids that was on trial on day,
// the disliked plate's date: a disliked first cooking hides it from planning
// the way "Not for me" does (NEW-4); the caller asks for the reason. A
// recipe whose trial had ended by then is untouched.
func (s *Service) ArchiveDisliked(ctx context.Context, ids []int64, day time.Time) ([]ArchivedTrial, error) {
	var out []ArchivedTrial
	for _, id := range ids {
		f, err := s.repo.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if f.Kind != KindComposed || !f.Trial || f.Archived {
			continue
		}
		if err := s.Derive(ctx, day, f); err != nil {
			return nil, err
		}
		if !f.OnTrial() {
			continue
		}
		_, undo, err := s.Archive(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, ArchivedTrial{FoodID: id, Name: f.Name, Undo: undo})
	}
	return out, nil
}
