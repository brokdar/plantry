package planning

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
)

// runAttrs are the attributes of a run's log lines (run_id, request_id): the
// run in ctx supplies them, else just the id.
func runAttrs(ctx context.Context, runID int64) []any {
	if attrs := airun.FromContext(ctx).LogAttrs(); attrs != nil {
		return attrs
	}
	return []any{"run_id", runID}
}

// logStage logs a stage transition of a generation run with the time since
// the run began. Heuristic and LLM runs both pass through it.
func (s *Service) logStage(ctx context.Context, runID int64, started time.Time, st Stage, extra ...any) {
	attrs := append(runAttrs(ctx, runID), "stage", st, "elapsed_ms", time.Since(started).Milliseconds())
	s.d.Logger.InfoContext(ctx, "plan.run.stage", append(attrs, extra...)...)
}

// issueAttrs summarises a checker result for the log: issue counts overall
// and the violations by rule and by rule.code.
func issueAttrs(issues []Issue) []any {
	var violations, warnings int
	byRule, byCode := map[string]int{}, map[string]int{}
	for _, is := range issues {
		if is.Severity != SeverityViolation {
			warnings++
			continue
		}
		violations++
		byRule[is.Rule]++
		byCode[is.Rule+"."+is.Code]++
	}
	return []any{
		"issues", len(issues), "violations", violations, "warnings", warnings,
		"by_rule", countList(byRule), "by_code", countList(byCode),
	}
}

func countList(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, m[k])
	}
	return strings.Join(parts, ",")
}

// logAttempt logs the checker result of one chooser attempt (attempt 0 is
// the first proposal, each later one a repair).
func logAttempt(ctx context.Context, attempt int, violations []Issue) {
	attrs := append(runAttrs(ctx, 0), "attempt", attempt)
	slog.InfoContext(ctx, "plan.run.attempt", append(attrs, issueAttrs(violations)...)...)
}
