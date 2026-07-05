// Package analysis implements the OpenE2EE gopacket analysis layer
// (HANDOFF §4.1 PR-4). It is the *only* place where raw packet bytes
// from client devices turn into metadata the backend is willing to
// look at — entropy, a TLS Client Hello fingerprint, and a
// deterministic 0–100 encryption-quality score.
//
// THREE FILES, ONE OBLIGATION
//
//   - entropy.go  ShannonEntropy(data []byte) float64
//   - tls.go      ParseTLSClientHello(pkt []byte) (*TLSClientHelloInfo, error)
//                 ClientHelloFingerprint(info) [16]byte
//   - score.go    ComputeScore(metrics ScoreMetrics) (score, confidence)
//
// PRIVACY (RISKS §F5, §F12, ADR-0006)
//
// The analysis layer never returns, logs, or stores raw packet
// bytes. Every public function works on either:
//
//   - derived numeric values (entropy float64, version enum, extension
//     type ID, cipher-suite ID, fingerprint hex), or
//   - SHA-256 hashes whose pre-image is the ordered concatenation of
//     cipher-suite values and extension bytes (the JA3/JA4-style
//     "what did this client advertise" fingerprint, not the payload).
//
// Callers that hand raw `[]byte` to the package *must not* persist,
// log, or transmit the slice after ParseTLSClientHello returns. The
// package itself never makes a copy beyond what gopacket needs to
// walk the record, and no helper prints or Logs *info or *info with
// raw bytes. (Verified by code-review checklist in PR-4 Notes.)
//
// DEPENDENCY
//
// Pulls from `github.com/gopacket/gopacket v1.7.0` (the upstream
// Go module; the opene2ee-com/gopacket fork is API-identical at
// master and is wired in by the `replace` directive in `go.mod`).
// The HANDOFF text that says `layers.TLSClientHello` references the
// older Google-naming for what is now
// `layers.TLSHandshakeRecordClientHello` — same field set, no
// behavioural difference.
//
// NON-GOALS (Sprint 1)
//
//   - No live packet capture / socket bind: the API takes a parsed
//     []byte so the system under test (mobile / integration) can
//     feed synthetic TLS Client Hello bytes through the same path.
//   - No ML/heuristic tuning surface: ComputeScore's weights are
//     fixed; per the HANDOFF "Sabit eşikler değil — config-dışı
//     başlangıç heuristic'i yeterli" the configuration knobs are
//     deferred until we have a labelled dataset.
//   - No ServerHello parsing: a future PR can mirror the parsing for
//     server-side fingerprinting; the Sprint 1 surface is Client Hello
//     only.
package analysis
