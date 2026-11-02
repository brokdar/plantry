package discovery

import (
	"net/url"
	"strings"
)

// trackingParams are query parameters that do not change the page.
var trackingParams = map[string]bool{"fbclid": true, "gclid": true}

// NormalizeURL is raw as compared for duplicates (not as stored): scheme and
// host lowercased, fragment, utm_*/fbclid/gclid parameters and a trailing
// path slash dropped. An unparsable raw is returned trimmed.
func NormalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Scheme, u.Host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
	u.Fragment, u.RawFragment = "", ""
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			lk := strings.ToLower(k)
			if strings.HasPrefix(lk, "utm_") || trackingParams[lk] {
				q.Del(k)
			}
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}
