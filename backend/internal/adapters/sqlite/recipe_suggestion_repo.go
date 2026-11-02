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

// Decide moves a pending suggestion to d.Status and recomputes its run's
// outcome (D11) in one transaction.
func (r *RecipeSuggestionRepo) Decide(ctx context.Context, id int64, d discovery.Decision) (*discovery.Suggestion, error) {
	var out *discovery.Suggestion
	err := r.inTx(ctx, func(ctx context.Context) error {
		q := r.queriesFor(ctx)
		var reason *string
		if d.Reason != nil {
			s := string(*d.Reason)
			reason = &s
		}
		row, err := q.DecideRecipeSuggestion(ctx, sqlcgen.DecideRecipeSuggestionParams{
			Status: string(d.Status), Reason: toNullString(reason), FoodID: toNullInt64(d.FoodID), ID: id,
		})
		if errors.Is(err, sql.ErrNoRows) {
			if _, gerr := r.Get(ctx, id); gerr != nil {
				return gerr
			}
			return fmt.Errorf("%w: recipe suggestion %d", domain.ErrDiscoveryDecided, id)
		}
		if err != nil {
			return fmt.Errorf("decide recipe suggestion %d: %w", id, err)
		}
		if row.AiRunID.Valid {
			if err := q.SetDiscoveryRunOutcome(ctx, row.AiRunID.Int64); err != nil {
				return fmt.Errorf("discovery run %d outcome: %w", row.AiRunID.Int64, err)
			}
		}
		out, err = mapRecipeSuggestion(row)
		return err
	})
	return out, err
}

// List returns suggestions, last decided (else newest) first.
func (r *RecipeSuggestionRepo) List(ctx context.Context, f discovery.ListFilter) ([]discovery.Suggestion, error) {
	var status *string
	if f.Status != nil {
		s := string(*f.Status)
		status = &s
	}
	rows, err := r.queriesFor(ctx).ListRecipeSuggestions(ctx, sqlcgen.ListRecipeSuggestionsParams{
		Status: toNullString(status), RunID: toNullInt64(f.RunID), Lim: int64(f.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list recipe suggestions: %w", err)
	}
	out := make([]discovery.Suggestion, 0, len(rows))
	for _, row := range rows {
		s, err := mapRecipeSuggestion(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, nil
}

// DeletePendingBefore deletes the pending suggestions created before the
// cutoff and returns their ids.
func (r *RecipeSuggestionRepo) DeletePendingBefore(ctx context.Context, before time.Time) ([]int64, error) {
	return r.queriesFor(ctx).DeletePendingSuggestionsBefore(ctx, before.UTC().Format(timeLayout))
}

// ShownOn returns the suggestion assigned to day, or nil.
func (r *RecipeSuggestionRepo) ShownOn(ctx context.Context, day string) (*discovery.Suggestion, error) {
	row, err := r.queriesFor(ctx).GetShownSuggestion(ctx, toNullString(&day))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("suggestion shown on %s: %w", day, err)
	}
	return mapRecipeSuggestion(row)
}

// ShowNext clears day from the suggestion holding it and assigns day to the
// oldest pending, never-shown suggestion with an id above after, in one
// transaction. It returns that suggestion, or nil when there is none (the
// day stays cleared).
func (r *RecipeSuggestionRepo) ShowNext(ctx context.Context, day string, after int64) (*discovery.Suggestion, error) {
	var out *discovery.Suggestion
	err := r.inTx(ctx, func(ctx context.Context) error {
		q := r.queriesFor(ctx)
		if err := q.ClearShownOn(ctx, toNullString(&day)); err != nil {
			return fmt.Errorf("clear shown on %s: %w", day, err)
		}
		row, err := q.ShowNextPending(ctx, sqlcgen.ShowNextPendingParams{Day: toNullString(&day), After: after})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("show next suggestion on %s: %w", day, err)
		}
		out, err = mapRecipeSuggestion(row)
		return err
	})
	return out, err
}

// DailyRunFailed reports whether a discovery run for day ended without
// storing a suggestion.
func (r *RecipeSuggestionRepo) DailyRunFailed(ctx context.Context, day string) (bool, error) {
	n, err := r.queriesFor(ctx).DailyRunFailed(ctx, day)
	return n != 0, err
}

// Run returns a discovery run's status, error key and notes, or
// domain.ErrNotFound.
func (r *RecipeSuggestionRepo) Run(ctx context.Context, id int64) (discovery.RunRow, error) {
	row, err := r.queriesFor(ctx).GetDiscoveryRun(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return discovery.RunRow{}, fmt.Errorf("%w: discovery run %d", domain.ErrNotFound, id)
	}
	if err != nil {
		return discovery.RunRow{}, err
	}
	return discovery.RunRow{
		ID: row.ID, Status: row.Status, ErrorKey: fromNullString(row.ErrorKey), Notes: row.Notes.String,
	}, nil
}
