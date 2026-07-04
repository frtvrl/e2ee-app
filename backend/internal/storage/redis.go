package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore implements ReceiverPool against Redis.
//
// Active Pool model (ADR-0004 §1):
//   - Receivers register themselves with a TTL (15 min default).
//   - Senders atomically pop one receiver via SPOP / LPOP.
//   - Crashed clients self-clean when TTL expires; no sweeper needed.
type RedisStore struct {
	client *redis.Client
}

// Compile-time interface check.
var _ ReceiverPool = (*RedisStore)(nil)

// activePoolKey is the Redis set/list used to track waiting receivers.
// We use a list so we can do atomic LPOP / RPUSH for FIFO semantics — a
// round-robin pool is fairer than a random selection from a set.
const activePoolKey = "opene2ee:pool:active"

// NewRedisStore dials Redis. password may be empty for local/dev.
func NewRedisStore(ctx context.Context, addr, password string) (*RedisStore, error) {
	c := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})
	// Ping early so we fail fast at startup, not at first request.
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("storage: redis ping %s: %w", addr, err)
	}
	return &RedisStore{client: c}, nil
}

// Client exposes the underlying go-redis client for health checks (e.g. /healthz).
func (s *RedisStore) Client() *redis.Client { return s.client }

// Close releases the connection pool.
func (s *RedisStore) Close() error {
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

// Add registers a device hash as an active receiver with the given TTL.
//
// Semantics: RPUSH (tail insert) so the list behaves FIFO. The TTL is
// reflected by the caller calling Add again on heartbeat — if Add is never
// called again, the entry sticks around until the list key is evicted
// (we set the *list* TTL to the same value on each Add so that an idle
// pool naturally drains).
func (s *RedisStore) Add(ctx context.Context, deviceHash string, ttl time.Duration) error {
	if deviceHash == "" {
		return fmt.Errorf("storage: redis Add: empty deviceHash")
	}
	if ttl <= 0 {
		ttl = DefaultPoolTTL
	}
	// LPUSH instead of RPUSH: receivers always append, matching pops. Using
	// LPUSH + RPOP would be FIFO-from-oldest, while LPUSH + LPOP is LIFO.
	// We want receivers who waited longest to be matched first → use RPUSH
	// (push to tail) + LPOP (pop from head) for FIFO.
	if err := s.client.RPush(ctx, activePoolKey, deviceHash).Err(); err != nil {
		return fmt.Errorf("storage: redis RPush: %w", err)
	}
	// Refresh the list-level TTL each time a receiver joins. This keeps the
	// whole pool alive while traffic is flowing and expires it during idle.
	if err := s.client.Expire(ctx, activePoolKey, ttl).Err(); err != nil {
		return fmt.Errorf("storage: redis Expire: %w", err)
	}
	return nil
}

// PopMatching atomically removes and returns the next waiting receiver.
// Returns ErrNotFound if the pool is empty.
func (s *RedisStore) PopMatching(ctx context.Context) (string, error) {
	res, err := s.client.LPop(ctx, activePoolKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("storage: redis LPop: %w", err)
	}
	return res, nil
}

// Count returns the number of currently waiting receivers.
func (s *RedisStore) Count(ctx context.Context) (int64, error) {
	n, err := s.client.LLen(ctx, activePoolKey).Result()
	if err != nil {
		return 0, fmt.Errorf("storage: redis LLEN: %w", err)
	}
	return n, nil
}
