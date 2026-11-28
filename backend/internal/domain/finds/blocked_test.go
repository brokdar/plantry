package finds_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/fake"
	"github.com/jaltszeimer/plantry/backend/internal/domain/finds"
	"github.com/jaltszeimer/plantry/backend/internal/domain/llm"
)

// blockingClient refuses a web search whose allowed domains include one of
// blocked, as Anthropic does for sites that block its crawler.
type blockingClient struct {
	*fake.Client
	blocked []string
	refused int
}

func (b *blockingClient) Stream(ctx context.Context, req llm.Request, out chan<- llm.Event) (*llm.Response, error) {
	if req.WebSearch != nil {
		var hit []string
		for _, d := range req.WebSearch.AllowedDomains {
			if slices.Contains(b.blocked, d) {
				hit = append(hit, d)
			}
		}
		if len(hit) > 0 {
			close(out)
			b.refused++
			return nil, &llm.DomainsBlockedError{Domains: hit}
		}
	}
	return b.Client.Stream(ctx, req, out)
}

// A site that blocks the provider's crawler is dropped from the search, which
// runs once more without it, and skipped by later searches; with no site
// left the find fails with no_sites.
func TestPipeline_BlockedSearchDomains(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("retried without the blocked site and remembered", func(t *testing.T) {
		t.Parallel()
		bc := &blockingClient{blocked: []string{"bbcgoodfood.com"}}
		r := newRigWrapped(t, func(c *fake.Client) llm.Client { bc.Client = c; return bc }, true, lineMatcher{}, kitchen,
			searchTurn(curryURLs...), adaptTurn(true, fitsKitchen))
		f := r.run(t, finds.TriggerPlanning)
		require.Equal(t, 1, bc.refused)
		require.Equal(t, "searched", f.Steps[0].Key)
		require.Equal(t, "budgetbytes.com, thewoksoflife.com", f.Steps[0].Params["sites"])
		require.Equal(t, []string{"budgetbytes.com", "thewoksoflife.com"}, r.client.Requests()[0].WebSearch.AllowedDomains)
		blocked, err := r.store.SearchBlocked(ctx, time.Now().Add(29*24*time.Hour))
		require.NoError(t, err)
		require.Equal(t, []string{"bbcgoodfood.com"}, blocked, "skipped for about 30 days")
		blocked, err = r.store.SearchBlocked(ctx, time.Now().Add(31*24*time.Hour))
		require.NoError(t, err)
		require.Empty(t, blocked)
	})

	t.Run("every site blocked fails with no_sites", func(t *testing.T) {
		t.Parallel()
		bc := &blockingClient{blocked: []string{"bbcgoodfood.com", "budgetbytes.com", "thewoksoflife.com"}}
		r := newRigWrapped(t, func(c *fake.Client) llm.Client { bc.Client = c; return bc }, true, lineMatcher{}, kitchen)
		f := r.run(t, finds.TriggerPlanning)
		require.Equal(t, finds.StateFailed, f.State)
		require.Equal(t, "no_sites", f.Steps[len(f.Steps)-1].Key)
		require.Equal(t, 1, bc.refused, "no second search without sites")
	})
}

// CP4 fix: a site whose robots.txt refuses us is left out of the search's
// allowed list.
func TestPipeline_RobotsRefusedSiteLeftOut(t *testing.T) {
	t.Parallel()
	r := newRig(t, true, lineMatcher{}, kitchen, searchTurn(curryURLs...), adaptTurn(true, fitsKitchen))
	r.robots = func(d string) bool { return d != "bbcgoodfood.com" }
	f := r.run(t, finds.TriggerPlanning)
	require.Equal(t, "budgetbytes.com, thewoksoflife.com", f.Steps[0].Params["sites"])
	require.Equal(t, []string{"budgetbytes.com", "thewoksoflife.com"}, r.client.Requests()[0].WebSearch.AllowedDomains)
}
