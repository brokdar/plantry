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
	seq     int
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
			if _, seen := run.codePending[blk.ID]; seen {
				run.mu.Unlock()
				continue
			}
			run.seq++
			p := pendingCode{name: blk.Name, code: code, started: call.started, seq: run.seq}
			run.codePending[blk.ID] = p
			container := run.containerID
			run.mu.Unlock()
			// The step exists from the moment the code is issued, so a run
			// that ends before the result still shows what the model ran.
			pending, _ := json.Marshal(map[string]any{"status": "pending", "tool_use_id": blk.ID, "container_id": container})
			run.addStep(ctx, Step{
				Seq: p.seq, Type: StepCodeExecution, Name: p.name, ArgsJSON: code, ResultJSON: string(pending),
				StartedAt: call.started,
			})
		case isCodeBlock(blk.Type) && strings.HasSuffix(blk.Type, "_tool_result"):
			run.mu.Lock()
			p, ok := run.codePending[blk.ToolUseID]
			delete(run.codePending, blk.ToolUseID)
			if !ok {
				run.seq++
				p = pendingCode{name: "code_execution", started: call.started, seq: run.seq}
			}
			seq := p.seq
			container := run.containerID
			run.mu.Unlock()
			started := p.started
			returnCode := 0
			if blk.Content.ReturnCode != nil {
				returnCode = *blk.Content.ReturnCode
			}
			// A pending call that timed out raises TimeoutError inside the
			// code and arrives in stderr with return code 0.
			isErr := returnCode != 0 || blk.Content.ErrorCode != "" || strings.HasSuffix(blk.Content.Type, "_error") ||
				strings.Contains(blk.Content.Stderr, "TimeoutError")
			result, _ := json.Marshal(map[string]any{
				"status": "completed", "tool_use_id": blk.ToolUseID, "container_id": container, "result_type": blk.Content.Type,
				"stdout": blk.Content.Stdout, "stderr": blk.Content.Stderr, "return_code": returnCode,
				"error_code": blk.Content.ErrorCode, "code_execution_requests": resp.Usage.CodeExecutionRequests,
			})
			dur := finished.Sub(started)
			if !ok {
				run.addStep(ctx, Step{
					Seq: seq, Type: StepCodeExecution, Name: p.name, ArgsJSON: p.code, ResultJSON: string(result),
					IsError: isErr, DurationMs: dur.Milliseconds(), StartedAt: started,
				})
			} else {
				run.updateStep(ctx, seq, string(result), isErr, dur.Milliseconds())
			}
			run.rec.logger.InfoContext(ctx, "ai.code.execution", append(run.LogAttrs(),
				"kind", run.kind, "step_seq", seq, "tool_use_id", blk.ToolUseID, "container_id", container,
				"return_code", returnCode, "is_error", isErr, "duration_ms", dur.Milliseconds(),
				"code_requests", resp.Usage.CodeExecutionRequests, "code_bytes", len(p.code), "stdout_bytes", len(blk.Content.Stdout), "stderr_bytes", len(blk.Content.Stderr))...)
		}
	}
}

// closeCode completes the code steps whose result never arrived (the run
// ended first: error, cancel, a cap) as unfinished errors.
func (run *Run) closeCode(ctx context.Context, finished time.Time) {
	run.mu.Lock()
	pending := run.codePending
	run.codePending = nil
	container := run.containerID
	run.mu.Unlock()
	for id, p := range pending {
		result, _ := json.Marshal(map[string]any{"status": "unfinished", "tool_use_id": id, "container_id": container})
		run.updateStep(ctx, p.seq, string(result), true, finished.Sub(p.started).Milliseconds())
	}
}

func (run *Run) updateStep(ctx context.Context, seq int, result string, isErr bool, durMs int64) {
	if run.id == 0 {
		return
	}
	if err := run.rec.store.UpdateStepResult(context.WithoutCancel(ctx), run.id, seq, result, isErr, durMs); err != nil {
		run.warn(ctx, "ai.run.step update failed", err)
	}
}
