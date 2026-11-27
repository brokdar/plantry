package anthropic_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/anthropic"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// A web search refused for allowed domains that block the crawler comes
// back typed with those domains.
func TestStream_BlockedDomainsTyped(t *testing.T) {
	ts := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"The following domains are not accessible to our user agent: ['bbcgoodfood.com', 'budgetbytes.com']"}}`))
	})
	c := anthropic.New("k", anthropic.WithBaseURL(ts.URL), anthropic.WithHTTPClient(ts.Client()))
	events := make(chan llm.Event, 16)
	go func() {
		for range events { //nolint:revive // drain
		}
	}()
	_, err := c.Stream(context.Background(), llm.Request{
		Model: "claude-x", MaxTokens: 10, WebSearch: &llm.WebSearch{MaxUses: 1, AllowedDomains: []string{"bbcgoodfood.com", "budgetbytes.com", "x.org"}},
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "pho"}}}},
	}, events)
	var blocked *llm.DomainsBlockedError
	require.True(t, errors.As(err, &blocked), "%v", err)
	assert.Equal(t, []string{"bbcgoodfood.com", "budgetbytes.com"}, blocked.Domains)
	var apiErr *anthropic.APIError
	assert.True(t, errors.As(err, &apiErr), "the API error stays reachable")
}
