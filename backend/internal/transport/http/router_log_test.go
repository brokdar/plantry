package http_test

import (
	"bytes"
	"encoding/json"
	stdlog "log"
	"log/slog"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	transport "github.com/jaltszeimer/plantry/backend/internal/transport/http"
)

func jsonLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &m), "not JSON: %q", l)
		out = append(out, m)
	}
	return out
}

func serve(t *testing.T, static nethttp.HandlerFunc) (*httptest.ResponseRecorder, []map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := transport.NewRouter(logger, static, transport.Handlers{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(nethttp.MethodGet, "/boom", nil))
	return rec, jsonLines(t, &buf)
}

// A panic answers 500 and leaves one JSON line with the request id, route,
// panic value and stack, then the request line at error level.
func TestRouter_PanicIsLoggedAsJSON(t *testing.T) {
	rec, lines := serve(t, func(nethttp.ResponseWriter, *nethttp.Request) { panic("kaboom") })
	assert.Equal(t, nethttp.StatusInternalServerError, rec.Code)

	var pl, rl map[string]any
	for _, l := range lines {
		switch l["msg"] {
		case "http.panic":
			pl = l
		case "http.request":
			rl = l
		}
	}
	require.NotNil(t, pl, "no http.panic line in %v", lines)
	assert.Equal(t, "ERROR", pl["level"])
	assert.Equal(t, "kaboom", pl["panic"])
	assert.Equal(t, "GET", pl["method"])
	assert.Equal(t, "/boom", pl["path"])
	assert.NotEmpty(t, pl["request_id"])
	assert.Contains(t, pl["stack"], "router_log_test.go")

	require.NotNil(t, rl)
	assert.Equal(t, "ERROR", rl["level"])
	assert.EqualValues(t, 500, rl["status"])
	assert.Equal(t, pl["request_id"], rl["request_id"])
}

// A 5xx is an error line, a 4xx stays info.
func TestRouter_5xxIsErrorLevel(t *testing.T) {
	for code, level := range map[int]string{502: "ERROR", 404: "INFO", 200: "INFO"} {
		_, lines := serve(t, func(w nethttp.ResponseWriter, _ *nethttp.Request) { w.WriteHeader(code) })
		require.Len(t, lines, 1)
		assert.Equal(t, level, lines[0]["level"], "status %d", code)
		assert.NotEmpty(t, lines[0]["request_id"])
	}
}

// A panic after the reply started must not write a second header: net/http
// would print a plain-text "superfluous" line and break the JSON-lines log.
func TestRouter_PanicAfterHeadersWritesNoSecondHeader(t *testing.T) {
	var logBuf, serverErr bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, nil))
	h := transport.NewRouter(logger, nethttp.HandlerFunc(func(w nethttp.ResponseWriter, _ *nethttp.Request) {
		w.WriteHeader(nethttp.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("late")
	}), transport.Handlers{})
	srv := httptest.NewUnstartedServer(h)
	srv.Config.ErrorLog = stdlog.New(&serverErr, "", 0)
	srv.Start()
	defer srv.Close()

	resp, err := nethttp.Get(srv.URL + "/boom")
	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, nethttp.StatusOK, resp.StatusCode)
	assert.NotContains(t, serverErr.String(), "superfluous")
	assert.Contains(t, logBuf.String(), "http.panic")
}
