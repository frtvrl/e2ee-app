// service.go — orchestration layer for the Operator Tespit Servisi.
//
// Service composes the cache and the adapters and presents the
// whole thing to the REST layer (PR-7) as a single OperatorLookup.
// It also decides what to do when no adapter knows the answer:
// instead of bubbling ErrUnknownOperator, it returns a
// well-formed "unknown" OperatorInfo so the REST handler can
// always respond 200 with a JSON body that matches
// shared/schemas/operator-lookup.schema.json.
//
// CACHE STRATEGY
//
//   - Cache key: built via the Cache's BuildKey (e.g. RedisCache's
//     salted SHA-256). The Service treats the Cache as opaque —
//     any Cache implementation works.
//   - Read path: cache.Get → on hit, return immediately. The
//     cached OperatorInfo is returned as-is (its Timestamp is
//     the original query time, which the REST layer can expose
//     if useful).
//   - Write path: on a fresh adapter success, cache.Set with the
//     configured TTL (default 24h per HANDOFF §4 PR-3). On a
//     negative result (fallback_unknown) we ALSO cache it — a
//     "we don't know" answer is still a stable answer for the
//     TTL window. Caching negatives prevents DoS-by-unknown-
//     prefix (RISKS §F12).
//
// ERRORS
//
//   - ErrInvalidInput → propagates (caller bug, must surface).
//   - Any non-ErrUnknownOperator error from an adapter → propagates
//     (caller should log + 5xx; we do not mask infrastructure
//     failures as "unknown").
//   - ErrUnknownOperator from ALL adapters → swallowed; Service
//     returns a synthetic OperatorInfo with source=fallback_unknown.
package operator

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Service is the cache-fronted OperatorLookup. The zero value is
// NOT usable — call NewService.
type Service struct {
	cache   Cache
	phones  []OperatorLookup
	ips     []OperatorLookup
	ttl     time.Duration
	now     func() time.Time
}

// NewService wires the cache and the adapter chains.
//
// Parameters:
//   - cache:   the Cache layer. Required (use NoopCache{} to disable).
//   - phones:  ordered chain of OperatorLookup implementations that
//     can resolve phones. The first non-ErrUnknownOperator result
//     wins. May be empty (Service will return fallback_unknown for
//     every phone query).
//   - ips:     same, for IP lookups.
//   - opts:    variadic configuration; see ServiceOption.
func NewService(cache Cache, phones, ips []OperatorLookup, opts ...ServiceOption) (*Service, error) {
	if cache == nil {
		return nil, errors.New("operator: NewService: nil cache")
	}
	s := &Service{
		cache:  cache,
		phones: phones,
		ips:    ips,
		ttl:    DefaultCacheTTL,
		now:    time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.ttl <= 0 {
		s.ttl = DefaultCacheTTL
	}
	return s, nil
}

// ServiceOption configures a Service at construction time.
type ServiceOption func(*Service)

// WithTTL overrides the cache TTL. Values <= 0 keep the default (24h).
func WithTTL(d time.Duration) ServiceOption {
	return func(s *Service) { s.ttl = d }
}

// withNow is exported for tests only — overrides the wall-clock
// source so timestamp assertions are deterministic.
func withNow(now func() time.Time) ServiceOption {
	return func(s *Service) { s.now = now }
}

// Compile-time interface check.
var _ OperatorLookup = (*Service)(nil)

// LookupByPhone is the cache-fronted phone resolver.
func (s *Service) LookupByPhone(ctx context.Context, e164 string) (*OperatorInfo, error) {
	if !looksLikeE164(e164) {
		return nil, fmt.Errorf("phone %q: %w", e164, ErrInvalidInput)
	}
	key := s.cache.BuildKey(KeyKindPhone, e164)

	// (1) Cache hit.
	if info, hit, err := s.cache.Get(ctx, key); err != nil {
		// Cache error is non-fatal — log & continue to adapters.
		// (We don't have a logger here; the REST layer can wire one.)
		_ = info
		_ = err
	} else if hit {
		return info, nil
	}

	// (2) Adapter chain.
	info, err := s.resolveChain(ctx, s.phones, QueryPhoneE164, e164)
	if err != nil {
		if errors.Is(err, ErrUnknownOperator) {
			// Negative cache: cache the "unknown" answer.
			neg := s.makeUnknown(QueryPhoneE164, e164)
			_ = s.cache.Set(ctx, key, neg, s.ttl)
			return neg, nil
		}
		// Real error (input validation already passed, so it's an
		// adapter infrastructure issue) — propagate.
		return nil, err
	}

	// (3) Cache the positive result.
	_ = s.cache.Set(ctx, key, info, s.ttl)
	return info, nil
}

// LookupByIP is the cache-fronted IP resolver.
func (s *Service) LookupByIP(ctx context.Context, ip string) (*OperatorInfo, error) {
	if !looksLikeIP(ip) {
		return nil, fmt.Errorf("ip %q: %w", ip, ErrInvalidInput)
	}
	key := s.cache.BuildKey(KeyKindIP, ip)

	if info, hit, err := s.cache.Get(ctx, key); err != nil {
		_ = info
		_ = err
	} else if hit {
		return info, nil
	}

	info, err := s.resolveChain(ctx, s.ips, QueryIPv4, ip)
	if err != nil {
		if errors.Is(err, ErrUnknownOperator) {
			neg := s.makeUnknown(ipQueryType(ip), ip)
			_ = s.cache.Set(ctx, key, neg, s.ttl)
			return neg, nil
		}
		return nil, err
	}

	_ = s.cache.Set(ctx, key, info, s.ttl)
	return info, nil
}

// resolveChain walks an adapter chain and returns the first
// non-ErrUnknownOperator result. ErrUnknownOperator from every
// adapter is converted to a single error at the end.
func (s *Service) resolveChain(
	ctx context.Context,
	chain []OperatorLookup,
	qt QueryType,
	value string,
) (*OperatorInfo, error) {
	// Pick the method to call based on qt.
	lookupFn := func(a OperatorLookup) (*OperatorInfo, error) {
		switch qt {
		case QueryPhoneE164, QueryPhoneNational:
			return a.LookupByPhone(ctx, value)
		case QueryIPv4, QueryIPv6:
			return a.LookupByIP(ctx, value)
		default:
			return nil, fmt.Errorf("unsupported query type %q", qt)
		}
	}
	for _, a := range chain {
		info, err := lookupFn(a)
		if err == nil {
			return info, nil
		}
		if errors.Is(err, ErrUnknownOperator) {
			continue
		}
		// Non-"unknown" error: propagate immediately. We don't
		// want a downstream adapter to mask a real failure from
		// an earlier one.
		return nil, err
	}
	return nil, ErrUnknownOperator
}

// makeUnknown synthesises a "we don't know" OperatorInfo. This is
// what the REST layer will marshal as a 200 response when no
// adapter can resolve the query.
func (s *Service) makeUnknown(qt QueryType, value string) *OperatorInfo {
	return &OperatorInfo{
		QueryType:  qt,
		QueryValue: value,
		Operator:   "unknown",
		Source:     SourceFallbackUnknown,
		Confidence: 0.0,
		Timestamp:  s.now().UTC(),
	}
}

// ipQueryType picks the right QueryType for a syntactically-valid IP.
func ipQueryType(ip string) QueryType {
	if v, ok := ipVersion(ip); ok {
		if v == "v6" {
			return QueryIPv6
		}
	}
	return QueryIPv4
}
