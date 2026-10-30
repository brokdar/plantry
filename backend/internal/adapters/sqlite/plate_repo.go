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

const dateLayout = "2006-01-02"

// PlateRepo implements plate.Repository backed by SQLite.
type PlateRepo struct {
	conn
}

// NewPlateRepo creates a SQLite-backed plate repository.
func NewPlateRepo(db *sql.DB) *PlateRepo {
	return &PlateRepo{conn: conn{db: db}}
}

// Create inserts the plate and its components atomically.
func (r *PlateRepo) Create(ctx context.Context, p *plate.Plate) error {
	return r.inTx(ctx, func(ctx context.Context) error {
		q := r.queriesFor(ctx)
		row, err := q.CreatePlate(ctx, sqlcgen.CreatePlateParams{
			SlotID: p.SlotID,
			Note:   toNullString(p.Note),
			Date:   p.Date.Format(dateLayout),
			Eaters: p.Eaters,
		})
		if err != nil {
			if isForeignKeyViolation(err) {
				return fmt.Errorf("%w: invalid slot reference", domain.ErrInvalidInput)
			}
			if isUniqueViolation(err, "plates.date") {
				return fmt.Errorf("%w: %s slot %d", domain.ErrCellOccupied, p.DateString(), p.SlotID)
			}
			return err
		}
		mapPlateToDomain(&row, p)
		for i := range p.Components {
			pc := &p.Components[i]
			pc.PlateID = p.ID
			pcRow, err := q.CreatePlateComponent(ctx, plateComponentParams(pc))
			if err != nil {
				if isForeignKeyViolation(err) {
					return fmt.Errorf("%w: invalid food or session reference", domain.ErrInvalidInput)
				}
				return err
			}
			mapPlateComponentToDomain(&pcRow, pc)
		}
		return nil
	})
}

func (r *PlateRepo) Get(ctx context.Context, id int64) (*plate.Plate, error) {
	q := r.queriesFor(ctx)
	row, err := q.GetPlate(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: plate %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var p plate.Plate
	mapPlateToDomain(&row, &p)
	rows, err := q.ListPlateComponentsByPlate(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.Components = make([]plate.PlateComponent, len(rows))
	for i := range rows {
		mapPlateComponentToDomain(&rows[i].PlateComponent, &p.Components[i])
		setComponentFood(&p.Components[i], rows[i].FoodName, rows[i].FoodKind, rows[i].FoodRole)
	}
	fb, err := q.GetPlateFeedback(ctx, p.ID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return nil, err
	default:
		p.Feedback = mapFeedbackToPlate(&fb)
	}
	return &p, nil
}

func (r *PlateRepo) Update(ctx context.Context, p *plate.Plate) error {
	row, err := r.queriesFor(ctx).UpdatePlate(ctx, sqlcgen.UpdatePlateParams{
		ID:     p.ID,
		SlotID: p.SlotID,
		Note:   toNullString(p.Note),
		Date:   p.Date.Format(dateLayout),
		Eaters: p.Eaters,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: plate %d", domain.ErrNotFound, p.ID)
		}
		if isUniqueViolation(err, "plates.date") {
			return fmt.Errorf("%w: %s slot %d", domain.ErrCellOccupied, p.DateString(), p.SlotID)
		}
		return err
	}
	mapPlateToDomain(&row, p)
	return nil
}

// Touch bumps the plate's updated_at.
func (r *PlateRepo) Touch(ctx context.Context, id int64) error {
	return r.queriesFor(ctx).TouchPlate(ctx, id)
}

// GetIDByCell returns the id of the (date, slot) cell's plate.
func (r *PlateRepo) GetIDByCell(ctx context.Context, date time.Time, slotID int64) (int64, error) {
	id, err := r.queriesFor(ctx).GetPlateIDByCell(ctx, sqlcgen.GetPlateIDByCellParams{
		Date:   date.Format(dateLayout),
		SlotID: slotID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("%w: no plate at %s slot %d", domain.ErrNotFound, date.Format(dateLayout), slotID)
		}
		return 0, err
	}
	return id, nil
}

func (r *PlateRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.queriesFor(ctx).DeletePlate(ctx, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: plate %d", domain.ErrNotFound, id)
	}
	return nil
}

// ListByDateRange returns all plates whose date falls within [from, to]
// inclusive, with components and feedback, ordered by date, slot_id, id. It
// issues three queries regardless of the number of plates.
func (r *PlateRepo) ListByDateRange(ctx context.Context, from, to time.Time) ([]plate.Plate, error) {
	q := r.queriesFor(ctx)
	fromS, toS := from.Format(dateLayout), to.Format(dateLayout)
	plateRows, err := q.ListPlatesByDateRange(ctx, sqlcgen.ListPlatesByDateRangeParams{FromDate: fromS, ToDate: toS})
	if err != nil {
		return nil, err
	}
	out := make([]plate.Plate, len(plateRows))
	if len(plateRows) == 0 {
		return out, nil
	}
	index := make(map[int64]int, len(plateRows))
	for i := range plateRows {
		mapPlateToDomain(&plateRows[i], &out[i])
		out[i].Components = []plate.PlateComponent{}
		index[out[i].ID] = i
	}
	compRows, err := q.ListPlateComponentsByDateRange(ctx, sqlcgen.ListPlateComponentsByDateRangeParams{FromDate: fromS, ToDate: toS})
	if err != nil {
		return nil, err
	}
	for i := range compRows {
		var pc plate.PlateComponent
		mapPlateComponentToDomain(&compRows[i].PlateComponent, &pc)
		setComponentFood(&pc, compRows[i].FoodName, compRows[i].FoodKind, compRows[i].FoodRole)
		p := &out[index[pc.PlateID]]
		p.Components = append(p.Components, pc)
	}
	fbRows, err := q.ListPlateFeedbackByDateRange(ctx, sqlcgen.ListPlateFeedbackByDateRangeParams{FromDate: fromS, ToDate: toS})
	if err != nil {
		return nil, err
	}
	for i := range fbRows {
		out[index[fbRows[i].PlateID]].Feedback = mapFeedbackToPlate(&fbRows[i])
	}
	return out, nil
}

func (r *PlateRepo) CreateComponent(ctx context.Context, pc *plate.PlateComponent) error {
	row, err := r.queriesFor(ctx).CreatePlateComponent(ctx, plateComponentParams(pc))
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: invalid plate, food or session reference", domain.ErrInvalidInput)
		}
		return err
	}
	mapPlateComponentToDomain(&row, pc)
	return nil
}

func (r *PlateRepo) GetComponent(ctx context.Context, id int64) (*plate.PlateComponent, error) {
	row, err := r.queriesFor(ctx).GetPlateComponent(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: plate_component %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var pc plate.PlateComponent
	mapPlateComponentToDomain(&row, &pc)
	return &pc, nil
}

func (r *PlateRepo) UpdateComponent(ctx context.Context, pc *plate.PlateComponent) error {
	p := plateComponentParams(pc)
	row, err := r.queriesFor(ctx).UpdatePlateComponent(ctx, sqlcgen.UpdatePlateComponentParams{
		ID:            pc.ID,
		FoodID:        p.FoodID,
		Portions:      p.Portions,
		Amount:        p.Amount,
		Unit:          p.Unit,
		Grams:         p.Grams,
		GramsSource:   p.GramsSource,
		CookSessionID: p.CookSessionID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: plate_component %d", domain.ErrNotFound, pc.ID)
		}
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: invalid food or session reference", domain.ErrInvalidInput)
		}
		return err
	}
	mapPlateComponentToDomain(&row, pc)
	return nil
}

func (r *PlateRepo) DeleteComponent(ctx context.Context, id int64) error {
	res, err := r.queriesFor(ctx).DeletePlateComponent(ctx, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: plate_component %d", domain.ErrNotFound, id)
	}
	return nil
}

// RestoreFeedback writes the plate's feedback row with its original rated_at.
func (r *PlateRepo) RestoreFeedback(ctx context.Context, plateID int64, fb plate.Feedback) error {
	return r.queriesFor(ctx).RestorePlateFeedback(ctx, sqlcgen.RestorePlateFeedbackParams{
		PlateID: plateID,
		Status:  fb.Status,
		Note:    toNullString(fb.Note),
		RatedAt: fb.RatedAt.UTC().Format(timeLayout),
	})
}

func (r *PlateRepo) CountUsingFood(ctx context.Context, foodID int64) (int64, error) {
	return r.queriesFor(ctx).CountPlatesUsingFood(ctx, foodID)
}

// RecentUnitForFood returns the most-recently-stored unit for any plate
// component on the given food, or "" if none exists. Composed components
// have unit IS NULL and are skipped automatically by the WHERE clause.
func (r *PlateRepo) RecentUnitForFood(ctx context.Context, foodID int64) (string, error) {
	v, err := r.queriesFor(ctx).RecentUnitForFood(ctx, foodID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	if !v.Valid {
		return "", nil
	}
	return v.String, nil
}

func (r *PlateRepo) CountUsingTimeSlot(ctx context.Context, slotID int64) (int64, error) {
	return r.queriesFor(ctx).CountPlatesUsingTimeSlot(ctx, slotID)
}

func mapPlateToDomain(row *sqlcgen.Plate, p *plate.Plate) {
	p.ID = row.ID
	p.SlotID = row.SlotID
	p.Note = fromNullString(row.Note)
	p.Eaters = row.Eaters
	p.CreatedAt, _ = time.Parse(timeLayout, row.CreatedAt)
	p.UpdatedAt, _ = time.Parse(timeLayout, row.UpdatedAt)
	p.Date, _ = time.Parse(dateLayout, row.Date)
}

func mapFeedbackToPlate(row *sqlcgen.PlateFeedback) *plate.Feedback {
	fb := &plate.Feedback{Status: row.Status, Note: fromNullString(row.Note)}
	fb.RatedAt, _ = time.Parse(timeLayout, row.RatedAt) //nolint:errcheck // layout controlled by migration
	return fb
}

func mapPlateComponentToDomain(row *sqlcgen.PlateComponent, pc *plate.PlateComponent) {
	pc.ID = row.ID
	pc.PlateID = row.PlateID
	pc.FoodID = row.FoodID
	pc.SortOrder = int(row.SortOrder)
	pc.Portions = fromNullFloat(row.Portions)
	pc.Amount = fromNullFloat(row.Amount)
	pc.Unit = fromNullString(row.Unit)
	pc.Grams = fromNullFloat(row.Grams)
	pc.GramsSource = fromNullString(row.GramsSource)
	pc.CookSessionID = fromNullInt64(row.CookSessionID)
}

func setComponentFood(pc *plate.PlateComponent, name, kind string, role sql.NullString) {
	pc.FoodName = name
	pc.FoodKind = kind
	pc.FoodRole = fromNullString(role)
}

// plateComponentParams converts a domain PlateComponent into the
// sqlc-generated insert params, mapping pointer fields onto the right
// sql.Null* wrappers. Used by Create, CreateComponent and UpdateComponent so
// the null-handling is in one place.
func plateComponentParams(pc *plate.PlateComponent) sqlcgen.CreatePlateComponentParams {
	return sqlcgen.CreatePlateComponentParams{
		PlateID:       pc.PlateID,
		FoodID:        pc.FoodID,
		Portions:      toNullFloat(pc.Portions),
		Amount:        toNullFloat(pc.Amount),
		Unit:          toNullString(pc.Unit),
		Grams:         toNullFloat(pc.Grams),
		GramsSource:   toNullString(pc.GramsSource),
		CookSessionID: toNullInt64(pc.CookSessionID),
		SortOrder:     int64(pc.SortOrder),
	}
}
