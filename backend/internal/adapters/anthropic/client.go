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

	now  func() time.Time
	mu   sync.Mutex
	caps map[string]capability // web search support per model
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
		httpClient: http.DefaultClient,
		now:        time.Now,
		caps:       map[string]capability{},
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

	resp, err := c.post(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusBadRequest && req.WebSearch != nil {
		// Web search can be turned off for the organisation (D3): run the
		// same turn without it and stop offering it for a while. Other 400s
		// are the caller's to see.
		apiErr := decodeAPIError(resp)
		_ = resp.Body.Close()
		if !rejectsWebSearch(apiErr) {
			return nil, apiErr
		}
		c.setWebSearch(req.Model, false)
		slog.WarnContext(ctx, "anthropic.web_search.rejected",
			"model", req.Model, "request_id", apiErr.RequestID, "message", apiErr.Message)
		if replaysServerBlocks(req.Messages) {
			// A continuation's history holds search blocks; without the
			// tool the API would reject it again, less clearly.
			return nil, apiErr
		}
		req.WebSearch = nil
		if resp, err = c.post(ctx, req); err != nil {
			return nil, err
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp)
	}

	return parseStream(ctx, resp.Body, out)
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

// post sends req as a streaming Messages request.
func (c *Client) post(ctx context.Context, req llm.Request) (*http.Response, error) {
	body, err := encodeRequest(req, true)
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
		return nil, fmt.Errorf("anthropic: request: %w", err)
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
	Model       string           `json:"model"`
	MaxTokens   int              `json:"max_tokens"`
	Temperature *float64         `json:"temperature,omitempty"`
	System      []systemBlock    `json:"system,omitempty"`
	Messages    []messageBody    `json:"messages"`
	Tools       []any            `json:"tools,omitempty"`
	ToolChoice  *toolChoiceBody  `json:"tool_choice,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
	Metadata    *requestMetadata `json:"metadata,omitempty"`
}

// toolChoiceBody is {"type":"tool","name":...} (forced) or {"type":"auto"}.
type toolChoiceBody struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
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
}

// webSearchTool is the server-side web search tool (D1: the basic version,
// no domain filters or location).
type webSearchTool struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	MaxUses int    `json:"max_uses,omitempty"`
}

func encodeRequest(req llm.Request, stream bool) ([]byte, error) {
	maxTok := req.MaxTokens
	if maxTok <= 0 {
		maxTok = DefaultMaxTokens
	}
	body := reqBody{
		Model:     req.Model,
		MaxTokens: maxTok,
		Stream:    stream,
	}
	rules := rulesFor(req.Model)
	if req.Temperature > 0 && rules.Sampling {
		tmp := req.Temperature
		body.Temperature = &tmp
	}
	system := req.System
	if req.ToolChoice != nil {
		if rules.ForcedTool {
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
	if len(req.Tools) > 0 {
		for i, t := range req.Tools {
			tool := toolBody{
				Name: t.Name, Description: t.Description, InputSchema: t.Schema,
			}
			if i == len(req.Tools)-1 {
				// Cache the tail of the tool list so the full tool block counts
				// as part of the cached prefix on subsequent turns.
				tool.CacheControl = &cacheCtrl{Type: "ephemeral", TTL: "5m"}
			}
			body.Tools = append(body.Tools, tool)
		}
	}
	if req.WebSearch != nil {
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
				Type: "tool_use", ID: b.ToolUseID, Name: b.ToolUseName, Input: input,
			})
		case llm.ContentTypeServer:
			rest = append(rest, contentBlock{raw: b.Raw})
		case llm.ContentTypeToolResult:
			content := b.ToolResultContent
			if len(content) == 0 {
				content = json.RawMessage(`""`)
			}
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
	RawBody   string
	RequestID string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("anthropic: %d %s: %s", e.Status, e.ErrorType, e.Message)
}

// rejectsWebSearch reports whether a 400 refused the web search tool itself.
func rejectsWebSearch(e *APIError) bool {
	msg := strings.ToLower(e.Message)
	return strings.Contains(msg, "web search") || strings.Contains(msg, "web_search")
}

func decodeAPIError(resp *http.Response) *APIError {
	raw, _ := io.ReadAll(resp.Body)
	e := &APIError{Status: resp.StatusCode, RawBody: string(raw), RequestID: resp.Header.Get("request-id")}
	var body apiErrorBody
	if err := json.Unmarshal(raw, &body); err == nil {
		e.ErrorType = body.Error.Type
		e.Message = body.Error.Message
	} else {
		e.Message = string(raw)
	}
	return e
}
