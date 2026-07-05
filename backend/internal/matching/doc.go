// Package matching implements the OpenE2EE P2P matching and
// WebSocket signalling layer (HANDOFF §4.1 PR-6).
//
// CONTEXT
//
// OpenE2EE's three-tier matching strategy (ADR-0004 §1) puts a
// volunteer-based "Active Pool" at the top of the funnel:
//
//	1. P2P  — sender is matched to a volunteer receiver from the
//	          Active Pool; the two run a WebRTC data channel and the
//	          backend only relays SDP/ICE messages.
//	2. Echo-Bot — central server-side fallback (see internal/echobot,
//	          PR-5) when no volunteer is waiting.
//	3. Single-sided — last resort, sender's measurement only.
//
// This package implements tier 1 — the volunteer Active Pool and
// the signalling channel that lets two matched peers exchange
// WebRTC handshake bytes. The Echo-Bot (PR-5) handles tier 2.
//
// TWO PILLARS
//
//   - Pool        — `pool.go`. A Redis-backed set of currently-
//                   waiting receivers. Receivers register themselves
//                   with a TTL (15 min default). Senders atomically
//                   pop one receiver matching their criteria
//                   (operator, country, app). Atomicity is enforced
//                   by a Lua script that scans a sorted set,
//                   filters, and removes the chosen member in a
//                   single round-trip — preventing two concurrent
//                   senders from getting the same volunteer.
//   - Signalling  — `signalling.go`. A WebSocket-based relay that
//                   exchanges SDP offers / answers and ICE
//                   candidates between two peers. Per ADR-0004 §1,
//                   Sprint 1 is JSON-passthrough only (no Pion
//                   WebRTC integration yet — that lands in Sprint
//                   2). The handler upgrades the HTTP connection,
//                   reads Envelope JSON from the wire, and routes
//                   it to the recipient's inbox through an
//                   in-process Hub. The Hub interface is
//                   intentionally minimal so PR-7 can wire the
//                   route into the chi router without ceremony and
//                   so future Sprints can swap in a clustered hub
//                   (Redis pub/sub) without breaking callers.
//
// RECEIVER METADATA
//
// Receivers don't just sit in a flat list — each one carries
// metadata so the sender can choose a peer whose network context
// (operator, country, app-under-test) is comparable to its own.
// The criteria-aware match is what makes the measurement
// scientifically interesting: a measurement against "any"
// volunteer on any network doesn't tell you whether your local
// app's encryption is actually working — only whether *someone*
// is producing the bytes you expect.
//
// PRIVACY (ADR-0006)
//
// The pool stores anonymous device hashes (already SHA-256-derived
// server-side identifiers from PR-2's auth layer). The signalling
// layer is a dumb pipe: it forwards opaque JSON envelopes without
// inspecting SDP/ICE content. The handler does NOT log raw
// envelopes — only the routing decisions (`from -> to`,
// session_id, message type). Verified by the package's
// `TestPackageNoLoggingOrPrinting` privacy invariant.
//
// REFERENCES
//
//   - HANDOFF.md §4.1 PR-6 (this PR)
//   - ADR-0004 §1 (3-tier strategy, WebRTC, Active Pool flow)
//   - BRD.md FR-2 (system must choose P2P or Echo-Bot fallback)
//   - RISKS.md §F22 (fake receiver / bot attack → reputation
//     score + manual review for first N tests, future Sprint)
//   - RISKS.md §F25 (P2P security: DTLS required + ICE rate-limit
//     enforced at the signalling layer via token-bucket per
//     session — see Hub.throttle)
//   - shared/schemas/p2p-signalling.schema.json (envelope shape)
package matching