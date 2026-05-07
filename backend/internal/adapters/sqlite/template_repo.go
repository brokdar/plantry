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
	scope := t.Scope
	if scope == "" {
		scope = template.ScopeSlot
	}
	row, err := q.CreateTemplate(ctx, sqlcgen.CreateTemplateParams{Name: t.Name, Scope: string(scope)})
	if err != nil {
		return err
	}
	mapTemplateToDomain(&row, t)
	for i := range t.Entries {
		te := &t.Entries[i]
		te.TemplateID = t.ID
		if te.SortOrder == 0 && i > 0 {
			te.SortOrder = i
		}
		teRow, err := q.CreateTemplateEntry(ctx, sqlcgen.CreateTemplateEntryParams{
			TemplateID: te.TemplateID,
			FoodID:     te.FoodID,
			Portions:   te.Portions,
			SortOrder:  int64(te.SortOrder),
			DayOffset:  int64(te.DayOffset),
			SlotID:     toNullInt64(te.SlotID),
			Note:       toNullString(te.Note),
		})
		if err != nil {
			if isForeignKeyViolation(err) {
				return fmt.Errorf("%w: invalid food or slot reference", domain.ErrInvalidInput)
			}
			return err
		}
		mapTemplateEntryToDomain(&teRow, te)
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
	return r.hydrateTemplates(ctx, rows)
}

func (r *TemplateRepo) ListByScope(ctx context.Context, scope template.Scope) ([]template.Template, error) {
	rows, err := r.q.ListTemplatesByScope(ctx, string(scope))
	if err != nil {
		return nil, err
	}
	return r.hydrateTemplates(ctx, rows)
}

func (r *TemplateRepo) hydrateTemplates(ctx context.Context, rows []sqlcgen.Template) ([]template.Template, error) {
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

func (r *TemplateRepo) ReplaceEntries(ctx context.Context, templateID int64, entries []template.TemplateEntry) error {
	if r.db == nil {
		return r.replaceWith(ctx, r.q, templateID, entries)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.replaceWith(ctx, sqlcgen.New(tx), templateID, entries); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *TemplateRepo) replaceWith(ctx context.Context, q *sqlcgen.Queries, templateID int64, entries []template.TemplateEntry) error {
	if _, err := q.DeleteTemplateEntriesByTemplate(ctx, templateID); err != nil {
		return err
	}
	for i, te := range entries {
		if _, err := q.CreateTemplateEntry(ctx, sqlcgen.CreateTemplateEntryParams{
			TemplateID: templateID,
			FoodID:     te.FoodID,
			Portions:   te.Portions,
			SortOrder:  int64(i),
			DayOffset:  int64(te.DayOffset),
			SlotID:     toNullInt64(te.SlotID),
			Note:       toNullString(te.Note),
		}); err != nil {
			if isForeignKeyViolation(err) {
				return fmt.Errorf("%w: invalid food or slot reference", domain.ErrInvalidInput)
			}
			return err
		}
	}
	return nil
}

func (r *TemplateRepo) ListEntriesByTemplate(ctx context.Context, templateID int64) ([]template.TemplateEntry, error) {
	rows, err := r.q.ListTemplateEntriesByTemplate(ctx, templateID)
	if err != nil {
		return nil, err
	}
	out := make([]template.TemplateEntry, len(rows))
	for i := range rows {
		mapTemplateEntryToDomain(&rows[i], &out[i])
	}
	return out, nil
}

func (r *TemplateRepo) CountUsingFood(ctx context.Context, foodID int64) (int64, error) {
	return r.q.CountTemplatesUsingFood(ctx, foodID)
}

func (r *TemplateRepo) loadTemplateChildren(ctx context.Context, t *template.Template) error {
	rows, err := r.q.ListTemplateEntriesByTemplate(ctx, t.ID)
	if err != nil {
		return err
	}
	t.Entries = make([]template.TemplateEntry, len(rows))
	for i := range rows {
		mapTemplateEntryToDomain(&rows[i], &t.Entries[i])
	}
	return nil
}

func mapTemplateToDomain(row *sqlcgen.Template, t *template.Template) {
	t.ID = row.ID
	t.Name = row.Name
	t.Scope = template.Scope(row.Scope)
	t.CreatedAt, _ = time.Parse(timeLayout, row.CreatedAt) //nolint:errcheck // layout is controlled by our migration
}

func mapTemplateEntryToDomain(row *sqlcgen.TemplateEntry, te *template.TemplateEntry) {
	te.ID = row.ID
	te.TemplateID = row.TemplateID
	te.FoodID = row.FoodID
	te.Portions = row.Portions
	te.SortOrder = int(row.SortOrder)
	te.DayOffset = int(row.DayOffset)
	te.SlotID = fromNullInt64(row.SlotID)
	te.Note = fromNullString(row.Note)
}
