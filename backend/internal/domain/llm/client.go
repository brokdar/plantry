// Package llm defines the provider-agnostic LLM port and the canonical event
// vocabulary the agent loop speaks. Adapters (anthropic, openai, fake) translate
// their wire format into these types.
package llm

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

// Role identifies the author of a message in a conversation.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ContentType discriminates the body shape carried by a ContentBlock.
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeToolUse    ContentType = "tool_use"
	ContentTypeToolResult ContentType = "tool_result"
	// ContentTypeServer is an opaque provider block (Anthropic server tool use
	// and its result, e.g. web search). Raw holds it verbatim so it can be
	// replayed unchanged on the next request.
	ContentTypeServer ContentType = "server"
)

// ContentBlock is a provider-neutral content item inside a Message.
// Exactly one of the shape fields is populated based on Type.
type ContentBlock struct {
	Type ContentType `json:"type"`

	// Text block payload. Citations lists the URLs the text cites (web
	// search); they are not sent back to the provider.
	Text      string   `json:"text,omitempty"`
	Citations []string `json:"citations,omitempty"`

	// ToolUse block payload (assistant emitting a tool call).
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	ToolUseName  string          `json:"tool_use_name,omitempty"`
	ToolUseInput json.RawMessage `json:"tool_use_input,omitempty"`

	// ToolResult block payload (user reporting back the tool output).
	ToolResultID      string          `json:"tool_result_id,omitempty"`
	ToolResultContent json.RawMessage `json:"tool_result_content,omitempty"`
	ToolResultIsError bool            `json:"tool_result_is_error,omitempty"`

	// Caller says who made a ToolUse call: the model directly or code it ran
	// in the provider's code execution container (programmatic tool calling).
	// Nil for blocks that are not tool calls or whose provider reports none.
	Caller *Caller `json:"caller,omitempty"`

	// Server block payload: the provider's block JSON, kept verbatim.
	Raw json.RawMessage `json:"raw,omitempty"`
}

// Caller types: Anthropic's wire values of tool_use.caller.type and of a
// tool's allowed_callers.
const (
	CallerDirect        = "direct"
	CallerCodeExecution = "code_execution_20260120"
)

// Caller is who called a tool. For a code execution caller ToolID is the id
// of the server tool use block that holds the code.
type Caller struct {
	Type   string `json:"type"`
	ToolID string `json:"tool_id,omitempty"`
}

// FromCode reports whether the call came from code the model ran.
func (c *Caller) FromCode() bool { return c != nil && c.Type == CallerCodeExecution }

// Message is a single turn in a conversation.
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// Tool describes a callable tool the model may invoke.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
	// AllowedCallers lists who may call the tool (CallerDirect,
	// CallerCodeExecution); empty: only the model directly. A provider
	// without programmatic tool calling ignores it.
	AllowedCallers []string `json:"allowed_callers,omitempty"`
}

// ToolChoice forces the model to call the named tool.
type ToolChoice struct {
	Name string `json:"name"`
}

// WebSearch enables the provider's server-side web search for a request.
// MaxUses caps the searches the provider may run in one turn.
type WebSearch struct {
	MaxUses int `json:"max_uses"`
}

// Request is the provider-neutral prompt sent to a Client. A zero
// Temperature leaves the provider default; a nil ToolChoice lets the model
// decide whether to call a tool; a nil WebSearch sends no search tool.
type Request struct {
	Model       string
	System      string
	Messages    []Message
	Tools       []Tool
	ToolChoice  *ToolChoice
	WebSearch   *WebSearch
	Temperature float64
	MaxTokens   int
	// Reasoning asks for visible reasoning (adaptive thinking) where the model
	// supports it. It makes the adapter ignore Temperature and downgrade a
	// forced ToolChoice to auto; leave it false for short or cheap calls.
	Reasoning bool
	// Effort is the reasoning effort ("low", "medium" or "high"; empty: the
	// provider default). An adapter sends it only for a model that supports
	// it (Anthropic output_config.effort, OpenAI reasoning_effort).
	Effort string
	// CodeExecution offers the provider's code execution tool so the model
	// can call the tools whose AllowedCallers name CallerCodeExecution from
	// code. Adapters that cannot (or models that do not support it) ignore it.
	CodeExecution bool
	// Container is the id of the code execution container to keep using: the
	// ContainerID of an earlier response. A request that answers programmatic
	// tool calls must carry it.
	Container string
	// CacheTail asks for a prompt-cache breakpoint on the last message, for
	// multi-turn loops that resend the history as a prefix. Leave it false for
	// one-shot calls: they would pay the cache write and never read it.
	CacheTail bool
}

// Usage reports token accounting for a single provider call.
// WebSearchRequests counts the server-side searches the provider billed.
type Usage struct {
	InputTokens       int
	OutputTokens      int
	CacheReadTokens   int
	CacheWriteTokens  int
	WebSearchRequests int
	// CodeExecutionRequests counts the code execution requests billed.
	CodeExecutionRequests int
	// ReasoningTokens is what the provider reports as spent on reasoning
	// (OpenAI reasoning_tokens, Anthropic output_tokens_details.thinking_tokens);
	// both are part of OutputTokens.
	ReasoningTokens int
}

// StopReason identifies why the assistant turn ended.
type StopReason string

const (
	StopReasonEndTurn   StopReason = "end_turn"
	StopReasonToolUse   StopReason = "tool_use"
	StopReasonMaxTokens StopReason = "max_tokens"
	StopReasonRefusal   StopReason = "refusal"
	StopReasonError     StopReason = "error"
	// StopReasonPauseTurn: the provider paused a long server-tool turn. The
	// caller resends the conversation plus the returned assistant blocks,
	// without adding a user message, to let it continue.
	StopReasonPauseTurn StopReason = "pause_turn"
)

// SearchResult is one page a server-side web search returned.
type SearchResult struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// Response is the final (non-streamed) shape of an assistant turn.
// SearchResults lists the pages the turn's web searches returned, in order
// and without duplicates.
type Response struct {
	Message       Message
	StopReason    StopReason
	Usage         Usage
	SearchResults []SearchResult
	// Model is the model the provider says it served, empty when unknown.
	Model string
	// Partial marks what a failed Stream returns next to its error: the
	// content and billed usage of a reply that broke off mid-stream.
	Partial bool
	// RequestID is the provider's id of the HTTP request (Anthropic
	// request-id, OpenAI x-request-id), empty when unknown.
	RequestID string
	// ContainerID is the code execution container of the turn (empty: none)
	// and ContainerExpiresAt when the provider says it expires (zero: unknown).
	ContainerID        string
	ContainerExpiresAt time.Time
}

// Client is the port an LLM provider adapter must implement.
//
// Stream streams the assistant turn as a sequence of canonical Events on out
// and returns the final (assembled) Response when the turn ends. The adapter
// closes out before returning. Stream must respect ctx cancellation and
// return promptly. The returned error is non-nil only for network/protocol
// failures; a successful turn that ended in tool_use still returns a non-nil
// Response whose StopReason == StopReasonToolUse.
//
// Complete sends req and returns the assembled text content of the assistant
// turn. It drains the event channel internally and is equivalent to calling
// Stream with a throwaway channel. Use when streaming events are not needed.
type Client interface {
	Stream(ctx context.Context, req Request, out chan<- Event) (*Response, error)
	Complete(ctx context.Context, req Request) (string, error)
}

// Capabilities is an optional port a Client may implement to report what the
// configured provider supports. Callers type-assert for it; a Client without
// it supports none of the capabilities.
//
// WebSearch reports whether model can use server-side web search. It
// returns false when the provider says no or cannot be asked; err is
// non-nil only when the check itself failed.
type Capabilities interface {
	WebSearch(ctx context.Context, model string) (bool, error)
}

// Effective is what an adapter actually sends for a request: its default
// max_tokens and the model's cap applied, a nil Temperature when it sends
// none, and the tool choice and web search that went over the wire. Adapters
// implement it so the run log stores what was sent, not what was asked for.
//
// ToolChoice is "forced" (the named tool was forced), "auto" (the model
// decides, the tool asked for in the system prompt) or "" (the request had
// none). Fallbacks lists why the wire request differs from the asked one,
// e.g. "capability_lookup_failed: 503 api_error", "model_always_thinks",
// "temperature_rejected_by_model", "web_search_rejected_by_org".
type Effective struct {
	MaxTokens   int
	Temperature *float64
	ToolChoice  string
	WebSearch   bool
	Thinking    bool // adaptive thinking went out
	Fallbacks   []string
	// The remaining fields describe the wire request beyond the above, for
	// the run log: thinking type and display, effort (empty when none is
	// sent), beta headers and where cache breakpoints sit ("system", "tools").
	ThinkingType     string
	ThinkingDisplay  string
	Effort           string
	Betas            []string
	CacheBreakpoints []string
	// CodeExecution says the code execution tool went out; CodeTools are the
	// tools sent with a code execution caller and Container the container id
	// sent (empty: a new one).
	CodeExecution bool
	CodeTools     []string
	Container     string
}

// ParamsReporter is implemented by clients that can report the effective
// parameters of a request (call it after Stream: the model's capabilities
// are known by then).
type ParamsReporter interface {
	EffectiveParams(req Request) Effective
}

// ProviderErrorInfo is what a provider error says, for the run log: never
// the key or the prompt. RawBody is capped by the adapter.
type ProviderErrorInfo struct {
	Status    int    `json:"status"`
	Type      string `json:"type,omitempty"`
	Message   string `json:"message,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	RateLimit string `json:"rate_limit,omitempty"` // retry-after and ratelimit headers
	RawBody   string `json:"raw_body,omitempty"`
}

// ProviderError is implemented by adapter errors that carry the provider's
// HTTP status and error details, so the run log can record them.
type ProviderError interface {
	error
	ProviderStatus() int
	ProviderErrorType() string
	ProviderInfo() ProviderErrorInfo
}

// Attempt is one failed wire request an adapter retried with different
// parameters (a 400 naming temperature, tool_choice or web search). The
// adapter reports each through the observer in ctx (WithAttemptObserver) so
// the run log records every attempt as its own step.
type Attempt struct {
	Started  time.Time
	Duration time.Duration
	Err      error
	// Sent is what this attempt put on the wire; Reason why the next one
	// differs (e.g. "temperature_rejected_400").
	Sent   Effective
	Reason string
}

type ctxKey int

const (
	attemptKey ctxKey = iota
	logAttrsKey
	callerKey
)

// WithCaller returns ctx carrying who called the tool being executed, so the
// tool and the run log can tell a call from code from a direct one.
func WithCaller(ctx context.Context, c *Caller) context.Context {
	return context.WithValue(ctx, callerKey, c)
}

// CallerFrom returns the caller set by WithCaller; nil means a direct call.
func CallerFrom(ctx context.Context) *Caller {
	c, _ := ctx.Value(callerKey).(*Caller)
	return c
}

// WithAttemptObserver returns ctx carrying fn, which adapters call for each
// retried attempt.
func WithAttemptObserver(ctx context.Context, fn func(Attempt)) context.Context {
	return context.WithValue(ctx, attemptKey, fn)
}

// ReportAttempt hands a to the observer in ctx, if any.
func ReportAttempt(ctx context.Context, a Attempt) {
	if fn, ok := ctx.Value(attemptKey).(func(Attempt)); ok {
		fn(a)
	}
}

// WithLogAttrs returns ctx carrying the attributes (run_id, request_id)
// adapters add to their log lines.
func WithLogAttrs(ctx context.Context, attrs []any) context.Context {
	return context.WithValue(ctx, logAttrsKey, attrs)
}

// Log logs msg with the ctx's run attributes ahead of attrs.
func Log(ctx context.Context, level slog.Level, msg string, attrs ...any) {
	base, _ := ctx.Value(logAttrsKey).([]any)
	slog.Log(ctx, level, msg, append(append([]any(nil), base...), attrs...)...)
}
