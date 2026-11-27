package httpfetch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/httpfetch"
)

// site serves a robots.txt fixture and HTML pages, counting hits per path.
type site struct {
	*httptest.Server
	mu         sync.Mutex
	hits       map[string]int
	pageStatus int
	robotsCode int
	robotsUA   string
}

func newSite(t *testing.T, robotsFixture string) *site {
	t.Helper()
	s := &site{hits: map[string]int{}, pageStatus: http.StatusOK}
	var robots []byte
	if robotsFixture != "" {
		var err error
		robots, err = os.ReadFile(filepath.Join("testdata", "robots", robotsFixture))
		require.NoError(t, err)
	}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits[r.URL.Path]++
		status, rcode := s.pageStatus, s.robotsCode
		s.mu.Unlock()
		if r.URL.Path == "/robots.txt" {
			s.mu.Lock()
			s.robotsUA = r.Header.Get("User-Agent")
			s.mu.Unlock()
			switch {
			case rcode != 0:
				w.WriteHeader(rcode)
			case robotsFixture == "":
				http.NotFound(w, r)
			default:
				w.Header().Set("Content-Type", "text/plain")
				_, _ = w.Write(robots)
			}
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(status)
		_, _ = w.Write([]byte("<html>ok</html>"))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *site) count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[path]
}

// fakeClock advances only when the fetcher waits.
type fakeClock struct {
	t      time.Time
	sleeps []time.Duration
}

func (c *fakeClock) now() time.Time { return c.t }
func (c *fakeClock) sleep(_ context.Context, d time.Duration) error {
	c.sleeps = append(c.sleeps, d)
	c.t = c.t.Add(d)
	return nil
}

func newPolite(exempt bool) (*httpfetch.Polite, *fakeClock) {
	clk := &fakeClock{t: time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)}
	return httpfetch.NewPolite(httpfetch.New(), httpfetch.WithClock(clk.now, clk.sleep),
		httpfetch.WithAIGroupsExempt(func(string) bool { return exempt })), clk
}

// AC-8: each robots group case on the recorded files.
func TestPolite_RobotsGroups(t *testing.T) {
	tests := []struct {
		name, fixture, path string
		exempt              bool
		allowed             bool
	}{
		{"allow all", "allow_all.txt", "/recipe", false, true},
		{"missing file allows", "", "/recipe", false, true},
		{"star group disallows", "disallow_all.txt", "/recipe", false, false},
		{"Plantry group disallows", "disallow_plantry.txt", "/recipe", false, false},
		{"Plantry group does not hide the star group", "disallow_plantry.txt", "/admin/x", false, false},
		{"AI user groups block", "ai_user_groups.txt", "/recipe", false, false},
		{"exempt site skips AI groups", "ai_user_groups.txt", "/recipe", true, true},
		{"star group still applies to an exempt site", "ai_user_groups.txt", "/private/x", true, false},
		{"training-only groups do not block", "training_only_groups.txt", "/recipe", false, true},
		{"star group of a training-only file applies", "training_only_groups.txt", "/search?q=x", false, false},
		{"chefkoch-like blocked by AI groups", "chefkoch_like.txt", "/rezepte/1/pasta", false, false},
		{"chefkoch-like exempt reads a recipe", "chefkoch_like.txt", "/rezepte/1/pasta", true, true},
		{"chefkoch-like exempt keeps star disallow", "chefkoch_like.txt", "/user/me", true, false},
		{"parse error disallows", "malformed.txt", "/recipe", false, false},
		{"parse error disallows an exempt site", "malformed.txt", "/recipe", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSite(t, tt.fixture)
			p, _ := newPolite(tt.exempt)
			_, _, err := p.Fetch(context.Background(), s.URL+tt.path)
			if tt.allowed {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, httpfetch.ErrRobotsDisallowed)
			assert.Zero(t, s.count("/recipe")+s.count("/rezepte/1/pasta"), "a disallowed page is never requested")
		})
	}
}

func TestPolite_RobotsCachedFor24h(t *testing.T) {
	s := newSite(t, "allow_all.txt")
	p, clk := newPolite(false)
	ctx := context.Background()
	for range 2 {
		_, _, err := p.Fetch(ctx, s.URL+"/a")
		require.NoError(t, err)
	}
	assert.Equal(t, 1, s.count("/robots.txt"))

	clk.t = clk.t.Add(24*time.Hour + time.Second)
	_, _, err := p.Fetch(ctx, s.URL+"/a")
	require.NoError(t, err)
	assert.Equal(t, 2, s.count("/robots.txt"), "refetched after 24 h")
}

func TestPolite_OneRequestPerSecondPerHost(t *testing.T) {
	s := newSite(t, "allow_all.txt")
	p, clk := newPolite(false)
	start := clk.t
	for range 3 {
		_, _, err := p.Fetch(context.Background(), s.URL+"/a")
		require.NoError(t, err)
	}
	// robots.txt + three pages = four requests, three gaps.
	assert.Equal(t, 3*time.Second, clk.t.Sub(start))
	for _, d := range clk.sleeps {
		assert.Equal(t, time.Second, d)
	}
}

func TestPolite_RefusalBacksHostOffFor24h(t *testing.T) {
	for _, code := range []int{401, 402, 403, 429} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			s := newSite(t, "allow_all.txt")
			s.pageStatus = code
			p, clk := newPolite(false)
			ctx := context.Background()

			_, _, err := p.Fetch(ctx, s.URL+"/a")
			require.Error(t, err)
			require.NotErrorIs(t, err, httpfetch.ErrHostBackedOff)

			_, _, err = p.Fetch(ctx, s.URL+"/b")
			require.ErrorIs(t, err, httpfetch.ErrHostBackedOff)
			assert.Zero(t, s.count("/b"))

			clk.t = clk.t.Add(24*time.Hour + time.Second)
			s.mu.Lock()
			s.pageStatus = http.StatusOK
			s.mu.Unlock()
			_, _, err = p.Fetch(ctx, s.URL+"/b")
			require.NoError(t, err)
		})
	}
}

func TestPolite_RobotsRefusalBacksHostOff(t *testing.T) {
	s := newSite(t, "allow_all.txt")
	s.robotsCode = http.StatusForbidden
	p, _ := newPolite(false)
	_, _, err := p.Fetch(context.Background(), s.URL+"/a")
	require.ErrorIs(t, err, httpfetch.ErrHostBackedOff)
	_, _, err = p.Fetch(context.Background(), s.URL+"/a")
	require.ErrorIs(t, err, httpfetch.ErrHostBackedOff)
	assert.Equal(t, 1, s.count("/robots.txt"))
	assert.Zero(t, s.count("/a"))
}

func TestPolite_RobotsServerErrorDisallows(t *testing.T) {
	s := newSite(t, "allow_all.txt")
	s.robotsCode = http.StatusInternalServerError
	p, _ := newPolite(false)
	_, _, err := p.Fetch(context.Background(), s.URL+"/a")
	require.ErrorIs(t, err, httpfetch.ErrRobotsDisallowed)
	assert.Zero(t, s.count("/a"))
}

func TestPolite_HonestUserAgent(t *testing.T) {
	s := newSite(t, "allow_all.txt")
	p, _ := newPolite(false)
	_, _, err := p.Fetch(context.Background(), s.URL+"/a")
	require.NoError(t, err)
	assert.Contains(t, s.robotsUA, "Plantry")
}

func TestExemptDomains(t *testing.T) {
	ex := httpfetch.ExemptDomains([]string{"chefkoch.de"})
	assert.True(t, ex("chefkoch.de"))
	assert.True(t, ex("www.Chefkoch.de"))
	assert.False(t, ex("notchefkoch.de"))
	assert.False(t, ex("example.com"))
}
