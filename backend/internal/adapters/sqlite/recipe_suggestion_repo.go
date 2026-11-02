package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
)

// RecipeSuggestionRepo implements discovery.Repository over recipe_suggestions
// (00039).
type RecipeSuggestionRepo struct {
	conn
}

// NewRecipeSuggestionRepo creates a SQLite-backed suggestion repository.
func NewRecipeSuggestionRepo(db *sql.DB) *RecipeSuggestionRepo {
	return &RecipeSuggestionRepo{conn: conn{db: db}}
}

var _ discovery.Repository = (*RecipeSuggestionRepo)(nil)

// Create inserts s as a pending suggestion and sets its ID, Status and CreatedAt.
func (r *RecipeSuggestionRepo) Create(ctx context.Context, s *discovery.Suggestion) error {
	draft, err := json.Marshal(s.Draft)
	if err != nil {
		return fmt.Errorf("marshal draft: %w", err)
	}
	ann, err := json.Marshal(s.Annotations)
	if err != nil {
		return fmt.Errorf("marshal annotations: %w", err)
	}
	grounded := int64(0)
	if s.Grounded {
		grounded = 1
	}
	row, err := r.queriesFor(ctx).CreateRecipeSuggestion(ctx, sqlcgen.CreateRecipeSuggestionParams{
		Mode: string(s.Mode), Query: toNullString(s.Query), SourceUrl: s.SourceURL,
		DraftJson: string(draft), AnnotationsJson: string(ann), Grounded: grounded,
		AiRunID: toNullInt64(s.RunID), ShownOn: toNullString(s.ShownOn),
	})
	if err != nil {
		return fmt.Errorf("create recipe suggestion: %w", err)
	}
	s.ID = row.ID
	s.Status = discovery.Status(row.Status)
	s.CreatedAt, err = time.Parse(timeLayout, row.CreatedAt)
	return err
}

// Get returns the suggestion or domain.ErrNotFound.
func (r *RecipeSuggestionRepo) Get(ctx context.Context, id int64) (*discovery.Suggestion, error) {
	row, err := r.queriesFor(ctx).GetRecipeSuggestion(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: recipe suggestion %d", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	return mapRecipeSuggestion(row)
}

// SetImagePath stores (or, with nil, clears) the suggestion's image path.
func (r *RecipeSuggestionRepo) SetImagePath(ctx context.Context, id int64, path *string) error {
	return r.queriesFor(ctx).SetRecipeSuggestionImage(ctx, sqlcgen.SetRecipeSuggestionImageParams{
		ImagePath: toNullString(path), ID: id,
	})
}

// SourceURLKnown reports whether a food or any suggestion has this URL.
func (r *RecipeSuggestionRepo) SourceURLKnown(ctx context.Context, url string) (bool, error) {
	n, err := r.queriesFor(ctx).SourceURLKnown(ctx, url)
	return n != 0, err
}

func mapRecipeSuggestion(row sqlcgen.RecipeSuggestion) (*discovery.Suggestion, error) {
	s := &discovery.Suggestion{
		ID: row.ID, Mode: discovery.Mode(row.Mode), Query: fromNullString(row.Query),
		SourceURL: row.SourceUrl, ImagePath: fromNullString(row.ImagePath),
		Grounded: row.Grounded != 0, Status: discovery.Status(row.Status),
		RunID: fromNullInt64(row.AiRunID), FoodID: fromNullInt64(row.FoodID), ShownOn: fromNullString(row.ShownOn),
	}
	if row.Reason.Valid {
		reason := discovery.Reason(row.Reason.String)
		s.Reason = &reason
	}
	var err error
	if s.CreatedAt, err = time.Parse(timeLayout, row.CreatedAt); err != nil {
		return nil, err
	}
	if row.DecidedAt.Valid {
		t, err := time.Parse(timeLayout, row.DecidedAt.String)
		if err != nil {
			return nil, err
		}
		s.DecidedAt = &t
	}
	if err := json.Unmarshal([]byte(row.DraftJson), &s.Draft); err != nil {
		return nil, fmt.Errorf("recipe suggestion %d draft: %w", row.ID, err)
	}
	if err := json.Unmarshal([]byte(row.AnnotationsJson), &s.Annotations); err != nil {
		return nil, fmt.Errorf("recipe suggestion %d annotations: %w", row.ID, err)
	}
	return s, nil
}
