package discovery_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/httpfetch"
	"github.com/jaltszeimer/plantry/backend/internal/adapters/jsonld"
	"github.com/jaltszeimer/plantry/backend/internal/domain/discovery"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

const recipePage = `<html><head><script type="application/ld+json">
{"@type":"Recipe","name":"Fesenjan","recipeYield":"4","recipeIngredient":["500 g Walnüsse","1 Zwiebel"]}
</script></head><body>ok</body></html>`

// spyFetcher records every URL the importer requests.
type spyFetcher struct {
	inner *httpfetch.Client
	mu    sync.Mutex
	urls  []string
}

func (s *spyFetcher) Fetch(ctx context.Context, url string) (string, string, error) {
	s.mu.Lock()
	s.urls = append(s.urls, url)
	s.mu.Unlock()
	return s.inner.Fetch(ctx, url)
}

// urlSet is a discovery.URLIndex over a fixed set.
type urlSet map[string]bool

func (u urlSet) SourceURLKnown(_ context.Context, url string) (bool, error) { return u[url], nil }

func site(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	page := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(body))
		}
	}
	mux.HandleFunc("/fesenjan", page(recipePage))
	mux.HandleFunc("/blog", page("<html><body>no recipe here</body></html>"))
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/fesenjan", http.StatusFound)
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func newVerifier(allowLoopback bool, known urlSet) (*discovery.Verifier, *spyFetcher) {
	spy := &spyFetcher{inner: httpfetch.New()}
	return &discovery.Verifier{
		Extractor: importer.NewService(spy, jsonld.Extractor{}, nil, nil, nil),
		Index:     known,
		Guard:     func(u string) error { return httpfetch.PublicURL(u, nil, allowLoopback) },
	}, spy
}

func urlsOf(ds []discovery.Discarded) map[string]string {
	m := map[string]string{}
	for _, d := range ds {
		m[d.URL] = d.Reason
	}
	return m
}

func TestVerify_KeepsOneOfFour(t *testing.T) {
	srv := site(t)
	private := "http://10.255.255.1/secret"
	v, spy := newVerifier(true, nil)

	kept, discarded, err := v.Verify(context.Background(), []discovery.Candidate{
		{URL: srv.URL + "/fesenjan"}, {URL: srv.URL + "/blog"}, {URL: srv.URL + "/gone"}, {URL: private},
	})
	require.NoError(t, err)

	require.Len(t, kept, 1)
	assert.Equal(t, "Fesenjan", kept[0].Draft.Name)
	assert.Equal(t, srv.URL+"/fesenjan", kept[0].Candidate.URL)
	assert.Equal(t, map[string]string{
		srv.URL + "/blog": discovery.ReasonNoRecipe,
		srv.URL + "/gone": discovery.ReasonFetch,
		private:           discovery.ReasonBlockedURL,
	}, urlsOf(discarded))
	assert.NotContains(t, spy.urls, private, "a private address is never requested")
	assert.Len(t, spy.urls, 3)
}

func TestVerify_LoopbackOnlyInDevMode(t *testing.T) {
	srv := site(t)
	cands := []discovery.Candidate{{URL: srv.URL + "/fesenjan"}}

	v, spy := newVerifier(false, nil)
	kept, discarded, err := v.Verify(context.Background(), cands)
	require.NoError(t, err)
	assert.Empty(t, kept)
	assert.Equal(t, map[string]string{srv.URL + "/fesenjan": discovery.ReasonBlockedURL}, urlsOf(discarded))
	assert.Empty(t, spy.urls, "loopback is not requested outside dev mode")

	v, _ = newVerifier(true, nil)
	kept, _, err = v.Verify(context.Background(), cands)
	require.NoError(t, err)
	assert.Len(t, kept, 1)
}

func TestVerify_SkipsKnownURLs(t *testing.T) {
	srv := site(t)
	// A food already has the final URL; another candidate's URL is known as is.
	v, spy := newVerifier(true, urlSet{srv.URL + "/fesenjan": true, srv.URL + "/blog": true})

	kept, discarded, err := v.Verify(context.Background(), []discovery.Candidate{
		{URL: srv.URL + "/blog"},    // known as given: never fetched
		{URL: srv.URL + "/moved"},   // redirects to a known final URL
		{URL: srv.URL + "/blog"},    // repeated in the batch
		{URL: srv.URL + "/unknown"}, // 404, fetched
	})
	require.NoError(t, err)

	assert.Empty(t, kept)
	assert.Len(t, discarded, 4)
	assert.Equal(t, discovery.ReasonDuplicate, discarded[0].Reason)
	assert.Equal(t, discovery.ReasonDuplicate, discarded[1].Reason)
	assert.Equal(t, discovery.ReasonDuplicate, discarded[2].Reason)
	assert.Equal(t, discovery.ReasonFetch, discarded[3].Reason)
	assert.NotContains(t, spy.urls, srv.URL+"/blog", "a known URL is skipped before import")
}

func TestVerify_FinalURLRepeatedInBatch(t *testing.T) {
	srv := site(t)
	v, _ := newVerifier(true, nil)

	kept, discarded, err := v.Verify(context.Background(), []discovery.Candidate{
		{URL: srv.URL + "/moved"}, {URL: srv.URL + "/fesenjan"},
	})
	require.NoError(t, err)
	require.Len(t, kept, 1)
	assert.Equal(t, srv.URL+"/fesenjan", kept[0].Draft.SourceURL)
	require.Len(t, discarded, 1)
	assert.Equal(t, discovery.ReasonDuplicate, discarded[0].Reason)
}

func TestVerify_ContextCancelled(t *testing.T) {
	srv := site(t)
	v, _ := newVerifier(true, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := v.Verify(ctx, []discovery.Candidate{{URL: srv.URL + "/fesenjan"}})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestVerify_RedirectToBlockedHostDiscarded(t *testing.T) {
	hit := false
	spy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(recipePage))
	}))
	t.Cleanup(spy.Close)
	pub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, spy.URL, http.StatusFound)
	}))
	t.Cleanup(pub.Close)

	v, _ := newVerifier(true, urlSet{})
	v.Guard = func(u string) error {
		if strings.HasPrefix(u, spy.URL) {
			return errors.New("private host")
		}
		return nil
	}
	kept, discarded, err := v.Verify(context.Background(), []discovery.Candidate{{URL: pub.URL}})
	require.NoError(t, err)
	require.Empty(t, kept)
	assert.Equal(t, discovery.ReasonBlockedURL, urlsOf(discarded)[pub.URL])
	assert.False(t, hit)
}
