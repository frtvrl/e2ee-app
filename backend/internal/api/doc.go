// Package api implements the OpenE2EE REST surface (HANDOFF.md §4.1
// PR-7).
//
// Routes (all under /api/v1/):
//
//	POST   /api/v1/sessions                       — create test session
//	GET    /api/v1/sessions                       — list recent sessions
//	GET    /api/v1/sessions/{id}                  — fetch one session
//	POST   /api/v1/sessions/{id}/telemetry        — submit telemetry row
//	GET    /api/v1/matrix                         — transparency matrix (filtered)
//	GET    /api/v1/operator/lookup                — phone/IP → operator
//	DELETE /api/v1/users/{device_id_hash}         — KVKK hard-delete
//
// Non-API surface (mounted by PR-8 wire-up):
//
//	GET    /healthz                               — JSON liveness
//
// PRIVACY CONTRACT (ADR-0006 §Veri Minimizasyonu)
//
// The api package is the LAST line of defense before user data hits
// storage. Every byte that crosses this boundary is governed by the
// following invariants — verified at code-review and by
// TestPackageNoForbiddenFieldsInLogs below.
//
//  1. The request body is NEVER written to a log line. The struct
//     decoded from it is never serialized into a debug log either.
//     Bodies are accepted as io.Reader and bounded by a max-bytes
//     reader (MaxBytesReader) before any handler touches them.
//  2. Raw IPs are NEVER logged. The middleware that selects the
//     rate-limit bucket uses the IP only as an in-memory map key
//     and discards it after the bucket is found.
//  3. Raw UUIDs are NEVER logged. The device identifier that
//     crosses the wire is the salted SHA-256 hash
//     (auth.HashDeviceID), and that is the only form the package
//     ever sees or emits.
//  4. Phone numbers, e-mail addresses and other PII are NEVER
//     logged. The operator lookup handler logs the QUERY TYPE
//     (phone_e164 / ip_v4) but never the value. Validation
//     failures return generic 400 messages.
//  5. Karşı numara (the "other side" of a test session) is NOT
//     represented in this package at all. If a session record
//     ever needs the other party, it carries only that party's
//     device_id_hash (the salted hash, not their UUID v7 nor
//     their phone number).
//  6. The X-API-Version request header is required (1). Missing
//     or unsupported versions get a 400 before any handler runs.
//
// SCOPE (this PR)
//   - chi router + middleware wiring
//   - request decoding + JSON-Schema validation (gojsonschema)
//   - 7 handlers above
//   - rate-limit middleware (per-device, in-memory token bucket)
//   - CORS middleware (production origins allowlist)
//   - access-log middleware (privacy-preserving fields only)
//
// OUT OF SCOPE (deferred to PR-8 wire-up)
//   - Actually instantiating storage / operator / matching services
//     and injecting them into New(...) — wire-up does that.
//   - WebSocket signalling handler (that lives in matching).
//   - The /healthz body extension that pings Postgres + Redis.
package api

// Compile-time privacy audit — every .go file in this package
// MUST contain a comment asserting the privacy contract. The
// TestPackagePrivacyInvariants test in middleware_test.go
// re-verifies this at every `go test` run.