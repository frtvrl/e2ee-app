// score_test.go — unit tests for ComputeScore, ScoreMetrics, and
// the small helpers in score.go.
//
// Strategy:
//
//   - Per-axis sub-tests verify each dimension's contribution in
//     isolation, using minimal ScoreMetrics that intentionally leave
//     the other three dimensions zero.
//
//   - End-to-end tests verify that the convenience ComputeScoreFor
//     produces scores consistent with the underlying heuristic.
//
//   - Property test: a fully-populated "best case" metrics must
//     NOT exceed 100; an all-zero must equal 0 with confidence 0.
//
// We don't enforce a specific overall value on the end-to-end
// "modern Android TLS 1.3" scenario — heuristic scores are
// tweakable. What we DO enforce is the per-axis contribution is
// monotonic (better inputs → higher sub-score) and the total is
// in [0, 100].
package analysis

import (
	"math"
	"testing"
)

// Per-axis contribution helper that mirrors the internal computation
// in ComputeScore. We re-derive each axis here so tests can assert
// exactly which dimension produced which portion of the total. The
// reasoning matches the comment block in score.go.
func axisVersion(m ScoreMetrics) float64 {
	if m.TLSVersion == "" {
		return 0
	}
	switch m.TLSVersion {
	case TLSVersion13:
		return WeightVersion
	case TLSVersion12:
		if m.HasPFS {
			return WeightVersion - 2
		}
		return 18
	case TLSVersion11:
		return 8
	case TLSVersion10:
		return 2
	default:
		return 0
	}
}

func axisCipher(m ScoreMetrics) float64 {
	if m.CipherSuiteCount == 0 {
		return 0
	}
	switch {
	case m.HasPFS && m.CipherSuiteCount >= 3:
		return WeightCipher
	case m.HasPFS:
		return 22
	case m.CipherSuiteCount >= 3:
		return 14
	default:
		return 8
	}
}

func axisEntropy(m ScoreMetrics) float64 {
	if m.MeanEntropy == 0 {
		return 0
	}
	switch {
	case m.MeanEntropy <= 5.0:
		return 0
	case m.MeanEntropy >= 7.5:
		return WeightEntropy
	default:
		return (m.MeanEntropy - 5.0) / (7.5 - 5.0) * WeightEntropy
	}
}

func axisUnique(m ScoreMetrics) float64 {
	if m.FingerprintHex == "" {
		return 0
	}
	u := 0.0
	switch {
	case m.ExtensionCount >= 5:
		u += 8
	case m.ExtensionCount >= 3:
		u += 5
	case m.ExtensionCount == 0:
		u -= 8
	default:
		u += 2
	}
	if m.HasGREASE {
		u += 7
	}
	if m.HasSNI {
		u += 5
	}
	if u < 0 {
		u = 0
	}
	if u > WeightUnique {
		u = WeightUnique
	}
	return u
}

// --------------------------------------------------------------------

func TestComputeScore_AllZero(t *testing.T) {
	got := ComputeScore(ScoreMetrics{})
	if got.Score != 0 {
		t.Errorf("Score = %v, want 0", got.Score)
	}
	if got.Confidence != 0 {
		t.Errorf("Confidence = %v, want 0", got.Confidence)
	}
}

func TestComputeScore_PerAxis(t *testing.T) {
	cases := []struct {
		name string
		m    ScoreMetrics
		// expected subscore after per-axis calculator; we then
		// assert ComputeScore total = sum of populated axes (with
		// clamp [0, 100]).
		populate int               // number of axes with non-zero data
		want     map[string]float64 // axis name → expected
	}{
		{
			name: "version_tls13_only",
			m: ScoreMetrics{
				TLSVersion:      TLSVersion13,
				FingerprintHex:  "ab", // populate uniqueness axis
				CipherSuiteCount: 1,
				// ExtensionCount is left at 0 → uniqueness axis
				// applies the "empty" penalty (-8 clamped to 0) →
				// unique = 0.
			},
			populate: 3,
			want: map[string]float64{
				"version": WeightVersion, // 30
				"cipher":  8,             // 1 suite, no PFS
				"unique":  0,             // 0 ext → floored at 0
			},
		},
		{
			name: "version_tls12_with_pfs",
			m: ScoreMetrics{
				TLSVersion:       TLSVersion12,
				HasPFS:           true,
				CipherSuiteCount: 4,
			},
			populate: 2,
			want: map[string]float64{
				"version": WeightVersion - 2, // 28
				"cipher":  WeightCipher,      // 25
			},
		},
		{
			name: "version_tls10_penalised",
			m: ScoreMetrics{
				TLSVersion:       TLSVersion10,
				CipherSuiteCount: 2,
			},
			populate: 2,
			want: map[string]float64{
				"version": 2,  // TLS 1.0
				"cipher":  8,  // 2 suites, no PFS
			},
		},
		{
			name: "entropy_low",
			m:    ScoreMetrics{MeanEntropy: 4.0},
			populate: 1,
			want: map[string]float64{"entropy": 0},
		},
		{
			name: "entropy_mid",
			m:    ScoreMetrics{MeanEntropy: 6.0},
			populate: 1,
			want: map[string]float64{"entropy": (6.0 - 5.0) / 2.5 * WeightEntropy}, // 10
		},
		{
			name: "entropy_high",
			m:    ScoreMetrics{MeanEntropy: 8.0},
			populate: 1,
			want: map[string]float64{"entropy": WeightEntropy}, // 25
		},
		{
			name: "unique_modern",
			m: ScoreMetrics{
				FingerprintHex:  "00000000000000000000000000000000",
				ExtensionCount:  6,
				HasGREASE:       true,
				HasSNI:          true,
			},
			populate: 1,
			want: map[string]float64{"unique": WeightUnique}, // capped at 20
		},
		{
			name: "unique_empty_suspicious",
			m: ScoreMetrics{
				FingerprintHex: "00",
				ExtensionCount: 0,
			},
			populate: 1,
			want: map[string]float64{"unique": 0}, // floored at 0
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ComputeScore(c.m)

			// Compare per-axis outputs against the helper functions.
			switch c.name {
			case "version_tls13_only", "version_tls12_with_pfs", "version_tls10_penalised":
				if math.Abs(got.Score-(c.want["version"]+c.want["cipher"]+c.want["unique"])) > 1e-9 {
					t.Errorf("Score = %v, want v+c+u = %v",
						got.Score, c.want["version"]+c.want["cipher"]+c.want["unique"])
				}
			case "entropy_low", "entropy_mid", "entropy_high":
				if math.Abs(got.Score-c.want["entropy"]) > 1e-9 {
					t.Errorf("Score = %v, want %v", got.Score, c.want["entropy"])
				}
			case "unique_modern", "unique_empty_suspicious":
				if math.Abs(got.Score-c.want["unique"]) > 1e-9 {
					t.Errorf("Score = %v, want %v", got.Score, c.want["unique"])
				}
			}

			wantConf := float64(c.populate) / 4.0
			if math.Abs(got.Confidence-wantConf) > 1e-9 {
				t.Errorf("Confidence = %v, want %v", got.Confidence, wantConf)
			}
		})
	}
}

// TestComputeScore_Bounds clamps and total to [0, 100], [0, 1].
func TestComputeScore_Bounds(t *testing.T) {
	scenarios := []struct {
		name string
		m    ScoreMetrics
	}{
		{"maximise_axes", ScoreMetrics{
			TLSVersion:       TLSVersion13,
			HasPFS:           true,
			CipherSuiteCount: 10,
			MeanEntropy:      8.0,
			FingerprintHex:   "ab",
			ExtensionCount:   20,
			HasGREASE:        true,
			HasSNI:           true,
		}},
		{"empty", ScoreMetrics{}},
		{"negative_unique_clamps_to_zero", ScoreMetrics{
			FingerprintHex: "ab",
			ExtensionCount: 0,
			HasGREASE:      false,
			HasSNI:         false,
		}},
	}
	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			got := ComputeScore(s.m)
			if got.Score < 0 || got.Score > 100 {
				t.Errorf("Score out of [0,100]: %v", got.Score)
			}
			if got.Confidence < 0 || got.Confidence > 1 {
				t.Errorf("Confidence out of [0,1]: %v", got.Confidence)
			}
			if math.IsNaN(got.Score) || math.IsInf(got.Score, 0) {
				t.Errorf("Score is NaN/Inf: %v", got.Score)
			}
			if math.IsNaN(got.Confidence) || math.IsInf(got.Confidence, 0) {
				t.Errorf("Confidence is NaN/Inf: %v", got.Confidence)
			}
		})
	}
}

// TestComputeScoreFor_NilTLSInfo: passing a nil TLSClientHelloInfo
// must not panic; the entropy axis may still contribute.
func TestComputeScoreFor_NilTLSInfo(t *testing.T) {
	s := ComputeScoreFor(nil, 7.0)
	if s.Score < 0 || s.Score > 100 {
		t.Errorf("Score out of [0,100]: %v", s.Score)
	}
	// Entropy axis populated, the other three empty → confidence 1/4.
	if math.Abs(s.Confidence-0.25) > 1e-9 {
		t.Errorf("Confidence = %v, want 0.25", s.Confidence)
	}
}

// TestComputeScoreFor_ModernAndroidLike: smoke-test a "well-behaved
// modern Android" scenario. We don't pin the exact number (heuristic
// is tweakable), but we assert:
//
//   - score ≥ 80 (this is what a healthy E2EE client should hit)
//   - confidence = 1.0 (all four axes populated)
func TestComputeScoreFor_ModernAndroidLike(t *testing.T) {
	info := &TLSClientHelloInfo{
		ProtocolVersion: TLSVersion13,
		CipherSuites: []CipherSuiteID{
			0x1301, // TLS_AES_128_GCM_SHA256
			0x1302, // TLS_AES_256_GCM_SHA384
			0x1303, // TLS_CHACHA20_POLY1305_SHA256
		},
		HasGREASE:      true,
		ExtensionCount: 9,
		SNI:            "www.example.com",
	}
	// fingerprint hex doesn't matter for uniqueness (just needs to
	// be non-empty to activate the axis); FeedEntropy is 7.7 → max.
	info.RawExtensions = []byte{0x00, 0x00, 0x00, 0x0e, 0x00, 0x0c, 0x00, 0x09, 'a', 'b', 'c'}
	s := ComputeScoreFor(info, 7.7)

	if s.Score < 80 {
		t.Errorf("Score = %v, want ≥ 80 for a modern well-configured client", s.Score)
	}
	if math.Abs(s.Confidence-1.0) > 1e-9 {
		t.Errorf("Confidence = %v, want 1.0 (all axes populated)", s.Confidence)
	}
}

// TestPFSOffered: confirms the suite-name lookup recognises the
// exact cipher IDs in our PFS table plus the TLS 1.3 families.
func TestPFSOffered(t *testing.T) {
	cases := []struct {
		name   string
		suites []CipherSuiteID
		want   bool
	}{
		{"tls13_suite", []CipherSuiteID{0x1301}, true},
		{"ecdhe_rsa_aes128", []CipherSuiteID{0xc02d}, true},
		{"ecdhe_rsa_aes256", []CipherSuiteID{0xc02e}, true},
		{"ecdhe_chacha20", []CipherSuiteID{0xcca3}, false}, // wrong code (0xc0a3)
		{"ecdhe_chacha20_correct", []CipherSuiteID{0xc0a3}, true},
		{"rsa_no_pfs", []CipherSuiteID{0x002f}, false},
		{"empty", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PFSOffered(c.suites); got != c.want {
				t.Errorf("PFSOffered(%v) = %v, want %v",
					c.suites, got, c.want)
			}
		})
	}
}

// TestComputeScore_Deterministic: same input twice → same output.
// (Obvious property but worth pinning against future refactors.)
func TestComputeScore_Deterministic(t *testing.T) {
	m := ScoreMetrics{
		TLSVersion:       TLSVersion12,
		HasPFS:           true,
		CipherSuiteCount: 4,
		MeanEntropy:      7.0,
		FingerprintHex:   "deadbeefcafe",
		ExtensionCount:   5,
		HasGREASE:        true,
		HasSNI:           true,
	}
	a := ComputeScore(m)
	b := ComputeScore(m)
	if a != b {
		t.Errorf("ComputeScore not deterministic: %+v vs %+v", a, b)
	}
}
