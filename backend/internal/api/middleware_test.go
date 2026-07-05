package api

// middleware_test.go — middleware-specific tests.
//
// Covers:
//   - CORS: allowed origin emits headers; blocked origin emits
//     none (browser then refuses)
//   - CORS: preflight OPTIONS short-circuits with 204
//   - Rate limit: burst is honored; refill is honored over time
//   - Rate limit: per-device bucket isolated from anon bucket
//   - Access log: emits exactly the allowed fields (privacy)
//   - MaxBytes: rejects oversized bodies with 413

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------------
// CORS
// -----------------------------------------------------------------------------

func TestCORS_AllowedOriginEmitsHeaders(t *testing.T) {
	h := CORSMiddleware(DefaultCORSConfig)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/api/v1/sessions", nil)
	req.Header.Set("Origin", "https://dashboard.opene2ee.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "https://dashboard.opene2ee.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), HeaderAPIVersion)
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), HeaderDeviceIDHash)
	assert.Contains(t, w.Header().Get("Access-Control-Expose-Headers"), HeaderRequestID)
}

func TestCORS_BlockedOriginEmitsNoHeaders(t *testing.T) {
	h := CORSMiddleware(DefaultCORSConfig)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/api/v1/sessions", nil)
	req.Header.Set("Origin", "https://attacker.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// The handler still runs, but the browser will block the
	// response because no Access-Control-Allow-Origin was set.
	assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
}

func TestCORS_PreflightShortCircuits(t *testing.T) {
	handlerCalled := false
	h := CORSMiddleware(DefaultCORSConfig)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("OPTIONS", "/api/v1/sessions", nil)
	req.Header.Set("Origin", "https://opene2ee.com")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.False(t, handlerCalled, "OPTIONS preflight must NOT reach the inner handler")
}

// -----------------------------------------------------------------------------
// Rate limit
// -----------------------------------------------------------------------------

func TestRateLimit_BurstThen429(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{Burst: 5, RefillPerMinute: 60, AnonymousBurst: 5})
	for i := 0; i < 5; i++ {
		ok, _ := rl.Take("device-1")
		require.True(t, ok, "request %d/5 should succeed", i+1)
	}
	// 6th must fail
	ok, retry := rl.Take("device-1")
	assert.False(t, ok, "6th request must hit the bucket floor")
	assert.GreaterOrEqual(t, retry, 1)
}

func TestRateLimit_PerDeviceIsolation(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{Burst: 3, RefillPerMinute: 60, AnonymousBurst: 3})
	for i := 0; i < 3; i++ {
		ok, _ := rl.Take("device-A")
		require.True(t, ok)
	}
	ok, _ := rl.Take("device-A")
	require.False(t, ok, "device-A exhausted")

	// device-B still has its full quota.
	for i := 0; i < 3; i++ {
		ok, _ := rl.Take("device-B")
		require.True(t, ok, "device-B must not be affected by device-A's bucket")
	}
}

func TestRateLimit_AnonHasOwnSmallerBucket(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{Burst: 100, RefillPerMinute: 100, AnonymousBurst: 2})
	// Two anon requests succeed.
	for i := 0; i < 2; i++ {
		ok, _ := rl.Take("")
		require.True(t, ok)
	}
	ok, _ := rl.Take("")
	require.False(t, ok, "third anon request must 429 — anon bucket is much smaller")
}

func TestRateLimit_RefillAfterTime(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{Burst: 1, RefillPerMinute: 600, AnonymousBurst: 1}) // 10/sec refill
	ok, _ := rl.Take("dev")
	require.True(t, ok)
	ok, _ = rl.Take("dev")
	require.False(t, ok, "burst exhausted")

	// Advance the clock past the refill interval (1 token at
	// 10/s = 100ms).
	rl.now = func() time.Time { return time.Now().Add(150 * time.Millisecond) }
	ok, _ = rl.Take("dev")
	assert.True(t, ok, "after 150ms the bucket should have refilled ≥ 1 token")
}

func TestRateLimit_429ResponseIncludesRetryAfter(t *testing.T) {
	ta := newTestAPI(t)
	// Set the rate-limit burst to 1 so we can exhaust quickly.
	rl := NewRateLimiter(RateLimitConfig{Burst: 1, RefillPerMinute: 60, AnonymousBurst: 1})
	ta.API.cfg.RateLimit = rl
	// Re-wire the router so the new rate limiter is used.
	mux := ta.Handler()

	headers := withAPIHeaders(map[string]string{
		HeaderDeviceIDHash: "test-device-hash",
	})
	// First request succeeds.
	w := do(t, mux, "GET", "/api/v1/sessions", headers, "")
	require.Equal(t, http.StatusOK, w.Code)
	// Second request (same device, same bucket) is 429.
	w = do(t, mux, "GET", "/api/v1/sessions", headers, "")
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}

// -----------------------------------------------------------------------------
// Access log privacy
// -----------------------------------------------------------------------------

func TestAccessLogEmitsOnlyAllowedFields(t *testing.T) {
	logger := newFakeLogger()
	// Wire the full chain so the request-id + device context
	// middlewares populate ctx before AccessLogMiddleware
	// reads them.
	h := RequestIDMiddleware()(DeviceContextMiddleware()(AccessLogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))))

	req := httptest.NewRequest("POST", "/api/v1/sessions", strings.NewReader(`{"a":1}`))
	req.Header.Set(HeaderRequestID, "test-req-id")
	req.Header.Set(HeaderDeviceIDHash, "abcdef0123456789abcdef0123456789")
	// These SHOULD NOT appear in the log:
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("X-Forwarded-For", "192.0.2.42")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	require.NotEmpty(t, logger.Entries)
	e := logger.LastEntry()
	require.Equal(t, "info", e.Level)
	require.Equal(t, "http", e.Msg)

	// Every key in the log line must be in AccessLogFields.
	for k := range e.Args {
		found := false
		for _, allowed := range AccessLogFields {
			if k == allowed {
				found = true
				break
			}
		}
		assert.True(t, found, "log key %q is not in AccessLogFields whitelist", k)
	}
	// Required keys present.
	assert.Equal(t, "POST", e.Args["method"])
	assert.Equal(t, http.StatusCreated, e.Args["status"])
	assert.Equal(t, "test-req-id", e.Args["request_id"])
	assert.Equal(t, "abcdef01", e.Args["device_prefix"], "device_prefix must be first 8 chars only")

	// Sanity-check that the dangerous headers were not echoed.
	for k := range e.Args {
		assert.NotContains(t, k, "Authorization")
		assert.NotContains(t, k, "X-Forwarded-For")
		assert.NotContains(t, k, "Bearer")
	}
}

func TestAccessLog_NoDevicePrefixWhenHeaderAbsent(t *testing.T) {
	logger := newFakeLogger()
	accessLog := AccessLogMiddleware(logger)
	h := accessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/api/v1/sessions", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	e := logger.LastEntry()
	assert.Equal(t, "", e.Args["device_prefix"])
}

func TestAccessLog_DoesNotLogRawURL(t *testing.T) {
	logger := newFakeLogger()
	accessLog := AccessLogMiddleware(logger)
	h := accessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// URL with a PII-shaped query parameter.
	req := httptest.NewRequest("GET", "/api/v1/operator/lookup?qtype=phone_e164&q=%2B905321234567", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	e := logger.LastEntry()
	pathVal, _ := e.Args["path"].(string)
	assert.NotContains(t, pathVal, "+905321234567",
		"raw URL (with query) must NEVER appear in the log line")
	assert.NotContains(t, pathVal, "905321234567",
		"phone number digits must never appear in the log line")
}

// -----------------------------------------------------------------------------
// MaxBytes
// -----------------------------------------------------------------------------

func TestMaxBytes_RejectsOversizedBody(t *testing.T) {
	max := MaxBytesMiddleware(64)
	h := max(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		_, err := r.Body.Read(buf)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("POST", "/api/v1/sessions", strings.NewReader(strings.Repeat("a", 200)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	// The handler will hit MaxBytesError on the Read; we don't
	// care which status comes out as long as it's NOT a
	// successful 200 (the limit is enforced).
	assert.NotEqual(t, http.StatusOK, w.Code)
}

// -----------------------------------------------------------------------------
// Concurrency safety of the rate limiter
// -----------------------------------------------------------------------------

func TestRateLimit_ConcurrentAccessIsSafe(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{Burst: 1000, RefillPerMinute: 60000, AnonymousBurst: 1000})
	var wg sync.WaitGroup
	var ok, denied int64
	var mu sync.Mutex
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, _ := rl.Take("concurrent-device")
			mu.Lock()
			if got {
				ok++
			} else {
				denied++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	// Burst is 1000 — 200 concurrent requests must all succeed.
	assert.Equal(t, int64(200), ok, "all 200 concurrent takes must succeed within burst")
	assert.Equal(t, int64(0), denied)
}