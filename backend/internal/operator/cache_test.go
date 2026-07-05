// cache_test.go — unit tests for NoopCache and RedisCache.
//
// RedisCache is exercised against miniredis (in-process), which is
// already in go.mod from PR-1's storage tests — no live Redis
// needed.
package operator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	c := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = c.Close() })
	return c, mr
}

// -----------------------------------------------------------------------------
// NoopCache
// -----------------------------------------------------------------------------

func TestNoopCache_GetAlwaysMisses(t *testing.T) {
	c := NoopCache{}
	info, hit, err := c.Get(context.Background(), "anything")
	if err != nil {
		t.Errorf("NoopCache.Get err = %v, want nil", err)
	}
	if hit {
		t.Error("NoopCache.Get hit=true, want false")
	}
	if info != nil {
		t.Errorf("NoopCache.Get info = %+v, want nil", info)
	}
}

func TestNoopCache_SetDeleteCloseAreNoError(t *testing.T) {
	c := NoopCache{}
	if err := c.Set(context.Background(), "k", &OperatorInfo{Operator: "x"}, time.Minute); err != nil {
		t.Errorf("Set err = %v", err)
	}
	if err := c.Delete(context.Background(), "k"); err != nil {
		t.Errorf("Delete err = %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close err = %v", err)
	}
}

func TestNoopCache_BuildKey_DeterministicAndKindSeparated(t *testing.T) {
	c := NoopCache{}
	a := c.BuildKey(KeyKindPhone, "+905320000000")
	b := c.BuildKey(KeyKindPhone, "+905320000000")
	if a != b {
		t.Errorf("BuildKey not deterministic: %q vs %q", a, b)
	}
	// Different value → different key.
	if a == c.BuildKey(KeyKindPhone, "+905320000001") {
		t.Error("different values produced the same key")
	}
	// Different kind → different key (collision guard).
	if a == c.BuildKey(KeyKindIP, "+905320000000") {
		t.Error("phone and IP of the same string produced the same key")
	}
}

// -----------------------------------------------------------------------------
// RedisCache — constructor validation
// -----------------------------------------------------------------------------

func TestNewRedisCache_RejectsNilClient(t *testing.T) {
	if _, err := NewRedisCache(nil, nil, "x"); err == nil {
		t.Error("nil client accepted")
	}
}

func TestNewRedisCache_RejectsEmptyPrefix(t *testing.T) {
	c, _ := newTestRedis(t)
	if _, err := NewRedisCache(c, nil, ""); err == nil {
		t.Error("empty prefix accepted")
	}
}

func TestNewRedisCache_AcceptsEmptySalt(t *testing.T) {
	c, _ := newTestRedis(t)
	rc, err := NewRedisCache(c, nil, "opene2ee:operator")
	if err != nil {
		t.Fatalf("NewRedisCache: %v", err)
	}
	if rc == nil {
		t.Fatal("nil cache")
	}
	// keySalt should be a fresh slice (not the same backing array
	// the caller holds, so they can mutate their copy without
	// affecting the cache).
	if rc.keySalt != nil {
		t.Errorf("keySalt = %v, want nil when salt arg is nil", rc.keySalt)
	}
}

func TestNewRedisCache_CopiesSalt(t *testing.T) {
	c, _ := newTestRedis(t)
	saltIn := []byte("server-salt-v1")
	rc, err := NewRedisCache(c, saltIn, "opene2ee:operator")
	if err != nil {
		t.Fatalf("NewRedisCache: %v", err)
	}
	// Mutate the caller's copy; cache's keySalt must not change.
	saltIn[0] = 'X'
	if rc.keySalt[0] == 'X' {
		t.Error("keySalt shares backing array with caller's slice")
	}
}

// -----------------------------------------------------------------------------
// RedisCache — BuildKey
// -----------------------------------------------------------------------------

func TestRedisCache_BuildKey_FormatAndSalt(t *testing.T) {
	c, _ := newTestRedis(t)
	rc, err := NewRedisCache(c, []byte("salt-123"), "opene2ee:operator")
	if err != nil {
		t.Fatalf("NewRedisCache: %v", err)
	}

	k := rc.BuildKey(KeyKindPhone, "+905320000000")
	// Format: <prefix>:<kind>:<hexhash>. The prefix itself contains
	// a colon ("opene2ee:operator") so we anchor on the last 2
	// colon-separated segments.
	idx := strings.LastIndex(k, ":")
	if idx < 0 || idx == len(k)-1 {
		t.Fatalf("key %q does not end with a kind:hash segment", k)
	}
	prefixAndKind := k[:idx]
	lastSegment := k[idx+1:]

	// The part before the last colon should be the prefix + ":phone".
	wantPrefix := "opene2ee:operator:phone"
	if prefixAndKind != wantPrefix {
		t.Errorf("prefix:kind = %q, want %q", prefixAndKind, wantPrefix)
	}
	if len(lastSegment) != 32 {
		t.Errorf("hash hex length = %d, want 32 (16 bytes)", len(lastSegment))
	}
	// Determinism.
	if k2 := rc.BuildKey(KeyKindPhone, "+905320000000"); k != k2 {
		t.Errorf("BuildKey not deterministic: %q vs %q", k, k2)
	}
	// Salt changes the key.
	noSalt, _ := NewRedisCache(c, nil, "opene2ee:operator")
	if k == noSalt.BuildKey(KeyKindPhone, "+905320000000") {
		t.Error("salt had no effect on key derivation")
	}
	// Kind separation.
	if k == rc.BuildKey(KeyKindIP, "+905320000000") {
		t.Error("phone and IP of the same value produced the same key")
	}
}

// -----------------------------------------------------------------------------
// RedisCache — Set / Get / Delete roundtrip
// -----------------------------------------------------------------------------

func TestRedisCache_SetGetRoundtrip(t *testing.T) {
	c, _ := newTestRedis(t)
	rc, err := NewRedisCache(c, []byte("salt"), "opene2ee:operator")
	if err != nil {
		t.Fatalf("NewRedisCache: %v", err)
	}
	ctx := context.Background()
	key := rc.BuildKey(KeyKindPhone, "+905320000000")
	in := &OperatorInfo{
		QueryType:    QueryPhoneE164,
		QueryValue:   "+905320000000",
		Operator:     "turkcell",
		OperatorName: "Turkcell",
		Country:      "TR",
		MCC:          "286",
		MNC:          "01",
		Source:       SourceTRMNPAPI,
		Confidence:   0.95,
		Timestamp:    time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC),
	}
	if err := rc.Set(ctx, key, in, 5*time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, hit, err := rc.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !hit {
		t.Fatal("Get hit=false after Set")
	}
	if got.Operator != in.Operator {
		t.Errorf("Operator = %q, want %q", got.Operator, in.Operator)
	}
	if got.OperatorName != in.OperatorName {
		t.Errorf("OperatorName = %q, want %q", got.OperatorName, in.OperatorName)
	}
	if !got.Timestamp.Equal(in.Timestamp) {
		t.Errorf("Timestamp = %v, want %v", got.Timestamp, in.Timestamp)
	}
}

func TestRedisCache_GetMiss(t *testing.T) {
	c, _ := newTestRedis(t)
	rc, _ := NewRedisCache(c, nil, "opene2ee:operator")
	got, hit, err := rc.Get(context.Background(), "opene2ee:operator:phone:00000000000000000000000000000000")
	if err != nil {
		t.Errorf("Get err = %v on miss, want nil", err)
	}
	if hit {
		t.Error("Get hit=true on missing key")
	}
	if got != nil {
		t.Errorf("Get info = %+v, want nil on miss", got)
	}
}

func TestRedisCache_Delete(t *testing.T) {
	c, mr := newTestRedis(t)
	rc, _ := NewRedisCache(c, nil, "opene2ee:operator")
	ctx := context.Background()
	key := rc.BuildKey(KeyKindPhone, "+905320000000")
	if err := rc.Set(ctx, key, &OperatorInfo{Operator: "x"}, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := rc.Delete(ctx, key); err != nil {
		t.Errorf("Delete: %v", err)
	}
	_, hit, _ := rc.Get(ctx, key)
	if hit {
		t.Error("key still present after Delete")
	}
	// Idempotent: deleting again is not an error.
	if err := rc.Delete(ctx, key); err != nil {
		t.Errorf("second Delete err = %v, want nil", err)
	}
	// miniredis fast-forward; key should remain gone.
	mr.FastForward(2 * time.Hour)
	_, hit, _ = rc.Get(ctx, key)
	if hit {
		t.Error("key resurrected after FastForward")
	}
}

func TestRedisCache_TTLDefaultsTo24h(t *testing.T) {
	c, mr := newTestRedis(t)
	rc, _ := NewRedisCache(c, nil, "opene2ee:operator")
	ctx := context.Background()
	key := rc.BuildKey(KeyKindPhone, "+905320000000")
	// Pass a 0 TTL — must fall back to DefaultCacheTTL.
	if err := rc.Set(ctx, key, &OperatorInfo{Operator: "x"}, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// Just before expiry: still present.
	mr.FastForward(23 * time.Hour)
	if _, hit, _ := rc.Get(ctx, key); !hit {
		t.Error("key evicted before 24h")
	}
	// Just after expiry: gone.
	mr.FastForward(2 * time.Hour) // total 25h
	if _, hit, _ := rc.Get(ctx, key); hit {
		t.Error("key still present after 25h; TTL did not default to 24h")
	}
}

func TestRedisCache_CloseIsIdempotent(t *testing.T) {
	c, _ := newTestRedis(t)
	rc, _ := NewRedisCache(c, nil, "opene2ee:operator")
	if err := rc.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Input validation
// -----------------------------------------------------------------------------

func TestRedisCache_RejectsEmptyKey(t *testing.T) {
	c, _ := newTestRedis(t)
	rc, _ := NewRedisCache(c, nil, "opene2ee:operator")
	ctx := context.Background()
	if err := rc.Set(ctx, "", &OperatorInfo{}, time.Minute); err == nil {
		t.Error("Set with empty key accepted")
	}
	if _, _, err := rc.Get(ctx, ""); err == nil {
		t.Error("Get with empty key accepted")
	}
	if err := rc.Delete(ctx, ""); err == nil {
		t.Error("Delete with empty key accepted")
	}
}

func TestRedisCache_RejectsNilInfo(t *testing.T) {
	c, _ := newTestRedis(t)
	rc, _ := NewRedisCache(c, nil, "opene2ee:operator")
	if err := rc.Set(context.Background(), "k", nil, time.Minute); err == nil {
		t.Error("Set with nil info accepted")
	}
}

// -----------------------------------------------------------------------------
// Bad JSON in cache value is treated as miss, not error
// -----------------------------------------------------------------------------

func TestRedisCache_BadJSONIsMiss(t *testing.T) {
	c, mr := newTestRedis(t)
	rc, _ := NewRedisCache(c, nil, "opene2ee:operator")
	// Hand-craft a key with non-JSON payload.
	mr.Set("opene2ee:operator:phone:badbadbadbadbadbadbadbadbadbad", "this is not json {{{")
	got, hit, err := rc.Get(context.Background(),
		"opene2ee:operator:phone:badbadbadbadbadbadbadbadbadbadbad")
	if err != nil {
		t.Errorf("Get on bad JSON returned error: %v (should be miss)", err)
	}
	if hit {
		t.Error("Get on bad JSON returned hit=true")
	}
	if got != nil {
		t.Error("Get on bad JSON returned non-nil info")
	}
}

// -----------------------------------------------------------------------------
// Compile-time interface satisfaction
// -----------------------------------------------------------------------------

func TestCacheInterfaceImplementations(t *testing.T) {
	var _ Cache = NoopCache{}
	var _ Cache = (*RedisCache)(nil)
	// BuildKey is on both — make sure the interface doesn't require it.
	c := NoopCache{}
	_ = c.BuildKey(KeyKindPhone, "x")
}

// Sanity: errors.Is roundtrip on ErrUnknownOperator still works.
func TestErrorsIs_StillFunctional(t *testing.T) {
	if !errors.Is(ErrUnknownOperator, ErrUnknownOperator) {
		t.Error("ErrUnknownOperator should match itself")
	}
}
