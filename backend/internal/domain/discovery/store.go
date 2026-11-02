package discovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// Mode is the kind of discovery run a suggestion came from.
type Mode string

const (
	ModeDaily   Mode = "daily"
	ModeExplore Mode = "explore"
	ModeKnown   Mode = "known"
)

// Status is where a suggestion stands.
type Status string

const (
	StatusPending  Status = "pending"
	StatusAccepted Status = "accepted"
	StatusRejected Status = "rejected"
)

// Reason says why the user rejected a suggestion.
type Reason string

const (
	ReasonTooLong     Reason = "too_long"
	ReasonIngredients Reason = "ingredients"
	ReasonTaste       Reason = "taste"
)

// ImageCategory is the imagestore category of suggestion images
// (suggestions/<id>.jpg).
const ImageCategory = "suggestions"

// Suggestion is one stored recipe suggestion (D13).
type Suggestion struct {
	ID          int64
	Mode        Mode
	Query       *string
	SourceURL   string
	Draft       importer.Draft
	Annotations Annotations
	ImagePath   *string
	// Grounded: the page came from a live web search of the run, not from
	// the model's memory.
	Grounded  bool
	Status    Status
	Reason    *Reason
	RunID     *int64
	FoodID    *int64
	ShownOn   *string
	CreatedAt time.Time
	DecidedAt *time.Time
}

// Repository persists suggestions. Implemented by adapters/sqlite.
type Repository interface {
	URLIndex
	Create(ctx context.Context, s *Suggestion) error
	Get(ctx context.Context, id int64) (*Suggestion, error)
	SetImagePath(ctx context.Context, id int64, path *string) error
	// Decide moves a pending suggestion to d.Status and recomputes its run's
	// outcome (D11) in one transaction. A suggestion that is decided already
	// fails with domain.ErrDiscoveryDecided, a missing one with
	// domain.ErrNotFound.
	Decide(ctx context.Context, id int64, d Decision) (*Suggestion, error)
	// List returns suggestions, last decided (else newest) first.
	List(ctx context.Context, f ListFilter) ([]Suggestion, error)
	// DeletePendingBefore deletes the pending suggestions created before
	// the cutoff and returns their ids.
	DeletePendingBefore(ctx context.Context, before time.Time) ([]int64, error)
	// ShownOn returns the suggestion assigned to day (YYYY-MM-DD), or nil.
	ShownOn(ctx context.Context, day string) (*Suggestion, error)
	// ShowNext clears day from the suggestion holding it and assigns it to
	// the oldest pending, never-shown suggestion with an id above after; nil
	// when there is none, leaving day as it was (D12).
	ShowNext(ctx context.Context, day string, after int64) (*Suggestion, error)
	// ClearShown clears day from the suggestion holding it.
	ClearShown(ctx context.Context, day string) error
	// DailyRunFailed reports whether a discovery run for day ended without
	// storing a suggestion.
	DailyRunFailed(ctx context.Context, day string) (bool, error)
	// Run returns a discovery run's row or domain.ErrNotFound.
	Run(ctx context.Context, id int64) (RunRow, error)
}

// RunRow is the part of a discovery run's ai_runs row the API reports.
type RunRow struct {
	ID       int64
	Status   string
	ErrorKey *string
	Notes    string
}

// Decision is an accept (FoodID set) or a reject (Reason optional).
type Decision struct {
	Status Status
	Reason *Reason
	FoodID *int64
}

// ListFilter narrows List; zero fields do not filter. Limit is required.
type ListFilter struct {
	Status *Status
	RunID  *int64
	Limit  int
}

// Images stores suggestion images. Implemented by imagestore.Store.
type Images interface {
	SaveFromURL(ctx context.Context, url, category string, id int64) (string, error)
	Delete(category string, id int64) error
	Copy(fromCategory string, fromID int64, toCategory string, toID int64) (string, error)
}

// Store saves verified, annotated suggestions and records the user's
// decisions on them.
type Store struct {
	Repo Repository
	// Images is optional (no image path configured): suggestions are then
	// stored without images.
	Images Images
	// ImageGuard vets the image URL a fetched page names before it is
	// requested (any error skips the image); nil allows every URL.
	ImageGuard func(rawURL string) error
}

// NewSuggestion is what Save stores.
type NewSuggestion struct {
	Mode        Mode
	Query       *string
	Draft       *importer.Draft
	Annotations Annotations
	Grounded    bool
	RunID       *int64
	ShownOn     *string
}

// ErrDayTaken: another suggestion is shown on the day Save was given.
var ErrDayTaken = errors.New("discovery: day already has a suggestion")

// Save stores the suggestion as pending, then downloads its image to
// suggestions/<id>.jpg (D16). The draft's SourceURL is the row's source_url.
// An image that is missing, fails to download or whose path cannot be
// recorded leaves image_path NULL; the row stays stored.
func (s *Store) Save(ctx context.Context, in NewSuggestion) (*Suggestion, error) {
	sug := &Suggestion{
		Mode: in.Mode, Query: in.Query, SourceURL: in.Draft.SourceURL, Draft: *in.Draft,
		Annotations: in.Annotations, Grounded: in.Grounded, RunID: in.RunID, ShownOn: in.ShownOn,
	}
	if err := s.Repo.Create(ctx, sug); err != nil {
		return nil, fmt.Errorf("save suggestion: %w", err)
	}
	if path := s.saveImage(ctx, sug); path != "" {
		if err := s.Repo.SetImagePath(ctx, sug.ID, &path); err != nil {
			// The row is stored: keep it without an image rather than fail
			// a run whose suggestion is already visible.
			slog.WarnContext(ctx, "discovery.image.path_failed", "suggestion_id", sug.ID, "error", err)
			if err := s.Images.Delete(ImageCategory, sug.ID); err != nil {
				slog.WarnContext(ctx, "discovery.image.delete_failed", "suggestion_id", sug.ID, "error", err)
			}
			return sug, nil
		}
		sug.ImagePath = &path
	}
	return sug, nil
}

func (s *Store) saveImage(ctx context.Context, sug *Suggestion) string {
	url := sug.Draft.ImageURL
	if url == "" || s.Images == nil {
		return ""
	}
	if s.ImageGuard != nil {
		if err := s.ImageGuard(url); err != nil {
			slog.Warn("discovery.image.blocked", "suggestion_id", sug.ID, "host", host(url), "error", err)
			return ""
		}
		// The download re-checks redirect hops and the dialled address.
		ctx = importer.WithURLGuard(ctx, s.ImageGuard)
	}
	path, err := s.Images.SaveFromURL(ctx, url, ImageCategory, sug.ID)
	if err != nil {
		slog.Warn("discovery.image.failed", "suggestion_id", sug.ID, "host", host(url), "error", logCause(err))
		return ""
	}
	return path
}

// Accept marks the suggestion accepted as the saved food foodID and moves
// its run's outcome (D11). The image copy to the food is Service.Accept's.
func (s *Store) Accept(ctx context.Context, id, foodID int64) (*Suggestion, error) {
	return s.Repo.Decide(ctx, id, Decision{Status: StatusAccepted, FoodID: &foodID})
}

// Reject marks the suggestion rejected, moves its run's outcome (D11) and
// removes its image (D16). Once the decision is recorded it is the result:
// a failed image removal or path reset is logged, not returned.
func (s *Store) Reject(ctx context.Context, id int64, reason *Reason) (*Suggestion, error) {
	sug, err := s.Repo.Decide(ctx, id, Decision{Status: StatusRejected, Reason: reason})
	if err != nil {
		return nil, err
	}
	s.deleteImage(sug.ID)
	if err := s.Repo.SetImagePath(ctx, sug.ID, nil); err != nil {
		slog.WarnContext(ctx, "discovery.image.clear_failed", "suggestion_id", sug.ID, "error", err)
	}
	sug.ImagePath = nil
	return sug, nil
}

// Rejected returns the n most recently rejected suggestions.
func (s *Store) Rejected(ctx context.Context, n int) ([]Suggestion, error) {
	st := StatusRejected
	return s.Repo.List(ctx, ListFilter{Status: &st, Limit: n})
}

// DeleteStale deletes the pending suggestions created before the cutoff
// and their images (D12), and returns how many went.
func (s *Store) DeleteStale(ctx context.Context, before time.Time) (int, error) {
	ids, err := s.Repo.DeletePendingBefore(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("delete stale suggestions: %w", err)
	}
	for _, id := range ids {
		s.deleteImage(id)
	}
	return len(ids), nil
}

func (s *Store) deleteImage(id int64) {
	if s.Images == nil {
		return
	}
	if err := s.Images.Delete(ImageCategory, id); err != nil {
		slog.Warn("discovery.image.delete_failed", "suggestion_id", id, "error", err)
	}
}
