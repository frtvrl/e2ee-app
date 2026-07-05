// service.go — Central Echo-Bot service.
//
// This file defines:
//
//   - Mode constants (matching telemetry.schema.json `match_mode` enum).
//   - Sentinel errors callers match against via errors.Is.
//   - Message:   input to Handle — sender's TLS bytes + payload + their
//                pre-computed fingerprint claim.
//   - Response:  output of Handle — bot's re-computed fingerprint, the
//                match verdict, echo of payload, score + confidence.
//   - Service:   the interface the REST layer (PR-7) depends on.
//   - Bot:       the concrete implementation. Currently the only
//                implementation; injected via NewBot for tests.
//
// The Service interface is kept minimal (one method) so PR-7 can
// wire it into a chi route handler without ceremony, and so future
// Sprint 2 changes (streaming, batching, multi-message) are
// non-breaking.
package echobot

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/opene2ee-com/e2ee-app/backend/internal/analysis"
)

// Mode is the match_mode value the response carries. Hard-coded to
// "echobot" per BRD §8.1 (match_mode enum) and telemetry.schema.json.
const Mode = "echobot"

// Sentinel errors. All callers match via errors.Is — implementation
// never returns a bare error.
var (
	// ErrNilMessage is returned when the caller passes a nil *Message.
	// This is a programming error, not a runtime condition.
	ErrNilMessage = errors.New("echobot: nil message")

	// ErrEmptyTLSHello is returned when the message's TLSHelloBytes
	// is empty. The bot cannot compute a fingerprint without at
	// least a TLS record header (5 bytes per RFC 5246 §6.2.1).
	ErrEmptyTLSHello = errors.New("echobot: empty TLS Client Hello bytes")

	// ErrInvalidTLSHello is returned when ParseTLSClientHello
	// rejects the bytes (not a TLS record, or a TLS record that
	// isn't a Client Hello). It wraps the underlying analysis
	// error so callers can distinguish "not TLS at all" from
	// "TLS but not Client Hello" via errors.Is against
	// analysis.ErrNotTLS / analysis.ErrNotClientHello.
	ErrInvalidTLSHello = errors.New("echobot: invalid TLS Client Hello")
)

// Message is the input to Echo-Bot.Handle.
//
// SenderFP is the fingerprint the mobile app computed on its own
// device before sending. It is the "claim" the Echo-Bot verifies
// against its independent re-computation.
//
// TLSHelloBytes is the raw TLS Client Hello record the sender
// captured from the wire (a single TLS record: 5-byte record
// header + handshake bytes, per RFC 5246 §6.2.1). The bot feeds
// these into analysis.ParseTLSClientHello. Wire-encoded bytes
// preserve any subtle flag/extension state that a parsed struct
// would round-trip away.
//
// Payload is the message body whose entropy the bot scores.
// The bot echoes the bytes back unmodified (per BRD US-4 "aynı
// mesaj döner"). The bot never logs Payload bytes.
type Message struct {
	// SenderFP is the sender's claimed fingerprint in lowercase
	// 32-char hex (the format analysis.FingerprintHex returns).
	// Empty string is allowed — it just means Matched will be
	// false (no claim to verify against).
	SenderFP string

	// TLSHelloBytes is the raw TLS Client Hello record.
	TLSHelloBytes []byte

	// Payload is the message body to echo back + score on.
	Payload []byte
}

// Response is what Echo-Bot.Handle returns.
//
// Fields are exported because the REST handler (PR-7) marshals
// this struct directly to JSON (the schema field names line up with
// telemetry.schema.json).
//
// Privacy: only the boolean `Matched` and the score are safe to
// surface in logs / dashboards. `EchoedPayload` is identical to
// the sender's input by construction — the sender already has it,
// and surfacing it would double the persistence surface (RISKS §F12).
type Response struct {
	// BotFP is the fingerprint the Echo-Bot independently
	// computed from TLSHelloBytes, in lowercase 32-char hex.
	// Same encoding as SenderFP.
	BotFP string

	// Matched is true iff both SenderFP and BotFP are non-empty
	// AND they are byte-equal. False when:
	//   - SenderFP is empty (no claim to verify)
	//   - BotFP is empty (TLS parse failed; in that case Handle
	//     returns an error and the caller never sees this field)
	//   - they differ (sender's local fingerprint disagrees
	//     with the bot's re-computation → integrity issue)
	Matched bool

	// Score is the bot's analysis.ComputeScore result over the
	// parsed TLSClientHelloInfo + payload entropy, in [0, 100].
	Score float64

	// Confidence is the score's confidence in [0, 1].
	Confidence float64

	// EchoedPayload is a copy of Message.Payload, returned
	// verbatim. The bot does NOT mutate it (defensive copy on
	// input + here again — paranoid, but the alternative is
	// sharing a backing array with the caller's lifetime and
	// that's a future debugging nightmare).
	EchoedPayload []byte

	// Mode is always the string "echobot" — the telemetry
	// schema's match_mode enum value for this fallback path.
	Mode string

	// TLSVersion is the version string the bot parsed out
	// (one of "TLSv1.0" .. "TLSv1.3", empty on parse failure).
	// Surfaced so the sender's telemetry row can populate the
	// tls_version field without re-parsing.
	TLSVersion analysis.TLSVersionString
}

// Service is the single dependency the REST layer (PR-7) takes on
// this package. One method — keep it that way for now.
type Service interface {
	// Handle processes one Echo-Bot message and returns the
	// response. See package + type docs for the contract.
	//
	// Errors:
	//   - ErrNilMessage        — caller bug
	//   - ErrEmptyTLSHello     — caller bug
	//   - ErrInvalidTLSHello   — wraps analysis.ErrNotTLS /
	//                            analysis.ErrNotClientHello
	//   - ctx.Err()            — context cancellation / deadline
	Handle(ctx context.Context, msg *Message) (*Response, error)
}

// Compile-time check: Bot must satisfy Service.
var _ Service = (*Bot)(nil)

// Bot is the only implementation of Service in Sprint 1. It has
// no internal state — every Handle call is independent and the
// package holds no caches, no goroutines, no background workers
// (per BRD §NFR-9 the bot must be safe under rate-limiting; the
// simplest way to be safe is to share nothing).
type Bot struct {
	// no fields — see above
}

// NewBot returns a ready-to-use Bot. There is no configuration
// surface in Sprint 1 (the analysis layer is the only dependency
// and it has no knobs per HANDOFF §4.1 PR-4). Future Sprints
// might add things like per-tenant caches here.
func NewBot() *Bot {
	return &Bot{}
}

// Handle implements Service.Handle. See Message / Response docs
// for field semantics. The algorithm:
//
//  1. Validate inputs (nil check, non-empty TLS bytes).
//  2. Parse TLSHelloBytes via analysis.ParseTLSClientHello.
//     - On ErrNotTLS / ErrNotClientHello: wrap as ErrInvalidTLSHello.
//  3. Compute the bot fingerprint via analysis.ClientHelloFingerprint.
//  4. Compute Shannon entropy over Payload via analysis.ShannonEntropy.
//  5. Compute score via analysis.ComputeScoreFor.
//  6. Determine Matched (SenderFP == BotFP, both non-empty).
//  7. Echo Payload bytes (defensive copy).
//  8. Return Response. The response never contains raw TLS bytes
//     or raw payload logging — privacy invariant.
//
// Cancellation: ctx.Err() is checked at the top. The function
// is fast enough that no further cancellation points are needed,
// but we still early-exit on ctx.Err() so a long-running test
// suite doesn't hang on a wedged handler.
func (b *Bot) Handle(ctx context.Context, msg *Message) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, ErrNilMessage
	}
	if len(msg.TLSHelloBytes) == 0 {
		return nil, ErrEmptyTLSHello
	}

	// Step 1: parse the TLS Client Hello. This is where 99% of
	// real failures will surface (caller fed us the wrong byte
	// stream). Re-wrap with our sentinel so callers can match
	// without importing analysis.
	info, err := analysis.ParseTLSClientHello(msg.TLSHelloBytes)
	if err != nil {
		// analysis.* errors are already wrapped by ParseTLSClientHello
		// with %w; we use multi-%w (Go 1.20+) so callers can match
		// both ErrInvalidTLSHello AND the underlying
		// analysis.ErrNotTLS / analysis.ErrNotClientHello via errors.Is.
		return nil, fmt.Errorf("%w: %w", ErrInvalidTLSHello, err)
	}

	// Step 2: compute the bot's fingerprint. analysis.FingerprintHex
	// returns 32 lowercase hex chars; matches the schema's
	// `pattern: ^[a-f0-9]+$` constraint on tls_fp.
	botFP := analysis.FingerprintHex(info)

	// Step 3: entropy + score over payload + parsed TLS info.
	// ComputeScoreFor handles the "all-zero ScoreMetrics" case
	// gracefully (it returns (0, 0) — we surface that).
	score := analysis.ComputeScoreFor(info, analysis.ShannonEntropy(msg.Payload))

	// Step 4: echo. Defensive copy — `bytes.Clone` is in the
	// stdlib since Go 1.20 and is what we want. We don't use
	// the input slice directly even if it's already detached
	// because: (a) the caller might mutate theirs after Handle
	// returns, (b) we don't want our Response to alias the
	// caller's buffer (an accidental modification post-return
	// would silently corrupt our response).
	echoed := bytes.Clone(msg.Payload)
	if echoed == nil {
		// Clone(nil) returns nil — represent "no echo" as an
		// empty (but non-nil) byte slice so the JSON encoding
		// is `[]` rather than `null` (the schema expects an
		// array, and JSON null would fail validation).
		echoed = []byte{}
	}

	// Step 5: determine Matched. Both non-empty AND equal.
	// If the sender didn't claim a fingerprint (SenderFP == ""),
	// Matched is false but the response is still useful — the
	// sender gets the bot's own fingerprint for cross-checking
	// against future tests.
	matched := msg.SenderFP != "" && botFP != "" && msg.SenderFP == botFP

	return &Response{
		BotFP:         botFP,
		Matched:       matched,
		Score:         score.Score,
		Confidence:    score.Confidence,
		EchoedPayload: echoed,
		Mode:          Mode,
		TLSVersion:    info.ProtocolVersion,
	}, nil
}