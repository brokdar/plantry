package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/actor"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// PlateEventRepo implements plate.EventLog backed by SQLite (D20).
type PlateEventRepo struct {
	conn
}

// NewPlateEventRepo creates a SQLite-backed plate event log.
func NewPlateEventRepo(db *sql.DB) *PlateEventRepo {
	return &PlateEventRepo{conn: newConn(db)}
}

var _ plate.EventLog = (*PlateEventRepo)(nil)

// Append writes one event, joining the unit of work in ctx.
func (r *PlateEventRepo) Append(ctx context.Context, e plate.Event) error {
	err := r.queriesFor(ctx).CreatePlateEvent(ctx, sqlcgen.CreatePlateEventParams{
		PlateID:       toNullInt64(e.PlateID),
		CookSessionID: toNullInt64(e.CookSessionID),
		Date:          e.Date.Format(dateLayout),
		SlotID:        e.SlotID,
		Action:        string(e.Action),
		Actor:         string(e.Actor),
		AiRunID:       toNullInt64(e.AIRunID),
		BeforeJson:    nullJSON(e.Before),
		AfterJson:     nullJSON(e.After),
		At:            formatTime(e.At),
	})
	if err != nil {
		return fmt.Errorf("append plate event: %w", err)
	}
	return nil
}

// List returns every event in insertion order. Phase 2 has no read API
// (D20); tests read the trail through it.
func (r *PlateEventRepo) List(ctx context.Context) ([]plate.Event, error) {
	rows, err := r.queriesFor(ctx).ListPlateEvents(ctx)
	if err != nil {
		return nil, fmt.Errorf("list plate events: %w", err)
	}
	return mapPlateEvents(rows)
}

// LastID returns the highest event id, 0 when there is none.
func (r *PlateEventRepo) LastID(ctx context.Context) (int64, error) {
	return r.queriesFor(ctx).LastPlateEventID(ctx)
}

// Range returns the events with first <= id <= last, ascending.
func (r *PlateEventRepo) Range(ctx context.Context, first, last int64) ([]plate.Event, error) {
	rows, err := r.queriesFor(ctx).ListPlateEventsInRange(ctx, sqlcgen.ListPlateEventsInRangeParams{FirstID: first, LastID: last})
	if err != nil {
		return nil, fmt.Errorf("list plate events in range: %w", err)
	}
	return mapPlateEvents(rows)
}

// CountAfter counts the events after afterID of the plates or sessions.
func (r *PlateEventRepo) CountAfter(ctx context.Context, afterID int64, plateIDs, sessionIDs []int64) (int64, error) {
	return r.queriesFor(ctx).CountPlateEventsAfter(ctx, sqlcgen.CountPlateEventsAfterParams{
		AfterID: afterID, PlateIds: nullInt64s(plateIDs), SessionIds: nullInt64s(sessionIDs),
	})
}

func mapPlateEvents(rows []sqlcgen.PlateEvent) ([]plate.Event, error) {
	out := make([]plate.Event, len(rows))
	for i, row := range rows {
		date, err := time.Parse(dateLayout, row.Date)
		if err != nil {
			return nil, fmt.Errorf("plate event %d date: %w", row.ID, err)
		}
		at, err := time.Parse(timeLayout, row.At)
		if err != nil {
			return nil, fmt.Errorf("plate event %d at: %w", row.ID, err)
		}
		out[i] = plate.Event{
			ID:            row.ID,
			PlateID:       fromNullInt64(row.PlateID),
			CookSessionID: fromNullInt64(row.CookSessionID),
			Date:          date,
			SlotID:        row.SlotID,
			Action:        plate.Action(row.Action),
			Actor:         actor.Kind(row.Actor),
			AIRunID:       fromNullInt64(row.AiRunID),
			Before:        rawJSON(row.BeforeJson),
			After:         rawJSON(row.AfterJson),
			At:            at.UTC(),
		}
	}
	return out, nil
}

func nullJSON(b json.RawMessage) sql.NullString {
	return sql.NullString{String: string(b), Valid: b != nil}
}

func rawJSON(ns sql.NullString) json.RawMessage {
	if !ns.Valid {
		return nil
	}
	return json.RawMessage(ns.String)
}
