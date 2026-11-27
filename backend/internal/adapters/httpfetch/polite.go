package httpfetch

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Politeness constants (FIND-6).
const (
	robotsTTL  = 24 * time.Hour
	backoffTTL = 24 * time.Hour
	hostGap    = time.Second
)

var (
	// ErrRobotsDisallowed: the host's robots.txt forbids the fetch, or could
	// not be read.
	ErrRobotsDisallowed = errors.New("httpfetch: robots.txt disallows the fetch")
	// ErrHostBackedOff: the host answered 401, 402, 403 or 429 within the
	// last 24 hours and is not asked again.
	ErrHostBackedOff = errors.New("httpfetch: host skipped after a refusal")
)

// Polite wraps Client for fetches the assistant starts itself (finds): it
// obeys robots.txt, spaces requests per host and backs off from hosts that
// refuse us. A link the user pastes goes through the plain Client.
type Polite struct {
	c     *Client
	now   func() time.Time
	sleep func(ctx context.Context, d time.Duration) error
	// aiExempt reports whether a host's AI user-agent groups are skipped.
	aiExempt func(host string) bool

	mu      sync.Mutex
	robots  map[string]robotsEntry
	blocked map[string]time.Time // host -> backoff end
	next    map[string]time.Time // host -> earliest next request
}

type robotsEntry struct {
	rules   *robots // nil: the host disallows everything
	expires time.Time
}

// PoliteOption configures a Polite.
type PoliteOption func(*Polite)

// WithClock injects the time source and the wait, for tests.
func WithClock(now func() time.Time, sleep func(ctx context.Context, d time.Duration) error) PoliteOption {
	return func(p *Polite) { p.now, p.sleep = now, sleep }
}

// WithAIGroupsExempt sets which hosts skip the AI user-agent groups of their
// robots.txt (the chef.sites ai_groups_exempt flag); `*` and `Plantry` groups
// always apply.
func WithAIGroupsExempt(fn func(host string) bool) PoliteOption {
	return func(p *Polite) { p.aiExempt = fn }
}

// ExemptDomains returns an exemption check for hosts equal to or below one of
// domains.
func ExemptDomains(domains []string) func(host string) bool {
	return func(host string) bool {
		host = strings.ToLower(host)
		for _, d := range domains {
			if d = strings.ToLower(d); host == d || strings.HasSuffix(host, "."+d) {
				return true
			}
		}
		return false
	}
}

// NewPolite wraps c.
func NewPolite(c *Client, opts ...PoliteOption) *Polite {
	p := &Polite{
		c: c, now: time.Now, sleep: sleepCtx,
		robots: map[string]robotsEntry{}, blocked: map[string]time.Time{}, next: map[string]time.Time{},
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Fetch is Client.Fetch after the politeness checks. The checks cover rawURL
// itself; a redirect to another host is followed unchecked.
func (p *Polite) Fetch(ctx context.Context, rawURL string) (string, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("httpfetch: bad url %q", rawURL)
	}
	host := strings.ToLower(u.Host)
	if p.isBlocked(host) {
		return "", "", ErrHostBackedOff
	}
	rules, err := p.robotsFor(ctx, u, host)
	if err != nil {
		return "", "", err
	}
	exempt := p.aiExempt != nil && p.aiExempt(u.Hostname())
	if rules == nil || !rules.allowed(u.RequestURI(), exempt) {
		return "", "", ErrRobotsDisallowed
	}
	if err := p.wait(ctx, host); err != nil {
		return "", "", err
	}
	body, final, err := p.c.Fetch(ctx, rawURL)
	p.noteRefusal(host, err)
	return body, final, err
}

func (p *Polite) isBlocked(host string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.now().Before(p.blocked[host])
}

// noteRefusal starts the 24 h backoff when err is a 401/402/403/429.
func (p *Polite) noteRefusal(host string, err error) {
	var use *UpstreamStatusError
	if !errors.As(err, &use) {
		return
	}
	switch use.Status {
	case http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden, http.StatusTooManyRequests:
		p.mu.Lock()
		p.blocked[host] = p.now().Add(backoffTTL)
		p.mu.Unlock()
	}
}

// wait reserves the host's next one-second slot and sleeps until it.
func (p *Polite) wait(ctx context.Context, host string) error {
	p.mu.Lock()
	now := p.now()
	slot := now
	if n := p.next[host]; n.After(slot) {
		slot = n
	}
	p.next[host] = slot.Add(hostGap)
	p.mu.Unlock()
	if d := slot.Sub(now); d > 0 {
		return p.sleep(ctx, d)
	}
	return nil
}

// robotsFor returns the host's rules from the 24 h cache or fetches them. A
// missing file (404, 410) allows everything; an unreadable one, a failed
// fetch or a 5xx disallows (nil rules). The rules of a host that refuses the
// robots request start the 24 h backoff.
func (p *Polite) robotsFor(ctx context.Context, u *url.URL, host string) (*robots, error) {
	p.mu.Lock()
	e, ok := p.robots[host]
	p.mu.Unlock()
	if ok && p.now().Before(e.expires) {
		return e.rules, nil
	}
	if err := p.wait(ctx, host); err != nil {
		return nil, err
	}
	body, err := p.c.FetchText(ctx, u.Scheme+"://"+u.Host+"/robots.txt")
	var rules *robots
	switch {
	case err == nil:
		if rules, err = parseRobots(body); err != nil {
			rules = nil
		}
	case isStatus(err, http.StatusNotFound, http.StatusGone):
		rules = &robots{}
	default:
		p.noteRefusal(host, err)
		if p.isBlocked(host) {
			return nil, ErrHostBackedOff
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Not cached: the next fetch asks again (after the host gap).
		return nil, nil
	}
	p.mu.Lock()
	p.robots[host] = robotsEntry{rules: rules, expires: p.now().Add(robotsTTL)}
	p.mu.Unlock()
	return rules, nil
}

func isStatus(err error, statuses ...int) bool {
	var use *UpstreamStatusError
	if !errors.As(err, &use) {
		return false
	}
	for _, s := range statuses {
		if use.Status == s {
			return true
		}
	}
	return false
}
