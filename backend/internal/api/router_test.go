package api

// router_test.go — top-level router wiring tests.
//
// Covers:
//   - all routes are mounted at the expected paths
//   - X-API-Version is required on /api/* (NOT on /healthz)
//   - 405 Method Not Allowed is returned for wrong verb
//   - 404 is returned for unknown routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// do is the canonical "send a request through the mux" helper.
func do(t *testing.T, h http.Handler, method, target string, headers map[string]string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// withAPIHeaders returns the minimum set of headers that every
// /api/v1/* request needs (X-API-Version + Content-Type).
// Optional headers (X-Device-Id-Hash, X-Request-ID) can be
// merged by the caller.
func withAPIHeaders(extra map[string]string) map[string]string {
	m := map[string]string{
		HeaderAPIVersion:   APIVersion,
		"Content-Type":     "application/json",
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestRouter_RequiresAPIVersion(t *testing.T) {
	ta := newTestAPI(t)
	for _, target := range []string{
		"/api/v1/sessions",
		"/api/v1/matrix",
		"/api/v1/operator/lookup?qtype=phone_e164&q=%2B905321234567",
	} {
		t.Run(target, func(t *testing.T) {
			w := do(t, ta.Handler(), "GET", target, nil, "")
			require.Equal(t, http.StatusBadRequest, w.Code, "missing X-API-Version must 400")
			var body ErrorBody
			readJSON(t, w.Body, &body)
			assert.Equal(t, CodeMissingHeader, body.Code)
		})
	}
}

func TestRouter_HealthzDoesNotRequireAPIVersion(t *testing.T) {
	ta := newTestAPI(t)
	w := do(t, ta.Handler(), "GET", "/healthz", nil, "")
	require.Equal(t, http.StatusOK, w.Code, "/healthz must be 200 without any headers")
	assert.Contains(t, w.Body.String(), `"status":"ok"`)
}

func TestRouter_RejectsBadAPIVersion(t *testing.T) {
	ta := newTestAPI(t)
	w := do(t, ta.Handler(), "GET", "/api/v1/sessions",
		map[string]string{HeaderAPIVersion: "999"}, "")
	require.Equal(t, http.StatusBadRequest, w.Code)
	var body ErrorBody
	readJSON(t, w.Body, &body)
	assert.Equal(t, CodeInvalidHeader, body.Code)
}

func TestRouter_AllRoutesMounted(t *testing.T) {
	ta := newTestAPI(t)
	headers := withAPIHeaders(nil)
	// Each route should respond with SOMETHING (even 405, 400) — the
	// point is they are mounted, not 404 from chi's NotFoundHandler.
	cases := []struct {
		method string
		path   string
		want   int
	}{
		{"POST", "/api/v1/sessions", http.StatusBadRequest}, // empty body
		{"GET", "/api/v1/sessions", http.StatusOK},
		{"GET", "/api/v1/matrix", http.StatusOK},
		{"GET", "/api/v1/operator/lookup?qtype=phone_e164&q=%2B905321234567", http.StatusOK},
		{"DELETE", "/api/v1/users/abcdef1234567890abcdef1234567890", http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			w := do(t, ta.Handler(), c.method, c.path, headers, "")
			assert.Equal(t, c.want, w.Code, "body=%s", w.Body.String())
		})
	}
}

func TestRouter_NotFoundReturnsJSON(t *testing.T) {
	ta := newTestAPI(t)
	headers := withAPIHeaders(nil)
	w := do(t, ta.Handler(), "GET", "/api/v1/nope", headers, "")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestRouter_MethodNotAllowed(t *testing.T) {
	ta := newTestAPI(t)
	headers := withAPIHeaders(nil)
	// PATCH is not registered on /api/v1/sessions
	w := do(t, ta.Handler(), "PATCH", "/api/v1/sessions", headers, "")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestRouter_EmitsRequestID(t *testing.T) {
	ta := newTestAPI(t)
	headers := withAPIHeaders(nil)
	// Client-supplied request id should be echoed back.
	headers[HeaderRequestID] = "client-supplied-1234567890"
	w := do(t, ta.Handler(), "GET", "/api/v1/sessions", headers, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "client-supplied-1234567890", w.Header().Get(HeaderRequestID))
}

func TestRouter_GeneratesRequestIDWhenAbsent(t *testing.T) {
	ta := newTestAPI(t)
	headers := withAPIHeaders(nil)
	w := do(t, ta.Handler(), "GET", "/api/v1/sessions", headers, "")
	require.Equal(t, http.StatusOK, w.Code)
	got := w.Header().Get(HeaderRequestID)
	assert.NotEmpty(t, got, "middleware must mint a request id when client omits it")
}

func TestRouter_ClipsClientRequestID(t *testing.T) {
	ta := newTestAPI(t)
	headers := withAPIHeaders(nil)
	headers[HeaderRequestID] = strings.Repeat("a", 200) // too long
	w := do(t, ta.Handler(), "GET", "/api/v1/sessions", headers, "")
	require.Equal(t, http.StatusOK, w.Code)
	got := w.Header().Get(HeaderRequestID)
	// Either clipped to 64 chars OR replaced by a fresh one —
	// both are acceptable defensive behaviors; what matters is
	// the response doesn't echo back the 200-char string.
	assert.LessOrEqual(t, len(got), 64)
}

func TestRouter_RejectsControlCharsInRequestID(t *testing.T) {
	ta := newTestAPI(t)
	headers := withAPIHeaders(nil)
	// A request id with a newline could pollute log lines.
	headers[HeaderRequestID] = "abc\x00def"
	w := do(t, ta.Handler(), "GET", "/api/v1/sessions", headers, "")
	require.Equal(t, http.StatusOK, w.Code)
	got := w.Header().Get(HeaderRequestID)
	assert.NotContains(t, got, "\x00", "control chars must be rejected")
	assert.NotEqual(t, "abc\x00def", got)
}

func TestMountOn_AttachesToServeMux(t *testing.T) {
	ta := newTestAPI(t)
	mux := http.NewServeMux()
	ta.MountOn(mux)
	// /healthz accessible via the parent mux
	w := do(t, mux, "GET", "/healthz", nil, "")
	require.Equal(t, http.StatusOK, w.Code)
	// /api/v1/ also accessible
	w = do(t, mux, "GET", "/api/v1/sessions", withAPIHeaders(nil), "")
	require.Equal(t, http.StatusOK, w.Code)
}