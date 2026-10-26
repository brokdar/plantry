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
)

// maxStepResultBytes caps the stored size of a step's result_json.
const maxStepResultBytes = 8192

const truncatedMarker = "…[truncated]"

// AIRunRepo implements airun.Store backed by SQLite.
type AIRunRepo struct {
	q *sqlcgen.Queries
}

// NewAIRunRepo creates a SQLite-backed AI run store.
func NewAIRunRepo(db *sql.DB) *AIRunRepo {
	return &AIRunRepo{q: sqlcgen.New(db)}
}

var _ airun.Store = (*AIRunRepo)(nil)

func (r *AIRunRepo) CreateRun(ctx context.Context, start airun.RunStart) (int64, error) {
	return r.q.CreateAIRun(ctx, sqlcgen.CreateAIRunParams{
		Kind:          string(start.Kind),
		StartedAt:     formatTime(start.StartedAt),
		PromptVersion: start.PromptVersion,
		WindowStart:   toNullDate(start.WindowStart),
		WindowEnd:     toNullDate(start.WindowEnd),
		Instruction:   toNullString(start.Instruction),
	})
}

func (r *AIRunRepo) FinishRun(ctx context.Context, end airun.RunEnd) error {
	res, err := r.q.FinishAIRun(ctx, sqlcgen.FinishAIRunParams{
		Status:           string(end.Status),
		FinishedAt:       sql.NullString{String: formatTime(end.FinishedAt), Valid: true},
		Provider:         end.Provider,
		Model:            end.Model,
		PromptHash:       toNullStringEmpty(end.PromptHash),
		Iterations:       int64(end.Iterations),
		InputTokens:      end.InputTokens,
		OutputTokens:     end.OutputTokens,
		CacheReadTokens:  end.CacheReadTokens,
		CacheWriteTokens: end.CacheWriteTokens,
		ErrorKey:         toNullStringEmpty(end.ErrorKey),
		ID:               end.ID,
	})
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("%w: prompt reference", domain.ErrInvalidInput)
		}
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
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
	err := r.q.CreateAIRunStep(ctx, sqlcgen.CreateAIRunStepParams{
		RunID:        step.RunID,
		Seq:          int64(step.Seq),
		Type:         string(step.Type),
		Name:         step.Name,
		ArgsJson:     toNullStringEmpty(step.ArgsJSON),
		ResultJson:   toNullStringEmpty(truncateUTF8(step.ResultJSON, maxStepResultBytes)),
		IsError:      isErr,
		DurationMs:   step.DurationMs,
		InputTokens:  step.InputTokens,
		OutputTokens: step.OutputTokens,
	})
	if err != nil && isForeignKeyViolation(err) {
		return fmt.Errorf("%w: ai run %d", domain.ErrInvalidInput, step.RunID)
	}
	return err
}

func (r *AIRunRepo) UpsertPrompt(ctx context.Context, hash, text string) error {
	return r.q.UpsertAIPrompt(ctx, sqlcgen.UpsertAIPromptParams{Hash: hash, Text: text})
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
