package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
)

// AppSettingsRepo implements settings.Repository backed by SQLite.
type AppSettingsRepo struct {
	conn
}

// NewAppSettingsRepo creates a SQLite-backed repo.
func NewAppSettingsRepo(db *sql.DB) *AppSettingsRepo {
	return &AppSettingsRepo{conn: conn{db: db}}
}

func (r *AppSettingsRepo) Get(ctx context.Context, key string) (settings.Row, bool, error) {
	row, err := r.queriesFor(ctx).GetSetting(ctx, key)
	if errors.Is(err, sql.ErrNoRows) {
		return settings.Row{}, false, nil
	}
	if err != nil {
		return settings.Row{}, false, err
	}
	return settings.Row{
		Key:       row.Key,
		Value:     row.Value,
		Encrypted: row.Encrypted != 0,
	}, true, nil
}

func (r *AppSettingsRepo) List(ctx context.Context) ([]settings.Row, error) {
	rows, err := r.queriesFor(ctx).ListSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]settings.Row, len(rows))
	for i, row := range rows {
		out[i] = settings.Row{
			Key:       row.Key,
			Value:     row.Value,
			Encrypted: row.Encrypted != 0,
		}
	}
	return out, nil
}

func (r *AppSettingsRepo) Upsert(ctx context.Context, row settings.Row) error {
	enc := int64(0)
	if row.Encrypted {
		enc = 1
	}
	return r.queriesFor(ctx).UpsertSetting(ctx, sqlcgen.UpsertSettingParams{
		Key:       row.Key,
		Value:     row.Value,
		Encrypted: enc,
	})
}

func (r *AppSettingsRepo) Delete(ctx context.Context, key string) error {
	return r.queriesFor(ctx).DeleteSetting(ctx, key)
}
