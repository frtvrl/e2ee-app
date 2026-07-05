// tls_test.go — unit tests for ParseTLSClientHello, ClientHelloFingerprint,
// and FingerprintHex.
//
// What we test:
//
//   - Synthetic TLS 1.2 Client Hello: built byte-by-byte so the test
//     has no dependency on a binary fixture blob. We hand-pick the
//     cipher suites, the (real) SNI hostname, and a couple of
//     extensions. Then we confirm the parsed TLSClientHelloInfo
//     matches expectations.
//
//   - Synthetic TLS 1.3 Client Hello with GREASE values mixed in:
//     confirms GREASE filtering doesn't break the rest of the parse.
//
//   - Truncated / non-TLS / Server-Hello inputs confirm the
//     sentinel-error contract.
//
//   - Fingerprint stability: two distinct clients produce different
//     hex; the same client parsed twice produces identical hex
//     (deterministic fingerprint — JA3-like property).
//
// PRIVACY: no assertion logs or dumps the raw input bytes.
package analysis

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

// synthClientHello builds a single TLS record whose payload is a
// ClientHello. Returns the full record bytes (record header +
// handshake). The caller doesn't need to know the offset of the
// 0x16 marker — just feed the result into ParseTLSClientHello.
//
// Construction is laid out to mirror the on-wire format, so a
// reviewer can verify by hand:
//
//	TLS Record Header  (5 bytes)
//	  0x16               HandshakeContentType
//	  <version, 2 BE>    Record-version (often TLS 1.0 even when
//	                     negotiating 1.2/1.3)
//	  <length, 2 BE>     Body length = len(handshake_body)
//
//	Handshake           (1 + 3 + body bytes)
//	  0x01               ClientHello
//	  <body_length, 3BE>
//	  <body>
//
//	Handshake Body
//	  <client_version, 2BE>      0x0303 = TLS 1.2, 0x0304 = TLS 1.3
//	  <random, 32B>              arbitrary
//	  <session_id_length, 1B>    typically 0
//	  <session_id, ..>
//	  <cipher_suites_length, 2BE>
//	  <cipher_suites, ..>        each suite is 2 bytes
//	  <compression_methods_length, 1B>
//	  <compression_methods, ..>  typically just 0x00 (null)
//	  [extensions_length, 2BE]
//	  [extensions, ..]           TLV stream; optional
func synthClientHello(t *testing.T, opts clientHelloOpts) []byte {
	t.Helper()

	body := &bytes.Buffer{}

	// Client version (record version uses the synthetic 1.0 below).
	binary.Write(body, binary.BigEndian, opts.clientVersion)

	// Random (32 bytes). Fill with a recognisable but stable pattern
	// so fingerprint tests get reproducibility on demand.
	var random [32]byte
	for i := range random {
		random[i] = byte(0x10 + i)
	}
	body.Write(random[:])

	// Session ID — empty.
	body.WriteByte(0x00)

	// Cipher suites — each 2 bytes BE, GREASE already mixed in by
	// the caller (we don't auto-insert).
	if len(opts.cipherSuites)%2 != 0 {
		t.Fatalf("cipherSuites must be an even-length byte slice (got %d)", len(opts.cipherSuites))
	}
	binary.Write(body, binary.BigEndian, uint16(len(opts.cipherSuites)))
	body.Write(opts.cipherSuites)

	// Compression methods: one null method.
	body.WriteByte(0x01)
	body.WriteByte(0x00)

	// Extensions — full TLV stream already built by the caller.
	binary.Write(body, binary.BigEndian, uint16(len(opts.extensions)))
	body.Write(opts.extensions)

	// Handshake prefix: type=1 (ClientHello), 3-byte length.
	hsBody := body.Bytes()
	handshake := &bytes.Buffer{}
	handshake.WriteByte(0x01)
	// 3-byte big-endian length: hi byte 0, then 2-byte BE.
	handshake.WriteByte(0)
	binary.Write(handshake, binary.BigEndian, uint16(len(hsBody)))
	handshake.Write(hsBody)

	// TLS record header.
	rec := &bytes.Buffer{}
	rec.WriteByte(0x16) // ContentType: Handshake
	rec.WriteByte(0x03)
	rec.WriteByte(0x01) // record-version: TLS 1.0 (RFC 5246 §6.2.1)
	binary.Write(rec, binary.BigEndian, uint16(handshake.Len()))
	rec.Write(handshake.Bytes())
	return rec.Bytes()
}

type clientHelloOpts struct {
	clientVersion  uint16        // e.g. 0x0303 (TLS 1.2), 0x0304 (TLS 1.3)
	cipherSuites   []byte        // raw 2-byte-BE suite IDs
	extensions     []byte        // pre-built TLV stream
}

// buildExtTLV is a small helper for tests: type + length + payload.
func buildExtTLV(typ uint16, payload []byte) []byte {
	out := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint16(out[0:2], typ)
	binary.BigEndian.PutUint16(out[2:4], uint16(len(payload)))
	copy(out[4:], payload)
	return out
}

// buildSNIExt builds an SNI extension payload (per RFC 6066 §3) for
// the given hostname. Returns the full extension TLV (so callers can
// append it to the Client Hello extensions stream).
func buildSNIExt(t *testing.T, host string) []byte {
	t.Helper()
	if len(host) == 0 {
		t.Fatal("empty host in buildSNIExt")
	}
	// server_name_list length (2) + entry type (1) + hostname len (2) + hostname
	listLen := uint16(1 + 2 + len(host))
	body := &bytes.Buffer{}
	binary.Write(body, binary.BigEndian, listLen)
	body.WriteByte(0x00) // host_name type
	binary.Write(body, binary.BigEndian, uint16(len(host)))
	body.WriteString(host)
	return buildExtTLV(0x0000, body.Bytes()) // TLSExtServerName = 0
}

// ---------- tests --------------------------------------------------

func TestParseTLSClientHello_BasicTLS12(t *testing.T) {
	opts := clientHelloOpts{
		clientVersion: 0x0303, // TLS 1.2
		cipherSuites: []byte{
			0xc0, 0x2f, // TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA
			0xc0, 0x30, // TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA
			0x00, 0x2f, // TLS_RSA_WITH_AES_128_CBC_SHA (no PFS)
		},
		extensions: buildSNIExt(t, "example.com"),
	}
	raw := synthClientHello(t, opts)

	info, err := ParseTLSClientHello(raw)
	if err != nil {
		t.Fatalf("ParseTLSClientHello: unexpected error: %v", err)
	}
	if info.ProtocolVersion != TLSVersion12 {
		t.Errorf("ProtocolVersion = %q, want %q",
			info.ProtocolVersion, TLSVersion12)
	}
	if got, want := len(info.CipherSuites), 3; got != want {
		t.Errorf("CipherSuites len = %d, want %d", got, want)
	}
	if info.HasGREASE {
		t.Errorf("HasGREASE = true, want false (no GREASE in input)")
	}
	if info.SNI != "example.com" {
		t.Errorf("SNI = %q, want %q", info.SNI, "example.com")
	}
	if info.ExtensionCount != 1 {
		t.Errorf("ExtensionCount = %d, want 1", info.ExtensionCount)
	}
}

func TestParseTLSClientHello_TLS13_WithGREASE(t *testing.T) {
	opts := clientHelloOpts{
		clientVersion: 0x0304, // TLS 1.3
		cipherSuites: []byte{
			0x2a, 0x2a, // GREASE 0x2A2A
			0x13, 0x01, // TLS_AES_128_GCM_SHA256
			0x13, 0x02, // TLS_AES_256_GCM_SHA384
			0x6a, 0x6a, // GREASE 0x6A6A
		},
		extensions: func() []byte {
			var b bytes.Buffer
			// SNI
			b.Write(buildSNIExt(t, "grease.test"))
			// supported_versions (greased)
			b.Write(buildExtTLV(0x002b, []byte{
				0x02, 0x04, 0x03, 0x04, // supported_versions length=2, version=0x0304
			}))
			// another extension with GREASE type (0x3a3a)
			b.Write(buildExtTLV(0x3a3a, []byte{0x00, 0x00}))
			return b.Bytes()
		}(),
	}
	raw := synthClientHello(t, opts)

	info, err := ParseTLSClientHello(raw)
	if err != nil {
		t.Fatalf("ParseTLSClientHello: %v", err)
	}

	if info.ProtocolVersion != TLSVersion13 {
		t.Errorf("ProtocolVersion = %q, want TLSv1.3", info.ProtocolVersion)
	}
	// GREASE filtered out → 2 real suites.
	if got, want := len(info.CipherSuites), 2; got != want {
		t.Errorf("CipherSuites len = %d, want %d (after GREASE filter)", got, want)
	}
	if !info.HasGREASE {
		t.Errorf("HasGREASE = false, want true (GREASE values present)")
	}
	if info.SNI != "grease.test" {
		t.Errorf("SNI = %q, want grease.test", info.SNI)
	}
	// Extensions: SNI (0x0000), supported_versions (0x002b); GREASE ext 0x3a3a filtered.
	if got, want := info.ExtensionCount, 2; got != want {
		t.Errorf("ExtensionCount = %d, want %d", got, want)
	}
}

func TestParseTLSClientHello_Truncated(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"nil", nil},
		{"too-short-header", []byte{0x16, 0x03, 0x01, 0x00}},
		// valid header, length=200 but only 7 bytes of body — mismatch
		{"length-mismatch", []byte{0x16, 0x03, 0x01, 0x00, 0xc8,
			0x01, 0x00}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseTLSClientHello(c.in)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestParseTLSClientHello_ServerHelloRejected(t *testing.T) {
	// Build a valid-ish record but with handshake type = 2
	// (ServerHello, RFC 5246 §7.4.1.3).
	rec := &bytes.Buffer{}
	rec.WriteByte(0x16) // handshake
	rec.WriteByte(0x03)
	rec.WriteByte(0x03)
	body := []byte{
		0x02, // ServerHello
		0x00, 0x00, 0x04, // length
		0x03, 0x03, // version
		0x00, // dummy byte
	}
	rec.WriteByte(byte(len(body) >> 8))
	rec.WriteByte(byte(len(body) & 0xff))
	rec.Write(body)

	_, err := ParseTLSClientHello(rec.Bytes())
	if !errors.Is(err, ErrNotClientHello) {
		t.Errorf("ParseTLSClientHello(ServerHello) err = %v, want wraps %v",
			err, ErrNotClientHello)
	}
}

// TestClientHelloFingerprint_Deterministic confirms the
// fingerprint-on-same-input is stable. Two parses of identical
// bytes must produce the same hex.
func TestClientHelloFingerprint_Deterministic(t *testing.T) {
	opts := clientHelloOpts{
		clientVersion: 0x0303,
		cipherSuites: []byte{0xc0, 0x2f, 0xc0, 0x30},
		extensions:   buildSNIExt(t, "fingerprint.test"),
	}
	raw := synthClientHello(t, opts)

	a, err := ParseTLSClientHello(raw)
	if err != nil {
		t.Fatalf("ParseTLSClientHello: %v", err)
	}
	b, err := ParseTLSClientHello(raw)
	if err != nil {
		t.Fatalf("ParseTLSClientHello: %v", err)
	}

	if got, want := FingerprintHex(a), FingerprintHex(b); got != want {
		t.Errorf("FingerprintHex not deterministic: %q vs %q", got, want)
	}
}

// TestClientHelloFingerprint_DistinguishesClients: two distinct
// Client Hellos produce distinct hexes. Sanity check that the
// fingerprint actually carries a signal.
func TestClientHelloFingerprint_DistinguishesClients(t *testing.T) {
	a := synthClientHello(t, clientHelloOpts{
		clientVersion: 0x0303,
		cipherSuites:  []byte{0xc0, 0x2f, 0xc0, 0x30},
		extensions:    buildSNIExt(t, "a.example"),
	})
	b := synthClientHello(t, clientHelloOpts{
		clientVersion: 0x0303,
		cipherSuites:  []byte{0xc0, 0x2f, 0xc0, 0x30},
		// Same suites, different SNI host → different fingerprint.
		extensions: buildSNIExt(t, "b.example"),
	})

	ia, err := ParseTLSClientHello(a)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ib, err := ParseTLSClientHello(b)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if FingerprintHex(ia) == FingerprintHex(ib) {
		t.Errorf("FingerprintHex(a) == FingerprintHex(b) for distinct hosts")
	}
}

// TestClientHelloFingerprint_NilSafe: a nil *TLSClientHelloInfo
// produces a 16-byte slice of zeroes — defensive default for the
// "telemetry arrived without TLS metadata" path.
func TestClientHelloFingerprint_NilSafe(t *testing.T) {
	fp := ClientHelloFingerprint(nil)
	if len(fp) != 16 {
		t.Fatalf("ClientHelloFingerprint(nil) = %d bytes, want 16", len(fp))
	}
	for i, b := range fp {
		if b != 0 {
			t.Errorf("byte %d = 0x%02x, want 0x00", i, b)
		}
	}
}

// TestFingerprintHex_Format asserts the hex string is exactly 32
// lowercase hex characters — schema's `pattern: ^[a-f0-9]+$`.
func TestFingerprintHex_Format(t *testing.T) {
	opts := clientHelloOpts{
		clientVersion: 0x0303,
		cipherSuites:  []byte{0xc0, 0x2f},
		extensions:    buildSNIExt(t, "x.test"),
	}
	raw := synthClientHello(t, opts)
	info, err := ParseTLSClientHello(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	hex := FingerprintHex(info)
	if len(hex) != 32 {
		t.Errorf("FingerprintHex len = %d, want 32", len(hex))
	}
	if strings.ToLower(hex) != hex {
		t.Errorf("FingerprintHex not lowercase: %q", hex)
	}
	for _, r := range hex {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("FingerprintHex has non-hex char %q in %q", r, hex)
		}
	}
}
