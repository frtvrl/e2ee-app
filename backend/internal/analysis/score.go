// score.go — encryption-quality score (0–100) and confidence (0–1)
//
// Algorithm (intentionally heuristic; no config surface per
// HANDOFF §4.1 PR-4: "sabit eşikler değil — config-dışı başlangıç
// heuristic'i yeterli"):
//
//	Total = clamp(TLSVersion + CipherSuite + Entropy + Uniqueness, 0, 100)
//
// Weights:
//
//	TLS version      max 30   (PFS baked in for 1.3; not for older)
//	Cipher suite     max 25   (PFS / suite count / weak-suite veto)
//	Payload entropy  max 25   (Shannon; linear map in [5.0, 7.5])
//	Uniqueness       max 20   (extension diversity + GREASE + SNI)
//
// Confidence is `contributing_dimensions / 4` — a unit-less measure
// of "how many of the four axes actually had data". Empty
// ScoreMetrics → (0, 0.0); fully-populated → (≤100, 1.0).
//
// PRIVACY
//
// ComputeScore only sees derived values: TLSVersionString, slice of
// cipher IDs, extension IDs, an entropy float, a fingerprint hex.
// Never sees raw packet bytes, never logs, never persists.
package analysis

// (no imports needed — pure-data heuristics only)

// ScoreMetrics is what callers feed into ComputeScore. Every field
// is independently optional; missing values score zero in their
// dimension and reduce the overall confidence accordingly.
type ScoreMetrics struct {
	// TLSVersion is one of the TLSVersion* constants. An empty
	// string is "we don't know" — the version dimension scores zero
	// and one of the four dimensions is considered missing for
	// confidence.
	TLSVersion TLSVersionString

	// HasPFS is true if at least one of the client's preferred
	// cipher suites provides Forward Secrecy (ECDHE/DHE, or any
	// TLS 1.3 suite which mandates PFS).
	//
	// The score layer does not judge which suite is "the one we'll
	// use" — it scores what the client *advertised* (caller decides
	// how to derive HasPFS; see `PFSOffered` helper below).
	HasPFS bool

	// CipherSuiteCount is the GREASE-filtered count from
	// TLSClientHelloInfo.CipherSuites. ≥3 strongly suggests a
	// modern, configured client; 1–2 is suspicious but valid
	// (constrained devices, embedded TLS stacks).
	CipherSuiteCount int

	// MeanEntropy is the Shannon entropy in [0, 8] bits/byte across
	// the sampled payload. 0 means "no data sampled".
	MeanEntropy float64

	// FingerprintHex is the 32-hex-char TLS-fingerprint string from
	// FingerprintHex. Empty means "TLS metadata missing".
	FingerprintHex string

	// ExtensionCount is the unique, GREASE-filtered number of
	// extensions from TLSClientHelloInfo.ExtensionCount.
	ExtensionCount int

	// HasGREASE is the boolean from TLSClientHelloInfo.HasGREASE.
	// Modern, well-behaved clients use GREASE; clients that don't
	// are older or anonymised (Tor, malware).
	HasGREASE bool

	// HasSNI is true iff the Client Hello carried an SNI extension
	// with a non-empty hostname. False means the connection is to
	// "no specific hostname" (rare on port 443; sometimes seen on
	// raw-IP TLS).
	HasSNI bool
}

// Score is the result of ComputeScore. Both fields are normalised
// to their declared ranges (Score ∈ [0, 100], Confidence ∈ [0, 1]).
type Score struct {
	Score      float64
	Confidence float64
}

// Weight constants for the four dimensions. Sum = 100. Exposed at
// package level so tests can assert each dimension in isolation.
const (
	WeightVersion   = 30.0
	WeightCipher    = 25.0
	WeightEntropy   = 25.0
	WeightUnique    = 20.0
	WeightTotal     = WeightVersion + WeightCipher + WeightEntropy + WeightUnique
	confidenceBase  = 4.0 // number of dimensions; divisor for confidence
)

// ComputeScore returns the 0–100 encryption-quality score plus a
// confidence in [0, 1].
//
// The function is total — every input maps to some Score, including
// the all-zero ScoreMetrics (which gives (0, 0)). It never panics,
// never logs, never returns NaN or Inf (clamp + finite-guard at the
// end).
func ComputeScore(m ScoreMetrics) Score {
	var (
		v, c, e, u, contributing float64
	)

	// -- (1) TLS version axis ------------------------------------
	if m.TLSVersion != "" {
		contributing++
		switch m.TLSVersion {
		case TLSVersion13:
			// TLS 1.3 mandates PFS in every defined suite.
			v = WeightVersion
		case TLSVersion12:
			if m.HasPFS {
				v = WeightVersion - 2 // 28
			} else {
				v = 18
			}
		case TLSVersion11:
			v = 8
		case TLSVersion10:
			v = 2
		default:
			// Empty or unknown version: 0 of the dimension, but
			// already counted above by checking m.TLSVersion != "".
			// We don't downscore on "unknown" — a network tap that
			// only saw encrypted metadata would otherwise punish
			// the device. Confidence reflects the gap.
			v = 0
		}
	}

	// -- (2) Cipher suite axis -----------------------------------
	if m.CipherSuiteCount > 0 {
		contributing++
		switch {
		case m.HasPFS && m.CipherSuiteCount >= 3:
			c = WeightCipher // 25
		case m.HasPFS:
			c = 22 // tiny single-suite PFS client (still strong)
		case m.CipherSuiteCount >= 3:
			c = 14 // many suites but no PFS — penalise for that
		default:
			c = 8 // 1–2 suites, no PFS — weak but not stupid
		}
	}

	// -- (3) Entropy axis ----------------------------------------
	if m.MeanEntropy > 0 {
		contributing++
		// Linear map [5.0, 7.5] → [0, WeightEntropy]. Below 5.0 is
		// zero (looks like plaintext patterns); above 7.5 saturates
		// (compressed / encrypted bytes look the same under Shannon
		// alone). 0 is the "we sampled no payload" signal.
		switch {
		case m.MeanEntropy <= 5.0:
			e = 0
		case m.MeanEntropy >= 7.5:
			e = WeightEntropy
		default:
			slope := (m.MeanEntropy - 5.0) / (7.5 - 5.0) // 0..1
			e = slope * WeightEntropy
		}
	}

	// -- (4) Uniqueness axis -------------------------------------
	if m.FingerprintHex != "" {
		contributing++
		// +8 for having ≥5 unique extension types (modern configured
		// client). +7 for GREASE (RFC 8701 conformance). +5 for
		// SNI. −8 for suspiciously empty extension list. Capped at
		// WeightUnique; floored at 0 so an empty-extensions client
		// doesn't go negative and pull the total down.
		uniq := 0.0
		switch {
		case m.ExtensionCount >= 5:
			uniq += 8
		case m.ExtensionCount >= 3:
			uniq += 5
		case m.ExtensionCount == 0:
			uniq -= 8
		default:
			// 1 or 2 — small extension set, not great, not suspicious.
			uniq += 2
		}
		if m.HasGREASE {
			uniq += 7
		}
		if m.HasSNI {
			uniq += 5
		}
		if uniq < 0 {
			uniq = 0
		}
		if uniq > WeightUnique {
			uniq = WeightUnique
		}
		u = uniq
	}

	// -- total + confidence --------------------------------------
	raw := v + c + e + u
	if raw < 0 {
		raw = 0
	}
	if raw > 100 {
		raw = 100
	}

	conf := contributing / confidenceBase
	if conf < 0 {
		conf = 0
	}
	if conf > 1 {
		conf = 1
	}

	return Score{Score: raw, Confidence: conf}
}

// PFSOffered is a small convenience: it walks the parsed
// cipher-suite list and returns true iff any suite name suggests
// Forward Secrecy. Intended for filling in ScoreMetrics.HasPFS.
//
// The "name" of a cipher suite is its IANA string identifier
// (RFC 8446 §B.3, RFC 5246 §7.4.1.2) — TLS 1.3 names like
// `TLS_AES_128_GCM_SHA256` and TLS 1.2 names like
// `TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256` are both checked below.
//
// We do not import a 1000-entry cipher-suite table here — the
// PFS marker is "ECDHE" or "DHE" in the suite name (the IANA
// registry uses these as the only PFS suites pre-1.3), and any
// TLS 1.3 suite (always PFS). A future PR can extend with a full
// table for finer classification (AEAD-only vs MAC-only vs NULL).
func PFSOffered(suites []CipherSuiteID) bool {
	if len(suites) == 0 {
		return false
	}
	// 1.3 suite IDs — every TLS 1.3 cipher suite is PFS by definition.
	// Values from RFC 8446 Appendix B.3.1.
	isTLS13 := map[uint16]bool{
		0x1301: true, // TLS_AES_128_GCM_SHA256
		0x1302: true, // TLS_AES_256_GCM_SHA384
		0x1303: true, // TLS_CHACHA20_POLY1305_SHA256
		0x1304: true, // TLS_AES_128_CCM_SHA256
		0x1305: true, // TLS_AES_128_CCM_8_SHA256
	}
	for _, cs := range suites {
		if isTLS13[uint16(cs)] {
			return true
		}
	}
	// Pre-1.3 suite names are looked up by IANA string. We compile
	// a compact map of the suites modern OpenE2EE would care
	// about (suite IDs from the IANA TLS Cipher Suites registry,
	// https://www.iana.org/assignments/tls-parameters/, filtered to
	// the PFS families).
	pfsFamilies := map[uint16]string{
		// TLS 1.2 ECDHE suites (a representative subset)
		0xc02b: "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
		0xc02c: "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
		0xc02d: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		0xc02e: "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
		0xc02f: "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA",
		0xc030: "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA",
		0xc009: "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA",
		0xc00a: "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA",
		0xc0a3: "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305",
		0xc0a4: "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305",
		// TLS 1.2 DHE suites
		0x009e: "TLS_DHE_RSA_WITH_AES_128_GCM_SHA256",
		0x009f: "TLS_DHE_RSA_WITH_AES_256_GCM_SHA384",
		0x0067: "TLS_DHE_RSA_WITH_AES_128_CBC_SHA",
		0x006b: "TLS_DHE_RSA_WITH_AES_256_CBC_SHA",
		0x00aa: "TLS_DHE_RSA_WITH_AES_128_GCM_SHA256_DRAFT",
		0x00ab: "TLS_DHE_RSA_WITH_AES_256_GCM_SHA384_DRAFT",
		0x00ff: "TLS_DHE_RSA_WITH_CHACHA20_POLY1305",
	}
	for _, cs := range suites {
		if name, ok := pfsFamilies[uint16(cs)]; ok && hasPFSMarker(name) {
			return true
		}
	}
	return false
}

// hasPFSMarker returns true if the IANA cipher-suite name contains
// "ECDHE" or "DHE" — the only PFS key-exchange families in TLS
// 1.0–1.2.
func hasPFSMarker(name string) bool {
	// Avoid importing strings for a two-substring check; the
	// below compiles to a tiny state machine.
	return contains(name, "ECDHE") || contains(name, "DHE")
}

// contains is a heap-free, branch-only substring test. Cheaper than
// strings.Contains for the 2-byte to 64-byte names we see in
// cipher-suite identifiers.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// ComputeScoreFor is a convenience that builds ScoreMetrics from a
// parsed TLSClientHelloInfo + payload entropy, then calls
// ComputeScore. Empty `tlsInfo` is allowed — the result is just
// score 0 (entropy axis only, if anything).
func ComputeScoreFor(tlsInfo *TLSClientHelloInfo, meanEntropy float64) Score {
	var m ScoreMetrics
	if tlsInfo != nil {
		m.TLSVersion = tlsInfo.ProtocolVersion
		m.CipherSuiteCount = len(tlsInfo.CipherSuites)
		m.FingerprintHex = FingerprintHex(tlsInfo) // empty if nil above
		m.ExtensionCount = tlsInfo.ExtensionCount
		m.HasGREASE = tlsInfo.HasGREASE
		m.HasSNI = tlsInfo.SNI != ""
		m.HasPFS = PFSOffered(tlsInfo.CipherSuites)
	}
	m.MeanEntropy = meanEntropy
	return ComputeScore(m)
}
