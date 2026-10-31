package migrations

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upPlanState, downPlanState)
}

// planStateUp adds the Phase 3 D1 columns: draft/confirmed state on plates
// and cook sessions, the AI run that wrote a row and the chooser's reason
// ("Why this?"). Existing rows are the user's decisions, so the default marks
// them confirmed. It is a Go migration only because sqlc reads cook_sessions
// from schema_patches (00027), after db/migrations; sqlc reads the end state
// from schema_patches/00034_plan_state.sql.
var planStateUp = []string{
	`ALTER TABLE plates ADD COLUMN state TEXT NOT NULL DEFAULT 'confirmed' CHECK (state IN ('draft','confirmed'))`,
	`ALTER TABLE plates ADD COLUMN ai_run_id INTEGER NULL REFERENCES ai_runs(id) ON DELETE SET NULL`,
	`ALTER TABLE plates ADD COLUMN reason TEXT NULL`,
	`ALTER TABLE cook_sessions ADD COLUMN state TEXT NOT NULL DEFAULT 'confirmed' CHECK (state IN ('draft','confirmed'))`,
	`ALTER TABLE cook_sessions ADD COLUMN ai_run_id INTEGER NULL REFERENCES ai_runs(id) ON DELETE SET NULL`,
	`ALTER TABLE cook_sessions ADD COLUMN reason TEXT NULL`,
	`CREATE INDEX ix_plates_state_date ON plates (state, date)`,
	`CREATE INDEX ix_cook_sessions_ai_run ON cook_sessions (ai_run_id)`,
}

var planStateDown = []string{
	`DROP INDEX ix_cook_sessions_ai_run`,
	`DROP INDEX ix_plates_state_date`,
	`ALTER TABLE cook_sessions DROP COLUMN reason`,
	`ALTER TABLE cook_sessions DROP COLUMN ai_run_id`,
	`ALTER TABLE cook_sessions DROP COLUMN state`,
	`ALTER TABLE plates DROP COLUMN reason`,
	`ALTER TABLE plates DROP COLUMN ai_run_id`,
	`ALTER TABLE plates DROP COLUMN state`,
}

func upPlanState(ctx context.Context, tx *sql.Tx) error {
	return execPlanState(ctx, tx, planStateUp)
}

func downPlanState(ctx context.Context, tx *sql.Tx) error {
	return execPlanState(ctx, tx, planStateDown)
}

func execPlanState(ctx context.Context, tx *sql.Tx, stmts []string) error {
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("%s: %w", s, err)
		}
	}
	return nil
}
