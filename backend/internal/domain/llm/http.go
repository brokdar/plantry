package llm

import (
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

// maxErrBody caps the provider error body kept for the run log.
const maxErrBody = 4096

// CapBody shortens a provider error body to what the run log keeps.
func CapBody(s string) string {
	if len(s) <= maxErrBody {
		return s
	}
	n := maxErrBody
	for n > 0 && s[n]&0xC0 == 0x80 { // do not cut inside a rune
		n--
	}
	return s[:n] + "…[truncated]"
}

// RateLimitHeaders lists the retry-after and ratelimit headers of a provider
// response ("retry-after=30 anthropic-ratelimit-requests-remaining=0"), the
// part of a 429/529 that says how long to wait.
func RateLimitHeaders(h http.Header) string {
	var parts []string
	for k, v := range h {
		lk := strings.ToLower(k)
		if lk == "retry-after" || strings.Contains(lk, "ratelimit") {
			parts = append(parts, lk+"="+strings.Join(v, ","))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

// NewHTTPClient is the HTTP client of the provider adapters. http.DefaultClient
// has no timeouts, so a hung connection would block until the caller's ctx
// ends. This one bounds connecting, the TLS handshake and the wait for
// response headers; the body (a stream) is bounded by the caller's ctx.
func NewHTTPClient() *http.Client {
	return &http.Client{Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 2 * time.Minute,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}}
}
