package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
)

func init() {
	goose.AddMigrationNoTxContext(upAIRunsCost, downAIRunsCost)
}

// upAIRunsCost rebuilds ai_runs without the kind CHECK (airun.Kind.Valid
// guards the kind from now on, so new kinds need no table rebuild) and adds
// cost_usd, the run's cost in USD at the time of the run (NULL while running
// or when the model has no price). Finished runs are priced once here from
// the built-in table plus ai.price_overrides as of this migration.
//
// The rebuild follows backend/CLAUDE.md "Safe table rebuild": with foreign
// keys on, DROP TABLE ai_runs would cascade-delete ai_run_steps and
// ai_changes and null plates.ai_run_id, cook_sessions.ai_run_id and
// recipe_suggestions.ai_run_id. Children keep their textual REFERENCES
// ai_runs(id), which bind to the rebuilt table. sqlc reads the new column
// from schema_patches/00055_ai_runs_cost.sql.
func upAIRunsCost(ctx context.Context, db *sql.DB) (err error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("00055: pin connection: %w", err)
	}
	defer func() { err = errors.Join(err, conn.Close()) }()

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("00055: foreign keys off: %w", err)
	}
	defer func() {
		if _, onErr := conn.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys = ON`); onErr != nil {
			err = errors.Join(err, fmt.Errorf("00055: foreign keys on: %w", onErr))
		}
	}()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("00055: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, s := range aiRunsCostUp {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("00055: %w", err)
		}
	}
	if err := backfillRunCosts(ctx, tx); err != nil {
		return fmt.Errorf("00055: %w", err)
	}
	if err := checkForeignKeys(ctx, tx, "00055"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("00055: commit: %w", err)
	}
	return nil
}

const aiRunsColumns = `id, kind, started_at, finished_at, status, provider, model, prompt_version, prompt_hash,
	window_start, window_end, scope_json, instruction, chooser, iterations, input_tokens, output_tokens,
	cache_read_tokens, cache_write_tokens, repair_attempts, violations_json, score_json, judge_json, outcome,
	outcome_at, error_key, conversation_id, request_id, notes, web_search_requests, duration_ms,
	reasoning_tokens, app_version, container_id`

var aiRunsCostUp = []string{
	`CREATE TABLE ai_runs_new (
		id                  INTEGER PRIMARY KEY,
		kind                TEXT NOT NULL,
		started_at          TEXT NOT NULL,
		finished_at         TEXT,
		status              TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running','ok','error','iterations_exceeded','cancelled','partial')),
		provider            TEXT NOT NULL DEFAULT '',
		model               TEXT NOT NULL DEFAULT '',
		prompt_version      TEXT NOT NULL DEFAULT '',
		prompt_hash         TEXT REFERENCES ai_prompts(hash),
		window_start        TEXT,
		window_end          TEXT,
		scope_json          TEXT,
		instruction         TEXT,
		chooser             TEXT CHECK (chooser IS NULL OR chooser IN ('llm','heuristic')),
		iterations          INTEGER NOT NULL DEFAULT 0,
		input_tokens        INTEGER NOT NULL DEFAULT 0,
		output_tokens       INTEGER NOT NULL DEFAULT 0,
		cache_read_tokens   INTEGER NOT NULL DEFAULT 0,
		cache_write_tokens  INTEGER NOT NULL DEFAULT 0,
		repair_attempts     INTEGER NOT NULL DEFAULT 0,
		violations_json     TEXT,
		score_json          TEXT,
		judge_json          TEXT,
		outcome             TEXT CHECK (outcome IS NULL OR outcome IN ('accepted','discarded','partially_accepted','open')),
		outcome_at          TEXT,
		error_key           TEXT,
		conversation_id     INTEGER,
		request_id          TEXT,
		notes               TEXT,
		web_search_requests INTEGER NOT NULL DEFAULT 0,
		duration_ms         INTEGER,
		reasoning_tokens    INTEGER NOT NULL DEFAULT 0,
		app_version         TEXT,
		container_id        TEXT,
		cost_usd            REAL
	)`,
	`INSERT INTO ai_runs_new (` + aiRunsColumns + `) SELECT ` + aiRunsColumns + ` FROM ai_runs`,
	`DROP TABLE ai_runs`,
	`ALTER TABLE ai_runs_new RENAME TO ai_runs`,
	`CREATE INDEX ix_ai_runs_started ON ai_runs(started_at)`,
	`CREATE INDEX ix_ai_runs_conversation ON ai_runs(conversation_id)`,
}

// backfillRunCosts prices every finished run as the recorder now does on
// write. An unreadable ai.price_overrides value counts as none.
func backfillRunCosts(ctx context.Context, tx *sql.Tx) error {
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT value FROM app_settings WHERE key = 'ai.price_overrides'`).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read price overrides: %w", err)
	}
	overrides, err := airun.ParsePriceOverrides(raw)
	if err != nil {
		overrides = nil
	}

	rows, err := tx.QueryContext(ctx, `
SELECT id, model, provider, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, web_search_requests
FROM ai_runs WHERE status <> 'running'`)
	if err != nil {
		return fmt.Errorf("read runs: %w", err)
	}
	type priced struct {
		id   int64
		cost float64
	}
	var costs []priced
	for rows.Next() {
		var (
			id              int64
			model, provider string
			t               airun.Tokens
		)
		if err := rows.Scan(&id, &model, &provider, &t.Input, &t.Output, &t.CacheRead, &t.CacheWrite, &t.WebSearchRequests); err != nil {
			_ = rows.Close()
			return err
		}
		if c := airun.Cost(model, provider, t, overrides); c != nil {
			costs = append(costs, priced{id, *c})
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, c := range costs {
		if _, err := tx.ExecContext(ctx, `UPDATE ai_runs SET cost_usd = ? WHERE id = ?`, c.cost, c.id); err != nil {
			return fmt.Errorf("set cost of run %d: %w", c.id, err)
		}
	}
	return nil
}

// downAIRunsCost drops cost_usd. The kind CHECK is not restored: rows of the
// newer kinds would violate it.
func downAIRunsCost(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `ALTER TABLE ai_runs DROP COLUMN cost_usd`); err != nil {
		return fmt.Errorf("00055 down: %w", err)
	}
	return nil
}
