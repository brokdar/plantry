package airun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// Meter wraps inner so every call is recorded as an llm_call step of the run
// in ctx. A call made without a run gets its own untracked run.
func (r *Recorder) Meter(inner llm.Client, provider string) llm.Client {
	return &meteredClient{rec: r, inner: inner, provider: provider}
}

type meteredClient struct {
	rec      *Recorder
	inner    llm.Client
	provider string
}

// Stream calls inner.Stream and records the call. The response and error are
// returned unchanged.
func (m *meteredClient) Stream(ctx context.Context, req llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	run := FromContext(ctx)
	implicit := run == nil
	if implicit {
		ctx, run = m.rec.Start(ctx, Start{Kind: KindUntracked})
	}

	started := m.rec.now()
	resp, err := m.inner.Stream(ctx, req, out)
	run.llmCall(ctx, m.provider, req, resp, err, m.rec.now().Sub(started))

	if implicit {
		status := StatusOK
		switch {
		case errors.Is(err, context.Canceled):
			status = StatusCancelled
		case err != nil:
			status = StatusError
		}
		run.Finish(ctx, status, "")
	}
	return resp, err
}

// Complete goes through Stream so tokens are recorded. Every adapter's
// Complete is Stream plus concatenating the response's text blocks (see the
// llm.Client contract), so this returns the same text inner.Complete would.
func (m *meteredClient) Complete(ctx context.Context, req llm.Request) (string, error) {
	events := make(chan llm.Event, 16)
	done := make(chan struct{})
	go func() {
		for range events {
		}
		close(done)
	}()
	resp, err := m.Stream(ctx, req, events)
	<-done
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", errors.New("airun: nil response")
	}
	var sb strings.Builder
	for _, block := range resp.Message.Content {
		if block.Type == llm.ContentTypeText {
			sb.WriteString(block.Text)
		}
	}
	return sb.String(), nil
}

// llmCall folds one LLM call into the run totals and records it as a step
// whose request_json and response_json let the call be replayed. The
// provider, model and prompt hash of the first call are kept.
func (run *Run) llmCall(ctx context.Context, provider string, req llm.Request, resp *llm.Response, callErr error, dur time.Duration) {
	var usage llm.Usage
	if resp != nil {
		usage = resp.Usage
	}
	systemHash := ""
	if req.System != "" {
		systemHash = sha256Hex([]byte(req.System))
	}
	toolsHash, toolsJSON := "", []byte(nil)
	if len(req.Tools) > 0 {
		var err error
		if toolsJSON, err = json.Marshal(req.Tools); err != nil {
			run.warn(ctx, "ai.run.tools failed", err)
		} else {
			toolsHash = sha256Hex(toolsJSON)
		}
	}

	run.mu.Lock()
	run.seq++
	seq := run.seq
	run.iterations++
	run.inputTokens += int64(usage.InputTokens)
	run.outputTokens += int64(usage.OutputTokens)
	run.cacheReadTokens += int64(usage.CacheReadTokens)
	run.cacheWriteTokens += int64(usage.CacheWriteTokens)
	if run.provider == "" {
		run.provider = provider
		run.model = req.Model
	}
	// The call continues the last recorded one when it resends its messages
	// unchanged; otherwise the full history is stored from offset 0.
	offset := 0
	if n := len(run.lastMsgs); n > 0 && len(req.Messages) >= n && reflect.DeepEqual(run.lastMsgs, req.Messages[:n]) {
		offset = n
	}
	run.lastMsgs = req.Messages
	// Every distinct system prompt and tool set is stored once per run; the
	// run itself points at the first call's prompt.
	firstPrompt := systemHash != "" && !run.promptTried
	if firstPrompt {
		run.promptTried = true
	}
	storePrompt := systemHash != "" && !run.prompts[systemHash] && run.id != 0
	if storePrompt {
		if run.prompts == nil {
			run.prompts = map[string]bool{}
		}
		run.prompts[systemHash] = true
	}
	storeTools := toolsHash != "" && !run.toolSchemas[toolsHash] && run.id != 0
	if storeTools {
		if run.toolSchemas == nil {
			run.toolSchemas = map[string]bool{}
		}
		run.toolSchemas[toolsHash] = true
	}
	run.mu.Unlock()

	if storePrompt {
		if err := run.rec.store.UpsertPrompt(context.WithoutCancel(ctx), systemHash, req.System); err != nil {
			run.warn(ctx, "ai.run.prompt failed", err)
		} else if firstPrompt {
			// Set only once stored, so the run never points at a missing prompt.
			run.mu.Lock()
			run.promptHash = systemHash
			run.mu.Unlock()
		}
	}
	if storeTools {
		if err := run.rec.store.UpsertToolSchema(context.WithoutCancel(ctx), toolsHash, string(toolsJSON)); err != nil {
			run.warn(ctx, "ai.run.tools failed", err)
		}
	}

	step := Step{
		Seq:          seq,
		Type:         StepLLMCall,
		Name:         req.Model,
		IsError:      callErr != nil,
		DurationMs:   dur.Milliseconds(),
		InputTokens:  int64(usage.InputTokens),
		OutputTokens: int64(usage.OutputTokens),
	}
	var err error
	if step.RequestJSON, err = encodeRequest(req, offset, systemHash, toolsHash); err != nil {
		run.warn(ctx, "ai.run.request encode failed", err)
	}
	if callErr == nil && resp != nil {
		step.StopReason = string(resp.StopReason)
		if step.ResponseJSON, err = encodeResponse(resp); err != nil {
			run.warn(ctx, "ai.run.response encode failed", err)
		}
	}
	run.addStep(ctx, step)
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// maxBodyBytes caps request_json and response_json of an llm_call step.
const maxBodyBytes = 256 << 10

// cutMarker ends a payload shortened to fit maxBodyBytes.
const cutMarker = "…[truncated]"

// minCutBytes is the smallest payload worth shortening; anything shorter
// would barely shrink once the marker is appended.
const minCutBytes = 64

// requestBody is the stored request_json: the messages the call appended
// (Messages[MessageOffset:]), its parameters, and the hashes of its system
// prompt (ai_prompts) and tools (ai_tool_schemas), null when absent.
type requestBody struct {
	MessageOffset int           `json:"message_offset"`
	Messages      []llm.Message `json:"messages"`
	Temperature   float64       `json:"temperature"`
	MaxTokens     int           `json:"max_tokens"`
	SystemHash    *string       `json:"system_hash"`
	ToolsHash     *string       `json:"tools_hash"`
	Truncated     bool          `json:"truncated"`
}

// responseBody is the stored response_json: the reply's content blocks.
type responseBody struct {
	Content    []llm.ContentBlock `json:"content"`
	StopReason llm.StopReason     `json:"stop_reason"`
	Truncated  bool               `json:"truncated,omitempty"`
}

func encodeRequest(req llm.Request, offset int, systemHash, toolsHash string) (string, error) {
	delta := make([]llm.Message, len(req.Messages)-offset)
	var blocks []*llm.ContentBlock
	for i, m := range req.Messages[offset:] {
		delta[i] = llm.Message{Role: m.Role, Content: append([]llm.ContentBlock(nil), m.Content...)}
		for j := range delta[i].Content {
			blocks = append(blocks, &delta[i].Content[j])
		}
	}
	body := requestBody{
		MessageOffset: offset,
		Messages:      delta,
		Temperature:   req.Temperature,
		MaxTokens:     req.MaxTokens,
		SystemHash:    nilIfEmpty(systemHash),
		ToolsHash:     nilIfEmpty(toolsHash),
	}
	return encodeCapped(&body, blocks, &body.Truncated)
}

func encodeResponse(resp *llm.Response) (string, error) {
	content := append([]llm.ContentBlock{}, resp.Message.Content...)
	blocks := make([]*llm.ContentBlock, len(content))
	for i := range content {
		blocks[i] = &content[i]
	}
	body := responseBody{Content: content, StopReason: resp.StopReason}
	return encodeCapped(&body, blocks, &body.Truncated)
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// encodeCapped marshals body. While the result exceeds maxBodyBytes it
// shortens the largest payload among blocks (which body references) and sets
// *truncated, so the stored body stays valid JSON.
func encodeCapped(body any, blocks []*llm.ContentBlock, truncated *bool) (string, error) {
	data, err := json.Marshal(body)
	for err == nil && len(data) > maxBodyBytes {
		if !shrinkLargest(blocks, len(data)-maxBodyBytes) {
			break // nothing left to shorten
		}
		*truncated = true
		data, err = json.Marshal(body)
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// shrinkLargest cuts the largest text, tool input or tool result among blocks
// by at least excess bytes. A cut JSON payload is replaced by a JSON string
// holding its shortened text. It reports false when no payload is long
// enough to cut.
func shrinkLargest(blocks []*llm.ContentBlock, excess int) bool {
	var text *string
	var raw *json.RawMessage
	size := minCutBytes
	for _, b := range blocks {
		if len(b.Text) > size {
			text, raw, size = &b.Text, nil, len(b.Text)
		}
		for _, r := range []*json.RawMessage{&b.ToolUseInput, &b.ToolResultContent} {
			if len(*r) > size {
				text, raw, size = nil, r, len(*r)
			}
		}
	}
	switch {
	case text != nil:
		*text = cutUTF8(*text, max(len(*text)-excess-len(cutMarker), 0)) + cutMarker
	case raw != nil:
		// Cut the text of a payload already turned into a JSON string, so
		// repeated cuts never escape it twice.
		s := string(*raw)
		var str string
		if s[0] == '"' && json.Unmarshal(*raw, &str) == nil {
			s = str
		}
		cut, err := json.Marshal(cutUTF8(s, max(len(s)-excess-len(cutMarker), 0)) + cutMarker)
		if err != nil {
			return false
		}
		*raw = cut
	default:
		return false
	}
	return true
}

// cutUTF8 returns the longest prefix of s of at most n bytes that ends on a
// rune boundary.
func cutUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
