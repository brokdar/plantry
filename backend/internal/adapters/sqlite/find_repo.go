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
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// FindRepo implements finds.Store over recipe_suggestions rows with a state
// (00071).
type FindRepo struct {
	conn
}

// NewFindRepo creates a SQLite-backed find store.
func NewFindRepo(db *sql.DB) *FindRepo { return &FindRepo{conn: newConn(db)} }

var _ finds.Store = (*FindRepo)(nil)

// Create inserts a searching find.
func (r *FindRepo) Create(ctx context.Context, dish string, trigger finds.Trigger) (*finds.Find, error) {
	row, err := r.queriesFor(ctx).CreateFind(ctx, sqlcgen.CreateFindParams{
		Query: sql.NullString{String: dish, Valid: true}, FindTrigger: sql.NullString{String: string(trigger), Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("create find: %w", err)
	}
	return mapFind(row)
}

// Get returns the find or domain.ErrNotFound (also for a Discover row that
// is no find).
func (r *FindRepo) Get(ctx context.Context, id int64) (*finds.Find, error) {
	row, err := r.queriesFor(ctx).GetRecipeSuggestion(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !row.State.Valid) {
		return nil, fmt.Errorf("%w: find %d", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	return mapFind(row)
}

// Finish stores the outcome of a searching find.
func (r *FindRepo) Finish(ctx context.Context, id int64, o finds.Outcome) (*finds.Find, error) {
	js := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			return "null"
		}
		return string(b)
	}
	draft := "{}"
	if o.Draft != nil {
		draft = js(o.Draft)
	}
	var origin sql.NullString
	if o.Origin != nil {
		origin = sql.NullString{String: string(*o.Origin), Valid: true}
	}
	row, err := r.queriesFor(ctx).FinishFind(ctx, sqlcgen.FinishFindParams{
		State: sql.NullString{String: string(o.State), Valid: true}, SourceUrl: o.SourceURL, DraftJson: draft,
		SourcesJson: js(nonNil(o.Sources)), FlagsJson: js(nonNil(o.Flags)), Origin: origin,
		ChangesJson: js(nonNil(o.Changes)), GatesJson: js(o.Gates), StepsJson: js(nonNil(o.Steps)),
		QuestionsJson: js(nonNil(o.Questions)), AiRunID: toNullInt64(o.RunID), ID: id,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: searching find %d", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("finish find %d: %w", id, err)
	}
	return mapFind(row)
}

// List returns finds, optionally of one state, newest first.
func (r *FindRepo) List(ctx context.Context, state *finds.State, limit int) ([]finds.Find, error) {
	var st sql.NullString
	if state != nil {
		st = sql.NullString{String: string(*state), Valid: true}
	}
	rows, err := r.queriesFor(ctx).ListFinds(ctx, sqlcgen.ListFindsParams{State: st, Lim: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]finds.Find, 0, len(rows))
	for _, row := range rows {
		f, err := mapFind(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *f)
	}
	return out, nil
}

// Decide keeps or discards a ready or needs_check find.
func (r *FindRepo) Decide(ctx context.Context, id int64, state finds.State, reason *string, foodID *int64) (*finds.Find, error) {
	row, err := r.queriesFor(ctx).DecideFind(ctx, sqlcgen.DecideFindParams{
		State: sql.NullString{String: string(state), Valid: true}, Reason: toNullString(reason), FoodID: toNullInt64(foodID), ID: id,
	})
	if errors.Is(err, sql.ErrNoRows) {
		if _, gerr := r.Get(ctx, id); gerr != nil {
			return nil, gerr
		}
		return nil, fmt.Errorf("%w: find %d", domain.ErrDiscoveryDecided, id)
	}
	if err != nil {
		return nil, fmt.Errorf("decide find %d: %w", id, err)
	}
	return mapFind(row)
}

// Stranded returns the ids of searching finds without a queued or running job.
func (r *FindRepo) Stranded(ctx context.Context) ([]int64, error) {
	ids, err := r.queriesFor(ctx).ListStrandedFindIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list stranded finds: %w", err)
	}
	return ids, nil
}

// SourceURLs lists every food's and suggestion's source URL.
func (r *FindRepo) SourceURLs(ctx context.Context) ([]string, error) {
	return r.queriesFor(ctx).ListSourceURLs(ctx)
}

// PenaliseSource counts a "looks wrong" against the site.
func (r *FindRepo) PenaliseSource(ctx context.Context, site string) error {
	return r.queriesFor(ctx).PenaliseSource(ctx, site)
}

// SourcePenalties maps sites to their "looks wrong" counts.
func (r *FindRepo) SourcePenalties(ctx context.Context) (map[string]int, error) {
	rows, err := r.queriesFor(ctx).ListSourcePenalties(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.Site] = int(row.Count)
	}
	return out, nil
}

// MarkAsked records that a KIT-2/3 question was asked.
func (r *FindRepo) MarkAsked(ctx context.Context, question string) error {
	return r.queriesFor(ctx).MarkKitchenQuestionAsked(ctx, question)
}

// Asked lists the KIT-2/3 questions asked so far.
func (r *FindRepo) Asked(ctx context.Context) ([]string, error) {
	return r.queriesFor(ctx).ListKitchenQuestionsAsked(ctx)
}

func nonNil[T any](xs []T) []T {
	if xs == nil {
		return []T{}
	}
	return xs
}

func mapFind(row sqlcgen.RecipeSuggestion) (*finds.Find, error) {
	f := &finds.Find{
		ID: row.ID, Trigger: finds.Trigger(row.FindTrigger.String), State: finds.State(row.State.String),
		Dish: row.Query.String, SourceURL: row.SourceUrl, Reason: fromNullString(row.Reason),
		FoodID: fromNullInt64(row.FoodID), RunID: fromNullInt64(row.AiRunID),
	}
	if row.Origin.Valid {
		o := finds.Origin(row.Origin.String)
		f.Origin = &o
	}
	if row.DraftJson != "{}" {
		f.Draft = &importer.Draft{}
	}
	for _, p := range []struct {
		raw  string
		into any
	}{
		{row.SourcesJson, &f.Sources},
		{row.FlagsJson, &f.Flags},
		{row.ChangesJson, &f.Changes},
		{row.GatesJson, &f.Gates},
		{row.StepsJson, &f.Steps},
		{row.QuestionsJson, &f.Questions},
	} {
		if err := json.Unmarshal([]byte(p.raw), p.into); err != nil {
			return nil, fmt.Errorf("find %d: %w", row.ID, err)
		}
	}
	if f.Draft != nil {
		if err := json.Unmarshal([]byte(row.DraftJson), f.Draft); err != nil {
			return nil, fmt.Errorf("find %d draft: %w", row.ID, err)
		}
		// Finds never show a photo (FIND-14), migrated Discover rows included.
		f.Draft.ImageURL = ""
	}
	var err error
	if f.CreatedAt, err = time.Parse(timeLayout, row.CreatedAt); err != nil {
		return nil, err
	}
	if f.UpdatedAt, err = time.Parse(timeLayout, row.UpdatedAt); err != nil {
		return nil, err
	}
	if row.DecidedAt.Valid {
		t, err := time.Parse(timeLayout, row.DecidedAt.String)
		if err != nil {
			return nil, err
		}
		f.DecidedAt = &t
	}
	return f, nil
}
