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
		Name:             i.Name,
		Source:           i.Source,
		Barcode:          toNullString(i.Barcode),
		OffID:            toNullString(i.OffID),
		FdcID:            toNullString(i.FdcID),
		ImagePath:        toNullString(i.ImagePath),
		Kcal100g:         i.Kcal100g,
		Protein100g:      i.Protein100g,
		Fat100g:          i.Fat100g,
		Carbs100g:        i.Carbs100g,
		Fiber100g:        i.Fiber100g,
		Sodium100g:       i.Sodium100g,
		SaturatedFat100g: toNullFloat(i.SaturatedFat100g),
		TransFat100g:     toNullFloat(i.TransFat100g),
		Cholesterol100g:  toNullFloat(i.Cholesterol100g),
		Sugar100g:        toNullFloat(i.Sugar100g),
		Potassium100g:    toNullFloat(i.Potassium100g),
		Calcium100g:      toNullFloat(i.Calcium100g),
		Iron100g:         toNullFloat(i.Iron100g),
		Magnesium100g:    toNullFloat(i.Magnesium100g),
		Phosphorus100g:   toNullFloat(i.Phosphorus100g),
		Zinc100g:         toNullFloat(i.Zinc100g),
		VitaminA100g:     toNullFloat(i.VitaminA100g),
		VitaminC100g:     toNullFloat(i.VitaminC100g),
		VitaminD100g:     toNullFloat(i.VitaminD100g),
		VitaminB12100g:   toNullFloat(i.VitaminB12100g),
		VitaminB6100g:    toNullFloat(i.VitaminB6100g),
		Folate100g:       toNullFloat(i.Folate100g),
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
		ID:               i.ID,
		Name:             i.Name,
		Source:           i.Source,
		Barcode:          toNullString(i.Barcode),
		OffID:            toNullString(i.OffID),
		FdcID:            toNullString(i.FdcID),
		ImagePath:        toNullString(i.ImagePath),
		Kcal100g:         i.Kcal100g,
		Protein100g:      i.Protein100g,
		Fat100g:          i.Fat100g,
		Carbs100g:        i.Carbs100g,
		Fiber100g:        i.Fiber100g,
		Sodium100g:       i.Sodium100g,
		SaturatedFat100g: toNullFloat(i.SaturatedFat100g),
		TransFat100g:     toNullFloat(i.TransFat100g),
		Cholesterol100g:  toNullFloat(i.Cholesterol100g),
		Sugar100g:        toNullFloat(i.Sugar100g),
		Potassium100g:    toNullFloat(i.Potassium100g),
		Calcium100g:      toNullFloat(i.Calcium100g),
		Iron100g:         toNullFloat(i.Iron100g),
		Magnesium100g:    toNullFloat(i.Magnesium100g),
		Phosphorus100g:   toNullFloat(i.Phosphorus100g),
		Zinc100g:         toNullFloat(i.Zinc100g),
		VitaminA100g:     toNullFloat(i.VitaminA100g),
		VitaminC100g:     toNullFloat(i.VitaminC100g),
		VitaminD100g:     toNullFloat(i.VitaminD100g),
		VitaminB12100g:   toNullFloat(i.VitaminB12100g),
		VitaminB6100g:    toNullFloat(i.VitaminB6100g),
		Folate100g:       toNullFloat(i.Folate100g),
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
		tokens := strings.Fields(sanitizeFTS5(q.Search))
		searchTerm := strings.Join(tokens, "* ") + "*"
		where := sq.Or{
			sq.Expr("id IN (SELECT rowid FROM ingredients_fts WHERE ingredients_fts MATCH ?)", searchTerm),
		}
		// LIKE fallback catches substrings inside compound words (e.g. "mais" → "Goldmais").
		likeConds := sq.And{}
		for _, t := range strings.Fields(q.Search) {
			likeConds = append(likeConds, sq.Expr(`name LIKE ? ESCAPE '\'`, "%"+escapeLike(t)+"%"))
		}
		if len(likeConds) > 0 {
			where = append(where, likeConds)
		}
		builder = builder.Where(where)
		countBuilder = countBuilder.Where(where)
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
	defer func() { _ = rows.Close() }()

	items := make([]ingredient.Ingredient, 0)
	for rows.Next() {
		var row sqlcgen.Ingredient
		if err := rows.Scan(
			&row.ID, &row.Name, &row.Source, &row.Barcode, &row.OffID,
			&row.FdcID, &row.ImagePath, &row.Kcal100g, &row.Protein100g,
			&row.Fat100g, &row.Carbs100g, &row.Fiber100g, &row.Sodium100g,
			&row.CreatedAt, &row.UpdatedAt,
			&row.SaturatedFat100g, &row.TransFat100g, &row.Cholesterol100g, &row.Sugar100g,
			&row.Potassium100g, &row.Calcium100g, &row.Iron100g, &row.Magnesium100g,
			&row.Phosphorus100g, &row.Zinc100g,
			&row.VitaminA100g, &row.VitaminC100g, &row.VitaminD100g,
			&row.VitaminB12100g, &row.VitaminB6100g, &row.Folate100g,
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

// LookupForNutrition fetches multiple ingredients by ID for nutrition calculation.
func (r *IngredientRepo) LookupForNutrition(ctx context.Context, ids []int64) (map[int64]*ingredient.Ingredient, error) {
	if len(ids) == 0 {
		return make(map[int64]*ingredient.Ingredient), nil
	}
	builder := sq.Select("*").From("ingredients").Where(sq.Eq{"id": ids})
	query, args, err := builder.PlaceholderFormat(sq.Question).ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int64]*ingredient.Ingredient, len(ids))
	for rows.Next() {
		var row sqlcgen.Ingredient
		if err := rows.Scan(
			&row.ID, &row.Name, &row.Source, &row.Barcode, &row.OffID,
			&row.FdcID, &row.ImagePath, &row.Kcal100g, &row.Protein100g,
			&row.Fat100g, &row.Carbs100g, &row.Fiber100g, &row.Sodium100g,
			&row.CreatedAt, &row.UpdatedAt,
			&row.SaturatedFat100g, &row.TransFat100g, &row.Cholesterol100g, &row.Sugar100g,
			&row.Potassium100g, &row.Calcium100g, &row.Iron100g, &row.Magnesium100g,
			&row.Phosphorus100g, &row.Zinc100g,
			&row.VitaminA100g, &row.VitaminC100g, &row.VitaminD100g,
			&row.VitaminB12100g, &row.VitaminB6100g, &row.Folate100g,
		); err != nil {
			return nil, err
		}
		var i ingredient.Ingredient
		mapToDomain(&row, &i)
		result[i.ID] = &i
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// escapeLike escapes LIKE wildcards so user input is treated literally. Pair with `ESCAPE '\'`.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
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
	i.SaturatedFat100g = fromNullFloat(row.SaturatedFat100g)
	i.TransFat100g = fromNullFloat(row.TransFat100g)
	i.Cholesterol100g = fromNullFloat(row.Cholesterol100g)
	i.Sugar100g = fromNullFloat(row.Sugar100g)
	i.Potassium100g = fromNullFloat(row.Potassium100g)
	i.Calcium100g = fromNullFloat(row.Calcium100g)
	i.Iron100g = fromNullFloat(row.Iron100g)
	i.Magnesium100g = fromNullFloat(row.Magnesium100g)
	i.Phosphorus100g = fromNullFloat(row.Phosphorus100g)
	i.Zinc100g = fromNullFloat(row.Zinc100g)
	i.VitaminA100g = fromNullFloat(row.VitaminA100g)
	i.VitaminC100g = fromNullFloat(row.VitaminC100g)
	i.VitaminD100g = fromNullFloat(row.VitaminD100g)
	i.VitaminB12100g = fromNullFloat(row.VitaminB12100g)
	i.VitaminB6100g = fromNullFloat(row.VitaminB6100g)
	i.Folate100g = fromNullFloat(row.Folate100g)
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

func toNullFloat(f *float64) sql.NullFloat64 {
	if f == nil {
		return sql.NullFloat64{}
	}
	return sql.NullFloat64{Float64: *f, Valid: true}
}

func fromNullFloat(nf sql.NullFloat64) *float64 {
	if !nf.Valid {
		return nil
	}
	v := nf.Float64
	return &v
}
