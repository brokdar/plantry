package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/sqlite/sqlcgen"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// maxStepResultBytes caps the stored size of a step's args_json and result_json.
const maxStepResultBytes = 8192

const truncatedMarker = "…[truncated]"

// AIRunRepo implements airun.Store backed by SQLite.
type AIRunRepo struct {
	conn
}

// NewAIRunRepo creates a SQLite-backed AI run store.
func NewAIRunRepo(db *sql.DB) *AIRunRepo {
	return &AIRunRepo{conn: conn{db: db}}
}

var (
	_ airun.Store       = (*AIRunRepo)(nil)
	_ plate.RunOutcomes = (*AIRunRepo)(nil)
)

// Record moves the run's outcome by ev (D25; the transitions are in the
// SetRunOutcome query) and stamps outcome_at.
func (r *AIRunRepo) Record(ctx context.Context, runID int64, ev plate.OutcomeEvent) error {
	switch ev {
	case plate.OutcomeAccepted, plate.OutcomeDiscarded, plate.OutcomeReset:
	default:
		return fmt.Errorf("record run outcome: unknown event %q", ev)
	}
	if err := r.queriesFor(ctx).SetRunOutcome(ctx, sqlcgen.SetRunOutcomeParams{Event: string(ev), ID: runID}); err != nil {
		return fmt.Errorf("record run %d outcome: %w", runID, err)
	}
	return nil
}

func (r *AIRunRepo) CreateRun(ctx context.Context, start airun.RunStart) (int64, error) {
	id, err := r.queriesFor(ctx).CreateAIRun(ctx, sqlcgen.CreateAIRunParams{
		Kind:           string(start.Kind),
		StartedAt:      formatTime(start.StartedAt),
		PromptVersion:  start.PromptVersion,
		WindowStart:    toNullDate(start.WindowStart),
		WindowEnd:      toNullDate(start.WindowEnd),
		Instruction:    toNullString(start.Instruction),
		ConversationID: toNullInt64(start.ConversationID),
		RequestID:      toNullStringEmpty(start.RequestID),
	})
	if err != nil {
		return 0, fmt.Errorf("create ai run: %w", err)
	}
	return id, nil
}

func (r *AIRunRepo) FinishRun(ctx context.Context, end airun.RunEnd) error {
	res, err := r.queriesFor(ctx).FinishAIRun(ctx, sqlcgen.FinishAIRunParams{
		Status:           string(end.Status),
		FinishedAt:       sql.NullString{String: formatTime(end.FinishedAt), Valid: true},
		Provider:         end.Provider,
		Model:            end.Model,
		Hash:             end.PromptHash,
		Iterations:       int64(end.Iterations),
		InputTokens:      end.InputTokens,
		OutputTokens:     end.OutputTokens,
		CacheReadTokens:  end.CacheReadTokens,
		CacheWriteTokens: end.CacheWriteTokens,
		ErrorKey:         toNullStringEmpty(end.ErrorKey),
		ID:               end.ID,
	})
	if err != nil {
		return fmt.Errorf("finish ai run %d: %w", end.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("finish ai run %d: %w", end.ID, err)
	}
	if n == 0 {
		return fmt.Errorf("%w: ai run %d", domain.ErrNotFound, end.ID)
	}
	return nil
}

func (r *AIRunRepo) AddStep(ctx context.Context, step airun.Step) error {
	var isErr int64
	if step.IsError {
		isErr = 1
	}
	err := r.queriesFor(ctx).CreateAIRunStep(ctx, sqlcgen.CreateAIRunStepParams{
		RunID:        step.RunID,
		Seq:          int64(step.Seq),
		Type:         string(step.Type),
		Name:         step.Name,
		ArgsJson:     toNullStringEmpty(truncateUTF8(step.ArgsJSON, maxStepResultBytes)),
		ResultJson:   toNullStringEmpty(truncateUTF8(step.ResultJSON, maxStepResultBytes)),
		IsError:      isErr,
		DurationMs:   step.DurationMs,
		InputTokens:  step.InputTokens,
		OutputTokens: step.OutputTokens,
		RequestJson:  toNullStringEmpty(step.RequestJSON),
		ResponseJson: toNullStringEmpty(step.ResponseJSON),
		StopReason:   toNullStringEmpty(step.StopReason),
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("add ai run step: %w: ai run %d", domain.ErrInvalidInput, step.RunID)
		}
		return fmt.Errorf("add ai run step %d/%d: %w", step.RunID, step.Seq, err)
	}
	return nil
}

func (r *AIRunRepo) UpsertPrompt(ctx context.Context, hash, text string) error {
	if err := r.queriesFor(ctx).UpsertAIPrompt(ctx, sqlcgen.UpsertAIPromptParams{Hash: hash, Text: text}); err != nil {
		return fmt.Errorf("upsert ai prompt: %w", err)
	}
	return nil
}

func (r *AIRunRepo) UpsertToolSchema(ctx context.Context, hash, json string) error {
	if err := r.queriesFor(ctx).UpsertAIToolSchema(ctx, sqlcgen.UpsertAIToolSchemaParams{Hash: hash, Json: json}); err != nil {
		return fmt.Errorf("upsert ai tool schema: %w", err)
	}
	return nil
}

// PruneSteps deletes the steps of runs started before the cutoff in batches
// and returns how many were deleted in total.
func (r *AIRunRepo) PruneSteps(ctx context.Context, before time.Time) (int64, error) {
	var total int64
	for {
		res, err := r.queriesFor(ctx).PruneAIRunSteps(ctx, formatTime(before))
		if err != nil {
			return total, fmt.Errorf("prune ai run steps: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, fmt.Errorf("prune ai run steps: %w", err)
		}
		if n == 0 {
			return total, nil
		}
		total += n
	}
}

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func toNullDate(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: t.Format(dateLayout), Valid: true}
}

func toNullStringEmpty(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// truncateUTF8 cuts s to at most limit bytes on a rune boundary and appends
// truncatedMarker. Strings within the limit are returned unchanged.
func truncateUTF8(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + truncatedMarker
}
