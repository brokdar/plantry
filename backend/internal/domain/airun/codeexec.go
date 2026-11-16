package airun

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// pendingCode is the code of a server tool use whose result has not arrived.
type pendingCode struct {
	name    string
	code    string
	started time.Time
}

// containerOf is the container a call used: the reply's, else the one sent.
func containerOf(resp *llm.Response, eff *llm.Effective) string {
	if resp != nil && resp.ContainerID != "" {
		return resp.ContainerID
	}
	if eff != nil {
		return eff.Container
	}
	return ""
}

// isCodeBlock reports whether a server block type belongs to a code
// execution tool (code_execution, bash_code_execution, text_editor_code_execution).
func isCodeBlock(kind string) bool { return strings.Contains(kind, "code_execution") }

// recordCode stores a code_execution step for every code execution result in
// resp. The code comes from the server tool use block, which arrived with an
// earlier reply when the code paused on a tool call, so it is remembered
// until its result shows up; the step's duration runs from the reply that
// carried the code to the end of the reply that carried the result, i.e. it
// includes the pauses for tool results.
func (run *Run) recordCode(ctx context.Context, resp *llm.Response, call callInfo) {
	finished := call.started.Add(call.dur)
	for _, b := range resp.Message.Content {
		if b.Type != llm.ContentTypeServer || len(b.Raw) == 0 {
			continue
		}
		var blk struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Input     json.RawMessage `json:"input"`
			ToolUseID string          `json:"tool_use_id"`
			Content   struct {
				Type       string `json:"type"`
				Stdout     string `json:"stdout"`
				Stderr     string `json:"stderr"`
				ReturnCode *int   `json:"return_code"`
				ErrorCode  string `json:"error_code"`
			} `json:"content"`
		}
		if json.Unmarshal(b.Raw, &blk) != nil {
			continue
		}
		switch {
		case blk.Type == "server_tool_use" && isCodeBlock(blk.Name):
			var in struct {
				Code    string `json:"code"`
				Command string `json:"command"`
			}
			_ = json.Unmarshal(blk.Input, &in)
			code := in.Code
			if code == "" {
				code = in.Command
			}
			if code == "" {
				code = string(blk.Input)
			}
			run.mu.Lock()
			if run.codePending == nil {
				run.codePending = map[string]pendingCode{}
			}
			run.codePending[blk.ID] = pendingCode{name: blk.Name, code: code, started: call.started}
			run.mu.Unlock()
		case isCodeBlock(blk.Type) && strings.HasSuffix(blk.Type, "_tool_result"):
			run.mu.Lock()
			p, ok := run.codePending[blk.ToolUseID]
			delete(run.codePending, blk.ToolUseID)
			run.seq++
			seq := run.seq
			container := run.containerID
			run.mu.Unlock()
			name, started := "code_execution", call.started
			if ok {
				name, started = p.name, p.started
			}
			returnCode := 0
			if blk.Content.ReturnCode != nil {
				returnCode = *blk.Content.ReturnCode
			}
			isErr := returnCode != 0 || blk.Content.ErrorCode != "" || strings.HasSuffix(blk.Content.Type, "_error")
			result, _ := json.Marshal(map[string]any{
				"tool_use_id": blk.ToolUseID, "container_id": container, "result_type": blk.Content.Type,
				"stdout": blk.Content.Stdout, "stderr": blk.Content.Stderr, "return_code": returnCode,
				"error_code": blk.Content.ErrorCode,
			})
			dur := finished.Sub(started)
			run.addStep(ctx, Step{
				Seq: seq, Type: StepCodeExecution, Name: name, ArgsJSON: p.code, ResultJSON: string(result),
				IsError: isErr, DurationMs: dur.Milliseconds(), StartedAt: started,
			})
			run.rec.logger.InfoContext(ctx, "ai.code.execution", append(run.LogAttrs(),
				"kind", run.kind, "step_seq", seq, "tool_use_id", blk.ToolUseID, "container_id", container,
				"return_code", returnCode, "is_error", isErr, "duration_ms", dur.Milliseconds(),
				"code_bytes", len(p.code), "stdout_bytes", len(blk.Content.Stdout), "stderr_bytes", len(blk.Content.Stderr))...)
		}
	}
}
