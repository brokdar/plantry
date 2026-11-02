package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// capabilityTTL is how long a model's web search answer is trusted (D3);
// transientTTL how long a failed check counts as unsupported before it is
// retried.
const (
	capabilityTTL = 24 * time.Hour
	transientTTL  = 5 * time.Minute
)

type capability struct {
	webSearch bool
	until     time.Time
}

// WebSearch implements llm.Capabilities. It reads
// capabilities.server_tools.web_search.supported from GET /v1/models/{model}
// once per model per 24 h. A missing field, or a 400/404 for the model,
// counts as unsupported for 24 h too; any other failure (network, 5xx, 429,
// 401, an undecodable body) counts as unsupported for 5 min only. err reports
// the failed call only on the call that made it.
func (c *Client) WebSearch(ctx context.Context, model string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock() // held across the call so a model is fetched once
	if cp, ok := c.caps[model]; ok && c.now().Before(cp.until) {
		return cp.webSearch, nil
	}
	supported, err := c.fetchWebSearch(ctx, model)
	if ctx.Err() != nil {
		return false, err // the caller's cancellation says nothing about the model
	}
	ttl := capabilityTTL
	if err != nil && !definitive(err) {
		ttl = transientTTL
	}
	c.caps[model] = capability{webSearch: supported, until: c.now().Add(ttl)}
	return supported, err
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
