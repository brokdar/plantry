package httpfetch

import (
	"errors"
	"net"
	"net/url"
)

// Errors returned by PublicURL.
var (
	ErrInvalidURL = errors.New("httpfetch: invalid url")
	ErrBlockedURL = errors.New("httpfetch: blocked url")
)

// Resolver maps a host name to its IP addresses (net.LookupIP in production).
type Resolver func(host string) ([]net.IP, error)

// PublicURL reports whether raw may be fetched on behalf of an untrusted
// party (an LLM-supplied URL, an image proxy request): an http(s) URL whose
// host resolves only to public addresses. Loopback, private, link-local and
// unspecified addresses are blocked; allowLoopback (PLANTRY_DEV_MODE, the e2e
// fixture site) lifts the loopback block only. A nil resolve uses
// net.LookupIP. The error is ErrInvalidURL or ErrBlockedURL.
func PublicURL(raw string, resolve Resolver, allowLoopback bool) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return ErrInvalidURL
	}
	if resolve == nil {
		resolve = net.LookupIP
	}
	ips, err := resolve(u.Hostname())
	if err != nil || len(ips) == 0 {
		return ErrBlockedURL
	}
	for _, ip := range ips {
		if ip.IsLoopback() && allowLoopback {
			continue
		}
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() ||
			ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return ErrBlockedURL
		}
	}
	return nil
}
