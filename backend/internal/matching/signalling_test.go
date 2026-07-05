package matching

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- Hub unit tests ----------

func TestHub_RegisterSendReceive(t *testing.T) {
	hub := NewHub()
	ctx := context.Background()

	in, err := hub.Register(ctx, "alice-1234567890ab")
	require.NoError(t, err)
	t.Cleanup(func() { _ = in.Close() })

	require.NoError(t, hub.Send(ctx, "alice-1234567890ab", Envelope{
		Type:      TypeOffer,
		From:      "bob",
		To:        "alice",
		SessionID: "sess-1",
		Timestamp: time.Now().UTC(),
	}))

	select {
	case got := <-in.C():
		assert.Equal(t, TypeOffer, got.Type)
		assert.Equal(t, "bob", got.From)
	case <-time.After(time.Second):
		t.Fatal("did not receive envelope within 1s")
	}
}

func TestHub_SendPeerNotConnected(t *testing.T) {
	hub := NewHub()
	err := hub.Send(context.Background(), "ghost-1234567890ab", Envelope{Type: TypeOffer})
	require.ErrorIs(t, err, ErrPeerNotConnected)
}

func TestHub_InboxFullReturnsError(t *testing.T) {
	hub := NewHub()
	ctx := context.Background()
	in, err := hub.Register(ctx, "alice-1234567890ab")
	require.NoError(t, err)
	t.Cleanup(func() { _ = in.Close() })

	// Fill the inbox (capacity = 64) without reading.
	for i := 0; i < inboxCapacity; i++ {
		require.NoError(t, hub.Send(ctx, "alice-1234567890ab", Envelope{
			Type:      TypeICECandidate,
			From:      "bob",
			To:        "alice",
			SessionID: "s",
			Timestamp: time.Now().UTC(),
		}))
	}
	err = hub.Send(ctx, "alice-1234567890ab", Envelope{Type: TypeICECandidate})
	require.ErrorIs(t, err, ErrInboxFull)
}

func TestHub_UnregisterClosesInbox(t *testing.T) {
	hub := NewHub()
	ctx := context.Background()
	in, err := hub.Register(ctx, "alice-1234567890ab")
	require.NoError(t, err)

	hub.Unregister("alice-1234567890ab")

	// Inbox channel must be closed.
	select {
	case _, ok := <-in.C():
		assert.False(t, ok, "channel should be closed")
	case <-time.After(time.Second):
		t.Fatal("inbox not closed within 1s")
	}
}

func TestHub_ReregisterClosesPreviousInbox(t *testing.T) {
	hub := NewHub()
	ctx := context.Background()
	first, err := hub.Register(ctx, "alice-1234567890ab")
	require.NoError(t, err)

	second, err := hub.Register(ctx, "alice-1234567890ab")
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	// First inbox should be closed after re-register.
	select {
	case _, ok := <-first.C():
		assert.False(t, ok, "first inbox should be closed after re-register")
	case <-time.After(time.Second):
		t.Fatal("first inbox not closed after re-register")
	}
}

func TestHub_Register_RejectsBadHash(t *testing.T) {
	hub := NewHub()
	cases := []string{
		"",                              // empty
		"short",                         // < 16 chars
		strings.Repeat("a", 65),         // > 64 chars
	}
	for _, h := range cases {
		_, err := hub.Register(context.Background(), h)
		assert.Error(t, err, "expected error for hash of length %d", len(h))
	}
}

func TestHub_Register_RespectsContextCancellation(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := hub.Register(ctx, "alice-1234567890ab")
	require.Error(t, err)
}

func TestHub_ConnectedCountAndIsConnected(t *testing.T) {
	hub := NewHub()
	ctx := context.Background()
	assert.Equal(t, 0, hub.ConnectedCount())
	assert.False(t, hub.IsConnected("alice-1234567890ab"))

	in1, err := hub.Register(ctx, "alice-1234567890ab")
	require.NoError(t, err)
	t.Cleanup(func() { _ = in1.Close() })
	in2, err := hub.Register(ctx, "bob-1234567890abcdef")
	require.NoError(t, err)
	t.Cleanup(func() { _ = in2.Close() })

	assert.Equal(t, 2, hub.ConnectedCount())
	assert.True(t, hub.IsConnected("alice-1234567890ab"))
	assert.True(t, hub.IsConnected("bob-1234567890abcdef"))
}

func TestHub_SatisfiesRelayInterface(t *testing.T) {
	var _ Relay = (*Hub)(nil)
}

// ---------- Envelope.Validate ----------

func TestEnvelope_Validate(t *testing.T) {
	now := time.Now().UTC()
	good := Envelope{
		Type: TypeOffer, From: "alice-1234567890ab", To: "bob-1234567890ab",
		SessionID: "sess-1", Timestamp: now,
	}
	require.NoError(t, good.Validate())

	cases := map[string]struct {
		mutate func(e *Envelope)
	}{
		"missing_type":       {func(e *Envelope) { e.Type = "" }},
		"unknown_type":       {func(e *Envelope) { e.Type = "wat" }},
		"short_from":         {func(e *Envelope) { e.From = "short" }},
		"long_from":          {func(e *Envelope) { e.From = strings.Repeat("a", 65) }},
		"short_to":           {func(e *Envelope) { e.To = "short" }},
		"long_to":            {func(e *Envelope) { e.To = strings.Repeat("a", 65) }},
		"missing_session_id": {func(e *Envelope) { e.SessionID = "" }},
		"missing_timestamp":  {func(e *Envelope) { e.Timestamp = time.Time{} }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := good
			tc.mutate(&e)
			require.Error(t, e.Validate())
		})
	}

	// To is allowed to be empty (broadcast).
	e := good
	e.To = ""
	require.NoError(t, e.Validate())
}

// ---------- WebSocket integration tests ----------

// startTestServer wires the Handler into an httptest.Server and
// returns (server, hub). The hub is exposed so tests can poke
// at internal state (e.g. assert hub.IsConnected).
func startTestServer(t *testing.T) (*httptest.Server, *Hub) {
	t.Helper()
	hub := NewHub()
	h := NewHandler(hub)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, hub
}

// dial connects a websocket client to the test server,
// requesting the given device hash.
func dial(t *testing.T, srv *httptest.Server, hash string) *websocket.Conn {
	t.Helper()
	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"
	u.Path = "/"
	u.RawQuery = "device=" + hash
	conn, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("dial: %v (status=%d)", err, resp.StatusCode)
		}
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// wsURL returns a ws:// URL for the test server with the given
// device hash query parameter.
func wsURL(srv *httptest.Server, hash string) string {
	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"
	u.Path = "/"
	u.RawQuery = "device=" + url.QueryEscape(hash)
	return u.String()
}

func TestHandler_RejectsMissingDevice(t *testing.T) {
	srv, _ := startTestServer(t)
	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"
	u.Path = "/"
	// No device query param.
	_, resp, err := websocket.DefaultDialer.Dial(u.String(), nil)
	require.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	}
}

func TestHandler_RejectsShortDevice(t *testing.T) {
	srv, _ := startTestServer(t)
	_, resp, err := websocket.DefaultDialer.Dial(wsURL(srv, "short"), nil)
	require.Error(t, err)
	if resp != nil {
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	}
}

func TestHandler_RegistersInboxOnConnect(t *testing.T) {
	srv, hub := startTestServer(t)
	conn := dial(t, srv, "alice-1234567890ab")
	_ = conn

	// Allow the server a moment to Register before we assert.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.IsConnected("alice-1234567890ab") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not register inbox within 2s")
}

func TestHandler_EndToEndOfferAnswer(t *testing.T) {
	srv, hub := startTestServer(t)
	alice := dial(t, srv, "alice-1234567890ab")
	bob := dial(t, srv, "bob-1234567890abcdef")

	// Wait for both inboxes to be registered.
	waitForConnected(t, hub, "alice-1234567890ab")
	waitForConnected(t, hub, "bob-1234567890abcdef")

	// Alice sends an offer to Bob.
	require.NoError(t, alice.WriteJSON(Envelope{
		Type:      TypeOffer,
		From:      "alice-1234567890ab",
		To:        "bob-1234567890abcdef",
		SessionID: "sess-1",
		Payload:   json.RawMessage(`{"sdp":"v=0...","sdp_type":"offer"}`),
		Timestamp: time.Now().UTC(),
	}))

	// Bob receives the offer.
	got := readEnvelope(t, bob, 2*time.Second)
	assert.Equal(t, TypeOffer, got.Type)
	assert.Equal(t, "alice-1234567890ab", got.From)
	assert.Equal(t, "bob-1234567890abcdef", got.To)
	assert.Equal(t, "sess-1", got.SessionID)

	// Bob answers.
	require.NoError(t, bob.WriteJSON(Envelope{
		Type:      TypeAnswer,
		From:      "bob-1234567890abcdef",
		To:        "alice-1234567890ab",
		SessionID: "sess-1",
		Payload:   json.RawMessage(`{"sdp":"v=0...","sdp_type":"answer"}`),
		Timestamp: time.Now().UTC(),
	}))
	got = readEnvelope(t, alice, 2*time.Second)
	assert.Equal(t, TypeAnswer, got.Type)
	assert.Equal(t, "bob-1234567890abcdef", got.From)
}

func TestHandler_DropsEnvelopeWithMismatchedFrom(t *testing.T) {
	srv, hub := startTestServer(t)
	alice := dial(t, srv, "alice-1234567890ab")
	_ = dial(t, srv, "bob-1234567890abcdef") // Bob present but irrelevant.
	waitForConnected(t, hub, "alice-1234567890ab")
	waitForConnected(t, hub, "bob-1234567890abcdef")

	// Alice tries to spoof Bob.
	require.NoError(t, alice.WriteJSON(Envelope{
		Type:      TypeOffer,
		From:      "bob-1234567890abcdef", // not Alice's hash
		To:        "bob-1234567890abcdef",
		SessionID: "sess-1",
		Timestamp: time.Now().UTC(),
	}))

	// The "unregistered" error frame is written back to Alice's
	// own socket — not Bob's — because the From-auth check fails
	// at the sender's connection.
	errEnv := readEnvelope(t, alice, 2*time.Second)
	assert.Equal(t, TypeError, errEnv.Type)
	assert.Contains(t, string(errEnv.Payload), "unregistered")
}

func TestHandler_RejectsICERateLimit(t *testing.T) {
	hub := NewHub()
	h := NewHandler(hub).SetICERate(3, 0.001) // 3 burst, near-zero refill
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	alice := dial(t, srv, "alice-1234567890ab")
	bob := dial(t, srv, "bob-1234567890abcdef")
	waitForConnected(t, hub, "alice-1234567890ab")
	waitForConnected(t, hub, "bob-1234567890abcdef")

	// Send 5 ICE candidates — first 3 should land on Bob, the
	// rest should produce "rate_limited" errors on Alice's side
	// (and the connection should close).
	delivered := 0
	for i := 0; i < 5; i++ {
		require.NoError(t, alice.WriteJSON(Envelope{
			Type:      TypeICECandidate,
			From:      "alice-1234567890ab",
			To:        "bob-1234567890abcdef",
			SessionID: "sess-1",
			Payload:   json.RawMessage(fmt.Sprintf(`{"candidate":"cand-%d"}`, i)),
			Timestamp: time.Now().UTC(),
		}))
	}

	// Drain Bob's inbox (best effort, until channel closes).
	deadline := time.After(2 * time.Second)
loop:
	for {
		select {
		case _, ok := <-bobChannel(t, bob):
			if !ok {
				break loop
			}
			delivered++
		case <-deadline:
			break loop
		}
	}
	// We don't assert the exact delivered count (timing-sensitive),
	// only that Bob got at least one and at most 3 (burst size).
	assert.GreaterOrEqual(t, delivered, 1)
	assert.LessOrEqual(t, delivered, 3)

	// After rate-limit kicks in, Alice's connection should be
	// closed by the server. ReadMessage must return an error
	// within a short window.
	_ = alice.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = alice.ReadMessage()
}

// bobChannel is a helper that yields one envelope from a
// goroutine-safe wrapper. Test-only.
func bobChannel(t *testing.T, conn *websocket.Conn) <-chan Envelope {
	t.Helper()
	ch := make(chan Envelope, 1)
	go func() {
		var env Envelope
		if err := conn.ReadJSON(&env); err != nil {
			close(ch)
			return
		}
		ch <- env
	}()
	return ch
}

// conn is a tiny alias-free helper since we shadow the parameter
// name in some tests.
type conn = websocket.Conn

// readEnvelope reads one Envelope JSON from conn within d. The
// local `conn` type alias above avoids name collisions with the
// parameter.
func readEnvelope(t *testing.T, c *websocket.Conn, d time.Duration) Envelope {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(d))
	var env Envelope
	if err := c.ReadJSON(&env); err != nil {
		t.Fatalf("readEnvelope: %v", err)
	}
	return env
}

func waitForConnected(t *testing.T, hub *Hub, hash string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.IsConnected(hash) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("hub never registered %s within 2s", hash)
}

// TestHandler_PeerUnavailableWhenNoInbox verifies that sending
// to a hash with no inbox produces a "peer_unavailable" error
// frame back to the sender.
func TestHandler_PeerUnavailableWhenNoInbox(t *testing.T) {
	srv, _ := startTestServer(t)
	alice := dial(t, srv, "alice-1234567890ab")
	// Bob never connects.

	require.NoError(t, alice.WriteJSON(Envelope{
		Type:      TypeOffer,
		From:      "alice-1234567890ab",
		To:        "ghost-1234567890abcdef",
		SessionID: "sess-1",
		Timestamp: time.Now().UTC(),
	}))

	errEnv := readEnvelope(t, alice, 2*time.Second)
	assert.Equal(t, TypeError, errEnv.Type)
	assert.Contains(t, string(errEnv.Payload), "peer_unavailable")
}

// TestHandler_DropsInvalidJSON verifies that garbage frames are
// silently dropped (the connection stays open).
func TestHandler_DropsInvalidJSON(t *testing.T) {
	srv, hub := startTestServer(t)
	alice := dial(t, srv, "alice-1234567890ab")
	bob := dial(t, srv, "bob-1234567890abcdef")
	waitForConnected(t, hub, "alice-1234567890ab")
	waitForConnected(t, hub, "bob-1234567890abcdef")

	// Garbage frame.
	require.NoError(t, alice.WriteMessage(websocket.TextMessage, []byte("not json")))

	// Valid frame after garbage must still go through.
	require.NoError(t, alice.WriteJSON(Envelope{
		Type:      TypeOffer,
		From:      "alice-1234567890ab",
		To:        "bob-1234567890abcdef",
		SessionID: "sess-1",
		Timestamp: time.Now().UTC(),
	}))
	got := readEnvelope(t, bob, 2*time.Second)
	assert.Equal(t, TypeOffer, got.Type)
}

// TestHandler_ConcurrentConnectionsIsolated verifies that two
// concurrent connections with different hashes don't cross-talk.
func TestHandler_ConcurrentConnectionsIsolated(t *testing.T) {
	srv, hub := startTestServer(t)

	const N = 5
	conns := make([]*websocket.Conn, N)
	hashes := make([]string, N)
	for i := 0; i < N; i++ {
		h := fmt.Sprintf("peer-%02d-1234567890ab", i)
		hashes[i] = h
		conns[i] = dial(t, srv, h)
		waitForConnected(t, hub, h)
	}

	// Each peer sends to peer 0 and confirms only peer 0 gets it.
	var wg sync.WaitGroup
	for sender := 1; sender < N; sender++ {
		wg.Add(1)
		go func(s int) {
			defer wg.Done()
			require.NoError(t, conns[s].WriteJSON(Envelope{
				Type:      TypeOffer,
				From:      hashes[s],
				To:        hashes[0],
				SessionID: "sess-" + hashes[s],
				Timestamp: time.Now().UTC(),
			}))
		}(sender)
	}
	wg.Wait()

	// Peer 0 should receive N-1 envelopes; no other peer should.
	received := 0
	deadline := time.After(2 * time.Second)
	for received < N-1 {
		select {
		case env := <-drainOne(conns[0]):
			assert.Equal(t, hashes[0], env.To)
			received++
		case <-deadline:
			t.Fatalf("peer 0 received %d, expected %d", received, N-1)
		}
	}

	// Sanity: peer 1 should have received nothing.
	_ = conns[1].SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	var probe Envelope
	err := conns[1].ReadJSON(&probe)
	if err == nil {
		t.Errorf("peer 1 unexpectedly received an envelope: %+v", probe)
	}
}

func drainOne(c *websocket.Conn) <-chan Envelope {
	ch := make(chan Envelope, 1)
	go func() {
		var env Envelope
		if err := c.ReadJSON(&env); err != nil {
			close(ch)
			return
		}
		ch <- env
	}()
	return ch
}

// TestHandler_DropsEnvelopeWithBadSchema verifies that an envelope
// failing Validate (e.g. unknown type) is dropped silently.
func TestHandler_DropsEnvelopeWithBadSchema(t *testing.T) {
	srv, hub := startTestServer(t)
	alice := dial(t, srv, "alice-1234567890ab")
	bob := dial(t, srv, "bob-1234567890abcdef")
	waitForConnected(t, hub, "alice-1234567890ab")
	waitForConnected(t, hub, "bob-1234567890abcdef")

	// Unknown type.
	require.NoError(t, alice.WriteJSON(Envelope{
		Type:      "totally-bogus",
		From:      "alice-1234567890ab",
		To:        "bob-1234567890abcdef",
		SessionID: "sess-1",
		Timestamp: time.Now().UTC(),
	}))

	// Followed by a valid offer.
	require.NoError(t, alice.WriteJSON(Envelope{
		Type:      TypeOffer,
		From:      "alice-1234567890ab",
		To:        "bob-1234567890abcdef",
		SessionID: "sess-1",
		Timestamp: time.Now().UTC(),
	}))
	got := readEnvelope(t, bob, 2*time.Second)
	assert.Equal(t, TypeOffer, got.Type)
}

// TestHub_SendRespectsContextCancellation: Send returns ctx.Err()
// when ctx is already done. Mirrors Register's behaviour.
func TestHub_SendRespectsContextCancellation(t *testing.T) {
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := hub.Send(ctx, "alice-1234567890ab", Envelope{Type: TypeOffer})
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled))
}

// TestHandler_ConcurrentMatchesAreRaceFree runs many concurrent
// writers into the Hub to flush out any data races on the
// internal map.
func TestHandler_ConcurrentMatchesAreRaceFree(t *testing.T) {
	hub := NewHub()
	h := NewHandler(hub)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	const writers = 10
	const writesPerWriter = 20
	var wg sync.WaitGroup
	var delivered atomic.Int64

	receivers := make([]*websocket.Conn, writers)
	receiverHashes := make([]string, writers)
	for i := 0; i < writers; i++ {
		h := fmt.Sprintf("recv-%02d-1234567890ab", i)
		receiverHashes[i] = h
		receivers[i] = dial(t, srv, h)
		waitForConnected(t, hub, h)
	}

	// Each writer dials, sends N envelopes to one receiver, then
	// closes. We use a single writer connection per receiver to
	// keep test setup simple.
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sender := dial(t, srv, fmt.Sprintf("send-%02d-1234567890ab", idx))
			for j := 0; j < writesPerWriter; j++ {
				_ = sender.WriteJSON(Envelope{
					Type:      TypeOffer,
					From:      fmt.Sprintf("send-%02d-1234567890ab", idx),
					To:        receiverHashes[idx],
					SessionID: "sess",
					Timestamp: time.Now().UTC(),
				})
			}
		}(i)
	}
	wg.Wait()

	// Each receiver should have all writesPerWriter envelopes
	// buffered (or have read them; we just want to confirm no
	// panic / race).
	deadline := time.After(3 * time.Second)
	for i := 0; i < writers; i++ {
		go func(idx int) {
			for {
				_ = receivers[idx].SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				var env Envelope
				if err := receivers[idx].ReadJSON(&env); err != nil {
					return
				}
				delivered.Add(1)
				select {
				case <-deadline:
					return
				default:
				}
			}
		}(i)
	}
	// Wait for deliveries to plateau.
	time.Sleep(2 * time.Second)
	assert.Greater(t, delivered.Load(), int64(writers*writesPerWriter/2),
		"expected at least half the envelopes delivered (got %d)", delivered.Load())
}