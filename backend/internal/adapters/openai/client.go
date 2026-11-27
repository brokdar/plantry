// Package openai implements llm.Client against the OpenAI Chat Completions
// API using stdlib net/http (no SDK).
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

const (
	defaultBaseURL = "https://api.openai.com"
	// DefaultMaxTokens is used when llm.Request.MaxTokens is 0.
	DefaultMaxTokens = 4096
)

// Client implements llm.Client against /v1/chat/completions.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client

	mu sync.Mutex
	// noEffort holds the models that answered a 400 naming reasoning_effort;
	// they are sent no effort again for the life of the process.
	noEffort map[string]bool
}

// Option configures the client.
type Option func(*Client)

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithBaseURL overrides the default OpenAI base URL (for tests).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = u }
}

// New constructs a Client.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:     apiKey,
		baseURL:    defaultBaseURL,
		httpClient: llm.NewHTTPClient(),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// WebSearch implements llm.Capabilities: OpenAI runs no autonomous web
// search here (FIND-8), so the find pipeline asks and gets false.
func (c *Client) WebSearch(context.Context, string) (bool, error) { return false, nil }

// EffectiveParams implements llm.ParamsReporter.

func (c *Client) EffectiveParams(req llm.Request) llm.Effective {
	eff := llm.Effective{MaxTokens: req.MaxTokens}
	if eff.MaxTokens <= 0 {
		eff.MaxTokens = DefaultMaxTokens
	}
	if req.Temperature > 0 {
		t := req.Temperature
		eff.Temperature = &t
	}
	eff.Effort = c.effortFor(req)
	if req.Effort != "" && reasoningModel(req.Model) && eff.Effort == "" {
		eff.Fallbacks = append(eff.Fallbacks, effortRejectedFallback)
	}
	return eff
}

const effortRejectedFallback = "effort_rejected_by_model"

// effortFor is the reasoning_effort to send for req: none for a model that
// is not a known reasoning model or that refused it before.
func (c *Client) effortFor(req llm.Request) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.noEffort[req.Model] {
		return ""
	}
	return effort(req)
}

// learnEffortRejection reads a 400 naming reasoning_effort, remembers it for
// the model and says whether the call is worth retrying without it.
func (c *Client) learnEffortRejection(req llm.Request, e *APIError) bool {
	if !strings.Contains(strings.ToLower(e.Message), "reasoning_effort") {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.noEffort == nil {
		c.noEffort = map[string]bool{}
	}
	c.noEffort[req.Model] = true
	return true
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
		return "", fmt.Errorf("openai: nil response")
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

	for {
		resp, apiErr, err := c.post(ctx, req)
		if err != nil {
			return nil, err
		}
		if apiErr != nil {
			if apiErr.Status == http.StatusBadRequest && c.effortFor(req) != "" && c.learnEffortRejection(req, apiErr) {
				llm.ReportAttempt(ctx, llm.Attempt{
					Started: time.Now(), Err: apiErr, Sent: llm.Effective{Effort: req.Effort}, Reason: "effort_rejected_400",
				})
				continue
			}
			return nil, apiErr
		}
		defer func() { _ = resp.Body.Close() }()
		final, err := parseStream(ctx, resp.Body, out)
		if final != nil {
			final.RequestID = resp.Header.Get("x-request-id")
		}
		return final, err
	}
}

// post sends one chat-completions request; a non-200 answer comes back as
// the decoded *APIError with the body already closed.
func (c *Client) post(ctx context.Context, req llm.Request) (*http.Response, *APIError, error) {
	body, err := encodeRequest(req, true, c.effortFor(req))
	if err != nil {
		return nil, nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, nil, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("openai: request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		apiErr, _ := decodeAPIError(resp).(*APIError)
		if apiErr == nil {
			return nil, nil, fmt.Errorf("openai: status %d", resp.StatusCode)
		}
		return nil, apiErr, nil
	}
	return resp, nil, nil
}

// ---------------------------------------------------------------------------
// Request encoding
// ---------------------------------------------------------------------------

type reqBody struct {
	Model    string        `json:"model"`
	Messages []messageBody `json:"messages"`
	Tools    []toolBody    `json:"tools,omitempty"`
	// ToolChoice forces one function: {"type":"function","function":{"name":...}}.
	ToolChoice *toolChoiceBody `json:"tool_choice,omitempty"`
	Stream     bool            `json:"stream,omitempty"`
	// StreamOptions asks for the usage-only final chunk; without it a
	// streamed response carries no token counts.
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
	// OpenAI deprecated `max_tokens` for reasoning models (o1/o3/gpt-5.x).
	// `max_completion_tokens` is accepted by every current chat-completions
	// model, including the older gpt-4o family, so we send only the new key.
	MaxCompletionTokens int      `json:"max_completion_tokens,omitempty"`
	Temperature         *float64 `json:"temperature,omitempty"`
	// ReasoningEffort is sent only for reasoning models (see reasoningModel).
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

// reasoningPrefixes are the known reasoning models that accept
// reasoning_effort. OpenAI has no capabilities endpoint, so this is an
// explicit name allowlist; reasoningDenied lists variants of those families
// that reject it (chat aliases, pro models that only run at high, the early
// o1 models without it). A model outside the list gets no effort; one that
// is listed but refuses it is retried without (learnEffortRejection).
var (
	reasoningPrefixes = []string{"o1", "o3", "o4-mini", "gpt-5"}
	reasoningDenied   = []string{"o1-mini", "o1-preview", "-chat", "-pro", "-search"}
)

func reasoningModel(model string) bool {
	m := strings.ToLower(model)
	for _, d := range reasoningDenied {
		if strings.HasPrefix(m, d) || strings.Contains(m, d) {
			return false
		}
	}
	for _, p := range reasoningPrefixes {
		if strings.HasPrefix(m, p) {
			return true
		}
	}
	return false
}

// effort is the reasoning_effort to send for req ("" for none).
func effort(req llm.Request) string {
	if req.Effort == "" || !reasoningModel(req.Model) {
		return ""
	}
	return req.Effort
}

type toolChoiceBody struct {
	Type     string             `json:"type"`
	Function toolChoiceFunction `json:"function"`
}

type toolChoiceFunction struct {
	Name string `json:"name"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type messageBody struct {
	Role       string         `json:"role"`
	Content    *string        `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []toolCallBody `json:"tool_calls,omitempty"`
}

type toolCallBody struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function functionCallBody `json:"function"`
}

type functionCallBody struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolBody struct {
	Type     string       `json:"type"`
	Function functionDecl `json:"function"`
}

type functionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

func encodeRequest(req llm.Request, stream bool, effort string) ([]byte, error) {
	maxTok := req.MaxTokens
	if maxTok <= 0 {
		maxTok = DefaultMaxTokens
	}
	body := reqBody{
		Model:               req.Model,
		Stream:              stream,
		MaxCompletionTokens: maxTok,
	}
	if stream {
		body.StreamOptions = &streamOptions{IncludeUsage: true}
	}
	if req.Temperature > 0 {
		tmp := req.Temperature
		body.Temperature = &tmp
	}
	body.ReasoningEffort = effort
	// Top-level system message comes first.
	if req.System != "" {
		sys := req.System
		body.Messages = append(body.Messages, messageBody{Role: "system", Content: &sys})
	}
	for _, m := range req.Messages {
		body.Messages = append(body.Messages, convertMessage(m)...)
	}
	if len(req.Tools) > 0 {
		tools := make([]toolBody, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = toolBody{
				Type: "function",
				Function: functionDecl{
					Name: t.Name, Description: t.Description, Parameters: t.Schema,
				},
			}
		}
		body.Tools = tools
	}
	if req.ToolChoice != nil {
		body.ToolChoice = &toolChoiceBody{Type: "function", Function: toolChoiceFunction{Name: req.ToolChoice.Name}}
	}
	return json.Marshal(body)
}

// convertMessage translates one llm.Message into one or more OpenAI chat
// messages. Tool results become separate role=tool messages, each tied to
// its tool_call_id.
func convertMessage(m llm.Message) []messageBody {
	switch m.Role {
	case llm.RoleAssistant:
		out := messageBody{Role: "assistant"}
		var text string
		for _, b := range m.Content {
			switch b.Type {
			case llm.ContentTypeText:
				text += b.Text
			case llm.ContentTypeToolUse:
				args := string(llm.ObjectInput(b.ToolUseInput))
				out.ToolCalls = append(out.ToolCalls, toolCallBody{
					ID: b.ToolUseID, Type: "function",
					Function: functionCallBody{Name: b.ToolUseName, Arguments: args},
				})
			}
		}
		if text != "" {
			out.Content = &text
		}
		return []messageBody{out}

	case llm.RoleUser:
		// A single llm user message may carry tool_results (typical after an
		// assistant tool_use turn) plus free text. Each tool_result becomes
		// its own role=tool message.
		var msgs []messageBody
		var text string
		for _, b := range m.Content {
			switch b.Type {
			case llm.ContentTypeText:
				text += b.Text
			case llm.ContentTypeToolResult:
				content := toolResultAsString(b)
				msgs = append(msgs, messageBody{
					Role: "tool", ToolCallID: b.ToolResultID, Content: &content,
				})
			}
		}
		if text != "" {
			msgs = append(msgs, messageBody{Role: "user", Content: &text})
		}
		if len(msgs) == 0 {
			empty := ""
			msgs = append(msgs, messageBody{Role: "user", Content: &empty})
		}
		return msgs

	default:
		// system is encoded at the top of the message list, not here.
		return nil
	}
}

func toolResultAsString(b llm.ContentBlock) string {
	if len(b.ToolResultContent) == 0 {
		return ""
	}
	// If the content is already a JSON string, unwrap it so the model sees the
	// string text. Otherwise serialise as-is.
	var s string
	if err := json.Unmarshal(b.ToolResultContent, &s); err == nil {
		return s
	}
	return string(b.ToolResultContent)
}

// ---------------------------------------------------------------------------
// Error decoding
// ---------------------------------------------------------------------------

type apiErrorBody struct {
	Error struct {
		Type    string `json:"type"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Param   string `json:"param"`
	} `json:"error"`
}

// APIError represents a structured OpenAI API error.
type APIError struct {
	Status    int
	ErrorType string
	Code      string
	Message   string
	RawBody   string
	RequestID string
	RateLimit string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("openai: %d %s: %s", e.Status, e.ErrorType, e.Message)
}

func decodeAPIError(resp *http.Response) error {
	raw, _ := io.ReadAll(resp.Body)
	e := &APIError{Status: resp.StatusCode, RawBody: llm.CapBody(string(raw)), RequestID: resp.Header.Get("x-request-id"), RateLimit: llm.RateLimitHeaders(resp.Header)}
	var body apiErrorBody
	if err := json.Unmarshal(raw, &body); err == nil {
		e.ErrorType = body.Error.Type
		e.Code = body.Error.Code
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
