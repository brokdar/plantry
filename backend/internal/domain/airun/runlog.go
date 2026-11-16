package airun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// debugBodyChars is how much of a request or response body the debug line
// ai.llm.body carries.
const debugBodyChars = 2000

// logLLMCall logs one finished LLM call as ai.llm.call: no prompt text, no
// key; sizes only. The bodies themselves go to the debug line ai.llm.body,
// shortened.
func (run *Run) logLLMCall(ctx context.Context, provider string, req llm.Request, step Step, callErr error) {
	attrs := append(run.LogAttrs(),
		"kind", run.kind,
		"step_seq", step.Seq,
		"provider", provider,
		"model", req.Model,
		"served_model", step.ServedModel,
		"duration_ms", step.DurationMs,
		"ttft_ms", ptrVal(step.TTFTMs),
		"input_tokens", step.InputTokens,
		"output_tokens", step.OutputTokens,
		"cache_read_tokens", step.CacheReadTokens,
		"cache_write_tokens", step.CacheWriteTokens,
		"reasoning_tokens", step.ReasoningTokens,
		"thinking_chars", step.ThinkingChars,
		"stop_reason", step.StopReason,
		"tool_choice", toolChoiceName(req.ToolChoice),
		"max_tokens", requestMaxTokens(step.RequestJSON, req.MaxTokens),
		"request_bytes", len(step.RequestJSON),
		"response_bytes", len(step.ResponseJSON),
	)
	level := run.rec.logger.InfoContext
	if callErr != nil {
		attrs = append(attrs, "error", errorSummary(callErr))
		var pe llm.ProviderError
		if errors.As(callErr, &pe) {
			attrs = append(attrs, "provider_status", pe.ProviderStatus(), "provider_error_type", pe.ProviderErrorType())
		}
		level = run.rec.logger.WarnContext
	}
	level(ctx, "ai.llm.call", attrs...)
	run.rec.logger.DebugContext(ctx, "ai.llm.body", append(run.LogAttrs(),
		"step_seq", step.Seq,
		"request", cutChars(step.RequestJSON, debugBodyChars),
		"response", cutChars(step.ResponseJSON, debugBodyChars),
	)...)
}

func ptrVal(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func toolChoiceName(tc *llm.ToolChoice) string {
	if tc == nil {
		return ""
	}
	return tc.Name
}

// requestMaxTokens reads the effective max_tokens back from the stored body.
func requestMaxTokens(requestJSON string, fallback int) int {
	var b struct {
		MaxTokens int `json:"max_tokens"`
	}
	if json.Unmarshal([]byte(requestJSON), &b) == nil && b.MaxTokens > 0 {
		return b.MaxTokens
	}
	return fallback
}

// errorSummary is the error text without anything that could carry the key
// or the prompt: provider errors give status and type only.
func errorSummary(err error) string {
	var pe llm.ProviderError
	if errors.As(err, &pe) {
		return fmt.Sprintf("%d %s", pe.ProviderStatus(), pe.ProviderErrorType())
	}
	return cutChars(err.Error(), 200)
}

func cutChars(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return cutUTF8(s, n) + cutMarker
}

// thinkingChars sums the thinking text length of the reply's thinking blocks
// (Anthropic reports no separate thinking token count).
func thinkingChars(blocks []llm.ContentBlock) int64 {
	var n int64
	for _, b := range blocks {
		if len(b.Raw) == 0 {
			continue
		}
		var raw struct {
			Type     string `json:"type"`
			Thinking string `json:"thinking"`
		}
		if json.Unmarshal(b.Raw, &raw) == nil && raw.Type == "thinking" {
			n += int64(len([]rune(raw.Thinking)))
		}
	}
	return n
}

// resultErrorKey is the message key of a failed tool result (its "error"
// string), empty otherwise.
func resultErrorKey(result json.RawMessage) string {
	var r struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(result, &r) != nil {
		return ""
	}
	if s, ok := r.Error.(string); ok {
		return cutChars(s, 80)
	}
	return ""
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// argsSummary reduces a tool call's arguments to what is safe and short to
// log: numbers, booleans, dates and ids (keys named id/ids/*_id/*_ids), never
// free text.
func argsSummary(args json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(args, &m) != nil {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		switch v := m[k].(type) {
		case float64, bool:
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		case string:
			if dateRe.MatchString(v) {
				parts = append(parts, k+"="+v)
			}
		case []any:
			if k == "ids" || strings.HasSuffix(k, "_ids") {
				parts = append(parts, fmt.Sprintf("%s=%v", k, v))
			}
		}
	}
	return cutChars(strings.Join(parts, " "), 200)
}
