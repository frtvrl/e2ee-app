// signalling.go — WebSocket signalling handler + in-process Hub.
//
// Architecture:
//
//   Mobile A                      Mobile B
//      |                              |
//      |       ws/v1/signalling       |
//      +------> Upgrade + parse ------+----> Bridge (Handler)
//                                       |
//                                       v
//                            +---------------------+
//                            |        Hub          |
//                            |  inboxes map<hash>  |
//                            +---------------------+
//
// The Handler upgrades the HTTP request to a WebSocket, reads
// Envelope JSON frames from the socket, validates them, and
// passes them through the Hub. Outgoing envelopes (from the
// Hub inbox to the wire) are written back to the socket.
//
// Per ADR-0004 §1, Sprint 1 is JSON-passthrough only — no Pion
// WebRTC integration. The WebSocket bytes the two mobile apps
// exchange through this Hub carry their own SDP/ICE content as
// opaque base64 (see shared/schemas/p2p-signalling.schema.json).
// Sprint 2 introduced the actual SDP parser + ICE candidate
// validator inside the Hub envelope — the Hub interface stays
// the same so PR-7 (REST wiring) and the mobile apps don't break.
//
// Sprint 3 PR-21a added a parallel REST signalling channel
// (`webrtc.go`) that handles the canonical "perfect negotiation"
// WebRTC peer-connection state machine (new → connecting →
// connected → closed/failed) and aggregates ICE candidates per
// peer hash. The WebSocket channel remains for backward
// compatibility with Sprint 1+2 mobile apps; new clients should
// use the /api/v1/webrtc/{offer,answer,ice,config} endpoints.
//
// Rate limiting (RISKS §F25): a simple token-bucket per
// session_id caps ICE-candidate bursts so a malicious peer can't
// flood the signalling channel with candidates and exfiltrate the
// peer-reflexive address via the relayed packets. SDP/answer
// frames are not rate-limited (there's at most one of each per
// session by definition). The REST handler in webrtc.go applies
// a parallel sliding-window cap on candidates per peer (50).
package matching

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Sentinel errors.
var (
	// ErrPeerNotConnected is returned by Hub.Send when the
	// recipient has no live inbox (WebSocket not yet open, or
	// already closed).
	ErrPeerNotConnected = errors.New("matching: peer not connected")

	// ErrInboxFull is returned by Hub.Send when the recipient's
	// inbox channel is at capacity. Callers may either back off
	// and retry or surface it as a transient error to the
	// mobile app.
	ErrInboxFull = errors.New("matching: peer inbox full")

	// ErrRateLimited is returned by Handler when the session has
	// exceeded its ICE-candidate rate budget. The handler closes
	// the connection so the peer doesn't continue flooding.
	ErrRateLimited = errors.New("matching: session rate-limited")

	// ErrInvalidEnvelope is returned when a frame fails JSON
	// decoding or schema validation (missing required field,
	// unknown type, etc.).
	ErrInvalidEnvelope = errors.New("matching: invalid envelope")

	// ErrUnregistered is returned when a frame's `from` hash
	// doesn't match the connection's registered hash (auth
	// failure — someone is trying to spoof another device).
	ErrUnregistered = errors.New("matching: device not registered for this connection")
)

// Message-type constants — kept as untyped strings to match the
// `enum` in shared/schemas/p2p-signalling.schema.json.
const (
	TypeJoin          = "join"
	TypeLeave         = "leave"
	TypeOffer         = "offer"
	TypeAnswer        = "answer"
	TypeICECandidate  = "ice_candidate"
	TypeRenegotiate   = "renegotiate"
	TypeBye           = "bye"
	TypeError         = "error"
)

// Envelope is the wire format for one signalling message. JSON
// field tags line up with shared/schemas/p2p-signalling.schema.json
// so a successful unmarshal here means schema validation would
// also pass (modulo the `oneOf` on payload, which we don't
// re-validate in Sprint 1 — base64 strings survive arbitrary
// transports).
//
// Privacy: Payload is json.RawMessage so we don't second-guess
// its contents. The Handler never inspects Payload beyond
// re-encoding it back to the recipient. Per RISKS §F25 the
// payload is opaque base64 — any cleartext PII would already be
// inside the encrypted WebRTC data channel and never reach the
// signalling layer.
type Envelope struct {
	Type      string          `json:"type"`
	From      string          `json:"from"`
	To        string          `json:"to"`
	SessionID string          `json:"session_id"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// Validate enforces the must-have fields per the schema's
// `required` array. The schema's `enum` constraint on Type is
// also enforced here so the Hub / Handler can short-circuit
// garbage frames before they reach Send.
//
// Note: the schema's `oneOf` on Payload (SDP vs ICE candidate)
// is NOT validated here — the handler is schema-agnostic in
// Sprint 1 and the two mobile ends agree on payload shape by
// shared knowledge of the message Type.
func (e *Envelope) Validate() error {
	if e.Type == "" {
		return fmt.Errorf("%w: missing type", ErrInvalidEnvelope)
	}
	switch e.Type {
	case TypeJoin, TypeLeave, TypeOffer, TypeAnswer,
		TypeICECandidate, TypeRenegotiate, TypeBye, TypeError:
		// OK
	default:
		return fmt.Errorf("%w: unknown type %q", ErrInvalidEnvelope, e.Type)
	}
	if len(e.From) < 16 || len(e.From) > 64 {
		return fmt.Errorf("%w: from length out of range", ErrInvalidEnvelope)
	}
	if len(e.To) > 0 && (len(e.To) < 16 || len(e.To) > 64) {
		return fmt.Errorf("%w: to length out of range", ErrInvalidEnvelope)
	}
	if e.SessionID == "" {
		return fmt.Errorf("%w: missing session_id", ErrInvalidEnvelope)
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("%w: missing timestamp", ErrInvalidEnvelope)
	}
	return nil
}

// Incoming is the per-device inbox. The Hub hands one to each
// connection that successfully registers. C() returns the
// receive-only channel that the connection's write-loop drains.
type Incoming interface {
	C() <-chan Envelope
	Close() error
}

// Relay is the abstract message router the Handler depends on.
// Implementations: in-process Hub (Sprint 1), Redis pub/sub
// (Sprint 3+ for multi-instance backend).
//
// Contract:
//
//   - Register(hash) MUST be called exactly once per WebSocket
//     connection before any Send targeting that hash will work.
//     It returns an Incoming that yields envelopes addressed to
//     `hash` until Close is called.
//
//   - Send is non-blocking. ErrPeerNotConnected means the
//     recipient isn't registered (yet) — the caller should
//     decide whether to wait, queue, or surface the error.
//
//   - Send may return ErrInboxFull if the recipient's channel
//     is saturated. The caller decides whether to drop the
//     message (Sprint 1 policy: ICE candidates are
//     drop-tolerant; SDP/answer are not — see Handler).
type Relay interface {
	Register(ctx context.Context, hash string) (Incoming, error)
	Send(ctx context.Context, toHash string, env Envelope) error
}

// Compile-time check.
var _ Relay = (*Hub)(nil)

// inboxCapacity bounds the per-device inbox. Each unconsumed
// envelope takes ~200 bytes; 64 envelopes = ~12 KB per device,
// which is plenty for a SDP + a handful of ICE candidates and
// small enough that a stalled consumer doesn't balloon memory.
const inboxCapacity = 64

// Hub is the in-process Relay implementation. Safe for
// concurrent use. Holds no goroutines of its own — the inbox
// channels are drained by the per-connection write loops the
// Handler spawns.
type Hub struct {
	mu      sync.Mutex
	inboxes map[string]*hubInbox
}

// hubInbox is the concrete inbox record stored in the Hub map.
type hubInbox struct {
	ch     chan Envelope
	closed bool
}

func (i *hubInbox) C() <-chan Envelope { return i.ch }

// Close marks the inbox as closed. Subsequent Send calls return
// ErrPeerNotConnected; subsequent Register calls with the same
// hash return a fresh inbox.
func (i *hubInbox) Close() error {
	if i == nil {
		return nil
	}
	if !i.closed {
		i.closed = true
		close(i.ch)
	}
	return nil
}

// NewHub returns an empty Hub. No background goroutines are
// spawned.
func NewHub() *Hub {
	return &Hub{
		inboxes: make(map[string]*hubInbox),
	}
}

// Register attaches a fresh inbox to `hash` and returns it.
// If `hash` is already registered, the previous inbox is
// closed (its pending messages are drained by the old
// connection's write loop until it sees the closed channel).
//
// The hash is the device-id-hash from PR-2's auth layer — the
// connection's identity for the lifetime of the WebSocket.
func (h *Hub) Register(ctx context.Context, hash string) (Incoming, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(hash) < 16 || len(hash) > 64 {
		return nil, fmt.Errorf("matching: Register: hash length out of range")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if prev, ok := h.inboxes[hash]; ok {
		// Idempotency: caller is reconnecting. Close the old
		// inbox so the old connection's write loop exits.
		_ = prev.Close()
	}
	box := &hubInbox{ch: make(chan Envelope, inboxCapacity)}
	h.inboxes[hash] = box
	return box, nil
}

// Send delivers env to the inbox registered under toHash.
// Returns ErrPeerNotConnected if no inbox exists; ErrInboxFull
// if the channel is at capacity. Envelopes addressed to a
// closed inbox are silently dropped (the recipient's
// connection dropped; nothing to do).
func (h *Hub) Send(ctx context.Context, toHash string, env Envelope) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	h.mu.Lock()
	box, ok := h.inboxes[toHash]
	h.mu.Unlock()
	if !ok {
		return ErrPeerNotConnected
	}
	select {
	case box.ch <- env:
		return nil
	default:
		return ErrInboxFull
	}
}

// Unregister removes the inbox for `hash`. Called by the
// Handler's cleanup goroutine when the WebSocket closes.
func (h *Hub) Unregister(hash string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if box, ok := h.inboxes[hash]; ok {
		_ = box.Close()
		delete(h.inboxes, hash)
	}
}

// ConnectedCount returns the current number of registered
// inboxes — for /healthz + tests.
func (h *Hub) ConnectedCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.inboxes)
}

// IsConnected is a convenience for tests / health checks.
func (h *Hub) IsConnected(hash string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, ok := h.inboxes[hash]
	return ok
}

// Handler is the http.Handler that bridges a WebSocket
// connection to a Hub.
//
// Wire protocol:
//
//   - Client opens WS to `/ws/v1/signalling?device=<hash>`.
//   - Handler upgrades, registers an inbox for `<hash>`, then
//     runs two goroutines:
//       * readLoop  — decode Envelope frames, route via Hub.Send
//       * writeLoop — drain the inbox, encode as JSON, write
//   - Either loop returning terminates the other (via
//     conn.Close) and unregisters the inbox.
//
// ICE rate-limit (RISKS §F25): a token bucket per session_id
// caps TypeICECandidate frames at `iceBurst` immediately and
// `iceRefill` per second thereafter. SDP/answer frames are
// never throttled (one per session by definition).
type Handler struct {
	Hub       Relay
	Upgrader  websocket.Upgrader
	iceBurst  int
	iceRefill float64 // tokens per second
}

// NewHandler returns a Handler wired to the given Hub with
// sensible defaults. iceBurst=20, iceRefill=10/s — a real
// WebRTC handshake sends ~5-15 ICE candidates total, so 20
// burst + 10/s refills gives generous headroom while still
// catching floods.
func NewHandler(hub Relay) *Handler {
	return &Handler{
		Hub:      hub,
		Upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			// Sprint 1: any origin. PR-7 will tighten this to
			// the production CORS allowlist (BRD NFR-9 +
			// DEPLOYMENT.md §CORS).
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		iceBurst:  20,
		iceRefill: 10,
	}
}

// SetICERate overrides the ICE-candidate token-bucket
// parameters. Returns the receiver for chaining. Mostly useful
// in tests; production should leave defaults.
func (h *Handler) SetICERate(burst int, refillPerSec float64) *Handler {
	if burst > 0 {
		h.iceBurst = burst
	}
	if refillPerSec > 0 {
		h.iceRefill = refillPerSec
	}
	return h
}

// tokenBucket is a minimal per-session ICE rate limiter.
// Lock-free on the hot path thanks to floating-point token
// arithmetic; safe because the only writers are the readLoop
// goroutine (one per WS, no shared state between sessions).
type tokenBucket struct {
	tokens float64
	max    float64
	rate   float64 // tokens per second
	last   time.Time
}

func newTokenBucket(max, rate float64) *tokenBucket {
	return &tokenBucket{tokens: max, max: max, rate: rate, last: time.Now()}
}

// allow returns true and consumes one token if there's at
// least one available, false otherwise. Refills proportionally
// to elapsed wall time.
func (tb *tokenBucket) allow() bool {
	now := time.Now()
	elapsed := now.Sub(tb.last).Seconds()
	tb.last = now
	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.max {
		tb.tokens = tb.max
	}
	if tb.tokens >= 1 {
		tb.tokens -= 1
		return true
	}
	return false
}

// ServeHTTP upgrades the request and runs the bridge until
// either side closes the connection.
//
// The `device` query parameter carries the device-id-hash
// (from PR-2's auth layer) and is the inbox key. It's the
// ONLY auth check in Sprint 1 — the Handler assumes the
// edge layer (Kong / Nginx, per DEPLOYMENT.md) has already
// authenticated the WebSocket upgrade. PR-7 will add a
// bearer-token check here.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hash := r.URL.Query().Get("device")
	if len(hash) < 16 || len(hash) > 64 {
		http.Error(w, "missing or invalid device hash", http.StatusBadRequest)
		return
	}

	conn, err := h.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade already wrote an HTTP error response.
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	inbox, err := h.Hub.Register(ctx, hash)
	if err != nil {
		_ = conn.WriteJSON(Envelope{
			Type:      TypeError,
			From:      "server",
			To:        hash,
			SessionID: "",
			Timestamp: time.Now().UTC(),
			Payload:   json.RawMessage(`{"reason":"register_failed"}`),
		})
		_ = conn.Close()
		return
	}
	// If Hub is the concrete *Hub, Unregister on disconnect
	// (best-effort type assertion — other Relay impls will
	// have their own cleanup story).
	if concrete, ok := h.Hub.(*Hub); ok {
		defer concrete.Unregister(hash)
	}

	// Per-session ICE rate limit buckets.
	buckets := make(map[string]*tokenBucket)
	var bucketsMu sync.Mutex
	getBucket := func(sessionID string) *tokenBucket {
		bucketsMu.Lock()
		defer bucketsMu.Unlock()
		tb, ok := buckets[sessionID]
		if !ok {
			tb = newTokenBucket(float64(h.iceBurst), h.iceRefill)
			buckets[sessionID] = tb
		}
		return tb
	}

	// writeLoop drains the inbox until the channel closes.
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		for env := range inbox.C() {
			// Set write deadline to bound a wedged peer.
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(env); err != nil {
				return
			}
		}
	}()

	// readLoop blocks on the connection. On exit it cancels
	// the inbox so writeLoop also exits.
	for {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var env Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			continue // silently drop garbage frames
		}
		if err := env.Validate(); err != nil {
			continue
		}
		// Auth: from must match this connection's hash.
		if env.From != hash {
			_ = conn.WriteJSON(Envelope{
				Type:      TypeError,
				From:      "server",
				To:        hash,
				SessionID: env.SessionID,
				Timestamp: time.Now().UTC(),
				Payload:   json.RawMessage(`{"reason":"unregistered"}`),
			})
			continue
		}
		// ICE rate-limit (RISKS §F25).
		if env.Type == TypeICECandidate {
			if !getBucket(env.SessionID).allow() {
				_ = conn.WriteJSON(Envelope{
					Type:      TypeError,
					From:      "server",
					To:        hash,
					SessionID: env.SessionID,
					Timestamp: time.Now().UTC(),
					Payload:   json.RawMessage(`{"reason":"rate_limited"}`),
				})
				break
			}
		}
		// Route to recipient. If To is empty we treat it as a
		// broadcast (e.g. a "leave" addressed to the session
		// itself); with a single-Hub Sprint 1 there's only one
		// peer per session, so the mobile app sets To
		// explicitly.
		if env.To != "" {
			if err := h.Hub.Send(ctx, env.To, env); err != nil {
				// Peer not connected — translate to a
				// peer_unavailable error frame so the
				// sender can fall back (Echo-Bot or
				// single-sided).
				_ = conn.WriteJSON(Envelope{
					Type:      TypeError,
					From:      "server",
					To:        hash,
					SessionID: env.SessionID,
					Timestamp: time.Now().UTC(),
					Payload:   json.RawMessage(`{"reason":"peer_unavailable"}`),
				})
			}
		}
	}

	// Closing the conn unblocks writeLoop's WriteJSON.
	_ = conn.Close()
	<-writeDone
}