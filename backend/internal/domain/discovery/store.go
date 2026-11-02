package discovery

import (
	"context"
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
}

// ImageSaver downloads and stores an image. Implemented by imagestore.Store.
type ImageSaver interface {
	SaveFromURL(ctx context.Context, url, category string, id int64) (string, error)
}

// Store saves verified, annotated suggestions.
type Store struct {
	Repo   Repository
	Images ImageSaver
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

// Save stores the suggestion as pending, then downloads its image to
// suggestions/<id>.jpg (D16). The draft's SourceURL is the row's source_url.
// An image that is missing or fails to download leaves image_path NULL; the
// row stays stored.
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
			return nil, fmt.Errorf("save suggestion image path: %w", err)
		}
		sug.ImagePath = &path
	}
	return sug, nil
}

func (s *Store) saveImage(ctx context.Context, sug *Suggestion) string {
	url := sug.Draft.ImageURL
	if url == "" {
		return ""
	}
	if s.ImageGuard != nil {
		if err := s.ImageGuard(url); err != nil {
			slog.Warn("discovery.image.blocked", "suggestion_id", sug.ID, "url", url, "error", err)
			return ""
		}
	}
	path, err := s.Images.SaveFromURL(ctx, url, ImageCategory, sug.ID)
	if err != nil {
		slog.Warn("discovery.image.failed", "suggestion_id", sug.ID, "url", url, "error", err)
		return ""
	}
	return path
}
