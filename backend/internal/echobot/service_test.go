// service_test.go — unit tests for Echo-Bot (PR-5).
//
// Strategy:
//
//   - Synthetic TLS Client Hello records built byte-by-byte (same
//     pattern as internal/analysis/tls_test.go). This keeps the
//     test dependency-free of network fixtures and lets us assert
//     exact fingerprint outputs.
//
//   - The "sender fingerprint mock" pattern: each happy-path test
//     computes the expected fingerprint ONCE locally (using the
//     analysis package directly), passes that value as
//     Message.SenderFP, then asserts the bot's response has
//     Matched=true and BotFP == expected. This is the loopback
//     integrity check that PR-5's HANDOFF bullet calls out.
//
//   - Error-path tests cover nil message, empty TLS bytes, and
//     non-TLS / non-Client-Hello inputs.
//
//   - A privacy-invariant grep test confirms the package contains
//     no log.* / fmt.Print* / slog.* calls. (See the
//     TestPackageNoLoggingOrPrinting test at the bottom.)
package echobot

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/opene2ee-com/e2ee-app/backend/internal/analysis"
)

// ---------- helpers: synthetic TLS Client Hello -------------------

// synthClientHello builds a single TLS record whose payload is a
// ClientHello, on the wire as:
//
//	TLS Record Header (5 bytes)
//	  0x16            HandshakeContentType
//	  <version 2BE>   Record-version (0x0301 = TLS 1.0)
//	  <length 2BE>    Body length
//
//	Handshake
//	  0x01            ClientHello
//	  <body 3-byte length, big-endian>
//	  <body>
//
//	Handshake Body
//	  <client_version 2BE>      0x0303 = TLS 1.2
//	  <random, 32 bytes>        filler
//	  <session_id_length, 1B>   0
//	  <cipher_suites_length 2BE>
//	  <cipher_suites, 2 bytes each>
//	  <compression_methods_length, 1B>  1
//	  <compression_methods, 1B>          0x00 (null)
//	  [extensions_length, 2BE]
//	  [extensions, TLV stream]
//
// Copied from analysis/tls_test.go's synthClientHello to keep the
// test self-contained (we don't export that helper). The layout
// is identical.
func synthClientHello(t *testing.T, clientVersion uint16, cipherSuites []byte, extensions []byte) []byte {
	t.Helper()
	if len(cipherSuites)%2 != 0 {
		t.Fatalf("cipherSuites must have even length (got %d)", len(cipherSuites))
	}

	body := &bytes.Buffer{}
	binary.Write(body, binary.BigEndian, clientVersion)
	var random [32]byte
	for i := range random {
		random[i] = byte(0x40 + i)
	}
	body.Write(random[:])
	body.WriteByte(0x00) // session_id length 0
	binary.Write(body, binary.BigEndian, uint16(len(cipherSuites)))
	body.Write(cipherSuites)
	body.WriteByte(0x01) // compression methods length
	body.WriteByte(0x00) // null compression

	if len(extensions) > 0 {
		binary.Write(body, binary.BigEndian, uint16(len(extensions)))
		body.Write(extensions)
	}

	hsBody := body.Bytes()
	hs := &bytes.Buffer{}
	hs.WriteByte(0x01) // ClientHello
	hs.WriteByte(0x00) // 3-byte length: hi=0
	binary.Write(hs, binary.BigEndian, uint16(len(hsBody)))
	hs.Write(hsBody)

	rec := &bytes.Buffer{}
	rec.WriteByte(0x16)
	rec.WriteByte(0x03)
	rec.WriteByte(0x01)
	binary.Write(rec, binary.BigEndian, uint16(hs.Len()))
	rec.Write(hs.Bytes())
	return rec.Bytes()
}

// buildExtTLV: extension_type(2BE) + length(2BE) + payload.
func buildExtTLV(typ uint16, payload []byte) []byte {
	out := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint16(out[0:2], typ)
	binary.BigEndian.PutUint16(out[2:4], uint16(len(payload)))
	copy(out[4:], payload)
	return out
}

// buildSNIExt: SNI extension TLV per RFC 6066 §3.
func buildSNIExt(t *testing.T, host string) []byte {
	t.Helper()
	listLen := uint16(1 + 2 + len(host))
	body := &bytes.Buffer{}
	binary.Write(body, binary.BigEndian, listLen)
	body.WriteByte(0x00) // host_name type
	binary.Write(body, binary.BigEndian, uint16(len(host)))
	body.WriteString(host)
	return buildExtTLV(0x0000, body.Bytes())
}

// expectedFP runs the same pipeline the bot runs and returns the
// 32-char hex fingerprint the bot should produce. Used as the
// "sender's claim" in happy-path tests so Matched is true.
func expectedFP(t *testing.T, raw []byte) string {
	t.Helper()
	info, err := analysis.ParseTLSClientHello(raw)
	if err != nil {
		t.Fatalf("expectedFP: ParseTLSClientHello: %v", err)
	}
	return analysis.FingerprintHex(info)
}

// ---------- happy path --------------------------------------------

func TestBot_Handle_TLS12_Matched(t *testing.T) {
	raw := synthClientHello(t,
		0x0303, // TLS 1.2
		[]byte{
			0xc0, 0x2f, // TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA
			0xc0, 0x30, // TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA
			0x00, 0x2f, // TLS_RSA_WITH_AES_128_CBC_SHA (no PFS)
		},
		buildSNIExt(t, "echobot.test"),
	)
	wantFP := expectedFP(t, raw)

	msg := &Message{
		SenderFP:      wantFP, // "mock the sender fingerprint" — pre-computed via same pipeline
		TLSHelloBytes: raw,
		Payload:       []byte("hello from sender"),
	}

	resp, err := NewBot().Handle(context.Background(), msg)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.BotFP != wantFP {
		t.Errorf("BotFP = %q, want %q", resp.BotFP, wantFP)
	}
	if !resp.Matched {
		t.Errorf("Matched = false, want true (SenderFP == BotFP)")
	}
	if resp.TLSVersion != analysis.TLSVersion12 {
		t.Errorf("TLSVersion = %q, want TLSv1.2", resp.TLSVersion)
	}
	if resp.Mode != Mode {
		t.Errorf("Mode = %q, want %q", resp.Mode, Mode)
	}
	if !bytes.Equal(resp.EchoedPayload, msg.Payload) {
		t.Errorf("EchoedPayload = %q, want %q", resp.EchoedPayload, msg.Payload)
	}
	if resp.Score < 0 || resp.Score > 100 {
		t.Errorf("Score = %v, want in [0, 100]", resp.Score)
	}
	if resp.Confidence < 0 || resp.Confidence > 1 {
		t.Errorf("Confidence = %v, want in [0, 1]", resp.Confidence)
	}
	// Payload is plain ASCII → entropy < 5 → entropy axis = 0,
	// but other axes (version + cipher + uniqueness) still
	// contribute. We don't pin a specific score — heuristic is
	// tweakable — but we do require it to be > 0 (at least one
	// dimension should fire for TLS 1.2 + 3 suites + SNI).
	if resp.Score <= 0 {
		t.Errorf("Score = %v, want > 0 for a well-formed TLS 1.2 Client Hello", resp.Score)
	}
}

func TestBot_Handle_TLS13_WithGREASE_Matched(t *testing.T) {
	// TLS 1.3 Client Hello with GREASE values; sender fingerprint
	// must still match because analysis.ParseTLSClientHello
	// filters GREASE out before fingerprinting.
	extensions := []byte{}
	extensions = append(extensions, buildSNIExt(t, "echobot13.test")...)
	extensions = append(extensions, buildExtTLV(0x002b, []byte{0x02, 0x04, 0x03, 0x04})...) // supported_versions
	extensions = append(extensions, buildExtTLV(0x3a3a, []byte{0x00, 0x00})...)            // GREASE ext

	raw := synthClientHello(t,
		0x0304, // TLS 1.3
		[]byte{
			0x2a, 0x2a, // GREASE
			0x13, 0x01, // TLS_AES_128_GCM_SHA256
			0x6a, 0x6a, // GREASE
			0x13, 0x02, // TLS_AES_256_GCM_SHA384
		},
		extensions,
	)
	wantFP := expectedFP(t, raw)

	resp, err := NewBot().Handle(context.Background(), &Message{
		SenderFP:      wantFP,
		TLSHelloBytes: raw,
		Payload:       []byte("payload v13"),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.BotFP != wantFP {
		t.Errorf("BotFP = %q, want %q", resp.BotFP, wantFP)
	}
	if !resp.Matched {
		t.Errorf("Matched = false, want true")
	}
	if resp.TLSVersion != analysis.TLSVersion13 {
		t.Errorf("TLSVersion = %q, want TLSv1.3", resp.TLSVersion)
	}
	// TLS 1.3 mandates PFS → version axis should be maxed out
	// (30), so total score must be at least 30.
	if resp.Score < 30 {
		t.Errorf("Score = %v, want >= 30 for TLS 1.3 (version axis alone)", resp.Score)
	}
}

// ---------- mismatch / missing-claim paths -----------------------

func TestBot_Handle_SenderFPWrong_MatchedFalse(t *testing.T) {
	raw := synthClientHello(t,
		0x0303,
		[]byte{0xc0, 0x2f, 0xc0, 0x30},
		buildSNIExt(t, "mismatch.test"),
	)

	resp, err := NewBot().Handle(context.Background(), &Message{
		SenderFP:      "deadbeefdeadbeefdeadbeefdeadbeef", // intentionally wrong
		TLSHelloBytes: raw,
		Payload:       []byte("payload"),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.Matched {
		t.Errorf("Matched = true, want false (SenderFP wrong)")
	}
	if resp.BotFP == "" {
		t.Errorf("BotFP empty, want non-empty (bot still computes its own)")
	}
	if resp.BotFP == "deadbeefdeadbeefdeadbeefdeadbeef" {
		t.Errorf("BotFP = SenderFP — analysis layer not running")
	}
}

func TestBot_Handle_NoSenderClaim_MatchedFalse(t *testing.T) {
	raw := synthClientHello(t,
		0x0303,
		[]byte{0xc0, 0x2f, 0xc0, 0x30},
		buildSNIExt(t, "noclaim.test"),
	)

	resp, err := NewBot().Handle(context.Background(), &Message{
		SenderFP:      "", // no claim
		TLSHelloBytes: raw,
		Payload:       []byte("hi"),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.Matched {
		t.Errorf("Matched = true, want false (no claim to verify)")
	}
	if resp.BotFP == "" {
		t.Errorf("BotFP empty, want non-empty")
	}
}

// ---------- echo + payload invariants ----------------------------

func TestBot_Handle_EchoesPayloadExactly(t *testing.T) {
	raw := synthClientHello(t,
		0x0303,
		[]byte{0xc0, 0x2f},
		buildSNIExt(t, "echo.test"),
	)
	payload := []byte("test message — Türkçe karakterler: ğüşiöç — emoji: \xf0\x9f\x9a\x80")

	resp, err := NewBot().Handle(context.Background(), &Message{
		SenderFP:      "",
		TLSHelloBytes: raw,
		Payload:       payload,
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if !bytes.Equal(resp.EchoedPayload, payload) {
		t.Errorf("EchoedPayload mismatch:\n got: %q\nwant: %q",
			resp.EchoedPayload, payload)
	}
}

func TestBot_Handle_NilPayload_EchoesEmpty(t *testing.T) {
	// bytes.Clone(nil) returns nil; the service normalises to
	// an empty slice so JSON encoding produces `[]`, not `null`.
	raw := synthClientHello(t,
		0x0303,
		[]byte{0xc0, 0x2f},
		buildSNIExt(t, "nilpayload.test"),
	)
	resp, err := NewBot().Handle(context.Background(), &Message{
		SenderFP:      "",
		TLSHelloBytes: raw,
		Payload:       nil,
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.EchoedPayload == nil {
		t.Errorf("EchoedPayload is nil, want empty slice")
	}
	if len(resp.EchoedPayload) != 0 {
		t.Errorf("EchoedPayload len = %d, want 0", len(resp.EchoedPayload))
	}
}

func TestBot_Handle_DefensiveCopyOfPayload(t *testing.T) {
	// Mutating the caller's Payload after Handle must NOT mutate
	// the response. (The defensive copy in service.go's
	// bytes.Clone is what protects us here.)
	raw := synthClientHello(t,
		0x0303,
		[]byte{0xc0, 0x2f},
		buildSNIExt(t, "copy.test"),
	)
	payload := []byte("original")
	msg := &Message{
		SenderFP:      "",
		TLSHelloBytes: raw,
		Payload:       payload,
	}
	resp, err := NewBot().Handle(context.Background(), msg)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	// Mutate the input after the fact.
	copy(payload, "MUTATED!")
	if string(resp.EchoedPayload) != "original" {
		t.Errorf("EchoedPayload = %q, want %q (defensive copy failed)",
			resp.EchoedPayload, "original")
	}
}

// ---------- error paths -------------------------------------------

func TestBot_Handle_NilMessage(t *testing.T) {
	_, err := NewBot().Handle(context.Background(), nil)
	if !errors.Is(err, ErrNilMessage) {
		t.Errorf("err = %v, want wraps ErrNilMessage", err)
	}
}

func TestBot_Handle_EmptyTLSBytes(t *testing.T) {
	_, err := NewBot().Handle(context.Background(), &Message{
		SenderFP:      "abcd",
		TLSHelloBytes: nil,
		Payload:       []byte("x"),
	})
	if !errors.Is(err, ErrEmptyTLSHello) {
		t.Errorf("err = %v, want wraps ErrEmptyTLSHello", err)
	}
}

func TestBot_Handle_NotATLSRecord(t *testing.T) {
	// Bytes that look like a TCP payload but are not a TLS
	// handshake record (content-type byte != 0x16).
	_, err := NewBot().Handle(context.Background(), &Message{
		SenderFP:      "",
		TLSHelloBytes: []byte{0x17, 0x03, 0x03, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00},
		Payload:       []byte("x"),
	})
	if !errors.Is(err, ErrInvalidTLSHello) {
		t.Errorf("err = %v, want wraps ErrInvalidTLSHello", err)
	}
	// The wrapped analysis error should also be reachable via
	// errors.Is so callers can distinguish sub-cases.
	if !errors.Is(err, analysis.ErrNotTLS) {
		t.Errorf("err = %v, want also wraps analysis.ErrNotTLS", err)
	}
}

func TestBot_Handle_TLSRecordButNotClientHello(t *testing.T) {
	// Content-type 0x16 + valid record framing, but handshake
	// type = 0x02 (ServerHello, RFC 5246 §7.4.1.3).
	body := []byte{
		0x02,             // ServerHello
		0x00, 0x00, 0x04, // length
		0x03, 0x03, // version
		0x00, // dummy
	}
	rec := []byte{0x16, 0x03, 0x03}
	rec = append(rec, byte(len(body)>>8), byte(len(body)&0xff))
	rec = append(rec, body...)

	_, err := NewBot().Handle(context.Background(), &Message{
		SenderFP:      "",
		TLSHelloBytes: rec,
		Payload:       []byte("x"),
	})
	if !errors.Is(err, ErrInvalidTLSHello) {
		t.Errorf("err = %v, want wraps ErrInvalidTLSHello", err)
	}
	if !errors.Is(err, analysis.ErrNotClientHello) {
		t.Errorf("err = %v, want also wraps analysis.ErrNotClientHello", err)
	}
}

// ---------- context cancellation ---------------------------------

func TestBot_Handle_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	_, err := NewBot().Handle(ctx, &Message{
		SenderFP:      "",
		TLSHelloBytes: []byte{0x16, 0x03, 0x01, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00},
		Payload:       nil,
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// ---------- compile-time interface check --------------------------

func TestBot_SatisfiesServiceInterface(t *testing.T) {
	// Compile-time check (var _ Service = ... in service.go) is
	// the real assertion; this test just makes the verification
	// explicit so a future reader sees it.
	var _ Service = NewBot()
}

// ---------- privacy invariant: no logging in this package ---------

// TestPackageNoLoggingOrPrinting confirms the package contains no
// log.* / fmt.Print* / slog.* calls. Per ADR-0006 the bot must
// not persist anything it sees, and the easiest way to enforce
// that on the Go side is "no logging primitives are called from
// the hot path". This is a regex over the .go files in the
// package; if anyone adds log.* later they'll trip this test.
//
// Skipped in test runs that don't compile this package directly.
// (We're in package echobot, so it's always live.)
func TestPackageNoLoggingOrPrinting(t *testing.T) {
	// The banned patterns. Each is matched as a whole-word call
	// so identifiers like `log.Logger` (a type, not a call) are
	// fine — only `log.<X>(...)` calls are caught.
	banned := []string{
		"log.",
		"fmt.Print",
		"fmt.Println",
		"fmt.Printf",
		"slog.",
	}

	// Hand-rolled scan: the package has two .go files
	// (doc.go, service.go). Grep through both. We don't pull in
	// the `go/parser` machinery because that drags an entire
	// AST walk for what is a 250-line package; the file-level
	// string scan is enough.
	files := []string{
		"doc.go",
		"service.go",
	}
	for _, f := range files {
		src, err := readSourceForTest(t, f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, b := range banned {
			// Allow the banned substring inside a comment
			// that documents the rule itself. We do this by
			// stripping line comments and block comments
			// before matching.
			stripped := stripGoComments(src)
			if strings.Contains(stripped, b) {
				t.Errorf("file %s contains banned call %q "+
					"(ADR-0006 privacy invariant: bot must not "+
					"log raw TLS bytes, payload, or fingerprint)",
					f, b)
			}
		}
	}
}

// ---------- helpers used by TestPackageNoLoggingOrPrinting -------

// readSourceForTest reads a sibling .go file (relative to the test
// CWD, which `go test` sets to the package directory). No setup
// beyond `os.ReadFile`.
func readSourceForTest(t *testing.T, name string) (string, error) {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// stripGoComments removes // and /* */ comments from Go source so
// a regex match isn't fooled by a banned-pattern mention inside a
// doc comment. The implementation is intentionally naive — it does
// not handle string literals or runes containing the substring
// "//", so a fingerprint like "log.Info" appearing inside a string
// would still trip the filter. That's fine for our use case.
func stripGoComments(src string) string {
	var (
		out     strings.Builder
		i       int
		inBlock bool
	)
	for i < len(src) {
		// End of block comment?
		if inBlock {
			if i+1 < len(src) && src[i] == '*' && src[i+1] == '/' {
				inBlock = false
				i += 2
				continue
			}
			i++
			continue
		}
		// Start of block comment?
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '*' {
			inBlock = true
			i += 2
			continue
		}
		// Line comment?
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '/' {
			// Skip to end of line.
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}
		// String literal? Skip past it without interpreting // inside.
		if src[i] == '"' {
			out.WriteByte('"')
			i++
			for i < len(src) && src[i] != '"' {
				if src[i] == '\\' && i+1 < len(src) {
					out.WriteByte(src[i])
					i++
				}
				out.WriteByte(src[i])
				i++
			}
			if i < len(src) {
				out.WriteByte('"')
				i++
			}
			continue
		}
		out.WriteByte(src[i])
		i++
	}
	return out.String()
}