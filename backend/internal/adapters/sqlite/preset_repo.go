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
	"github.com/jaltszeimer/plantry/backend/internal/domain/preset"
)

// PresetRepo implements preset.Repository backed by SQLite.
type PresetRepo struct {
	db   *sql.DB
	dbtx dbtxRunner
	q    *sqlcgen.Queries
}

// dbtxRunner is the minimal Query interface satisfied by *sql.DB and *sql.Tx.
// Used by the squirrel-driven List query which sqlc can't express (FTS5 MATCH).
type dbtxRunner interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// NewPresetRepo creates a SQLite-backed preset repository.
func NewPresetRepo(db *sql.DB) *PresetRepo {
	return &PresetRepo{db: db, dbtx: db, q: sqlcgen.New(db)}
}

// newPresetRepoTx returns a PresetRepo bound to an outer transaction.
func newPresetRepoTx(tx *sql.Tx) *PresetRepo {
	return &PresetRepo{db: nil, dbtx: tx, q: sqlcgen.New(tx)}
}

func (r *PresetRepo) Create(ctx context.Context, p *preset.Preset) error {
	if r.db == nil {
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

func (r *PresetRepo) createWith(ctx context.Context, q *sqlcgen.Queries, p *preset.Preset) error {
	row, err := q.CreatePreset(ctx, p.Name)
	if err != nil {
		return err
	}
	mapPresetRowToDomain(&row, p)

	for i := range p.Plates {
		pp := &p.Plates[i]
		pp.PresetID = p.ID
		pp.SortOrder = i
		ppRow, err := q.CreatePresetPlate(ctx, sqlcgen.CreatePresetPlateParams{
			PresetID:  p.ID,
			SlotID:    pp.SlotID,
			SortOrder: int64(pp.SortOrder),
		})
		if err != nil {
			if isForeignKeyViolation(err) {
				return fmt.Errorf("%w: invalid slot reference", domain.ErrInvalidInput)
			}
			return err
		}
		pp.ID = ppRow.ID
		for j := range pp.Components {
			c := &pp.Components[j]
			c.PresetPlateID = pp.ID
			c.SortOrder = j
			cRow, err := q.CreatePresetComponent(ctx, presetComponentParams(c))
			if err != nil {
				if isForeignKeyViolation(err) {
					return fmt.Errorf("%w: invalid food reference", domain.ErrInvalidInput)
				}
				return err
			}
			mapPresetComponentRowToDomain(&cRow, c)
		}
	}

	for _, tag := range p.Tags {
		if err := q.AddPresetTag(ctx, sqlcgen.AddPresetTagParams{PresetID: p.ID, Tag: tag}); err != nil {
			return err
		}
	}
	return nil
}

func (r *PresetRepo) Get(ctx context.Context, id int64) (*preset.Preset, error) {
	row, err := r.q.GetPreset(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: preset %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var p preset.Preset
	mapPresetRowToDomain(&row, &p)
	if err := r.loadChildren(ctx, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PresetRepo) loadChildren(ctx context.Context, p *preset.Preset) error {
	plateRows, err := r.q.ListPresetPlatesByPreset(ctx, p.ID)
	if err != nil {
		return err
	}
	p.Plates = make([]preset.Plate, len(plateRows))
	for i := range plateRows {
		pp := &p.Plates[i]
		pp.ID = plateRows[i].ID
		pp.PresetID = plateRows[i].PresetID
		pp.SlotID = plateRows[i].SlotID
		pp.SortOrder = int(plateRows[i].SortOrder)
		compRows, err := r.q.ListPresetComponentsByPlate(ctx, pp.ID)
		if err != nil {
			return err
		}
		pp.Components = make([]preset.Component, len(compRows))
		for j := range compRows {
			mapPresetComponentRowToDomain(&compRows[j], &pp.Components[j])
		}
	}
	tags, err := r.q.ListPresetTagsByPreset(ctx, p.ID)
	if err != nil {
		return err
	}
	p.Tags = tags
	if p.Tags == nil {
		p.Tags = []string{}
	}
	return nil
}

// List implements name-search + slot-filter + tag-filter + sort + pagination
// via squirrel (sqlc can't express FTS5 MATCH).
func (r *PresetRepo) List(ctx context.Context, f preset.ListFilter) (*preset.ListResult, error) {
	idsBuilder := sq.Select("p.id").From("presets p")
	countBuilder := sq.Select("COUNT(DISTINCT p.id)").From("presets p")

	if f.Search != "" {
		// FTS5 prefix match: every token wrapped + suffixed with *.
		tokens := strings.Fields(sanitizeFTS5(f.Search))
		if len(tokens) > 0 {
			for i, t := range tokens {
				tokens[i] = t + "*"
			}
			ftsTerm := strings.Join(tokens, " ")
			ftsClause := "p.id IN (SELECT rowid FROM presets_fts WHERE presets_fts MATCH ?)"
			likeConds := sq.And{}
			for _, raw := range strings.Fields(f.Search) {
				likeConds = append(likeConds, sq.Expr(`p.name LIKE ? ESCAPE '\'`, "%"+escapeLike(raw)+"%"))
			}
			where := sq.Or{sq.Expr(ftsClause, ftsTerm)}
			if len(likeConds) > 0 {
				where = append(where, likeConds)
			}
			idsBuilder = idsBuilder.Where(where)
			countBuilder = countBuilder.Where(where)
		}
	}

	if len(f.SlotIDs) > 0 {
		args := make([]any, len(f.SlotIDs))
		for i, s := range f.SlotIDs {
			args[i] = s
		}
		placeholders := strings.Repeat("?,", len(f.SlotIDs))
		placeholders = placeholders[:len(placeholders)-1]
		clause := "p.id IN (SELECT preset_id FROM preset_plates WHERE slot_id IN (" + placeholders + "))"
		idsBuilder = idsBuilder.Where(clause, args...)
		countBuilder = countBuilder.Where(clause, args...)
	}

	// AND-semantic tag filtering: preset must carry every supplied tag.
	for _, t := range f.Tags {
		clause := "p.id IN (SELECT preset_id FROM preset_tags WHERE tag = ?)"
		idsBuilder = idsBuilder.Where(clause, preset.NormalizeTag(t))
		countBuilder = countBuilder.Where(clause, preset.NormalizeTag(t))
	}

	switch f.Sort {
	case preset.SortRecent:
		idsBuilder = idsBuilder.OrderBy("p.last_used_at IS NULL", "p.last_used_at DESC", "p.name COLLATE NOCASE", "p.id")
	default:
		idsBuilder = idsBuilder.OrderBy("p.name COLLATE NOCASE", "p.id")
	}
	if f.Limit > 0 {
		idsBuilder = idsBuilder.Limit(uint64(f.Limit))
	}
	if f.Offset > 0 {
		idsBuilder = idsBuilder.Offset(uint64(f.Offset))
	}

	countSQL, countArgs, err := countBuilder.PlaceholderFormat(sq.Question).ToSql()
	if err != nil {
		return nil, err
	}
	var total int
	if err := r.dbtx.QueryRowContext(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, err
	}

	listSQL, listArgs, err := idsBuilder.PlaceholderFormat(sq.Question).ToSql()
	if err != nil {
		return nil, err
	}
	rows, err := r.dbtx.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	items := make([]preset.Preset, 0, len(ids))
	for _, id := range ids {
		p, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, *p)
	}
	return &preset.ListResult{Items: items, Total: total}, nil
}

func (r *PresetRepo) UpdateName(ctx context.Context, id int64, name string) (*preset.Preset, error) {
	row, err := r.q.UpdatePresetName(ctx, sqlcgen.UpdatePresetNameParams{Name: name, ID: id})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: preset %d", domain.ErrNotFound, id)
		}
		return nil, err
	}
	var p preset.Preset
	mapPresetRowToDomain(&row, &p)
	if err := r.loadChildren(ctx, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PresetRepo) ReplacePlates(ctx context.Context, presetID int64, plates []preset.Plate) error {
	if r.db == nil {
		return r.replacePlatesWith(ctx, r.q, presetID, plates)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.replacePlatesWith(ctx, sqlcgen.New(tx), presetID, plates); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PresetRepo) replacePlatesWith(ctx context.Context, q *sqlcgen.Queries, presetID int64, plates []preset.Plate) error {
	if _, err := q.DeletePresetPlatesByPreset(ctx, presetID); err != nil {
		return err
	}
	for i := range plates {
		pp := &plates[i]
		pp.PresetID = presetID
		pp.SortOrder = i
		ppRow, err := q.CreatePresetPlate(ctx, sqlcgen.CreatePresetPlateParams{
			PresetID:  presetID,
			SlotID:    pp.SlotID,
			SortOrder: int64(pp.SortOrder),
		})
		if err != nil {
			if isForeignKeyViolation(err) {
				return fmt.Errorf("%w: invalid slot reference", domain.ErrInvalidInput)
			}
			return err
		}
		pp.ID = ppRow.ID
		for j := range pp.Components {
			c := &pp.Components[j]
			c.PresetPlateID = pp.ID
			c.SortOrder = j
			cRow, err := q.CreatePresetComponent(ctx, presetComponentParams(c))
			if err != nil {
				if isForeignKeyViolation(err) {
					return fmt.Errorf("%w: invalid food reference", domain.ErrInvalidInput)
				}
				return err
			}
			mapPresetComponentRowToDomain(&cRow, c)
		}
	}
	if err := q.TouchPresetUpdatedAt(ctx, presetID); err != nil {
		return err
	}
	return nil
}

func (r *PresetRepo) ReplaceTags(ctx context.Context, presetID int64, tags []string) error {
	if r.db == nil {
		return r.replaceTagsWith(ctx, r.q, presetID, tags)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.replaceTagsWith(ctx, sqlcgen.New(tx), presetID, tags); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *PresetRepo) replaceTagsWith(ctx context.Context, q *sqlcgen.Queries, presetID int64, tags []string) error {
	if err := q.DeletePresetTagsByPreset(ctx, presetID); err != nil {
		return err
	}
	for _, t := range tags {
		if err := q.AddPresetTag(ctx, sqlcgen.AddPresetTagParams{PresetID: presetID, Tag: t}); err != nil {
			return err
		}
	}
	if err := q.TouchPresetUpdatedAt(ctx, presetID); err != nil {
		return err
	}
	return nil
}

func (r *PresetRepo) AddTag(ctx context.Context, presetID int64, tag string) error {
	if err := r.q.AddPresetTag(ctx, sqlcgen.AddPresetTagParams{PresetID: presetID, Tag: tag}); err != nil {
		return err
	}
	return r.q.TouchPresetUpdatedAt(ctx, presetID)
}

func (r *PresetRepo) RemoveTag(ctx context.Context, presetID int64, tag string) error {
	if err := r.q.DeletePresetTag(ctx, sqlcgen.DeletePresetTagParams{PresetID: presetID, Tag: tag}); err != nil {
		return err
	}
	return r.q.TouchPresetUpdatedAt(ctx, presetID)
}

func (r *PresetRepo) TouchLastUsed(ctx context.Context, presetID int64) error {
	return r.q.TouchPresetLastUsedAt(ctx, presetID)
}

func (r *PresetRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.q.DeletePreset(ctx, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("%w: preset %d", domain.ErrNotFound, id)
	}
	return nil
}

func (r *PresetRepo) KnownTags(ctx context.Context, limit int) ([]preset.TagUsage, error) {
	rows, err := r.q.ListKnownTags(ctx, int64(limit))
	if err != nil {
		return nil, err
	}
	out := make([]preset.TagUsage, len(rows))
	for i, row := range rows {
		out[i] = preset.TagUsage{Tag: row.Tag, Count: row.UsageCount}
	}
	return out, nil
}

func (r *PresetRepo) CountUsingFood(ctx context.Context, foodID int64) (int64, error) {
	return r.q.CountPresetsUsingFood(ctx, foodID)
}

func presetComponentParams(c *preset.Component) sqlcgen.CreatePresetComponentParams {
	return sqlcgen.CreatePresetComponentParams{
		PresetPlateID: c.PresetPlateID,
		FoodID:        c.FoodID,
		Portions:      toNullInt64FromIntPtr(c.Portions),
		Amount:        toNullFloat(c.Amount),
		Unit:          toNullString(c.Unit),
		Grams:         toNullFloat(c.Grams),
		GramsSource:   toNullString(c.GramsSource),
		Note:          toNullString(c.Note),
		SortOrder:     int64(c.SortOrder),
	}
}

func mapPresetRowToDomain(row *sqlcgen.Preset, p *preset.Preset) {
	p.ID = row.ID
	p.Name = row.Name
	p.CreatedAt, _ = time.Parse(timeLayout, row.CreatedAt) //nolint:errcheck // controlled layout
	p.UpdatedAt, _ = time.Parse(timeLayout, row.UpdatedAt) //nolint:errcheck // controlled layout
	if row.LastUsedAt.Valid {
		t, err := time.Parse(timeLayout, row.LastUsedAt.String)
		if err == nil {
			p.LastUsedAt = &t
		}
	}
}

func mapPresetComponentRowToDomain(row *sqlcgen.PresetComponent, c *preset.Component) {
	c.ID = row.ID
	c.PresetPlateID = row.PresetPlateID
	c.FoodID = row.FoodID
	c.Portions = fromNullInt64ToIntPtr(row.Portions)
	c.Amount = fromNullFloat(row.Amount)
	c.Unit = fromNullString(row.Unit)
	c.Grams = fromNullFloat(row.Grams)
	c.GramsSource = fromNullString(row.GramsSource)
	c.Note = fromNullString(row.Note)
	c.SortOrder = int(row.SortOrder)
}
