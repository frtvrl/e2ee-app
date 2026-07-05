// service_test.go — unit + integration tests for the Service layer.
//
// "Unit" tests use NoopCache + real adapters. "Integration" tests
// use RedisCache against miniredis (in-process Redis). Either way,
// no live Redis is required.
package operator

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

func TestNewService_RejectsNilCache(t *testing.T) {
	if _, err := NewService(nil, nil, nil); err == nil {
		t.Error("nil cache accepted")
	}
}

func TestNewService_DefaultsTTLTo24h(t *testing.T) {
	s, err := NewService(NoopCache{}, nil, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if s.ttl != DefaultCacheTTL {
		t.Errorf("ttl = %v, want %v", s.ttl, DefaultCacheTTL)
	}
}

func TestNewService_AppliesOptions(t *testing.T) {
	custom := 7 * time.Minute
	frozen := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	now := func() time.Time { return frozen }
	s, err := NewService(NoopCache{}, nil, nil, WithTTL(custom), withNow(now))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if s.ttl != custom {
		t.Errorf("ttl = %v, want %v", s.ttl, custom)
	}
	if s.now == nil {
		t.Fatal("now not set")
	}
	if !s.now().Equal(frozen) {
		t.Errorf("custom now() not honoured: got %v, want %v", s.now(), frozen)
	}
}

func TestNewService_InvalidTTLFallsBackToDefault(t *testing.T) {
	s, _ := NewService(NoopCache{}, nil, nil, WithTTL(0))
	if s.ttl != DefaultCacheTTL {
		t.Errorf("ttl = %v, want %v (default)", s.ttl, DefaultCacheTTL)
	}
	s, _ = NewService(NoopCache{}, nil, nil, WithTTL(-1*time.Second))
	if s.ttl != DefaultCacheTTL {
		t.Errorf("ttl = %v, want %v (default for negative)", s.ttl, DefaultCacheTTL)
	}
}

// ---------------------------------------------------------------------------
// LookupByPhone — happy paths
// ---------------------------------------------------------------------------

func TestService_LookupByPhone_KnownPrefixUsesAdapter(t *testing.T) {
	s, _ := NewService(NoopCache{}, []OperatorLookup{NewMNPTRAdapter()}, nil)
	info, err := s.LookupByPhone(context.Background(), "+905320000000")
	if err != nil {
		t.Fatalf("LookupByPhone: %v", err)
	}
	if info.Operator != "turkcell" {
		t.Errorf("Operator = %q, want turkcell", info.Operator)
	}
	if info.Source != SourceTRMNPAPI {
		t.Errorf("Source = %q, want %q", info.Source, SourceTRMNPAPI)
	}
	if info.QueryType != QueryPhoneE164 {
		t.Errorf("QueryType = %q, want %q", info.QueryType, QueryPhoneE164)
	}
}

func TestService_LookupByPhone_UnknownPrefixReturnsFallback(t *testing.T) {
	s, _ := NewService(NoopCache{}, []OperatorLookup{NewMNPTRAdapter()}, nil)
	// +90560 is in no prefix — adapter returns ErrUnknownOperator.
	info, err := s.LookupByPhone(context.Background(), "+905600000000")
	if err != nil {
		t.Fatalf("LookupByPhone: %v", err)
	}
	if info.Operator != "unknown" {
		t.Errorf("Operator = %q, want unknown", info.Operator)
	}
	if info.Source != SourceFallbackUnknown {
		t.Errorf("Source = %q, want %q", info.Source, SourceFallbackUnknown)
	}
	if info.Confidence != 0 {
		t.Errorf("Confidence = %v, want 0 for unknown", info.Confidence)
	}
}

func TestService_LookupByPhone_InvalidInput(t *testing.T) {
	s, _ := NewService(NoopCache{}, []OperatorLookup{NewMNPTRAdapter()}, nil)
	_, err := s.LookupByPhone(context.Background(), "not a phone")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestService_LookupByPhone_NoAdapters(t *testing.T) {
	// No adapters at all → every phone query is fallback_unknown.
	s, _ := NewService(NoopCache{}, nil, nil)
	info, err := s.LookupByPhone(context.Background(), "+905320000000")
	if err != nil {
		t.Fatalf("err = %v, want nil (fallback is not an error)", err)
	}
	if info.Operator != "unknown" {
		t.Errorf("Operator = %q, want unknown", info.Operator)
	}
}

// ---------------------------------------------------------------------------
// LookupByIP — happy paths
// ---------------------------------------------------------------------------

func TestService_LookupByIP_KnownRangeUsesAdapter(t *testing.T) {
	s, _ := NewService(NoopCache{}, nil, []OperatorLookup{NewIPReverseAdapter()})
	info, err := s.LookupByIP(context.Background(), "78.180.1.1")
	if err != nil {
		t.Fatalf("LookupByIP: %v", err)
	}
	if info.Operator != "turkcell" {
		t.Errorf("Operator = %q, want turkcell", info.Operator)
	}
	if info.Source != SourceASNDB {
		t.Errorf("Source = %q, want %q", info.Source, SourceASNDB)
	}
	if info.QueryType != QueryIPv4 {
		t.Errorf("QueryType = %q, want %q", info.QueryType, QueryIPv4)
	}
}

func TestService_LookupByIP_UnknownReturnsFallback(t *testing.T) {
	s, _ := NewService(NoopCache{}, nil, []OperatorLookup{NewIPReverseAdapter()})
	info, err := s.LookupByIP(context.Background(), "1.2.3.4")
	if err != nil {
		t.Fatalf("err = %v, want nil (fallback is not an error)", err)
	}
	if info.Operator != "unknown" {
		t.Errorf("Operator = %q, want unknown", info.Operator)
	}
	if info.QueryType != QueryIPv4 {
		t.Errorf("QueryType = %q, want %q", info.QueryType, QueryIPv4)
	}
}

func TestService_LookupByIP_IPv6ReturnsFallback(t *testing.T) {
	s, _ := NewService(NoopCache{}, nil, []OperatorLookup{NewIPReverseAdapter()})
	info, err := s.LookupByIP(context.Background(), "2001:db8::1")
	if err != nil {
		t.Fatalf("err = %v, want nil (fallback is not an error)", err)
	}
	if info.Operator != "unknown" {
		t.Errorf("Operator = %q, want unknown", info.Operator)
	}
	if info.QueryType != QueryIPv6 {
		t.Errorf("QueryType = %q, want %q", info.QueryType, QueryIPv6)
	}
}

func TestService_LookupByIP_InvalidInput(t *testing.T) {
	s, _ := NewService(NoopCache{}, nil, []OperatorLookup{NewIPReverseAdapter()})
	_, err := s.LookupByIP(context.Background(), "not an ip")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

// ---------------------------------------------------------------------------
// Cache behaviour
// ---------------------------------------------------------------------------

func TestService_CachesPositiveResults(t *testing.T) {
	// Use a counter adapter to verify the second call hits the cache
	// (and thus does NOT invoke the adapter). NoopCache intentionally
	// never stores, so we use RedisCache against miniredis here to
	// exercise the full Set/Get path.
	c, _ := newTestRedis(t)
	cache, err := NewRedisCache(c, nil, "opene2ee:operator")
	if err != nil {
		t.Fatalf("NewRedisCache: %v", err)
	}
	counter := &countingAdapter{
		phone: &OperatorInfo{Operator: "turkcell", Source: SourceTRMNPAPI,
			Confidence: 0.95, QueryType: QueryPhoneE164, Timestamp: time.Now().UTC()},
	}
	s, _ := NewService(cache, []OperatorLookup{counter}, nil)

	if _, err := s.LookupByPhone(context.Background(), "+905320000000"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := s.LookupByPhone(context.Background(), "+905320000000"); err != nil {
		t.Fatalf("second: %v", err)
	}
	if counter.phoneCalls != 1 {
		t.Errorf("phoneCalls = %d, want 1 (second call should hit cache)", counter.phoneCalls)
	}
}

func TestService_CachesNegativeResults(t *testing.T) {
	// A "we don't know" answer must also be cached, so a flood of
	// unknown-prefix queries doesn't hammer the adapter (F12).
	c, _ := newTestRedis(t)
	cache, _ := NewRedisCache(c, nil, "opene2ee:operator")
	counter := &countingAdapter{phoneErr: ErrUnknownOperator}
	s, _ := NewService(cache, []OperatorLookup{counter}, nil)

	for i := 0; i < 5; i++ {
		if _, err := s.LookupByPhone(context.Background(), "+905600000000"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if counter.phoneCalls != 1 {
		t.Errorf("phoneCalls = %d, want 1 (unknown result must be cached)", counter.phoneCalls)
	}
}

func TestService_DistinctKeysForPhoneAndIP(t *testing.T) {
	// Same value, different kinds → different cache keys → both
	// queries still go to the adapter.
	counter := &countingAdapter{
		phone: &OperatorInfo{Operator: "x", Source: SourceTRMNPAPI, Confidence: 1, QueryType: QueryPhoneE164},
		ip:    &OperatorInfo{Operator: "y", Source: SourceASNDB, Confidence: 1, QueryType: QueryIPv4},
	}
	s, _ := NewService(NoopCache{}, []OperatorLookup{counter}, []OperatorLookup{counter})

	if _, err := s.LookupByPhone(context.Background(), "+905320000000"); err != nil {
		t.Fatalf("phone: %v", err)
	}
	if _, err := s.LookupByIP(context.Background(), "1.2.3.4"); err != nil {
		t.Fatalf("ip: %v", err)
	}
	if counter.phoneCalls != 1 || counter.ipCalls != 1 {
		t.Errorf("calls phone=%d ip=%d, want 1+1", counter.phoneCalls, counter.ipCalls)
	}
}

// ---------------------------------------------------------------------------
// Integration: Service + RedisCache (miniredis)
// ---------------------------------------------------------------------------

func TestService_IntegrationWithRedisCache(t *testing.T) {
	c, _ := newTestRedis(t)
	cache, err := NewRedisCache(c, []byte("salt"), "opene2ee:operator")
	if err != nil {
		t.Fatalf("NewRedisCache: %v", err)
	}
	s, err := NewService(cache, []OperatorLookup{NewMNPTRAdapter()}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx := context.Background()

	// First call: miss → adapter.
	a, err := s.LookupByPhone(ctx, "+905320000000")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	// Second call: cache hit → same payload.
	b, err := s.LookupByPhone(ctx, "+905320000000")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if a.Operator != b.Operator {
		t.Errorf("cache returned different operator: %q vs %q", a.Operator, b.Operator)
	}

	// Delete the cache key — next call must hit the adapter again.
	key := cache.BuildKey(KeyKindPhone, "+905320000000")
	if err := cache.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// A fresh counter adapter would prove the call hit it; here we
	// just verify the result is still correct.
	if _, err := s.LookupByPhone(ctx, "+905320000000"); err != nil {
		t.Fatalf("after delete: %v", err)
	}
}

func TestService_IntegrationNegativeCache(t *testing.T) {
	c, _ := newTestRedis(t)
	cache, _ := NewRedisCache(c, nil, "opene2ee:operator")
	s, _ := NewService(cache, []OperatorLookup{NewMNPTRAdapter()}, nil)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		info, err := s.LookupByPhone(ctx, "+905600000000")
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if info.Operator != "unknown" {
			t.Errorf("call %d operator = %q, want unknown", i, info.Operator)
		}
	}
	// After 3 calls there should be exactly one Redis key written.
	keys, err := c.Keys(ctx, "opene2ee:operator:*").Result()
	if err != nil {
		t.Fatalf("Keys: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("Redis key count = %d, want 1 (negative cache should reuse the entry)", len(keys))
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

// countingAdapter is a test double that returns canned values and
// counts how many times each method was invoked. Used to assert
// that the Service is or isn't calling the adapter (cache hit /
// miss semantics).
type countingAdapter struct {
	phone     *OperatorInfo
	phoneErr  error
	ip        *OperatorInfo
	ipErr     error
	phoneCalls int
	ipCalls    int
}

func (c *countingAdapter) LookupByPhone(_ context.Context, _ string) (*OperatorInfo, error) {
	c.phoneCalls++
	return c.phone, c.phoneErr
}
func (c *countingAdapter) LookupByIP(_ context.Context, _ string) (*OperatorInfo, error) {
	c.ipCalls++
	return c.ip, c.ipErr
}
