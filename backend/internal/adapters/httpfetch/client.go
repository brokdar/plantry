// Package httpfetch wraps a polite HTML fetcher used by the recipe importer.
package httpfetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
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
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the underlying http.Client (useful in tests to dial httptest.Server).
func WithHTTPClient(hc *http.Client) Option { return func(c *Client) { c.hc = hc } }

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option { return func(c *Client) { c.ua = ua } }

// WithMaxBytes caps the number of bytes read from a response body.
func WithMaxBytes(n int64) Option { return func(c *Client) { c.maxBytes = n } }

// WithMaxRedirects caps the number of HTTP redirects followed.
func WithMaxRedirects(n int) Option { return func(c *Client) { c.maxRedirects = n } }

// WithTimeout sets the per-request timeout on the underlying http.Client.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if c.hc == nil {
			c.hc = &http.Client{}
		}
		c.hc.Timeout = d
	}
}

// WithAcceptLanguage overrides the Accept-Language header.
func WithAcceptLanguage(v string) Option { return func(c *Client) { c.acceptLang = v } }

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
		c.hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= c.maxRedirects {
				return ErrTooManyRedirects
			}
			return nil
		}
	}
	return c
}

// Fetch retrieves an HTML document from url. It returns the body as a string, the final
// URL after redirects, and an error. On non-2xx status, oversize body, or non-HTML
// content, the response body is discarded and an appropriate sentinel error is returned.
func (c *Client) Fetch(ctx context.Context, url string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", fmt.Errorf("httpfetch: new request: %w", err)
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept", c.accept)
	req.Header.Set("Accept-Language", c.acceptLang)

	resp, err := c.hc.Do(req)
	if err != nil {
		// Surface the redirect-cap error as ErrTooManyRedirects for callers.
		if errors.Is(err, ErrTooManyRedirects) {
			return "", "", ErrTooManyRedirects
		}
		return "", "", fmt.Errorf("%w: %v", domain.ErrImportFetchFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("%w: %w", domain.ErrImportFetchFailed, &UpstreamStatusError{Status: resp.StatusCode, URL: url})
	}

	if err := assertHTMLContentType(resp.Header.Get("Content-Type")); err != nil {
		return "", "", err
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
