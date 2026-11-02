package httpfetch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// Errors returned by PublicURL.
var (
	ErrInvalidURL = errors.New("httpfetch: invalid url")
	ErrBlockedURL = errors.New("httpfetch: blocked url")
)

// Resolver maps a host name to its IP addresses (net.LookupIP in production).
type Resolver func(host string) ([]net.IP, error)

// nonPublic are the special-purpose IPv4 ranges the net.IP predicates do not
// cover.
var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // CGNAT, Tailscale
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // TEST-NET-1
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // TEST-NET-2
	netip.MustParsePrefix("203.0.113.0/24"),  // TEST-NET-3
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, broadcast
}

// blocked reports whether ip is not a public unicast address. allowLoopback
// lifts the loopback block only.
func blocked(ip net.IP, allowLoopback bool) bool {
	if ip.IsLoopback() {
		return !allowLoopback
	}
	if ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
		return true
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	for _, p := range nonPublic {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// PublicURL reports whether raw may be fetched on behalf of an untrusted
// party (an LLM-supplied URL, an image proxy request): an http(s) URL whose
// host resolves only to public addresses. Loopback, private, link-local,
// CGNAT, multicast, reserved and unspecified addresses are blocked;
// allowLoopback (PLANTRY_DEV_MODE, the e2e fixture site) lifts the loopback
// block only. A nil resolve uses net.LookupIP. The error is ErrInvalidURL or
// ErrBlockedURL. It is a pre-check only: guarded requests also go through
// GuardedClient (or WithDialGuard), which re-checks the dialled address.
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
		if blocked(ip, allowLoopback) {
			return ErrBlockedURL
		}
	}
	return nil
}

// guardedDial resolves the host once and connects only when every address is
// public, so a host that re-resolves to a private address after PublicURL's
// pre-check (DNS rebinding) is refused. A nil resolve uses the default
// resolver.
func guardedDial(resolve Resolver, allowLoopback bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	var d net.Dialer
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		var ips []net.IP
		if resolve != nil {
			ips, err = resolve(host)
		} else {
			ips, err = net.DefaultResolver.LookupIP(ctx, "ip", host)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: resolve %s: %w", ErrBlockedURL, host, err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("%w: %s has no address", ErrBlockedURL, host)
		}
		for _, ip := range ips {
			if blocked(ip, allowLoopback) {
				return nil, fmt.Errorf("%w: %s resolves to %s", ErrBlockedURL, host, ip)
			}
		}
		var lastErr error
		for _, ip := range ips {
			conn, err := d.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

// GuardedClient is an http.Client for requests on behalf of an untrusted
// party: it dials public addresses only (allowLoopback: dev mode), uses no
// proxy, follows at most 5 redirects and re-runs the ctx's URL guard
// (importer.WithURLGuard) on every hop. A nil resolve uses the default
// resolver.
func GuardedClient(resolve Resolver, allowLoopback bool) *http.Client {
	return guardedClient(resolve, allowLoopback, defaultTimeout, defaultMaxRedirects)
}

func guardedClient(resolve Resolver, allowLoopback bool, timeout time.Duration, maxRedirects int) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = nil
	tr.DialContext = guardedDial(resolve, allowLoopback)
	return &http.Client{Timeout: timeout, Transport: tr, CheckRedirect: checkRedirect(maxRedirects)}
}

// checkRedirect caps the redirects at maxRedirects and re-runs the ctx's URL
// guard on every hop.
func checkRedirect(maxRedirects int) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return ErrTooManyRedirects
		}
		if guard := importer.URLGuardFrom(req.Context()); guard != nil {
			if err := guard(req.URL.String()); err != nil {
				return fmt.Errorf("%w: %w", importer.ErrURLBlocked, err)
			}
		}
		return nil
	}
}
