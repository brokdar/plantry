// Package httpfetch wraps a polite HTML fetcher used by the recipe importer.
package httpfetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// Default settings chosen to be polite but resilient to cheap bot checks.
const (
	defaultUserAgent    = "Plantry/1.0 recipe-importer"
	defaultAcceptLang   = "de-DE,de;q=0.9,en;q=0.8"
	defaultAccept       = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
	defaultMaxBytes     = int64(5 << 20) // 5 MiB
	defaultMaxRedirects = 5
	defaultTimeout      = 30 * time.Second
)

// Sentinel errors bubbled through from the domain so handlers map them
// directly via errormap.go. ErrTooManyRedirects is adapter-local.
var (
	ErrBodyTooLarge     = domain.ErrImportBodyTooLarge
	ErrNotHTML          = domain.ErrImportNotHTML
	ErrTooManyRedirects = errors.New("httpfetch: too many redirects")
)

// UpstreamStatusError is returned for any non-2xx HTTP response.
type UpstreamStatusError struct {
	Status int
	URL    string
}

func (e *UpstreamStatusError) Error() string {
	return fmt.Sprintf("httpfetch: upstream returned %d for %s", e.Status, e.URL)
}

// Client is a polite HTML fetcher.
type Client struct {
	hc           *http.Client
	ua           string
	acceptLang   string
	accept       string
	maxBytes     int64
	maxRedirects int
	// guarded serves fetches whose ctx carries a URL guard (WithDialGuard).
	guarded   *http.Client
	dialGuard *dialGuard
}

type dialGuard struct {
	resolve       Resolver
	allowLoopback bool
}

// Option configures a Client.
type Option func(*Client)

// WithMaxBytes caps the number of bytes read from a response body.
func WithMaxBytes(n int64) Option { return func(c *Client) { c.maxBytes = n } }

// WithMaxRedirects caps the number of HTTP redirects followed.
func WithMaxRedirects(n int) Option { return func(c *Client) { c.maxRedirects = n } }

// WithDialGuard makes fetches whose ctx carries a URL guard
// (importer.WithURLGuard) dial public addresses only (allowLoopback: dev
// mode), closing the gap between the guard's lookup and the dial. A nil
// resolve uses the default resolver. Unguarded fetches are unaffected.
func WithDialGuard(resolve Resolver, allowLoopback bool) Option {
	return func(c *Client) { c.dialGuard = &dialGuard{resolve, allowLoopback} }
}

// New creates a Client with sensible defaults.
func New(opts ...Option) *Client {
	c := &Client{
		ua:           defaultUserAgent,
		acceptLang:   defaultAcceptLang,
		accept:       defaultAccept,
		maxBytes:     defaultMaxBytes,
		maxRedirects: defaultMaxRedirects,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.hc == nil {
		c.hc = &http.Client{Timeout: defaultTimeout}
	}
	// Always install the redirect cap; tests may override the http.Client entirely
	// via WithHTTPClient, in which case we leave their choice alone.
	if c.hc.CheckRedirect == nil {
		c.hc.CheckRedirect = checkRedirect(c.maxRedirects)
	}
	if c.dialGuard != nil {
		c.guarded = guardedClient(c.dialGuard.resolve, c.dialGuard.allowLoopback, c.hc.Timeout, c.maxRedirects)
	}
	return c
}

// Fetch retrieves an HTML document from url. It returns the body as a string, the final
// URL after redirects, and an error. On non-2xx status, oversize body, or non-HTML
// content, the response body is discarded and an appropriate sentinel error is returned.
func (c *Client) Fetch(ctx context.Context, url string) (string, string, error) {
	return c.get(ctx, url, true)
}

// FetchText is Fetch for a text document such as robots.txt: any content type
// is accepted.
func (c *Client) FetchText(ctx context.Context, url string) (string, error) {
	body, _, err := c.get(ctx, url, false)
	return body, err
}

func (c *Client) get(ctx context.Context, url string, html bool) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", fmt.Errorf("httpfetch: new request: %w", err)
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", c.accept)
	req.Header.Set("Accept-Language", c.acceptLang)

	hc := c.hc
	if c.guarded != nil && importer.URLGuardFrom(ctx) != nil {
		hc = c.guarded
	}
	resp, err := hc.Do(req)
	if err != nil {
		// Surface the redirect-cap error as ErrTooManyRedirects for callers.
		if errors.Is(err, ErrTooManyRedirects) {
			return "", "", ErrTooManyRedirects
		}
		if errors.Is(err, ErrBlockedURL) {
			return "", "", fmt.Errorf("%w: %w", importer.ErrURLBlocked, err)
		}
		return "", "", fmt.Errorf("%w: %w", classifyTransportError(err), err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("%w: %w", classifyStatus(resp.StatusCode), &UpstreamStatusError{Status: resp.StatusCode, URL: url})
	}

	if html {
		if err := assertHTMLContentType(resp.Header.Get("Content-Type")); err != nil {
			return "", "", err
		}
	}

	// Read up to maxBytes + 1 so we can detect overflow.
	limited := io.LimitReader(resp.Body, c.maxBytes+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return "", "", fmt.Errorf("httpfetch: read: %w", err)
	}
	if int64(len(buf)) > c.maxBytes {
		return "", "", ErrBodyTooLarge
	}

	finalURL := url
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	return string(buf), finalURL, nil
}

// classifyStatus names the sentinel of a non-2xx response: a missing page, a
// site that refuses us, or any other failure.
func classifyStatus(status int) error {
	switch status {
	case http.StatusNotFound, http.StatusGone:
		return domain.ErrImportNotFound
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return domain.ErrImportBlocked
	}
	return domain.ErrImportFetchFailed
}

// classifyTransportError tells a timeout and an unresolvable or unreachable
// host from other transport failures.
func classifyTransportError(err error) error {
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return domain.ErrImportTimeout
	case errors.As(err, &dnsErr), errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH):
		return domain.ErrImportUnreachable
	}
	return domain.ErrImportFetchFailed
}

func assertHTMLContentType(header string) error {
	if header == "" {
		return nil // Tolerate missing Content-Type; many sites omit it.
	}
	mediaType, _, err := mime.ParseMediaType(header)
	if err != nil {
		return nil // Malformed Content-Type is not fatal.
	}
	mediaType = strings.ToLower(mediaType)
	switch mediaType {
	case "text/html", "application/xhtml+xml", "application/xml":
		return nil
	}
	return ErrNotHTML
}
