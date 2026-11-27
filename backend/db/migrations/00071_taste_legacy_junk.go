package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
)

func init() {
	goose.AddMigrationContext(upTasteLegacyJunk, downTasteLegacyJunk)
}

// upTasteLegacyJunk removes the hero items 00068 made from imported recipe
// tags that name no food ("celebration", "christmas dinner"):
// CleanTasteLegacyJunk does the work.
func upTasteLegacyJunk(ctx context.Context, tx *sql.Tx) error {
	return CleanTasteLegacyJunk(ctx, tx)
}

// downTasteLegacyJunk restores nothing: the removed items were never taste.
func downTasteLegacyJunk(context.Context, *sql.Tx) error { return nil }

// CleanTasteLegacyJunk deletes each learned hero item whose only evidence is
// migrated signals and whose key names no leaf food (leafKeys), with those
// signals. An item with any other signal, or a statement, stays. Running it
// again changes nothing.
func CleanTasteLegacyJunk(ctx context.Context, tx *sql.Tx) error {
	foods, err := leafKeys(ctx, tx)
	if err != nil {
		return fmt.Errorf("00071: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT i.id, i.key FROM taste_items i
		WHERE i.level = ? AND i.source = 'learned' AND i.scope = 'always'
		  AND NOT EXISTS (SELECT 1 FROM taste_signals s WHERE s.level = i.level AND s.key = i.key AND s.kind <> ?)`,
		string(taste.LevelHero), string(taste.SignalMigrated))
	if err != nil {
		return fmt.Errorf("00071: migrated hero items: %w", err)
	}
	junk := map[int64]string{}
	for rows.Next() {
		var id int64
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			_ = rows.Close()
			return fmt.Errorf("00071: migrated hero items: %w", err)
		}
		if !foods[key] {
			junk[id] = key
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("00071: migrated hero items: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("00071: migrated hero items: %w", err)
	}
	for id, key := range junk {
		if _, err := tx.ExecContext(ctx, `DELETE FROM taste_signals WHERE level = ? AND key = ? AND kind = ?`,
			string(taste.LevelHero), key, string(taste.SignalMigrated)); err != nil {
			return fmt.Errorf("00071: delete signals: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM taste_items WHERE id = ?`, id); err != nil {
			return fmt.Errorf("00071: delete item: %w", err)
		}
	}
	return nil
}
