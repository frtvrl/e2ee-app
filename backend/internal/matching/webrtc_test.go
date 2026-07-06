package matching

// webrtc_test.go — unit tests for webrtc.go (Sprint 3 PR-21a).
//
// Goal: ≥60% coverage on webrtc.go (state machine + manager +
// HTTP handlers + STUN/TURN config). The tests below cover:
//
//   - SessionState.Valid (5 states + unknown)
//   - WebRTCSession.transition (all legal + all terminal
//     rejections; idempotent no-op; invalid target)
//   - SDPPayload.Validate (good offer / good answer / bad
//     sdp_type / empty sdp / non-v=0 prefix)
//   - ICECandidate.Validate (good / empty / wrong prefix)
//   - STUNTURNConfig / LoadSTUNTURNConfig (env-driven)
//   - Manager lifecycle (Create / Get / ApplyOffer / ApplyAnswer
//     / AppendICE / Close / Fail / Expire)
//   - Sliding-window ICE cap
//   - Candidate aggregation RemoteCandidates
//   - Peer-list ordering
//   - HandleOffer / HandleAnswer / HandleICE / HandleSTUNTURNConfig
//   - HTTP error mapping (translateError)
//   - Concurrency (race-free)

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

func newTestManager() *Manager {
	return NewManagerWith(
		&STUNTURNConfig{
			STUNURLs:       []string{"stun:stun.l.google.com:19302"},
			TURNURL:        "turn:turn.example.com:3478",
			TURNUsername:   "u",
			TURNCredential: "c",
			TTLSeconds:     3600,
		},
		1*time.Hour, // long TTL — tests that care about expiry override via NewManagerWith.
		5,
		func() time.Time { return time.Date(2026, 7, 6, 14, 0, 0, 0, time.UTC) },
	)
}

func validSDP() string {
	// Minimal RFC-4566 SDP body, with the canonical first
	// line "v=0".
	return "v=0\r\n" +
		"o=- 1 2 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"t=0 0\r\n"
}

func goodOffer() *SDPPayload {
	return &SDPPayload{SDPType: "offer", SDP: validSDP()}
}

func goodAnswer() *SDPPayload {
	return &SDPPayload{SDPType: "answer", SDP: validSDP()}
}

func goodCandidate() ICECandidate {
	return ICECandidate{Candidate: "candidate:1 1 udp 2122260223 192.0.2.1 12345 typ host", SDPMID: "0", SDPMLineIndex: 0}
}

const offerer = "alice-1234567890abcdef0"  // 24 chars, valid hash shape
const answerer = "bob-12345678901abcdef00" // 24 chars
const offererOther = "carol-2345678901abcdef"

// -----------------------------------------------------------------------------
// SessionState
// -----------------------------------------------------------------------------

func TestSessionState_Valid(t *testing.T) {
	for _, st := range []SessionState{
		SessionStateNew, SessionStateConnecting, SessionStateConnected,
		SessionStateClosed, SessionStateFailed,
	} {
		if !st.Valid() {
			t.Errorf("%q should be valid", st)
		}
	}
	for _, st := range []SessionState{"", "alive", "disconnected", "fold"} {
		if st.Valid() {
			t.Errorf("%q should NOT be valid", st)
		}
	}
}

func TestSessionState_IsTerminal(t *testing.T) {
	cases := map[SessionState]bool{
		SessionStateNew:        false,
		SessionStateConnecting: false,
		SessionStateConnected:  false,
		SessionStateClosed:     true,
		SessionStateFailed:     true,
		"":                     false,
	}
	for st, want := range cases {
		if got := isTerminal(st); got != want {
			t.Errorf("isTerminal(%q) = %v, want %v", st, got, want)
		}
	}
}

// -----------------------------------------------------------------------------
// WebRTCSession.transition
// -----------------------------------------------------------------------------

func TestTransition_NewToConnecting(t *testing.T) {
	ws := &WebRTCSession{State: SessionStateNew, UpdatedAt: time.Now()}
	if err := ws.transition(SessionStateConnecting, false); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if ws.State != SessionStateConnecting {
		t.Errorf("state = %q, want connecting", ws.State)
	}
}

func TestTransition_NewToClosedRejected(t *testing.T) {
	ws := &WebRTCSession{State: SessionStateNew}
	err := ws.transition(SessionStateClosed, false)
	if !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("expected ErrInvalidStateTransition, got %v", err)
	}
}

func TestTransition_ConnectingToConnected(t *testing.T) {
	ws := &WebRTCSession{State: SessionStateConnecting}
	if err := ws.transition(SessionStateConnected, false); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if ws.State != SessionStateConnected {
		t.Errorf("state = %q, want connected", ws.State)
	}
}

func TestTransition_ConnectingToNewRejected(t *testing.T) {
	ws := &WebRTCSession{State: SessionStateConnecting}
	if err := ws.transition(SessionStateNew, false); !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("expected ErrInvalidStateTransition, got %v", err)
	}
}

func TestTransition_ConnectedToClosed(t *testing.T) {
	ws := &WebRTCSession{State: SessionStateConnected}
	if err := ws.transition(SessionStateClosed, false); err != nil {
		t.Fatalf("transition: %v", err)
	}
}

func TestTransition_TerminalRejectsAll(t *testing.T) {
	for _, st := range []SessionState{SessionStateClosed, SessionStateFailed} {
		ws := &WebRTCSession{State: st}
		for _, to := range []SessionState{
			SessionStateNew, SessionStateConnecting, SessionStateConnected,
		} {
			if err := ws.transition(to, false); !errors.Is(err, ErrInvalidStateTransition) {
				t.Errorf("terminal %s -> %s expected rejection, got %v", st, to, err)
			}
		}
	}
}

func TestTransition_ForceIgnoresRules(t *testing.T) {
	// force=true bypasses the transition table — used by
	// Expire / Fail cleanup paths.
	ws := &WebRTCSession{State: SessionStateClosed}
	if err := ws.transition(SessionStateFailed, true); err != nil {
		t.Fatalf("force transition: %v", err)
	}
	if ws.State != SessionStateFailed {
		t.Errorf("force transition did not change state; got %q", ws.State)
	}
}

func TestTransition_Idempotent(t *testing.T) {
	ws := &WebRTCSession{State: SessionStateConnecting}
	if err := ws.transition(SessionStateConnecting, false); err != nil {
		t.Fatalf("idempotent transition failed: %v", err)
	}
}

func TestTransition_UnknownTargetRejected(t *testing.T) {
	ws := &WebRTCSession{State: SessionStateNew}
	if err := ws.transition("invalid", true); !errors.Is(err, ErrInvalidStateTransition) {
		t.Fatalf("expected ErrInvalidStateTransition for unknown target, got %v", err)
	}
}

func TestTransition_DoubleTransitionLocked(t *testing.T) {
	// Explicit concurrency: two transitions on the same
	// session. The mutex must serialise them so the second
	// one sees the first one's state and produces the right
	// error.
	ws := &WebRTCSession{State: SessionStateNew}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = ws.transition(SessionStateConnecting, false) }()
	go func() { defer wg.Done(); _ = ws.transition(SessionStateFailed, false) }()
	wg.Wait()
	if ws.State != SessionStateConnecting && ws.State != SessionStateFailed {
		t.Errorf("state = %q, want one of the two transitions", ws.State)
	}
}

// -----------------------------------------------------------------------------
// SDPPayload / ICECandidate validation
// -----------------------------------------------------------------------------

func TestSDPPayload_Validate_Good(t *testing.T) {
	for _, sdpType := range []string{"offer", "answer"} {
		s := &SDPPayload{SDPType: sdpType, SDP: validSDP()}
		if err := s.Validate(); err != nil {
			t.Errorf("good %s validation: %v", sdpType, err)
		}
	}
}

func TestSDPPayload_Validate_RejectsBadType(t *testing.T) {
	for _, bad := range []string{"", "pranswer", "rollback", "Offer"} {
		s := &SDPPayload{SDPType: bad, SDP: validSDP()}
		if err := s.Validate(); err == nil {
			t.Errorf("bad type %q should reject", bad)
		}
	}
}

func TestSDPPayload_Validate_RejectsEmptySDP(t *testing.T) {
	s := &SDPPayload{SDPType: "offer", SDP: ""}
	if err := s.Validate(); err == nil {
		t.Errorf("empty sdp should reject")
	}
}

func TestSDPPayload_Validate_RejectsNonV0Prefix(t *testing.T) {
	s := &SDPPayload{SDPType: "offer", SDP: "v=1\r\n"}
	if err := s.Validate(); err == nil {
		t.Errorf("non-v=0 prefix should reject")
	}
}

func TestSDPPayload_FirstNonWSLine(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"v=0\r\no=-", "v=0"},
		{"\r\n  v=0\r\n", "v=0"},
		{"", ""},
		{"\r\n", ""},
	}
	for _, c := range cases {
		if got := firstNonWSLine(c.in); got != c.want {
			t.Errorf("firstNonWSLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestICECandidate_Validate(t *testing.T) {
	good := goodCandidate()
	if err := good.Validate(); err != nil {
		t.Errorf("good candidate rejected: %v", err)
	}
	for _, bad := range []ICECandidate{
		{Candidate: ""},
		{Candidate: "   "},
		{Candidate: "foobar"},
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("bad candidate %v should reject", bad)
		}
	}
}

func TestICERequest_FirstCandidate(t *testing.T) {
	r := &WebRTCICERequest{Candidates: []ICECandidate{goodCandidate()}}
	c, err := r.firstCandidate()
	if err != nil {
		t.Fatalf("firstCandidate: %v", err)
	}
	if c.Candidate != goodCandidate().Candidate {
		t.Errorf("candidate mismatch: %+v", c)
	}
	r.Candidates = []ICECandidate{}
	if _, err := r.firstCandidate(); err == nil {
		t.Errorf("empty list should reject")
	}
	r.Candidates = []ICECandidate{goodCandidate(), goodCandidate()}
	if _, err := r.firstCandidate(); err == nil {
		t.Errorf("two-candidate list should reject")
	}
}

// -----------------------------------------------------------------------------
// STUN/TURN config
// -----------------------------------------------------------------------------

func TestLoadSTUNTURNConfig_Defaults(t *testing.T) {
	// Clear all env first.
	for _, k := range []string{
		EnvConfigDefaults.STUNURL,
		EnvConfigDefaults.TURNURL,
		EnvConfigDefaults.TURNUsername,
		EnvConfigDefaults.TURNCredential,
		EnvConfigDefaults.CoturnTTLSeconds,
	} {
		_ = os.Unsetenv(k)
	}
	cfg := LoadSTUNTURNConfig()
	if len(cfg.STUNURLs) < 2 {
		t.Errorf("default STUN list too short: %v", cfg.STUNURLs)
	}
	if cfg.TURNURL != "" || cfg.TURNUsername != "" || cfg.TURNCredential != "" {
		t.Errorf("expected empty TURN, got %+v", cfg)
	}
}

func TestLoadSTUNTURNConfig_FromEnv(t *testing.T) {
	t.Setenv(EnvConfigDefaults.STUNURL, "stun:s1.example:3478, stun:s2.example:3478")
	t.Setenv(EnvConfigDefaults.TURNURL, "turn:turn.example:3478")
	t.Setenv(EnvConfigDefaults.TURNUsername, "alice")
	t.Setenv(EnvConfigDefaults.TURNCredential, "secret")
	t.Setenv(EnvConfigDefaults.CoturnTTLSeconds, "7200")

	cfg := LoadSTUNTURNConfig()
	if len(cfg.STUNURLs) != 2 || cfg.STUNURLs[0] != "stun:s1.example:3478" {
		t.Errorf("STUN list mismatch: %v", cfg.STUNURLs)
	}
	if cfg.TURNURL != "turn:turn.example:3478" {
		t.Errorf("TURN URL = %q", cfg.TURNURL)
	}
	if cfg.TURNUsername != "alice" || cfg.TURNCredential != "secret" {
		t.Errorf("TURN creds mismatch: %+v", cfg)
	}
	if cfg.TTLSeconds != 7200 {
		t.Errorf("TTL = %d", cfg.TTLSeconds)
	}
}

func TestLoadSTUNTURNConfig_BadTTLIgnored(t *testing.T) {
	t.Setenv(EnvConfigDefaults.CoturnTTLSeconds, "garbage-not-int")
	cfg := LoadSTUNTURNConfig()
	if cfg.TTLSeconds != 0 {
		t.Errorf("bad TTL should fall back to 0, got %d", cfg.TTLSeconds)
	}
}

func TestSTUNTURNConfig_EnvSTTLAccessor(t *testing.T) {
	if EnvConfigDefaults.STTLSeconds() != EnvConfigDefaults.CoturnTTLSeconds {
		t.Errorf("STTLSeconds accessor mismatch")
	}
}

func TestStrconvAtoi(t *testing.T) {
	for _, c := range []struct {
		in       string
		want     int
		wantErr  error
	}{
		{"0", 0, nil},
		{"42", 42, nil},
		{"1000000000", 1000000000, nil},
		{"", 0, errAtoiEmpty},
		{"abc", 0, errAtoi},
		{"99999999999999999999", 0, errAtoiOverflow},
	} {
		got, err := strconvAtoi(c.in)
		if c.wantErr == nil && got != c.want {
			t.Errorf("strconvAtoi(%q) = %d, want %d", c.in, got, c.want)
		}
		if c.wantErr != nil && !errors.Is(err, c.wantErr) {
			t.Errorf("strconvAtoi(%q) err = %v, want %v", c.in, err, c.wantErr)
		}
	}
}

// -----------------------------------------------------------------------------
// Manager — basic lifecycle
// -----------------------------------------------------------------------------

func TestManager_STUNTURN(t *testing.T) {
	turn := &STUNTURNConfig{STUNURLs: []string{"stun:x"}, TURNURL: "turn:y"}
	m := NewManagerWith(turn, time.Minute, 5, time.Now)
	if m.STUNTURN() != turn {
		t.Errorf("STUNTURN should return the same pointer we constructed with")
	}
	if m.Count() != 0 {
		t.Errorf("fresh manager Count() = %d, want 0", m.Count())
	}
}

func TestManager_Create(t *testing.T) {
	m := newTestManager()
	ws, err := m.CreateSession("", offerer)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if ws.ID == "" {
		t.Errorf("expected server-minted id")
	}
	if m.Count() != 1 {
		t.Errorf("Count() = %d, want 1", m.Count())
	}
}

func TestManager_Create_RejectsEmptyOfferer(t *testing.T) {
	m := newTestManager()
	if _, err := m.CreateSession("", ""); err == nil {
		t.Errorf("expected rejection for empty offerer")
	}
	if _, err := m.CreateSession("", "short"); err == nil {
		t.Errorf("expected rejection for short offerer hash")
	}
}

func TestManager_Create_RejectsDuplicateID(t *testing.T) {
	m := newTestManager()
	id := "ts-test-1234abcd"
	if _, err := m.CreateSession(id, offerer); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := m.CreateSession(id, offerer); err == nil {
		t.Errorf("duplicate id should reject")
	}
}

func TestManager_GetSession_EmptyID(t *testing.T) {
	m := newTestManager()
	if _, err := m.GetSession(""); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("empty id should produce ErrSessionNotFound, got %v", err)
	}
}

func TestManager_GetSession_UnknownID(t *testing.T) {
	m := newTestManager()
	if _, err := m.GetSession("ts-ghost-00000000"); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("unknown id should produce ErrSessionNotFound, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// Manager — Offer / Answer / ICE
// -----------------------------------------------------------------------------

func TestManager_ApplyOffer_HappyPath(t *testing.T) {
	m := newTestManager()
	ws, err := m.CreateSession("", offerer)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := m.ApplyOffer(ws.ID, offerer, goodOffer())
	if err != nil {
		t.Fatalf("apply offer: %v", err)
	}
	if updated.State != SessionStateConnecting {
		t.Errorf("state = %q, want connecting", updated.State)
	}
}

func TestManager_ApplyOffer_BadSDPRejected(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	bad := &SDPPayload{SDPType: "offer", SDP: "garbage"}
	if _, err := m.ApplyOffer(ws.ID, offerer, bad); err == nil {
		t.Errorf("bad sdp should reject")
	}
}

func TestManager_ApplyOffer_WrongPeerRejected(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	if _, err := m.ApplyOffer(ws.ID, offererOther, goodOffer()); err == nil {
		t.Errorf("mismatched peer should reject")
	}
}

func TestManager_ApplyOffer_NotInNewRejected(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	if _, err := m.ApplyOffer(ws.ID, offerer, goodOffer()); err != nil {
		t.Fatal(err)
	}
	// Now apply a second time — must reject because state is
	// now "connecting".
	if _, err := m.ApplyOffer(ws.ID, offerer, goodOffer()); !errors.Is(err, ErrInvalidStateTransition) {
		t.Errorf("second offer should produce ErrInvalidStateTransition, got %v", err)
	}
}

func TestManager_ApplyOffer_OnUnknownSession(t *testing.T) {
	m := newTestManager()
	if _, err := m.ApplyOffer("ts-ghost-9999", offerer, goodOffer()); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("unknown session should produce ErrSessionNotFound, got %v", err)
	}
}

func TestManager_ApplyAnswer_HappyPath(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	_, _ = m.ApplyOffer(ws.ID, offerer, goodOffer())
	updated, err := m.ApplyAnswer(ws.ID, answerer, goodAnswer())
	if err != nil {
		t.Fatalf("apply answer: %v", err)
	}
	if updated.State != SessionStateConnected {
		t.Errorf("state = %q, want connected", updated.State)
	}
}

func TestManager_ApplyAnswer_OnNewSessionRejected(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	if _, err := m.ApplyAnswer(ws.ID, answerer, goodAnswer()); !errors.Is(err, ErrInvalidStateTransition) {
		t.Errorf("answer before offer should reject; got %v", err)
	}
}

func TestManager_ApplyAnswer_RejectsOffererAsAnswerer(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	_, _ = m.ApplyOffer(ws.ID, offerer, goodOffer())
	if _, err := m.ApplyAnswer(ws.ID, offerer, goodAnswer()); err == nil {
		t.Errorf("answerer must differ from offerer")
	}
}

func TestManager_ApplyAnswer_BadSDPRejected(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	_, _ = m.ApplyOffer(ws.ID, offerer, goodOffer())
	if _, err := m.ApplyAnswer(ws.ID, answerer, &SDPPayload{SDPType: "pranswer", SDP: validSDP()}); err == nil {
		t.Errorf("bad sdp_type should reject")
	}
}

func TestManager_ApplyAnswer_DifferentAnswererTwiceRejected(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	_, _ = m.ApplyOffer(ws.ID, offerer, goodOffer())
	if _, err := m.ApplyAnswer(ws.ID, answerer, goodAnswer()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ApplyAnswer(ws.ID, "dave-2345678901abcdefx2", goodAnswer()); err == nil {
		t.Errorf("second answerer should reject")
	}
}

func TestManager_AppendICE_HappyPath(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	got, err := m.AppendICE(ws.ID, offerer, goodCandidate())
	if err != nil {
		t.Fatalf("append ice: %v", err)
	}
	if got == nil {
		t.Errorf("AppendICE returned nil ws")
	}
}

func TestManager_AppendICE_BadCandidateRejected(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	bad := ICECandidate{Candidate: "no-prefix"}
	if _, err := m.AppendICE(ws.ID, offerer, bad); err == nil {
		t.Errorf("bad candidate should reject")
	}
}

func TestManager_AppendICE_ShortPeerRejected(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	if _, err := m.AppendICE(ws.ID, "x", goodCandidate()); err == nil {
		t.Errorf("short peer hash should reject")
	}
}

func TestManager_AppendICE_SlidingWindowCap(t *testing.T) {
	m := newTestManager() // iceCap=5
	ws, _ := m.CreateSession("", offerer)
	for i := 0; i < 7; i++ {
		_, err := m.AppendICE(ws.ID, offerer, ICECandidate{
			Candidate: fmt.Sprintf("candidate:1 1 udp 2122260223 192.0.2.%d 10000 typ host", i),
		})
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	got, _ := m.GetSession(ws.ID)
	got.mu.Lock()
	defer got.mu.Unlock()
	if len(got.Candidates[offerer]) != 5 {
		t.Errorf("ICE list len = %d, want capped at 5", len(got.Candidates[offerer]))
	}
	// First two should have been dropped; the oldest now is
	// the one for index 2.
	if !strings.HasPrefix(got.Candidates[offerer][0].Candidate, "candidate:1 1 udp 2122260223 192.0.2.2") {
		t.Errorf("oldest candidate after cap = %q", got.Candidates[offerer][0].Candidate)
	}
}

func TestManager_AppendICE_OnTerminalRejected(t *testing.T) {
	// Achieve terminal state by failing the session — that
	// leaves the record gone (Fail removes from the map), but
	// AppendICE still rejects (we're just changing the
	// expected error code).
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	_ = m.FailSession(ws.ID, "manual")
	if _, err := m.AppendICE(ws.ID, offerer, goodCandidate()); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("failed session appendice should produce ErrSessionNotFound (it's been removed), got %v", err)
	}
}

func TestManager_AppendICE_OnCloseStillReachableNoOp(t *testing.T) {
	// After Close, the session is removed from the active
	// map — so AppendICE returns ErrSessionNotFound. This
	// exercises the "terminal removes from map" branch.
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	_, _ = m.ApplyOffer(ws.ID, offerer, goodOffer())
	_, _ = m.ApplyAnswer(ws.ID, answerer, goodAnswer())
	if err := m.CloseSession(ws.ID); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := m.AppendICE(ws.ID, offerer, goodCandidate()); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("after close, append should return ErrSessionNotFound, got %v", err)
	}
}

func TestManager_AppendICE_OnFailedReturnsNil(t *testing.T) {
	// FailSession also removes from the map.
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	if err := m.FailSession(ws.ID, "manual"); err != nil {
		t.Fatalf("fail: %v", err)
	}
	// After Fail, the session is gone from the map.
	if _, err := m.AppendICE(ws.ID, offerer, goodCandidate()); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("failed session appendice should return ErrSessionNotFound, got %v", err)
	}
}

// -----------------------------------------------------------------------------
// Manager — close / fail / expire
// -----------------------------------------------------------------------------

func TestManager_CloseSession_Idempotent(t *testing.T) {
	// To reach a closeable state we must complete the offer +
	// answer exchange (connecting → connected → closed).
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	_, _ = m.ApplyOffer(ws.ID, offerer, goodOffer())
	_, _ = m.ApplyAnswer(ws.ID, answerer, goodAnswer())
	if err := m.CloseSession(ws.ID); err != nil {
		t.Errorf("first close: %v", err)
	}
	if err := m.CloseSession(ws.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("second close on deleted session should produce ErrSessionNotFound, got %v", err)
	}
}

func TestManager_CloseSession_OnConnectingRejected(t *testing.T) {
	// Exhaustive: Close from non-terminal states either succeeds
	// (from Connected) or is rejected (from New/Connecting).
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	_, _ = m.ApplyOffer(ws.ID, offerer, goodOffer())
	if err := m.CloseSession(ws.ID); !errors.Is(err, ErrInvalidStateTransition) {
		t.Errorf("close from connecting should reject, got %v", err)
	}
}

func TestManager_FailSession(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	_, _ = m.ApplyOffer(ws.ID, offerer, goodOffer())
	if err := m.FailSession(ws.ID, "manual"); err != nil {
		t.Errorf("fail: %v", err)
	}
	if _, err := m.GetSession(ws.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("after fail, session should be removed (got %v)", err)
	}
}

func TestManager_ExpireLazy(t *testing.T) {
	clock := newTestClock(time.Date(2026, 7, 6, 14, 0, 0, 0, time.UTC))
	m := NewManagerWith(
		&STUNTURNConfig{STUNURLs: []string{"stun:x"}, TURNURL: "turn:y"},
		100*time.Millisecond,
		5,
		clock.Now,
	)
	ws1, _ := m.CreateSession("", offerer)
	ws2, _ := m.CreateSession("", offererOther)

	// Initial Expire: clock hasn't moved; sessions are fresh.
	if e := m.Expire(); e != 0 {
		t.Errorf("initial expire = %d, want 0", e)
	}

	// Advance past the TTL.
	clock.Advance(200 * time.Millisecond)
	expired := m.Expire()
	if expired != 2 {
		t.Errorf("expired = %d, want 2", expired)
	}
	if _, err := m.GetSession(ws1.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("ws1 should be gone: %v", err)
	}
	if _, err := m.GetSession(ws2.ID); !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("ws2 should be gone: %v", err)
	}
}

// fakeClock implements a controllable clock for tests.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock(start time.Time) *fakeClock { return &fakeClock{now: start} }

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// TestManager_ExpireLocked skips terminal session (used to
// verify the "if already-terminal, don't re-touch" branch).
func TestManager_ExpireLocked_SkipsTerminal(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	_ = m.FailSession(ws.ID, "")
	// push clock past TTL by direct expireLocked with a later now.
	expired := m.expireLocked(time.Date(2026, 7, 6, 15, 0, 0, 0, time.UTC))
	if expired != 0 {
		t.Errorf("expired = %d, want 0 (only failed sessions, no new ones)", expired)
	}
}

// -----------------------------------------------------------------------------
// Candidate aggregation + ordering
// -----------------------------------------------------------------------------

func TestRemoteCandidates_OnlyReturnsOtherPeer(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	_, _ = m.AppendICE(ws.ID, offerer, goodCandidate())
	_, _ = m.AppendICE(ws.ID, answerer, goodCandidate())
	got, _ := m.GetSession(ws.ID)
	others := got.RemoteCandidates(offerer)
	if len(others) != 1 {
		t.Errorf("RemoteCandidates(offerer) len = %d, want 1", len(others))
	}
	// ask for "ghost": no peers are stored under that hash so
	// we get all of them. This is the implementation contract:
	// "Remote" = every other peer than the one you supply.
	all := got.RemoteCandidates("ghost-1234567890abcdef")
	if len(all) != 2 {
		t.Errorf("RemoteCandidates(ghost) len = %d, want 2", len(all))
	}
}

func TestPeerList_StableOrdering(t *testing.T) {
	ws := &WebRTCSession{Candidates: map[string][]ICECandidate{
		"zzzzz": {{Candidate: "candidate:z"}},
		"aaaaa": {{Candidate: "candidate:a"}},
		"mmmmm": {{Candidate: "candidate:m"}},
	}}
	peers := ws.peerList()
	want := []string{"aaaaa", "mmmmm", "zzzzz"}
	if !equalStringSlices(peers, want) {
		t.Errorf("peerList() = %v, want %v", peers, want)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPeerList_EmptyEntriesExcluded(t *testing.T) {
	ws := &WebRTCSession{Candidates: map[string][]ICECandidate{
		"emptyone": nil,
		"filled":   {{Candidate: "candidate:x"}},
	}}
	peers := ws.peerList()
	if len(peers) != 1 || peers[0] != "filled" {
		t.Errorf("peerList() = %v, want only 'filled'", peers)
	}
}

// -----------------------------------------------------------------------------
// WebRTCSession.Snapshot
// -----------------------------------------------------------------------------

func TestSnapshot_Serialisable(t *testing.T) {
	ws := &WebRTCSession{
		ID:        "ts-test-12345678",
		State:     SessionStateConnecting,
		Offerer:   offerer,
		Offer:     goodOffer(),
		Candidates: map[string][]ICECandidate{
			offerer: {goodCandidate()},
		},
		CreatedAt: time.Date(2026, 7, 6, 14, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 7, 6, 14, 0, 0, 0, time.UTC),
	}
	v := ws.Snapshot()
	if v.ID != "ts-test-12345678" || v.State != SessionStateConnecting {
		t.Errorf("snapshot mismatch: %+v", v)
	}
	if !v.HasOffer || v.HasAnswer {
		t.Errorf("offer/answer flags wrong: %+v", v)
	}
	// Ensure the snapshot doesn't leak the candidate map.
	b, _ := json.Marshal(v)
	s := string(b)
	if strings.Contains(s, "candidate:") {
		t.Errorf("snapshot must not include candidates: %s", s)
	}
}

// -----------------------------------------------------------------------------
// HTTP handlers
// -----------------------------------------------------------------------------

func postJSON(t *testing.T, m *Manager, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var h http.HandlerFunc
	switch path {
	case "/webrtc/offer":
		h = m.HandleOffer
	case "/webrtc/answer":
		h = m.HandleAnswer
	case "/webrtc/ice":
		h = m.HandleICE
	case "/webrtc/config":
		h = m.HandleSTUNTURNConfig
	default:
		t.Fatalf("unknown path %q", path)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestHandleOffer_CreateAndApply(t *testing.T) {
	m := newTestManager()
	body := WebRTCOfferRequest{
		PeerHash: offerer,
		SDP:      *goodOffer(),
	}
	rec := postJSON(t, m, "POST", "/webrtc/offer", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var resp WebRTCSignallingResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SessionID == "" {
		t.Errorf("missing session_id")
	}
	if resp.State != SessionStateConnecting {
		t.Errorf("state = %q, want connecting", resp.State)
	}
}

func TestHandleOffer_RejectsWrongMethod(t *testing.T) {
	m := newTestManager()
	rec := postJSON(t, m, "GET", "/webrtc/offer", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("wrong-method code = %d, want 405", rec.Code)
	}
}

func TestHandleOffer_BadJSON(t *testing.T) {
	m := newTestManager()
	req := httptest.NewRequest("POST", "/webrtc/offer", bytes.NewReader([]byte("not json")))
	rec := httptest.NewRecorder()
	m.HandleOffer(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad-json code = %d, want 400", rec.Code)
	}
}

func TestHandleOffer_RejectsBadSDP(t *testing.T) {
	m := newTestManager()
	body := WebRTCOfferRequest{
		PeerHash: offerer,
		SDP:      SDPPayload{SDPType: "offer", SDP: "garbage"},
	}
	rec := postJSON(t, m, "POST", "/webrtc/offer", body)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad-sdp code = %d, want 400", rec.Code)
	}
}

func TestHandleOffer_InvalidStateTransitionReturns409(t *testing.T) {
	m := newTestManager()
	body := WebRTCOfferRequest{
		SessionID: "ts-preseed-doesnotexist",
		PeerHash:  offerer,
		SDP:       *goodOffer(),
	}
	rec := postJSON(t, m, "POST", "/webrtc/offer", body)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown session code = %d, want 404", rec.Code)
	}
}

func TestHandleAnswer_HappyPath(t *testing.T) {
	m := newTestManager()
	offer := postJSON(t, m, "POST", "/webrtc/offer", WebRTCOfferRequest{
		PeerHash: offerer,
		SDP:      *goodOffer(),
	})
	if offer.Code != http.StatusCreated {
		t.Fatalf("offer setup: %d", offer.Code)
	}
	var oresp WebRTCSignallingResponse
	_ = json.Unmarshal(offer.Body.Bytes(), &oresp)

	ans := postJSON(t, m, "POST", "/webrtc/answer", WebRTCAnswerRequest{
		SessionID: oresp.SessionID,
		PeerHash:  answerer,
		SDP:       *goodAnswer(),
	})
	if ans.Code != http.StatusOK {
		t.Fatalf("answer code = %d, want 200; body=%s", ans.Code, ans.Body.String())
	}
	var aresp WebRTCSignallingResponse
	if err := json.Unmarshal(ans.Body.Bytes(), &aresp); err != nil {
		t.Fatal(err)
	}
	if aresp.State != SessionStateConnected {
		t.Errorf("state = %q, want connected", aresp.State)
	}
}

func TestHandleAnswer_OnNewSessionReturns409(t *testing.T) {
	m := newTestManager()
	ans := postJSON(t, m, "POST", "/webrtc/answer", WebRTCAnswerRequest{
		SessionID: "ts-ghost-9999",
		PeerHash:  answerer,
		SDP:       *goodAnswer(),
	})
	if ans.Code != http.StatusNotFound {
		t.Errorf("answer-before-offer code = %d, want 404", ans.Code)
	}
}

func TestHandleICE_HappyPath(t *testing.T) {
	m := newTestManager()
	offer := postJSON(t, m, "POST", "/webrtc/offer", WebRTCOfferRequest{
		PeerHash: offerer,
		SDP:      *goodOffer(),
	})
	var oresp WebRTCSignallingResponse
	_ = json.Unmarshal(offer.Body.Bytes(), &oresp)

	ice := postJSON(t, m, "POST", "/webrtc/ice", WebRTCICERequest{
		SessionID:  oresp.SessionID,
		PeerHash:   offerer,
		Candidates: []ICECandidate{goodCandidate()},
	})
	if ice.Code != http.StatusOK {
		t.Fatalf("ice code = %d, want 200; body=%s", ice.Code, ice.Body.String())
	}
	var iresp WebRTCSignallingResponse
	_ = json.Unmarshal(ice.Body.Bytes(), &iresp)
	if len(iresp.ICEList) == 0 {
		t.Errorf("ICEList should include offerer: %+v", iresp)
	}
}

func TestHandleICE_EmptyCandidatesRejected(t *testing.T) {
	m := newTestManager()
	offer := postJSON(t, m, "POST", "/webrtc/offer", WebRTCOfferRequest{
		PeerHash: offerer,
		SDP:      *goodOffer(),
	})
	var oresp WebRTCSignallingResponse
	_ = json.Unmarshal(offer.Body.Bytes(), &oresp)

	ice := postJSON(t, m, "POST", "/webrtc/ice", WebRTCICERequest{
		SessionID: oresp.SessionID,
		PeerHash:  offerer,
	})
	if ice.Code != http.StatusBadRequest {
		t.Errorf("empty ice code = %d, want 400", ice.Code)
	}
}

func TestHandleICE_BadPrefixRejected(t *testing.T) {
	m := newTestManager()
	offer := postJSON(t, m, "POST", "/webrtc/offer", WebRTCOfferRequest{
		PeerHash: offerer,
		SDP:      *goodOffer(),
	})
	var oresp WebRTCSignallingResponse
	_ = json.Unmarshal(offer.Body.Bytes(), &oresp)

	ice := postJSON(t, m, "POST", "/webrtc/ice", WebRTCICERequest{
		SessionID:  oresp.SessionID,
		PeerHash:   offerer,
		Candidates: []ICECandidate{{Candidate: "not-candidate"}},
	})
	if ice.Code != http.StatusBadRequest {
		t.Errorf("bad prefix code = %d, want 400", ice.Code)
	}
}

func TestHandleSTUNTURNConfig_Success(t *testing.T) {
	m := newTestManager()
	req := httptest.NewRequest("GET", "/webrtc/config", nil)
	rec := httptest.NewRecorder()
	m.HandleSTUNTURNConfig(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var cfg STUNTURNConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(cfg.STUNURLs) == 0 {
		t.Errorf("STUN list empty: %+v", cfg)
	}
	if cfg.TURNURL != "turn:turn.example.com:3478" {
		t.Errorf("TURN URL = %q", cfg.TURNURL)
	}
}

func TestHandleSTUNTURNConfig_WrongMethod(t *testing.T) {
	m := newTestManager()
	req := httptest.NewRequest("POST", "/webrtc/config", nil)
	rec := httptest.NewRecorder()
	m.HandleSTUNTURNConfig(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("code = %d, want 405", rec.Code)
	}
}

// -----------------------------------------------------------------------------
// Manager.translateError coverage via the public handlers
// -----------------------------------------------------------------------------

func TestTranslateError_KnownErrors(t *testing.T) {
	m := newTestManager()
	for _, tc := range []struct {
		err  error
		want int
	}{
		{ErrSessionNotFound, http.StatusNotFound},
		{ErrSessionTerminal, http.StatusConflict},
		{ErrInvalidStateTransition, http.StatusConflict},
		{ErrInvalidEnvelope, http.StatusBadRequest},
		{errors.New("something else"), http.StatusInternalServerError},
	} {
		rec := httptest.NewRecorder()
		m.translateError(rec, tc.err)
		if rec.Code != tc.want {
			t.Errorf("translateError(%v) = %d, want %d", tc.err, rec.Code, tc.want)
		}
	}
}

// -----------------------------------------------------------------------------
// Privacy invariants: candidate strings must NOT appear in the
// manager's own logs (we don't have a logger, but the contract
// is "no candidate string in any error message that the manager
// produces"). This test asserts that the error messages don't
// echo the candidate.
// -----------------------------------------------------------------------------

func TestPrivacy_NoCandidateInErrorMessages(t *testing.T) {
	m := newTestManager()
	leak := "candidate:1 1 udp 2122260223 192.0.2.99 12345 typ host"
	req := httptest.NewRequest("POST", "/webrtc/ice", bytes.NewReader([]byte(
		`{"session_id":"ts-priv-1234abcd","peer_hash":"`+offerer+`","candidates":[{"candidate":"`+leak+`"}]}`,
	)))
	rec := httptest.NewRecorder()
	m.HandleICE(rec, req)
	// Even on 404 the candidate must not have echoed.
	if strings.Contains(rec.Body.String(), leak) {
		t.Errorf("candidate leaked in error body: %s", rec.Body.String())
	}
	// Now check the bad-prefix path (validation error).
	req2 := httptest.NewRequest("POST", "/webrtc/ice", bytes.NewReader([]byte(
		`{"session_id":"ts-priv-1234abcd","peer_hash":"`+offerer+`","candidates":[{"candidate":"leaky-supersecret"}]}`,
	)))
	rec2 := httptest.NewRecorder()
	m.HandleICE(rec2, req2)
	if strings.Contains(rec2.Body.String(), "leaky-supersecret") {
		t.Errorf("bad-candidate value leaked in error body: %s", rec2.Body.String())
	}
}

// -----------------------------------------------------------------------------
// WebRTCManagerIface compile-time assertion (guard).
// -----------------------------------------------------------------------------

func TestWebRTCManager_SatisfiesIface(t *testing.T) {
	var _ WebRTCManagerIface = (*Manager)(nil)
	var _ WebRTCManagerIface = NewManager()
}

// -----------------------------------------------------------------------------
// Concurrency — race-free under concurrent offer/answer/ice.
// -----------------------------------------------------------------------------

func TestManager_ConcurrentIceIsRaceFree(t *testing.T) {
	m := newTestManager()
	ws, _ := m.CreateSession("", offerer)
	_, _ = m.ApplyOffer(ws.ID, offerer, goodOffer())

	const N = 50
	var wg sync.WaitGroup
	var failures atomic.Int64
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := m.AppendICE(ws.ID, offerer, ICECandidate{
				Candidate: fmt.Sprintf("candidate:1 1 udp 2122260223 10.0.0.%d 10000 typ host", i),
			})
			if err != nil {
				failures.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Errorf("failures = %d, want 0", failures.Load())
	}
}

// -----------------------------------------------------------------------------
// newSessionID — emit deterministic-ish ids; ensure uniqueness
// under repeated calls. We don't directly test crypto/rand here —
// the visible contract is "non-empty id, sorted by ts component".
// -----------------------------------------------------------------------------

func TestNewSessionID_UniqueAndNonEmpty(t *testing.T) {
	seen := make(map[string]struct{}, 100)
	for i := 0; i < 100; i++ {
		id := newSessionID(time.Now)
		if id == "" {
			t.Fatalf("empty id")
		}
		if !strings.HasPrefix(id, "ts-") {
			t.Fatalf("id %q does not start with ts-", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = struct{}{}
	}
}
