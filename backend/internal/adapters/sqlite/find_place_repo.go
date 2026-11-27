package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
)

// Place marks a ready find in_library as foodID, placed in the window that
// starts on windowStart (C3).
func (r *FindRepo) Place(ctx context.Context, id, foodID int64, windowStart time.Time) (*finds.Find, error) {
	row, err := r.queriesFor(ctx).PlaceFind(ctx, sqlcgen.PlaceFindParams{
		FoodID: sql.NullInt64{Int64: foodID, Valid: true}, PlacedOn: sql.NullString{String: windowStart.Format(time.DateOnly), Valid: true}, ID: id,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: find %d is not ready", domain.ErrDiscoveryDecided, id)
	}
	if err != nil {
		return nil, fmt.Errorf("place find %d: %w", id, err)
	}
	return mapFind(row)
}

// Unplace puts a placed find back to ready.
func (r *FindRepo) Unplace(ctx context.Context, id int64) (*finds.Find, error) {
	row, err := r.queriesFor(ctx).UnplaceFind(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: find %d is not placed", domain.ErrNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("unplace find %d: %w", id, err)
	}
	return mapFind(row)
}

// Pass records that a generation offered the ready find to the window that
// starts on windowStart and did not use it.
func (r *FindRepo) Pass(ctx context.Context, id int64, windowStart time.Time) error {
	err := r.queriesFor(ctx).PassFind(ctx, sqlcgen.PassFindParams{
		PassedOn: sql.NullString{String: windowStart.Format(time.DateOnly), Valid: true}, ID: id,
	})
	if err != nil {
		return fmt.Errorf("pass find %d: %w", id, err)
	}
	return nil
}

// Placed lists the finds placed in the window that starts on windowStart.
func (r *FindRepo) Placed(ctx context.Context, windowStart time.Time) ([]finds.Find, error) {
	rows, err := r.queriesFor(ctx).ListPlacedFinds(ctx, sql.NullString{String: windowStart.Format(time.DateOnly), Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list placed finds: %w", err)
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

// TouchSession keeps the planning session open until at least until.
func (r *FindRepo) TouchSession(ctx context.Context, until time.Time) error {
	if err := r.queriesFor(ctx).TouchPlanningSession(ctx, until.UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("touch planning session: %w", err)
	}
	return nil
}

// SessionUntil is the time the planning session is open until; zero when
// there never was one.
func (r *FindRepo) SessionUntil(ctx context.Context) (time.Time, error) {
	v, err := r.queriesFor(ctx).GetPlanningSession(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("planning session: %w", err)
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("planning session %q: %w", v, err)
	}
	return t, nil
}

// BlockSearch skips domains in the find search until until.
func (r *FindRepo) BlockSearch(ctx context.Context, domains []string, until time.Time) error {
	for _, d := range domains {
		if err := r.queriesFor(ctx).BlockSearchDomain(ctx, sqlcgen.BlockSearchDomainParams{Domain: d, BlockedUntil: until.UTC().Format(time.RFC3339)}); err != nil {
			return fmt.Errorf("block search domain %s: %w", d, err)
		}
	}
	return nil
}

// SearchBlocked lists the domains the find search skips at now.
func (r *FindRepo) SearchBlocked(ctx context.Context, now time.Time) ([]string, error) {
	out, err := r.queriesFor(ctx).ListSearchBlocked(ctx, now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("list search blocks: %w", err)
	}
	return out, nil
}
