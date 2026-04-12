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
	"github.com/jaltszeimer/plantry/backend/internal/domain/ingredient"
)

const timeLayout = "2006-01-02 15:04:05"

// allowedSortColumns prevents SQL injection through sort parameters.
var allowedSortColumns = map[string]string{
	"name":       "name",
	"created_at": "created_at",
	"updated_at": "updated_at",
	"kcal_100g":  "kcal_100g",
}

// IngredientRepo implements ingredient.Repository backed by SQLite.
type IngredientRepo struct {
	db *sql.DB
	q  *sqlcgen.Queries
}

// NewIngredientRepo creates a new SQLite-backed ingredient repository.
func NewIngredientRepo(db *sql.DB) *IngredientRepo {
	return &IngredientRepo{db: db, q: sqlcgen.New(db)}
}

func (r *IngredientRepo) Create(ctx context.Context, i *ingredient.Ingredient) error {
	row, err := r.q.CreateIngredient(ctx, sqlcgen.CreateIngredientParams{
		Name:        i.Name,
		Source:      i.Source,
		Barcode:     toNullString(i.Barcode),
		OffID:       toNullString(i.OffID),
		FdcID:       toNullString(i.FdcID),
		ImagePath:   toNullString(i.ImagePath),
		Kcal100g:    i.Kcal100g,
		Protein100g: i.Protein100g,
		Fat100g:     i.Fat100g,
		Carbs100g:   i.Carbs100g,
		Fiber100g:   i.Fiber100g,
		Sodium100g:  i.Sodium100g,
	})
	if err != nil {
		if isUniqueViolation(err, "ingredients.name") {
			return fmt.Errorf("%w: %s", domain.ErrDuplicateName, i.Name)
		}
		return err
	}
	mapToDomain(&row, i)
	return nil
}

func (r *IngredientRepo) Get(ctx context.Context, id int64) (*ingredient.Ingredient, error) {
	row, err := r.q.GetIngredient(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: id %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var i ingredient.Ingredient
	mapToDomain(&row, &i)
	return &i, nil
}

func (r *IngredientRepo) Update(ctx context.Context, i *ingredient.Ingredient) error {
	row, err := r.q.UpdateIngredient(ctx, sqlcgen.UpdateIngredientParams{
		ID:          i.ID,
		Name:        i.Name,
		Source:      i.Source,
		Barcode:     toNullString(i.Barcode),
		OffID:       toNullString(i.OffID),
		FdcID:       toNullString(i.FdcID),
		ImagePath:   toNullString(i.ImagePath),
		Kcal100g:    i.Kcal100g,
		Protein100g: i.Protein100g,
		Fat100g:     i.Fat100g,
		Carbs100g:   i.Carbs100g,
		Fiber100g:   i.Fiber100g,
		Sodium100g:  i.Sodium100g,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: id %d", domain.ErrNotFound, i.ID)
		}
		if isUniqueViolation(err, "ingredients.name") {
			return fmt.Errorf("%w: %s", domain.ErrDuplicateName, i.Name)
		}
		return err
	}
	mapToDomain(&row, i)
	return nil
}

func (r *IngredientRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.q.DeleteIngredient(ctx, id)
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

func (r *IngredientRepo) List(ctx context.Context, q ingredient.ListQuery) (*ingredient.ListResult, error) {
	col, ok := allowedSortColumns[q.SortBy]
	if !ok {
		col = "name"
	}
	dir := "ASC"
	if q.SortDesc {
		dir = "DESC"
	}
	orderClause := col + " " + dir

	// Build the base query
	builder := sq.Select("*").From("ingredients").OrderBy(orderClause).Limit(uint64(q.Limit)).Offset(uint64(q.Offset))
	countBuilder := sq.Select("COUNT(*)").From("ingredients")

	if q.Search != "" {
		ftsClause := "id IN (SELECT rowid FROM ingredients_fts WHERE ingredients_fts MATCH ?)"
		searchTerm := sanitizeFTS5(q.Search) + "*"
		builder = builder.Where(ftsClause, searchTerm)
		countBuilder = countBuilder.Where(ftsClause, searchTerm)
	}

	// Run count query
	countSQL, countArgs, err := countBuilder.PlaceholderFormat(sq.Question).ToSql()
	if err != nil {
		return nil, err
	}
	var total int
	if err := r.db.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, err
	}

	// Run list query
	listSQL, listArgs, err := builder.PlaceholderFormat(sq.Question).ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, err
	}

	items := make([]ingredient.Ingredient, 0)
	for rows.Next() {
		var row sqlcgen.Ingredient
		if err := rows.Scan(
			&row.ID, &row.Name, &row.Source, &row.Barcode, &row.OffID,
			&row.FdcID, &row.ImagePath, &row.Kcal100g, &row.Protein100g,
			&row.Fat100g, &row.Carbs100g, &row.Fiber100g, &row.Sodium100g,
			&row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		var i ingredient.Ingredient
		mapToDomain(&row, &i)
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	return &ingredient.ListResult{Items: items, Total: total}, nil
}

// sanitizeFTS5 wraps each word in double-quotes so FTS5 treats reserved words
// (AND, OR, NOT, NEAR) and special characters (*, ^, ") as literals.
func sanitizeFTS5(input string) string {
	words := strings.Fields(input)
	for i, w := range words {
		w = strings.ReplaceAll(w, `"`, `""`)
		words[i] = `"` + w + `"`
	}
	return strings.Join(words, " ")
}

// --- mapping helpers ---

func mapToDomain(row *sqlcgen.Ingredient, i *ingredient.Ingredient) {
	i.ID = row.ID
	i.Name = row.Name
	i.Source = row.Source
	i.Barcode = fromNullString(row.Barcode)
	i.OffID = fromNullString(row.OffID)
	i.FdcID = fromNullString(row.FdcID)
	i.ImagePath = fromNullString(row.ImagePath)
	i.Kcal100g = row.Kcal100g
	i.Protein100g = row.Protein100g
	i.Fat100g = row.Fat100g
	i.Carbs100g = row.Carbs100g
	i.Fiber100g = row.Fiber100g
	i.Sodium100g = row.Sodium100g
	i.CreatedAt, _ = time.Parse(timeLayout, row.CreatedAt) //nolint:errcheck // layout is controlled by our migration
	i.UpdatedAt, _ = time.Parse(timeLayout, row.UpdatedAt) //nolint:errcheck // layout is controlled by our migration
}

func toNullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func fromNullString(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}
