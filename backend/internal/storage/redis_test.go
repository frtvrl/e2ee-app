package storage

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRedisTestStore(t *testing.T) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	store, err := NewRedisStore(context.Background(), mr.Addr(), "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	return store, mr
}

func TestNewRedisStore_PingFailure(t *testing.T) {
	// Bind a real TCP port, then close it so the address is unreachable.
	// Avoids race conditions with naive port-counter incrementing.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	require.NoError(t, lis.Close())

	_, err = NewRedisStore(context.Background(), addr, "")
	require.Error(t, err)
}

func TestRedisStore_AddAndPop(t *testing.T) {
	store, _ := newRedisTestStore(t)
	ctx := context.Background()

	require.NoError(t, store.Add(ctx, "dev-1", 5*time.Minute))
	require.NoError(t, store.Add(ctx, "dev-2", 5*time.Minute))

	got, err := store.PopMatching(ctx)
	require.NoError(t, err)
	assert.Equal(t, "dev-1", got, "FIFO: first-pushed must be first-popped")

	got, err = store.PopMatching(ctx)
	require.NoError(t, err)
	assert.Equal(t, "dev-2", got)
}

func TestRedisStore_PopMatching_EmptyReturnsNotFound(t *testing.T) {
	store, _ := newRedisTestStore(t)
	got, err := store.PopMatching(context.Background())
	assert.Equal(t, "", got)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRedisStore_Add_RejectsEmpty(t *testing.T) {
	store, _ := newRedisTestStore(t)
	require.Error(t, store.Add(context.Background(), "", time.Minute))
}

func TestRedisStore_Add_DefaultTTLOnZero(t *testing.T) {
	store, mr := newRedisTestStore(t)
	require.NoError(t, store.Add(context.Background(), "dev", 0))
	// List key must exist with the default TTL applied.
	assert.True(t, mr.Exists(activePoolKey))
	ttl := mr.TTL(activePoolKey)
	assert.InDelta(t, DefaultPoolTTL, ttl, float64(time.Second))
}

func TestRedisStore_Count(t *testing.T) {
	store, _ := newRedisTestStore(t)
	ctx := context.Background()

	require.NoError(t, store.Add(ctx, "a", time.Minute))
	require.NoError(t, store.Add(ctx, "b", time.Minute))
	require.NoError(t, store.Add(ctx, "c", time.Minute))

	n, err := store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(3), n)

	_, err = store.PopMatching(ctx)
	require.NoError(t, err)

	n, err = store.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
}

func TestRedisStore_FIFOOrderRespectsTTL(t *testing.T) {
	store, _ := newRedisTestStore(t)
	ctx := context.Background()

	require.NoError(t, store.Add(ctx, "first", 30*time.Second))

	// Subsequent Add refreshes the list-level TTL.
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, store.Add(ctx, "second", 30*time.Second))

	got, err := store.PopMatching(ctx)
	require.NoError(t, err)
	assert.Equal(t, "first", got)
	got, err = store.PopMatching(ctx)
	require.NoError(t, err)
	assert.Equal(t, "second", got)
}
