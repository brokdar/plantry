package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/component"
)

var allowedComponentSortColumns = map[string]string{
	"name":           "name",
	"created_at":     "created_at",
	"updated_at":     "updated_at",
	"role":           "role",
	"last_cooked_at": "last_cooked_at",
}

// ComponentRepo implements component.Repository backed by SQLite.
type ComponentRepo struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

// NewComponentRepo creates a new SQLite-backed component repository.
func NewComponentRepo(db *sql.DB) *ComponentRepo {
	return &ComponentRepo{db: db, q: sqlcgen.New(db)}
}

func (r *ComponentRepo) Create(ctx context.Context, c *component.Component) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := sqlcgen.New(tx)
	row, err := qtx.CreateComponent(ctx, sqlcgen.CreateComponentParams{
		Name:              c.Name,
		Role:              string(c.Role),
		VariantGroupID:    toNullInt64(c.VariantGroupID),
		ReferencePortions: c.ReferencePortions,
		PrepMinutes:       toNullInt64FromIntPtr(c.PrepMinutes),
		CookMinutes:       toNullInt64FromIntPtr(c.CookMinutes),
		ImagePath:         toNullString(c.ImagePath),
		Notes:             toNullString(c.Notes),
	})
	if err != nil {
		return err
	}
	mapComponentToDomain(&row, c)

	if err := r.insertChildren(ctx, qtx, c); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ComponentRepo) Get(ctx context.Context, id int64) (*component.Component, error) {
	row, err := r.q.GetComponent(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: id %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var c component.Component
	mapComponentToDomain(&row, &c)

	if err := r.loadChildren(ctx, r.q, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *ComponentRepo) Update(ctx context.Context, c *component.Component) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	qtx := sqlcgen.New(tx)
	row, err := qtx.UpdateComponent(ctx, sqlcgen.UpdateComponentParams{
		ID:                c.ID,
		Name:              c.Name,
		Role:              string(c.Role),
		VariantGroupID:    toNullInt64(c.VariantGroupID),
		ReferencePortions: c.ReferencePortions,
		PrepMinutes:       toNullInt64FromIntPtr(c.PrepMinutes),
		CookMinutes:       toNullInt64FromIntPtr(c.CookMinutes),
		ImagePath:         toNullString(c.ImagePath),
		Notes:             toNullString(c.Notes),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: id %d", domain.ErrNotFound, c.ID)
		}
		return err
	}
	mapComponentToDomain(&row, c)

	// Replace all children: delete then re-insert.
	if err := qtx.DeleteComponentIngredients(ctx, c.ID); err != nil {
		return err
	}
	if err := qtx.DeleteComponentInstructions(ctx, c.ID); err != nil {
		return err
	}
	if err := qtx.DeleteComponentTags(ctx, c.ID); err != nil {
		return err
	}
	if err := r.insertChildren(ctx, qtx, c); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *ComponentRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.q.DeleteComponent(ctx, id)
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: id %d", domain.ErrInUse, id)
		}
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: id %d", domain.ErrNotFound, id)
	}
	return nil
}

func (r *ComponentRepo) List(ctx context.Context, q component.ListQuery) (*component.ListResult, error) {
	col, ok := allowedComponentSortColumns[q.SortBy]
	if !ok {
		col = "name"
	}
	dir := "ASC"
	if q.SortDesc {
		dir = "DESC"
	}
	orderClause := col + " " + dir

	builder := sq.Select("*").From("components").OrderBy(orderClause).Limit(uint64(q.Limit)).Offset(uint64(q.Offset))
	countBuilder := sq.Select("COUNT(*)").From("components")

	if q.Search != "" {
		ftsClause := "id IN (SELECT rowid FROM components_fts WHERE components_fts MATCH ?)"
		tokens := strings.Fields(sanitizeFTS5(q.Search))
		searchTerm := strings.Join(tokens, "* ") + "*"
		builder = builder.Where(ftsClause, searchTerm)
		countBuilder = countBuilder.Where(ftsClause, searchTerm)
	}
	if q.Role != "" {
		builder = builder.Where("role = ?", q.Role)
		countBuilder = countBuilder.Where("role = ?", q.Role)
	}
	if q.Tag != "" {
		tagClause := "id IN (SELECT component_id FROM component_tags WHERE tag = ?)"
		builder = builder.Where(tagClause, q.Tag)
		countBuilder = countBuilder.Where(tagClause, q.Tag)
	}

	countSQL, countArgs, err := countBuilder.PlaceholderFormat(sq.Question).ToSql()
	if err != nil {
		return nil, err
	}
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, err
	}

	listSQL, listArgs, err := builder.PlaceholderFormat(sq.Question).ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]component.Component, 0)
	for rows.Next() {
		var row sqlcgen.Component
		if err := rows.Scan(
			&row.ID, &row.Name, &row.Role, &row.VariantGroupID,
			&row.ReferencePortions, &row.PrepMinutes, &row.CookMinutes,
			&row.ImagePath, &row.Notes, &row.LastCookedAt,
			&row.CookCount, &row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		var c component.Component
		mapComponentToDomain(&row, &c)
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	return &component.ListResult{Items: items, Total: total}, nil
}

func (r *ComponentRepo) CreateVariantGroup(ctx context.Context, name string) (int64, error) {
	group, err := r.q.CreateVariantGroup(ctx, name)
	if err != nil {
		return 0, err
	}
	return group.ID, nil
}

// Exists reports whether a component with the given ID exists.
// Implements plate.ComponentChecker.
func (r *ComponentRepo) Exists(ctx context.Context, id int64) (bool, error) {
	_, err := r.q.GetComponent(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *ComponentRepo) Siblings(ctx context.Context, variantGroupID int64, excludeID int64) ([]component.Component, error) {
	rows, err := r.q.ListSiblingComponents(ctx, sqlcgen.ListSiblingComponentsParams{
		VariantGroupID: sql.NullInt64{Int64: variantGroupID, Valid: true},
		ID:             excludeID,
	})
	if err != nil {
		return nil, err
	}
	items := make([]component.Component, len(rows))
	for i := range rows {
		mapComponentToDomain(&rows[i], &items[i])
		if err := r.loadChildren(ctx, r.q, &items[i]); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// --- internal helpers ---

func (r *ComponentRepo) insertChildren(ctx context.Context, qtx *sqlcgen.Queries, c *component.Component) error {
	for _, ci := range c.Ingredients {
		if err := qtx.CreateComponentIngredient(ctx, sqlcgen.CreateComponentIngredientParams{
			ComponentID:  c.ID,
			IngredientID: ci.IngredientID,
			Amount:       ci.Amount,
			Unit:         ci.Unit,
			Grams:        ci.Grams,
			SortOrder:    int64(ci.SortOrder),
		}); err != nil {
			return err
		}
	}
	for _, inst := range c.Instructions {
		if err := qtx.CreateComponentInstruction(ctx, sqlcgen.CreateComponentInstructionParams{
			ComponentID: c.ID,
			StepNumber:  int64(inst.StepNumber),
			Text:        inst.Text,
		}); err != nil {
			return err
		}
	}
	for _, tag := range c.Tags {
		if err := qtx.CreateComponentTag(ctx, sqlcgen.CreateComponentTagParams{
			ComponentID: c.ID,
			Tag:         tag,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *ComponentRepo) loadChildren(ctx context.Context, q *sqlcgen.Queries, c *component.Component) error {
	ciRows, err := q.ListComponentIngredients(ctx, c.ID)
	if err != nil {
		return err
	}
	c.Ingredients = make([]component.ComponentIngredient, len(ciRows))
	for i, row := range ciRows {
		c.Ingredients[i] = component.ComponentIngredient{
			ID:           row.ID,
			ComponentID:  row.ComponentID,
			IngredientID: row.IngredientID,
			Amount:       row.Amount,
			Unit:         row.Unit,
			Grams:        row.Grams,
			SortOrder:    int(row.SortOrder),
		}
	}

	instRows, err := q.ListComponentInstructions(ctx, c.ID)
	if err != nil {
		return err
	}
	c.Instructions = make([]component.Instruction, len(instRows))
	for i, row := range instRows {
		c.Instructions[i] = component.Instruction{
			ID:          row.ID,
			ComponentID: row.ComponentID,
			StepNumber:  int(row.StepNumber),
			Text:        row.Text,
		}
	}

	tagRows, err := q.ListComponentTags(ctx, c.ID)
	if err != nil {
		return err
	}
	c.Tags = make([]string, len(tagRows))
	for i, row := range tagRows {
		c.Tags[i] = row.Tag
	}
	return nil
}

func mapComponentToDomain(row *sqlcgen.Component, c *component.Component) {
	c.ID = row.ID
	c.Name = row.Name
	c.Role = component.Role(row.Role)
	c.VariantGroupID = fromNullInt64(row.VariantGroupID)
	c.ReferencePortions = row.ReferencePortions
	c.PrepMinutes = fromNullInt64ToIntPtr(row.PrepMinutes)
	c.CookMinutes = fromNullInt64ToIntPtr(row.CookMinutes)
	c.ImagePath = fromNullString(row.ImagePath)
	c.Notes = fromNullString(row.Notes)
	c.CookCount = int(row.CookCount)
	c.CreatedAt, _ = time.Parse(timeLayout, row.CreatedAt)
	c.UpdatedAt, _ = time.Parse(timeLayout, row.UpdatedAt)
	if row.LastCookedAt.Valid {
		t, _ := time.Parse(timeLayout, row.LastCookedAt.String)
		c.LastCookedAt = &t
	}
}

func toNullInt64FromIntPtr(v *int) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*v), Valid: true}
}

func fromNullInt64ToIntPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func toNullInt64(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func fromNullInt64(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}
