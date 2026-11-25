package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/foodevent"
)

// FoodEventRepo implements foodevent.Log backed by SQLite.
type FoodEventRepo struct {
	conn
}

// NewFoodEventRepo creates a SQLite-backed food event log.
func NewFoodEventRepo(db *sql.DB) *FoodEventRepo {
	return &FoodEventRepo{conn: newConn(db)}
}

var _ foodevent.Log = (*FoodEventRepo)(nil)

// Append writes one event, joining the unit of work in ctx.
func (r *FoodEventRepo) Append(ctx context.Context, e foodevent.Event) error {
	err := r.queriesFor(ctx).CreateFoodEvent(ctx, sqlcgen.CreateFoodEventParams{
		Kind: string(e.Kind), SubjectID: e.SubjectID, Action: string(e.Action), Actor: string(e.Actor),
		BeforeJson: nullJSON(e.Before), AfterJson: nullJSON(e.After), At: formatTime(e.At),
	})
	if err != nil {
		return fmt.Errorf("append food event: %w", err)
	}
	return nil
}

// LastID returns the highest event id, 0 when there is none.
func (r *FoodEventRepo) LastID(ctx context.Context) (int64, error) {
	return r.queriesFor(ctx).LastFoodEventID(ctx)
}

// Range returns the events with first <= id <= last, ascending.
func (r *FoodEventRepo) Range(ctx context.Context, first, last int64) ([]foodevent.Event, error) {
	rows, err := r.queriesFor(ctx).ListFoodEventsInRange(ctx, sqlcgen.ListFoodEventsInRangeParams{FirstID: first, LastID: last})
	if err != nil {
		return nil, fmt.Errorf("list food events in range: %w", err)
	}
	out := make([]foodevent.Event, 0, len(rows))
	for _, row := range rows {
		at, err := time.Parse(timeLayout, row.At)
		if err != nil {
			return nil, fmt.Errorf("food event %d time: %w", row.ID, err)
		}
		out = append(out, foodevent.Event{
			ID: row.ID, Kind: foodevent.Kind(row.Kind), SubjectID: row.SubjectID, Action: foodevent.Action(row.Action),
			Actor: actor.Kind(row.Actor), Before: rawJSON(row.BeforeJson), After: rawJSON(row.AfterJson), At: at,
		})
	}
	return out, nil
}
