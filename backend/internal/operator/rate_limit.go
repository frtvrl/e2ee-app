// rate_limit.go — per-IP rate limiting for the Operator Tespit
// Servisi REST surface.
//
// Sprint 3 (PR-23) adds a 60-requests-per-minute per-IP rate limit
// so a single misbehaving client cannot pin the BTK MNP feed (the
// upstream is rate-limited by BTK itself, typically 1 req/s) or
// starve other callers.
//
// TWO IMPLEMENTATIONS
//
//   - InMemoryLimiter: per-process token bucket. No external
//     dependency. Suitable for single-process dev / test, NOT for
//     multi-replica production (each replica has its own bucket).
//
//   - RedisLimiter: shared fixed-window counter backed by Redis
//     INCR + EXPIRE. Suitable for production multi-replica
//     deployments where the limit must be enforced cluster-wide.
//
// CHOOSING AN IMPLEMENTATION
//
//   NewRateLimiter picks InMemoryLimiter by default (no Redis
//   needed for tests). Production wiring in api/router.go should
//   pass RedisLimiter when a Redis client is available.
//
// LIMITS
//
//   The default is 60 requests per minute per IP. This is the
//   constant DefaultRateLimit (also exposed as DefaultRateLimitWindow).
//   Callers can override per-instance via RateLimiterOption.
//
// IDENTIFYING CALLERS
//
//   The Limiter does not parse HTTP requests itself — it takes a
//   string identifier (typically the client IP from the request
//   RemoteAddr, or an X-Forwarded-For value when running behind
//   a trusted reverse proxy). The chi middleware in api/router.go
//   extracts the IP and calls Allow(ip).
//
// FAIL-OPEN vs FAIL-CLOSED
//
//   The RedisLimiter fails OPEN: if Redis is unreachable, Allow
//   returns (true, nil) so an outage in the rate-limit plane does
//   not take down the lookup service. The error is exposed via
//   Limiter.LastError() for observability.
package operator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// DefaultRateLimit is the maximum number of requests per IP per
// DefaultRateLimitWindow. Sprint 3 (PR-23) spec: 60 req/min.
const DefaultRateLimit = 60

// DefaultRateLimitWindow is the sliding time window over which
// the rate limit is enforced. Default 1 minute.
const DefaultRateLimitWindow = 1 * time.Minute

// Limiter is the contract the HTTP middleware depends on.
// Implementations must be safe for concurrent use.
type Limiter interface {
	// Allow returns (true, nil) when the call is within the
	// configured limit, (false, nil) when the limit has been
	// reached, and (false, err) only for unrecoverable errors.
	// Redis outages fail OPEN (true, nil) — see package doc.
	Allow(ctx context.Context, key string) (bool, error)

	// Reset clears the counter for key. Used by tests and by the
	// /admin/rate-limit/reset endpoint.
	Reset(ctx context.Context, key string) error
}

// -----------------------------------------------------------------------------
// In-memory limiter
// -----------------------------------------------------------------------------

// inMemoryBucket is one (key, window) pair's counter.
type inMemoryBucket struct {
	count     int
	resetAt   time.Time
}

// InMemoryLimiter is a single-process token bucket. NOT for
// production multi-replica use — each replica has its own
// counter, so an attacker hitting N replicas in parallel gets
// N * DefaultRateLimit requests through.
type InMemoryLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu      sync.Mutex
	buckets map[string]*inMemoryBucket
}

// NewInMemoryLimiter returns a per-process limiter with the given
// limit and window. Defaults are applied when limit or window
// is non-positive.
func NewInMemoryLimiter(limit int, window time.Duration) *InMemoryLimiter {
	if limit <= 0 {
		limit = DefaultRateLimit
	}
	if window <= 0 {
		window = DefaultRateLimitWindow
	}
	return &InMemoryLimiter{
		limit:   limit,
		window:  window,
		now:     time.Now,
		buckets: make(map[string]*inMemoryBucket),
	}
}

// Allow increments the bucket for key and returns whether the
// call is within the limit.
func (l *InMemoryLimiter) Allow(_ context.Context, key string) (bool, error) {
	if key == "" {
		return false, errors.New("operator: rate limit: empty key")
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok || now.After(b.resetAt) {
		b = &inMemoryBucket{count: 0, resetAt: now.Add(l.window)}
		l.buckets[key] = b
	}
	if b.count >= l.limit {
		return false, nil
	}
	b.count++
	return true, nil
}

// Reset clears the bucket for key. Idempotent.
func (l *InMemoryLimiter) Reset(_ context.Context, key string) error {
	if key == "" {
		return errors.New("operator: rate limit: empty key")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.buckets, key)
	return nil
}

// SweepStale removes buckets whose resetAt has passed. Callers
// can invoke this periodically to bound memory growth in long-
// running single-process deployments. Returns the number of
// buckets removed.
func (l *InMemoryLimiter) SweepStale() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	removed := 0
	for k, b := range l.buckets {
		if now.After(b.resetAt) {
			delete(l.buckets, k)
			removed++
		}
	}
	return removed
}

// -----------------------------------------------------------------------------
// Redis-backed limiter
// -----------------------------------------------------------------------------

// RedisLimiter is the production multi-replica limiter backed
// by a Redis fixed-window counter. The key format is:
//
//	<prefix>:<window-epoch>:<ip>
//
// where window-epoch = unix_seconds / window_seconds. INCR on a
// missing key starts at 1; we follow with EXPIRE to bound the
// key's lifetime to 2× the window so a missing EXPIRE call (e.g.
// transient network blip after INCR but before EXPIRE) does not
// leak a permanent counter.
//
// Fail-open on Redis errors: Allow returns (true, nil) when the
// INCR/EXPIRE round-trip fails. The error is exposed via
// LastError() for observability / alerting.
type RedisLimiter struct {
	client  *redis.Client
	prefix  string
	limit   int
	window  time.Duration
	now     func() time.Time

	// lastErr is the most recent backend error, for observability.
	// Tests assert against it.
	mu      sync.Mutex
	lastErr error
}

// NewRedisLimiter validates cfg and returns a usable limiter.
// prefix must be non-empty; client must be non-nil.
func NewRedisLimiter(client *redis.Client, prefix string, limit int, window time.Duration) (*RedisLimiter, error) {
	if client == nil {
		return nil, errors.New("operator: NewRedisLimiter: nil client")
	}
	if prefix == "" {
		return nil, errors.New("operator: NewRedisLimiter: empty prefix")
	}
	if limit <= 0 {
		limit = DefaultRateLimit
	}
	if window <= 0 {
		window = DefaultRateLimitWindow
	}
	return &RedisLimiter{
		client: client,
		prefix: prefix,
		limit:  limit,
		window: window,
		now:    time.Now,
	}, nil
}

// Allow increments the counter for key and returns whether the
// call is within the limit. Fails OPEN on Redis errors.
func (l *RedisLimiter) Allow(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, errors.New("operator: RedisLimiter.Allow: empty key")
	}
	windowSec := int64(l.window / time.Second)
	if windowSec == 0 {
		windowSec = 1
	}
	epoch := l.now().Unix() / windowSec
	redisKey := fmt.Sprintf("%s:%d:%s", l.prefix, epoch, key)

	count, err := l.client.Incr(ctx, redisKey).Result()
	if err != nil {
		l.recordErr(err)
		// Fail open — a Redis outage must not take down the API.
		return true, nil
	}
	if count == 1 {
		// New key — set the expiry. If this fails the worst case
		// is a permanent counter (memory leak), not a wrong
		// answer. Best-effort.
		if err := l.client.Expire(ctx, redisKey, 2*l.window).Err(); err != nil {
			l.recordErr(err)
		}
	}
	return count <= int64(l.limit), nil
}

// Reset deletes the current window's counter for key. Idempotent.
func (l *RedisLimiter) Reset(ctx context.Context, key string) error {
	if key == "" {
		return errors.New("operator: RedisLimiter.Reset: empty key")
	}
	windowSec := int64(l.window / time.Second)
	if windowSec == 0 {
		windowSec = 1
	}
	epoch := l.now().Unix() / windowSec
	redisKey := fmt.Sprintf("%s:%d:%s", l.prefix, epoch, key)
	if err := l.client.Del(ctx, redisKey).Err(); err != nil {
		l.recordErr(err)
		return err
	}
	return nil
}

// LastError returns the most recent backend error (for
// observability). Returns nil when the limiter has been
// successful since construction.
func (l *RedisLimiter) LastError() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lastErr
}

func (l *RedisLimiter) recordErr(err error) {
	l.mu.Lock()
	l.lastErr = err
	l.mu.Unlock()
}

// -----------------------------------------------------------------------------
// Default selector
// -----------------------------------------------------------------------------

// NewRateLimiter returns the default Limiter for this process.
// When a Redis client is provided (non-nil) a RedisLimiter is
// returned; otherwise an InMemoryLimiter is used. This keeps
// the production code path simple ("just call NewRateLimiter
// with the same Redis client the cache uses") while letting
// tests run without Redis.
func NewRateLimiter(client *redis.Client, prefix string) (Limiter, error) {
	if client == nil {
		return NewInMemoryLimiter(DefaultRateLimit, DefaultRateLimitWindow), nil
	}
	return NewRedisLimiter(client, prefix, DefaultRateLimit, DefaultRateLimitWindow)
}