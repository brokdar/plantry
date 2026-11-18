package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/airun"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// ErrIterationsExceeded is raised when the agent loop hits its maximum
// iteration budget without the model returning an end_turn.
var ErrIterationsExceeded = errors.New("agent iteration budget exceeded")

// MaxIterations caps the model turns of the tool-calling loop. The cap
// protects against runaway tool-tool cycles and unbounded token cost.
const MaxIterations = 20

// MaxCodeContinuations caps the requests that only continue a script the
// model runs in the code execution container (answers to code-called tools,
// a paused turn resent); they are not model turns, so MaxIterations does not
// count them. Past it the pending calls are answered with an error telling
// the script to stop, so the model closes its turn with a final message.
const MaxCodeContinuations = 40

// cutoffMessage answers the tool calls of a reply cut off at the output limit.
const cutoffMessage = "Your reply was cut off at the output limit, so nothing was run. Keep your reasoning short and let the tools compute: use apply_changes with dry_run true."

// MaxRequests is the hard bound on provider requests per Run, wrap-up
// requests included.
const MaxRequests = MaxIterations + MaxCodeContinuations + 4

// RunRequest is the input to Run.
type RunRequest struct {
	ConversationID int64
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
	// Effort is the reasoning effort (ai.reasoning_effort.chat); empty: the
	// provider default.
	Effort string
	// CodeExecution offers the code execution tool (programmatic tool
	// calling). Container is the container to reuse (empty: a new one) and
	// SaveContainer, when set, is told every new container the provider names.
	CodeExecution bool
	Container     string
	SaveContainer func(ctx context.Context, id string, expires time.Time)
}

// Run drives a tool-calling agent loop to completion on the given request and
// streams canonical llm.Events to out. Events include MessageStart,
// AssistantDelta, ToolCallStart, ToolCallDelta (relayed from the adapter) plus
// ToolExecStart/ToolExecEnd/ToolResult/Change/UnlockRequest/Done/Error (emitted by
// the loop itself). The caller owns the out channel and must drain it.
//
// The loop persists one assistant message per turn (with any tool_use blocks)
// plus a tool message containing tool_result blocks for that turn. On fatal
// error it persists a role=error message and emits an Error event.
func Run(ctx context.Context, req RunRequest, client llm.Client, tools *ToolSet, repo Repository, out chan<- llm.Event) error {
	history := append([]llm.Message{}, req.History...)
	var usage llm.Usage
	container := req.Container
	var lastExpires time.Time

	// pend holds the messages of an open code execution turn: they are
	// stored only once the turn is closed, so the stored conversation never
	// ends inside a turn the API cannot continue from (a text message after
	// unanswered programmatic calls is a 400 on every later request).
	var pend []pendingMessage
	var turn []llm.Message
	store := func(role Role, m llm.Message) error {
		pend = append(pend, pendingMessage{role, m})
		turn = append(turn, m)
		if llm.CodeTurnOpen(turn) {
			return nil
		}
		return flushPending(ctx, repo, req.ConversationID, &pend, &turn, false)
	}
	// closeTurn stores what remains of an unfinished code turn without its
	// code blocks, so the next request replays a complete exchange.
	closeTurn := func() {
		if len(pend) > 0 {
			_ = flushPending(context.WithoutCancel(ctx), repo, req.ConversationID, &pend, &turn, true)
		}
	}
	fail := func(key string, err error) error {
		closeTurn()
		return emitAndPersistError(ctx, out, repo, req.ConversationID, key, err)
	}

	requests, turns, conts, continuation, wrapUp := 0, 0, 0, false, false
	for {
		if requests++; requests > MaxRequests {
			break
		}
		if continuation {
			conts++
		} else if turns++; turns > MaxIterations {
			break
		}
		iter := requests - 1
		providerReq := llm.Request{
			Model:       req.Model,
			System:      req.SystemPrompt,
			Messages:    history,
			Tools:       req.Tools,
			Temperature: req.Temperature,
			MaxTokens:   req.MaxTokens,
			Reasoning:   true,
			Effort:      req.Effort,

			CodeExecution: req.CodeExecution,
			Container:     container,
			CacheTail:     true,
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
				closeTurn()
				return ctx.Err()
			}
		}
		res := <-done
		if res.err != nil {
			key := "error.ai.stream_interrupted"
			var perr llm.ProviderError
			if errors.As(res.err, &perr) {
				// The provider answered with an error (bad request, auth, ...):
				// not an interrupted stream.
				key = "error.ai.provider_error"
			}
			if f := llm.FailureOf(res.err); f != llm.FailureOther {
				// A limit, a rejected key or a timeout tells the user what to do.
				key = "error.ai." + string(f)
			}
			return fail(key, res.err)
		}
		if res.resp == nil {
			err := errors.New("llm client returned nil response without error")
			return fail("error.server", err)
		}
		if id, exp := res.resp.ContainerID, res.resp.ContainerExpiresAt; id != "" {
			// The expiry rolls forward with every reply: store it each time,
			// so the next turn's reuse check sees the current one.
			if (id != container || !exp.Equal(lastExpires)) && req.SaveContainer != nil && !exp.IsZero() {
				req.SaveContainer(ctx, id, exp)
			}
			container, lastExpires = id, exp
		}
		usage.InputTokens += res.resp.Usage.InputTokens
		usage.OutputTokens += res.resp.Usage.OutputTokens
		usage.CacheReadTokens += res.resp.Usage.CacheReadTokens
		usage.CacheWriteTokens += res.resp.Usage.CacheWriteTokens

		// The model declined: run none of what it started, say so.
		if res.resp.StopReason == llm.StopReasonRefusal {
			return fail("error.ai.refused", fmt.Errorf("%w: the model refused the request", domain.ErrAIRefused))
		}

		// Persist the assistant message (text + any tool_use blocks).
		// A cut tool input is replayed as `{}`; the run steps keep the raw text.
		replay := llm.ReplayMessage(res.resp.Message)
		if err := store(RoleAssistant, replay); err != nil {
			return fail("error.server", err)
		}
		history = append(history, replay)

		toolUses := extractToolUses(res.resp.Message)

		// A long server-side turn (code execution) paused: resend the
		// conversation, now ending in the assistant blocks, to continue it.
		continuation = false
		if res.resp.StopReason == llm.StopReasonPauseTurn && len(toolUses) == 0 {
			continuation = true
			continue
		}

		// No tool calls → the turn is the final response.
		if res.resp.StopReason == llm.StopReasonEndTurn || len(toolUses) == 0 {
			closeTurn()
			send(ctx, out, llm.Event{Type: llm.EventDone, Payload: llm.DonePayload{
				StopReason:     res.resp.StopReason,
				Usage:          usage,
				IterationCount: iter + 1,
			}})
			return nil
		}

		// A reply cut off at the output limit may hold cut tool input: run
		// nothing, answer every call with an error. It stays a model turn.
		if res.resp.StopReason == llm.StopReasonMaxTokens {
			resultBlocks := make([]llm.ContentBlock, 0, len(toolUses))
			modelSeq := airun.FromContext(ctx).Seq()
			var seqs []int
			for _, call := range toolUses {
				payload := mustMarshal(map[string]string{"error": cutoffMessage})
				resultBlocks = append(resultBlocks, llm.ContentBlock{
					Type: llm.ContentTypeToolResult, ToolResultID: call.ToolUseID, ToolResultContent: payload, ToolResultIsError: true,
				})
				seqs = append(seqs, airun.FromContext(ctx).ToolStep(llm.WithCaller(ctx, call.Caller), call.ToolUseName, call.ToolUseInput, payload, true, 0))
				send(ctx, out, llm.Event{Type: llm.EventToolExecStart, Payload: llm.ToolExecStartPayload{
					ID: call.ToolUseID, Name: call.ToolUseName,
				}})
				send(ctx, out, llm.Event{Type: llm.EventToolExecEnd, Payload: llm.ToolExecEndPayload{
					ID: call.ToolUseID, Name: call.ToolUseName, Status: llm.ToolExecStatusError,
				}})
				send(ctx, out, llm.Event{Type: llm.EventToolResult, Payload: llm.ToolResultPayload{
					ID: call.ToolUseID, Name: call.ToolUseName, Output: payload, IsError: true,
				}})
			}
			slog.WarnContext(ctx, "agent.loop.cutoff", append(airun.FromContext(ctx).LogAttrs(), "conversation_id", req.ConversationID, "model_seq", modelSeq, "tool_seqs", seqs)...)
			toolMsg := llm.Message{Role: llm.RoleUser, Content: resultBlocks}
			if err := store(RoleTool, toolMsg); err != nil {
				return fail("error.server", err)
			}
			history = append(history, toolMsg)
			continue
		}

		// Past the code continuation bound the script gets errors only, so it
		// ends and the model closes the turn with a message.
		if !wrapUp && onlyCodeCalls(toolUses) && conts >= MaxCodeContinuations {
			wrapUp = true
			slog.WarnContext(ctx, "agent.loop.code_limit", "conversation_id", req.ConversationID, "continuations", conts)
		}
		continuation = onlyCodeCalls(toolUses)

		// Execute each tool call.
		resultBlocks := make([]llm.ContentBlock, 0, len(toolUses))
		for _, call := range toolUses {
			if wrapUp && call.Caller.FromCode() {
				resultBlocks = append(resultBlocks, llm.ContentBlock{
					Type: llm.ContentTypeToolResult, ToolResultID: call.ToolUseID, ToolResultIsError: true,
					ToolResultContent: mustMarshal(map[string]string{"error": "tool call limit reached for this turn; stop calling tools and answer with what you have"}),
				})
				continue
			}
			send(ctx, out, llm.Event{Type: llm.EventToolExecStart, Payload: llm.ToolExecStartPayload{
				ID: call.ToolUseID, Name: call.ToolUseName,
			}})
			start := time.Now()
			caller := call.Caller
			if caller == nil {
				caller = &llm.Caller{Type: llm.CallerDirect}
			}
			output, effect, change, err := tools.Execute(llm.WithCaller(ctx, caller), call.ToolUseName, call.ToolUseInput)
			dur := time.Since(start).Milliseconds()

			if err != nil {
				// Tool errors are fed back to the model as tool_result with is_error.
				// This gives the model a chance to correct its arguments or try a
				// different approach instead of failing the whole turn.
				errorPayload := withChange(mustMarshal(map[string]string{
					"error":       err.Error(),
					"message_key": errorMessageKey(err),
				}), change)
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
				if change != nil {
					// A failed call that still changed the plan (a regenerate
					// whose wait ended) shows its line.
					send(ctx, out, llm.Event{Type: llm.EventChange, Payload: change.Payload()})
				}
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
			if change != nil {
				send(ctx, out, llm.Event{Type: llm.EventChange, Payload: change.Payload()})
			}
			switch effect {
			case ToolEffectUnlockRequest:
				var unlock llm.UnlockRequestPayload
				if err := json.Unmarshal(output, &unlock); err != nil {
					return fail("error.server", fmt.Errorf("unlock request: %w", err))
				}
				send(ctx, out, llm.Event{Type: llm.EventUnlockRequest, Payload: unlock})
			case ToolEffectNone:
			}
		}

		// Persist and append the tool message carrying all result blocks.
		toolMsg := llm.Message{Role: llm.RoleUser, Content: resultBlocks}
		if err := store(RoleTool, toolMsg); err != nil {
			return fail("error.server", err)
		}
		history = append(history, toolMsg)
	}

	err := fmt.Errorf("%w: %d model turns, %d requests", ErrIterationsExceeded, turns, requests)
	return fail("error.ai.iterations_exceeded", err)
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

// onlyCodeCalls reports whether every call comes from code the model ran.
func onlyCodeCalls(calls []llm.ContentBlock) bool {
	for _, c := range calls {
		if !c.Caller.FromCode() {
			return false
		}
	}
	return len(calls) > 0
}

type pendingMessage struct {
	role Role
	msg  llm.Message
}

// flushPending stores the buffered messages of a code turn and empties the
// buffer. With strip the turn is unfinished: its code blocks, the code-called
// tool calls and their results are left out (later messages are appended, so
// no stored message is rewritten).
func flushPending(ctx context.Context, repo Repository, convID int64, pend *[]pendingMessage, turn *[]llm.Message, strip bool) error {
	msgs := *pend
	*pend, *turn = nil, nil
	if strip {
		plain := make([]llm.Message, len(msgs))
		for i, m := range msgs {
			plain[i] = m.msg
		}
		kept := llm.StripCodeBlocks(plain)
		msgs = msgs[:0:0]
		for _, m := range kept {
			role := RoleAssistant
			if m.Role == llm.RoleUser {
				role = RoleTool
			}
			msgs = append(msgs, pendingMessage{role, m})
		}
	}
	for _, m := range msgs {
		if err := persistMessage(ctx, repo, convID, m.role, m.msg.Content); err != nil {
			return err
		}
	}
	return nil
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

// PlainMessage is the sentence the chat shows for an error key: what happened
// and what to do, without the raw error (provider text, ids, internals). The
// frontend translates the key; this text is for the stored transcript row.
func PlainMessage(key string) string {
	switch key {
	case "error.ai.rate_limited":
		return "The AI provider is busy or its limit is reached. Wait a minute and try again."
	case "error.ai.key_rejected":
		return "The AI provider rejected the API key. Check the key in Settings."
	case "error.ai.timeout":
		return "The assistant took too long to answer. Please try again."
	case "error.ai.refused":
		return "The assistant declined this request, so nothing was changed. Rephrase it or plan without the assistant."
	case "error.ai.provider_error":
		return "The assistant could not be reached. Try again."
	case "error.ai.iterations_exceeded":
		return "The assistant ran too many steps. Try a simpler request."
	case "error.ai.stream_interrupted":
		return "The connection to the assistant was interrupted. Try again."
	default:
		return "Something went wrong. Try again."
	}
}

// keyedError carries the i18n message key the loop emitted for a fatal error.
// It wraps the original error, so errors.Is and Error() are unchanged.
type keyedError struct {
	key string
	err error
}

func (e *keyedError) Error() string { return e.err.Error() }
func (e *keyedError) Unwrap() error { return e.err }

// ErrorEmitted reports whether the loop already sent err to the client as an
// error event, so the caller must not send it again.
func ErrorEmitted(err error) bool {
	var keyed *keyedError
	return errors.As(err, &keyed)
}

// emitAndPersistError reports a fatal loop error and returns err tagged with
// key for the caller to return.
func emitAndPersistError(ctx context.Context, out chan<- llm.Event, repo Repository, convID int64, key string, err error) error {
	send(ctx, out, llm.Event{Type: llm.EventError, Payload: llm.StreamErrorPayload{
		MessageKey: key, Message: PlainMessage(key),
	}})
	// Persist best-effort; swallow secondary errors. The transcript shows the
	// row as it is, so it holds the plain sentence, not the provider's text
	// (the run log keeps that).
	b, _ := json.Marshal([]llm.ContentBlock{{
		Type: llm.ContentTypeText,
		Text: PlainMessage(key),
	}})
	_ = repo.AppendMessage(context.Background(), &Message{
		ConversationID: convID,
		Role:           RoleError,
		Content:        b,
	})
	return &keyedError{key: key, err: err}
}

// errorMessageKey returns a stable i18n key for domain-level error types,
// from the same tables the HTTP error map uses. Unknown errors map to
// error.server.
func errorMessageKey(err error) string {
	if errors.Is(err, ErrToolNotFound) {
		return "error.ai.tool_not_found"
	}
	if key, ok := plate.MessageKey(err); ok {
		return key
	}
	key, _ := domain.MessageKey(err, "food")
	return key
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
