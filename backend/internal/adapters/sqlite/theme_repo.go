package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/theme"
)

// ThemeRepo implements theme.Store backed by SQLite.
type ThemeRepo struct {
	conn
}

// NewThemeRepo creates a SQLite-backed theme store.
func NewThemeRepo(db *sql.DB) *ThemeRepo {
	return &ThemeRepo{conn: newConn(db)}
}

var _ theme.Store = (*ThemeRepo)(nil)

func day(t time.Time) string { return t.Format(time.DateOnly) }

func themeOf(row sqlcgen.WeekTheme) theme.Theme {
	start, _ := time.Parse(time.DateOnly, row.WindowStart)
	return theme.Theme{
		ID: row.ID, WindowStart: start, Kind: theme.Kind(row.Kind), Value: row.Value,
		Source: theme.Source(row.Source), WholeWeek: row.WholeWeek == 1,
	}
}

func (r *ThemeRepo) List(ctx context.Context, start time.Time) ([]theme.Theme, error) {
	rows, err := r.queriesFor(ctx).ListWeekThemes(ctx, day(start))
	if err != nil {
		return nil, fmt.Errorf("list week themes: %w", err)
	}
	out := make([]theme.Theme, len(rows))
	for i, row := range rows {
		out[i] = themeOf(row)
	}
	return out, nil
}

func (r *ThemeRepo) Add(ctx context.Context, t theme.Theme) (theme.Theme, error) {
	whole := int64(0)
	if t.WholeWeek {
		whole = 1
	}
	row, err := r.queriesFor(ctx).InsertWeekTheme(ctx, sqlcgen.InsertWeekThemeParams{
		WindowStart: day(t.WindowStart), Kind: string(t.Kind), Value: t.Value, Source: string(t.Source), WholeWeek: whole,
	})
	if err != nil {
		return theme.Theme{}, fmt.Errorf("insert week theme: %w", err)
	}
	return themeOf(row), nil
}

func (r *ThemeRepo) Delete(ctx context.Context, start time.Time, id int64) error {
	n, err := r.queriesFor(ctx).DeleteWeekTheme(ctx, sqlcgen.DeleteWeekThemeParams{ID: id, WindowStart: day(start)})
	if err != nil {
		return fmt.Errorf("delete week theme: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ThemeRepo) Suggestion(ctx context.Context, start time.Time) (*theme.Suggestion, error) {
	row, err := r.queriesFor(ctx).GetThemeSuggestion(ctx, day(start))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get theme suggestion: %w", err)
	}
	return &theme.Suggestion{Kind: theme.Kind(row.Kind), Value: row.Value, State: row.State}, nil
}

func (r *ThemeRepo) Suggest(ctx context.Context, start time.Time, s theme.Suggestion) error {
	return r.queriesFor(ctx).InsertThemeSuggestion(ctx, sqlcgen.InsertThemeSuggestionParams{
		WindowStart: day(start), Kind: string(s.Kind), Value: s.Value,
	})
}

func (r *ThemeRepo) Decide(ctx context.Context, start time.Time, state string, at time.Time) error {
	n, err := r.queriesFor(ctx).DecideThemeSuggestion(ctx, sqlcgen.DecideThemeSuggestionParams{
		State: state, DecidedAt: sql.NullString{String: at.UTC().Format(time.RFC3339), Valid: true}, WindowStart: day(start),
	})
	if err != nil {
		return fmt.Errorf("decide theme suggestion: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ThemeRepo) RecentDecisions(ctx context.Context) ([]theme.Decision, error) {
	rows, err := r.queriesFor(ctx).RecentThemeDecisions(ctx)
	if err != nil {
		return nil, fmt.Errorf("recent theme decisions: %w", err)
	}
	out := make([]theme.Decision, 0, len(rows))
	for _, row := range rows {
		at, err := time.Parse(time.RFC3339, row.DecidedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse theme decision time: %w", err)
		}
		out = append(out, theme.Decision{State: row.State, At: at})
	}
	return out, nil
}

func (r *ThemeRepo) RecentValues(ctx context.Context, since time.Time) ([]string, error) {
	vs, err := r.queriesFor(ctx).RecentThemeValues(ctx, day(since))
	if err != nil {
		return nil, fmt.Errorf("recent theme values: %w", err)
	}
	return vs, nil
}
