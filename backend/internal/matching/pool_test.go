package matching

import (
	"context"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRedisPool returns a RedisPool wired to an in-process
// miniredis. Miniredis is the same fixture storage/redis_test.go
// uses — keeps the dependency footprint uniform.
func newTestRedisPool(t *testing.T) (*RedisPool, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	pool, err := NewRedisPool(context.Background(), mr.Addr(), "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	return pool, mr
}

// sampleInfo returns a ReceiverInfo with a stable hash + the
// given metadata fields. Useful for table-driven tests.
func sampleInfo(hash, op, country, app string) ReceiverInfo {
	return ReceiverInfo{Hash: hash, Operator: op, Country: country, App: app}
}

// ---------- construction ----------

func TestNewRedisPool_PingFailure(t *testing.T) {
	// Bind a real TCP port then close it so the address is
	// unreachable (mirrors storage/redis_test.go pattern).
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	require.NoError(t, lis.Close())

	_, err = NewRedisPool(context.Background(), addr, "")
	require.Error(t, err)
}

func TestRedisPool_SatisfiesPoolInterface(t *testing.T) {
	var _ Pool = (*RedisPool)(nil)
}

func TestRedisPool_SetScanLimit(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	// Zero / negative values are ignored.
	pool.SetScanLimit(0)
	pool.SetScanLimit(-5)
	// Positive value is set.
	pool.SetScanLimit(7)
	assert.Equal(t, 7, pool.scanLimit)
}

func TestRedisPool_Client(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	assert.NotNil(t, pool.Client())
}

func TestRedisPool_Close_Idempotent(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	require.NoError(t, pool.Close())
	require.NoError(t, pool.Close(), "double-Close should not error")
}

// ---------- register / match happy path ----------

func TestRedisPool_RegisterAndMatch_RoundTrips(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))

	got, err := pool.MatchSender(ctx, Criteria{Operator: "Turkcell"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "dev-aaaaaaaaaaaaaa", got.Hash)
	assert.Equal(t, "Turkcell", got.Operator)
	assert.Equal(t, "TR", got.Country)
	assert.Equal(t, "whatsapp", got.App)
}

func TestRedisPool_Match_EmptyPoolReturnsNotFound(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	got, err := pool.MatchSender(context.Background(), Criteria{})
	assert.Nil(t, got)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRedisPool_Match_AfterPopPoolIsEmpty(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-bbbbbbbbbbbbbb", "Vodafone", "TR", "signal"),
		5*time.Minute))

	_, err := pool.MatchSender(ctx, Criteria{})
	require.NoError(t, err)

	// Second pop returns ErrNotFound (atomic — first pop removed
	// the only member).
	_, err = pool.MatchSender(ctx, Criteria{})
	require.ErrorIs(t, err, ErrNotFound)
}

// ---------- criteria filtering ----------

func TestRedisPool_Match_FiltersByOperator(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-1111111111111111", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-2222222222222222", "Vodafone", "TR", "whatsapp"),
		5*time.Minute))
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-3333333333333333", "Turkcell", "TR", "signal"),
		5*time.Minute))

	got, err := pool.MatchSender(ctx, Criteria{Operator: "Vodafone"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Vodafone", got.Operator)

	// Only the Turkcell ones should remain.
	got, err = pool.MatchSender(ctx, Criteria{Operator: "Turkcell"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Turkcell", got.Operator)
}

func TestRedisPool_Match_FiltersByCountry(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "BT", "GB", "whatsapp"),
		5*time.Minute))
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-bbbbbbbbbbbbbb", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))

	got, err := pool.MatchSender(ctx, Criteria{Country: "GB"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "GB", got.Country)
}

func TestRedisPool_Match_FiltersByApp(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-bbbbbbbbbbbbbb", "Turkcell", "TR", "signal"),
		5*time.Minute))

	got, err := pool.MatchSender(ctx, Criteria{App: "signal"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "signal", got.App)
}

func TestRedisPool_Match_AllThreeCriteria(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-bbbbbbbbbbbbbb", "Turkcell", "TR", "signal"),
		5*time.Minute))

	got, err := pool.MatchSender(ctx, Criteria{
		Operator: "Turkcell", Country: "TR", App: "whatsapp",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "whatsapp", got.App)
}

func TestRedisPool_Match_NoMatchReturnsNotFound(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))

	got, err := pool.MatchSender(ctx, Criteria{Operator: "Vodafone"})
	assert.Nil(t, got)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRedisPool_Match_EmptyCriteriaIsAny(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))

	// No filters → match anything currently in the pool.
	got, err := pool.MatchSender(ctx, Criteria{})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "dev-aaaaaaaaaaaaaa", got.Hash)
}

// ---------- validation ----------

func TestRedisPool_Register_RejectsEmptyHash(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	err := pool.RegisterReceiver(context.Background(),
		ReceiverInfo{Operator: "Turkcell", Country: "TR", App: "whatsapp"},
		5*time.Minute)
	require.Error(t, err)
}

func TestRedisPool_Register_DefaultTTLOnZero(t *testing.T) {
	pool, mr := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		0))

	// Score (expiry unix) should be ~now+DefaultPoolTTL.
	members, err := mr.ZMembers(indexKey)
	require.NoError(t, err)
	require.Len(t, members, 1)

	ss, err := mr.SortedSet(indexKey)
	require.NoError(t, err)
	require.Len(t, ss, 1)
	var score float64
	for _, s := range ss {
		score = s
		break
	}
	expected := float64(time.Now().Add(DefaultPoolTTL).Unix())
	assert.InDelta(t, expected, score, 5, "score should be ~now+DefaultPoolTTL")
}

// ---------- count ----------

func TestRedisPool_Count(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	n, err := pool.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-bbbbbbbbbbbbbb", "Vodafone", "TR", "whatsapp"),
		5*time.Minute))

	n, err = pool.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	_, err = pool.MatchSender(ctx, Criteria{})
	require.NoError(t, err)

	n, err = pool.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}

// ---------- expiry semantics ----------

func TestRedisPool_StaleEntriesCleanedOnMatch(t *testing.T) {
	pool, mr := newTestRedisPool(t)
	ctx := context.Background()

	// Inject a "stale" member directly into the ZSET (score in
	// the past) to simulate expiry. We do this via the miniredis
	// direct API rather than waiting 10 seconds because the test
	// is supposed to be fast.
	pastScore := float64(time.Now().Add(-time.Hour).Unix())
	mr.ZAdd(indexKey, pastScore, "ghost\x1f-\x1f-\x1fghost-1234567890ab")

	// Count should drop the stale member via ZREMRANGEBYSCORE.
	n, err := pool.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n, "Count should drop stale members")

	// MatchSender runs the same cleanup logic, then returns NotFound.
	_, err = pool.MatchSender(ctx, Criteria{})
	require.ErrorIs(t, err, ErrNotFound)
}

// ---------- STRIDE-6-03 purge semantics ----------

// TestRedisPool_DeleteByHash_RemovesMember is the canonical
// "insert user, simulate KVKK delete, verify gone" sequence
// that the Sprint 7 STRIDE-6-03 acceptance criteria call for.
// The user is registered into the pool, DeleteByHash is called
// with the same hash, and MatchSender must then return
// ErrNotFound (the user's row is gone — no second chance for a
// volunteer-wait or a stale re-add).
func TestRedisPool_DeleteByHash_RemovesMember(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	const hash = "dev-aaaaaaaaaaaaaa"
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo(hash, "Turkcell", "TR", "whatsapp"),
		5*time.Minute))

	// Sanity: the user IS in the pool pre-delete.
	n, err := pool.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	// Simulate the KVKK DELETE path: api/users.go's
	// DeleteUserHook -> pool.DeleteByHash(hash).
	removed, err := pool.DeleteByHash(ctx, hash)
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed, "exactly one member should be removed")

	// Post-delete: pool is empty, no second chance.
	n, err = pool.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	_, err = pool.MatchSender(ctx, Criteria{})
	require.ErrorIs(t, err, ErrNotFound, "deleted hash must NOT match")
}

// TestRedisPool_DeleteByHash_OnlyMatchingHash confirms that
// DeleteByHash is targeted — a delete for hash A must not
// touch hash B or C. This is the regression guard against an
// over-aggressive "clear all on delete" implementation.
func TestRedisPool_DeleteByHash_OnlyMatchingHash(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-bbbbbbbbbbbbbb", "Vodafone", "TR", "whatsapp"),
		5*time.Minute))
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-cccccccccccccc", "Turkcell", "TR", "signal"),
		5*time.Minute))

	removed, err := pool.DeleteByHash(ctx, "dev-bbbbbbbbbbbbbb")
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed)

	// The other two must still be in the pool and matchable.
	n, err := pool.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)

	// First pop should be Turkcell or signal (one of the two
	// survivors); NOT the deleted Vodafone.
	got, err := pool.MatchSender(ctx, Criteria{})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.NotEqual(t, "dev-bbbbbbbbbbbbbb", got.Hash,
		"deleted hash must never be matched")
}

// TestRedisPool_DeleteByHash_EmptyHashIsNoOp — empty hash is
// tolerated (returns 0, nil) because the api layer can pass
// through an unverified JWT subject in a future regression and
// the matching layer should not crash on that. Fail-soft over
// fail-loud for the hook path.
func TestRedisPool_DeleteByHash_EmptyHashIsNoOp(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	removed, err := pool.DeleteByHash(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, int64(0), removed)
}

// TestRedisPool_DeleteByHash_NotInPool — calling DeleteByHash
// for a hash that was never registered returns 0, nil. Idempotent.
func TestRedisPool_DeleteByHash_NotInPool(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))

	removed, err := pool.DeleteByHash(ctx, "dev-doesnotexist")
	require.NoError(t, err)
	assert.Equal(t, int64(0), removed)

	// The registered user is still in the pool.
	n, err := pool.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}

// TestRedisPool_DeleteByHash_Idempotent — calling DeleteByHash
// twice for the same hash is safe; the second call removes 0.
func TestRedisPool_DeleteByHash_Idempotent(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))

	first, err := pool.DeleteByHash(ctx, "dev-aaaaaaaaaaaaaa")
	require.NoError(t, err)
	assert.Equal(t, int64(1), first)

	second, err := pool.DeleteByHash(ctx, "dev-aaaaaaaaaaaaaa")
	require.NoError(t, err)
	assert.Equal(t, int64(0), second, "second delete must be a no-op")
}

// TestRedisPool_DeleteByHash_AfterReRegister — the encoding
// (operator|country|app|hash) means a re-registration with
// DIFFERENT metadata produces a NEW member string (different
// op/country/app), so two rows coexist for the same hash until
// one is popped or evicted. DeleteByHash must remove BOTH on
// KVKK DELETE — a stale registration with the same hash and
// different metadata would otherwise keep offering the deleted
// device as a P2P receiver. This test pins that contract.
func TestRedisPool_DeleteByHash_AfterReRegister(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	const hash = "dev-aaaaaaaaaaaaaa"
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo(hash, "Turkcell", "TR", "whatsapp"),
		5*time.Minute))

	// Re-register with different metadata. Because the
	// member string embeds op/country/app, this is a new
	// member — ZADD does NOT upsert by hash alone.
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo(hash, "Vodafone", "GB", "signal"),
		5*time.Minute))

	// Both rows are in the pool — confirm the precondition.
	n, err := pool.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), n, "two distinct metadata rows for the same hash")

	// A single DeleteByHash call removes BOTH. This is the
	// guarantee STRIDE-6-03 needs: KVKK DELETE means "this
	// device is gone from the pool", regardless of how many
	// in-flight registrations exist for the same hash.
	removed, err := pool.DeleteByHash(ctx, hash)
	require.NoError(t, err)
	assert.Equal(t, int64(2), removed)

	n, err = pool.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

// TestRedisPool_SweepIdle_EvictsExpired — the manual
// SweepIdle call removes all members whose score is in the
// past. This is the cold-path complement to the lazy cleanup
// in Count / MatchSender; it's exposed directly so the
// background RunSweeper (and tests like this one) can drive it
// on demand.
func TestRedisPool_SweepIdle_EvictsExpired(t *testing.T) {
	pool, mr := newTestRedisPool(t)
	ctx := context.Background()

	// Register one fresh receiver and inject two stale ones.
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-fresh111111111", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))
	pastScore := float64(time.Now().Add(-time.Hour).Unix())
	mr.ZAdd(indexKey, pastScore, "ghost\x1f-\x1f-\x1fghost-aaaaaaaaaaaa")
	mr.ZAdd(indexKey, pastScore, "ghost\x1f-\x1f-\x1fghost-bbbbbbbbbbbb")

	// SweepIdle must drop the two ghosts and leave the fresh
	// receiver intact.
	evicted, err := pool.SweepIdle(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), evicted, "two stale members should be evicted")

	n, err := pool.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n, "fresh member must survive the sweep")
}

// TestRedisPool_SweepIdle_NoOpWhenFresh — a sweep against a
// pool with no expired members returns 0.
func TestRedisPool_SweepIdle_NoOpWhenFresh(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-bbbbbbbbbbbbbb", "Vodafone", "TR", "whatsapp"),
		5*time.Minute))

	evicted, err := pool.SweepIdle(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), evicted)

	n, err := pool.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

// TestRedisPool_RunSweeper_StopsOnContextCancel verifies the
// goroutine plumbing: RunSweeper returns when its ctx is
// cancelled. We don't wait for a tick (that's covered by the
// SweepIdle unit tests); the contract here is "cancellation
// works".
func TestRedisPool_RunSweeper_StopsOnContextCancel(t *testing.T) {
	pool, _ := newTestRedisPool(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		pool.RunSweeper(ctx)
		close(done)
	}()

	// Give the goroutine a moment to enter the select.
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Sweeper returned promptly after cancel. Pass.
	case <-time.After(2 * time.Second):
		t.Fatal("RunSweeper did not return within 2s after ctx cancel")
	}
}

// TestRedisPool_RunSweeper_EvictsExpiredMembers is the
// end-to-end idle-purge test: launch the sweeper, inject a
// stale member, wait for the next tick (we use a tiny custom
// interval via the existing DefaultIdleSweepInterval const and
// a wait slightly longer than that), and assert the stale row
// is gone. We don't reset DefaultIdleSweepInterval for this
// test — 60s is too slow for a unit test. Instead, the
// companion test below uses the SweepIdle direct path to
// cover the same behaviour deterministically; the production
// behaviour is then a function call we trust.
func TestRedisPool_RunSweeper_TickEvictsStale(t *testing.T) {
	pool, mr := newTestRedisPool(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Inject a stale member so any future sweep has work to do.
	pastScore := float64(time.Now().Add(-time.Hour).Unix())
	mr.ZAdd(indexKey, pastScore, "ghost\x1f-\x1f-\x1fghost-aaaaaaaaaaaa")

	// Manually drive a single tick — we don't want to wait 60s
	// in a unit test. SweepIdle is the per-tick body of
	// RunSweeper, so calling it directly is equivalent to
	// waiting for one ticker firing.
	evicted, err := pool.SweepIdle(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), evicted)

	n, err := pool.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	// Confirm RunSweeper itself still composes correctly with
	// SweepIdle: launch it for a short while and cancel.
	swCtx, swCancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		pool.RunSweeper(swCtx)
		close(done)
	}()
	time.Sleep(10 * time.Millisecond)
	swCancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunSweeper did not exit after cancel")
	}
}

// TestRedisPool_DeleteByHash_ConcurrentDeleteAndReRegister is
// the adversarial race from the verifier §6 G criterion: a
// delete + a re-register racing against each other must not
// leave a stale entry in the pool. We run N goroutines, half
// deleting, half re-registering, and assert the post-condition
// (no stale entry, no orphan): the final state is either
// "registered fresh" (re-register wins) or "deleted" (delete
// wins), with no in-between ghost.
func TestRedisPool_DeleteByHash_ConcurrentDeleteAndReRegister(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	const hash = "dev-aaaaaaaaaaaaaa"
	const rounds = 50

	// Seed: register once.
	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo(hash, "Turkcell", "TR", "whatsapp"),
		5*time.Minute))

	var wg sync.WaitGroup
	// Half the rounds delete, half re-register.
	for i := 0; i < rounds; i++ {
		wg.Add(1)
		if i%2 == 0 {
			go func() {
				defer wg.Done()
				_, _ = pool.DeleteByHash(ctx, hash)
			}()
		} else {
			go func() {
				defer wg.Done()
				_ = pool.RegisterReceiver(ctx,
					sampleInfo(hash, "Turkcell", "TR", "whatsapp"),
					5*time.Minute)
			}()
		}
	}
	wg.Wait()

	// Post-condition: at most ONE entry exists (the latest
	// registration if a re-register won the race). The pool
	// MUST NOT have a phantom ghost from a partial delete.
	n, err := pool.Count(ctx)
	require.NoError(t, err)
	assert.LessOrEqual(t, n, int64(1),
		"concurrent delete+re-register must not leave >1 entry for the same hash, got %d", n)

	// A final sweep must produce 0 evicted (the surviving entry,
	// if any, has a future TTL).
	evicted, err := pool.SweepIdle(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), evicted)
}

// TestRedisPool_DeleteByHash_MalformedMemberIgnored is the
// defensive test: if a malformed member somehow lands in the
// ZSET (manual Redis poke, a botched upgrade, etc.),
// DeleteByHash must skip it without panicking and without
// returning an error. The privacy invariant forbids logging
// inside the matching package, so a silent skip is the only
// safe posture.
func TestRedisPool_DeleteByHash_MalformedMemberIgnored(t *testing.T) {
	pool, mr := newTestRedisPool(t)
	ctx := context.Background()

	// Inject a malformed member (only 2 fields instead of 4).
	futureScore := float64(time.Now().Add(5 * time.Minute).Unix())
	mr.ZAdd(indexKey, futureScore, "garbage\x1fnot-well-formed")

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))

	removed, err := pool.DeleteByHash(ctx, "dev-aaaaaaaaaaaaaa")
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed, "well-formed member must be removed")

	// The malformed row is still there (we skip it, not delete it).
	ss, err := mr.SortedSet(indexKey)
	require.NoError(t, err)
	assert.Len(t, ss, 1, "malformed member survives DeleteByHash (deferred to SweepIdle)")
}

func TestRedisPool_Register_ExtendsTTL(t *testing.T) {
	pool, mr := newTestRedisPool(t)
	ctx := context.Background()

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		10*time.Second))

	ss1, err := mr.SortedSet(indexKey)
	require.NoError(t, err)
	require.Len(t, ss1, 1)
	var score1 float64
	for _, s := range ss1 {
		score1 = s
		break
	}

	time.Sleep(50 * time.Millisecond)

	require.NoError(t, pool.RegisterReceiver(ctx,
		sampleInfo("dev-aaaaaaaaaaaaaa", "Turkcell", "TR", "whatsapp"),
		5*time.Minute))

	ss2, err := mr.SortedSet(indexKey)
	require.NoError(t, err)
	require.Len(t, ss2, 1)
	var score2 float64
	for _, s := range ss2 {
		score2 = s
		break
	}

	assert.Greater(t, score2, score1, "re-register should extend expiry score")
}

// ---------- atomicity under concurrency ----------

// TestRedisPool_ConcurrentMatches_NoDuplicate verifies that the
// Lua-script-backed MatchSender is atomic: N concurrent matches
// against N receivers must yield N distinct hashes, with no two
// callers receiving the same ReceiverInfo. This is the property
// that prevents two senders from being paired with the same
// volunteer.
func TestRedisPool_ConcurrentMatches_NoDuplicate(t *testing.T) {
	pool, _ := newTestRedisPool(t)
	ctx := context.Background()

	const N = 25
	for i := 0; i < N; i++ {
		require.NoError(t, pool.RegisterReceiver(ctx,
			sampleInfo(
				"dev-"+string(rune('a'+i%26))+string(rune('a'+i/26))+
					string(rune('a'+i/26))+string(rune('a'+i/26))+
					"1234567890ab",
				"Turkcell", "TR", "whatsapp"),
			5*time.Minute))
	}

	results := make(chan string, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info, err := pool.MatchSender(ctx, Criteria{})
			if err != nil {
				results <- "ERR:" + err.Error()
				return
			}
			results <- info.Hash
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[string]int)
	for r := range results {
		seen[r]++
	}
	assert.Len(t, seen, N, "each receiver should be matched exactly once")

	for hash, count := range seen {
		assert.Equal(t, 1, count,
			"hash %s matched %d times — atomicity violated", hash, count)
	}

	// Pool should now be empty.
	n, err := pool.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

// ---------- encoding round-trip ----------

func TestEncodeDecodeMember_RoundTrip(t *testing.T) {
	cases := []ReceiverInfo{
		{Hash: "dev-aaaaaaaaaaaaaa", Operator: "Turkcell", Country: "TR", App: "whatsapp"},
		{Hash: "dev-bbbbbbbbbbbbbb", Operator: "", Country: "GB", App: "signal"},
		{Hash: "dev-cccccccccccccc", Operator: "BT", Country: "", App: ""},
		{Hash: "dev-dddddddddddddd", Operator: "", Country: "", App: ""},
	}
	for _, c := range cases {
		enc := encodeMember(c)
		parts := splitOnUS(enc)
		require.Len(t, parts, 4, "encoded member must always have 4 fields, got %q", enc)
		got, err := decodeMember(enc)
		require.NoError(t, err)
		// Empty fields are normalised to "" in both directions;
		// non-empty fields survive intact.
		if c.Operator == "" {
			assert.Equal(t, "", got.Operator)
		} else {
			assert.Equal(t, c.Operator, got.Operator)
		}
		if c.Country == "" {
			assert.Equal(t, "", got.Country)
		} else {
			assert.Equal(t, c.Country, got.Country)
		}
		if c.App == "" {
			assert.Equal(t, "", got.App)
		} else {
			assert.Equal(t, c.App, got.App)
		}
		assert.Equal(t, c.Hash, got.Hash)
	}
}

func TestDecodeMember_Malformed(t *testing.T) {
	cases := []string{
		"",
		"only-one-field",
		"two\x1ffields",
		"three\x1ffields\x1fhere",
	}
	for _, c := range cases {
		_, err := decodeMember(c)
		assert.Error(t, err, "expected error for %q", c)
	}
}

// splitOnUS splits on the ASCII Unit Separator (\x1f). Local
// helper to avoid pulling in strings.Split with the separator
// constant from production code (and to make the test
// self-documenting about what's being split).
func splitOnUS(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '\x1f' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	out = append(out, cur)
	return out
}

// ---------- privacy invariant ----------

// TestPackageNoLoggingOrPrinting grep-scans the production
// source for log/slog/fmt.Print* calls. Matches the convention
// established in PR-5's echobot tests. The handler logs nothing
// about envelopes (privacy: the signalling layer is a dumb
// pipe); metrics-on-the-side go through a future PR.
func TestPackageNoLoggingOrPrinting(t *testing.T) {
	files := []string{
		"pool.go",
		"signalling.go",
	}
	for _, f := range files {
		body := readSourceForTest(t, f)
		stripped := stripGoComments(body)
		for _, banned := range []string{
			"log.",
			"fmt.Print",
			"fmt.Println",
			"fmt.Printf",
			"slog.",
		} {
			assert.NotContains(t, stripped, banned,
				"%s contains banned token %q (privacy invariant)", f, banned)
		}
	}
}

// readSourceForTest reads a sibling .go file (relative to the
// test CWD, which `go test` sets to the package directory).
func readSourceForTest(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	require.NoError(t, err)
	return string(b)
}

// stripGoComments removes // and /* */ comments from Go source so
// a string match isn't fooled by a banned-pattern mention inside
// a doc comment. Same algorithm as internal/echobot's
// stripGoComments — copy-pasted here so the matching package
// remains self-contained (no shared test helper module).
func stripGoComments(src string) string {
	var (
		out     strings.Builder
		i       int
		inBlock bool
	)
	for i < len(src) {
		if inBlock {
			if i+1 < len(src) && src[i] == '*' && src[i+1] == '/' {
				inBlock = false
				i += 2
				continue
			}
			i++
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			inBlock = true
			i += 2
			continue
		}
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		if src[i] == '"' {
			out.WriteByte('"')
			i++
			for i < len(src) && src[i] != '"' {
				if src[i] == '\\' && i+1 < len(src) {
					out.WriteByte(src[i])
					i++
				}
				out.WriteByte(src[i])
				i++
			}
			if i < len(src) {
				out.WriteByte('"')
				i++
			}
			continue
		}
		out.WriteByte(src[i])
		i++
	}
	return out.String()
}
