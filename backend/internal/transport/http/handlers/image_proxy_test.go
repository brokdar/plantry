package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testImagePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newProxy(resolve func(string) ([]net.IP, error)) *ImageProxyHandler {
	return &ImageProxyHandler{
		client:  &http.Client{},
		resolve: resolve,
	}
}

func doFetch(t *testing.T, h *ImageProxyHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/image/fetch-url", strings.NewReader(body))
	h.Fetch(rr, req)
	return rr
}

func TestImageProxy_Success(t *testing.T) {
	png := testImagePNG(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer srv.Close()

	h := newProxy(func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})

	rr := doFetch(t, h, `{"url":"`+srv.URL+`/x.png"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("content-type = %q", ct)
	}
	if !bytes.Equal(rr.Body.Bytes(), png) {
		t.Errorf("body mismatch")
	}
}

func TestImageProxy_RejectsPrivateIP(t *testing.T) {
	cases := []string{
		"127.0.0.1", "10.0.0.5", "192.168.1.1", "::1", "169.254.0.1",
	}
	for _, ip := range cases {
		t.Run(ip, func(t *testing.T) {
			h := newProxy(func(string) ([]net.IP, error) {
				return []net.IP{net.ParseIP(ip)}, nil
			})
			rr := doFetch(t, h, `{"url":"http://evil.example/x.png"}`)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", rr.Code)
			}
			var body map[string]any
			_ = json.NewDecoder(rr.Body).Decode(&body)
			if body["message_key"] != "error.image.url_blocked" {
				t.Errorf("message_key = %v", body["message_key"])
			}
		})
	}
}

func TestImageProxy_RejectsNonHTTPScheme(t *testing.T) {
	h := newProxy(func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("1.1.1.1")}, nil })
	rr := doFetch(t, h, `{"url":"file:///etc/passwd"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestImageProxy_DNSFailureBlocks(t *testing.T) {
	h := newProxy(func(string) ([]net.IP, error) {
		return nil, errors.New("dns lookup failed")
	})
	rr := doFetch(t, h, `{"url":"http://nonexistent.example/x.png"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestImageProxy_EmptyURL(t *testing.T) {
	h := newProxy(func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("1.1.1.1")}, nil })
	rr := doFetch(t, h, `{"url":""}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestImageProxy_InvalidBody(t *testing.T) {
	h := newProxy(func(string) ([]net.IP, error) { return []net.IP{net.ParseIP("1.1.1.1")}, nil })
	rr := doFetch(t, h, `not-json`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestImageProxy_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	h := newProxy(func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	rr := doFetch(t, h, `{"url":"`+srv.URL+`/missing"}`)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestImageProxy_SizeCap(t *testing.T) {
	big := bytes.Repeat([]byte{0x55}, (proxyMaxBytes + 100))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(big)
	}))
	defer srv.Close()

	h := newProxy(func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	})
	rr := doFetch(t, h, `{"url":"`+srv.URL+`/huge.jpg"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	if got := rr.Body.Len(); got != proxyMaxBytes {
		t.Errorf("body len = %d, want %d", got, proxyMaxBytes)
	}
	_ = io.Discard
}
