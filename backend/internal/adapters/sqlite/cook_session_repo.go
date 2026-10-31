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
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: invalid food or slot reference", domain.ErrInvalidInput)
		}
		return err
	}
	mapCookSessionToDomain(&row, s)
	return nil
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
	mapCookSessionToDomain(&row, &s)
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
	mapCookSessionToDomain(&row, s)
	return nil
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
		out[i] = linkFromRow(row.PlateID, row.ComponentID, row.FoodID, row.Date, row.SlotID, row.Portions, row.Eaters)
	}
	return out, nil
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
		mapCookSessionToDomain(&rows[i], &sessions[i])
	}
	linkRows, err := q.ListCookSessionLinksInRange(ctx, sqlcgen.ListCookSessionLinksInRangeParams{FromDate: fromS, ToDate: toS})
	if err != nil {
		return nil, nil, err
	}
	links := make(map[int64][]plate.Link, len(sessions))
	for _, row := range linkRows {
		sid := row.CookSessionID.Int64
		links[sid] = append(links[sid], linkFromRow(row.PlateID, row.ComponentID, row.FoodID, row.Date, row.SlotID, row.Portions, row.Eaters))
	}
	return sessions, links, nil
}

// linkFromRow maps the columns both link queries share to a plate.Link.
func linkFromRow(plateID, componentID, foodID int64, date string, slotID int64, portions sql.NullFloat64, eaters float64) plate.Link {
	d, _ := time.Parse(dateLayout, date) //nolint:errcheck // layout controlled by the service
	return plate.Link{
		PlateID:     plateID,
		ComponentID: componentID,
		FoodID:      foodID,
		Cell:        plate.CellKey{Date: d, SlotID: slotID},
		PerEater:    portions.Float64,
		Eaters:      eaters,
	}
}

func mapCookSessionToDomain(row *sqlcgen.CookSession, s *plate.CookSession) {
	s.ID = row.ID
	s.FoodID = row.FoodID
	s.CookDate, _ = time.Parse(dateLayout, row.CookDate)
	s.CookSlotID = row.CookSlotID
	s.Portions = row.Portions
	s.FrozenPortions = row.FrozenPortions
	s.CookedAt = nil
	if row.CookedAt.Valid {
		if t, err := time.Parse(timeLayout, row.CookedAt.String); err == nil {
			t = t.UTC()
			s.CookedAt = &t
		}
	}
	s.Note = fromNullString(row.Note)
	s.CreatedAt, _ = time.Parse(timeLayout, row.CreatedAt)
	s.UpdatedAt, _ = time.Parse(timeLayout, row.UpdatedAt)
}

func toNullTime(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: t.UTC().Format(timeLayout), Valid: true}
}
