// cache.go — Cache layer for the Operator Tespit Servisi.
//
// Per HANDOFF §4 PR-3: "Redis cache TTL=24h, device_id_hash veya ip
// key ile". Here the "device_id_hash" pattern is generalised: every
// cache key is `prefix + ":" + sha256(salt || value)`, so the cache
// dump never contains a plaintext phone number or IP address even
// if Redis is breached (RISKS §F12).
//
// INTERFACE
//
// The Cache interface is intentionally narrow — four methods, no
// business logic. Two implementations ship:
//
//   - NoopCache: returns nothing, never errors. Used in tests and
//     when the operator wants the Service to talk to upstream
//     adapters on every request.
//   - RedisCache: thin wrapper over go-redis that JSON-encodes the
//     OperatorInfo and lets Redis handle TTL.
//
// KEY FORMAT
//
//	prefix + ":" + kind + ":" + hex( SHA-256( salt || value ) )[:16]
//
// where:
//
//	prefix    = cache key namespace, e.g. "opene2ee:operator"
//	kind      = "phone" | "ip"   (separation so the same hash can't
//	             collide between the two namespaces)
//	salt      = optional, server-side, []byte. If non-empty it's
//	             prepended to the value before hashing. An empty
//	             salt still hashes (just without a secret).
//	value     = the E.164 phone number or the IP string.
//
// The hex truncation to 16 bytes (128 bits) matches auth.TruncateBytes
// — collision probability is ~2^-64 with the same (salt, kind, value)
// tuple, which is well within "cache key" territory.
//
// CONCURRENCY
//
// RedisCache is safe for concurrent use (go-redis client is). The
// hex-encoding / sha256 helpers use a per-call hash.Hash, so they
// don't share state between goroutines. No mutexes needed in the
// adapter itself.
package operator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// KeyKind is the discriminator inside a cache key — separates phone
// and IP values so a hash collision between the two namespaces
// cannot accidentally return the wrong kind of OperatorInfo.
type KeyKind string

const (
	KeyKindPhone KeyKind = "phone"
	KeyKindIP    KeyKind = "ip"
)

// Cache is the contract the Service layer depends on.
//
// Semantics:
//   - Set writes info under key with the given TTL. A zero or
//     negative TTL is treated as "use the default" (5m per
//     Sprint 3 PR-23; 24h is preserved as LongCacheTTL).
//   - Get returns (info, true, nil) on a hit, (nil, false, nil) on
//     a miss, or (nil, false, err) on a Redis / I/O error. When
//     RefreshOnHit is enabled (default), Get also PEXPIRES the
//     key back to the original TTL on every hit so a hot key
//     stays hot — see RefreshOnHit on RedisCache.
//   - Delete removes the key. Missing keys are NOT an error —
//     Delete is idempotent.
//   - Close releases any underlying resources. Idempotent.
//   - BuildKey derives the canonical cache key for a (kind, value)
//     pair. The Service uses this to keep its Get/Set/Delete calls
//     consistent with itself.
type Cache interface {
	Set(ctx context.Context, key string, info *OperatorInfo, ttl time.Duration) error
	Get(ctx context.Context, key string) (*OperatorInfo, bool, error)
	Delete(ctx context.Context, key string) error
	Close() error
	BuildKey(kind KeyKind, value string) string
}

// NoopCache is a Cache that does nothing. Useful in tests and as
// the default for development builds where Redis is not available.
//
// The zero value is ready to use — no constructor needed.
type NoopCache struct{}

// Compile-time interface check.
var _ Cache = NoopCache{}

// Set is a no-op.
func (NoopCache) Set(_ context.Context, _ string, _ *OperatorInfo, _ time.Duration) error {
	return nil
}

// Get always reports a miss.
func (NoopCache) Get(_ context.Context, _ string) (*OperatorInfo, bool, error) {
	return nil, false, nil
}

// Delete is a no-op.
func (NoopCache) Delete(_ context.Context, _ string) error { return nil }

// Close is a no-op.
func (NoopCache) Close() error { return nil }

// RedisCache is the production Cache implementation backed by go-redis.
//
// It owns the *redis.Client; callers that also need the client for
// health checks should hold a separate reference.
//
// RefreshOnHit (Sprint 3 PR-23) controls the sliding-window refresh
// behaviour: when true (default), every Get that returns a hit
// re-issues a PEXPIRE so the entry's TTL is reset to the original
// value it was Set with. This keeps hot keys hot — an identity
// that's queried every 30s stays in cache indefinitely — while
// cold entries still evict on the original TTL.
//
// The original TTL is NOT remembered per-key; it is remembered on
// the OperatorInfo struct (info.CacheTTLSecs, set by Service.Set)
// or, when that is zero, defaults to DefaultCacheTTL.
type RedisCache struct {
	client       *redis.Client
	keySalt      []byte
	prefix       string
	RefreshOnHit bool
}

// NewRedisCache wraps an already-connected *redis.Client. The caller
// is responsible for verifying connectivity (Ping) before this —
// NewRedisStore in the storage package does the same pattern.
//
// Parameters:
//   - client:  the go-redis client (must not be nil)
//   - keySalt: optional, may be empty. Prepended to the value
//              before SHA-256 to make cache keys unguessable /
//              un-reversible without the salt.
//   - prefix:  the cache key namespace, e.g. "opene2ee:operator".
//              Must not be empty.
func NewRedisCache(client *redis.Client, keySalt []byte, prefix string) (*RedisCache, error) {
	if client == nil {
		return nil, errors.New("operator: NewRedisCache: nil redis client")
	}
	if prefix == "" {
		return nil, errors.New("operator: NewRedisCache: empty prefix")
	}
	return &RedisCache{
		client:       client,
		keySalt:      append([]byte(nil), keySalt...), // copy so caller can mutate
		prefix:       prefix,
		RefreshOnHit: true,
	}, nil
}

// Compile-time interface check.
var _ Cache = (*RedisCache)(nil)

// BuildKey is the public cache-key builder. It is exposed on the
// Cache type so the Service can use the same key derivation for
// Get / Set / Delete without having to know the salt.
//
// Format:
//
//	prefix + ":" + kind + ":" + hex( SHA-256( salt || value ) )[:16]
//
// With an empty salt, the value is still hashed (the "salted" name
// is a misnomer for the empty-salt case but the function name is
// kept stable for clarity in the call sites).
func (c *RedisCache) BuildKey(kind KeyKind, value string) string {
	h := sha256.New()
	if len(c.keySalt) > 0 {
		_, _ = h.Write(c.keySalt)
	}
	_, _ = h.Write([]byte(value))
	sum := h.Sum(nil)
	return c.prefix + ":" + string(kind) + ":" + hex.EncodeToString(sum[:cacheKeyHashBytes])
}

// BuildKey is the no-op equivalent. It still returns a deterministic
// string (so the Service can log keys consistently) but the value
// is just prefixed, never sent anywhere.
func (NoopCache) BuildKey(kind KeyKind, value string) string {
	// Use sha256 so the returned string is always well-formed and
	// bounded-length — matches the production format minus the
	// namespace prefix, so a NoopCache and a RedisCache with the
	// same value produce stable key strings in test logs.
	h := sha256.New()
	_, _ = h.Write([]byte(value))
	return "noop:" + string(kind) + ":" + hex.EncodeToString(h.Sum(nil)[:cacheKeyHashBytes])
}

// cacheKeyHashBytes is how many bytes of the SHA-256 we keep when
// building a cache key. 16 bytes = 128 bits = 32 hex chars.
const cacheKeyHashBytes = 16

// Set writes info as JSON under key with the given TTL. Zero /
// negative TTL falls back to DefaultCacheTTL (5m as of Sprint 3).
// The info's CacheTTLSecs is also stamped so a later Get with
// RefreshOnHit=true can re-issue PEXPIRE with the SAME TTL value
// the entry was originally written with (rather than the global
// default), which is what "TTL refresh" means in practice.
func (c *RedisCache) Set(ctx context.Context, key string, info *OperatorInfo, ttl time.Duration) error {
	if key == "" {
		return errors.New("operator: RedisCache.Set: empty key")
	}
	if info == nil {
		return errors.New("operator: RedisCache.Set: nil info")
	}
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	// Stamp the TTL onto the OperatorInfo so Get/Refresh can use it.
	info.CacheTTLSecs = int(ttl / time.Second)
	payload, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("operator: RedisCache.Set: marshal: %w", err)
	}
	if err := c.client.Set(ctx, key, payload, ttl).Err(); err != nil {
		return fmt.Errorf("operator: RedisCache.Set: %w", err)
	}
	return nil
}

// Get returns (info, true, nil) on a hit, (nil, false, nil) on a
// miss, (nil, false, err) on any other error. A JSON unmarshal
// failure is treated as a miss + log, NOT an error — the cache
// may hold a value from an older schema version and we don't want
// one bad row to take down the lookup service.
//
// When RefreshOnHit is true (default) AND the hit carries a
// non-zero CacheTTLSecs, Get re-issues PEXPIRE so the entry's TTL
// is reset to its original value. This is the Sprint 3 "TTL
// refresh" behaviour — hot keys stay hot while cold keys still
// evict. The PEXPIRE error is intentionally swallowed: a hit is
// a hit, regardless of whether the refresh succeeded.
func (c *RedisCache) Get(ctx context.Context, key string) (*OperatorInfo, bool, error) {
	if key == "" {
		return nil, false, errors.New("operator: RedisCache.Get: empty key")
	}
	raw, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("operator: RedisCache.Get: %w", err)
	}
	var info OperatorInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		// Treat bad payload as miss — see doc comment.
		return nil, false, nil
	}
	// TTL refresh on hit. Best-effort: a PEXPIRE failure must not
	// downgrade the response to an error.
	if c.RefreshOnHit && info.CacheTTLSecs > 0 {
		_ = c.client.PExpire(ctx, key,
			time.Duration(info.CacheTTLSecs)*time.Second).Err()
	}
	return &info, true, nil
}

// Delete removes the key. A missing key is not an error (Del
// returns 0 in that case in go-redis; we don't surface that
// distinction).
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	if key == "" {
		return errors.New("operator: RedisCache.Delete: empty key")
	}
	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("operator: RedisCache.Delete: %w", err)
	}
	return nil
}

// Close closes the underlying redis client. Idempotent: go-redis
// returns an error on a double-close ("redis: client is closed"),
// so we guard with a sync.Once-style nil check to make the Cache
// interface contract hold.
func (c *RedisCache) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	if err := c.client.Close(); err != nil {
		// go-redis returns "redis: client is closed" on a second
		// call. We treat that as success for the Cache interface
		// (Close is documented idempotent).
		if err.Error() == "redis: client is closed" {
			c.client = nil
			return nil
		}
		return err
	}
	c.client = nil
	return nil
}
