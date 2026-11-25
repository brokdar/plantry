package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/platefit"
	"github.com/jaltszeimer/plantry/backend/internal/domain/tips"
)

// ChefTipRepo implements tips.Store backed by SQLite.
type ChefTipRepo struct {
	conn
}

// NewChefTipRepo creates a SQLite-backed tip store.
func NewChefTipRepo(db *sql.DB) *ChefTipRepo {
	return &ChefTipRepo{conn: newConn(db)}
}

var _ tips.Store = (*ChefTipRepo)(nil)

func (r *ChefTipRepo) Create(ctx context.Context, t *tips.Tip) error {
	change, err := tips.MarshalOp(t.Op)
	if err != nil {
		return fmt.Errorf("encode tip change: %w", err)
	}
	id, err := r.queriesFor(ctx).CreateChefTip(ctx, sqlcgen.CreateChefTipParams{
		PlateID: toNullInt64(t.PlateID), RecipeID: toNullInt64(t.RecipeID),
		WindowStart: t.WindowStart.Format(time.DateOnly), Kind: string(t.Kind), Dimension: string(t.Dimension),
		Change: change, CreatedAt: formatTime(t.CreatedAt), UpdatedAt: formatTime(t.UpdatedAt),
	})
	if err != nil {
		return fmt.Errorf("create tip: %w", err)
	}
	t.ID = id
	return nil
}

func (r *ChefTipRepo) Get(ctx context.Context, id int64) (tips.Tip, error) {
	row, err := r.queriesFor(ctx).GetChefTip(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return tips.Tip{}, domain.ErrNotFound
	}
	if err != nil {
		return tips.Tip{}, fmt.Errorf("get tip %d: %w", id, err)
	}
	return tipOf(row)
}

func (r *ChefTipRepo) Window(ctx context.Context, start time.Time) ([]tips.Tip, error) {
	rows, err := r.queriesFor(ctx).ListChefTipsByWindow(ctx, start.Format(time.DateOnly))
	if err != nil {
		return nil, fmt.Errorf("list tips: %w", err)
	}
	out := make([]tips.Tip, 0, len(rows))
	for _, row := range rows {
		t, err := tipOf(row)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *ChefTipRepo) SetState(ctx context.Context, id int64, st tips.State, at time.Time) error {
	return r.queriesFor(ctx).SetChefTipState(ctx, sqlcgen.SetChefTipStateParams{State: string(st), UpdatedAt: formatTime(at), ID: id})
}

func (r *ChefTipRepo) DismissedSince(ctx context.Context, since time.Time) ([]tips.Dismissal, error) {
	rows, err := r.queriesFor(ctx).ListChefTipsDismissedSince(ctx, formatTime(since))
	if err != nil {
		return nil, fmt.Errorf("list dismissed tips: %w", err)
	}
	out := make([]tips.Dismissal, 0, len(rows))
	for _, row := range rows {
		at, err := time.Parse(timeLayout, row.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse tip time: %w", err)
		}
		out = append(out, tips.Dismissal{Kind: tips.Kind(row.Kind), At: at})
	}
	return out, nil
}

func (r *ChefTipRepo) CountByState(ctx context.Context, from, to time.Time) (map[tips.State]int, error) {
	rows, err := r.queriesFor(ctx).CountChefTipsCreatedBetween(ctx, sqlcgen.CountChefTipsCreatedBetweenParams{
		CreatedAt: formatTime(from), CreatedAt_2: formatTime(to),
	})
	if err != nil {
		return nil, fmt.Errorf("count tips: %w", err)
	}
	out := make(map[tips.State]int, len(rows))
	for _, row := range rows {
		out[tips.State(row.State)] = int(row.N)
	}
	return out, nil
}

func tipOf(row sqlcgen.ChefTip) (tips.Tip, error) {
	op, err := tips.UnmarshalOp(row.Change)
	if err != nil {
		return tips.Tip{}, fmt.Errorf("decode tip %d change: %w", row.ID, err)
	}
	start, err := time.Parse(time.DateOnly, row.WindowStart)
	if err != nil {
		return tips.Tip{}, fmt.Errorf("parse tip %d window: %w", row.ID, err)
	}
	created, err := time.Parse(timeLayout, row.CreatedAt)
	if err != nil {
		return tips.Tip{}, fmt.Errorf("parse tip %d time: %w", row.ID, err)
	}
	updated, err := time.Parse(timeLayout, row.UpdatedAt)
	if err != nil {
		return tips.Tip{}, fmt.Errorf("parse tip %d time: %w", row.ID, err)
	}
	return tips.Tip{
		ID: row.ID, PlateID: fromNullInt64(row.PlateID), RecipeID: fromNullInt64(row.RecipeID),
		WindowStart: start, Kind: tips.Kind(row.Kind), Dimension: platefit.Dimension(row.Dimension),
		Op: op, State: tips.State(row.State), CreatedAt: created, UpdatedAt: updated,
	}, nil
}
