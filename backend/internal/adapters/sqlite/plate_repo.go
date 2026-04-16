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

func (r *PlateRepo) createWith(ctx context.Context, q *sqlcgen.Queries, p *plate.Plate) error {
	row, err := q.CreatePlate(ctx, sqlcgen.CreatePlateParams{
		WeekID: p.WeekID,
		Day:    int64(p.Day),
		SlotID: p.SlotID,
		Note:   toNullString(p.Note),
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
			PlateID:     pc.PlateID,
			ComponentID: pc.ComponentID,
			Portions:    pc.Portions,
			SortOrder:   int64(pc.SortOrder),
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
	row, err := r.q.UpdatePlate(ctx, sqlcgen.UpdatePlateParams{
		ID:     p.ID,
		Day:    int64(p.Day),
		SlotID: p.SlotID,
		Note:   toNullString(p.Note),
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

func (r *PlateRepo) CreateComponent(ctx context.Context, pc *plate.PlateComponent) error {
	row, err := r.q.CreatePlateComponent(ctx, sqlcgen.CreatePlateComponentParams{
		PlateID:     pc.PlateID,
		ComponentID: pc.ComponentID,
		Portions:    pc.Portions,
		SortOrder:   int64(pc.SortOrder),
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: invalid plate or component reference", domain.ErrInvalidInput)
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
		ID:          pc.ID,
		ComponentID: pc.ComponentID,
		Portions:    pc.Portions,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: plate_component %d", domain.ErrNotFound, pc.ID)
		}
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: invalid component reference", domain.ErrInvalidInput)
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

func (r *PlateRepo) CountUsingComponent(ctx context.Context, componentID int64) (int64, error) {
	return r.q.CountPlatesUsingComponent(ctx, componentID)
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
	p.CreatedAt, _ = time.Parse(timeLayout, row.CreatedAt)
}

func mapPlateComponentToDomain(row *sqlcgen.PlateComponent, pc *plate.PlateComponent) {
	pc.ID = row.ID
	pc.PlateID = row.PlateID
	pc.ComponentID = row.ComponentID
	pc.Portions = row.Portions
	pc.SortOrder = int(row.SortOrder)
}
