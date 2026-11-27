package migrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/taste"
)

func init() {
	goose.AddMigrationContext(upTasteLegacy, downTasteLegacy)
}

// upTasteLegacy carries the old profile taste data into the taste model
// (TASTE-10); MigrateTasteLegacy does the work.
func upTasteLegacy(ctx context.Context, tx *sql.Tx) error {
	return MigrateTasteLegacy(ctx, tx, time.Now().UTC())
}

// downTasteLegacy leaves the copied data: 00067's down drops the tables, and
// the never items are the user's own once made.
func downTasteLegacy(context.Context, *sql.Tx) error { return nil }

// MigrateTasteLegacy moves the profile's restriction chips that name a food
// (case-insensitively) to never items, and its likes and dislikes tags to
// weak evidence: one migrated signal of taste.WeightMigrated per tag on the
// item taste.Classify names, below the learned threshold on its own. Only a
// tag that is a label (food.IsLabel) and names a cuisine, technique, format
// or taste word of the vocabularies, or a food of the catalogue (a hero is
// a food's name), becomes an item: imported recipe tags such as
// "celebration" or "chili con carne rezepte" name no food. The
// chips and tags stay on the profile (the prompts still read them). It is
// idempotent: a tag that already has a migrated signal is skipped.
func MigrateTasteLegacy(ctx context.Context, tx *sql.Tx, now time.Time) error {
	var restrictions, preferences string
	err := tx.QueryRowContext(ctx, `SELECT dietary_restrictions, preferences FROM user_profile WHERE id = 1`).Scan(&restrictions, &preferences)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("00068: read profile: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO never_items (food_id)
		SELECT f.id FROM foods f, json_each(?) r
		WHERE lower(trim(f.name)) = lower(trim(r.value))
		ON CONFLICT DO NOTHING`, restrictions); err != nil {
		return fmt.Errorf("00068: never items: %w", err)
	}
	var prefs map[string]any
	if err := json.Unmarshal([]byte(preferences), &prefs); err != nil {
		return fmt.Errorf("00068: preferences: %w", err)
	}
	foods, err := leafKeys(ctx, tx)
	if err != nil {
		return err
	}
	at := now.Format("2006-01-02 15:04:05")
	for key, sign := range map[string]float64{"likes": 1, "dislikes": -1} {
		tags, _ := prefs[key].([]any)
		for _, t := range tags {
			tag, ok := t.(string)
			if !ok || !food.IsLabel(tag) {
				continue
			}
			ref := taste.Classify(strings.ReplaceAll(tag, "_", " "))
			if ref.Key == "" || (ref.Level == taste.LevelHero && !foods[ref.Key]) {
				continue
			}
			if err := migrateTag(ctx, tx, ref, sign*taste.WeightMigrated, at); err != nil {
				return err
			}
		}
	}
	return nil
}

func migrateTag(ctx context.Context, tx *sql.Tx, ref taste.Ref, w float64, at string) error {
	var seen int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM taste_signals WHERE kind = ? AND level = ? AND key = ? AND weight = ?`,
		string(taste.SignalMigrated), string(ref.Level), ref.Key, w).Scan(&seen); err != nil {
		return fmt.Errorf("00068: migrated signal: %w", err)
	}
	if seen > 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO taste_signals (kind, level, key, weight, share, at) VALUES (?, ?, ?, ?, 1, ?)`,
		string(taste.SignalMigrated), string(ref.Level), ref.Key, w, at); err != nil {
		return fmt.Errorf("00068: add signal: %w", err)
	}
	pos, posN, neg, negN := w, 1.0, 0.0, 0.0
	if w < 0 {
		pos, posN, neg, negN = 0, 0, w, 1
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO taste_items (level, key, scope, window_start, source, pos_evidence, pos_n, neg_evidence, neg_n, created_at, updated_at)
		VALUES (?, ?, 'always', '', 'learned', ?, ?, ?, ?, ?, ?)
		ON CONFLICT (level, key, scope, window_start) DO UPDATE SET
			pos_evidence = pos_evidence + excluded.pos_evidence, pos_n = pos_n + excluded.pos_n,
			neg_evidence = neg_evidence + excluded.neg_evidence, neg_n = neg_n + excluded.neg_n`,
		string(ref.Level), ref.Key, pos, posN, neg, negN, at, at); err != nil {
		return fmt.Errorf("00068: add item: %w", err)
	}
	return nil
}

// leafKeys are the item keys (taste.NormKey) of the catalogue's foods that a
// hero can name: the leaf foods, as recipe attributes name their heroes.
func leafKeys(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM foods WHERE kind = 'leaf'`)
	if err != nil {
		return nil, fmt.Errorf("leaf foods: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("leaf foods: %w", err)
		}
		out[taste.NormKey(name)] = true
	}
	return out, rows.Err()
}
