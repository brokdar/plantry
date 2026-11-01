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
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// AIChangeRepo implements agent.ChangeStore: ai_changes (00037) and the
// plate events they span (Phase 4 D13).
type AIChangeRepo struct {
	conn
}

// NewAIChangeRepo creates a SQLite-backed change store.
func NewAIChangeRepo(db *sql.DB) *AIChangeRepo {
	return &AIChangeRepo{conn: conn{db: db}}
}

var _ agent.ChangeStore = (*AIChangeRepo)(nil)

// LastEventID returns the highest plate event id, 0 when there is none.
func (r *AIChangeRepo) LastEventID(ctx context.Context) (int64, error) {
	return r.queriesFor(ctx).LastPlateEventID(ctx)
}

// RunEvents returns the events of runIDs with afterID < id <= throughID.
func (r *AIChangeRepo) RunEvents(ctx context.Context, afterID, throughID int64, runIDs []int64) ([]plate.Event, error) {
	rows, err := r.queriesFor(ctx).ListPlateEventsOfRuns(ctx, sqlcgen.ListPlateEventsOfRunsParams{
		AfterID: afterID, ThroughID: throughID, RunIds: nullInt64s(runIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("list run events: %w", err)
	}
	return mapPlateEvents(rows)
}

// LaterEvents counts the events after afterID of the plates or sessions.
func (r *AIChangeRepo) LaterEvents(ctx context.Context, afterID int64, plateIDs, sessionIDs []int64) (int64, error) {
	return r.queriesFor(ctx).CountPlateEventsAfter(ctx, sqlcgen.CountPlateEventsAfterParams{
		AfterID: afterID, PlateIds: nullInt64s(plateIDs), SessionIds: nullInt64s(sessionIDs),
	})
}

// CreateChange inserts c and sets its ID and CreatedAt.
func (r *AIChangeRepo) CreateChange(ctx context.Context, c *agent.Change) error {
	p := sqlcgen.CreateAIChangeParams{
		RunID: c.RunID, StepSeq: int64(c.StepSeq), Tool: c.Tool, TextKey: c.TextKey,
		FirstEventID: toNullInt64(c.FirstEventID), LastEventID: toNullInt64(c.LastEventID),
	}
	var err error
	for _, f := range []struct {
		dst *string
		v   any
	}{
		{&p.ParamsJson, c.Params},
		{&p.RunIdsJson, c.RunIDs},
		{&p.DatesJson, c.Dates},
		{&p.PlateIdsJson, c.PlateIDs},
		{&p.SessionIdsJson, c.SessionIDs},
	} {
		if *f.dst, err = marshalString(f.v); err != nil {
			return err
		}
	}
	if c.CellBefore != nil {
		s, err := marshalString(c.CellBefore)
		if err != nil {
			return err
		}
		p.AvailabilityBeforeJson = sql.NullString{String: s, Valid: true}
	}
	row, err := r.queriesFor(ctx).CreateAIChange(ctx, p)
	if err != nil {
		return fmt.Errorf("create ai change: %w", err)
	}
	out, err := mapAIChange(row)
	if err != nil {
		return err
	}
	c.ID, c.CreatedAt = out.ID, out.CreatedAt
	return nil
}

// GetChange returns the change or an error wrapping domain.ErrNotFound.
func (r *AIChangeRepo) GetChange(ctx context.Context, id int64) (*agent.Change, error) {
	row, err := r.queriesFor(ctx).GetAIChange(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: ai change %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	return mapAIChange(row)
}

// ConversationChanges lists the changes of a conversation's runs by id.
func (r *AIChangeRepo) ConversationChanges(ctx context.Context, conversationID int64) ([]agent.Change, error) {
	rows, err := r.queriesFor(ctx).ListAIChangesByConversation(ctx, sql.NullInt64{Int64: conversationID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]agent.Change, len(rows))
	for i, row := range rows {
		c, err := mapAIChange(row)
		if err != nil {
			return nil, err
		}
		out[i] = *c
	}
	return out, nil
}

// MarkUndone sets undone_at; false when it was set already.
func (r *AIChangeRepo) MarkUndone(ctx context.Context, id int64, at time.Time) (bool, error) {
	res, err := r.queriesFor(ctx).MarkAIChangeUndone(ctx, sqlcgen.MarkAIChangeUndoneParams{
		UndoneAt: sql.NullString{String: formatTime(at), Valid: true}, ID: id,
	})
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func mapAIChange(row sqlcgen.AiChange) (*agent.Change, error) {
	c := &agent.Change{
		ID: row.ID, RunID: row.RunID, StepSeq: int(row.StepSeq), Tool: row.Tool, TextKey: row.TextKey,
		FirstEventID: fromNullInt64(row.FirstEventID), LastEventID: fromNullInt64(row.LastEventID),
	}
	for _, f := range []struct {
		src string
		dst any
	}{
		{row.ParamsJson, &c.Params},
		{row.RunIdsJson, &c.RunIDs},
		{row.DatesJson, &c.Dates},
		{row.PlateIdsJson, &c.PlateIDs},
		{row.SessionIdsJson, &c.SessionIDs},
	} {
		if err := json.Unmarshal([]byte(f.src), f.dst); err != nil {
			return nil, fmt.Errorf("ai change %d: %w", row.ID, err)
		}
	}
	if row.AvailabilityBeforeJson.Valid {
		c.CellBefore = &agent.CellBefore{}
		if err := json.Unmarshal([]byte(row.AvailabilityBeforeJson.String), c.CellBefore); err != nil {
			return nil, fmt.Errorf("ai change %d availability: %w", row.ID, err)
		}
	}
	var err error
	if c.CreatedAt, err = time.Parse(timeLayout, row.CreatedAt); err != nil {
		return nil, fmt.Errorf("ai change %d created_at: %w", row.ID, err)
	}
	if row.UndoneAt.Valid {
		t, err := time.Parse(timeLayout, row.UndoneAt.String)
		if err != nil {
			return nil, fmt.Errorf("ai change %d undone_at: %w", row.ID, err)
		}
		c.UndoneAt = &t
	}
	return c, nil
}

func marshalString(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal ai change: %w", err)
	}
	return string(b), nil
}

func nullInt64s(ids []int64) []sql.NullInt64 {
	out := make([]sql.NullInt64, len(ids))
	for i, id := range ids {
		out[i] = sql.NullInt64{Int64: id, Valid: true}
	}
	return out
}
