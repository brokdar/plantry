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
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

var allowedFoodSortColumns = map[string]string{
	"name":       "name",
	"created":    "created_at",
	"created_at": "created_at",
	"updated_at": "updated_at",
	"role":       "role",
	"kcal_100g":  "kcal_100g",
}

// sortLastEaten orders by the derived last_eaten date (D16): never eaten
// first (NULL sorts first ascending), then the oldest.
const sortLastEaten = "last_eaten"

// lastEatenExpr is the derived last_eaten date of the outer foods row as of
// the bound date, NULL when never eaten. It mirrors the FoodHistory query
// (queries/foods.sql), which fills the response fields.
const lastEatenExpr = `(SELECT MAX(p.date) FROM plate_components pc
	JOIN plates p ON p.id = pc.plate_id
	WHERE pc.food_id = foods.id AND p.date <= ?
	  AND NOT EXISTS (SELECT 1 FROM plate_feedback pf
	                  WHERE pf.plate_id = p.id AND pf.status = 'not_eaten'))`

// foodCols enumerates every column of `foods` in CREATE TABLE order.
// Using an explicit list instead of SELECT * makes the scan resilient to
// future ALTER TABLE ADD COLUMN migrations.
var foodCols = []string{
	"id", "name", "kind", "role",
	"source", "barcode", "off_id", "fdc_id",
	"kcal_100g", "protein_100g", "fat_100g", "carbs_100g",
	"fiber_100g", "sodium_100g",
	"saturated_fat_100g", "trans_fat_100g", "cholesterol_100g", "sugar_100g",
	"potassium_100g", "calcium_100g", "iron_100g", "magnesium_100g",
	"phosphorus_100g", "zinc_100g",
	"vitamin_a_100g", "vitamin_c_100g", "vitamin_d_100g",
	"vitamin_b12_100g", "vitamin_b6_100g", "folate_100g",
	"variant_group_id", "reference_portions", "prep_minutes", "cook_minutes",
	"notes", "image_path", "favorite",
	"created_at", "updated_at",
	"source_url", "description", "keep_days", "freezable",
}

// FoodRepo implements food.Repository backed by SQLite.
type FoodRepo struct {
	conn
}

// NewFoodRepo constructs a FoodRepo.
func NewFoodRepo(db *sql.DB) *FoodRepo {
	return &FoodRepo{conn: conn{db: db}}
}

func (r *FoodRepo) Create(ctx context.Context, f *food.Food) error {
	return r.inTx(ctx, func(ctx context.Context) error {
		return r.createWith(ctx, r.queriesFor(ctx), f)
	})
}

func (r *FoodRepo) createWith(ctx context.Context, qtx *sqlcgen.Queries, f *food.Food) error {
	row, err := qtx.CreateFood(ctx, foodCreateParams(f))
	if err != nil {
		if isUniqueViolation(err, "foods.name") {
			return fmt.Errorf("%w: %s", domain.ErrDuplicateName, f.Name)
		}
		return err
	}
	mapFoodToDomain(&row, f)

	return r.insertComposedChildren(ctx, qtx, f)
}

func (r *FoodRepo) Get(ctx context.Context, id int64) (*food.Food, error) {
	row, err := r.queriesFor(ctx).GetFood(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: id %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var f food.Food
	mapFoodToDomain(&row, &f)
	if err := r.loadComposed(ctx, r.queriesFor(ctx), &f); err != nil {
		return nil, err
	}
	if f.Kind == food.KindLeaf {
		portions, err := r.ListPortions(ctx, f.ID)
		if err != nil {
			return nil, err
		}
		f.Portions = portions
	}
	return &f, nil
}

func (r *FoodRepo) LeafIDByName(ctx context.Context, name string) (int64, error) {
	id, err := r.queriesFor(ctx).GetLeafFoodIDByName(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("%w: leaf %q", domain.ErrNotFound, name)
		}
		return 0, err
	}
	return id, nil
}

func (r *FoodRepo) Update(ctx context.Context, f *food.Food) error {
	return r.inTx(ctx, func(ctx context.Context) error {
		return r.updateWith(ctx, r.queriesFor(ctx), f)
	})
}

func (r *FoodRepo) updateWith(ctx context.Context, qtx *sqlcgen.Queries, f *food.Food) error {
	row, err := qtx.UpdateFood(ctx, foodUpdateParams(f))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: id %d", domain.ErrNotFound, f.ID)
		}
		if isUniqueViolation(err, "foods.name") {
			return fmt.Errorf("%w: %s", domain.ErrDuplicateName, f.Name)
		}
		return err
	}
	mapFoodToDomain(&row, f)

	if err := qtx.DeleteFoodComponents(ctx, f.ID); err != nil {
		return err
	}
	if err := qtx.DeleteFoodInstructions(ctx, f.ID); err != nil {
		return err
	}
	if err := qtx.DeleteFoodTags(ctx, f.ID); err != nil {
		return err
	}
	return r.insertComposedChildren(ctx, qtx, f)
}

func (r *FoodRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.queriesFor(ctx).DeleteFood(ctx, id)
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

func (r *FoodRepo) List(ctx context.Context, q food.ListQuery) (*food.ListResult, error) {
	dir := "ASC"
	if q.SortDesc {
		dir = "DESC"
	}
	today := q.Today.Format(time.DateOnly)

	builder := sq.Select(foodCols...).From("foods").
		Limit(uint64(q.Limit)).
		Offset(uint64(q.Offset))
	if q.SortBy == sortLastEaten {
		// Favorites do not float here: the point is to find what was
		// forgotten.
		builder = builder.OrderByClause(lastEatenExpr+" "+dir, today).OrderBy("name ASC")
	} else {
		col, ok := allowedFoodSortColumns[q.SortBy]
		if !ok {
			col = "name"
		}
		builder = builder.OrderBy("favorite DESC", col+" "+dir)
	}
	countBuilder := sq.Select("COUNT(*)").From("foods")

	if q.Kind != "" {
		builder = builder.Where("kind = ?", string(q.Kind))
		countBuilder = countBuilder.Where("kind = ?", string(q.Kind))
	}
	if q.Search != "" {
		ftsClause := "id IN (SELECT rowid FROM foods_fts WHERE foods_fts MATCH ?)"
		tokens := strings.Fields(sanitizeFTS5(q.Search))
		searchTerm := strings.Join(tokens, "* ") + "*"
		// LIKE fallback catches substrings inside compound words.
		likeConds := sq.And{}
		for _, t := range strings.Fields(q.Search) {
			likeConds = append(likeConds, sq.Expr(`name LIKE ? ESCAPE '\'`, "%"+escapeLike(t)+"%"))
		}
		where := sq.Or{sq.Expr(ftsClause, searchTerm)}
		if len(likeConds) > 0 {
			where = append(where, likeConds)
		}
		builder = builder.Where(where)
		countBuilder = countBuilder.Where(where)
	}
	if q.Role != "" {
		builder = builder.Where("role = ?", q.Role)
		countBuilder = countBuilder.Where("role = ?", q.Role)
	}
	if q.Tag != "" {
		tagClause := "id IN (SELECT food_id FROM food_tags WHERE tag = ?)"
		builder = builder.Where(tagClause, q.Tag)
		countBuilder = countBuilder.Where(tagClause, q.Tag)
	}
	if q.FavoriteOnly {
		builder = builder.Where("favorite = 1")
		countBuilder = countBuilder.Where("favorite = 1")
	}
	if q.MaxMinutes > 0 {
		where := sq.Expr("(prep_minutes IS NOT NULL OR cook_minutes IS NOT NULL) AND COALESCE(prep_minutes, 0) + COALESCE(cook_minutes, 0) <= ?", q.MaxMinutes)
		builder = builder.Where(where)
		countBuilder = countBuilder.Where(where)
	}
	if q.NotEatenWeeks > 0 {
		cutoff := q.Today.AddDate(0, 0, -7*q.NotEatenWeeks).Format(time.DateOnly)
		where := sq.Expr("COALESCE("+lastEatenExpr+", '') <= ?", today, cutoff)
		builder = builder.Where(where)
		countBuilder = countBuilder.Where(where)
	}
	if q.CreatedWithinDays > 0 {
		cutoff := q.Today.AddDate(0, 0, -q.CreatedWithinDays).Format(time.DateOnly)
		where := sq.Expr("date(created_at) >= ?", cutoff)
		builder = builder.Where(where)
		countBuilder = countBuilder.Where(where)
	}

	countSQL, countArgs, err := countBuilder.PlaceholderFormat(sq.Question).ToSql()
	if err != nil {
		return nil, err
	}
	var total int
	if err := r.dbtx(ctx).QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, err
	}

	listSQL, listArgs, err := builder.PlaceholderFormat(sq.Question).ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.dbtx(ctx).QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	items := make([]food.Food, 0)
	for rows.Next() {
		row, err := scanFoodRow(rows)
		if err != nil {
			return nil, err
		}
		var f food.Food
		mapFoodToDomain(row, &f)
		items = append(items, f)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	// Attach tags for composed foods so list consumers can render tag chips.
	for i := range items {
		if items[i].Kind != food.KindComposed {
			continue
		}
		tagRows, err := r.queriesFor(ctx).ListFoodTags(ctx, items[i].ID)
		if err != nil {
			return nil, err
		}
		items[i].Tags = make([]string, len(tagRows))
		for j, tr := range tagRows {
			items[i].Tags[j] = tr.Tag
		}
	}

	return &food.ListResult{Items: items, Total: total}, nil
}

// Reachable returns every food ID reachable from parentID via food_components
// edges (BFS). Used by the service for cycle detection before writing.
func (r *FoodRepo) Reachable(ctx context.Context, parentID int64) (map[int64]struct{}, error) {
	seen := map[int64]struct{}{parentID: {}}
	queue := []int64{parentID}
	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]
		rows, err := r.dbtx(ctx).QueryContext(ctx,
			`SELECT child_id FROM food_components WHERE parent_id = ?`, head)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var cid int64
			if err := rows.Scan(&cid); err != nil {
				_ = rows.Close()
				return nil, err
			}
			if _, ok := seen[cid]; ok {
				continue
			}
			seen[cid] = struct{}{}
			queue = append(queue, cid)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return seen, nil
}

func (r *FoodRepo) LookupByIDs(ctx context.Context, ids []int64) (map[int64]*food.Food, error) {
	if len(ids) == 0 {
		return map[int64]*food.Food{}, nil
	}
	builder := sq.Select(foodCols...).From("foods").Where(sq.Eq{"id": ids})
	query, args, err := builder.PlaceholderFormat(sq.Question).ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.dbtx(ctx).QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int64]*food.Food, len(ids))
	for rows.Next() {
		row, err := scanFoodRow(rows)
		if err != nil {
			return nil, err
		}
		var f food.Food
		mapFoodToDomain(row, &f)
		result[f.ID] = &f
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *FoodRepo) ListChildren(ctx context.Context, parentID int64) ([]food.FoodComponent, error) {
	rows, err := r.queriesFor(ctx).ListFoodComponents(ctx, parentID)
	if err != nil {
		return nil, err
	}
	out := make([]food.FoodComponent, len(rows))
	for i, row := range rows {
		out[i] = food.FoodComponent{
			ID:        row.ID,
			ParentID:  row.ParentID,
			ChildID:   row.ChildID,
			ChildName: row.ChildName,
			ChildKind: food.Kind(row.ChildKind),
			Amount:    row.Amount,
			Unit:      row.Unit,
			Grams:     row.Grams,
			SortOrder: int(row.SortOrder),
		}
	}
	return out, nil
}

// ── Portions ──────────────────────────────────────────────────────────

func (r *FoodRepo) ListPortions(ctx context.Context, foodID int64) ([]food.Portion, error) {
	rows, err := r.queriesFor(ctx).ListFoodPortions(ctx, foodID)
	if err != nil {
		return nil, err
	}
	out := make([]food.Portion, len(rows))
	for i, row := range rows {
		out[i] = food.Portion{
			FoodID: row.FoodID,
			Unit:   row.Unit,
			Grams:  row.Grams,
		}
	}
	return out, nil
}

func (r *FoodRepo) UpsertPortion(ctx context.Context, p *food.Portion) error {
	return r.queriesFor(ctx).UpsertFoodPortion(ctx, sqlcgen.UpsertFoodPortionParams{
		FoodID: p.FoodID,
		Unit:   p.Unit,
		Grams:  p.Grams,
	})
}

func (r *FoodRepo) DeletePortion(ctx context.Context, foodID int64, unit string) error {
	res, err := r.queriesFor(ctx).DeleteFoodPortion(ctx, sqlcgen.DeleteFoodPortionParams{
		FoodID: foodID,
		Unit:   unit,
	})
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: portion (food_id=%d, unit=%s)", domain.ErrNotFound, foodID, unit)
	}
	return nil
}

// ── Variants ──────────────────────────────────────────────────────────

func (r *FoodRepo) CreateVariantGroup(ctx context.Context, name string) (int64, error) {
	g, err := r.queriesFor(ctx).CreateVariantGroup(ctx, name)
	if err != nil {
		return 0, err
	}
	return g.ID, nil
}

func (r *FoodRepo) Siblings(ctx context.Context, variantGroupID int64, excludeID int64) ([]food.Food, error) {
	rows, err := r.queriesFor(ctx).ListSiblingFoods(ctx, sqlcgen.ListSiblingFoodsParams{
		VariantGroupID: sql.NullInt64{Int64: variantGroupID, Valid: true},
		ID:             excludeID,
	})
	if err != nil {
		return nil, err
	}
	items := make([]food.Food, len(rows))
	for i := range rows {
		mapFoodToDomain(&rows[i], &items[i])
		if err := r.loadComposed(ctx, r.queriesFor(ctx), &items[i]); err != nil {
			return nil, err
		}
	}
	return items, nil
}

// ── Derived read data ─────────────────────────────────────────────────

// CountComposedFoods returns the number of composed foods (recipes).
func (r *FoodRepo) CountComposedFoods(ctx context.Context) (int64, error) {
	return r.queriesFor(ctx).CountComposedFoods(ctx)
}

// History returns the derived eating history (D16) of the given foods as of
// today, in one query.
func (r *FoodRepo) History(ctx context.Context, ids []int64, today time.Time) (map[int64]food.History, error) {
	rows, err := r.queriesFor(ctx).FoodHistory(ctx, sqlcgen.FoodHistoryParams{
		Today: today.Format(time.DateOnly),
		Ids:   ids,
	})
	if err != nil {
		return nil, err
	}
	out := make(map[int64]food.History, len(rows))
	for _, row := range rows {
		h := food.History{TimesCooked: int(row.TimesCooked)}
		if row.LastEaten != "" {
			d, err := time.Parse(time.DateOnly, row.LastEaten)
			if err != nil {
				return nil, fmt.Errorf("food %d last_eaten %q: %w", row.FoodID, row.LastEaten, err)
			}
			h.LastEaten = &d
		}
		out[row.FoodID] = h
	}
	return out, nil
}

// ListTags returns the food's tags.
func (r *FoodRepo) ListTags(ctx context.Context, foodID int64) ([]string, error) {
	rows, err := r.queriesFor(ctx).ListFoodTags(ctx, foodID)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(rows))
	for i, row := range rows {
		out[i] = row.Tag
	}
	return out, nil
}

// UsedInCounts returns how many recipes use each given leaf, in one query.
func (r *FoodRepo) UsedInCounts(ctx context.Context, ids []int64) (map[int64]int, error) {
	rows, err := r.queriesFor(ctx).UsedInCount(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]int, len(rows))
	for _, row := range rows {
		out[row.ChildID] = int(row.UsedIn)
	}
	return out, nil
}

func (r *FoodRepo) SetFavorite(ctx context.Context, id int64, favorite bool) (*food.Food, error) {
	fav := int64(0)
	if favorite {
		fav = 1
	}
	row, err := r.queriesFor(ctx).SetFoodFavorite(ctx, sqlcgen.SetFoodFavoriteParams{
		ID:       id,
		Favorite: fav,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: id %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var f food.Food
	mapFoodToDomain(&row, &f)
	if err := r.loadComposed(ctx, r.queriesFor(ctx), &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// Exists reports whether a food with the given ID exists. Implements
// food.ChildExistsChecker (used by plate + template services).
func (r *FoodRepo) Exists(ctx context.Context, id int64) (bool, error) {
	_, err := r.queriesFor(ctx).GetFood(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// KindOf returns the food's kind ("leaf" or "composed"). Used by callers that
// need to discriminate quantity shape without paying the cost of a full Get.
func (r *FoodRepo) KindOf(ctx context.Context, id int64) (string, error) {
	kind, err := r.queriesFor(ctx).GetFoodKind(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: food %d", domain.ErrNotFound, id)
		}
		return "", err
	}
	return kind, nil
}

// ── Internal ──────────────────────────────────────────────────────────

func (r *FoodRepo) insertComposedChildren(ctx context.Context, qtx *sqlcgen.Queries, f *food.Food) error {
	if f.Kind != food.KindComposed {
		return nil
	}
	for _, ch := range f.Children {
		if err := qtx.CreateFoodComponent(ctx, sqlcgen.CreateFoodComponentParams{
			ParentID:  f.ID,
			ChildID:   ch.ChildID,
			Amount:    ch.Amount,
			Unit:      ch.Unit,
			Grams:     ch.Grams,
			SortOrder: int64(ch.SortOrder),
		}); err != nil {
			return err
		}
	}
	for _, inst := range f.Instructions {
		if err := qtx.CreateFoodInstruction(ctx, sqlcgen.CreateFoodInstructionParams{
			FoodID:     f.ID,
			StepNumber: int64(inst.StepNumber),
			Text:       inst.Text,
		}); err != nil {
			return err
		}
	}
	for _, tag := range f.Tags {
		if err := qtx.CreateFoodTag(ctx, sqlcgen.CreateFoodTagParams{
			FoodID: f.ID,
			Tag:    tag,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *FoodRepo) loadComposed(ctx context.Context, q *sqlcgen.Queries, f *food.Food) error {
	if f.Kind != food.KindComposed {
		return nil
	}
	childRows, err := q.ListFoodComponents(ctx, f.ID)
	if err != nil {
		return err
	}
	f.Children = make([]food.FoodComponent, len(childRows))
	for i, row := range childRows {
		f.Children[i] = food.FoodComponent{
			ID:        row.ID,
			ParentID:  row.ParentID,
			ChildID:   row.ChildID,
			ChildName: row.ChildName,
			ChildKind: food.Kind(row.ChildKind),
			Amount:    row.Amount,
			Unit:      row.Unit,
			Grams:     row.Grams,
			SortOrder: int(row.SortOrder),
		}
	}
	instRows, err := q.ListFoodInstructions(ctx, f.ID)
	if err != nil {
		return err
	}
	f.Instructions = make([]food.Instruction, len(instRows))
	for i, row := range instRows {
		f.Instructions[i] = food.Instruction{
			ID:         row.ID,
			FoodID:     row.FoodID,
			StepNumber: int(row.StepNumber),
			Text:       row.Text,
		}
	}
	tagRows, err := q.ListFoodTags(ctx, f.ID)
	if err != nil {
		return err
	}
	f.Tags = make([]string, len(tagRows))
	for i, row := range tagRows {
		f.Tags[i] = row.Tag
	}
	return nil
}

// ── Mapping ───────────────────────────────────────────────────────────

func foodCreateParams(f *food.Food) sqlcgen.CreateFoodParams {
	fav := int64(0)
	if f.Favorite {
		fav = 1
	}
	return sqlcgen.CreateFoodParams{
		Name:              f.Name,
		Kind:              string(f.Kind),
		Role:              roleToNullString(f.Role),
		Source:            sourceToNullString(f.Source),
		Barcode:           toNullString(f.Barcode),
		OffID:             toNullString(f.OffID),
		FdcID:             toNullString(f.FdcID),
		ImagePath:         toNullString(f.ImagePath),
		Kcal100g:          toNullFloat(f.Kcal100g),
		Protein100g:       toNullFloat(f.Protein100g),
		Fat100g:           toNullFloat(f.Fat100g),
		Carbs100g:         toNullFloat(f.Carbs100g),
		Fiber100g:         toNullFloat(f.Fiber100g),
		Sodium100g:        toNullFloat(f.Sodium100g),
		SaturatedFat100g:  toNullFloat(f.SaturatedFat100g),
		TransFat100g:      toNullFloat(f.TransFat100g),
		Cholesterol100g:   toNullFloat(f.Cholesterol100g),
		Sugar100g:         toNullFloat(f.Sugar100g),
		Potassium100g:     toNullFloat(f.Potassium100g),
		Calcium100g:       toNullFloat(f.Calcium100g),
		Iron100g:          toNullFloat(f.Iron100g),
		Magnesium100g:     toNullFloat(f.Magnesium100g),
		Phosphorus100g:    toNullFloat(f.Phosphorus100g),
		Zinc100g:          toNullFloat(f.Zinc100g),
		VitaminA100g:      toNullFloat(f.VitaminA100g),
		VitaminC100g:      toNullFloat(f.VitaminC100g),
		VitaminD100g:      toNullFloat(f.VitaminD100g),
		VitaminB12100g:    toNullFloat(f.VitaminB12100g),
		VitaminB6100g:     toNullFloat(f.VitaminB6100g),
		Folate100g:        toNullFloat(f.Folate100g),
		VariantGroupID:    toNullInt64(f.VariantGroupID),
		ReferencePortions: toNullFloat(f.ReferencePortions),
		PrepMinutes:       toNullInt64FromIntPtr(f.PrepMinutes),
		CookMinutes:       toNullInt64FromIntPtr(f.CookMinutes),
		Notes:             toNullString(f.Notes),
		Favorite:          fav,
		SourceUrl:         toNullStringEmpty(derefString(f.SourceURL)),
		Description:       toNullStringEmpty(derefString(f.Description)),
		KeepDays:          toNullInt64FromIntPtr(f.KeepDays),
		Freezable:         boolPtrToNullInt64(f.Freezable),
	}
}

func foodUpdateParams(f *food.Food) sqlcgen.UpdateFoodParams {
	fav := int64(0)
	if f.Favorite {
		fav = 1
	}
	return sqlcgen.UpdateFoodParams{
		ID:                f.ID,
		Name:              f.Name,
		Role:              roleToNullString(f.Role),
		Source:            sourceToNullString(f.Source),
		Barcode:           toNullString(f.Barcode),
		OffID:             toNullString(f.OffID),
		FdcID:             toNullString(f.FdcID),
		ImagePath:         toNullString(f.ImagePath),
		Kcal100g:          toNullFloat(f.Kcal100g),
		Protein100g:       toNullFloat(f.Protein100g),
		Fat100g:           toNullFloat(f.Fat100g),
		Carbs100g:         toNullFloat(f.Carbs100g),
		Fiber100g:         toNullFloat(f.Fiber100g),
		Sodium100g:        toNullFloat(f.Sodium100g),
		SaturatedFat100g:  toNullFloat(f.SaturatedFat100g),
		TransFat100g:      toNullFloat(f.TransFat100g),
		Cholesterol100g:   toNullFloat(f.Cholesterol100g),
		Sugar100g:         toNullFloat(f.Sugar100g),
		Potassium100g:     toNullFloat(f.Potassium100g),
		Calcium100g:       toNullFloat(f.Calcium100g),
		Iron100g:          toNullFloat(f.Iron100g),
		Magnesium100g:     toNullFloat(f.Magnesium100g),
		Phosphorus100g:    toNullFloat(f.Phosphorus100g),
		Zinc100g:          toNullFloat(f.Zinc100g),
		VitaminA100g:      toNullFloat(f.VitaminA100g),
		VitaminC100g:      toNullFloat(f.VitaminC100g),
		VitaminD100g:      toNullFloat(f.VitaminD100g),
		VitaminB12100g:    toNullFloat(f.VitaminB12100g),
		VitaminB6100g:     toNullFloat(f.VitaminB6100g),
		Folate100g:        toNullFloat(f.Folate100g),
		VariantGroupID:    toNullInt64(f.VariantGroupID),
		ReferencePortions: toNullFloat(f.ReferencePortions),
		PrepMinutes:       toNullInt64FromIntPtr(f.PrepMinutes),
		CookMinutes:       toNullInt64FromIntPtr(f.CookMinutes),
		Notes:             toNullString(f.Notes),
		Favorite:          fav,
		SourceUrl:         toNullStringEmpty(derefString(f.SourceURL)),
		Description:       toNullStringEmpty(derefString(f.Description)),
		KeepDays:          toNullInt64FromIntPtr(f.KeepDays),
		Freezable:         boolPtrToNullInt64(f.Freezable),
	}
}

func boolPtrToNullInt64(b *bool) sql.NullInt64 {
	if b == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: boolToInt(*b), Valid: true}
}

// derefString returns *s, or "" for nil. Paired with toNullStringEmpty so
// both nil and "" are stored as NULL.
func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mapFoodToDomain(row *sqlcgen.Food, f *food.Food) {
	f.ID = row.ID
	f.Name = row.Name
	f.Kind = food.Kind(row.Kind)
	f.Role = nullStringToRole(row.Role)
	f.Source = nullStringToSource(row.Source)
	f.Barcode = fromNullString(row.Barcode)
	f.OffID = fromNullString(row.OffID)
	f.FdcID = fromNullString(row.FdcID)
	f.Kcal100g = fromNullFloat(row.Kcal100g)
	f.Protein100g = fromNullFloat(row.Protein100g)
	f.Fat100g = fromNullFloat(row.Fat100g)
	f.Carbs100g = fromNullFloat(row.Carbs100g)
	f.Fiber100g = fromNullFloat(row.Fiber100g)
	f.Sodium100g = fromNullFloat(row.Sodium100g)
	f.SaturatedFat100g = fromNullFloat(row.SaturatedFat100g)
	f.TransFat100g = fromNullFloat(row.TransFat100g)
	f.Cholesterol100g = fromNullFloat(row.Cholesterol100g)
	f.Sugar100g = fromNullFloat(row.Sugar100g)
	f.Potassium100g = fromNullFloat(row.Potassium100g)
	f.Calcium100g = fromNullFloat(row.Calcium100g)
	f.Iron100g = fromNullFloat(row.Iron100g)
	f.Magnesium100g = fromNullFloat(row.Magnesium100g)
	f.Phosphorus100g = fromNullFloat(row.Phosphorus100g)
	f.Zinc100g = fromNullFloat(row.Zinc100g)
	f.VitaminA100g = fromNullFloat(row.VitaminA100g)
	f.VitaminC100g = fromNullFloat(row.VitaminC100g)
	f.VitaminD100g = fromNullFloat(row.VitaminD100g)
	f.VitaminB12100g = fromNullFloat(row.VitaminB12100g)
	f.VitaminB6100g = fromNullFloat(row.VitaminB6100g)
	f.Folate100g = fromNullFloat(row.Folate100g)
	f.VariantGroupID = fromNullInt64(row.VariantGroupID)
	f.ReferencePortions = fromNullFloat(row.ReferencePortions)
	f.PrepMinutes = fromNullInt64ToIntPtr(row.PrepMinutes)
	f.CookMinutes = fromNullInt64ToIntPtr(row.CookMinutes)
	f.Notes = fromNullString(row.Notes)
	f.SourceURL = fromNullString(row.SourceUrl)
	f.Description = fromNullString(row.Description)
	f.ImagePath = fromNullString(row.ImagePath)
	f.Favorite = row.Favorite != 0
	f.KeepDays = fromNullInt64ToIntPtr(row.KeepDays)
	if row.Freezable.Valid {
		v := row.Freezable.Int64 == 1
		f.Freezable = &v
	} else {
		f.Freezable = nil
	}
	f.CreatedAt, _ = time.Parse(timeLayout, row.CreatedAt)
	f.UpdatedAt, _ = time.Parse(timeLayout, row.UpdatedAt)
}

func roleToNullString(r *food.Role) sql.NullString {
	if r == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(*r), Valid: true}
}

func nullStringToRole(ns sql.NullString) *food.Role {
	if !ns.Valid {
		return nil
	}
	r := food.Role(ns.String)
	return &r
}

func sourceToNullString(s *food.Source) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func nullStringToSource(ns sql.NullString) *food.Source {
	if !ns.Valid {
		return nil
	}
	v := food.Source(ns.String)
	return &v
}

// scanFoodRow scans one row from a `SELECT * FROM foods` into a sqlcgen.Food
// in the column order declared by the foods CREATE TABLE.
func scanFoodRow(rows *sql.Rows) (*sqlcgen.Food, error) {
	var row sqlcgen.Food
	if err := rows.Scan(
		&row.ID, &row.Name, &row.Kind, &row.Role,
		&row.Source, &row.Barcode, &row.OffID, &row.FdcID,
		&row.Kcal100g, &row.Protein100g, &row.Fat100g, &row.Carbs100g,
		&row.Fiber100g, &row.Sodium100g,
		&row.SaturatedFat100g, &row.TransFat100g, &row.Cholesterol100g, &row.Sugar100g,
		&row.Potassium100g, &row.Calcium100g, &row.Iron100g, &row.Magnesium100g,
		&row.Phosphorus100g, &row.Zinc100g,
		&row.VitaminA100g, &row.VitaminC100g, &row.VitaminD100g,
		&row.VitaminB12100g, &row.VitaminB6100g, &row.Folate100g,
		&row.VariantGroupID, &row.ReferencePortions, &row.PrepMinutes, &row.CookMinutes,
		&row.Notes, &row.ImagePath, &row.Favorite,
		&row.CreatedAt, &row.UpdatedAt,
		&row.SourceUrl, &row.Description, &row.KeepDays, &row.Freezable,
	); err != nil {
		return nil, err
	}
	return &row, nil
}
