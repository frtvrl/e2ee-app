package api

// webrtc.go — the REST handlers that adapt the matching
// package's WebRTCManager to the HTTP router.
//
// Wire surface (Sprint 3 PR-21a):
//
//   POST /api/v1/webrtc/offer   — SDP offer exchange
//   POST /api/v1/webrtc/answer  — SDP answer exchange
//   POST /api/v1/webrtc/ice     — single ICE candidate
//   GET  /api/v1/webrtc/config  — ICE-server config (STUN/TURN)
//
// All four endpoints sit behind the standard middleware chain
// (request-id, device-context, access-log, CORS, max-bytes,
// API-version, rate-limit) — the same chain the existing
// sessions / matrix / users endpoints see. The handlers are
// thin wrappers: they parse the JSON body, call into
// matching.WebRTCManagerIface, and serialise the canonical
// matching.WebRTCSignallingResponse (or translate the
// matching-side error envelope to an api.ErrorBody on the
// failure path).
//
// PRIVACY (ADR-0006 §Veri Minimizasyonu):
//   - X-Device-Id-Hash is captured into context by the standard
//     middleware. The handlers ALSO accept a `peer_hash` in the
//     body — the body's value is treated as authoritative for
//     state-machine checks (the offer/answer flow is
//     peer-driven, not device-driven).
//   - The handlers never log SDP text, candidate strings, or
//     peer hashes directly. The access-log middleware logs only
//     the device hash prefix (8 hex chars).

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/opene2ee-com/e2ee-app/backend/internal/matching"
)

// handleWebRTCOffer is POST /api/v1/webrtc/offer.
func (a *API) handleWebRTCOffer() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.deps.Cfg.WebRTC == nil {
			writeInternal(w)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeBadRequest(w, "Failed to read request body.")
			return
		}
		if len(body) == 0 {
			writeBadRequest(w, "Empty request body.")
			return
		}
		var req matching.WebRTCOfferRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeBadRequest(w, "Malformed JSON.")
			return
		}
		// Replace the body so the matching-side handler can
		// also parse it. The matching package keeps its own
		// http.HandlerFunc signature for backward compatibility
		// with the WebSocket-shaped Hub signal layer.
		r.Body = io.NopCloser(bytesNewReader(body))
		rec := &captureWriter{header: http.Header{}}
		a.deps.Cfg.WebRTC.HandleOffer(rec, r)
		writeCaptured(w, rec)
	}
}

// handleWebRTCAnswer is POST /api/v1/webrtc/answer.
func (a *API) handleWebRTCAnswer() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.deps.Cfg.WebRTC == nil {
			writeInternal(w)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeBadRequest(w, "Failed to read request body.")
			return
		}
		if len(body) == 0 {
			writeBadRequest(w, "Empty request body.")
			return
		}
		var req matching.WebRTCAnswerRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeBadRequest(w, "Malformed JSON.")
			return
		}
		r.Body = io.NopCloser(bytesNewReader(body))
		rec := &captureWriter{header: http.Header{}}
		a.deps.Cfg.WebRTC.HandleAnswer(rec, r)
		writeCaptured(w, rec)
	}
}

// handleWebRTCICE is POST /api/v1/webrtc/ice.
func (a *API) handleWebRTCICE() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.deps.Cfg.WebRTC == nil {
			writeInternal(w)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeBadRequest(w, "Failed to read request body.")
			return
		}
		if len(body) == 0 {
			writeBadRequest(w, "Empty request body.")
			return
		}
		var req matching.WebRTCICERequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeBadRequest(w, "Malformed JSON.")
			return
		}
		r.Body = io.NopCloser(bytesNewReader(body))
		rec := &captureWriter{header: http.Header{}}
		a.deps.Cfg.WebRTC.HandleICE(rec, r)
		writeCaptured(w, rec)
	}
}

// bytesNewReader is a tiny indirection that lets us swap to
// strings.NewReader or bytes.NewReader without polluting the
// surrounding code. Kept private.
func bytesNewReader(b []byte) *bytesReader { return &bytesReader{b: b} }

// bytesReader is the smallest possible io.Reader/Seeker/ReaderAt
// implementation over a byte slice. We don't implement Seek/ReadAt
// here — http.Request doesn't need them for body reuse.
type bytesReader struct {
	b []byte
	i int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

// handleWebRTCConfig is GET /api/v1/webrtc/config.
func (a *API) handleWebRTCConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.deps.Cfg.WebRTC == nil {
			writeInternal(w)
			return
		}
		rec := &captureWriter{header: http.Header{}}
		a.deps.Cfg.WebRTC.HandleSTUNTURNConfig(rec, r)
		writeCaptured(w, rec)
	}
}

// -----------------------------------------------------------------------------
// captureWriter — minimal http.ResponseWriter that buffers the
// status, headers, and body so the api handler can re-emit them
// through the api package's writeError on the failure path.
// -----------------------------------------------------------------------------

type captureWriter struct {
	header http.Header
	body   []byte
	status int
	wrote  bool
}

func (c *captureWriter) Header() http.Header { return c.header }
func (c *captureWriter) WriteHeader(s int) {
	if c.wrote {
		return
	}
	c.status = s
	c.wrote = true
}
func (c *captureWriter) Write(b []byte) (int, error) {
	if !c.wrote {
		c.status = http.StatusOK
		c.wrote = true
	}
	c.body = append(c.body, b...)
	return len(b), nil
}

// writeCaptured streams the captured response through the real
// ResponseWriter. If the matching-side status is an error
// (4xx/5xx) we translate the body's {code, message} envelope
// to the api-side ErrorBody so the client sees the same
// wire-level code/message contract as every other endpoint.
func writeCaptured(w http.ResponseWriter, rec *captureWriter) {
	status := rec.status
	if status == 0 {
		status = http.StatusOK
	}
	for k, vs := range rec.header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	if status >= 400 && len(rec.body) > 0 {
		var envelope struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rec.body, &envelope); err == nil && envelope.Code != "" {
			writeError(w, status, ErrorBody{
				Code:    ErrorCode(envelope.Code),
				Message: envelope.Message,
			})
			return
		}
		// Fall-through: forward the raw body — this covers the
		// case where matching emitted a non-JSON error (which
		// shouldn't happen, but we don't want to drop the body).
	}
	w.WriteHeader(status)
	_, _ = w.Write(rec.body)
}
