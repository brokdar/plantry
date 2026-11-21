package airun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// Meter wraps inner so every call is recorded as an llm_call step of the run
// in ctx. A call made without a run gets its own untracked run. When inner
// implements llm.Capabilities, so does the returned client.
func (r *Recorder) Meter(inner llm.Client, provider string) llm.Client {
	m := &meteredClient{rec: r, inner: inner, provider: provider}
	if caps, ok := inner.(llm.Capabilities); ok {
		return &meteredCapsClient{meteredClient: m, caps: caps}
	}
	return m
}

// meteredCapsClient is a meteredClient whose inner client reports
// capabilities; the capability checks are not recorded.
type meteredCapsClient struct {
	*meteredClient
	caps llm.Capabilities
}

func (m *meteredCapsClient) WebSearch(ctx context.Context, model string) (bool, error) {
	return m.caps.WebSearch(ctx, model)
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
	inner := make(chan llm.Event, max(cap(out), 1))
	stop := make(chan struct{})
	done := make(chan struct{})
	var ttft *time.Duration
	go func() {
		defer close(done)
		defer close(out)
		forward := func(ev llm.Event) {
			if ttft == nil && (ev.Type == llm.EventAssistantDelta || ev.Type == llm.EventToolCallStart) {
				d := m.rec.now().Sub(started)
				ttft = &d
			}
			select {
			case out <- ev:
			case <-ctx.Done():
			}
		}
		for {
			select {
			case ev, ok := <-inner:
				if !ok {
					return
				}
				forward(ev)
			case <-stop:
				for { // the inner client returned: pass on what is buffered
					select {
					case ev, ok := <-inner:
						if !ok {
							return
						}
						forward(ev)
					default:
						return
					}
				}
			}
		}
	}()
	// Wire requests the adapter retried with other parameters (a 400 naming
	// temperature, tool_choice or web search) each become a failed step.
	var attempts []llm.Attempt
	var attemptsMu sync.Mutex
	ctx = llm.WithAttemptObserver(ctx, func(a llm.Attempt) {
		attemptsMu.Lock()
		attempts = append(attempts, a)
		attemptsMu.Unlock()
	})
	resp, err := m.inner.Stream(ctx, req, inner)
	close(stop)
	<-done
	// Read after the forwarder is done: it reads the clock for the first
	// token, and reading before it made the order of the two reads, and so a
	// stepping test clock's durations, depend on scheduling (a race-tier flake).
	finished := m.rec.now()
	attemptsMu.Lock()
	for _, a := range attempts {
		eff := a.Sent
		run.llmCall(ctx, m.provider, req, nil, a.Err, callInfo{
			started: a.Started, dur: a.Duration, eff: &eff, retryReason: a.Reason,
		})
	}
	attemptsMu.Unlock()
	if n := len(attempts); n > 0 { // the final attempt starts where the last failed one ended
		last := attempts[n-1]
		shifted := last.Started.Add(last.Duration)
		if ttft != nil {
			d := *ttft - shifted.Sub(started)
			ttft = &d
		}
		started = shifted
	}
	call := callInfo{started: started, dur: finished.Sub(started), ttft: ttft}
	if rep, ok := m.inner.(llm.ParamsReporter); ok {
		eff := rep.EffectiveParams(req)
		call.eff = &eff
	}
	run.llmCall(ctx, m.provider, req, resp, err, call)

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

// callInfo is what the metered client measured around one call.
type callInfo struct {
	started     time.Time
	dur         time.Duration
	ttft        *time.Duration // nil: no token or tool call streamed
	eff         *llm.Effective // nil: the client reports no effective parameters
	retryReason string         // a failed attempt the adapter retried: why it did
}

// llmCall folds one LLM call into the run totals and records it as a step
// whose request_json and response_json let the call be replayed. The
// provider, model and prompt hash of the first call are kept. A failed call
// stores the provider's error (and the partial reply of a broken stream) in
// response_json.
func (run *Run) llmCall(ctx context.Context, provider string, req llm.Request, resp *llm.Response, callErr error, call callInfo) {
	dur := call.dur
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
	run.webSearches += int64(usage.WebSearchRequests)
	run.reasoningTokens += int64(usage.ReasoningTokens)
	if run.provider == "" {
		run.provider = provider
		run.model = req.Model
	}
	if resp != nil && resp.ContainerID != "" {
		run.containerID = resp.ContainerID
	}
	// The call continues the last recorded one when it resends its messages
	// unchanged; otherwise the full history is stored from offset 0.
	offset := 0
	if n := len(run.lastMsgs); n > 0 && len(req.Messages) >= n && reflect.DeepEqual(run.lastMsgs, req.Messages[:n]) {
		offset = n
	}
	// Every distinct system prompt and tool set is stored once per run; the
	// run itself points at the first call's prompt.
	if run.firstPrompt == "" {
		run.firstPrompt = systemHash
	}
	firstPrompt := systemHash != "" && systemHash == run.firstPrompt
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
			run.mu.Lock()
			delete(run.prompts, systemHash)
			run.mu.Unlock()
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
			run.mu.Lock()
			delete(run.toolSchemas, toolsHash)
			run.mu.Unlock()
		}
	}

	step := Step{
		Seq:               seq,
		Type:              StepLLMCall,
		Name:              req.Model,
		IsError:           callErr != nil,
		DurationMs:        dur.Milliseconds(),
		InputTokens:       int64(usage.InputTokens),
		OutputTokens:      int64(usage.OutputTokens),
		WebSearchRequests: int64(usage.WebSearchRequests),
		StartedAt:         call.started,
		CacheReadTokens:   int64(usage.CacheReadTokens),
		CacheWriteTokens:  int64(usage.CacheWriteTokens),
		ReasoningTokens:   int64(usage.ReasoningTokens),
	}
	if call.ttft != nil {
		ms := call.ttft.Milliseconds()
		step.TTFTMs = &ms
	}
	var err error
	if step.RequestJSON, err = encodeRequest(req, call.eff, offset, systemHash, toolsHash); err != nil {
		run.warn(ctx, "ai.run.request encode failed", err)
	}
	step.ProviderRequestID = providerRequestID(resp, callErr)
	if resp != nil {
		step.ServedModel = resp.Model
		step.ThinkingChars = thinkingChars(resp.Message.Content)
	}
	switch {
	case callErr == nil && resp != nil:
		step.StopReason = string(resp.StopReason)
		if step.ResponseJSON, err = encodeResponse(resp, nil, call); err != nil {
			run.warn(ctx, "ai.run.response encode failed", err)
		}
	case callErr != nil:
		if step.ResponseJSON, err = encodeResponse(resp, callErr, call); err != nil {
			run.warn(ctx, "ai.run.response encode failed", err)
		}
	}
	// Callers grow their history append-only (the agent loop, the import
	// retry), so a continuing call resends the recorded messages unchanged.
	// The slice is copied so a caller reusing its backing array cannot make
	// a changed history look like a continuation; the messages' own content
	// is assumed not to be mutated after the call. A step that was not
	// stored resets the history, so the next call is recorded from offset 0
	// instead of continuing a call replay cannot see.
	run.logLLMCall(ctx, provider, req, step, callErr, call, resp)
	var lastMsgs []llm.Message
	if run.addStep(ctx, step) {
		lastMsgs = append([]llm.Message(nil), req.Messages...)
	}
	if resp != nil {
		run.recordCode(ctx, resp, call)
	}
	run.mu.Lock()
	run.lastMsgs = lastMsgs
	run.mu.Unlock()
}

// providerRequestID is the provider's id of the call: the response's, else
// the one a provider error carries.
func providerRequestID(resp *llm.Response, callErr error) string {
	if resp != nil && resp.RequestID != "" {
		return resp.RequestID
	}
	var pe llm.ProviderError
	if errors.As(callErr, &pe) {
		return pe.ProviderInfo().RequestID
	}
	return ""
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
	MessageOffset int             `json:"message_offset"`
	Messages      []llm.Message   `json:"messages"`
	ToolChoice    *llm.ToolChoice `json:"tool_choice,omitempty"`
	WebSearch     *llm.WebSearch  `json:"web_search,omitempty"`
	// Temperature and MaxTokens are the effective values sent (nil
	// temperature: none was sent).
	Temperature *float64 `json:"temperature"`
	MaxTokens   int      `json:"max_tokens"`
	// Sent is what went over the wire when the client says so: tool_choice
	// and web_search above are what the caller asked for.
	Sent       *sentBody `json:"sent,omitempty"`
	SystemHash *string   `json:"system_hash"`
	ToolsHash  *string   `json:"tools_hash"`
	Truncated  bool      `json:"truncated"`
}

// sentBody is the tool choice ("forced", "auto" or ""), web search and the
// reasons for every fallback of one wire request (see llm.Effective).
type sentBody struct {
	ToolChoice string   `json:"tool_choice"`
	WebSearch  bool     `json:"web_search"`
	Thinking   bool     `json:"thinking"`
	Fallbacks  []string `json:"fallbacks,omitempty"`
	// Every other provider parameter except system, tools and messages.
	ThinkingType     string   `json:"thinking_type,omitempty"`
	ThinkingDisplay  string   `json:"thinking_display,omitempty"`
	Effort           string   `json:"effort,omitempty"`
	Betas            []string `json:"betas,omitempty"`
	CacheBreakpoints []string `json:"cache_breakpoints,omitempty"`
	// CodeExecution says the code execution tool went out, CodeTools which
	// tools allowed a code caller and Container the container id sent.
	CodeExecution bool     `json:"code_execution"`
	CodeTools     []string `json:"code_tools,omitempty"`
	Container     string   `json:"container,omitempty"`
}

// responseBody is the stored response_json: the reply's content blocks and
// stop reason. A failed call also carries Error, and Partial when the reply
// broke off mid-stream (the content is what streamed before the break).
type responseBody struct {
	Content    []llm.ContentBlock `json:"content"`
	StopReason llm.StopReason     `json:"stop_reason"`
	Partial    bool               `json:"partial,omitempty"`
	// ContainerID is the code execution container the reply names.
	ContainerID string     `json:"container_id,omitempty"`
	Error       *errorBody `json:"error,omitempty"`
	Truncated   bool       `json:"truncated,omitempty"`
}

// errorBody is why a call failed: the provider's answer (status, type,
// message, request id, rate limit headers, capped raw body), or just the
// error text for a failure without one (network, decode). AfterFirstToken
// says whether a token had streamed when it failed; RetryReason is set on an
// attempt the adapter retried with other parameters.
type errorBody struct {
	llm.ProviderErrorInfo
	Text            string `json:"text,omitempty"`
	AfterFirstToken bool   `json:"after_first_token"`
	RetryReason     string `json:"retry_reason,omitempty"`
}

func encodeRequest(req llm.Request, eff *llm.Effective, offset int, systemHash, toolsHash string) (string, error) {
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
		ToolChoice:    req.ToolChoice,
		WebSearch:     req.WebSearch,
		MaxTokens:     req.MaxTokens,
		SystemHash:    nilIfEmpty(systemHash),
		ToolsHash:     nilIfEmpty(toolsHash),
	}
	if req.Temperature > 0 {
		t := req.Temperature
		body.Temperature = &t
	}
	if eff != nil {
		body.Temperature, body.MaxTokens = eff.Temperature, eff.MaxTokens
		body.Sent = &sentBody{
			ToolChoice: eff.ToolChoice, WebSearch: eff.WebSearch, Thinking: eff.Thinking, Fallbacks: eff.Fallbacks,
			ThinkingType: eff.ThinkingType, ThinkingDisplay: eff.ThinkingDisplay, Effort: eff.Effort, Betas: eff.Betas, CacheBreakpoints: eff.CacheBreakpoints,
			CodeExecution: eff.CodeExecution, CodeTools: eff.CodeTools, Container: eff.Container,
		}
	}
	return encodeCapped(&body, blocks, &body.Truncated)
}

// encodeResponse is the stored response_json of a call: its reply, or for a
// failed call (callErr set) the error plus whatever streamed before it.
func encodeResponse(resp *llm.Response, callErr error, call callInfo) (string, error) {
	content := []llm.ContentBlock{}
	body := responseBody{}
	if resp != nil {
		content = append(content, resp.Message.Content...)
		body.StopReason, body.Partial, body.ContainerID = resp.StopReason, resp.Partial, resp.ContainerID
	}
	body.Content = content
	if callErr != nil {
		eb := &errorBody{AfterFirstToken: call.ttft != nil, RetryReason: call.retryReason}
		var pe llm.ProviderError
		if errors.As(callErr, &pe) {
			eb.ProviderErrorInfo = pe.ProviderInfo()
		}
		if eb.Status == 0 { // a provider answer is structured above; the text of other errors is all there is
			eb.Text = cutChars(callErr.Error(), 500)
		}
		body.Error = eb
	}
	blocks := make([]*llm.ContentBlock, len(content))
	for i := range content {
		blocks[i] = &content[i]
	}
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

// shrinkLargest cuts the largest text, tool input, tool result or server block among blocks
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
		for _, r := range []*json.RawMessage{&b.ToolUseInput, &b.ToolResultContent, &b.Raw} {
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
