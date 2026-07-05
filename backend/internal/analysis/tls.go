// tls.go — TLS Client Hello parsing + fingerprinting.
//
// What this file does:
//
//   1. ParseTLSClientHello(raw []byte) — feeds a TLS record / TCP
//      segment through gopacket's layers.TLS decoder and extracts the
//      ClientHello fields we care about. Returns ErrNotClientHello
//      if the record isn't a handshake / doesn't have a ClientHello.
//   2. ClientHelloFingerprint(info) — SHA-256 over the concatenation
//      `cipher_suites || extensions`, truncated to 16 bytes (32 hex
//      chars). This is the `tls_fp` field in the telemetry schema.
//
// Why we don't call gopacket's NewPacket() with the lower layers:
//
// gopacket has a layered decode pipeline (Ethernet → IPv4 → TCP →
// TLS). On the mobile side we already have a TCP payload as []byte;
// going through Ethernet/IPv4/TCP adds nothing and creates
// version-dependent header framing. We call
// layers.TLS.DecodeFromBytes directly instead, which is the same
// path gopacket takes after the TCP layer has yielded the payload.
//
// PRIVACY
//
// Every value exposed by TLSClientHelloInfo is metadata the client
// willingly advertises in plain text during the handshake (RFC 5246
// §7.4.1.2 — Client Hello is intentionally public so a server can
// pick a cipher). The fingerprint is a one-way hash, so the cache
// and telemetry stream never carry the raw cipher/extension bytes.
//
// We do NOT expose gopacket's TLS struct (it keeps references into
// the input buffer; we'd rather not let callers keep those alive).
package analysis

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// Sentinel errors for ParseTLSClientHello. All return paths are
// tagged so callers can distinguish "not a TLS record at all" from
// "TLS but not a ClientHello" from "parse error mid-message".
var (
	ErrNotTLS            = errors.New("analysis: input is not a TLS record")
	ErrNotClientHello    = errors.New("analysis: TLS record is not a Client Hello")
	ErrTLSHandshakeParse = errors.New("analysis: Client Hello parse failed")
)

// TLSVersionString mirrors the strings used in
// shared/schemas/telemetry.schema.json `tls_version` enum.
type TLSVersionString string

const (
	TLSVersionUnknown TLSVersionString = ""
	TLSVersion10      TLSVersionString = "TLSv1.0"
	TLSVersion11      TLSVersionString = "TLSv1.1"
	TLSVersion12      TLSVersionString = "TLSv1.2"
	TLSVersion13      TLSVersionString = "TLSv1.3"
)

// TLSExtensionID is a 16-bit IANA extension type. Values with the
// "GREASE" shape (both bytes have 0x?A as their low nibble) are
// filtered out of ExtensionTypes so fingerprint uniqueness scoring
// doesn't miscount.
type TLSExtensionID uint16

// CipherSuiteID is a 16-bit IANA cipher-suite identifier, with the
// same GREASE filter.
type CipherSuiteID uint16

// TLSClientHelloInfo is what ParseTLSClientHello returns. All
// fields are derived metadata; nothing holds a reference into the
// input buffer (gopacket may hold a slice internally, but we copy
// out only the parts we need before returning).
type TLSClientHelloInfo struct {
	// ProtocolVersion is the highest version the client *advertised*
	// (sometimes misleading; real-negotiated version only appears
	// in ServerHello). Matches the schema's `tls_version` enum.
	ProtocolVersion TLSVersionString

	// CipherSuites is the client's preference list, in order,
	// GREASE-filtered. Length ≥ 1 for a well-formed Client Hello.
	CipherSuites []CipherSuiteID

	// Extensions is the parsed extension-type IDs, GREASE-filtered
	// and de-duplicated. Order is preserved.
	Extensions []TLSExtensionID

	// ExtensionCount is len(Extensions) — kept as a separate field
	// so the score layer doesn't have to recompute.
	ExtensionCount int

	// HasGREASE is true if either the cipher-suites list or the
	// extensions contained at least one GREASE value, i.e. a
	// well-behaved modern client (RFC 8701).
	HasGREASE bool

	// SNI is the Server Name Indication extension value, or empty
	// if absent.
	SNI string

	// RawExtensions is a copy of the on-wire extension TLV stream
	// (extension_type || length || payload for each entry). It is
	// what ClientHelloFingerprint feeds into the SHA-256 alongside
	// the cipher-suite list; storing it here keeps the fingerprint
	// bit-stable across changes in the parsed-TLV walker.
	//
	// Empty if the Client Hello had no extensions.
	RawExtensions []byte
}

// ParseTLSClientHello parses a TLS record (typically the bytes that
// follow a TCP header) and extracts the Client Hello fields into
// *TLSClientHelloInfo.
//
// The input must be a complete TLS record — at minimum a record
// header (5 bytes) followed by a handshake message. Truncated
// inputs return one of ErrNotTLS / ErrNotClientHello depending on
// how "far" the parse got:
//
//   - input is shorter than 5 bytes OR the content-type byte is
//     not a TLS handshake (0x16) → ErrNotTLS
//   - input IS a TLS record but the handshake message isn't a
//     Client Hello (e.g. Server Hello, Finished, NewSessionTicket)
//     → ErrNotClientHello
//
// The returned *TLSClientHelloInfo is the caller's to keep; the
// input buffer may be garbage-collected once this returns.
func ParseTLSClientHello(raw []byte) (*TLSClientHelloInfo, error) {
	if len(raw) < 5 {
		return nil, fmt.Errorf("%w: shorter than record header (%d bytes)",
			ErrNotTLS, len(raw))
	}

	// Fast pre-check on the record content-type byte. RFC 5246 §6.2.1
	// says ContentType 0x16 = handshake; everything else (alert,
	// ChangeCipherSpec, application data) is a record-layer non-match
	// and means the caller fed us the wrong byte stream. We don't
	// call into gopacket at all for these.
	if raw[0] != 0x16 {
		return nil, fmt.Errorf("%w: content-type byte 0x%02x (want 0x16)",
			ErrNotTLS, raw[0])
	}

	var tls layers.TLS
	if err := tls.DecodeFromBytes(raw, gopacket.NilDecodeFeedback); err != nil {
		// gopacket reports "Unknown TLS record type" when the
		// content-type byte is invalid — we already masked those
		// above, so any error here is a record-layer framing
		// problem or a handshake the parser couldn't decode (the
		// "Unknown TLS handshake type" path for non-ClientHello
		// handshakes). Both are "we couldn't find a Client Hello
		// in this record", so report ErrNotClientHello.
		return nil, fmt.Errorf("%w: %v", ErrNotClientHello, err)
	}

	// Locate the first ClientHello. The handshake is a repeated
	// (TLSHandshakeRecord, ...) stream — for our telemetry use case
	// we only ever send one ClientHello at a time, but gopacket
	// will accept more in one buffer.
	// Per gopacket's layers.TLS, each HandshakeRecord carries a
	// ClientHello struct (and a stub ClientKeyChange). The handshake
	// type byte (RFC 5246 §7.4) lives inside the ClientHello struct
	// itself — the outer TLSRecordHeader only carries the record-layer
	// ContentType (always TLSType=22 for handshake records).
	//
	// The constant `TLSHandshakeClientHello` (= 1) is declared as
	// untyped int in gopacket; comparing against a uint8 field works
	// because untyped constants auto-convert on comparison.
	var hello *layers.TLSHandshakeRecordClientHello
	for i := range tls.Handshake {
		r := &tls.Handshake[i]
		// Only attempt to read when the parser actually populated
		// ClientHello (the unsupported handshake type would leave
		// CipherSuits empty, and isEncryptedHandshakeMessage would
		// have left everything zero).
		if r.ClientHello.HandshakeType == layers.TLSHandshakeClientHello &&
			len(r.ClientHello.CipherSuits) > 0 {
			hello = &r.ClientHello
			break
		}
	}
	if hello == nil {
		return nil, fmt.Errorf("%w: handshake has no parsed Client Hello",
			ErrNotClientHello)
	}

	info := &TLSClientHelloInfo{
		ProtocolVersion: tlsVersionToString(hello.ProtocolVersion),
		SNI:             string(hello.SNI), // copy; hello holds the input
	}

	// Walk the cipher-suites bytes (each suite is 2 bytes BE) and
	// the raw extensions bytes. Both contain GREASE values we
	// filter out before storing.
	info.CipherSuites, info.HasGREASE = decodeCipherSuites(hello.CipherSuits)
	exts, hasGE := decodeExtensions(hello.Extensions)
	info.HasGREASE = info.HasGREASE || hasGE

	// De-dup extensions while preserving order (most clients do
	// this implicitly, but some have duplicates in malformed
	// captures).
	info.Extensions = dedupOrdered(exts)
	info.ExtensionCount = len(info.Extensions)

	// Copy the on-wire extension stream out of gopacket's buffer so
	// the fingerprint remains valid even after the caller discards
	// the input. The empty slice is a valid "no extensions" value.
	if n := len(hello.Extensions); n > 0 {
		info.RawExtensions = make([]byte, n)
		copy(info.RawExtensions, hello.Extensions)
	}

	return info, nil
}

// ClientHelloFingerprint returns the SHA-256-based 16-byte
// fingerprint used as the `tls_fp` field in telemetry. The input
// is the ordered concatenation (per HANDOFF §4.1 PR-4):
//
//	cipher_suites_as_uint16_be (GREASE-filtered) || on-wire extensions bytes
//
// The cipher suites are re-packed as 16-bit big-endian values so the
// fingerprint is portable (the wire encoding and the parsed list
// agree). The extensions stream is kept raw — re-pack would risk a
// silent drift between what the parser did and what the fingerprint
// saw — and includes any GREASE extension entries that gopacket
// passed through (gopacket's layers.TLS doesn't filter them, our
// `decodeExtensions` does, so the fingerprint intentionally sees a
// stable, on-the-wire view).
//
// Truncated to 16 bytes (32 hex chars). The schema's `tls_fp` `min:
// 16, max: 128` accepts both 32 and 64-hex-char lengths; 32 is the
// JA3-short convention.
func ClientHelloFingerprint(info *TLSClientHelloInfo) []byte {
	if info == nil {
		// Nil-safe: return a 16-byte slice of zeroes. Callers that
		// hand a nil in get a determinate fingerprint rather than
		// panicking — defensive default for the "telemetry arrived
		// without TLS metadata" path.
		return make([]byte, 16)
	}

	// Pre-size to avoid grow/copy in the append path:
	//   2 bytes/cipher-suite + len(RawExtensions)
	bufSize := 2*len(info.CipherSuites) + len(info.RawExtensions)
	buf := make([]byte, 0, bufSize)

	// 2 bytes per cipher suite, BE.
	for _, cs := range info.CipherSuites {
		var b [2]byte
		binary.BigEndian.PutUint16(b[:], uint16(cs))
		buf = append(buf, b[:]...)
	}

	// Then the raw extensions TLV stream as-is on the wire.
	buf = append(buf, info.RawExtensions...)

	sum := sha256.Sum256(buf)
	return sum[:16]
}

// FingerprintHex is a convenience: ClientHelloFingerprint + hex
// encoding. 32 hex chars for the 16-byte value.
func FingerprintHex(info *TLSClientHelloInfo) string {
	fp := ClientHelloFingerprint(info)
	const hexTable = "0123456789abcdef"
	out := make([]byte, len(fp)*2)
	for i, b := range fp {
		out[i*2] = hexTable[b>>4]
		out[i*2+1] = hexTable[b&0x0f]
	}
	return string(out)
}

// --- helpers ---------------------------------------------------------

// tlsVersionToString maps the gopacket TLSVersion code to the schema
// enum. Numbers come from RFC 5246 (TLS 1.0/1.1/1.2) and the
// draft-irtf-tls-rfc8446bis section for 1.3. The case values are
// untyped int literals — Go converts them to TLSVersion (=uint16)
// implicitly on switch.
func tlsVersionToString(v layers.TLSVersion) TLSVersionString {
	switch v {
	case 0x0301:
		return TLSVersion10
	case 0x0302:
		return TLSVersion11
	case 0x0303:
		return TLSVersion12
	case 0x0304:
		return TLSVersion13
	default:
		return TLSVersionUnknown
	}
}

// isGREASE returns true if `v` is one of the 16 GREASE values from
// RFC 8701 (0x0A0A, 0x1A1A, …, 0xFAFA). The defining property is
// "the two bytes are equal and the low nibble of either byte is 0xA".
func isGREASE(v uint16) bool {
	hi := byte(v >> 8)
	lo := byte(v)
	return hi == lo && (hi&0x0f) == 0x0a
}

// decodeCipherSuites parses raw 2-byte-BE cipher-suite values out
// of `raw` and returns the GREASE-filtered list. Returns
// `hasGREASE=true` if at least one filtered value was dropped.
//
// The TLV walk is unnecessary here — cipher suites have no per-item
// length field, they're a packed uint16 array.
func decodeCipherSuites(raw []byte) ([]CipherSuiteID, bool) {
	if len(raw) == 0 || len(raw)%2 != 0 {
		return nil, false
	}
	out := make([]CipherSuiteID, 0, len(raw)/2)
	hasGE := false
	for i := 0; i < len(raw); i += 2 {
		v := binary.BigEndian.Uint16(raw[i : i+2])
		if isGREASE(v) {
			hasGE = true
			continue
		}
		out = append(out, CipherSuiteID(v))
	}
	return out, hasGE
}

// decodeExtensions walks a packed TLV-style extension stream:
//
//	uint16 extension_type;
//	uint16 extension_data_length;
//	uint8  extension_data[extension_data_length];
//
// and returns the GREASE-filtered type IDs in wire order, plus a
// flag indicating if any GREASE values were seen.
//
// We intentionally do NOT parse the per-extension payloads here;
// that's TLS-version-specific and the score layer only needs type
// IDs and presence of SNI (SNI is parsed by gopacket into ClientHello.SNI).
func decodeExtensions(raw []byte) ([]TLSExtensionID, bool) {
	out := []TLSExtensionID{}
	hasGE := false
	for len(raw) >= 4 {
		typ := binary.BigEndian.Uint16(raw[:2])
		lng := binary.BigEndian.Uint16(raw[2:4])
		// Defensive: if a malformed extension claims a length that
		// overruns the buffer, stop walking instead of panicking.
		if int(lng) > len(raw)-4 {
			break
		}
		if isGREASE(typ) {
			hasGE = true
		} else {
			out = append(out, TLSExtensionID(typ))
		}
		raw = raw[4+lng:]
	}
	return out, hasGE
}

// dedupOrdered returns `in` with duplicates removed, preserving the
// order of first appearance. Used because some malformed captures
// contain extension_list with repeated type IDs (handshake
// re-assembly glitch); we don't want duplicate-counted extension
// diversity to skew the uniqueness score.
func dedupOrdered(in []TLSExtensionID) []TLSExtensionID {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[TLSExtensionID]struct{}, len(in))
	out := make([]TLSExtensionID, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
