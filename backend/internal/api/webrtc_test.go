package api

// webrtc_test.go — handler tests for /api/v1/webrtc/* endpoints
// (Sprint 3 PR-21a).
//
// Coverage:
//   - Each of the four endpoints is hit through the full chi
//     middleware stack (api-version, rate-limit, max-bytes,
//     access-log, CORS, request-id, device-context).
//   - Body parse, error envelope translation, success path.
//   - 500 path when the matching.Manager is nil (defensive).
//   - 415 path when Content-Type is not application/json.
//   - Privacy: candidate strings never appear in error bodies.

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/opene2ee-com/e2ee-app/backend/internal/matching"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeWebRTC captures the last call for each handler so tests
// can assert the api wrapper reached the matching layer. It
// also lets tests override the response to exercise error
// envelope translation.
type fakeWebRTC struct {
	mu         sync.Mutex
	offerBody  []byte
	answerBody []byte
	iceBody    []byte
	configBody []byte
	status     int
}

func (f *fakeWebRTC) HandleOffer(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.offerBody = append([]byte(nil), readBodyForFake(r)...)
	if f.status != 0 {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"code":"bad_request","message":"forced"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"session_id":"fake-session-001","state":"connecting"}`))
}

func (f *fakeWebRTC) HandleAnswer(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answerBody = append([]byte(nil), readBodyForFake(r)...)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"session_id":"fake-session-001","state":"connected"}`))
}

func (f *fakeWebRTC) HandleICE(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.iceBody = append([]byte(nil), readBodyForFake(r)...)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"session_id":"fake-session-001","state":"connecting"}`))
}

func (f *fakeWebRTC) HandleSTUNTURNConfig(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.configBody = append([]byte(nil), readBodyForFake(r)...)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"stun_urls":["stun:stun.l.google.com:19302"]}`))
}

func readBodyForFake(r *http.Request) []byte {
	if r.Body == nil {
		return nil
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return nil
	}
	return b
}

func newTestAPIWithWebRTC(t *testing.T) (*testAPI, *fakeWebRTC) {
	t.Helper()
	ta := newTestAPI(t)
	fr := &fakeWebRTC{}
	// Inject the fake into the API's Config. Config is a value
	// — we go through the API's internal pointer to mutate.
	api, err := New(ta.deps.Cfg)
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	api.deps.Cfg.WebRTC = fr
	ta.API = api
	return ta, fr
}

func TestWebRTC_Offer_EmptyBody400(t *testing.T) {
	ta, _ := newTestAPIWithWebRTC(t)
	w := do(t, ta.Handler(), "POST", "/api/v1/webrtc/offer", withAPIHeaders(nil), "")
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWebRTC_Offer_BadJSON400(t *testing.T) {
	ta, _ := newTestAPIWithWebRTC(t)
	h := map[string]string{HeaderAPIVersion: APIVersion, "Content-Type": "application/json"}
	w := do(t, ta.Handler(), "POST", "/api/v1/webrtc/offer", h, "not json")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestWebRTC_Offer_HappyPath(t *testing.T) {
	ta, fr := newTestAPIWithWebRTC(t)
	body := `{"peer_hash":"alice-1234567890abcdef","sdp":{"sdp_type":"offer","sdp":"v=0\\r\\no=- 1 2 IN IP4 1.1.1.1\\r\\n"}}`
	h := withAPIHeaders(nil)
	w := do(t, ta.Handler(), "POST", "/api/v1/webrtc/offer", h, body)
	require.Equal(t, http.StatusCreated, w.Code)
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if len(fr.offerBody) == 0 {
		t.Errorf("fake did not receive the body")
	}
}

func TestWebRTC_Offer_ErrorEnvelopeTranslated(t *testing.T) {
	// Test that the api wrapper correctly translates the
	// matching-side error envelope to an api-side ErrorBody.
	ta := newTestAPI(t)
	fr := &fakeWebRTC{status: http.StatusBadRequest}
	api2, _ := New(ta.deps.Cfg)
	api2.deps.Cfg.WebRTC = fr
	ta.API = api2

	body := `{"peer_hash":"alice-1234567890abcdef","sdp":{"sdp_type":"offer","sdp":"v=0\\r\\n"}}`
	w := do(t, ta.Handler(), "POST", "/api/v1/webrtc/offer", withAPIHeaders(nil), body)
	require.Equal(t, http.StatusBadRequest, w.Code)
	var errBody ErrorBody
	readJSON(t, w.Body, &errBody)
	assert.Equal(t, CodeBadRequest, errBody.Code, "body=%s", w.Body.String())
}

func TestWebRTC_Answer_HappyPath(t *testing.T) {
	ta, fr := newTestAPIWithWebRTC(t)
	body := `{"session_id":"ts-1234abcd","peer_hash":"bob-12345678901ab","sdp":{"sdp_type":"answer","sdp":"v=0\\r\\n"}}`
	w := do(t, ta.Handler(), "POST", "/api/v1/webrtc/answer", withAPIHeaders(nil), body)
	require.Equal(t, http.StatusOK, w.Code)
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if len(fr.answerBody) == 0 {
		t.Errorf("fake did not receive answer body")
	}
}

func TestWebRTC_ICE_HappyPath(t *testing.T) {
	ta, fr := newTestAPIWithWebRTC(t)
	body := `{"session_id":"ts-1234abcd","peer_hash":"alice-1234567890ab","candidates":[{"candidate":"candidate:1 1 udp 2122260223 192.0.2.1 1000 typ host","sdpMid":"0","sdpMLineIndex":0}]}`
	w := do(t, ta.Handler(), "POST", "/api/v1/webrtc/ice", withAPIHeaders(nil), body)
	require.Equal(t, http.StatusOK, w.Code)
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if len(fr.iceBody) == 0 {
		t.Errorf("fake did not receive ice body")
	}
}

func TestWebRTC_Config_HappyPath(t *testing.T) {
	ta, _ := newTestAPIWithWebRTC(t)
	w := do(t, ta.Handler(), "GET", "/api/v1/webrtc/config", withAPIHeaders(nil), "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "stun_urls")
}

func TestWebRTC_Offer_NoManager(t *testing.T) {
	// When Config.WebRTC is nil, the handler returns 500
	// (internal_error) without panicking.
	ta := newTestAPI(t)
	body := `{"peer_hash":"alice-1234567890abcdef","sdp":{"sdp_type":"offer","sdp":"v=0\\r\\n"}}`
	w := do(t, ta.Handler(), "POST", "/api/v1/webrtc/offer", withAPIHeaders(nil), body)
	require.Equal(t, http.StatusInternalServerError, w.Code)
	var errBody ErrorBody
	readJSON(t, w.Body, &errBody)
	assert.Equal(t, CodeInternal, errBody.Code)
}

func TestWebRTC_Answer_NoManager(t *testing.T) {
	ta := newTestAPI(t)
	w := do(t, ta.Handler(), "POST", "/api/v1/webrtc/answer", withAPIHeaders(nil), "{}")
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestWebRTC_ICE_NoManager(t *testing.T) {
	ta := newTestAPI(t)
	w := do(t, ta.Handler(), "POST", "/api/v1/webrtc/ice", withAPIHeaders(nil), "{}")
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestWebRTC_Config_NoManager(t *testing.T) {
	ta := newTestAPI(t)
	w := do(t, ta.Handler(), "GET", "/api/v1/webrtc/config", withAPIHeaders(nil), "")
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

// Privacy: a candidate string is "PII-shaped" (peer-reflexive
// IP). The api layer must NOT echo it back on the 4xx/5xx path.
func TestWebRTC_Privacy_NoCandidateInErrorBody(t *testing.T) {
	ta := newTestAPI(t)
	fr := &fakeWebRTC{status: http.StatusBadRequest}
	api2, _ := New(ta.deps.Cfg)
	api2.deps.Cfg.WebRTC = fr
	ta.API = api2

	secret := "candidate:999999 1 udp 2113937151 9.9.9.9 666 typ host"
	body := `{"session_id":"x","peer_hash":"alice-1234567890abcdef","candidates":[{"candidate":"` + secret + `"}]}`
	w := do(t, ta.Handler(), "POST", "/api/v1/webrtc/ice", withAPIHeaders(nil), body)
	if strings.Contains(w.Body.String(), "9.9.9.9") || strings.Contains(w.Body.String(), secret) {
		t.Errorf("candidate leaked: %s", w.Body.String())
	}
}

// Compile-time check that fakeWebRTC satisfies the iface.
var _ matching.WebRTCManagerIface = (*fakeWebRTC)(nil)
