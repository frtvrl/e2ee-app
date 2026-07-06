// rate_limit_test.go — unit tests for the per-IP rate limiter.
//
// InMemoryLimiter is exercised synchronously. RedisLimiter is
// exercised against miniredis (no live Redis needed).
package operator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// In-memory limiter
// ---------------------------------------------------------------------------

func TestInMemoryLimiter_AllowsUpToLimit(t *testing.T) {
	l := NewInMemoryLimiter(3, 1*time.Minute)
	for i := 1; i <= 3; i++ {
		ok, err := l.Allow(context.Background(), "1.2.3.4")
		if err != nil {
			t.Fatalf("Allow #%d: %v", i, err)
		}
		if !ok {
			t.Errorf("Allow #%d returned false; want true within limit", i)
		}
	}
}

func TestInMemoryLimiter_RejectsPastLimit(t *testing.T) {
	l := NewInMemoryLimiter(2, 1*time.Minute)
	for i := 0; i < 2; i++ {
		_, _ = l.Allow(context.Background(), "1.2.3.4")
	}
	ok, err := l.Allow(context.Background(), "1.2.3.4")
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if ok {
		t.Error("Allow returned true past limit; want false")
	}
}

func TestInMemoryLimiter_PerKeyIsolation(t *testing.T) {
	// 1 req on key A and 1 req on key B must NOT share a counter.
	l := NewInMemoryLimiter(1, 1*time.Minute)
	if ok, _ := l.Allow(context.Background(), "A"); !ok {
		t.Error("first A rejected")
	}
	if ok, _ := l.Allow(context.Background(), "B"); !ok {
		t.Error("first B rejected — counters shared between keys")
	}
}

func TestInMemoryLimiter_WindowReset(t *testing.T) {
	// Inject a controllable clock so we can fast-forward the
	// window without sleeping.
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	l := NewInMemoryLimiter(1, 1*time.Minute)
	l.now = func() time.Time { return now }

	// First call within window 1.
	if ok, _ := l.Allow(context.Background(), "X"); !ok {
		t.Fatal("first call rejected")
	}
	// Second call within same window — should be rejected.
	if ok, _ := l.Allow(context.Background(), "X"); ok {
		t.Fatal("second call within window accepted")
	}
	// Advance the clock past the window.
	now = now.Add(2 * time.Minute)
	if ok, _ := l.Allow(context.Background(), "X"); !ok {
		t.Error("post-window call rejected; counter not reset")
	}
}

func TestInMemoryLimiter_Reset(t *testing.T) {
	l := NewInMemoryLimiter(1, 1*time.Minute)
	if _, _ = l.Allow(context.Background(), "X"); false {
	}
	if _, _ = l.Allow(context.Background(), "X"); false {
	}
	if err := l.Reset(context.Background(), "X"); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if ok, _ := l.Allow(context.Background(), "X"); !ok {
		t.Error("post-Reset call rejected")
	}
}

func TestInMemoryLimiter_SweepStale(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	l := NewInMemoryLimiter(1, 1*time.Minute)
	l.now = func() time.Time { return now }
	_, _ = l.Allow(context.Background(), "A")
	_, _ = l.Allow(context.Background(), "B")
	now = now.Add(2 * time.Minute)
	if got := l.SweepStale(); got != 2 {
		t.Errorf("SweepStale removed %d, want 2", got)
	}
}

func TestInMemoryLimiter_EmptyKeyRejected(t *testing.T) {
	l := NewInMemoryLimiter(1, time.Minute)
	// Allow's contract is (bool, error); we expect a non-nil
	// error on empty key (any non-nil is fine; not a public
	// sentinel contract).
	if _, err := l.Allow(context.Background(), ""); err == nil {
		t.Error("Allow empty key returned nil err; want non-nil")
	}
	if err := l.Reset(context.Background(), ""); err == nil {
		t.Error("Reset empty key accepted")
	}
}

func TestInMemoryLimiter_DefaultsApplied(t *testing.T) {
	l := NewInMemoryLimiter(0, 0)
	if l.limit != DefaultRateLimit {
		t.Errorf("limit = %d, want %d", l.limit, DefaultRateLimit)
	}
	if l.window != DefaultRateLimitWindow {
		t.Errorf("window = %v, want %v", l.window, DefaultRateLimitWindow)
	}
}

// ---------------------------------------------------------------------------
// Concurrency invariant
// ---------------------------------------------------------------------------

func TestInMemoryLimiter_ConcurrentAccessIsSafe(t *testing.T) {
	// Run many goroutines hitting the same key. The total
	// accepted count must equal the configured limit exactly —
	// any over-allowance is a race condition.
	l := NewInMemoryLimiter(50, 1*time.Minute)
	var wg sync.WaitGroup
	var accepted int32
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.Allow(context.Background(), "shared"); ok {
				atomic.AddInt32(&accepted, 1)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&accepted); got != 50 {
		t.Errorf("accepted = %d, want exactly 50 (race in limiter)", got)
	}
}

// ---------------------------------------------------------------------------
// Redis limiter
// ---------------------------------------------------------------------------

func TestRedisLimiter_AllowsUpToLimit(t *testing.T) {
	c, _ := newTestRedis(t)
	l, err := NewRedisLimiter(c, "opene2ee:ratelimit", 3, 1*time.Minute)
	if err != nil {
		t.Fatalf("NewRedisLimiter: %v", err)
	}
	for i := 1; i <= 3; i++ {
		if ok, err := l.Allow(context.Background(), "1.2.3.4"); err != nil || !ok {
			t.Errorf("Allow #%d ok=%v err=%v", i, ok, err)
		}
	}
}

func TestRedisLimiter_RejectsPastLimit(t *testing.T) {
	c, _ := newTestRedis(t)
	l, _ := NewRedisLimiter(c, "opene2ee:ratelimit", 2, 1*time.Minute)
	for i := 0; i < 2; i++ {
		_, _ = l.Allow(context.Background(), "1.2.3.4")
	}
	ok, _ := l.Allow(context.Background(), "1.2.3.4")
	if ok {
		t.Error("past-limit call accepted")
	}
}

func TestRedisLimiter_Reset(t *testing.T) {
	c, _ := newTestRedis(t)
	l, _ := NewRedisLimiter(c, "opene2ee:ratelimit", 1, 1*time.Minute)
	_, _ = l.Allow(context.Background(), "X")
	if err := l.Reset(context.Background(), "X"); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if ok, _ := l.Allow(context.Background(), "X"); !ok {
		t.Error("post-Reset call rejected")
	}
}

func TestRedisLimiter_FailsOpenOnBackendError(t *testing.T) {
	// Use a closed client to force a backend error. Allow must
	// return (true, nil) — fail-open.
	c, mr := newTestRedis(t)
	mr.Close() // close the in-process Redis to make INCR fail
	l, _ := NewRedisLimiter(c, "opene2ee:ratelimit", 1, 1*time.Minute)
	ok, err := l.Allow(context.Background(), "X")
	if err != nil {
		t.Errorf("Allow err = %v, want nil (fail-open)", err)
	}
	if !ok {
		t.Error("Allow returned false; want true (fail-open on backend error)")
	}
	if l.LastError() == nil {
		t.Error("LastError should be set after backend error")
	}
}

func TestNewRedisLimiter_Validation(t *testing.T) {
	if _, err := NewRedisLimiter(nil, "p", 1, time.Minute); err == nil {
		t.Error("nil client accepted")
	}
	c, _ := newTestRedis(t)
	if _, err := NewRedisLimiter(c, "", 1, time.Minute); err == nil {
		t.Error("empty prefix accepted")
	}
	// Non-positive limit/window fall back to defaults.
	l, err := NewRedisLimiter(c, "p", 0, 0)
	if err != nil {
		t.Fatalf("defaults applied: %v", err)
	}
	if l.limit != DefaultRateLimit {
		t.Errorf("limit = %d, want %d", l.limit, DefaultRateLimit)
	}
	if l.window != DefaultRateLimitWindow {
		t.Errorf("window = %v, want %v", l.window, DefaultRateLimitWindow)
	}
}

// ---------------------------------------------------------------------------
// Default selector
// ---------------------------------------------------------------------------

func TestNewRateLimiter_PicksInMemoryWhenNoRedis(t *testing.T) {
	l, err := NewRateLimiter(nil, "ignored")
	if err != nil {
		t.Fatalf("NewRateLimiter: %v", err)
	}
	if _, ok := l.(*InMemoryLimiter); !ok {
		t.Errorf("got %T, want *InMemoryLimiter", l)
	}
}

func TestNewRateLimiter_PicksRedisWhenProvided(t *testing.T) {
	c, _ := newTestRedis(t)
	l, err := NewRateLimiter(c, "opene2ee:ratelimit")
	if err != nil {
		t.Fatalf("NewRateLimiter: %v", err)
	}
	if _, ok := l.(*RedisLimiter); !ok {
		t.Errorf("got %T, want *RedisLimiter", l)
	}
}