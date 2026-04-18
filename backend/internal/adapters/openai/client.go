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
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp)
	}

	return parseStream(ctx, resp.Body, out)
}

// ---------------------------------------------------------------------------
// Request encoding
// ---------------------------------------------------------------------------

type reqBody struct {
	Model    string        `json:"model"`
	Messages []messageBody `json:"messages"`
	Tools    []toolBody    `json:"tools,omitempty"`
	Stream   bool          `json:"stream,omitempty"`
	// OpenAI deprecated `max_tokens` for reasoning models (o1/o3/gpt-5.x).
	// `max_completion_tokens` is accepted by every current chat-completions
	// model, including the older gpt-4o family, so we send only the new key.
	MaxCompletionTokens int      `json:"max_completion_tokens,omitempty"`
	Temperature         *float64 `json:"temperature,omitempty"`
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

func encodeRequest(req llm.Request, stream bool) ([]byte, error) {
	maxTok := req.MaxTokens
	if maxTok <= 0 {
		maxTok = DefaultMaxTokens
	}
	body := reqBody{
		Model:               req.Model,
		Stream:              stream,
		MaxCompletionTokens: maxTok,
	}
	if req.Temperature > 0 {
		tmp := req.Temperature
		body.Temperature = &tmp
	}
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
				args := string(b.ToolUseInput)
				if args == "" {
					args = "{}"
				}
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
}

func (e *APIError) Error() string {
	return fmt.Sprintf("openai: %d %s: %s", e.Status, e.ErrorType, e.Message)
}

func decodeAPIError(resp *http.Response) error {
	raw, _ := io.ReadAll(resp.Body)
	e := &APIError{Status: resp.StatusCode, RawBody: string(raw)}
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
