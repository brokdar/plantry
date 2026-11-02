package httpfetch_test

import (
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jaltszeimer/plantry/backend/internal/adapters/httpfetch"
)

func resolveTo(ips ...string) httpfetch.Resolver {
	return func(string) ([]net.IP, error) {
		out := make([]net.IP, len(ips))
		for i, s := range ips {
			out[i] = net.ParseIP(s)
		}
		return out, nil
	}
}

func TestPublicURL(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		resolve  httpfetch.Resolver
		loopback bool
		want     error
	}{
		{"public", "https://example.com/r", resolveTo("93.184.216.34"), false, nil},
		{"private", "http://10.0.0.5/r", resolveTo("10.0.0.5"), false, httpfetch.ErrBlockedURL},
		{"loopback blocked", "http://localhost/r", resolveTo("127.0.0.1"), false, httpfetch.ErrBlockedURL},
		{"loopback in dev mode", "http://localhost/r", resolveTo("127.0.0.1"), true, nil},
		{"private stays blocked in dev mode", "http://nas/r", resolveTo("192.168.1.2"), true, httpfetch.ErrBlockedURL},
		{"link local", "http://x/r", resolveTo("169.254.1.1"), false, httpfetch.ErrBlockedURL},
		{"one private among public", "http://x/r", resolveTo("93.184.216.34", "10.0.0.1"), false, httpfetch.ErrBlockedURL},
		{"unresolvable", "http://x/r", func(string) ([]net.IP, error) { return nil, errors.New("nxdomain") }, false, httpfetch.ErrBlockedURL},
		{"bad scheme", "ftp://example.com/r", resolveTo("93.184.216.34"), false, httpfetch.ErrInvalidURL},
		{"no host", "http:///r", resolveTo("93.184.216.34"), false, httpfetch.ErrInvalidURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := httpfetch.PublicURL(tt.raw, tt.resolve, tt.loopback)
			if tt.want == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.want)
		})
	}
}
