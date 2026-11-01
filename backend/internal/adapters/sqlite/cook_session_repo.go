package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// CookSessionRepo implements plate.SessionRepository backed by SQLite.
type CookSessionRepo struct {
	conn
}

// NewCookSessionRepo creates a SQLite-backed cook session repository.
func NewCookSessionRepo(db *sql.DB) *CookSessionRepo {
	return &CookSessionRepo{conn: conn{db: db}}
}

func (r *CookSessionRepo) CreateSession(ctx context.Context, s *plate.CookSession) error {
	row, err := r.queriesFor(ctx).CreateCookSession(ctx, sqlcgen.CreateCookSessionParams{
		FoodID:         s.FoodID,
		CookDate:       s.CookDate.Format(dateLayout),
		CookSlotID:     s.CookSlotID,
		Portions:       s.Portions,
		FrozenPortions: s.FrozenPortions,
		CookedAt:       toNullTime(s.CookedAt),
		Note:           toNullString(s.Note),
		State:          string(s.State),
		AiRunID:        toNullInt64(s.AIRunID),
		Reason:         toNullString(s.Reason),
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: invalid food, slot or ai run reference", domain.ErrInvalidInput)
		}
		return err
	}
	return mapCookSessionToDomain(&row, s)
}

// UpsertSession writes s with its own id as it is: inserts it when the row
// is gone, else overwrites every column (undo, Phase 4 D13).
func (r *CookSessionRepo) UpsertSession(ctx context.Context, s plate.CookSession) error {
	err := r.queriesFor(ctx).UpsertCookSession(ctx, sqlcgen.UpsertCookSessionParams{
		ID:             s.ID,
		FoodID:         s.FoodID,
		CookDate:       s.CookDate.Format(dateLayout),
		CookSlotID:     s.CookSlotID,
		Portions:       s.Portions,
		FrozenPortions: s.FrozenPortions,
		CookedAt:       toNullTime(s.CookedAt),
		Note:           toNullString(s.Note),
		State:          string(s.State),
		AiRunID:        toNullInt64(s.AIRunID),
		Reason:         toNullString(s.Reason),
	})
	if err != nil && isForeignKeyViolation(err) {
		return fmt.Errorf("%w: invalid food, slot or ai run reference", domain.ErrInvalidInput)
	}
	return err
}

func (r *CookSessionRepo) GetSession(ctx context.Context, id int64) (*plate.CookSession, error) {
	row, err := r.queriesFor(ctx).GetCookSession(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: cook session %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var s plate.CookSession
	if err := mapCookSessionToDomain(&row, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *CookSessionRepo) UpdateSession(ctx context.Context, s *plate.CookSession) error {
	row, err := r.queriesFor(ctx).UpdateCookSession(ctx, sqlcgen.UpdateCookSessionParams{
		ID:             s.ID,
		CookDate:       s.CookDate.Format(dateLayout),
		CookSlotID:     s.CookSlotID,
		Portions:       s.Portions,
		FrozenPortions: s.FrozenPortions,
		CookedAt:       toNullTime(s.CookedAt),
		Note:           toNullString(s.Note),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: cook session %d", domain.ErrNotFound, s.ID)
		}
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: invalid slot reference", domain.ErrInvalidInput)
		}
		return err
	}
	return mapCookSessionToDomain(&row, s)
}

func (r *CookSessionRepo) DeleteSession(ctx context.Context, id int64) error {
	res, err := r.queriesFor(ctx).DeleteCookSession(ctx, id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: cook session %d still has components", domain.ErrInUse, id)
		}
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: cook session %d", domain.ErrNotFound, id)
	}
	return nil
}

func (r *CookSessionRepo) ListLinks(ctx context.Context, sessionID int64) ([]plate.Link, error) {
	rows, err := r.queriesFor(ctx).ListCookSessionLinks(ctx, sql.NullInt64{Int64: sessionID, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]plate.Link, len(rows))
	for i, row := range rows {
		if out[i], err = linkFromRow(row.PlateID, row.ComponentID, row.FoodID, row.Date, row.SlotID, row.Portions, row.Eaters); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// PlateIDsOfSessions returns the plates with a component eating from one of
// the sessions, ascending.
func (r *CookSessionRepo) PlateIDsOfSessions(ctx context.Context, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return []int64{}, nil
	}
	args := make([]sql.NullInt64, len(ids))
	for i, id := range ids {
		args[i] = sql.NullInt64{Int64: id, Valid: true}
	}
	return r.queriesFor(ctx).ListPlateIDsBySessionIDs(ctx, args)
}

// ListSessionsByIDs returns the sessions of ids that exist, ordered by id.
func (r *CookSessionRepo) ListSessionsByIDs(ctx context.Context, ids []int64) ([]plate.CookSession, error) {
	if len(ids) == 0 {
		return []plate.CookSession{}, nil
	}
	rows, err := r.queriesFor(ctx).ListCookSessionsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]plate.CookSession, len(rows))
	for i := range rows {
		if err := mapCookSessionToDomain(&rows[i], &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// SetSessionsState writes the state of the sessions of ids.
func (r *CookSessionRepo) SetSessionsState(ctx context.Context, ids []int64, st plate.State) error {
	if len(ids) == 0 {
		return nil
	}
	return r.queriesFor(ctx).SetSessionsState(ctx, sqlcgen.SetSessionsStateParams{State: string(st), Ids: ids})
}

// ListInRange returns the sessions cooking in [from, to] or linked to a plate
// dated in it, with every link of each keyed by session id: two queries for
// the whole range.
func (r *CookSessionRepo) ListInRange(ctx context.Context, from, to time.Time) ([]plate.CookSession, map[int64][]plate.Link, error) {
	q := r.queriesFor(ctx)
	fromS, toS := from.Format(dateLayout), to.Format(dateLayout)
	rows, err := q.ListCookSessionsInRange(ctx, sqlcgen.ListCookSessionsInRangeParams{FromDate: fromS, ToDate: toS})
	if err != nil {
		return nil, nil, err
	}
	sessions := make([]plate.CookSession, len(rows))
	for i := range rows {
		if err := mapCookSessionToDomain(&rows[i], &sessions[i]); err != nil {
			return nil, nil, err
		}
	}
	linkRows, err := q.ListCookSessionLinksInRange(ctx, sqlcgen.ListCookSessionLinksInRangeParams{FromDate: fromS, ToDate: toS})
	if err != nil {
		return nil, nil, err
	}
	links := make(map[int64][]plate.Link, len(sessions))
	for _, row := range linkRows {
		sid := row.CookSessionID.Int64
		l, err := linkFromRow(row.PlateID, row.ComponentID, row.FoodID, row.Date, row.SlotID, row.Portions, row.Eaters)
		if err != nil {
			return nil, nil, err
		}
		links[sid] = append(links[sid], l)
	}
	return sessions, links, nil
}

// linkFromRow maps the columns both link queries share to a plate.Link.
func linkFromRow(plateID, componentID, foodID int64, date string, slotID int64, portions sql.NullFloat64, eaters float64) (plate.Link, error) {
	d, err := time.Parse(dateLayout, date)
	if err != nil {
		return plate.Link{}, fmt.Errorf("plate %d date: %w", plateID, err)
	}
	return plate.Link{
		PlateID:     plateID,
		ComponentID: componentID,
		FoodID:      foodID,
		Cell:        plate.CellKey{Date: d, SlotID: slotID},
		PerEater:    portions.Float64,
		Eaters:      eaters,
	}, nil
}

func mapCookSessionToDomain(row *sqlcgen.CookSession, s *plate.CookSession) error {
	s.ID = row.ID
	s.FoodID = row.FoodID
	var err error
	if s.CookDate, err = time.Parse(dateLayout, row.CookDate); err != nil {
		return fmt.Errorf("cook session %d cook_date: %w", row.ID, err)
	}
	s.CookSlotID = row.CookSlotID
	s.Portions = row.Portions
	s.FrozenPortions = row.FrozenPortions
	s.CookedAt = nil
	if row.CookedAt.Valid {
		t, err := time.Parse(timeLayout, row.CookedAt.String)
		if err != nil {
			return fmt.Errorf("cook session %d cooked_at: %w", row.ID, err)
		}
		t = t.UTC()
		s.CookedAt = &t
	}
	s.Note = fromNullString(row.Note)
	s.State = plate.State(row.State)
	s.AIRunID = fromNullInt64(row.AiRunID)
	s.Reason = fromNullString(row.Reason)
	if s.CreatedAt, err = time.Parse(timeLayout, row.CreatedAt); err != nil {
		return fmt.Errorf("cook session %d created_at: %w", row.ID, err)
	}
	if s.UpdatedAt, err = time.Parse(timeLayout, row.UpdatedAt); err != nil {
		return fmt.Errorf("cook session %d updated_at: %w", row.ID, err)
	}
	return nil
}

func toNullTime(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: t.UTC().Format(timeLayout), Valid: true}
}
