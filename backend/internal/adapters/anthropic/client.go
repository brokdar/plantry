// Package anthropic implements llm.Client against the Anthropic Messages API
// using stdlib net/http (no SDK).
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

const (
	defaultBaseURL = "https://api.anthropic.com"
	apiVersion     = "2023-06-01"
	// DefaultMaxTokens is used when llm.Request.MaxTokens is 0.
	DefaultMaxTokens = 4096
)

// Client implements llm.Client and llm.Capabilities.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client

	now           func() time.Time
	lookupTimeout time.Duration
	mu            sync.Mutex
	caps          map[string]capability // model capabilities (24 h cache)
	flights       map[string]*flight    // capability lookups in progress
	rejects       map[string]rejection  // parameters a model's API refused
}

// Option configures the client.
type Option func(*Client)

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithBaseURL overrides the default Anthropic base URL (for tests).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = u }
}

// New constructs a Client.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: llm.NewHTTPClient(),
		now:        time.Now,

		lookupTimeout: lookupTimeout,
		caps:          map[string]capability{},
		flights:       map[string]*flight{},
		rejects:       map[string]rejection{},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Complete implements llm.Client.Complete.
func (c *Client) Complete(ctx context.Context, req llm.Request) (string, error) {
	events := make(chan llm.Event, 16)
	done := make(chan struct{})
	go func() {
		for range events {
		}
		close(done)
	}()
	resp, err := c.Stream(ctx, req, events)
	<-done
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", fmt.Errorf("anthropic: nil response")
	}
	var sb strings.Builder
	for _, block := range resp.Message.Content {
		if block.Type == llm.ContentTypeText {
			sb.WriteString(block.Text)
		}
	}
	return sb.String(), nil
}

// Stream implements llm.Client.Stream.
func (c *Client) Stream(ctx context.Context, req llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)

	// The model's capabilities (looked up once per model per 24 h, failures
	// 5 min; a failed lookup is logged by the lookup itself) decide the forced
	// tool_choice and the max_tokens cap. A lookup that fails falls back to
	// `auto` plus a system line.
	_, _ = c.capability(ctx, req.Model)

	dropContainer := false
	for {
		w := c.plan(req)
		sent := req
		if dropContainer && w.container != "" {
			w.container = ""
			w.fallbacks = append(w.fallbacks, "container_dropped_after_400")
		}
		if !w.codeExecution && llm.HasCodeBlocks(req.Messages) {
			// The request carries no code execution tool (setting off,
			// capability gone, rejected), so a history with code blocks is
			// replayed without them: the docs do not say whether the tool
			// definition is needed to replay them, and a history without
			// them is always valid.
			sent.Messages = llm.StripCodeBlocks(req.Messages)
			w.fallbacks = append(w.fallbacks, "code_blocks_stripped")
		}
		started := c.now()
		resp, err := c.post(ctx, sent, w)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusOK {
			defer func() { _ = resp.Body.Close() }()
			final, err := parseStream(ctx, resp.Body, out)
			if err != nil {
				err = fmt.Errorf("anthropic: reading stream: %w", err)
			}
			if final != nil {
				final.RequestID = resp.Header.Get("request-id")
			}
			return final, err
		}
		apiErr := decodeAPIError(resp)
		_ = resp.Body.Close()
		reason := ""
		if resp.StatusCode == http.StatusBadRequest {
			reason = c.learnFromRejection(req, w, apiErr)
		}
		if reason == "" {
			return nil, apiErr
		}
		if reason == "container_rejected_400" {
			dropContainer = true
		}
		llm.Log(ctx, slog.LevelWarn, "anthropic.request.retry",
			"model", req.Model, "reason", reason, "request_id", apiErr.RequestID,
			"message", cutMessage(apiErr.Message), "sent_tool_choice", w.effective(req).ToolChoice,
			"sent_temperature", w.temperature, "sent_web_search", w.webSearch)
		llm.ReportAttempt(ctx, llm.Attempt{
			Started: started, Duration: c.now().Sub(started), Err: apiErr, Sent: w.effective(req), Reason: reason,
		})
	}
}

// learnFromRejection reads a 400 for a parameter the model's API refuses
// (web search, temperature, a forced tool_choice), remembers it per model so
// the next plan leaves it out, and returns the reason to retry once without
// it; "" means the 400 is the caller's to see.
func (c *Client) learnFromRejection(req llm.Request, w wire, e *APIError) string {
	msg := strings.ToLower(e.Message)
	switch {
	case w.webSearch && rejectsWebSearch(e):
		c.setWebSearch(req.Model, false)
		if replaysServerBlocks(req.Messages) {
			// A continuation's history holds search blocks; without the
			// tool the API would reject it again, less clearly.
			return ""
		}
		return "web_search_rejected_by_org"
	case w.container != "" && strings.Contains(msg, "container") && !pendingCodeCalls(req.Messages):
		// An expired or unknown container: the code execution docs say to
		// send again without it. Not with a programmatic call pending, whose
		// continuation requires the container.
		return "container_rejected_400"
	case w.codeExecution && rejectsCodeExecution(e):
		c.reject(req.Model, func(r *rejection) { r.codeExecution = true })
		return "code_execution_rejected_400"
	case w.temperature && strings.Contains(msg, "temperature"):
		c.reject(req.Model, func(r *rejection) { r.temperature = true })
		return "temperature_rejected_400"
	case w.thinking && strings.Contains(msg, "thinking"):
		c.reject(req.Model, func(r *rejection) { r.thinking = true })
		return "thinking_rejected_400"
	case w.effort != "" && (strings.Contains(msg, "effort") || strings.Contains(msg, "output_config")):
		c.reject(req.Model, func(r *rejection) { r.effort = true })
		return "effort_rejected_400"
	case w.forcedTool && (strings.Contains(msg, "tool_choice") || strings.Contains(msg, "tool choice")):
		c.reject(req.Model, func(r *rejection) { r.forcedTool = true })
		return "tool_choice_rejected_400"
	}
	return ""
}

func cutMessage(s string) string {
	if len(s) <= 300 {
		return s
	}
	return s[:300] + "…"
}

// replaysServerBlocks reports whether msgs carry server tool blocks (a
// pause_turn continuation).
func replaysServerBlocks(msgs []llm.Message) bool {
	for _, m := range msgs {
		for _, b := range m.Content {
			if b.Type == llm.ContentTypeServer {
				return true
			}
		}
	}
	return false
}

// pendingCodeCalls reports whether msgs end with the answers to tool calls
// made from code: the continuation of a paused code execution.
func pendingCodeCalls(msgs []llm.Message) bool {
	if len(msgs) < 2 || msgs[len(msgs)-1].Role != llm.RoleUser {
		return false
	}
	for _, b := range msgs[len(msgs)-2].Content {
		if b.Type == llm.ContentTypeToolUse && b.Caller.FromCode() {
			return true
		}
	}
	return false
}

// codeCallable lists the tools that allow a code execution caller.
func codeCallable(tools []llm.Tool) []string {
	var names []string
	for _, t := range tools {
		if slices.Contains(t.AllowedCallers, llm.CallerCodeExecution) {
			names = append(names, t.Name)
		}
	}
	return names
}

// rejectsCodeExecution reports whether a 400 refused the code execution tool
// or the allowed_callers field.
func rejectsCodeExecution(e *APIError) bool {
	msg := strings.ToLower(e.Message)
	return strings.Contains(msg, "code_execution") || strings.Contains(msg, "code execution") ||
		strings.Contains(msg, "allowed_callers") || strings.Contains(msg, "programmatic tool")
}

// post sends req as a streaming Messages request.
func (c *Client) post(ctx context.Context, req llm.Request, w wire) (*http.Response, error) {
	body, err := encodeRequest(req, true, w)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.buildRequest(ctx, http.MethodPost, "/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request (before response headers): %w", err)
	}
	return resp, nil
}

func (c *Client) buildRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", apiVersion)
	return req, nil
}

// ---------------------------------------------------------------------------
// Request encoding
// ---------------------------------------------------------------------------

type reqBody struct {
	Model        string           `json:"model"`
	MaxTokens    int              `json:"max_tokens"`
	Temperature  *float64         `json:"temperature,omitempty"`
	System       []systemBlock    `json:"system,omitempty"`
	Messages     []messageBody    `json:"messages"`
	Tools        []any            `json:"tools,omitempty"`
	ToolChoice   *toolChoiceBody  `json:"tool_choice,omitempty"`
	Thinking     *thinkingBody    `json:"thinking,omitempty"`
	OutputConfig *outputConfig    `json:"output_config,omitempty"`
	Stream       bool             `json:"stream,omitempty"`
	Container    string           `json:"container,omitempty"`
	Metadata     *requestMetadata `json:"metadata,omitempty"`
}

// toolChoiceBody is {"type":"tool","name":...} (forced) or {"type":"auto"}.
type toolChoiceBody struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// outputConfig is {"effort":"low"}.
type outputConfig struct {
	Effort string `json:"effort"`
}

// thinkingBody is {"type":"adaptive","display":"summarized"}.
type thinkingBody struct {
	Type    string `json:"type"`
	Display string `json:"display"`
}

type requestMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

type systemBlock struct {
	Type         string     `json:"type"`
	Text         string     `json:"text"`
	CacheControl *cacheCtrl `json:"cache_control,omitempty"`
}

type cacheCtrl struct {
	Type string `json:"type"`
	TTL  string `json:"ttl,omitempty"`
}

type messageBody struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	// Caller is replayed on a tool_use made by code (or directly).
	Caller *llm.Caller `json:"caller,omitempty"`

	raw json.RawMessage // a server block, sent verbatim
}

// MarshalJSON sends a server block's raw JSON unchanged.
func (b contentBlock) MarshalJSON() ([]byte, error) {
	if b.raw != nil {
		return b.raw, nil
	}
	type plain contentBlock
	return json.Marshal(plain(b))
}

type toolBody struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema"`
	CacheControl *cacheCtrl      `json:"cache_control,omitempty"`
	// AllowedCallers is sent only when the request carries the code
	// execution tool.
	AllowedCallers []string `json:"allowed_callers,omitempty"`
}

// codeExecutionTool is the code execution server tool programmatic tool
// calling needs.
type codeExecutionTool struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// webSearchTool is the server-side web search tool (D1: the basic version,
// no domain filters or location).
type webSearchTool struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	MaxUses int    `json:"max_uses,omitempty"`
}

// encodeRequest builds the Messages body from what plan decided to send.
// Temperature goes out when the caller set one and the model has not refused
// it; the Models API does not say whether a model accepts it, so a 400 naming
// it is retried without (see learnFromRejection). A forced tool_choice goes
// out when the capabilities allow it; otherwise the tool is only asked for in
// the system prompt. Web search goes out only when w says so.
func encodeRequest(req llm.Request, stream bool, w wire) ([]byte, error) {
	body := reqBody{
		Model:     req.Model,
		MaxTokens: w.maxTokens,
		Stream:    stream,
	}
	if w.temperature {
		t := req.Temperature
		body.Temperature = &t
	}
	if w.thinking {
		body.Thinking = &thinkingBody{Type: "adaptive", Display: "summarized"}
	}
	if w.effort != "" {
		body.OutputConfig = &outputConfig{Effort: w.effort}
	}
	system := req.System
	if req.ToolChoice != nil {
		if w.forcedTool {
			body.ToolChoice = &toolChoiceBody{Type: "tool", Name: req.ToolChoice.Name}
		} else {
			body.ToolChoice = &toolChoiceBody{Type: "auto"}
			if system != "" {
				system += "\n\n"
			}
			system += "Answer by calling `" + req.ToolChoice.Name + "`."
		}
	}
	if system != "" {
		body.System = []systemBlock{{
			Type: "text", Text: system,
			// Cache the (large) system prompt aggressively. Anthropic skips this
			// automatically for prompts under the minimum, so it's safe to set.
			CacheControl: &cacheCtrl{Type: "ephemeral", TTL: "5m"},
		}}
	}
	if w.codeExecution {
		body.Container = w.container
		body.Tools = append(body.Tools, codeExecutionTool{Type: llm.CallerCodeExecution, Name: "code_execution"})
	}
	if len(req.Tools) > 0 {
		for i, t := range req.Tools {
			tool := toolBody{
				Name: t.Name, Description: t.Description, InputSchema: t.Schema,
			}
			if w.codeExecution {
				tool.AllowedCallers = t.AllowedCallers
			}
			if i == len(req.Tools)-1 {
				// Cache the tail of the tool list so the full tool block counts
				// as part of the cached prefix on subsequent turns.
				tool.CacheControl = &cacheCtrl{Type: "ephemeral", TTL: "5m"}
			}
			body.Tools = append(body.Tools, tool)
		}
	}
	if w.webSearch {
		body.Tools = append(body.Tools, webSearchTool{
			Type: "web_search_20250305", Name: "web_search", MaxUses: req.WebSearch.MaxUses,
		})
	}
	body.Messages = make([]messageBody, len(req.Messages))
	for i, m := range req.Messages {
		body.Messages[i] = messageBody{
			Role:    string(m.Role),
			Content: encodeContent(m.Content),
		}
	}
	return json.Marshal(body)
}

func encodeContent(blocks []llm.ContentBlock) []contentBlock {
	out := make([]contentBlock, 0, len(blocks))
	// tool_result blocks must come first in their message; split and merge.
	var results, rest []contentBlock
	for _, b := range blocks {
		switch b.Type {
		case llm.ContentTypeText:
			rest = append(rest, contentBlock{Type: "text", Text: b.Text})
		case llm.ContentTypeToolUse:
			input := b.ToolUseInput
			if len(input) == 0 {
				input = json.RawMessage("{}")
			}
			rest = append(rest, contentBlock{
				Type: "tool_use", ID: b.ToolUseID, Name: b.ToolUseName, Input: input, Caller: b.Caller,
			})
		case llm.ContentTypeServer:
			rest = append(rest, contentBlock{raw: b.Raw})
		case llm.ContentTypeToolResult:
			content := toolResultString(b.ToolResultContent)
			results = append(results, contentBlock{
				Type:      "tool_result",
				ToolUseID: b.ToolResultID,
				Content:   content,
				IsError:   b.ToolResultIsError,
			})
		}
	}
	out = append(out, results...)
	out = append(out, rest...)
	return out
}

// toolResultString encodes a tool result for the wire. Anthropic accepts only
// a string or a list of content blocks as tool_result content, so a JSON
// object or array is sent as its JSON text; a JSON string is sent as is.
func toolResultString(raw json.RawMessage) json.RawMessage {
	var s string
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &s); err != nil {
			s = string(raw)
		}
	}
	out, _ := json.Marshal(s)
	return out
}

// ---------------------------------------------------------------------------
// Error decoding
// ---------------------------------------------------------------------------

type apiErrorBody struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// APIError represents a structured Anthropic API error.
type APIError struct {
	Status    int
	ErrorType string
	Message   string
	RawBody   string // capped
	RequestID string
	RateLimit string // retry-after and ratelimit headers
	// MidStream marks an `error` event inside a stream that started fine
	// (Status is 200 then).
	MidStream bool
}

func (e *APIError) Error() string {
	if e.MidStream {
		return fmt.Sprintf("anthropic: stream error %s: %s", e.ErrorType, e.Message)
	}
	return fmt.Sprintf("anthropic: %d %s: %s", e.Status, e.ErrorType, e.Message)
}

// capBody is llm.CapBody.
func capBody(s string) string { return llm.CapBody(s) }

// rejectsWebSearch reports whether a 400 refused the web search tool itself.
func rejectsWebSearch(e *APIError) bool {
	msg := strings.ToLower(e.Message)
	return strings.Contains(msg, "web search") || strings.Contains(msg, "web_search")
}

func decodeAPIError(resp *http.Response) *APIError {
	raw, _ := io.ReadAll(resp.Body)
	e := &APIError{
		Status: resp.StatusCode, RawBody: capBody(string(raw)), RequestID: resp.Header.Get("request-id"),
		RateLimit: llm.RateLimitHeaders(resp.Header),
	}
	var body apiErrorBody
	if err := json.Unmarshal(raw, &body); err == nil {
		e.ErrorType = body.Error.Type
		e.Message = body.Error.Message
	} else {
		e.Message = string(raw)
	}
	return e
}

// ProviderStatus implements llm.ProviderError.
func (e *APIError) ProviderStatus() int { return e.Status }

// ProviderErrorType implements llm.ProviderError.
func (e *APIError) ProviderErrorType() string { return e.ErrorType }

// ProviderInfo implements llm.ProviderError.
func (e *APIError) ProviderInfo() llm.ProviderErrorInfo {
	return llm.ProviderErrorInfo{
		Status: e.Status, Type: e.ErrorType, Message: e.Message,
		RequestID: e.RequestID, RateLimit: e.RateLimit, RawBody: e.RawBody,
	}
}
