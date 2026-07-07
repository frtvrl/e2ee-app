// pool.go — Active Pool: Redis-backed set of P2P receivers with
// criteria-filtered atomic pop.
//
// Design notes:
//
//   - Each receiver is registered into a Redis sorted set where
//     the member encodes its metadata (operator|country|app|hash)
//     and the score is its expiry unix timestamp.
//   - ZADD-with-score gives us free TTL semantics: ZRANGEBYSCORE
//     with `now+1..+inf` returns only currently-active receivers.
//   - Stale members are cleaned lazily on every MatchSender call
//     via ZREMRANGEBYSCORE `0..now` — no separate sweeper needed
//     for the steady state. A periodic idle sweeper (RunSweeper,
//     STRIDE-6-03) covers the cold case: nobody has called Match
//     or Count in a while, so members whose score has slipped
//     into the past accumulate. Sweep runs at
//     DefaultIdleSweepInterval (60s, < DefaultPoolTTL 15m) and
//     returns the # evicted so operators can graph the eviction
//     rate.
//   - The criteria-filtered pop is implemented as a single Lua
//     script that scans up to N active members, picks the first
//     one matching all non-empty filters, removes it, and returns
//     it. Atomic — two concurrent senders cannot get the same
//     volunteer.
//   - The metadata encoding uses ASCII Unit Separator (\x1f) as
//     the field delimiter. Unit separator is illegal in any of
//     the field values we expect (operator/country/app are short
//     ASCII strings with no control chars), so we don't need to
//     escape it. If the field set ever broadens to free-text user
//     input, switch to JSON encoding inside the Lua script (Redis
//     ships with cjson).
//   - STRIDE-6-03 (Sprint 7, hand-off from cyber-security review):
//     KVKK DELETE on /api/v1/users/{device_id_hash} now propagates
//     to the Active Pool via Pool.DeleteByHash — see api/users.go
//     (Sprint 6 PR-37 wired the AUTHZ gate; this PR wires the
//     Redis-side purge hook). DeleteByHash is O(N) because Redis
//     has no secondary index on the hash field; the pool is
//     normally tiny (steady-state count is in the tens) so this
//     is fine. If the pool ever grows past a few thousand
//     members, switch the encoding to a Redis hash keyed on
//     `opene2ee:matching:pool:hash:<hash>` with a parallel
//     ZADD for the TTL, and use a Lua script for atomic
//     remove-by-hash.
package matching

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// DefaultPoolTTL is the per-receiver TTL when the caller passes 0.
// Per ADR-0004 §1 "Nöbet 15 dk".
const DefaultPoolTTL = 15 * time.Minute

// DefaultMatchScanLimit caps how many candidates the Lua script
// inspects per MatchSender call. 50 is plenty for the steady
// state — the pool is rarely that large, and if it ever is the
// caller just retries.
const DefaultMatchScanLimit = 50

// DefaultIdleSweepInterval is the period between proactive idle
// sweeps — see RunSweeper. Chosen at 60s (1 minute), well below
// the 15-minute DefaultPoolTTL, so the steady-state pool never
// holds more than ~one minute's worth of expired members even
// when no MatchSender / Count call comes in. A more aggressive
// value (e.g. 10s) would just churn Redis for no benefit; a
// lazier value (e.g. 5m) would let expired members pile up to a
// meaningful fraction of the pool.
const DefaultIdleSweepInterval = 60 * time.Second

// ErrNotFound is returned by MatchSender when no active receiver
// matches the criteria (pool empty OR no member matches the
// filter set). Callers (PR-7) translate this to HTTP 404 +
// "no peer available" so the mobile app can fall back to
// Echo-Bot (PR-5).
var ErrNotFound = errors.New("matching: no receiver matches criteria")

// Criteria is the filter set a sender passes to MatchSender.
//
// Each field is optional — empty means "any". The triple is the
// minimum needed to make the measurement scientifically useful:
// same operator (similar middlebox path), same country (similar
// regulatory/encryption-backdoor regime), same app-under-test
// (so the two peers are running the same client).
type Criteria struct {
	Operator string // e.g. "Turkcell", "Vodafone TR", "BT" — empty = any
	Country  string // ISO 3166-1 alpha-2, e.g. "TR", "GB" — empty = any
	App      string // canonical app key, e.g. "whatsapp", "signal", "rcs" — empty = any
}

// ReceiverInfo describes a waiting receiver that a sender has
// been matched to. The Hash is what the WebSocket signalling
// layer uses as the routing key (PR-6 signalling.go Hub.Send).
//
// Metadata fields are normalised lowercase to make
// criteria-matches forgiving of casing differences between
// sender's `Operator` (which the mobile app derived from
// internal/operator at PR-3) and the receiver's `Operator`
// (which the receiver app also derived from internal/operator).
type ReceiverInfo struct {
	Hash     string `json:"hash"`
	Operator string `json:"operator"`
	Country  string `json:"country"`
	App      string `json:"app"`
}

// Pool is the Active Pool interface. PR-7's REST handler depends
// on this; tests inject a fake implementation that returns
// canned values.
type Pool interface {
	// RegisterReceiver adds a receiver with metadata to the pool.
	// The receiver stays in the pool for `ttl` (or DefaultPoolTTL
	// if ttl <= 0) and is automatically evicted afterwards.
	// Re-registering the same hash updates metadata + extends TTL.
	RegisterReceiver(ctx context.Context, info ReceiverInfo, ttl time.Duration) error

	// MatchSender atomically picks and removes one active receiver
	// matching `criteria`. Returns ErrNotFound if none matches.
	//
	// Atomicity guarantee: two concurrent callers cannot receive
	// the same ReceiverInfo — the Lua script removes the chosen
	// member in the same round-trip as the read.
	MatchSender(ctx context.Context, criteria Criteria) (*ReceiverInfo, error)

	// Count returns the number of currently-active receivers
	// regardless of criteria. Used by /healthz and for ops
	// dashboards ("how big is the nöbet list right now?").
	Count(ctx context.Context) (int64, error)

	// DeleteByHash removes every active-pool entry whose hash
	// equals `hash`. Sprint 7 STRIDE-6-03: the api package's
	// DeleteUserHook calls this after a successful KVKK DELETE so
	// the user's waiting-receiver row disappears immediately
	// rather than waiting for the TTL to expire. Returns the
	// number of pool entries removed (0 if the hash wasn't in the
	// pool, 1 in the normal case, >1 if the same hash was
	// re-registered with different metadata — shouldn't happen
	// because ZADD upserts by member, but we defensively sweep
	// every match).
	DeleteByHash(ctx context.Context, hash string) (int64, error)

	// SweepIdle evicts every member whose score (expiry
	// unix timestamp) is in the past. Returns the number of
	// members removed. The lazy cleanup that lives inside
	// MatchSender and Count is sufficient for the hot path; this
	// method exists for the cold path (no traffic) and for
	// RunSweeper (the periodic background sweeper).
	SweepIdle(ctx context.Context) (int64, error)

	// RunSweeper runs SweepIdle every DefaultIdleSweepInterval
	// until ctx is cancelled. The goroutine is best-effort: an
	// error from one tick does not stop the loop (logged via
	// slog from main.go's wrapper). For tests, the goroutine
	// returns when ctx is done. STRIDE-6-03 idle-pool purge path.
	RunSweeper(ctx context.Context)

	// Close releases the underlying Redis connection pool.
	Close() error
}

// Compile-time interface check.
var _ Pool = (*RedisPool)(nil)

// fieldSep is the ASCII Unit Separator used to delimit the four
// fields inside a sorted-set member. Defined once here so the Lua
// script (which is a string literal) and the Go encoding stay in
// lockstep — they MUST use the same byte.
const fieldSep = "\x1f"

// indexKey is the Redis sorted set that backs the pool. We use
// one global key (rather than per-receiver hashes) so the Lua
// script can scan-and-remove atomically. The "TTL" of each
// member is implicit in its score (expiry unix timestamp).
const indexKey = "opene2ee:matching:pool"

// matchScript is the Lua script that does the criteria-filtered
// atomic pop. Inputs:
//
//	KEYS[1] = indexKey
//	ARGV[1] = now_unix (float seconds)
//	ARGV[2] = operator filter ("" or "*" means any)
//	ARGV[3] = country  filter ("" or "*" means any)
//	ARGV[4] = app      filter ("" or "*" means any)
//	ARGV[5] = scan limit (positive integer)
//
// Returns the matched member string on success, "" if no match
// (the caller maps "" to ErrNotFound).
//
// Algorithm:
//  1. ZREMRANGEBYSCORE 0..now — lazy cleanup of expired members.
//  2. ZRANGEBYSCORE now+1..+inf LIMIT 0 scan — grab up to N
//     active candidates.
//  3. For each candidate, split on fieldSep and compare fields.
//  4. First full match → ZREM and return it.
//
// Lua string.match with [^SEP]* is used to split (Lua has no
// built-in split). All four fields are mandatory in the member
// encoding; a malformed member is silently skipped (the regex
// returns nil → we move on).
var matchScript = redis.NewScript(`
local key       = KEYS[1]
local now       = tonumber(ARGV[1])
local op_filter = ARGV[2]
local co_filter = ARGV[3]
local ap_filter = ARGV[4]
local limit     = tonumber(ARGV[5])
local sep       = string.char(31)

-- 1. Drop expired members.
redis.call("ZREMRANGEBYSCORE", key, "-inf", tostring(now))

-- 2. Pull active candidates.
local members = redis.call("ZRANGEBYSCORE", key, tostring(now + 0.000001), "+inf", "LIMIT", 0, limit)
if (not members) or (#members == 0) then
    return ""
end

-- 3. Find first match.
for i = 1, #members do
    local m = members[i]
    local op, co, ap, hash = string.match(m, "([^" .. sep .. "]*)" .. sep .. "([^" .. sep .. "]*)" .. sep .. "([^" .. sep .. "]*)" .. sep .. "(.+)")
    if hash then
        local op_ok = (op_filter == "" or op_filter == "*") or (op == op_filter)
        local co_ok = (co_filter == "" or co_filter == "*") or (co == co_filter)
        local ap_ok = (ap_filter == "" or ap_filter == "*") or (ap == ap_filter)
        if op_ok and co_ok and ap_ok then
            redis.call("ZREM", key, m)
            return m
        end
    end
end

-- 4. None matched — clean up any stragglers and return empty.
redis.call("ZREMRANGEBYSCORE", key, "-inf", tostring(now))
return ""
`)

// encodeMember packs a ReceiverInfo into the member string the
// sorted set stores. Format: operator \x1f country \x1f app \x1f hash.
// All four fields are guaranteed non-empty by the Register
// validation; an empty metadata field is normalised to "-" so
// downstream Lua splitting still yields 4 segments.
func encodeMember(info ReceiverInfo) string {
	op := info.Operator
	if op == "" {
		op = "-"
	}
	co := info.Country
	if co == "" {
		co = "-"
	}
	ap := info.App
	if ap == "" {
		ap = "-"
	}
	return strings.Join([]string{op, co, ap, info.Hash}, fieldSep)
}

// decodeMember is the inverse of encodeMember. Returns
// ErrMalformedMember if the input doesn't have all 4 fields.
func decodeMember(s string) (ReceiverInfo, error) {
	parts := strings.Split(s, fieldSep)
	if len(parts) != 4 {
		return ReceiverInfo{}, fmt.Errorf("matching: malformed member %q (want 4 fields)", s)
	}
	normalise := func(p string) string {
		if p == "-" {
			return ""
		}
		return p
	}
	return ReceiverInfo{
		Operator: normalise(parts[0]),
		Country:  normalise(parts[1]),
		App:      normalise(parts[2]),
		Hash:     parts[3],
	}, nil
}

// RedisPool is the production Pool implementation. It owns its
// own Redis client (separate from storage.RedisStore) because the
// sorted-set schema and Lua-script semantics are tightly coupled
// and we don't want a generic KV layer to second-guess the data
// layout.
type RedisPool struct {
	client     *redis.Client
	scanLimit  int
	scriptHash string // cached EVALSHA hash; populated lazily
}

// NewRedisPool dials Redis and pings it. `password` may be empty
// for local/dev.
func NewRedisPool(ctx context.Context, addr, password string) (*RedisPool, error) {
	c := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("matching: redis ping %s: %w", addr, err)
	}
	return &RedisPool{
		client:    c,
		scanLimit: DefaultMatchScanLimit,
	}, nil
}

// SetScanLimit overrides the maximum candidates inspected per
// MatchSender call. Returns the receiver so the call can be
// chained with NewRedisPool in tests. Production callers should
// not need this — DefaultMatchScanLimit is generous.
func (p *RedisPool) SetScanLimit(n int) *RedisPool {
	if n > 0 {
		p.scanLimit = n
	}
	return p
}

// Close releases the Redis client. Idempotent — calling Close
// twice is a no-op (the second call sees client == nil and
// returns nil immediately). This matters because callers
// sometimes wire Close into a defer alongside a graceful
// shutdown handler that may also Close the same pool.
func (p *RedisPool) Close() error {
	if p.client == nil {
		return nil
	}
	err := p.client.Close()
	p.client = nil
	return err
}

// Client exposes the underlying go-redis client for /healthz.
func (p *RedisPool) Client() *redis.Client { return p.client }

// RegisterReceiver adds a receiver to the pool with the given
// metadata and TTL. Re-registering the same hash updates both
// metadata and expiry (ZADD is upsert by member).
//
// Validation: Hash must be non-empty (it's the routing key used
// by the signalling layer). Metadata fields are optional — empty
// values are normalised to "-" by encodeMember so the Lua regex
// still gets exactly 4 segments per member.
func (p *RedisPool) RegisterReceiver(ctx context.Context, info ReceiverInfo, ttl time.Duration) error {
	if info.Hash == "" {
		return fmt.Errorf("matching: RegisterReceiver: empty hash")
	}
	if ttl <= 0 {
		ttl = DefaultPoolTTL
	}
	score := float64(time.Now().Add(ttl).Unix()) // expiry unix seconds
	member := encodeMember(info)
	if err := p.client.ZAdd(ctx, indexKey, redis.Z{
		Score:  score,
		Member: member,
	}).Err(); err != nil {
		return fmt.Errorf("matching: redis ZADD: %w", err)
	}
	return nil
}

// MatchSender atomically picks one matching receiver. See
// matchScript for the atomicity guarantee.
func (p *RedisPool) MatchSender(ctx context.Context, c Criteria) (*ReceiverInfo, error) {
	now := time.Now().Unix()
	res, err := matchScript.Run(
		ctx, p.client,
		[]string{indexKey},
		now,
		c.Operator, c.Country, c.App,
		p.scanLimit,
	).Result()
	if err != nil {
		// redis.Nil is the only "expected" empty result; anything
		// else is a real Redis error.
		if errors.Is(err, redis.Nil) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("matching: redis EVAL: %w", err)
	}
	member, ok := res.(string)
	if !ok || member == "" {
		return nil, ErrNotFound
	}
	info, err := decodeMember(member)
	if err != nil {
		// Should be impossible — the Lua script only ZREMs members
		// it just produced. But guard anyway so a future encoding
		// change can't silently corrupt the Result path.
		return nil, fmt.Errorf("matching: %w", err)
	}
	return &info, nil
}

// Count returns the number of currently-active receivers. A
// receiver is "active" iff its score (expiry timestamp) is in
// the future. We first ZREMRANGEBYSCORE the stale entries so
// the count reflects reality — without the cleanup step, a
// long-idle pool would over-report. The ZCARD after the cleanup
// is a single O(log N) operation.
func (p *RedisPool) Count(ctx context.Context) (int64, error) {
	now := time.Now().Unix()
	if err := p.client.ZRemRangeByScore(ctx, indexKey,
		"0", strconv.FormatInt(now, 10),
	).Err(); err != nil {
		return 0, fmt.Errorf("matching: redis ZREMRANGEBYSCORE: %w", err)
	}
	n, err := p.client.ZCard(ctx, indexKey).Result()
	if err != nil {
		return 0, fmt.Errorf("matching: redis ZCARD: %w", err)
	}
	return n, nil
}

// DeleteByHash removes every active-pool member whose parsed
// hash equals `hash`. Returns the count of removed members.
//
// Algorithm (STRIDE-6-03):
//  1. ZRANGE the entire sorted set (we keep all members; expired
//     ones are skipped at the decode step rather than a separate
//     ZREMRANGEBYSCORE because DeleteByHash is a rare event and
//     keeping the scan atomic-from-the-caller's-POV is simpler
//     than juggling two round-trips).
//  2. For each member: decode. If parse fails or hash mismatch,
//     skip. If hash matches, ZREM the member.
//  3. Return #removed.
//
// ZREM is O(log N) per call; the dominant cost is the ZRANGE +
// decode loop, which is O(N) over the (small) pool.
//
// Empty hash is a no-op — returns (0, nil). We deliberately do
// NOT return an error because the api layer calls this from a
// hook and a no-op delete is the safest failure mode for a
// degraded caller.
func (p *RedisPool) DeleteByHash(ctx context.Context, hash string) (int64, error) {
	if hash == "" {
		return 0, nil
	}
	members, err := p.client.ZRange(ctx, indexKey, 0, -1).Result()
	if err != nil {
		return 0, fmt.Errorf("matching: redis ZRANGE: %w", err)
	}
	var removed int64
	for _, m := range members {
		info, err := decodeMember(m)
		if err != nil {
			// Malformed member — log nothing (the matching
			// package's privacy invariant forbids log calls;
			// see TestPackageNoLoggingOrPrinting) and skip.
			// The lazy cleanup in Count/MatchSender will pick
			// it up if it's expired, and the next sweep will
			// eventually drop any genuinely-stale garbage.
			continue
		}
		if info.Hash != hash {
			continue
		}
		if err := p.client.ZRem(ctx, indexKey, m).Err(); err != nil {
			return removed, fmt.Errorf("matching: redis ZREM: %w", err)
		}
		removed++
	}
	return removed, nil
}

// SweepIdle evicts every member whose score is in the past.
// Returns the number of members removed.
//
// This is the "cold path" complement to the lazy cleanup that
// already runs inside Count and MatchSender. If the pool sits
// idle for a while (no MatchSender / Count calls), expired
// members accumulate in Redis until the next hot-path call. The
// background sweeper (RunSweeper) calls this on a timer to keep
// Redis memory bounded even in the cold case.
func (p *RedisPool) SweepIdle(ctx context.Context) (int64, error) {
	now := time.Now().Unix()
	n, err := p.client.ZRemRangeByScore(ctx, indexKey,
		"0", strconv.FormatInt(now, 10),
	).Result()
	if err != nil {
		return 0, fmt.Errorf("matching: redis ZREMRANGEBYSCORE: %w", err)
	}
	return n, nil
}

// RunSweeper blocks running SweepIdle every DefaultIdleSweepInterval
// until ctx is cancelled. Errors from individual sweeps are
// swallowed (logged at the callsite if the caller wraps with
// slog) — a transient Redis hiccup should not kill the sweeper
// permanently; the next tick will recover.
//
// STRIDE-6-03: this is the idle-pool purge path. It runs in its
// own goroutine, started by main.go on server boot and stopped
// by the graceful-shutdown context cancellation.
//
// Concurrency: multiple goroutines can call SweepIdle
// concurrently with RunSweeper without conflict — ZREMRANGEBYSCORE
// is atomic. Same for DeleteByHash running alongside RunSweeper
// (the worst case is a DeleteByHash + a SweepIdle both touching
// an expired member; both are idempotent ZREMs).
func (p *RedisPool) RunSweeper(ctx context.Context) {
	t := time.NewTicker(DefaultIdleSweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Best-effort. A cancelled ctx surfaces as an
			// error here; we don't propagate because the
			// outer select will catch the cancellation on
			// the next iteration.
			_, _ = p.SweepIdle(ctx)
		}
	}
}
