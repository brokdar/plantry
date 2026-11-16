package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// capabilityTTL is how long a model's capabilities are trusted (D3);
// transientTTL how long a failed check counts as "unknown" before it is
// retried.
const (
	capabilityTTL = 24 * time.Hour
	transientTTL  = 5 * time.Minute
)

// modelsPageLimit is the page size of GET /v1/models (the API allows 1000).
const modelsPageLimit = 1000

// support is the API's {"supported": bool} capability object.
type support struct {
	Supported bool `json:"supported"`
}

// ModelInfo is one entry of GET /v1/models (the fields Plantry uses).
type ModelInfo struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	CreatedAt      string `json:"created_at"`
	Lifecycle      string `json:"lifecycle"`
	Line           string `json:"line"`
	MaxInputTokens int    `json:"max_input_tokens"`
	MaxTokens      int    `json:"max_tokens"`

	Capabilities *struct {
		Thinking *struct {
			Supported bool `json:"supported"`
			Types     *struct {
				Disabled *support `json:"disabled"`
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

type capability struct {
	info  ModelInfo
	known bool // false: the call failed or the model is unknown to the API
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
// missing or stale (the lock is held across the call so a model is fetched
// once). err reports the failed call only on the call that made it.
func (c *Client) capability(ctx context.Context, model string) (capability, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cp, ok := c.caps[model]; ok && c.now().Before(cp.until) {
		return cp, nil
	}
	info, err := c.fetchModel(ctx, model)
	if ctx.Err() != nil {
		return capability{}, err // the caller's cancellation says nothing about the model
	}
	ttl := capabilityTTL
	if err != nil && !definitive(err) {
		ttl = transientTTL
	}
	cp := capability{info: info, known: err == nil, until: c.now().Add(ttl)}
	c.caps[model] = cp
	return cp, err
}

// cachedCapability returns the cache entry of model without calling the API.
func (c *Client) cachedCapability(model string) capability {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.caps[model]
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

// EffectiveParams implements llm.ParamsReporter: the max_tokens and
// temperature encodeRequest sends for req, from the cached capabilities.
func (c *Client) EffectiveParams(req llm.Request) llm.Effective {
	return llm.Effective{MaxTokens: c.maxTokens(req)}
}

// maxTokens is the request's max_tokens (DefaultMaxTokens when unset) capped
// at the model's own limit once the API has told it.
func (c *Client) maxTokens(req llm.Request) int {
	n := req.MaxTokens
	if n <= 0 {
		n = DefaultMaxTokens
	}
	if cp := c.cachedCapability(req.Model); cp.known && cp.info.MaxTokens > 0 && n > cp.info.MaxTokens {
		n = cp.info.MaxTokens
	}
	return n
}
