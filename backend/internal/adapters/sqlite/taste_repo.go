package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
)

// TasteRepo implements taste.Store backed by SQLite.
type TasteRepo struct {
	conn
}

// NewTasteRepo creates a SQLite-backed taste store.
func NewTasteRepo(db *sql.DB) *TasteRepo {
	return &TasteRepo{conn: newConn(db)}
}

var _ taste.Store = (*TasteRepo)(nil)

// AddSignal appends one signal.
func (r *TasteRepo) AddSignal(ctx context.Context, s taste.Signal) error {
	err := r.queriesFor(ctx).CreateTasteSignal(ctx, sqlcgen.CreateTasteSignalParams{
		Kind: string(s.Kind), Level: string(s.Ref.Level), Key: s.Ref.Key, Weight: s.Weight, Share: s.Share,
		At: formatTime(s.At), FoodID: toNullInt64(s.FoodID), PlateID: toNullInt64(s.PlateID),
	})
	if err != nil {
		return fmt.Errorf("add taste signal: %w", err)
	}
	return nil
}

// HasSignal reports whether a signal of kind about food exists.
func (r *TasteRepo) HasSignal(ctx context.Context, kind taste.SignalKind, foodID int64) (bool, error) {
	n, err := r.queriesFor(ctx).HasTasteSignal(ctx, sqlcgen.HasTasteSignalParams{Kind: string(kind), FoodID: sql.NullInt64{Int64: foodID, Valid: true}})
	if err != nil {
		return false, fmt.Errorf("has taste signal: %w", err)
	}
	return n == 1, nil
}

// Items lists every item.
func (r *TasteRepo) Items(ctx context.Context) ([]taste.Item, error) {
	rows, err := r.queriesFor(ctx).ListTasteItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("list taste items: %w", err)
	}
	out := make([]taste.Item, 0, len(rows))
	for _, row := range rows {
		it, err := tasteItemFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, nil
}

// Item returns one item, domain.ErrNotFound when missing.
func (r *TasteRepo) Item(ctx context.Context, id int64) (taste.Item, error) {
	row, err := r.queriesFor(ctx).GetTasteItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return taste.Item{}, fmt.Errorf("%w: taste item %d", domain.ErrNotFound, id)
	}
	if err != nil {
		return taste.Item{}, fmt.Errorf("get taste item: %w", err)
	}
	return tasteItemFrom(row)
}

// ItemByRef returns the item of ref in scope and window.
func (r *TasteRepo) ItemByRef(ctx context.Context, ref taste.Ref, scope taste.Scope, window time.Time) (taste.Item, bool, error) {
	row, err := r.queriesFor(ctx).GetTasteItemByRef(ctx, sqlcgen.GetTasteItemByRefParams{
		Level: string(ref.Level), Key: ref.Key, Scope: string(scope), WindowStart: windowStartOf(window),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return taste.Item{}, false, nil
	}
	if err != nil {
		return taste.Item{}, false, fmt.Errorf("get taste item by ref: %w", err)
	}
	it, err := tasteItemFrom(row)
	return it, err == nil, err
}

// SaveItem inserts it (ID 0) or writes it under its ID.
func (r *TasteRepo) SaveItem(ctx context.Context, it *taste.Item) error {
	q := r.queriesFor(ctx)
	polarity := sql.NullString{String: string(it.Polarity), Valid: it.Polarity != ""}
	if it.ID == 0 {
		id, err := q.CreateTasteItem(ctx, sqlcgen.CreateTasteItemParams{
			Level: string(it.Level), Key: it.Key, Scope: string(it.Scope), WindowStart: windowStartOf(it.WindowStart),
			Source: string(it.Source), Polarity: polarity,
			PosEvidence: it.Evidence.Pos, PosN: it.Evidence.PosN, NegEvidence: it.Evidence.Neg, NegN: it.Evidence.NegN,
			CreatedAt: formatTime(it.CreatedAt), UpdatedAt: formatTime(it.UpdatedAt),
		})
		if err != nil {
			return fmt.Errorf("create taste item: %w", err)
		}
		it.ID = id
		return nil
	}
	err := q.PutTasteItem(ctx, sqlcgen.PutTasteItemParams{
		ID: it.ID, Level: string(it.Level), Key: it.Key, Scope: string(it.Scope), WindowStart: windowStartOf(it.WindowStart),
		Source: string(it.Source), Polarity: polarity,
		PosEvidence: it.Evidence.Pos, PosN: it.Evidence.PosN, NegEvidence: it.Evidence.Neg, NegN: it.Evidence.NegN,
		CreatedAt: formatTime(it.CreatedAt), UpdatedAt: formatTime(it.UpdatedAt),
	})
	if err != nil {
		return fmt.Errorf("put taste item: %w", err)
	}
	return nil
}

// DeleteItem removes an item; an absent one is not an error.
func (r *TasteRepo) DeleteItem(ctx context.Context, id int64) error {
	if err := r.queriesFor(ctx).DeleteTasteItem(ctx, id); err != nil {
		return fmt.Errorf("delete taste item: %w", err)
	}
	return nil
}

func windowStartOf(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(dateLayout)
}

func tasteItemFrom(row sqlcgen.TasteItem) (taste.Item, error) {
	it := taste.Item{
		ID: row.ID, Level: taste.Level(row.Level), Key: row.Key, Scope: taste.Scope(row.Scope), Source: taste.Source(row.Source),
		Polarity: taste.Polarity(row.Polarity.String),
		Evidence: taste.Evidence{Pos: row.PosEvidence, PosN: row.PosN, Neg: row.NegEvidence, NegN: row.NegN},
	}
	var errs []error
	var err error
	it.CreatedAt, err = time.Parse(timeLayout, row.CreatedAt)
	errs = append(errs, err)
	it.UpdatedAt, err = time.Parse(timeLayout, row.UpdatedAt)
	errs = append(errs, err)
	if row.WindowStart != "" {
		it.WindowStart, err = time.Parse(dateLayout, row.WindowStart)
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return taste.Item{}, fmt.Errorf("taste item %d: %w", row.ID, err)
	}
	return it, nil
}
