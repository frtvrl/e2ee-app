// Package echobot implements the central Echo-Bot fallback service
// for OpenE2EE (HANDOFF §4.1 PR-5).
//
// CONTEXT
//
// The OpenE2EE measurement flow has three tiers per ADR-0004:
//
//   1. P2P (WebRTC data channel) — preferred, sender ↔ receiver.
//   2. Central Echo-Bot — fallback when no volunteer is available.
//   3. Single-sided — last resort.
//
// This package implements tier 2. When the mobile app can't find a
// P2P peer (no volunteer in the active pool — RISKS §F6), it sends
// its captured test message to the Echo-Bot. The bot:
//
//   1. Re-parses the sender's TLS Client Hello bytes through the
//      same `internal/analysis` pipeline that the sender used
//      locally, computing a fresh `tls_fp` it calls the "bot fp".
//   2. Compares `bot_fp` against the sender's claimed fingerprint.
//      A match means "your local analysis produced a fingerprint
//      consistent with what I get from the bytes you sent" — the
//      loopback integrity check.
//   3. Computes a score on its side (entropy + fingerprint +
//      uniqueness + version) so the sender sees a peer number
//      alongside its own score for cross-checking.
//   4. Echoes the payload back, per the BRD US-4 phrasing
//      "aynı mesaj döner".
//
// DESIGN NOTES
//
//   - The bot is a *single service*, not a connection-oriented
//     protocol. Sprint 1 REST surface (PR-7) calls Handle once per
//     test session; Sprint 2 can introduce streaming.
//   - All fingerprint math delegates to internal/analysis
//     (PR-4). The bot doesn't reimplement Shannon, TLS parsing,
//     or score heuristics.
//   - The bot does NOT log raw TLS bytes, raw payload, or the
//     raw fingerprint output. Only the boolean `matched` and the
//     score are safe to surface in logs (verified by the privacy
//     audit in PR-5's deliverable).
//
// PRIVACY (ADR-0006)
//
// The Echo-Bot sees what the mobile app chooses to send it. The
// sender is responsible for hashing / truncating anything
// identifying before transmission (the bot itself does no
// additional transformation — its only job is to re-run the
// same analysis pipeline). Per ADR-0006 §Backend'de Saklanan,
// the bot does NOT persist anything it sees: no Redis write, no
// Postgres insert, no file write. The Response is the only
// observable side effect.
//
// ERROR CONTRACT
//
//   - ErrNilMessage      — caller bug (the message itself was nil).
//   - ErrEmptyTLSHello   — caller bug (TLS bytes are required).
//   - ErrInvalidTLSHello — wraps analysis.ErrNotTLS / ErrNotClientHello;
//                          the caller should report the test as
//                          malformed to the mobile app.
//   - Other errors come from the analysis package, propagated
//     transparently via fmt.Errorf %w.
package echobot