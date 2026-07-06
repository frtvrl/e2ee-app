package api

// users.go — DELETE /api/v1/users/{device_id_hash}.
//
// KVKK (BRD §8 FR-7) + GDPR Art. 17 right to erasure. The 7-day
// SLA is operational; this handler gives the synchronous path.
//
// PRIVACY (ADR-0006 §KVKK + RISKS §E3):
//   - The URL path carries the salted device_id_hash, NOT the
//     raw UUID v7. A raw UUID in the URL would log alongside
//     the request line and could be correlated across
//     services.
//   - The handler runs the relational delete (Postgres + Timescale)
//     first. On success it fires the DeleteUserHook (Redis-side
//     purge of the Active Pool, operator cache entries, etc.).
//   - Hook failures are LOGGED but do NOT roll back the
//     relational delete — the user already exercised their
//     right to erasure and a transient Redis outage must not
//     undo it. The 7-day SLA is upheld by the next periodic
//     sweeper (PR-13 / cron, not in PR-7).
//   - The response body is intentionally minimal — just
//     {deleted:true, device_id_hash:<hash>}. We do NOT echo
//     any record of what was deleted (the mobile UI doesn't
//     need it).

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// deleteUserResponse is the success body. The mobile UI can
// use this to navigate the user back to a "your data is gone"
// screen.
type deleteUserResponse struct {
	Deleted      bool   `json:"deleted"`
	DeviceIDHash string `json:"device_id_hash"`
}

// handleDeleteUser is DELETE /api/v1/users/{device_id_hash}.
func (a *API) handleDeleteUser() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hash := chi.URLParam(r, "device_id_hash")
		if !isValidDeviceHash(hash) {
			writeBadRequest(w, "device_id_hash must be 16-64 lowercase hex characters")
			return
		}

		// (1) Hard-delete from relational storage. This is
		// the synchronous right-to-erasure step.
		if err := a.deps.Cfg.Users.DeleteUser(r.Context(), hash); err != nil {
			a.deps.Cfg.Logger.Error("delete user failed",
				"err_kind", "db",
			)
			writeInternal(w)
			return
		}

		// (2) Fire the Redis-side purge hook if configured.
		// Failures are logged but do not surface as 5xx — see
		// package doc on Config.DeleteUserHook.
		if a.deps.Cfg.DeleteUserHook != nil {
			hookCtx, cancel := context.WithTimeout(r.Context(), softTimeout)
			defer cancel()
			if err := a.deps.Cfg.DeleteUserHook(hookCtx, hash); err != nil {
				a.deps.Cfg.Logger.Warn("delete-user hook failed; relying on sweeper",
					"err_kind", "hook",
				)
			}
		}

		writeJSON(w, http.StatusOK, deleteUserResponse{
			Deleted:      true,
			DeviceIDHash: hash,
		})
	}
}

// isValidDeviceHash enforces the on-the-wire shape of the
// device_id_hash path parameter. Mirrors auth.TruncateHexLen
// (32 chars) as the lower bound; matches the schema's maxLength
// (64 chars) as the upper bound. Hex-only (matching the schema's
// pattern) so a phone-number-shaped injection can't sneak in.
func isValidDeviceHash(s string) bool {
	if len(s) < 16 || len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}