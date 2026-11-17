package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// PlanLightRepo stores the user's light requests per date (planning.LightStore).
type PlanLightRepo struct {
	conn
}

// NewPlanLightRepo creates a SQLite-backed light store.
func NewPlanLightRepo(db *sql.DB) *PlanLightRepo {
	return &PlanLightRepo{conn: conn{db: db}}
}

var _ planning.LightStore = (*PlanLightRepo)(nil)

// List returns the intents dated in [from, to].
func (r *PlanLightRepo) List(ctx context.Context, from, to time.Time) ([]planning.LightIntent, error) {
	rows, err := r.queriesFor(ctx).ListPlanLight(ctx, sqlcgen.ListPlanLightParams{
		Date: from.Format(dateLayout), Date_2: to.Format(dateLayout),
	})
	if err != nil {
		return nil, fmt.Errorf("list plan light: %w", err)
	}
	var out []planning.LightIntent
	for _, row := range rows {
		out = append(out, planning.LightIntent{Date: row.Date, Scope: row.Scope, Slot: row.Slot})
	}
	return out, nil
}

// Replace sets the intents of each of dates to the ones of intents dated
// then (a date without any is cleared) and prunes the dates before today.
// It joins the unit of work in ctx.
func (r *PlanLightRepo) Replace(ctx context.Context, today time.Time, dates []string, intents []planning.LightIntent) error {
	q := r.queriesFor(ctx)
	from := today.Format(dateLayout)
	if err := q.DeletePlanLightBefore(ctx, from); err != nil {
		return fmt.Errorf("prune plan light: %w", err)
	}
	for _, d := range dates {
		if err := q.DeletePlanLightDate(ctx, d); err != nil {
			return fmt.Errorf("clear plan light: %w", err)
		}
	}
	for _, l := range intents {
		if l.Date < from {
			continue
		}
		if err := q.CreatePlanLight(ctx, sqlcgen.CreatePlanLightParams{Date: l.Date, Scope: l.Scope, Slot: l.Slot}); err != nil {
			return fmt.Errorf("save plan light: %w", err)
		}
	}
	return nil
}
