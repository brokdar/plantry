package planning

import (
	"context"
	"encoding/json"
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

// maxLoggedViolations caps the violations one attempt line and step list.
const maxLoggedViolations = 20

// attemptRecord is what one chooser attempt leaves in the run log: the
// violations of a readable proposal, or the parse error of an unreadable
// reply. It is the result of the run's `plan.attempt` step.
type attemptRecord struct {
	Attempt    int      `json:"attempt"`
	Unreadable bool     `json:"unreadable,omitempty"`
	ParseError string   `json:"parse_error,omitempty"`
	Violations []string `json:"violations,omitempty"` // rule.code@date/slot plate session ids
	Count      int      `json:"violation_count"`
	// Autofix lists the corrections Go applied to the proposal before it
	// was checked (session portions grown to cover their plates).
	Autofix []string `json:"autofix,omitempty"`
}

// violationRef names one violation by rule, code and place:
// "H2.too_many_cooks@2026-11-02/slot3", with plate and session ids and the
// checker's parameters when it has them.
func violationRef(v Issue) string {
	s := v.Rule + "." + v.Code
	if v.Date != "" {
		s += "@" + v.Date
		if v.SlotID != nil {
			s += fmt.Sprintf("/slot%d", *v.SlotID)
		}
	}
	if v.PlateID != nil {
		s += fmt.Sprintf(" plate=%d", *v.PlateID)
	}
	if v.SessionID != nil {
		s += fmt.Sprintf(" session=%d", *v.SessionID)
	}
	if len(v.Params) > 0 {
		if b, err := json.Marshal(v.Params); err == nil {
			s += " " + string(b)
		}
	}
	return s
}

func newAttemptRecord(attempt int, violations []Issue, parseErr error, fixes []string) attemptRecord {
	rec := attemptRecord{Attempt: attempt, Count: len(violations), Autofix: fixes}
	if parseErr != nil {
		rec.Unreadable = true
		rec.ParseError = parseErr.Error()
		if len(rec.ParseError) > 300 {
			rec.ParseError = rec.ParseError[:300] + "…"
		}
	}
	for i, v := range violations {
		if i == maxLoggedViolations {
			break
		}
		rec.Violations = append(rec.Violations, violationRef(v))
	}
	return rec
}

// logAttempt logs one chooser attempt (attempt 0 is the first proposal, each
// later one a repair) on the service logger: the checker counts, the
// violations by rule.code@date/slot, or the parse error of an unreadable
// reply. It also stores the same as a `plan.attempt` step of the run, so a
// repair can be explained from the run alone.
func logAttempt(ctx context.Context, logger *slog.Logger, attempt int, violations []Issue, parseErr error, fixes ...string) {
	if logger == nil {
		logger = slog.Default()
	}
	rec := newAttemptRecord(attempt, violations, parseErr, fixes)
	attrs := append(runAttrs(ctx, 0), "attempt", attempt)
	if parseErr != nil {
		logger.WarnContext(ctx, "plan.run.attempt", append(attrs, "unreadable", true, "parse_error", rec.ParseError)...)
	} else {
		attrs = append(attrs, issueAttrs(violations)...)
		logger.InfoContext(ctx, "plan.run.attempt", append(attrs, "violation_list", strings.Join(rec.Violations, "; "))...)
	}
	args, _ := json.Marshal(map[string]any{"attempt": attempt})
	result, _ := json.Marshal(rec)
	airun.FromContext(ctx).ToolStep(ctx, "plan.attempt", args, result, parseErr != nil, 0)
}

// logRunStop logs why the LLM chooser ended a run on one of its request
// bounds (the same text goes to the run notes or the error).
func logRunStop(ctx context.Context, in ChooseInput, requests int, note string) {
	logger := in.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.WarnContext(ctx, "plan.run.stopped", append(runAttrs(ctx, 0), "requests", requests, "reason", note)...)
}
