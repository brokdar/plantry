// Package anthropic implements llm.Client against the Anthropic Messages API
// using stdlib net/http (no SDK).
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

const (
	defaultBaseURL = "https://api.anthropic.com"
	apiVersion     = "2023-06-01"
	// DefaultMaxTokens is used when llm.Request.MaxTokens is 0.
	DefaultMaxTokens = 4096
)

// Client implements llm.Client.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
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
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Stream implements llm.Client.Stream.
func (c *Client) Stream(ctx context.Context, req llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	defer close(out)

	body, err := encodeRequest(req, true)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.buildRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp)
	}

	return parseStream(ctx, resp.Body, out)
}

func (c *Client) buildRequest(ctx context.Context, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", apiVersion)
	return req, nil
}

// ---------------------------------------------------------------------------
// Request encoding
// ---------------------------------------------------------------------------

type reqBody struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	System    []systemBlock    `json:"system,omitempty"`
	Messages  []messageBody    `json:"messages"`
	Tools     []toolBody       `json:"tools,omitempty"`
	Stream    bool             `json:"stream,omitempty"`
	Metadata  *requestMetadata `json:"metadata,omitempty"`
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
}

type toolBody struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema"`
	CacheControl *cacheCtrl      `json:"cache_control,omitempty"`
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
	if req.System != "" {
		body.System = []systemBlock{{
			Type: "text", Text: req.System,
			// Cache the (large) system prompt aggressively. Anthropic skips this
			// automatically for prompts under the minimum, so it's safe to set.
			CacheControl: &cacheCtrl{Type: "ephemeral", TTL: "5m"},
		}}
	}
	if len(req.Tools) > 0 {
		tools := make([]toolBody, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = toolBody{
				Name: t.Name, Description: t.Description, InputSchema: t.Schema,
			}
		}
		// Cache the tail of the tool list so the full tool block counts as part
		// of the cached prefix on subsequent turns.
		tools[len(tools)-1].CacheControl = &cacheCtrl{Type: "ephemeral", TTL: "5m"}
		body.Tools = tools
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

func decodeAPIError(resp *http.Response) error {
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
