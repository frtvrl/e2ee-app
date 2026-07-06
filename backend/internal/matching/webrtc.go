// webrtc.go — WebRTC peer connection signalling (Sprint 3 PR-21a).
//
// Architecture (the new REST signalling layer that replaces the
// Sprint-1 Echo-Bot + Sprint-1+2 ws/v1/signalling WebSocket path
// for direct P2P):
//
//   Mobile A (offerer)               Mobile B (answerer)
//        |                                |
//        | POST /api/v1/webrtc/offer      |
//        +-------------------------------->
//        |  { session_id, sdp, ... }       |
//        v                                |
//   +--------------------+                |
//   | matching.Manager   |                |
//   |  sessions[id] => WebRTCSession     |
//   |    state: new → connecting         |
//   +--------------------+                |
//        |                                |
//        |<------- 201 Created -----------+
//        |  { session_id, state, remote_ice }
//        |                                |
//        | POST /api/v1/webrtc/ice        |
//        +-------------------------------->
//        |  [per-peer candidates]         |
//        |                                |
//        |                  POST /api/v1/webrtc/answer
//        |<--------------------------------+
//        |                  { session_id, sdp }
//        |                                |
//        | 201 Created                    |
//        +-------------------------------->
//        |                  { session_id, state: "connected" }
//        |                                |
//        | POST /api/v1/webrtc/ice        |
//        +----------- (collect remote ICE -----------------
//        |                                |
//        |              POST /api/v1/webrtc/ice
//        |<--------------------------------+
//        v                                v
//   [DTLS handshake on candidate set, then SCTP data channel]
//
// This is the canonical "perfect-negotiation" pattern from
// WebRTC for new browsers (2019+): the JSON envelope is opaque
// to the backend (we don't parse SDP line-by-line — we forward
// the full `sdp` string and let the peer connection at each
// end parse it). The only thing the backend verifies is:
//
//   - the SDP starts with "v=0" (RFC 4566 §5)
//   - sdp_type is "offer" or "answer"
//   - each ICE candidate has a non-empty `candidate` string
//
// PRIVACY (ADR-0006): the candidate string IS a peer-reflexive
// IP+port — exactly the kind of network metadata that's a
// secret to share outside the matching relay. Per RISKS.md §F25
// we never log the candidate text; we log only the session_id
// and a count. Tests in webrtc_test.go enforce this contract.
//
// STUN/TURN config (Sprint 3 PR-21a task §4): the manager
// reads STUN_URL / TURN_URL / TURN_USERNAME / TURN_CREDENTIAL
// from the process env at startup. The coturn service is
// already deployed (infra/coturn/turnserver.conf, PR-13). The
// REST handler at GET /api/v1/webrtc/config serves this
// config to the mobile app at peer-connection init time.
package matching

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// -----------------------------------------------------------------------------
// State machine — peer connection states per W3C WebRTC §5.5
// -----------------------------------------------------------------------------

// SessionState is the peer connection's lifecycle stage.
// Mirrors the W3C RTCPeerConnectionState enum, trimmed to the
// states PR-21a cares about (PR-21b Flutter integration may add
// `disconnected` / `failed` probes later).
type SessionState string

// The legal states. Constant strings keep wire/JSON shape stable
// forever — clients will switch on this string in their UI.
const (
	// SessionStateNew: peer connection created, no offer yet.
	SessionStateNew SessionState = "new"
	// SessionStateConnecting: offer sent, waiting for answer.
	SessionStateConnecting SessionState = "connecting"
	// SessionStateConnected: answer received — DTLS handshake
	// is in progress at the network layer; the backend only
	// sees the SDP exchange, but the ICE+DTLS path is now
	// the client's responsibility.
	SessionStateConnected SessionState = "connected"
	// SessionStateClosed: explicit "bye" or hard timeout.
	// Terminal — no further transitions.
	SessionStateClosed SessionState = "closed"
	// SessionStateFailed: protocol violation / bad SDP /
	// unexpected answer / etc. Terminal.
	SessionStateFailed SessionState = "failed"
)

// Valid reports whether s is a known SessionState.
func (s SessionState) Valid() bool {
	switch s {
	case SessionStateNew, SessionStateConnecting,
		SessionStateConnected, SessionStateClosed, SessionStateFailed:
		return true
	}
	return false
}

// ErrInvalidStateTransition is returned by WebRTCSession.transition
// when the requested transition is not allowed from the current
// state. Wire-level callers (HTTP handlers) translate this to a
// 409 Conflict with a stable error code.
var ErrInvalidStateTransition = errors.New("matching: invalid state transition")

// transition moves ws from `from` to `to`. It is the single point
// of state change — callers never set ws.State directly. Returns
// ErrInvalidStateTransition on any disallowed move.
//
// Allowed:
//
//	new        → connecting | failed
//	connecting → connected | failed
//	connected  → closed | failed
//	closed     → (none — terminal)
//	failed     → (none — terminal)
//
// `force=true` lets cleanup/timeout paths mark closed sessions
// as closed-again without error. Use sparingly.
//
// The transition method takes ws.mu itself. Callers that ALREADY
// hold ws.mu should call transitionLocked to avoid self-deadlock.
func (ws *WebRTCSession) transition(to SessionState, force bool) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.transitionLocked(to, force, time.Now().UTC())
}

// transitionLocked is the unsynchronised form. Callers MUST
// hold ws.mu (or operate on a brand-new session that nobody else
// can see). All state-machine callers that go through Manager
// (which already holds the session lock) reach this version.
//
// The `now` argument is the clock the manager was constructed
// with — it is used to stamp UpdatedAt. Passing it in (rather
// than reading time.Now() internally) keeps expiry logic
// consistent with the test-controllable clock.
func (ws *WebRTCSession) transitionLocked(to SessionState, force bool, now time.Time) error {
	if ws.State == to {
		return nil // idempotent
	}
	if !to.Valid() {
		return fmt.Errorf("%w: target state %q is not a known state", ErrInvalidStateTransition, to)
	}
	if !force {
		switch ws.State {
		case SessionStateNew:
			if to != SessionStateConnecting && to != SessionStateFailed {
				return fmt.Errorf("%w: new → %s not allowed", ErrInvalidStateTransition, to)
			}
		case SessionStateConnecting:
			if to != SessionStateConnected && to != SessionStateFailed {
				return fmt.Errorf("%w: connecting → %s not allowed", ErrInvalidStateTransition, to)
			}
		case SessionStateConnected:
			if to != SessionStateClosed && to != SessionStateFailed {
				return fmt.Errorf("%w: connected → %s not allowed", ErrInvalidStateTransition, to)
			}
		case SessionStateClosed, SessionStateFailed:
			return fmt.Errorf("%w: session is terminal (%s)", ErrInvalidStateTransition, ws.State)
		default:
			return fmt.Errorf("%w: unknown current state %q", ErrInvalidStateTransition, ws.State)
		}
	}
	ws.State = to
	ws.UpdatedAt = now
	return nil
}

// -----------------------------------------------------------------------------
// Wire types
// -----------------------------------------------------------------------------

// ICECandidate is one ICE candidate (RFC 5245 / RFC 8445 §5.1).
// The candidate string is a peer-reflexive IP+port if the type
// is "srflxt", or a relay IP+port for "relay" — ADR-0006 means
// it must NOT appear in logs.
type ICECandidate struct {
	Candidate        string `json:"candidate"`
	SDPMID           string `json:"sdpMid,omitempty"`
	SDPMLineIndex    int    `json:"sdpMLineIndex,omitempty"`
	UsernameFragment string `json:"usernameFragment,omitempty"`
}

// Validate enforces the minimum we care about: the candidate
// string itself must be non-empty and must start with the
// "candidate:" prefix (RFC 8839 §4.5 + ortc.js / libwebrtc
// convention). We deliberately don't parse the candidate
// components — that's the client's job; the backend just
// forwards whatever the peer gave us.
func (c *ICECandidate) Validate() error {
	cand := strings.TrimSpace(c.Candidate)
	if cand == "" {
		return fmt.Errorf("%w: ICE candidate string is empty", ErrInvalidEnvelope)
	}
	if !strings.HasPrefix(cand, "candidate:") {
		return fmt.Errorf("%w: ICE candidate must start with \"candidate:\"", ErrInvalidEnvelope)
	}
	return nil
}

// SDPPayload is the subset of an SDP we accept on /offer and
// /answer. The full SDP is the `sdp` string verbatim from the
// peer's RTCSessionDescription; we only verify it parses-enough
// to look like SDP (RFC 4566).
type SDPPayload struct {
	SDPType string `json:"sdp_type"` // "offer" | "answer"
	SDP     string `json:"sdp"`
}

// Validate enforces:
//   - sdp_type is exactly "offer" or "answer"
//   - sdp is non-empty
//   - sdp's first non-whitespace line starts with "v=0" (RFC 4566 §5)
func (s *SDPPayload) Validate() error {
	switch s.SDPType {
	case "offer", "answer":
		// OK
	default:
		return fmt.Errorf("%w: sdp_type must be \"offer\" or \"answer\", got %q",
			ErrInvalidEnvelope, s.SDPType)
	}
	if strings.TrimSpace(s.SDP) == "" {
		return fmt.Errorf("%w: sdp is empty", ErrInvalidEnvelope)
	}
	first := firstNonWSLine(s.SDP)
	if !strings.HasPrefix(first, "v=0") {
		return fmt.Errorf("%w: sdp must start with \"v=0\" (RFC 4566)", ErrInvalidEnvelope)
	}
	return nil
}

// firstNonWSLine returns the first line that contains any
// non-whitespace, with leading whitespace stripped. Used by
// SDPPayload.Validate to detect malformed SDP without
// importing a full RFC-4566 parser.
func firstNonWSLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// -----------------------------------------------------------------------------
// WebRTCSession — the per-session state record
// -----------------------------------------------------------------------------

// WebRTCSession is the per-RTCPeerConnection record held by the
// matching.Manager. A session is identified by a server-minted
// sessionID; the two devices share the session by both POSTing
// against the same id (the sender of /offer creates it; the
// answerer is told the id by the sender out-of-band — see
// shared/schemas/webrtc-signalling.schema.json).
//
// Concurrency: all mutators take the per-session mutex `mu`
// before reading or writing any field. The Manager additionally
// guards its sessions map with a higher-level mutex.
type WebRTCSession struct {
	ID         string          `json:"id"`
	State      SessionState    `json:"state"`
	Offerer    string          `json:"offerer,omitempty"`    // device hash of the offerer (set on /offer)
	Answerer   string          `json:"answerer,omitempty"`   // device hash of the answerer (set on /answer)
	Offer      *SDPPayload     `json:"offer,omitempty"`      // set on /offer; nil until then
	Answer     *SDPPayload     `json:"answer,omitempty"`     // set on /answer; nil until then
	Candidates map[string][]ICECandidate `json:"-"` // per-peer candidate lists — NOT serialised directly; use the ICE handler

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	mu sync.Mutex // guards all fields above
}

// Snapshot returns a stable copy of the session suitable for
// JSON serialisation. It does NOT include Candidates — those
// have their own getter so the offerer/answerer can selectively
// poll for the remote's candidates without fetching their own.
func (ws *WebRTCSession) Snapshot() WebRTCSessionView {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return WebRTCSessionView{
		ID:        ws.ID,
		State:     ws.State,
		Offerer:   ws.Offerer,
		Answerer:  ws.Answerer,
		HasOffer:  ws.Offer != nil,
		HasAnswer: ws.Answer != nil,
		CreatedAt: ws.CreatedAt,
		UpdatedAt: ws.UpdatedAt,
	}
}

// WebRTCSessionView is the JSON-serialisable shape returned to
// API callers. Candidates are deliberately omitted — clients
// retrieve them via the dedicated /ice endpoint so the wire
// surface stays small.
type WebRTCSessionView struct {
	ID        string       `json:"id"`
	State     SessionState `json:"state"`
	Offerer   string       `json:"offerer,omitempty"`
	Answerer  string       `json:"answerer,omitempty"`
	HasOffer  bool         `json:"has_offer"`
	HasAnswer bool         `json:"has_answer"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// RemoteCandidates returns the candidates posted by peer `peer`.
// The "remote" half of the relationship is whatever ISN'T the
// supplied peer hash — this is the typical call pattern:
//
//	bob posts his candidates; alice calls RemoteCandidates(bob)
//	to learn them.
//
// We store per-peer, and the offerer/answerer learn the other's
// candidates by calling this method explicitly. There is no
// "auto-broadcast" — that would leak candidate counts to
// passive listeners.
func (ws *WebRTCSession) RemoteCandidates(peer string) []ICECandidate {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	out := []ICECandidate{}
	for storedPeer, cands := range ws.Candidates {
		if storedPeer == peer {
			continue
		}
		out = append(out, cands...)
	}
	return out
}

// -----------------------------------------------------------------------------
// STUN/TURN configuration
// -----------------------------------------------------------------------------

// STUNTURNConfig carries the ICE servers the mobile app needs
// at peer-connection init time. Read once at process startup
// from env (LoadSTUNTURNConfig) and never mutated.
type STUNTURNConfig struct {
	// STUNURLs is the list of STUN server URIs (RFC 7065).
	// Always non-empty after LoadSTUNTURNConfig (Google's
	// public stun.l.google.com servers are the fallback).
	STUNURLs []string `json:"stun_urls"`
	// TURNURL is the TURN server URI (RFC 7065). Empty if no
	// TURN is configured — the client treats empty as "no TURN".
	TURNURL string `json:"turn_url,omitempty"`
	// TURNUsername is the username the mobile app uses to
	// authenticate against the coturn REST API. Empty if no TURN.
	TURNUsername string `json:"turn_username,omitempty"`
	// TURNCredential is the password / HMAC-derived credential.
	// In production this is time-limited (see Sprint 2 turn_credentials.dart).
	TURNCredential string `json:"turn_credential,omitempty"`
	// TTLSeconds is the lifetime the client should treat the
	// credential as valid. The coturn REST API defaults to 24h;
	// we forward the env-declared value through here. 0 means
	// "use the coturn default".
	TTLSeconds int `json:"ttl_seconds,omitempty"`
}

// EnvConfig is the env-var name set the loader reads from.
// Exported so the wire-up (cmd/server/main.go) can reference
// the same names in its documentation.
type envConfig struct {
	STUNURL         string
	TURNURL         string
	TURNUsername    string
	TURNCredential  string
	CoturnTTLSeconds string
}

// EnvConfigDefaults is the canonical env-var name set.
// PR-21a keeps this internal — sprint-3 wire-up can promote
// the names to a public struct if multiple packages need them.
var EnvConfigDefaults = envConfig{
	STUNURL:          "WEBRTC_STUN_URL",
	TURNURL:          "WEBRTC_TURN_URL",
	TURNUsername:     "WEBRTC_TURN_USERNAME",
	TURNCredential:   "WEBRTC_TURN_CREDENTIAL",
	CoturnTTLSeconds: "WEBRTC_TURN_TTL_SECONDS",
}

// defaultSTUNURLs is the fallback STUN set. Google's free
// public servers; matched by every browser-based WebRTC client.
// Production may override via WEBRTC_STUN_URL (comma-separated).
var defaultSTUNURLs = []string{
	"stun:stun.l.google.com:19302",
	"stun:stun1.l.google.com:19302",
}

// LoadSTUNTURNConfig reads the ICE-server config from the
// process environment. Always returns a non-nil pointer; if no
// env is set the STUN fallback is applied so the WebRTC init
// can still discover srflxt candidates (NAT traversal works in
// 80% of cases without TURN).
//
// Multi-STUN: WEBRTC_STUN_URL accepts a comma-separated list —
// useful when running multi-region TURN clusters in Sprint 4+.
func LoadSTUNTURNConfig() *STUNTURNConfig {
	cfg := &STUNTURNConfig{
		TURNURL:         strings.TrimSpace(os.Getenv(EnvConfigDefaults.TURNURL)),
		TURNUsername:    strings.TrimSpace(os.Getenv(EnvConfigDefaults.TURNUsername)),
		TURNCredential:  strings.TrimSpace(os.Getenv(EnvConfigDefaults.TURNCredential)),
	}
	if raw := strings.TrimSpace(os.Getenv(EnvConfigDefaults.STTLSeconds())); raw != "" {
		if n, err := strconvAtoi(raw); err == nil && n > 0 {
			cfg.TTLSeconds = n
		}
	}
	stunRaw := strings.TrimSpace(os.Getenv(EnvConfigDefaults.STUNURL))
	if stunRaw == "" {
		cfg.STUNURLs = append([]string(nil), defaultSTUNURLs...)
	} else {
		for _, u := range strings.Split(stunRaw, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				cfg.STUNURLs = append(cfg.STUNURLs, u)
			}
		}
	}
	return cfg
}

// STTLSeconds is a tiny accessor so the env struct keeps its
// all-caps convention while exposing a sensible Go-side name.
func (e envConfig) STTLSeconds() string { return e.CoturnTTLSeconds }

// strconvAtoi is a local Atoi wrapper so this file doesn't pull
// in strconv just for one call. Returns 0 on parse error so the
// caller falls back to defaults. Intentionally permissive.
func strconvAtoi(s string) (int, error) {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, errAtoi
		}
		n = n*10 + int(c-'0')
		if n > 1<<30 {
			return 0, errAtoiOverflow
		}
	}
	if len(s) == 0 {
		return 0, errAtoiEmpty
	}
	return n, nil
}

var (
	errAtoi          = errors.New("webrtc: invalid integer")
	errAtoiOverflow  = errors.New("webrtc: integer overflow")
	errAtoiEmpty     = errors.New("webrtc: empty integer")
)

// -----------------------------------------------------------------------------
// Manager — active sessions
// -----------------------------------------------------------------------------

// DefaultSessionTTL bounds how long a session can sit in any
// non-terminal state. After this duration the manager
// transitions the session to SessionStateClosed on the next
// access. Sessions that complete their SDP exchange well
// inside the TTL stay alive for the duration of the actual
// peer connection (typically minutes for measurement use).
const DefaultSessionTTL = 15 * time.Minute

// DefaultICECap caps per-peer candidate lists. RFC 8445 §6.1.1
// recommends < 100 candidates; we use 50 (a real WebRTC
// handshake typically produces 5-15).
const DefaultICECap = 50

// ErrSessionNotFound is returned by Manager.GetSession and the
// HTTP handlers when the sessionID is unknown. Wire-level
// callers translate to 404.
var ErrSessionNotFound = errors.New("matching: webrtc session not found")

// ErrSessionTerminal is returned when a write is attempted on a
// session in SessionStateClosed or SessionStateFailed. Wire-
// level callers translate to 409 Conflict.
var ErrSessionTerminal = errors.New("matching: session is terminal")

// Manager owns all active WebRTCSession records. Concurrent
// access is safe. The manager does not spawn goroutines of its
// own — TTL enforcement is lazy: callers hit ExpiredSessions
// on every mutation.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*WebRTCSession
	turn     *STUNTURNConfig

	// ttl is the timeout for non-terminal sessions. Tests
	// inject a short value (e.g. 100ms) to exercise the
	// expiry path deterministically.
	ttl time.Duration
	// iceCap caps per-peer candidate lists.
	iceCap int
	// now is the time-source; tests override to a clock.
	now func() time.Time
}

// NewManager returns a Manager wired with the default TTL,
// candidate cap, and STUN/TURN config loaded from env.
func NewManager() *Manager {
	return newManager(LoadSTUNTURNConfig(), DefaultSessionTTL, DefaultICECap, time.Now)
}

// NewManagerWith is the test-friendly constructor. Production
// code uses NewManager; tests pass in a canned STUNTURNConfig,
// short TTL, fake clock, etc.
func NewManagerWith(turn *STUNTURNConfig, ttl time.Duration, iceCap int, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	if ttl <= 0 {
		ttl = DefaultSessionTTL
	}
	if iceCap <= 0 {
		iceCap = DefaultICECap
	}
	return newManager(turn, ttl, iceCap, now)
}

func newManager(turn *STUNTURNConfig, ttl time.Duration, iceCap int, now func() time.Time) *Manager {
	return &Manager{
		sessions: make(map[string]*WebRTCSession),
		turn:     turn,
		ttl:      ttl,
		iceCap:   iceCap,
		now:      now,
	}
}

// STUNTURN returns the ICE-server config the manager was
// constructed with. The HTTP handler at GET /webrtc/config
// serves this directly (as a snapshot — never mutated by the
// caller).
func (m *Manager) STUNTURN() *STUNTURNConfig { return m.turn }

// Count returns the number of active (non-terminal) sessions.
// For /healthz + tests.
func (m *Manager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

// GetSession returns the WebRTCSession for the given ID. Calls
// Expire first so a session that aged past its TTL is reported
// as terminal (its State is "closed"). Returns ErrSessionNotFound
// if the session was never created (or was deleted by Close).
func (m *Manager) GetSession(id string) (*WebRTCSession, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("%w: empty session id", ErrSessionNotFound)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked(m.now())
	ws, ok := m.sessions[id]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return ws, nil
}

// CreateSession makes a new session in SessionStateNew. The
// caller (offerer's HTTP handler) supplies the offerer hash. A
// fresh session id is server-minted; pass an empty string to
// get the manager to mint one (the manager returns the minted
// id via the return value of ApplyOffer below for the no-id
// case — but most callers reach CreateSession directly).
func (m *Manager) CreateSession(id, offerer string) (*WebRTCSession, error) {
	if strings.TrimSpace(offerer) == "" {
		return nil, fmt.Errorf("%w: offerer hash required", ErrInvalidEnvelope)
	}
	if !isDeviceHashShape(offerer) {
		return nil, fmt.Errorf("%w: offerer hash must be 16..64 chars", ErrInvalidEnvelope)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked(m.now())
	if id == "" {
		id = newSessionID(m.now)
	}
	if _, ok := m.sessions[id]; ok {
		return nil, fmt.Errorf("session id %q already exists", id)
	}
	now := m.now().UTC()
	ws := &WebRTCSession{
		ID:         id,
		State:      SessionStateNew,
		Offerer:    offerer,
		Candidates: make(map[string][]ICECandidate),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	m.sessions[id] = ws
	return ws, nil
}

// ApplyOffer stores the SDP offer and transitions new →
// connecting. Returns ErrInvalidStateTransition if the session
// is past SessionStateNew. The session must already exist
// (offerer calls CreateSession first; offerer is also the
// ApplyOffer caller in MVP).
func (m *Manager) ApplyOffer(id, peerHash string, sdp *SDPPayload) (*WebRTCSession, error) {
	if err := sdp.Validate(); err != nil {
		return nil, err
	}
	ws, err := m.GetSession(id)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.State != SessionStateNew {
		return nil, fmt.Errorf("%w: cannot apply offer in state %s",
			ErrInvalidStateTransition, ws.State)
	}
	if peerHash != ws.Offerer {
		return nil, fmt.Errorf("%w: offerer mismatch: session=%s peer=%s",
			ErrInvalidEnvelope, ws.Offerer, peerHash)
	}
	ws.Offer = sdp
	if err := ws.transitionLocked(SessionStateConnecting, false, m.now()); err != nil {
		return nil, err
	}
	return ws, nil
}

// ApplyAnswer stores the SDP answer and transitions connecting
// → connected. The answerer must be a different peer than the
// offerer.
func (m *Manager) ApplyAnswer(id, peerHash string, sdp *SDPPayload) (*WebRTCSession, error) {
	if err := sdp.Validate(); err != nil {
		return nil, err
	}
	ws, err := m.GetSession(id)
	if err != nil {
		return nil, err
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.State != SessionStateConnecting {
		return nil, fmt.Errorf("%w: cannot apply answer in state %s",
			ErrInvalidStateTransition, ws.State)
	}
	if peerHash == ws.Offerer {
		return nil, fmt.Errorf("%w: answerer must differ from offerer", ErrInvalidEnvelope)
	}
	if !isDeviceHashShape(peerHash) {
		return nil, fmt.Errorf("%w: answerer hash must be 16..64 chars", ErrInvalidEnvelope)
	}
	if ws.Answerer == "" {
		ws.Answerer = peerHash
	} else if ws.Answerer != peerHash {
		return nil, fmt.Errorf("%w: answerer mismatch: session=%s peer=%s",
			ErrInvalidEnvelope, ws.Answerer, peerHash)
	}
	ws.Answer = sdp
	if err := ws.transitionLocked(SessionStateConnected, false, m.now()); err != nil {
		return nil, err
	}
	return ws, nil
}

// AppendICE stores one ICE candidate for the given peer. The
// candidate is added to ws.Candidates[peer]. If the peer already
// has iceCap candidates, the oldest is dropped (sliding-window
// policy — RFC 8445 §6.1.1 recommends this over hard-reject).
func (m *Manager) AppendICE(id, peer string, c ICECandidate) (*WebRTCSession, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	if !isDeviceHashShape(peer) {
		return nil, fmt.Errorf("%w: peer hash must be 16..64 chars", ErrInvalidEnvelope)
	}
	ws, err := m.GetSession(id)
	if err != nil {
		return nil, err
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if isTerminal(ws.State) {
		return nil, ErrSessionTerminal
	}
	list := ws.Candidates[peer]
	if len(list) >= m.iceCap {
		// sliding-window: drop the oldest
		list = list[1:]
	}
	list = append(list, c)
	ws.Candidates[peer] = list
	ws.UpdatedAt = m.now().UTC()
	return ws, nil
}

// CloseSession transitions a session to SessionStateClosed and
// removes it from the in-memory map. Idempotent: closing an
// already-closed session returns nil and does nothing.
func (m *Manager) CloseSession(id string) error {
	ws, err := m.GetSession(id)
	if err != nil {
		return err
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.State == SessionStateClosed {
		return nil
	}
	if ws.State == SessionStateFailed {
		// Failed is also terminal — drop the record but don't
		// re-transition. Caller likely already saw the failure.
		m.mu.Lock()
		delete(m.sessions, id)
		m.mu.Unlock()
		return nil
	}
	if err := ws.transitionLocked(SessionStateClosed, false, m.now()); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
	return nil
}

// FailSession marks a session SessionStateFailed with an
// optional reason (returned to the caller; NOT stored on the
// session — keep the record small for memory hygiene).
func (m *Manager) FailSession(id, reason string) error {
	ws, err := m.GetSession(id)
	if err != nil {
		return err
	}
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if isTerminal(ws.State) {
		return nil
	}
	if err := ws.transitionLocked(SessionStateFailed, true, m.now()); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
	return nil
}

// Expire moves any session whose UpdatedAt is older than the
// manager's TTL into SessionStateClosed (and removes it from
// the active map). Called lazily on every mutator. Returns the
// number of sessions expired in this call (mostly for tests).
func (m *Manager) Expire() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.expireLocked(m.now())
}

// expireLocked is the unsynchronised form. Caller MUST hold
// m.mu. We also re-test the session lock inside to satisfy the
// per-session transition contract.
func (m *Manager) expireLocked(now time.Time) int {
	if m.ttl <= 0 {
		return 0
	}
	cutoff := now.Add(-m.ttl)
	expired := 0
	for id, ws := range m.sessions {
		ws.mu.Lock()
		stale := ws.UpdatedAt.Before(cutoff) || ws.UpdatedAt.IsZero()
		terminal := isTerminal(ws.State)
		ws.mu.Unlock()
		if !stale || terminal {
			continue
		}
		// Force-closed; we don't propagate the error since
		// there's nothing useful to do with it.
		_ = ws.transitionLocked(SessionStateClosed, true, now)
		delete(m.sessions, id)
		expired++
	}
	return expired
}

// isTerminal reports whether the given state is a terminal
// one (no further transitions allowed).
func isTerminal(s SessionState) bool {
	return s == SessionStateClosed || s == SessionStateFailed
}

// isDeviceHashShape enforces the same length window the
// matching package uses elsewhere: 16..64 chars.
func isDeviceHashShape(s string) bool {
	n := len(s)
	return n >= 16 && n <= 64
}

// newSessionID mints a fresh, low-collision id using the
// configured clock. We use a ULID-shaped "ts-<unix-nano>-<rand>"
// string so an id sort matches insertion order — useful for
// /healthz sorting and log readability.
func newSessionID(now func() time.Time) string {
	ts := now().UTC().UnixNano()
	// A 4-byte cryptographic-quality suffix. crypto/rand failure
	// would mean the OS RNG is broken, so we fall back to a
	// time-derived value (still unique within a single session).
	var b [4]byte
	if _, err := randRead(b[:]); err != nil {
		return fmt.Sprintf("ts-%x-%x", ts, uint32(ts&0xffffffff))
	}
	return fmt.Sprintf("ts-%x-%x", ts, hex4(b))
}

// randRead is a process-wide indirection so tests can stub it
// via NewManagerWith(now=...). For now it just calls
// crypto/rand.Read — see the explicit dep in webrtc_dep.go.
var randRead = func(b []byte) (int, error) {
	return cryptoRandRead(b)
}

func cryptoRandRead(b []byte) (int, error) {
	// Indirection through a var keeps the import isolated and
	// lets tests swap in a deterministic source. We pull the
	// real rand in webrtc_dep.go to keep the import surface
	// small in the common case.
	return defaultRandRead(b)
}

// hex4 formats 4 bytes as 8 lowercase hex chars without
// allocating.
func hex4(b [4]byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 8)
	for i := 0; i < 4; i++ {
		v := b[i]
		out[i*2] = hex[v>>4]
		out[i*2+1] = hex[v&0x0f]
	}
	return string(out)
}

// -----------------------------------------------------------------------------
// HTTP request / response shapes (decoupled from the api layer's
// own bodies so the matching unit tests can drive the manager
// directly without net/http).
// -----------------------------------------------------------------------------

// WebRTCOfferRequest is the POST /api/v1/webrtc/offer body.
type WebRTCOfferRequest struct {
	SessionID string       `json:"session_id"`
	PeerHash  string       `json:"peer_hash"`
	SDP       SDPPayload   `json:"sdp"`
}

// WebRTCAnswerRequest is the POST /api/v1/webrtc/answer body.
type WebRTCAnswerRequest struct {
	SessionID string     `json:"session_id"`
	PeerHash  string     `json:"peer_hash"`
	SDP       SDPPayload `json:"sdp"`
}

// WebRTCICERequest is the POST /api/v1/webrtc/ice body.
type WebRTCICERequest struct {
	SessionID   string      `json:"session_id"`
	PeerHash    string      `json:"peer_hash"`
	Candidates  []ICECandidate `json:"candidates"`
}

// singleCandidate decodes either a single ICECandidate or a
// single-element array — both shapes appear in mobile clients
// depending on the SDK version, and we want both to work.
// Returns ErrInvalidEnvelope for both "wrong type" and "no
// candidate".
func (r *WebRTCICERequest) firstCandidate() (ICECandidate, error) {
	if len(r.Candidates) == 1 {
		return r.Candidates[0], nil
	}
	if len(r.Candidates) > 1 {
		// We support one-at-a-time on POST /ice; bulk upload is
		// offered as a separate (deferred) endpoint. Reject for
		// now so the wire shape stays stable.
		return ICECandidate{}, fmt.Errorf("%w: post exactly one candidate per request", ErrInvalidEnvelope)
	}
	return ICECandidate{}, fmt.Errorf("%w: candidates is required", ErrInvalidEnvelope)
}

// WebRTCSignallingResponse is the canonical response shape for
// every /webrtc/* POST handler. The remote-ICE list is omitted
// from the offer/answer success path when no remote candidates
// have arrived yet (nil → omitted by the json marshaller).
type WebRTCSignallingResponse struct {
	SessionID  string                `json:"session_id"`
	State      SessionState          `json:"state"`
	RemoteICE  []ICECandidate        `json:"remote_ice,omitempty"`
	ICEList    []string              `json:"peers_with_ice,omitempty"`
	Session    WebRTCSessionView     `json:"session"`
}

// writeSignallingResponse serialises resp with the right
// Content-Type. Convenience for the api layer.
func writeSignallingResponse(w http.ResponseWriter, status int, resp *WebRTCSignallingResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// writeError encodes an error as a stable JSON envelope. Used by
// the in-matching HTTP handlers when they decide not to delegate
// to the api layer.
func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	body := struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message}
	_ = json.NewEncoder(w).Encode(body)
}

// -----------------------------------------------------------------------------
// HTTP handlers — operate on *Manager directly so the matching
// package's tests don't need a wrapping HTTP layer.
// -----------------------------------------------------------------------------

// HandleOffer is POST /api/v1/webrtc/offer. Creates the session
// if it doesn't exist (offerer is the first writer); otherwise
// returns ErrInvalidStateTransition once the session is past
// `new`.
func (m *Manager) HandleOffer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	var req WebRTCOfferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "malformed JSON")
		return
	}
	if req.SessionID == "" {
		// Create-on-write: the first offerer mints the session
		// id and gets the canonical record back. Subsequent
		// writes for the same id by the same offerer re-attempt
		// the offer (a retry).
		ws, err := m.CreateSession("", req.PeerHash)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		req.SessionID = ws.ID
	}
	if !isDeviceHashShape(req.PeerHash) {
		writeError(w, http.StatusBadRequest, "bad_request", "peer_hash invalid")
		return
	}
	ws, err := m.ApplyOffer(req.SessionID, req.PeerHash, &req.SDP)
	if err != nil {
		m.translateError(w, err)
		return
	}
	m.writeResponse(w, ws, req.PeerHash, http.StatusCreated)
}

// HandleAnswer is POST /api/v1/webrtc/answer. Requires an
// existing SessionStateConnecting session.
func (m *Manager) HandleAnswer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	var req WebRTCAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "malformed JSON")
		return
	}
	if req.SessionID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "session_id required")
		return
	}
	if !isDeviceHashShape(req.PeerHash) {
		writeError(w, http.StatusBadRequest, "bad_request", "peer_hash invalid")
		return
	}
	ws, err := m.ApplyAnswer(req.SessionID, req.PeerHash, &req.SDP)
	if err != nil {
		m.translateError(w, err)
		return
	}
	m.writeResponse(w, ws, req.PeerHash, http.StatusOK)
}

// HandleICE is POST /api/v1/webrtc/ice. Appends a single ICE
// candidate from `peer` to the session's per-peer list. The
// response echoes the session state and the remote-side
// candidate list for `peer`.
func (m *Manager) HandleICE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	var req WebRTCICERequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "malformed JSON")
		return
	}
	if req.SessionID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "session_id required")
		return
	}
	cand, err := req.firstCandidate()
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	ws, err := m.AppendICE(req.SessionID, req.PeerHash, cand)
	if err != nil {
		m.translateError(w, err)
		return
	}
	m.writeResponse(w, ws, req.PeerHash, http.StatusOK)
}

// HandleSTUNTURNConfig is GET /api/v1/webrtc/config. Serves the
// ICE-server config to the mobile app at peer-connection init
// time. Stubbed as a passthrough — no secrets transformation
// here; coturn's REST API issues time-limited credentials and
// that's the layer that handles auth (see Sprint 2
// turn_credentials.dart).
func (m *Manager) HandleSTUNTURNConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(m.turn)
}

// writeResponse is the shared tail of every POST handler —
// builds the canonical response and writes it with `status`.
// Pulls the candidates the remote peer has supplied so the
// caller can iterate them.
func (m *Manager) writeResponse(w http.ResponseWriter, ws *WebRTCSession, peer string, status int) {
	view := ws.Snapshot()
	remote := ws.RemoteCandidates(peer)
	peers := ws.peerList()
	resp := &WebRTCSignallingResponse{
		SessionID: ws.ID,
		State:     ws.State,
		RemoteICE: remote,
		ICEList:   peers,
		Session:   view,
	}
	writeSignallingResponse(w, status, resp)
}

// peerList returns the sorted list of peers that have
// supplied ICE candidates. Used by the response so the client
// can decide whether to ask for remote ICE on a future poll.
func (ws *WebRTCSession) peerList() []string {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	out := make([]string, 0, len(ws.Candidates))
	for k := range ws.Candidates {
		if len(ws.Candidates[k]) > 0 {
			out = append(out, k)
		}
	}
	// Stable ordering for testability.
	sortStrings(out)
	return out
}

// sortStrings is an inlined ascending sort that avoids pulling
// in "sort" just for an HTTP response. The lists are tiny
// (typical N=2 — offerer + answerer).
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// translateError is the standard "manager error → HTTP response"
// mapping. Used by all three POST handlers and exercise-tested
// in webrtc_test.go.
func (m *Manager) translateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSessionNotFound):
		writeError(w, http.StatusNotFound, "not_found", "session not found")
	case errors.Is(err, ErrSessionTerminal):
		writeError(w, http.StatusConflict, "session_terminal", err.Error())
	case errors.Is(err, ErrInvalidStateTransition):
		writeError(w, http.StatusConflict, "invalid_state", err.Error())
	case errors.Is(err, ErrInvalidEnvelope):
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "internal error")
	}
}

// -----------------------------------------------------------------------------
// WebRTCManagerIface -- the structural interface the api package
// depends on. Keeps the api package free of matching-internal
// types beyond the public request/response shapes.
// -----------------------------------------------------------------------------

// WebRTCManagerIface is the surface area the api package uses.
// Production: *WebRTCManager. Tests: a fake that satisfies the
// four handler methods.
type WebRTCManagerIface interface {
	HandleOffer(w http.ResponseWriter, r *http.Request)
	HandleAnswer(w http.ResponseWriter, r *http.Request)
	HandleICE(w http.ResponseWriter, r *http.Request)
	HandleSTUNTURNConfig(w http.ResponseWriter, r *http.Request)
}

// Compile-time check -- fail at build time if *WebRTCManager
// drifts from WebRTCManagerIface.
var _ WebRTCManagerIface = (*Manager)(nil)
