package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// capabilityTTL is how long a model's capabilities are trusted (D3);
// transientTTL how long a failed check counts as "unknown" before it is
// retried; lookupTimeout bounds one capability lookup.
const (
	capabilityTTL = 24 * time.Hour
	transientTTL  = 5 * time.Minute
	lookupTimeout = 5 * time.Second
)

// modelsPageLimit is the page size of GET /v1/models (the API allows 1000).
const modelsPageLimit = 1000

// support is the API's {"supported": bool} capability object.
type support struct {
	Supported bool `json:"supported"`
}

// ModelInfo is one entry of GET /v1/models (the fields Plantry uses).
type ModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	CreatedAt   string `json:"created_at"`
	Lifecycle   string `json:"lifecycle"`
	MaxTokens   int    `json:"max_tokens"`

	Capabilities *struct {
		Thinking *struct {
			Supported bool `json:"supported"`
			Types     *struct {
				Disabled *support `json:"disabled"`
				Adaptive *support `json:"adaptive"`
			} `json:"types"`
		} `json:"thinking"`
		ServerTools *struct {
			WebSearch *support `json:"web_search"`
		} `json:"server_tools"`
	} `json:"capabilities"`
}

// WebSearch reports capabilities.server_tools.web_search.supported.
func (m ModelInfo) WebSearch() bool {
	c := m.Capabilities
	return c != nil && c.ServerTools != nil && c.ServerTools.WebSearch != nil && c.ServerTools.WebSearch.Supported
}

// ForcedTool reports whether the model accepts a forced tool_choice. The API
// rejects tool_choice "tool"/"any" while thinking is on, so a model that
// cannot turn thinking off (thinking.types.disabled.supported is false: it
// always thinks) cannot be forced. Without capability data the answer is
// false: `auto` plus a system line works on every model.
func (m ModelInfo) ForcedTool() bool {
	c := m.Capabilities
	if c == nil || c.Thinking == nil {
		return false
	}
	if !c.Thinking.Supported {
		return true
	}
	return c.Thinking.Types != nil && c.Thinking.Types.Disabled != nil && c.Thinking.Types.Disabled.Supported
}

// Thinking reports whether the model can think and accepts adaptive thinking
// (the types.adaptive entry, when the API lists one, must not say false). Its
// summary is requested with display "summarized": Claude 5.x and Haiku 5.5
// think by default with display "omitted", which leaves the trace empty.
func (m ModelInfo) Thinking() bool {
	c := m.Capabilities
	if c == nil || c.Thinking == nil || !c.Thinking.Supported {
		return false
	}
	t := c.Thinking.Types
	return t == nil || t.Adaptive == nil || t.Adaptive.Supported
}

type capability struct {
	info  ModelInfo
	known bool // false: the call failed or the model is unknown to the API
	// failure says why the lookup failed (status and type, no key) when it did.
	failure string
	// webSearch overrides info after the API rejected a search request.
	webSearch *bool
	until     time.Time
}

func (c capability) hasWebSearch() bool {
	if c.webSearch != nil {
		return *c.webSearch
	}
	return c.known && c.info.WebSearch()
}

// rejection is what a model's API answered 400 to: a parameter it does not
// accept although the capabilities said so (or said nothing). Kept per model
// for the life of the process.
type rejection struct {
	temperature bool
	forcedTool  bool
	thinking    bool
}

// flight is one capability lookup in progress; waiters block on done.
type flight struct {
	done chan struct{}
	cp   capability
	err  error
}

// WebSearch implements llm.Capabilities. It reads
// capabilities.server_tools.web_search.supported from GET /v1/models/{model}
// once per model per 24 h. A missing field, or a 400/404 for the model,
// counts as unsupported for 24 h too; any other failure (network, 5xx, 429,
// 401, an undecodable body) counts as unsupported for 5 min only. err reports
// the failed call only on the call that made it.
func (c *Client) WebSearch(ctx context.Context, model string) (bool, error) {
	cp, err := c.capability(ctx, model)
	return cp.hasWebSearch(), err
}

// capability returns the cached capabilities of model, fetching them when
// missing or stale. One lookup per model runs at a time, outside the lock,
// bounded by lookupTimeout and detached from the caller's ctx; concurrent
// callers wait for it (or their own ctx). err reports the failed call only
// to the caller that started it.
func (c *Client) capability(ctx context.Context, model string) (capability, error) {
	c.mu.Lock()
	if cp, ok := c.caps[model]; ok && c.now().Before(cp.until) {
		c.mu.Unlock()
		return cp, nil
	}
	f, running := c.flights[model]
	if !running {
		f = &flight{done: make(chan struct{})}
		c.flights[model] = f
		go c.lookup(context.WithoutCancel(ctx), model, f)
	}
	c.mu.Unlock()

	select {
	case <-f.done:
		if running {
			return f.cp, nil
		}
		return f.cp, f.err
	case <-ctx.Done():
		return capability{}, ctx.Err() // the caller's cancellation says nothing about the model
	}
}

// lookup fetches model's capabilities for f and caches the answer.
func (c *Client) lookup(ctx context.Context, model string, f *flight) {
	ctx, cancel := context.WithTimeout(ctx, c.lookupTimeout)
	defer cancel()
	info, err := c.fetchModel(ctx, model)
	ttl := capabilityTTL
	if err != nil && !definitive(err) {
		ttl = transientTTL
	}
	cp := capability{info: info, known: err == nil, until: c.now().Add(ttl)}
	if err != nil {
		cp.failure = failureText(err)
		llm.Log(ctx, slog.LevelWarn, "anthropic.model.capabilities failed",
			"model", model, "failure", cp.failure, "cached_for", ttl.String(),
			"effect", "forced tool_choice replaced by auto, max_tokens not clamped")
	}
	f.cp, f.err = cp, err
	c.mu.Lock()
	c.caps[model] = cp
	delete(c.flights, model)
	c.mu.Unlock()
	close(f.done)
}

// failureText says what failed without the key or the body: provider
// errors give status and type, others the error text.
func failureText(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return fmt.Sprintf("%d %s", apiErr.Status, apiErr.ErrorType)
	}
	return err.Error()
}

// cachedCapability returns the cache entry of model without calling the API.
func (c *Client) cachedCapability(model string) capability {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.caps[model]
}

// rejected returns what model's API has refused so far.
func (c *Client) rejected(model string) rejection {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rejects[model]
}

// definitive reports whether a failed Models API call answers for the model
// itself (it is unknown or rejected) rather than failing transiently.
func definitive(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) &&
		(apiErr.Status == http.StatusBadRequest || apiErr.Status == http.StatusNotFound)
}

// setWebSearch caches a known answer for model, e.g. after the API rejected
// a search request.
func (c *Client) setWebSearch(model string, supported bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := c.caps[model]
	cp.webSearch = &supported
	cp.until = c.now().Add(capabilityTTL)
	c.caps[model] = cp
}

// reject remembers that model's API refused the parameter.
func (c *Client) reject(model string, update func(*rejection)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.rejects[model]
	update(&r)
	c.rejects[model] = r
}

func (c *Client) fetchModel(ctx context.Context, model string) (ModelInfo, error) {
	var info ModelInfo
	err := c.getJSON(ctx, "/v1/models/"+url.PathEscape(model), &info)
	return info, err
}

// ListModels returns every model GET /v1/models lists (active and
// deprecated; the API leaves retired ones out), following has_more. It is
// also the cheapest authenticated call, so it doubles as the API key check:
// the error is an *APIError whose Status is 401 for a rejected key and 403
// for a key that is valid but not permitted.
func (c *Client) ListModels(ctx context.Context) ([]ModelInfo, error) {
	var out []ModelInfo
	after := ""
	for {
		path := "/v1/models?limit=" + strconv.Itoa(modelsPageLimit)
		if after != "" {
			path += "&after_id=" + url.QueryEscape(after)
		}
		var page struct {
			Data    []ModelInfo `json:"data"`
			HasMore bool        `json:"has_more"`
			LastID  string      `json:"last_id"`
		}
		if err := c.getJSON(ctx, path, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Data...)
		if !page.HasMore || page.LastID == "" {
			return out, nil
		}
		after = page.LastID
	}
}

func (c *Client) getJSON(ctx context.Context, path string, into any) error {
	req, err := c.buildRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("anthropic: models request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return decodeAPIError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("anthropic: models decode: %w", err)
	}
	return nil
}

// wire is what encodeRequest puts on the wire for req given the cached
// capabilities and what the API has refused so far.
type wire struct {
	forcedTool  bool
	temperature bool
	webSearch   bool
	thinking    bool // adaptive thinking with a summarized display
	maxTokens   int
	fallbacks   []string
}

// plan decides the wire parameters of req.
func (c *Client) plan(req llm.Request) wire {
	cp := c.cachedCapability(req.Model)
	rej := c.rejected(req.Model)
	w := wire{maxTokens: req.MaxTokens}
	if w.maxTokens <= 0 {
		w.maxTokens = DefaultMaxTokens
	}
	if cp.known && cp.info.MaxTokens > 0 && w.maxTokens > cp.info.MaxTokens {
		w.maxTokens = cp.info.MaxTokens
	}
	if req.ToolChoice != nil {
		switch {
		case rej.forcedTool:
			w.fallbacks = append(w.fallbacks, "tool_choice_rejected_by_model")
		case !cp.known && cp.failure != "":
			w.fallbacks = append(w.fallbacks, "capability_lookup_failed: "+cp.failure)
		case !cp.known:
			w.fallbacks = append(w.fallbacks, "capability_unknown")
		case !cp.info.ForcedTool():
			w.fallbacks = append(w.fallbacks, "model_always_thinks")
		default:
			w.forcedTool = true
		}
	}
	if req.Temperature > 0 {
		if rej.temperature {
			w.fallbacks = append(w.fallbacks, "temperature_rejected_by_model")
		} else {
			w.temperature = true
		}
	}
	// A forced tool and thinking exclude each other (the API refuses it), and
	// so does a temperature other than 1.
	w.thinking = cp.known && cp.info.Thinking() && !rej.thinking && !w.forcedTool && !w.temperature
	if req.WebSearch != nil {
		if cp.webSearch != nil && !*cp.webSearch {
			w.fallbacks = append(w.fallbacks, "web_search_rejected_by_org")
		} else {
			w.webSearch = true
		}
	}
	return w
}

// effective reports w as llm.Effective for req.
func (w wire) effective(req llm.Request) llm.Effective {
	e := llm.Effective{MaxTokens: w.maxTokens, WebSearch: w.webSearch, Fallbacks: w.fallbacks}
	if w.temperature {
		t := req.Temperature
		e.Temperature = &t
	}
	switch {
	case req.ToolChoice == nil:
	case w.forcedTool:
		e.ToolChoice = "forced"
	default:
		e.ToolChoice = "auto"
	}
	return e
}

// EffectiveParams implements llm.ParamsReporter: what encodeRequest sends
// for req, from the cached capabilities and the 400s seen so far (call it
// after Stream).
func (c *Client) EffectiveParams(req llm.Request) llm.Effective {
	return c.plan(req).effective(req)
}
