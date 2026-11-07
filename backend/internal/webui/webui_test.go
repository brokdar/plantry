package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandler(t *testing.T) {
	h := newHandler(fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html>shell")},
		"favicon.svg":     {Data: []byte("<svg/>")},
		"assets/app-1.js": {Data: []byte("console.log(1)")},
	})

	tests := []struct {
		name      string
		path      string
		wantCode  int
		wantCache string
		wantBody  string
	}{
		{"index", "/", http.StatusOK, "no-cache", "shell"},
		{"client route falls back to the shell", "/recipes/3", http.StatusOK, "no-cache", "shell"},
		{"hashed asset is immutable", "/assets/app-1.js", http.StatusOK, "public, max-age=31536000, immutable", "console.log(1)"},
		{"unhashed file is revalidated", "/favicon.svg", http.StatusOK, "no-cache", "<svg/>"},
		{"missing asset is a 404", "/assets/gone-2.js", http.StatusNotFound, "", ""},
		{"missing file with an extension is a 404", "/missing.png", http.StatusNotFound, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}
