package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// ErrIterationsExceeded is raised when the agent loop hits its maximum
// iteration budget without the model returning an end_turn.
var ErrIterationsExceeded = errors.New("agent iteration budget exceeded")

// MaxIterations caps the tool-calling loop. The cap protects against
// runaway tool-tool cycles and unbounded token cost.
const MaxIterations = 20

// RunRequest is the input to Run.
type RunRequest struct {
	ConversationID int64
	WeekID         *int64
	Model          string
	SystemPrompt   string
	// History is the conversation so far in canonical form (user + prior
	// assistant + tool messages). The loop does NOT persist these — the
	// caller is responsible for persisting the user message BEFORE calling
	// Run. The loop only persists the messages it generates (assistant +
	// tool + error).
	History     []llm.Message
	Tools       []llm.Tool
	Temperature float64
	MaxTokens   int
}

// Run drives a tool-calling agent loop to completion on the given request and
// streams canonical llm.Events to out. Events include MessageStart,
// AssistantDelta, ToolCallStart, ToolCallDelta (relayed from the adapter) plus
// ToolExecStart/ToolExecEnd/ToolResult/PlateChanged/Done/Error (emitted by
// the loop itself). The caller owns the out channel and must drain it.
//
// The loop persists one assistant message per turn (with any tool_use blocks)
// plus a tool message containing tool_result blocks for that turn. On fatal
// error it persists a role=error message and emits an Error event.
func Run(ctx context.Context, req RunRequest, client llm.Client, tools *ToolSet, repo Repository, out chan<- llm.Event) error {
	history := append([]llm.Message{}, req.History...)

	for iter := 0; iter < MaxIterations; iter++ {
		providerReq := llm.Request{
			Model:       req.Model,
			System:      req.SystemPrompt,
			Messages:    history,
			Tools:       req.Tools,
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
		}

		providerCh := make(chan llm.Event, 16)
		type result struct {
			resp *llm.Response
			err  error
		}
		done := make(chan result, 1)
		go func() {
			r, err := client.Stream(ctx, providerReq, providerCh)
			done <- result{resp: r, err: err}
		}()

		// Relay every provider event untouched to the outer consumer.
		for evt := range providerCh {
			if !send(ctx, out, evt) {
				<-done // wait for goroutine exit
				return ctx.Err()
			}
		}
		res := <-done
		if res.err != nil {
			emitAndPersistError(ctx, out, repo, req.ConversationID, "error.ai.stream_interrupted", res.err)
			return res.err
		}
		if res.resp == nil {
			err := errors.New("llm client returned nil response without error")
			emitAndPersistError(ctx, out, repo, req.ConversationID, "error.server", err)
			return err
		}

		// Persist the assistant message (text + any tool_use blocks).
		if err := persistMessage(ctx, repo, req.ConversationID, RoleAssistant, res.resp.Message.Content); err != nil {
			emitAndPersistError(ctx, out, repo, req.ConversationID, "error.server", err)
			return err
		}
		history = append(history, res.resp.Message)

		toolUses := extractToolUses(res.resp.Message)

		// No tool calls → the turn is the final response.
		if res.resp.StopReason == llm.StopReasonEndTurn || len(toolUses) == 0 {
			send(ctx, out, llm.Event{Type: llm.EventDone, Payload: llm.DonePayload{
				StopReason:     res.resp.StopReason,
				Usage:          res.resp.Usage,
				IterationCount: iter + 1,
			}})
			return nil
		}

		// Execute each tool call.
		resultBlocks := make([]llm.ContentBlock, 0, len(toolUses))
		plateChanged := false
		weekID := int64(0)
		if req.WeekID != nil {
			weekID = *req.WeekID
		}
		for _, call := range toolUses {
			send(ctx, out, llm.Event{Type: llm.EventToolExecStart, Payload: llm.ToolExecStartPayload{
				ID: call.ToolUseID, Name: call.ToolUseName,
			}})
			start := time.Now()
			output, effect, err := tools.Execute(ctx, call.ToolUseName, call.ToolUseInput)
			dur := time.Since(start).Milliseconds()

			if err != nil {
				// Tool errors are fed back to the model as tool_result with is_error.
				// This gives the model a chance to correct its arguments or try a
				// different approach instead of failing the whole turn.
				errorPayload := mustMarshal(map[string]string{
					"error":       err.Error(),
					"message_key": errorMessageKey(err),
				})
				resultBlocks = append(resultBlocks, llm.ContentBlock{
					Type:              llm.ContentTypeToolResult,
					ToolResultID:      call.ToolUseID,
					ToolResultContent: errorPayload,
					ToolResultIsError: true,
				})
				send(ctx, out, llm.Event{Type: llm.EventToolExecEnd, Payload: llm.ToolExecEndPayload{
					ID: call.ToolUseID, Name: call.ToolUseName,
					Status: llm.ToolExecStatusError, DurationMs: dur,
				}})
				send(ctx, out, llm.Event{Type: llm.EventToolResult, Payload: llm.ToolResultPayload{
					ID: call.ToolUseID, Name: call.ToolUseName,
					Output: errorPayload, IsError: true,
				}})
				continue
			}

			resultBlocks = append(resultBlocks, llm.ContentBlock{
				Type:              llm.ContentTypeToolResult,
				ToolResultID:      call.ToolUseID,
				ToolResultContent: output,
			})
			send(ctx, out, llm.Event{Type: llm.EventToolExecEnd, Payload: llm.ToolExecEndPayload{
				ID: call.ToolUseID, Name: call.ToolUseName,
				Status: llm.ToolExecStatusOK, DurationMs: dur,
			}})
			send(ctx, out, llm.Event{Type: llm.EventToolResult, Payload: llm.ToolResultPayload{
				ID: call.ToolUseID, Name: call.ToolUseName,
				Output: output,
			}})
			if effect == ToolEffectPlateChanged {
				plateChanged = true
			}
		}

		if plateChanged {
			send(ctx, out, llm.Event{Type: llm.EventPlateChanged, Payload: llm.PlateChangedPayload{
				WeekID: weekID,
			}})
		}

		// Persist and append the tool message carrying all result blocks.
		if err := persistMessage(ctx, repo, req.ConversationID, RoleTool, resultBlocks); err != nil {
			emitAndPersistError(ctx, out, repo, req.ConversationID, "error.server", err)
			return err
		}
		history = append(history, llm.Message{Role: llm.RoleUser, Content: resultBlocks})
	}

	err := fmt.Errorf("%w: %d iterations reached", ErrIterationsExceeded, MaxIterations)
	emitAndPersistError(ctx, out, repo, req.ConversationID, "error.ai.iterations_exceeded", err)
	return err
}

// send writes evt to out unless ctx is done. Returns false if the context was
// cancelled — the caller should abort.
func send(ctx context.Context, out chan<- llm.Event, evt llm.Event) bool {
	select {
	case <-ctx.Done():
		return false
	case out <- evt:
		return true
	}
}

func extractToolUses(m llm.Message) []llm.ContentBlock {
	var out []llm.ContentBlock
	for _, b := range m.Content {
		if b.Type == llm.ContentTypeToolUse {
			out = append(out, b)
		}
	}
	return out
}

func persistMessage(ctx context.Context, repo Repository, convID int64, role Role, content []llm.ContentBlock) error {
	b, err := json.Marshal(content)
	if err != nil {
		return err
	}
	return repo.AppendMessage(ctx, &Message{
		ConversationID: convID,
		Role:           role,
		Content:        b,
	})
}

func emitAndPersistError(ctx context.Context, out chan<- llm.Event, repo Repository, convID int64, key string, err error) {
	send(ctx, out, llm.Event{Type: llm.EventError, Payload: llm.StreamErrorPayload{
		MessageKey: key, Message: err.Error(),
	}})
	// Persist best-effort; swallow secondary errors.
	b, _ := json.Marshal([]llm.ContentBlock{{
		Type: llm.ContentTypeText,
		Text: err.Error(),
	}})
	_ = repo.AppendMessage(context.Background(), &Message{
		ConversationID: convID,
		Role:           RoleError,
		Content:        b,
	})
}

// errorMessageKey returns a stable i18n key for domain-level error types.
// Unknown errors map to error.server.
func errorMessageKey(err error) string {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return "error.not_found"
	case errors.Is(err, domain.ErrInvalidInput):
		return "error.invalid_body"
	case errors.Is(err, domain.ErrSlotUnknown):
		return "error.plate.slot_unknown"
	case errors.Is(err, ErrToolNotFound):
		return "error.ai.tool_not_found"
	default:
		return "error.server"
	}
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		// Only reachable for un-marshalable types (channels, funcs, cycles) —
		// none of which appear here. A panic is appropriate; chi.Recoverer catches it.
		panic(fmt.Sprintf("agent loop: json.Marshal: %v", err))
	}
	return b
}
