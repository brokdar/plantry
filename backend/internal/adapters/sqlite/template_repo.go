package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/template"
)

// TemplateRepo implements template.Repository backed by SQLite.
type TemplateRepo struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

// NewTemplateRepo creates a SQLite-backed template repository.
func NewTemplateRepo(db *sql.DB) *TemplateRepo {
	return &TemplateRepo{db: db, q: sqlcgen.New(db)}
}

// newTemplateRepoTx returns a TemplateRepo bound to an outer transaction.
func newTemplateRepoTx(tx *sql.Tx) *TemplateRepo {
	return &TemplateRepo{db: nil, q: sqlcgen.New(tx)}
}

func (r *TemplateRepo) Create(ctx context.Context, t *template.Template) error {
	if r.db == nil {
		return r.createWith(ctx, r.q, t)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.createWith(ctx, sqlcgen.New(tx), t); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *TemplateRepo) createWith(ctx context.Context, q *sqlcgen.Queries, t *template.Template) error {
	row, err := q.CreateTemplate(ctx, t.Name)
	if err != nil {
		return err
	}
	mapTemplateToDomain(&row, t)
	for i := range t.Components {
		tc := &t.Components[i]
		tc.TemplateID = t.ID
		if tc.SortOrder == 0 && i > 0 {
			tc.SortOrder = i
		}
		tcRow, err := q.CreateTemplateComponent(ctx, sqlcgen.CreateTemplateComponentParams{
			TemplateID: tc.TemplateID,
			FoodID:     tc.FoodID,
			Portions:   tc.Portions,
			SortOrder:  int64(tc.SortOrder),
			DayOffset:  int64(tc.DayOffset),
		})
		if err != nil {
			if isForeignKeyViolation(err) {
				return fmt.Errorf("%w: invalid food reference", domain.ErrInvalidInput)
			}
			return err
		}
		mapTemplateComponentToDomain(&tcRow, tc)
	}
	return nil
}

func (r *TemplateRepo) Get(ctx context.Context, id int64) (*template.Template, error) {
	row, err := r.q.GetTemplate(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: template %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var t template.Template
	mapTemplateToDomain(&row, &t)
	if err := r.loadTemplateChildren(ctx, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TemplateRepo) UpdateName(ctx context.Context, id int64, name string) (*template.Template, error) {
	row, err := r.q.UpdateTemplateName(ctx, sqlcgen.UpdateTemplateNameParams{Name: name, ID: id})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: template %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var t template.Template
	mapTemplateToDomain(&row, &t)
	if err := r.loadTemplateChildren(ctx, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *TemplateRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.q.DeleteTemplate(ctx, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: template %d", domain.ErrNotFound, id)
	}
	return nil
}

func (r *TemplateRepo) List(ctx context.Context) ([]template.Template, error) {
	rows, err := r.q.ListTemplates(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []template.Template{}, nil
	}
	out := make([]template.Template, len(rows))
	for i := range rows {
		mapTemplateToDomain(&rows[i], &out[i])
		if err := r.loadTemplateChildren(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (r *TemplateRepo) ReplaceComponents(ctx context.Context, templateID int64, comps []template.TemplateComponent) error {
	if r.db == nil {
		return r.replaceWith(ctx, r.q, templateID, comps)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.replaceWith(ctx, sqlcgen.New(tx), templateID, comps); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *TemplateRepo) replaceWith(ctx context.Context, q *sqlcgen.Queries, templateID int64, comps []template.TemplateComponent) error {
	if _, err := q.DeleteTemplateComponentsByTemplate(ctx, templateID); err != nil {
		return err
	}
	for i, tc := range comps {
		if _, err := q.CreateTemplateComponent(ctx, sqlcgen.CreateTemplateComponentParams{
			TemplateID: templateID,
			FoodID:     tc.FoodID,
			Portions:   tc.Portions,
			SortOrder:  int64(i),
			DayOffset:  int64(tc.DayOffset),
		}); err != nil {
			if isForeignKeyViolation(err) {
				return fmt.Errorf("%w: invalid food reference", domain.ErrInvalidInput)
			}
			return err
		}
	}
	return nil
}

func (r *TemplateRepo) ListComponentsByTemplate(ctx context.Context, templateID int64) ([]template.TemplateComponent, error) {
	rows, err := r.q.ListTemplateComponentsByTemplate(ctx, templateID)
	if err != nil {
		return nil, err
	}
	out := make([]template.TemplateComponent, len(rows))
	for i := range rows {
		mapTemplateComponentToDomain(&rows[i], &out[i])
	}
	return out, nil
}

func (r *TemplateRepo) CountUsingFood(ctx context.Context, foodID int64) (int64, error) {
	return r.q.CountTemplatesUsingFood(ctx, foodID)
}

func (r *TemplateRepo) loadTemplateChildren(ctx context.Context, t *template.Template) error {
	rows, err := r.q.ListTemplateComponentsByTemplate(ctx, t.ID)
	if err != nil {
		return err
	}
	t.Components = make([]template.TemplateComponent, len(rows))
	for i := range rows {
		mapTemplateComponentToDomain(&rows[i], &t.Components[i])
	}
	return nil
}

func mapTemplateToDomain(row *sqlcgen.Template, t *template.Template) {
	t.ID = row.ID
	t.Name = row.Name
	t.CreatedAt, _ = time.Parse(timeLayout, row.CreatedAt) //nolint:errcheck // layout is controlled by our migration
}

func mapTemplateComponentToDomain(row *sqlcgen.TemplateComponent, tc *template.TemplateComponent) {
	tc.ID = row.ID
	tc.TemplateID = row.TemplateID
	tc.FoodID = row.FoodID
	tc.Portions = row.Portions
	tc.SortOrder = int(row.SortOrder)
	tc.DayOffset = int(row.DayOffset)
}
