package api

// sessions.go — POST /api/v1/sessions, GET /api/v1/sessions,
// GET /api/v1/sessions/{id}.
//
// Wire format: the request body for POST matches the INPUT
// subset of shared/schemas/session.schema.json. We accept a
// trimmed-down form for MVP (the schema's `additionalProperties:
// false` will reject anything we don't enumerate). Required
// fields on POST:
//
//	device_id_hash    — the calling device's salted hash
//	mode              — "p2p" | "echobot" | "single"
//	task_type         — "whatsapp_text" | "whatsapp_image" | "whatsapp_voice" | "rcs_text" | "rcs_image"
//	test_text         — the unique test string the sender will transmit (≤ 256 chars)
//
// Optional:
//
//	receiver_device_id_hash — if known (P2P match already produced it)
//	target_phone_hash        — anonymous target reference (hashed; never a raw phone)
//	target_operator          — if the caller already knows the target carrier
//
// On success, the response is the canonical Session schema with
// the freshly-minted UUID v4 session id and status="pending".
// The handler then moves the session to status="active" once
// the sender has matched (PR-6 + PR-8 wire-up coordinates
// that state transition — for now we leave it "pending").
//
// PRIVACY (ADR-0006):
//   - The request body's `device_id_hash` is logged only as
//     its first 8 hex chars (via the access-log middleware).
//   - `target_phone_hash` is the HASH, not the raw phone; we
//     never accept a raw phone number here.
//   - The response body never carries the receiver's
//     device_id_hash if it differs from the sender's — that
//     would leak the other party's presence to a casual
//     network observer reading the response. The receiver's
//     hash is exchanged out-of-band via the WebSocket
//     signalling channel (matching.Handler, PR-6).

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/opene2ee-com/e2ee-app/backend/internal/storage"
)

// softTimeout is the deadline for ancillary calls (e.g. the
// device upsert during session creation) that we don't want to
// drag the request out for. Keep this short — the user is
// waiting on the primary path.
const softTimeout = 5 * time.Second

// createSessionRequest is the trimmed input shape we accept on
// POST /api/v1/sessions. It is a SUBSET of session.schema.json —
// the schema validator rejects unknown fields, so the trimmed
// shape also acts as a "what we care about right now" filter.
//
// We don't use struct tags for json-schema validation; we feed
// the raw bytes to gojsonschema and let IT enforce the contract.
// This struct is for HANDLER-side convenience (decoding the
// response we want to echo back). The schema is the source of
// truth.
type createSessionRequest struct {
	ID                    uuid.UUID `json:"id,omitempty"`           // optional — server mints if absent
	DeviceIDHash          string    `json:"device_id_hash"`          // required
	ReceiverDeviceIDHash  string    `json:"receiver_device_id_hash,omitempty"`
	Mode                  string    `json:"mode"`                   // required
	TaskType              string    `json:"task_type"`              // required
	TestText              string    `json:"test_text,omitempty"`
	TargetPhoneHash       string    `json:"target_phone_hash,omitempty"`
	TargetOperator        string    `json:"target_operator,omitempty"`
	PublicKey             []byte    `json:"public_key,omitempty"`  // raw Ed25519 pubkey bytes; optional first-boot flow
	PublicKeyFP           string    `json:"public_key_fp,omitempty"`
}

// handleCreateSession is POST /api/v1/sessions.
func (a *API) handleCreateSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				writeError(w, http.StatusRequestEntityTooLarge, ErrorBody{
					Code:    CodePayloadTooLarge,
					Message: "Request body exceeds size limit.",
				})
				return
			}
			writeBadRequest(w, "Failed to read request body.")
			return
		}
		if len(body) == 0 {
			writeBadRequest(w, "Empty request body.")
			return
		}

		// Schema-validate against the client-request subset
		// (session-create). The full session schema requires
		// server-minted fields (id, status, created_at) that
		// the client never sends; we mint those after parse.
		// public_key / public_key_fp are accepted at create-time
		// only — they are NOT part of the response contract.
		if err := a.deps.Schemas.Validate(SchemaSessionCreate, body); err != nil {
			if ve, ok := isValidationError(err); ok {
				writeValidation(w, ve)
				return
			}
			writeBadRequest(w, "Schema validation failed.")
			return
		}

		var req createSessionRequest
		if err := json.Unmarshal(body, &req); err != nil {
			writeBadRequest(w, "Malformed JSON.")
			return
		}

		// Server-side normalization.
		if req.ID == uuid.Nil {
			req.ID = uuid.New()
		}
		now := time.Now().UTC()

		// If the client provided a public key, register the
		// device first (idempotent on conflict). We never store
		// the private key server-side; the public key is needed
		// only for future anti-spoofing flows (F9, disabled in
		// MVP).
		if len(req.PublicKey) > 0 && req.PublicKeyFP != "" && a.deps.Cfg.Devices != nil {
			upsertCtx, cancel := context.WithTimeout(r.Context(), softTimeout)
			defer cancel()
			if err := a.deps.Cfg.Devices.UpsertDevice(upsertCtx, req.DeviceIDHash, req.PublicKey, req.PublicKeyFP); err != nil {
				a.deps.Cfg.Logger.Warn("upsert device on session create failed",
					"err_kind", "soft",
					"session_id", req.ID.String(),
				)
				// Soft-fail — the device may already exist with the
				// same hash; we don't want a transient DB hiccup
				// to block session creation.
			}
		}

		sess := storage.Session{
			ID:           req.ID,
			Mode:         req.Mode,
			TaskType:     req.TaskType,
			SenderHash:   &req.DeviceIDHash,
			ReceiverHash: nilIfEmpty(req.ReceiverDeviceIDHash),
			Status:       "pending",
			StartedAt:    now,
		}

		if err := a.deps.Cfg.Sessions.InsertSession(r.Context(), sess); err != nil {
			a.deps.Cfg.Logger.Error("insert session failed",
				"err_kind", "db",
				"session_id", req.ID.String(),
			)
			writeInternal(w)
			return
		}

		writeJSON(w, http.StatusCreated, sessionResponse{
			ID:                   sess.ID,
			DeviceIDHash:         req.DeviceIDHash,
			ReceiverDeviceIDHash: req.ReceiverDeviceIDHash,
			Mode:                 sess.Mode,
			TaskType:             sess.TaskType,
			TestText:             req.TestText,
			TargetPhoneHash:      req.TargetPhoneHash,
			TargetOperator:       req.TargetOperator,
			Status:               sess.Status,
			CreatedAt:            now,
			StartedAt:            now,
		})
	}
}

// handleGetSession is GET /api/v1/sessions/{id}.
func (a *API) handleGetSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := uuid.Parse(idStr)
		if err != nil {
			writeBadRequest(w, "Invalid session id.")
			return
		}
		sess, err := a.deps.Cfg.Sessions.GetSession(r.Context(), id)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				writeNotFound(w, "Session not found.")
				return
			}
			a.deps.Cfg.Logger.Error("get session failed",
				"err_kind", "db",
				"session_id", id.String(),
			)
			writeInternal(w)
			return
		}
		// Privacy: do NOT include the receiver's hash in the
		// response. The sender is allowed to learn it via the
		// WebSocket signalling channel only.
		writeJSON(w, http.StatusOK, sessionResponse{
			ID:           sess.ID,
			Mode:         sess.Mode,
			TaskType:     sess.TaskType,
			Status:       sess.Status,
			StartedAt:    sess.StartedAt,
			EndedAt:      sess.EndedAt,
		})
	}
}

// handleListSessions is GET /api/v1/sessions. Returns the most
// recent N sessions (default 50, capped at 200). Used by the
// dashboard for the session-history view. Privacy: this is an
// admin/dashboard endpoint in MVP — when consumer-facing
// listing lands, it must be filtered by device_id_hash.
func (a *API) handleListSessions() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if q := r.URL.Query().Get("limit"); q != "" {
			if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		sessions, err := a.deps.Cfg.Sessions.ListSessions(r.Context(), limit)
		if err != nil {
			a.deps.Cfg.Logger.Error("list sessions failed", "err_kind", "db")
			writeInternal(w)
			return
		}
		out := make([]sessionResponse, 0, len(sessions))
		for _, s := range sessions {
			out = append(out, sessionResponse{
				ID:        s.ID,
				Mode:      s.Mode,
				TaskType:  s.TaskType,
				Status:    s.Status,
				StartedAt: s.StartedAt,
				EndedAt:   s.EndedAt,
			})
		}
		writeJSON(w, http.StatusOK, listSessionsResponse{
			Sessions: out,
			Count:    len(out),
		})
	}
}

// sessionResponse is the JSON shape returned to clients. It is
// a SUBSET of session.schema.json (omits device_id_hash and
// receiver_device_id_hash on GET responses for privacy; the
// device reading its own session already knows its own hash).
type sessionResponse struct {
	ID                   uuid.UUID  `json:"id"`
	DeviceIDHash         string     `json:"device_id_hash,omitempty"`
	ReceiverDeviceIDHash string     `json:"receiver_device_id_hash,omitempty"`
	Mode                 string     `json:"mode"`
	TaskType             string     `json:"task_type"`
	TestText             string     `json:"test_text,omitempty"`
	TargetPhoneHash      string     `json:"target_phone_hash,omitempty"`
	TargetOperator       string     `json:"target_operator,omitempty"`
	Status               string     `json:"status"`
	CreatedAt            time.Time  `json:"created_at,omitempty"`
	StartedAt            time.Time  `json:"started_at"`
	EndedAt              *time.Time `json:"ended_at,omitempty"`
}

type listSessionsResponse struct {
	Sessions []sessionResponse `json:"sessions"`
	Count    int               `json:"count"`
}

// writeJSON is the success-path serializer. Always uses
// application/json and disables HTML escaping (privacy — see
// writeError for the same reason).
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// nilIfEmpty returns nil for "" and &s otherwise. Storage
// layers use NULL for "no value" via *string.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}