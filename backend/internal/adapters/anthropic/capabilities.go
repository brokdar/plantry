package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// capabilityTTL is how long a model's web search answer is trusted (D3).
const capabilityTTL = 24 * time.Hour

type capability struct {
	webSearch bool
	until     time.Time
}

// WebSearch implements llm.Capabilities. It reads
// capabilities.server_tools.web_search.supported from GET /v1/models/{model}
// once per model per 24 h. A missing field or a failed call counts as
// unsupported and is cached too; err reports the failed call only on the
// call that made it.
func (c *Client) WebSearch(ctx context.Context, model string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock() // held across the call so a model is fetched once
	if cp, ok := c.caps[model]; ok && c.now().Before(cp.until) {
		return cp.webSearch, nil
	}
	supported, err := c.fetchWebSearch(ctx, model)
	c.caps[model] = capability{webSearch: supported, until: c.now().Add(capabilityTTL)}
	return supported, err
}

// setWebSearch caches a known answer for model, e.g. after the API rejected
// a search request.
func (c *Client) setWebSearch(model string, supported bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.caps[model] = capability{webSearch: supported, until: c.now().Add(capabilityTTL)}
}

func (c *Client) fetchWebSearch(ctx context.Context, model string) (bool, error) {
	req, err := c.buildRequest(ctx, http.MethodGet, "/v1/models/"+url.PathEscape(model), nil)
	if err != nil {
		return false, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("anthropic: models request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, decodeAPIError(resp)
	}
	var body struct {
		Capabilities *struct {
			ServerTools *struct {
				WebSearch *struct {
					Supported bool `json:"supported"`
				} `json:"web_search"`
			} `json:"server_tools"`
		} `json:"capabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, fmt.Errorf("anthropic: models decode: %w", err)
	}
	if body.Capabilities == nil || body.Capabilities.ServerTools == nil || body.Capabilities.ServerTools.WebSearch == nil {
		return false, nil
	}
	return body.Capabilities.ServerTools.WebSearch.Supported, nil
}
