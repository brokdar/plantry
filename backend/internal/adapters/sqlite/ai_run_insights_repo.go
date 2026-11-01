package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
)

var _ airun.InsightsStore = (*AIRunRepo)(nil)

// ListRuns lists runs newest first (one query).
func (r *AIRunRepo) ListRuns(ctx context.Context, q airun.RunQuery) ([]airun.RunRow, error) {
	params := sqlcgen.ListAIRunsParams{
		Kind:   toNullStringEmpty(string(q.Kind)),
		FromTs: optTime(q.From),
		ToTs:   optTime(q.To),
		Lim:    int64(q.Limit),
	}
	if q.BeforeID > 0 {
		params.BeforeID = sql.NullInt64{Int64: q.BeforeID, Valid: true}
	}
	rows, err := r.queriesFor(ctx).ListAIRuns(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list ai runs: %w", err)
	}
	out := make([]airun.RunRow, len(rows))
	for i, row := range rows {
		out[i], err = runRow(sqlcgen.GetAIRunDetailRow{
			ID: row.ID, Kind: row.Kind, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, Status: row.Status,
			Provider: row.Provider, Model: row.Model, PromptVersion: row.PromptVersion, Chooser: row.Chooser,
			InputTokens: row.InputTokens, OutputTokens: row.OutputTokens,
			CacheReadTokens: row.CacheReadTokens, CacheWriteTokens: row.CacheWriteTokens,
			ScoreJson: row.ScoreJson, ViolationsJson: row.ViolationsJson, Outcome: row.Outcome,
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// GetRun reads one run with its stored prompt text.
func (r *AIRunRepo) GetRun(ctx context.Context, id int64) (airun.RunDetailRow, error) {
	row, err := r.queriesFor(ctx).GetAIRunDetail(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return airun.RunDetailRow{}, fmt.Errorf("%w: ai run %d", domain.ErrNotFound, id)
	}
	if err != nil {
		return airun.RunDetailRow{}, fmt.Errorf("get ai run %d: %w", id, err)
	}
	run, err := runRow(row)
	if err != nil {
		return airun.RunDetailRow{}, err
	}
	return airun.RunDetailRow{
		RunRow:      run,
		ScopeJSON:   row.ScopeJson.String,
		Instruction: row.Instruction.String,
		Notes:       row.Notes.String,
		ErrorKey:    row.ErrorKey.String,
		PromptText:  row.PromptText.String,
	}, nil
}

// ListSteps reads a run's steps in seq order, args and results cut to 8192
// characters.
func (r *AIRunRepo) ListSteps(ctx context.Context, runID int64) ([]airun.StepView, error) {
	rows, err := r.queriesFor(ctx).ListAIRunStepViews(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("list ai run steps %d: %w", runID, err)
	}
	out := make([]airun.StepView, len(rows))
	for i, row := range rows {
		out[i] = airun.StepView{
			Seq:             int(row.Seq),
			Type:            airun.StepType(row.Type),
			Name:            row.Name,
			Args:            row.Args,
			ArgsTruncated:   row.ArgsCut || hasTruncatedMarker(row.Args),
			Result:          row.Result,
			ResultTruncated: row.ResultCut || hasTruncatedMarker(row.Result),
			IsError:         row.IsError != 0,
			DurationMs:      row.DurationMs,
			InputTokens:     row.InputTokens,
			OutputTokens:    row.OutputTokens,
		}
	}
	return out, nil
}

// Corrections computes D24 for the runs with ids in [minID, maxID] (one query).
func (r *AIRunRepo) Corrections(ctx context.Context, minID, maxID int64) (map[int64]airun.Correction, error) {
	rows, err := r.queriesFor(ctx).AIRunCorrections(ctx, sqlcgen.AIRunCorrectionsParams{
		MinID: sql.NullInt64{Int64: minID, Valid: true},
		MaxID: sql.NullInt64{Int64: maxID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("ai run corrections: %w", err)
	}
	out := make(map[int64]airun.Correction, len(rows))
	for _, row := range rows {
		out[row.RunID] = airun.Correction{Generated: int(row.GeneratedPlates), Corrected: int(row.CorrectedPlates)}
	}
	return out, nil
}

// editsSQL is hand-written because sqlc cannot resolve json_each's value
// column. D12: the rows an agent run edited. A session edit counts once as
// its session (its linked plates' events carry cook_session_id and are
// skipped); every other plate counts as itself. A chat run also owns the
// events of the runs its changes link to (ai_changes.run_ids_json, e.g. the
// regenerate run it waited for; the generator wrote those) within the
// change's event range. Args: min, max, min, max.
const editsSQL = `SELECT run_id, COUNT(DISTINCT k) FROM (
    SELECT ai_run_id AS run_id,
           CASE WHEN plate_id IS NULL THEN 's' || cook_session_id
                WHEN cook_session_id IS NULL THEN 'p' || plate_id END AS k
    FROM plate_events
    WHERE actor = 'agent' AND ai_run_id >= ? AND ai_run_id <= ?
  UNION ALL
    SELECT c.run_id,
           CASE WHEN e.plate_id IS NULL THEN 's' || e.cook_session_id
                WHEN e.cook_session_id IS NULL THEN 'p' || e.plate_id END
    FROM ai_changes c
    JOIN json_each(c.run_ids_json) j ON j.value <> c.run_id
    JOIN plate_events e ON e.ai_run_id = j.value
         AND e.id >= c.first_event_id AND e.id <= c.last_event_id
    WHERE c.run_id >= ? AND c.run_id <= ? AND e.actor IN ('generator', 'agent')
) GROUP BY run_id`

// Edits counts the rows each agent run edited for the runs with ids in
// [minID, maxID] (one query); runs without agent events are absent.
func (r *AIRunRepo) Edits(ctx context.Context, minID, maxID int64) (map[int64]int, error) {
	rows, err := r.dbtx(ctx).QueryContext(ctx, editsSQL, minID, maxID, minID, maxID)
	if err != nil {
		return nil, fmt.Errorf("ai run edits: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("scan ai run edits: %w", err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ai run edits: %w", err)
	}
	return out, nil
}

func runRow(row sqlcgen.GetAIRunDetailRow) (airun.RunRow, error) {
	started, err := time.Parse(timeLayout, row.StartedAt)
	if err != nil {
		return airun.RunRow{}, fmt.Errorf("ai run %d started_at: %w", row.ID, err)
	}
	run := airun.RunRow{
		ID:            row.ID,
		Kind:          airun.Kind(row.Kind),
		StartedAt:     started.UTC(),
		Status:        airun.Status(row.Status),
		Provider:      row.Provider,
		Model:         row.Model,
		PromptVersion: row.PromptVersion,
		Chooser:       airun.Chooser(row.Chooser.String),
		Tokens: airun.Tokens{
			Input: row.InputTokens, Output: row.OutputTokens,
			CacheRead: row.CacheReadTokens, CacheWrite: row.CacheWriteTokens,
		},
		ScoreJSON:      row.ScoreJson.String,
		ViolationsJSON: row.ViolationsJson.String,
		Outcome:        row.Outcome.String,
	}
	if row.FinishedAt.Valid {
		f, err := time.Parse(timeLayout, row.FinishedAt.String)
		if err != nil {
			return airun.RunRow{}, fmt.Errorf("ai run %d finished_at: %w", row.ID, err)
		}
		f = f.UTC()
		run.FinishedAt = &f
	}
	return run, nil
}

func optTime(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: formatTime(*t), Valid: true}
}

// hasTruncatedMarker reports a tool payload AddStep already cut.
func hasTruncatedMarker(s string) bool {
	return len(s) >= len(truncatedMarker) && s[len(s)-len(truncatedMarker):] == truncatedMarker
}
