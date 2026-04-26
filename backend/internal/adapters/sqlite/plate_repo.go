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
	db *sql.DB
	q  *sqlcgen.Queries
}

// NewPlateRepo creates a SQLite-backed plate repository.
func NewPlateRepo(db *sql.DB) *PlateRepo {
	return &PlateRepo{db: db, q: sqlcgen.New(db)}
}

// newPlateRepoTx returns a PlateRepo bound to a transaction. Used by TxRunner.
// db is left nil so Create() knows it's already inside an outer transaction.
func newPlateRepoTx(tx *sql.Tx) *PlateRepo {
	return &PlateRepo{db: nil, q: sqlcgen.New(tx)}
}

func (r *PlateRepo) Create(ctx context.Context, p *plate.Plate) error {
	if r.db == nil {
		// Bound to an outer transaction; reuse it.
		return r.createWith(ctx, r.q, p)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.createWith(ctx, sqlcgen.New(tx), p); err != nil {
		return err
	}
	return tx.Commit()
}

// upsertWeek returns the id of the week row for (year, isoWeek), creating it if absent.
// Uses r.q (the non-transactional query set).
func (r *PlateRepo) upsertWeek(ctx context.Context, year, isoWeek int) (int64, error) {
	return r.upsertWeekQ(ctx, r.q, year, isoWeek)
}

// upsertWeekQ is the transactional variant of upsertWeek.
func (r *PlateRepo) upsertWeekQ(ctx context.Context, q *sqlcgen.Queries, year, isoWeek int) (int64, error) {
	row, err := q.GetWeekByYearAndNumber(ctx, sqlcgen.GetWeekByYearAndNumberParams{
		Year:       int64(year),
		WeekNumber: int64(isoWeek),
	})
	if err == nil {
		return row.ID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	created, err := q.CreateWeek(ctx, sqlcgen.CreateWeekParams{
		Year:       int64(year),
		WeekNumber: int64(isoWeek),
	})
	if err != nil {
		return 0, err
	}
	return created.ID, nil
}

// legacyFromDate derives the legacy week_id and day values from a date.
// day is 0=Mon … 6=Sun (same encoding as the plates.day column).
func (r *PlateRepo) legacyFromDate(ctx context.Context, date time.Time) (weekID int64, day int, err error) {
	year, week := date.ISOWeek()
	day = (int(date.Weekday()) + 6) % 7
	weekID, err = r.upsertWeek(ctx, year, week)
	return
}

func (r *PlateRepo) createWith(ctx context.Context, q *sqlcgen.Queries, p *plate.Plate) error {
	var dateStr string
	if !p.Date.IsZero() {
		// Date-keyed path: derive week/day from the date itself.
		year, week := p.Date.ISOWeek()
		weekID, err := r.upsertWeekQ(ctx, q, year, week)
		if err != nil {
			return fmt.Errorf("upsert week for plate date: %w", err)
		}
		p.WeekID = weekID
		p.Day = (int(p.Date.Weekday()) + 6) % 7
		dateStr = p.Date.Format(dateLayout)
	} else {
		// Legacy path: compute the canonical date from the plate's week_id and day.
		weekRow, err := q.GetWeek(ctx, p.WeekID)
		if err != nil {
			return fmt.Errorf("look up week for plate date: %w", err)
		}
		monday := isoWeekStart(int(weekRow.Year), int(weekRow.WeekNumber))
		dateStr = monday.AddDate(0, 0, p.Day).Format(dateLayout)
	}

	row, err := q.CreatePlate(ctx, sqlcgen.CreatePlateParams{
		WeekID: p.WeekID,
		Day:    int64(p.Day),
		SlotID: p.SlotID,
		Note:   toNullString(p.Note),
		Date:   dateStr,
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: invalid week or slot reference", domain.ErrInvalidInput)
		}
		return err
	}
	mapPlateToDomain(&row, p)

	for i := range p.Components {
		pc := &p.Components[i]
		pc.PlateID = p.ID
		if pc.SortOrder == 0 && i > 0 {
			pc.SortOrder = i
		}
		pcRow, err := q.CreatePlateComponent(ctx, sqlcgen.CreatePlateComponentParams{
			PlateID:   pc.PlateID,
			FoodID:    pc.FoodID,
			Portions:  pc.Portions,
			SortOrder: int64(pc.SortOrder),
		})
		if err != nil {
			return err
		}
		mapPlateComponentToDomain(&pcRow, pc)
	}
	return nil
}

func (r *PlateRepo) Get(ctx context.Context, id int64) (*plate.Plate, error) {
	row, err := r.q.GetPlate(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: plate %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var p plate.Plate
	mapPlateToDomain(&row, &p)
	if err := r.loadPlateChildren(ctx, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PlateRepo) Update(ctx context.Context, p *plate.Plate) error {
	var dateStr string
	if !p.Date.IsZero() {
		// When a target date is set (date-keyed move), derive WeekID+Day from it
		// so the plate is moved to the correct ISO week, even across week boundaries.
		year, week := p.Date.ISOWeek()
		weekID, err := r.upsertWeek(ctx, year, week)
		if err != nil {
			return fmt.Errorf("upsert week for plate date: %w", err)
		}
		p.WeekID = weekID
		p.Day = (int(p.Date.Weekday()) + 6) % 7
		dateStr = p.Date.Format(dateLayout)
	} else {
		// Legacy path: compute the canonical date from the plate's week_id and day.
		weekRow, err := r.q.GetWeek(ctx, p.WeekID)
		if err != nil {
			return fmt.Errorf("look up week for plate date: %w", err)
		}
		monday := isoWeekStart(int(weekRow.Year), int(weekRow.WeekNumber))
		dateStr = monday.AddDate(0, 0, p.Day).Format(dateLayout)
	}

	row, err := r.q.UpdatePlate(ctx, sqlcgen.UpdatePlateParams{
		ID:     p.ID,
		WeekID: p.WeekID,
		Day:    int64(p.Day),
		SlotID: p.SlotID,
		Note:   toNullString(p.Note),
		Date:   dateStr,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: plate %d", domain.ErrNotFound, p.ID)
		}
		return err
	}
	mapPlateToDomain(&row, p)
	return nil
}

func (r *PlateRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.q.DeletePlate(ctx, id)
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

func (r *PlateRepo) ListByWeek(ctx context.Context, weekID int64) ([]plate.Plate, error) {
	plateRows, err := r.q.ListPlatesByWeek(ctx, weekID)
	if err != nil {
		return nil, err
	}
	if len(plateRows) == 0 {
		return []plate.Plate{}, nil
	}
	pcRows, err := r.q.ListPlateComponentsByWeek(ctx, weekID)
	if err != nil {
		return nil, err
	}
	pcByPlate := make(map[int64][]plate.PlateComponent, len(plateRows))
	for i := range pcRows {
		var pc plate.PlateComponent
		mapPlateComponentToDomain(&pcRows[i], &pc)
		pcByPlate[pc.PlateID] = append(pcByPlate[pc.PlateID], pc)
	}
	out := make([]plate.Plate, len(plateRows))
	for i := range plateRows {
		mapPlateToDomain(&plateRows[i], &out[i])
		out[i].Components = pcByPlate[out[i].ID]
		if out[i].Components == nil {
			out[i].Components = []plate.PlateComponent{}
		}
	}
	return out, nil
}

// ListByDateRange returns all plates whose date falls within [from, to] inclusive.
// Results are ordered by date, slot_id, id.
func (r *PlateRepo) ListByDateRange(ctx context.Context, from, to time.Time) ([]plate.Plate, error) {
	plateRows, err := r.q.ListPlatesByDateRange(ctx, sqlcgen.ListPlatesByDateRangeParams{
		FromDate: from.Format(dateLayout),
		ToDate:   to.Format(dateLayout),
	})
	if err != nil {
		return nil, err
	}
	if len(plateRows) == 0 {
		return []plate.Plate{}, nil
	}
	out := make([]plate.Plate, len(plateRows))
	for i := range plateRows {
		mapPlateToDomain(&plateRows[i], &out[i])
	}
	return out, nil
}

func (r *PlateRepo) CreateComponent(ctx context.Context, pc *plate.PlateComponent) error {
	row, err := r.q.CreatePlateComponent(ctx, sqlcgen.CreatePlateComponentParams{
		PlateID:   pc.PlateID,
		FoodID:    pc.FoodID,
		Portions:  pc.Portions,
		SortOrder: int64(pc.SortOrder),
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: invalid plate or food reference", domain.ErrInvalidInput)
		}
		return err
	}
	mapPlateComponentToDomain(&row, pc)
	return nil
}

func (r *PlateRepo) GetComponent(ctx context.Context, id int64) (*plate.PlateComponent, error) {
	row, err := r.q.GetPlateComponent(ctx, id)
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
	row, err := r.q.UpdatePlateComponent(ctx, sqlcgen.UpdatePlateComponentParams{
		ID:       pc.ID,
		FoodID:   pc.FoodID,
		Portions: pc.Portions,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: plate_component %d", domain.ErrNotFound, pc.ID)
		}
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: invalid food reference", domain.ErrInvalidInput)
		}
		return err
	}
	mapPlateComponentToDomain(&row, pc)
	return nil
}

func (r *PlateRepo) DeleteComponent(ctx context.Context, id int64) error {
	res, err := r.q.DeletePlateComponent(ctx, id)
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

func (r *PlateRepo) ListComponentsByPlate(ctx context.Context, plateID int64) ([]plate.PlateComponent, error) {
	rows, err := r.q.ListPlateComponentsByPlate(ctx, plateID)
	if err != nil {
		return nil, err
	}
	out := make([]plate.PlateComponent, len(rows))
	for i := range rows {
		mapPlateComponentToDomain(&rows[i], &out[i])
	}
	return out, nil
}

func (r *PlateRepo) CountUsingFood(ctx context.Context, foodID int64) (int64, error) {
	return r.q.CountPlatesUsingFood(ctx, foodID)
}

// SetSkipped toggles the prospective "skip this slot" marker on a plate.
// When enabling skip, any attached components are cleared atomically.
func (r *PlateRepo) SetSkipped(ctx context.Context, plateID int64, skipped bool, note *string) (*plate.Plate, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := sqlcgen.New(tx)
	s := int64(0)
	if skipped {
		s = 1
		// Empty plates' components before marking skipped.
		rows, err := qtx.ListPlateComponentsByPlate(ctx, plateID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if _, err := qtx.DeletePlateComponent(ctx, row.ID); err != nil {
				return nil, err
			}
		}
	}
	row, err := qtx.SetPlateSkipped(ctx, sqlcgen.SetPlateSkippedParams{
		ID:      plateID,
		Skipped: s,
		Note:    toNullString(note),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: plate %d", domain.ErrNotFound, plateID)
		}
		return nil, err
	}

	var p plate.Plate
	mapPlateToDomain(&row, &p)
	if skipped {
		p.Components = []plate.PlateComponent{}
	} else {
		pcRows, err := qtx.ListPlateComponentsByPlate(ctx, plateID)
		if err != nil {
			return nil, err
		}
		p.Components = make([]plate.PlateComponent, len(pcRows))
		for i := range pcRows {
			mapPlateComponentToDomain(&pcRows[i], &p.Components[i])
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &p, nil
}

// DeleteByWeek clears every plate in a week (used by fill-empty revert).
func (r *PlateRepo) DeleteByWeek(ctx context.Context, weekID int64) (int64, error) {
	res, err := r.q.DeletePlatesByWeek(ctx, weekID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *PlateRepo) CountUsingTimeSlot(ctx context.Context, slotID int64) (int64, error) {
	return r.q.CountPlatesUsingTimeSlot(ctx, slotID)
}

func (r *PlateRepo) loadPlateChildren(ctx context.Context, p *plate.Plate) error {
	rows, err := r.q.ListPlateComponentsByPlate(ctx, p.ID)
	if err != nil {
		return err
	}
	p.Components = make([]plate.PlateComponent, len(rows))
	for i := range rows {
		mapPlateComponentToDomain(&rows[i], &p.Components[i])
	}
	return nil
}

func mapPlateToDomain(row *sqlcgen.Plate, p *plate.Plate) {
	p.ID = row.ID
	p.WeekID = row.WeekID
	p.Day = int(row.Day)
	p.SlotID = row.SlotID
	p.Note = fromNullString(row.Note)
	p.Skipped = row.Skipped != 0
	p.CreatedAt, _ = time.Parse(timeLayout, row.CreatedAt)
	p.Date, _ = time.Parse(dateLayout, row.Date)
}

func mapPlateComponentToDomain(row *sqlcgen.PlateComponent, pc *plate.PlateComponent) {
	pc.ID = row.ID
	pc.PlateID = row.PlateID
	pc.FoodID = row.FoodID
	pc.Portions = row.Portions
	pc.SortOrder = int(row.SortOrder)
}

// isoWeekStart returns the Monday that begins ISO week `week` of `year`.
// Algorithm mirrors migrations.IsoWeekStart but avoids a package dependency.
func isoWeekStart(year, week int) time.Time {
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	daysFromMonday := int(jan4.Weekday()+6) % 7
	week1Monday := jan4.AddDate(0, 0, -daysFromMonday)
	return week1Monday.AddDate(0, 0, (week-1)*7)
}
