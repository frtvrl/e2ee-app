package api

// middleware.go — chi-compatible middlewares for the REST surface.
//
// Five middlewares, in the order they are wired into the router:
//
//  1. requestID     — attach / propagate X-Request-ID
//  2. deviceContext — pull X-Device-Id-Hash into request context
//  3. accessLog     — log method, path template, status, latency
//  4. cors          — strict allowlist of origins / methods / headers
//  5. apiVersion    — require X-API-Version: 1 (configurable)
//  6. rateLimit     — per-device in-memory token bucket (100 req/dk)
//
// PRIVACY (ADR-0006 §Veri Minimizasyonu + BRD §8 NFR-9)
//
// Every field the access-log emits is enumerated in
// AccessLogFields. The test
// TestAccessLogEmitsOnlyAllowedFields fails the build if a
// forbidden field is ever added.
//
// FORBIDDEN in any log line, response body, or header that
// reaches the client:
//   - the request body (payload bytes — these may contain a
//     phone number or an E2EE payload hash that could be
//     correlated with another user)
//   - raw client IP (we use it as a transient bucket key, then
//     discard it — never write to log)
//   - any Authorization / Cookie header value
//   - the X-Device-Id-Hash header beyond its first 8 hex chars

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// APIVersion is the single value we accept for X-API-Version.
// The constant lives here (not in a config struct) because the
// router constructor wants a literal at compile time. When the
// API moves to v2 the value becomes a []string and the
// AcceptedAPIVersions field of Config drives the check.
const APIVersion = "1"

// Standard header names. Lower-case in constants so they read
// consistently in tests and log output.
const (
	HeaderDeviceIDHash = "X-Device-Id-Hash"
	HeaderAPIVersion   = "X-API-Version"
	HeaderRequestID    = "X-Request-ID"
)

// AccessLogFields enumerates every key that may appear in an
// access-log line. Anything outside this set triggers a test
// failure — the test exists precisely so a future contributor
// cannot accidentally add `r.RemoteAddr` or `r.Header.Get(...)`
// to the log line.
var AccessLogFields = []string{
	"method",
	"path",        // the route template, e.g. "/api/v1/sessions/{id}"
	"status",
	"latency_ms",
	"request_id",
	"device_prefix", // first 8 hex chars of device_id_hash, "" when absent
}

// Logger is the interface the accessLog middleware needs.
// *slog.Logger satisfies it. We accept an interface so tests
// can pass a silent / buffer-backed logger without standing up
// the full slog JSON handler.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Warn(msg string, args ...any)
	Debug(msg string, args ...any)
}

// Private context-key type so we never collide with another
// package's keys.
type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota + 1
	ctxKeyDeviceHash
)

// RequestIDFromContext returns the request id stamped by
// RequestIDMiddleware, or "" if absent.
func RequestIDFromContext(ctx context.Context) string {
	if v := ctx.Value(ctxKeyRequestID); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// DeviceHashFromContext returns the device id hash stamped by
// DeviceContextMiddleware, or "" if absent. NEVER log the
// return value — only its first 8 hex chars are safe to log.
func DeviceHashFromContext(ctx context.Context) string {
	if v := ctx.Value(ctxKeyDeviceHash); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// DevicePrefix returns the first 8 hex chars of the device
// hash — the only form safe to write into a log line.
func DevicePrefix(hash string) string {
	if len(hash) <= 8 {
		return hash
	}
	return hash[:8]
}

// ----- requestID middleware -----

// RequestIDMiddleware attaches X-Request-ID to every request.
// If the client already supplied one we keep it (clipped to
// 64 chars to bound log size, printable ASCII only); otherwise
// we mint a fresh 16-byte hex value. The id is stored in the
// request context AND echoed in the response header.
func RequestIDMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(HeaderRequestID)
			id = clipRequestID(id)
			if id == "" {
				id = newRequestID()
			}
			w.Header().Set(HeaderRequestID, id)
			ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func clipRequestID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if len(s) > 64 {
		s = s[:64]
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c > 0x7e {
			return ""
		}
	}
	return s
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// rand.Read only fails on a broken OS — fall back to a
		// time-based id (still unique within a process).
		ts := time.Now().UnixNano()
		return "ts-" + strconv.FormatInt(ts, 16)
	}
	return hex.EncodeToString(b[:])
}

// DeviceContextMiddleware stores the X-Device-Id-Hash in the
// request context so handlers can pull it without re-reading
// the header. Must run AFTER the request-id middleware so the
// logger has both keys.
func DeviceContextMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get(HeaderDeviceIDHash)
			if h != "" {
				ctx := context.WithValue(r.Context(), ctxKeyDeviceHash, h)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ----- accessLog middleware -----

// statusRecorder wraps http.ResponseWriter so the middleware can
// observe the final status code.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.wroteHeader {
		return
	}
	s.status = code
	s.wroteHeader = true
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.status = http.StatusOK
		s.wroteHeader = true
	}
	return s.ResponseWriter.Write(b)
}

// AccessLogMiddleware emits one JSON log line per request.
//
// Fields emitted (and ONLY these):
//
//	method         GET | POST | DELETE | ...
//	path           the matched route TEMPLATE (never the raw URL with query string)
//	status         HTTP status code (int)
//	latency_ms     wall-clock duration, integer milliseconds
//	request_id     propagated from the request-id middleware
//	device_prefix  first 8 hex chars of X-Device-Id-Hash, or "" when absent
//
// We deliberately do NOT log the raw URL (which can carry
// PII-shaped query parameters for /api/v1/operator/lookup?phone=
// and /api/v1/matrix?country=), the User-Agent, the remote
// address, or any header beyond the device prefix.
func AccessLogMiddleware(log Logger) func(http.Handler) http.Handler {
	if log == nil {
		log = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			latencyMS := time.Since(start).Milliseconds()
			if latencyMS < 0 {
				latencyMS = 0
			}
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			log.Info("http",
				"method", r.Method,
				"path", RoutePath(r),
				"status", status,
				"latency_ms", latencyMS,
				"request_id", RequestIDFromContext(r.Context()),
				"device_prefix", DevicePrefix(DeviceHashFromContext(r.Context())),
			)
		})
	}
}

// RoutePath returns the route template chi matched for this
// request. For unmatched paths we fall back to a coarse bucket
// so the log line still tells operators whether the request hit
// a real route or 404'd. We never return the raw URL.Path
// because it can carry query parameters shaped like PII.
func RoutePath(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if p := rctx.RoutePath; p != "" {
			return p
		}
	}
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/"):
		return "/api/*"
	case strings.HasPrefix(r.URL.Path, "/healthz"):
		return "/healthz"
	default:
		return "/other"
	}
}

// ----- cors middleware -----

// CORSConfig drives the CORS middleware. The defaults match
// production: only the opene2ee.com origins are allowed and
// only the methods/headers the API actually uses are
// pre-flightable.
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	ExposeHeaders  []string
	MaxAge         int
}

// DefaultCORSConfig is the production allowlist. Wire-up
// (PR-8) can override it via the CORSConfig field of
// RouterConfig.
var DefaultCORSConfig = CORSConfig{
	AllowedOrigins: []string{
		"https://opene2ee.com",
		"https://www.opene2ee.com",
		"https://dashboard.opene2ee.com",
	},
	AllowedMethods: []string{"GET", "POST", "DELETE", "OPTIONS"},
	AllowedHeaders: []string{
		"Content-Type",
		HeaderAPIVersion,
		HeaderDeviceIDHash,
		HeaderRequestID,
	},
	ExposeHeaders: []string{
		HeaderRequestID,
		"X-API-Version",
	},
	MaxAge: 600,
}

// CORSMiddleware emits the CORS response headers. For preflight
// (OPTIONS) requests it short-circuits with a 204. For actual
// requests it adds the headers and forwards to the next handler.
//
// We do NOT reflect the request Origin header into
// Access-Control-Allow-Origin. Reflection is the most common
// CORS misconfiguration and would let any malicious site hit
// our API from a logged-in browser.
func CORSMiddleware(cfg CORSConfig) func(http.Handler) http.Handler {
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 600
	}
	if len(cfg.AllowedMethods) == 0 {
		cfg.AllowedMethods = DefaultCORSConfig.AllowedMethods
	}
	if len(cfg.AllowedHeaders) == 0 {
		cfg.AllowedHeaders = DefaultCORSConfig.AllowedHeaders
	}
	if len(cfg.ExposeHeaders) == 0 {
		cfg.ExposeHeaders = DefaultCORSConfig.ExposeHeaders
	}
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		allowed[strings.TrimSpace(o)] = struct{}{}
	}
	methods := strings.Join(cfg.AllowedMethods, ", ")
	headers := strings.Join(cfg.AllowedHeaders, ", ")
	expose := strings.Join(cfg.ExposeHeaders, ", ")
	maxAge := strconv.Itoa(cfg.MaxAge)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if _, ok := allowed[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Methods", methods)
					w.Header().Set("Access-Control-Allow-Headers", headers)
					w.Header().Set("Access-Control-Expose-Headers", expose)
					w.Header().Set("Access-Control-Max-Age", maxAge)
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				// If origin is not in the allowlist, we send NO
				// Access-Control-Allow-Origin header. The browser
				// will block the response from JS — the desired
				// behavior.
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ----- apiVersion middleware -----

// APIVersionMiddleware enforces X-API-Version: <accepted> on
// every API route. We attach it ONLY to the /api/* subtree so
// /healthz (the liveness probe) doesn't need the header.
//
// On miss / wrong value we return a 400 with a stable
// ErrorBody so the mobile app can surface a clear "please
// update" message.
func APIVersionMiddleware(accepted ...string) func(http.Handler) http.Handler {
	if len(accepted) == 0 {
		accepted = []string{APIVersion}
	}
	allowed := make(map[string]struct{}, len(accepted))
	for _, v := range accepted {
		allowed[strings.TrimSpace(v)] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get(HeaderAPIVersion)
			if got == "" {
				writeError(w, http.StatusBadRequest, ErrorBody{
					Code:    CodeMissingHeader,
					Message: "Missing required header: " + HeaderAPIVersion,
				})
				return
			}
			if _, ok := allowed[got]; !ok {
				writeError(w, http.StatusBadRequest, ErrorBody{
					Code:    CodeInvalidHeader,
					Message: "Unsupported " + HeaderAPIVersion + "; expected one of: " + strings.Join(accepted, ", "),
				})
				return
			}
			w.Header().Set("X-API-Version", got)
			next.ServeHTTP(w, r)
		})
	}
}

// ----- rate-limit middleware -----

// RateLimitConfig drives the in-memory token bucket. We use a
// per-device key (X-Device-Id-Hash) when the header is present,
// and a shared "anonymous" bucket otherwise — keeping the
// unauthenticated traffic bounded while preserving the per-
// device 100 req/dk limit called out in BRD §8 NFR-9.
type RateLimitConfig struct {
	Burst           int
	RefillPerMinute int
	AnonymousBurst  int
}

// DefaultRateLimitConfig is the BRD §8 NFR-9 spec verbatim:
// 100 req/dk/device.
var DefaultRateLimitConfig = RateLimitConfig{
	Burst:           100,
	RefillPerMinute: 100,
	AnonymousBurst:  25, // quarter of the per-device quota — anonymous paths are /operator/lookup only
}

// RateLimiter is the in-memory token-bucket implementation.
// Not goroutine-safe across instances (each chi Router gets one
// RateLimiter; horizontal scaling would need a Redis-backed
// bucket, deferred to PR-8).
type RateLimiter struct {
	cfg     RateLimitConfig
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

// bucket is a single token-bucket. Tokens refill continuously
// at RefillPerMinute / 60 per second; we compute the refill
// lazily on every Take() so we don't need a background ticker.
type bucket struct {
	tokens    float64
	lastTaken time.Time
}

// NewRateLimiter builds a RateLimiter with the given config.
// The zero value is NOT usable.
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	if cfg.Burst <= 0 {
		cfg.Burst = DefaultRateLimitConfig.Burst
	}
	if cfg.RefillPerMinute <= 0 {
		cfg.RefillPerMinute = DefaultRateLimitConfig.RefillPerMinute
	}
	if cfg.AnonymousBurst <= 0 {
		cfg.AnonymousBurst = cfg.Burst / 4
		if cfg.AnonymousBurst < 1 {
			cfg.AnonymousBurst = 1
		}
	}
	return &RateLimiter{
		cfg:     cfg,
		buckets: make(map[string]*bucket),
		now:     time.Now,
	}
}

// Take attempts to consume one token. Returns (true, 0) on
// success, (false, retryAfterSeconds) when the bucket is empty.
//
// Bucket-key policy: when X-Device-Id-Hash is present and
// non-empty, the key is "dev:" + hash. Otherwise the key is
// "anon". "anon" buckets are shared across all anonymous
// callers — bounded by AnonymousBurst so a flood from a single
// client can't exhaust the global quota.
func (rl *RateLimiter) Take(deviceHash string) (bool, int) {
	key, capacity := rl.bucketKey(deviceHash)
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := rl.now()
	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(capacity), lastTaken: now}
		rl.buckets[key] = b
	}
	// Refill: tokens added per second = RefillPerMinute / 60.
	refillPerSec := float64(rl.cfg.RefillPerMinute) / 60.0
	elapsed := now.Sub(b.lastTaken).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * refillPerSec
		if b.tokens > float64(capacity) {
			b.tokens = float64(capacity)
		}
		b.lastTaken = now
	}
	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true, 0
	}
	missing := 1.0 - b.tokens
	retryAfter := int(missing/refillPerSec) + 1
	if retryAfter < 1 {
		retryAfter = 1
	}
	return false, retryAfter
}

func (rl *RateLimiter) bucketKey(deviceHash string) (string, int) {
	if deviceHash == "" {
		return "anon", rl.cfg.AnonymousBurst
	}
	return "dev:" + deviceHash, rl.cfg.Burst
}

// RateLimitMiddleware applies rl.Take to every request. The
// device hash is pulled from the request header (so we don't
// need to plumb it through context — handlers can re-read the
// header when they need the full hash).
func RateLimitMiddleware(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			device := r.Header.Get(HeaderDeviceIDHash)
			ok, retryAfter := rl.Take(device)
			if !ok {
				writeRateLimited(w, retryAfter)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// MaxBodyBytes caps the size of any single request body that
// the REST handlers will read. Telemetry rows are < 1 KB in
// practice; sessions are < 4 KB; 64 KB is a comfortable upper
// bound. Larger bodies get a 413 before the handler is invoked.
const MaxBodyBytes = 64 * 1024

// MaxBytesMiddleware wraps r.Body in http.MaxBytesReader so a
// runaway client cannot exhaust server memory. A read past the
// limit surfaces as *http.MaxBytesError; the handler can map
// that to a 413.
func MaxBytesMiddleware(limit int64) func(http.Handler) http.Handler {
	if limit <= 0 {
		limit = MaxBodyBytes
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ----- test / debug helpers -----

// FormatLogLine renders an access-log entry to a stable string.
// Used by tests to assert the exact field set emitted to the
// logger. NOT used at runtime — the production code goes
// straight through slog.
func FormatLogLine(method, path string, status int, latencyMS int64, requestID, devicePrefix string) string {
	return fmt.Sprintf("method=%s path=%s status=%d latency_ms=%d request_id=%s device_prefix=%s",
		method, path, status, latencyMS, requestID, devicePrefix)
}