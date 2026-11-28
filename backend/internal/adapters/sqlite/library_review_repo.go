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
	"github.com/jaltszeimer/plantry/backend/internal/domain/libreview"
)

// LibraryReviewRepo implements libreview.Store and the planning gap queue
// (finds.GapQueue) over library_review and library_gaps (00075).
type LibraryReviewRepo struct {
	conn
}

// NewLibraryReviewRepo creates a SQLite-backed library review store.
func NewLibraryReviewRepo(db *sql.DB) *LibraryReviewRepo {
	return &LibraryReviewRepo{conn: newConn(db)}
}

var (
	_ libreview.Store = (*LibraryReviewRepo)(nil)
	_ finds.GapQueue  = (*LibraryReviewRepo)(nil)
)

// State reads the stored review; the zero State when none was computed.
func (r *LibraryReviewRepo) State(ctx context.Context) (libreview.State, error) {
	row, err := r.queriesFor(ctx).GetLibraryReview(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return libreview.State{}, nil
	}
	if err != nil {
		return libreview.State{}, fmt.Errorf("get library review: %w", err)
	}
	st := libreview.State{Month: row.Month, CardDismissed: row.CardDismissed != 0}
	if err := json.Unmarshal([]byte(row.FindingsJson), &st.Findings); err != nil {
		return libreview.State{}, fmt.Errorf("library review findings: %w", err)
	}
	return st, nil
}

// SaveState stores the review.
func (r *LibraryReviewRepo) SaveState(ctx context.Context, s libreview.State) error {
	raw, err := json.Marshal(s.Findings)
	if err != nil {
		return fmt.Errorf("marshal library review findings: %w", err)
	}
	dismissed := int64(0)
	if s.CardDismissed {
		dismissed = 1
	}
	if err := r.queriesFor(ctx).SaveLibraryReview(ctx, sqlcgen.SaveLibraryReviewParams{Month: s.Month, FindingsJson: string(raw), CardDismissed: dismissed}); err != nil {
		return fmt.Errorf("save library review: %w", err)
	}
	return nil
}

// Recipes lists the active recipes with their ingredient leaves.
func (r *LibraryReviewRepo) Recipes(ctx context.Context) ([]libreview.Recipe, error) {
	q := r.queriesFor(ctx)
	rows, err := q.ListReviewRecipes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list review recipes: %w", err)
	}
	links, err := q.ListReviewIngredients(ctx)
	if err != nil {
		return nil, fmt.Errorf("list review ingredients: %w", err)
	}
	ingredients := map[int64][]int64{}
	for _, l := range links { // ordered by parent, child
		if ids := ingredients[l.ParentID]; len(ids) == 0 || ids[len(ids)-1] != l.ChildID {
			ingredients[l.ParentID] = append(ids, l.ChildID)
		}
	}
	out := make([]libreview.Recipe, 0, len(rows))
	for _, row := range rows {
		out = append(out, libreview.Recipe{
			ID: row.ID, Name: row.Name, Role: row.Role.String,
			Prep: int(row.PrepMinutes.Int64), Cook: int(row.CookMinutes.Int64), Steps: int(row.Steps),
			Ingredients: ingredients[row.ID], VariantGroup: row.VariantGroupID.Int64,
		})
	}
	return out, nil
}

// Replaced counts per recipe the plans the user took it off since.
func (r *LibraryReviewRepo) Replaced(ctx context.Context, since time.Time) (map[int64]int, error) {
	rows, err := r.queriesFor(ctx).ListReplacedCounts(ctx, formatTime(since))
	if err != nil {
		return nil, fmt.Errorf("list replaced counts: %w", err)
	}
	out := make(map[int64]int, len(rows))
	for _, row := range rows {
		out[row.FoodID.Int64] = int(row.Replaced)
	}
	return out, nil
}

// GroupVariants puts recipes a and b into one variant group: the group one
// of them has, else a new one named after a.
func (r *LibraryReviewRepo) GroupVariants(ctx context.Context, a, b int64) error {
	q := r.queriesFor(ctx)
	fa, err := q.GetFood(ctx, a)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: food %d", domain.ErrNotFound, a)
	}
	if err != nil {
		return fmt.Errorf("get food %d: %w", a, err)
	}
	fb, err := q.GetFood(ctx, b)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: food %d", domain.ErrNotFound, b)
	}
	if err != nil {
		return fmt.Errorf("get food %d: %w", b, err)
	}
	group := fa.VariantGroupID
	if !group.Valid {
		group = fb.VariantGroupID
	}
	if !group.Valid {
		g, err := q.CreateVariantGroup(ctx, fa.Name)
		if err != nil {
			return fmt.Errorf("create variant group: %w", err)
		}
		group = sql.NullInt64{Int64: g.ID, Valid: true}
	}
	for _, id := range []int64{a, b} {
		if err := q.SetFoodVariantGroup(ctx, sqlcgen.SetFoodVariantGroupParams{GroupID: group, ID: id}); err != nil {
			return fmt.Errorf("set variant group of food %d: %w", id, err)
		}
	}
	return nil
}

// QueueGap queues a library gap for planning (LIB-3).
func (r *LibraryReviewRepo) QueueGap(ctx context.Context, g libreview.Gap, at time.Time) error {
	if err := r.queriesFor(ctx).AddLibraryGap(ctx, sqlcgen.AddLibraryGapParams{Kind: g.Kind, Dish: g.Dish, CreatedAt: formatTime(at)}); err != nil {
		return fmt.Errorf("queue library gap %s: %w", g.Kind, err)
	}
	return nil
}

// Gaps lists the queued gaps, oldest first.
func (r *LibraryReviewRepo) Gaps(ctx context.Context) ([]finds.QueuedGap, error) {
	rows, err := r.queriesFor(ctx).ListLibraryGaps(ctx)
	if err != nil {
		return nil, fmt.Errorf("list library gaps: %w", err)
	}
	out := make([]finds.QueuedGap, 0, len(rows))
	for _, row := range rows {
		out = append(out, finds.QueuedGap{Kind: row.Kind, Dish: row.Dish, FindID: row.FindID.Int64, Offered: row.Offered != 0})
	}
	return out, nil
}

// StartGap records the find started for a gap and that this session took one.
func (r *LibraryReviewRepo) StartGap(ctx context.Context, kind string, findID int64) error {
	err := r.queriesFor(ctx).StartLibraryGap(ctx, sqlcgen.StartLibraryGapParams{FindID: sql.NullInt64{Int64: findID, Valid: true}, Kind: kind})
	if err != nil {
		return fmt.Errorf("start library gap %s: %w", kind, err)
	}
	return nil
}

// FreeGap forgets the find of a gap so a later session tries again.
func (r *LibraryReviewRepo) FreeGap(ctx context.Context, kind string) error {
	if err := r.queriesFor(ctx).FreeLibraryGap(ctx, kind); err != nil {
		return fmt.Errorf("free library gap %s: %w", kind, err)
	}
	return nil
}

// DeleteGap removes a gap that was filled.
func (r *LibraryReviewRepo) DeleteGap(ctx context.Context, kind string) error {
	if err := r.queriesFor(ctx).DeleteLibraryGap(ctx, kind); err != nil {
		return fmt.Errorf("delete library gap %s: %w", kind, err)
	}
	return nil
}

// NewGapSession lets one queued gap start again (a new planning session).
func (r *LibraryReviewRepo) NewGapSession(ctx context.Context) error {
	if err := r.queriesFor(ctx).NewGapSession(ctx); err != nil {
		return fmt.Errorf("new gap session: %w", err)
	}
	return nil
}
