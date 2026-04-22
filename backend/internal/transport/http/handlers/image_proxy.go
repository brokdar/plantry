package handlers

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const proxyMaxBytes = 10 << 20 // 10 MB

// ImageProxyHandler fetches an image from a user-supplied URL on the server
// side and streams the raw bytes back to the browser. The proxy bypasses
// image-host CORS and provides SSRF protection before each request.
type ImageProxyHandler struct {
	client  *http.Client
	resolve func(host string) ([]net.IP, error)
}

// NewImageProxyHandler returns a handler configured with a 30-second HTTP client.
func NewImageProxyHandler() *ImageProxyHandler {
	return &ImageProxyHandler{
		client:  &http.Client{Timeout: 30 * time.Second},
		resolve: net.LookupIP,
	}
}

// Fetch handles POST /api/image/fetch-url.
func (h *ImageProxyHandler) Fetch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_body")
		return
	}
	if body.URL == "" {
		writeError(w, http.StatusBadRequest, "error.image.url_invalid")
		return
	}

	if err := h.validateURL(body.URL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, body.URL, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.image.url_invalid")
		return
	}
	req.Header.Set("Accept", "image/*")
	req.Header.Set("User-Agent", "Plantry/1.0")

	resp, err := h.client.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "error.image.url_fetch_failed")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadGateway, "error.image.url_fetch_failed")
		return
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)

	limited := io.LimitReader(resp.Body, proxyMaxBytes)
	_, _ = io.Copy(w, limited)
}

func (h *ImageProxyHandler) validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errInvalidURL
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errInvalidURL
	}
	host := u.Hostname()
	if host == "" {
		return errInvalidURL
	}

	ips, err := h.resolve(host)
	if err != nil || len(ips) == 0 {
		return errBlockedURL
	}
	for _, ip := range ips {
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() ||
			ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return errBlockedURL
		}
	}
	return nil
}

type proxyErr string

func (e proxyErr) Error() string { return string(e) }

var (
	errInvalidURL proxyErr = "error.image.url_invalid"
	errBlockedURL proxyErr = "error.image.url_blocked"
)
