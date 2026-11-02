package httpfetch_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/httpfetch"
	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

func TestFetch_Success(t *testing.T) {
	wantBody := "<html><body>hi</body></html>"
	seenUA, seenLang := "", ""

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUA = r.Header.Get("User-Agent")
		seenLang = r.Header.Get("Accept-Language")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(wantBody))
	}))
	t.Cleanup(ts.Close)

	c := httpfetch.New()
	body, final, err := c.Fetch(context.Background(), ts.URL)
	require.NoError(t, err)
	require.Equal(t, wantBody, body)
	require.Equal(t, ts.URL, final)
	require.Contains(t, seenUA, "Plantry")
	require.Contains(t, seenLang, "de")
}

func TestFetch_404_ReturnsUpstreamStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(ts.Close)

	c := httpfetch.New()
	_, _, err := c.Fetch(context.Background(), ts.URL)
	var use *httpfetch.UpstreamStatusError
	require.True(t, errors.As(err, &use))
	require.Equal(t, http.StatusNotFound, use.Status)
}

func TestFetch_BodyTooLarge(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(strings.Repeat("x", 1024)))
	}))
	t.Cleanup(ts.Close)

	c := httpfetch.New(httpfetch.WithMaxBytes(100))
	_, _, err := c.Fetch(context.Background(), ts.URL)
	require.ErrorIs(t, err, httpfetch.ErrBodyTooLarge)
}

func TestFetch_NotHTML(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(ts.Close)

	c := httpfetch.New()
	_, _, err := c.Fetch(context.Background(), ts.URL)
	require.ErrorIs(t, err, httpfetch.ErrNotHTML)
}

func TestFetch_TooManyRedirects(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Infinite redirect to itself.
		http.Redirect(w, r, ts.URL+"/next", http.StatusFound)
	}))
	t.Cleanup(ts.Close)

	c := httpfetch.New(httpfetch.WithMaxRedirects(2))
	_, _, err := c.Fetch(context.Background(), ts.URL)
	require.ErrorIs(t, err, httpfetch.ErrTooManyRedirects)
}

func TestFetch_ContextCancel(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(ts.Close)

	c := httpfetch.New()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, _, err := c.Fetch(ctx, ts.URL)
	require.Error(t, err)
}

func TestFetch_EmptyContentType_StillAccepted(t *testing.T) {
	// Some sites omit Content-Type; the fetcher should tolerate it.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Note: Go's default serves with text/plain; clear it explicitly.
		w.Header()["Content-Type"] = nil
		_, _ = w.Write([]byte("<html></html>"))
	}))
	t.Cleanup(ts.Close)

	c := httpfetch.New()
	body, _, err := c.Fetch(context.Background(), ts.URL)
	require.NoError(t, err)
	require.Equal(t, "<html></html>", body)
}

func TestFetch_FollowsRedirect_ReturnsFinalURL(t *testing.T) {
	finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, "<html>final</html>")
	}))
	t.Cleanup(finalServer.Close)

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalServer.URL, http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	c := httpfetch.New()
	body, final, err := c.Fetch(context.Background(), redirector.URL)
	require.NoError(t, err)
	require.Contains(t, body, "final")
	require.Equal(t, finalServer.URL, final)
}

func TestFetch_RedirectGuardBlocksHop(t *testing.T) {
	hit := false
	spy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "text/html")
	}))
	t.Cleanup(spy.Close)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, spy.URL, http.StatusFound)
	}))
	t.Cleanup(ts.Close)

	guard := func(u string) error {
		if strings.HasPrefix(u, spy.URL) {
			return errors.New("blocked")
		}
		return nil
	}
	c := httpfetch.New()
	_, _, err := c.Fetch(importer.WithURLGuard(context.Background(), guard), ts.URL)
	require.ErrorIs(t, err, importer.ErrURLBlocked)
	require.ErrorIs(t, err, domain.ErrImportFetchFailed)
	require.False(t, hit)

	// Without a guard the redirect is followed.
	_, _, err = c.Fetch(context.Background(), ts.URL)
	require.NoError(t, err)
	require.True(t, hit)
}

// DoD S2: a guarded fetch checks the address it actually dials. The pre-check
// sees a public address; the dial-time lookup gets loopback (DNS rebinding)
// and the connection is refused.
func TestFetch_DialGuardBlocksRebinding(t *testing.T) {
	hit := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	}))
	t.Cleanup(ts.Close)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(ts.URL, "http://"))
	raw := "http://rebind.test:" + port + "/"

	preCheck := func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("93.184.216.34")}, nil }
	atDial := func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("127.0.0.1")}, nil }
	guard := func(u string) error { return httpfetch.PublicURL(u, preCheck, false) }

	c := httpfetch.New(httpfetch.WithDialGuard(atDial, false))
	_, _, err := c.Fetch(importer.WithURLGuard(context.Background(), guard), raw)
	require.ErrorIs(t, err, httpfetch.ErrBlockedURL)
	require.False(t, hit, "the private address is never connected to")

	// Dev mode allows loopback at dial time too.
	dev := httpfetch.New(httpfetch.WithDialGuard(atDial, true))
	_, _, err = dev.Fetch(importer.WithURLGuard(context.Background(), guard), raw)
	require.NoError(t, err)
	require.True(t, hit)
}

// A fetch without a guard in ctx (a user-pasted import URL) dials normally.
func TestFetch_DialGuardOnlyForGuardedFetches(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html></html>"))
	}))
	t.Cleanup(ts.Close)
	c := httpfetch.New(httpfetch.WithDialGuard(nil, false))
	_, _, err := c.Fetch(context.Background(), ts.URL)
	require.NoError(t, err)
}
