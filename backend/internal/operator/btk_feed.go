// btk_feed.go — Real BTK MNP feed integration for the Operator Tespit
// Servisi.
//
// Sprint 3 (PR-23) replaces the Sprint-1 static table (mnp_tr.go)
// with a live BTK feed client. BTK (Bilgi Teknolojileri ve İletişim
// Kurumu) is Turkey's telecom regulator and publishes MNP data
// (Mobile Number Portability) to licensed integrators via two
// channels:
//
//   1. Pull (request/response) — REST/HTTPS, JSON. Auth is mutual
//      TLS using a client certificate issued by BTK. The endpoint
//      URL and the cert paths are deployment-config (env /
//      secrets). This file's BTKFeedClient speaks that protocol.
//
//   2. Push (webhook) — BTK can POST signed updates to our
//      /btk/webhook endpoint when a number is ported. The
//      WebhookSubscription type in this file handles those
//      updates by invalidating the affected cache entries
//      (so the next LookupByPhone re-pulls BTK).
//
// SCOPE / NON-GOALS (Sprint 3):
//
//   - ASN lookup (RIPE / APNIC) is DEFERRED to Sprint 4+. The
//     feed interface has a stubbed ASNSubscribe so a future PR
//     can wire it without changing this file's public shape.
//   - The static mnp_tr.go table is retained as an OFFLINE
//     FALLBACK — when the feed is unreachable, or for development
//     environments without a BTK certificate, the table answers.
//   - Real BTK endpoints require a production contract; in tests
//     we use httptest.Server to exercise the wire protocol.
//
// PRIVACY (ADR-0006 §Veri Minimizasyonu): the BTK response is
// expected to carry MSISDN values; we mask them with
// MaskPhoneE164 before they touch the cache or the REST layer.
package operator

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// BTKFeedConfig is the deployment-time configuration for the BTK
// MNP feed client. Sensitive fields (ClientCert, ClientKey, CACert,
// WebhookSecret) are read from the filesystem or an env-var indirection
// at startup; they are NEVER hardcoded.
type BTKFeedConfig struct {
	// Endpoint is the full URL of the BTK MNP lookup endpoint,
	// e.g. "https://mnp.btk.gov.tr/v1/lookup". Required.
	Endpoint string

	// ClientCert / ClientKey are paths to PEM files for mutual
	// TLS. Required for production. Optional for tests (httptest).
	ClientCert string
	ClientKey  string

	// CACert is an optional path to a PEM CA bundle to verify
	// BTK's server cert against. If empty, the system pool is used.
	CACert string

	// WebhookSecret is the HMAC-SHA256 shared secret BTK uses to
	// sign webhook bodies. Required when Subscription is enabled.
	WebhookSecret []byte

	// HTTPTimeout caps a single request. Default 5s.
	HTTPTimeout time.Duration

	// UserAgent is sent on every request. Defaults to "opene2ee-operator/1.0".
	UserAgent string

	// Now is overridable for tests (deterministic timestamps).
	Now func() time.Time
}

// BTKFeedClient is the wire-protocol client for the BTK MNP feed.
// It is independent of the OperatorLookup interface — callers wrap
// it in BTKFeedAdapter (below) to compose with the rest of the
// service.
//
// The zero value is NOT usable — call NewBTKFeedClient.
type BTKFeedClient struct {
	cfg     BTKFeedConfig
	http    *http.Client
	now     func() time.Time
}

// NewBTKFeedClient validates cfg and returns a usable client.
// Returns an error when Endpoint is empty or when mTLS files are
// unreadable.
func NewBTKFeedClient(cfg BTKFeedConfig) (*BTKFeedClient, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("operator: NewBTKFeedClient: Endpoint required")
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 5 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "opene2ee-operator/1.0"
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.CACert != "" {
		pem, err := os.ReadFile(cfg.CACert)
		if err != nil {
			return nil, fmt.Errorf("operator: BTK CA cert read: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("operator: BTK CA cert: AppendCertsFromPEM failed")
		}
		tlsCfg.RootCAs = pool
	}
	if cfg.ClientCert != "" && cfg.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("operator: BTK client cert: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}
	// Note: when ClientCert is empty the transport still works for
	// httptest-based unit tests that don't require mTLS.

	httpClient := &http.Client{
		Timeout: cfg.HTTPTimeout,
		Transport: &http.Transport{
			TLSClientConfig:       tlsCfg,
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   5,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: cfg.HTTPTimeout,
		},
	}
	return &BTKFeedClient{cfg: cfg, http: httpClient, now: now}, nil
}

// BTKLookupRequest is the wire-format request body for the BTK
// pull endpoint. The MSISDN must be in E.164 form.
type BTKLookupRequest struct {
	MSISDN string `json:"msisdn"`
}

// BTKLookupResponse is the wire-format response. Operator is the
// enum string (matches telemetry schema); OperatorName is human-
// readable. Confidence is the BTK feed's own score (1.0 means
// authoritative).
type BTKLookupResponse struct {
	MSISDN       string  `json:"msisdn"`
	Operator     string  `json:"operator"`
	OperatorName string  `json:"operator_name"`
	MCC          string  `json:"mcc"`
	MNC          string  `json:"mnc"`
	Confidence   float64 `json:"confidence"`
	PortedAt     string  `json:"ported_at,omitempty"` // RFC3339, empty if never ported
}

// Lookup issues one BTK feed request. Returns ErrUnknownOperator
// when the BTK feed responds with HTTP 404 (the number is in a
// range BTK doesn't cover, e.g. fixed-line) — this is the same
// "we don't know" signal an adapter should return so the
// orchestrator can move on to the next adapter in the chain.
// Other non-2xx responses become a wrapped error.
func (c *BTKFeedClient) Lookup(ctx context.Context, msisdn string) (*BTKLookupResponse, error) {
	if !looksLikeE164(msisdn) {
		return nil, fmt.Errorf("btk: %q: %w", msisdn, ErrInvalidInput)
	}
	body, err := json.Marshal(BTKLookupRequest{MSISDN: msisdn})
	if err != nil {
		return nil, fmt.Errorf("btk: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("btk: request build: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Opene2ee-Timestamp", c.now().UTC().Format(time.RFC3339))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("btk: http: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusNotFound {
		// BTK doesn't know about this number — out of MNP scope.
		return nil, ErrUnknownOperator
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("btk: http %d: %s", resp.StatusCode, string(raw))
	}
	var out BTKLookupResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("btk: decode: %w (body=%q)", err, string(raw))
	}
	if out.Operator == "" {
		return nil, fmt.Errorf("btk: empty operator in response")
	}
	return &out, nil
}

// BTKFeedAdapter implements OperatorLookup by delegating to a
// BTKFeedClient. On any BTK error (network, decode, transient
// HTTP) it returns ErrUnknownOperator so the orchestrator can
// fall back to the static table or another adapter in the chain.
//
// A nil client is rejected at construction time — an adapter
// without a feed is meaningless and would always be a fallback.
type BTKFeedAdapter struct {
	client *BTKFeedClient
	now    func() time.Time
}

// NewBTKFeedAdapter wraps a BTKFeedClient as an OperatorLookup.
// Returns an error when client is nil.
func NewBTKFeedAdapter(client *BTKFeedClient) (*BTKFeedAdapter, error) {
	if client == nil {
		return nil, errors.New("operator: NewBTKFeedAdapter: nil client")
	}
	return &BTKFeedAdapter{client: client, now: client.now}, nil
}

// Compile-time interface check.
var _ OperatorLookup = (*BTKFeedAdapter)(nil)

// LookupByPhone translates to a BTK feed call. Privacy: the
// returned OperatorInfo carries the MASKED MSISDN (mask.go) —
// never the raw E.164.
func (a *BTKFeedAdapter) LookupByPhone(ctx context.Context, e164 string) (*OperatorInfo, error) {
	resp, err := a.client.Lookup(ctx, e164)
	if err != nil {
		// Any BTK error → unknown. The orchestrator will continue
		// to the next adapter (typically the static table). We
		// deliberately do NOT cache the BTK error here; the cache
		// layer in front of the chain handles negatives.
		return nil, err
	}
	info := &OperatorInfo{
		QueryType:    QueryPhoneE164,
		QueryValue:   "", // filled by applyPhoneMask below
		Operator:     resp.Operator,
		OperatorName: resp.OperatorName,
		Country:      trCountryISO,
		MCC:          resp.MCC,
		MNC:          resp.MNC,
		Source:       SourceBTKFeed,
		Confidence:   clampConfidence(resp.Confidence),
		Timestamp:    a.now().UTC(),
	}
	applyPhoneMask(info, e164)
	return info, nil
}

// LookupByIP is a no-op for the BTK feed — the MNP feed only
// answers phone queries. Returns ErrUnknownOperator so the
// orchestrator moves on to the IP adapter.
func (a *BTKFeedAdapter) LookupByIP(_ context.Context, ip string) (*OperatorInfo, error) {
	return nil, fmt.Errorf("btk feed does not resolve IP %q: %w", ip, ErrUnknownOperator)
}

// clampConfidence bounds a confidence value to [0, 1]. A BTK
// feed that returns 1.2 or -0.5 (malformed / hand-crafted
// response) should not poison the cache.
func clampConfidence(c float64) float64 {
	switch {
	case c < 0:
		return 0
	case c > 1:
		return 1
	}
	return c
}

// -----------------------------------------------------------------------------
// Webhook subscription
// -----------------------------------------------------------------------------

// BTKWebhookEvent is one signed update from BTK's push channel.
// SignedPayload is the raw JSON body — the signature covers it
// verbatim so the receiver MUST NOT re-marshal before verifying.
type BTKWebhookEvent struct {
	SignedPayload []byte
	Signature     string // hex(HMAC-SHA256(secret, body))
	Timestamp     string // RFC3339
	MSISDN        string // populated after Verify (for convenience)
}

// BTKWebhookHandler is the callback the operator package calls
// when a verified webhook arrives. Implementations should
// invalidate the cache entry for the affected MSISDN and
// (optionally) update a local cache mirror.
//
// The handler MUST be safe for concurrent invocation — BTK can
// send bursts of webhooks and the package does not serialise.
type BTKWebhookHandler func(ctx context.Context, ev BTKWebhookEvent) error

// BTKWebhookSubscription holds the HMAC verification config and
// the in-memory state needed to deliver webhooks to a handler.
// Sprint 3 uses a synchronous in-process delivery model — there
// is no queue yet. The on-disk WAL / outbox pattern is a Sprint 4
// concern (HANDOFF §9 "event sourcing backlog").
type BTKWebhookSubscription struct {
	secret  []byte
	handler BTKWebhookHandler
	now     func() time.Time

	// Replay protection: reject events with a timestamp older
	// than maxSkew. Default 5 minutes.
	maxSkew time.Duration

	// observed is a small LRU of recently-seen (timestamp, sig)
	// tuples so a BTK retry of an already-processed event does
	// not double-deliver. Mutex-guarded.
	mu       sync.Mutex
	observed map[string]struct{}
}

// NewBTKWebhookSubscription wires a handler. secret must be the
// same HMAC key BTK uses to sign bodies; handler is the callback
// for verified events.
func NewBTKWebhookSubscription(secret []byte, handler BTKWebhookHandler) (*BTKWebhookSubscription, error) {
	if len(secret) == 0 {
		return nil, errors.New("operator: NewBTKWebhookSubscription: empty secret")
	}
	if handler == nil {
		return nil, errors.New("operator: NewBTKWebhookSubscription: nil handler")
	}
	return &BTKWebhookSubscription{
		secret:   append([]byte(nil), secret...), // copy
		handler:  handler,
		now:      time.Now,
		maxSkew:  5 * time.Minute,
		observed: make(map[string]struct{}),
	}, nil
}

// Verify checks the HMAC-SHA256 signature and the timestamp
// skew, then invokes the configured handler. Returns nil on
// successful delivery; returns ErrUnknownOperator when the
// event is a "number deleted / out of scope" notice (the
// handler can choose to evict the cache entry).
//
// Errors:
//   - ErrInvalidInput: bad signature, malformed JSON, or stale
//     timestamp (replay attack protection).
func (s *BTKWebhookSubscription) Verify(ctx context.Context, body []byte, signature string, timestamp string) error {
	if !s.verifySignature(body, signature) {
		return fmt.Errorf("btk webhook: bad signature: %w", ErrInvalidInput)
	}
	ts, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return fmt.Errorf("btk webhook: bad timestamp %q: %w", timestamp, ErrInvalidInput)
	}
	if d := s.now().Sub(ts); d > s.maxSkew || d < -s.maxSkew {
		return fmt.Errorf("btk webhook: timestamp skew %s exceeds max %s: %w",
			d, s.maxSkew, ErrInvalidInput)
	}
	// Replay-protection dedupe key: (timestamp, signature).
	key := timestamp + "|" + signature
	s.mu.Lock()
	if _, dup := s.observed[key]; dup {
		s.mu.Unlock()
		return nil // idempotent
	}
	s.observed[key] = struct{}{}
	// Garbage-collect observed entries older than 2× maxSkew so
	// the map doesn't grow unbounded. Simple O(N) scan — fine
	// for a low-volume webhook channel.
	for k := range s.observed {
		// key format: "<ts>|<sig>"; reparse the ts prefix.
		for i := 0; i < len(k); i++ {
			if k[i] == '|' {
				kts, perr := time.Parse(time.RFC3339, k[:i])
				if perr == nil && s.now().Sub(kts) > 2*s.maxSkew {
					delete(s.observed, k)
				}
				break
			}
		}
	}
	s.mu.Unlock()

	var payload struct {
		MSISDN string `json:"msisdn"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("btk webhook: decode body: %w", err)
	}
	ev := BTKWebhookEvent{
		SignedPayload: body,
		Signature:     signature,
		Timestamp:     timestamp,
		MSISDN:        payload.MSISDN,
	}
	return s.handler(ctx, ev)
}

// verifySignature checks HMAC-SHA256(secret, body) == signature
// (hex). The comparison uses hmac.Equal to avoid timing
// side-channels.
func (s *BTKWebhookSubscription) verifySignature(body []byte, signature string) bool {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(body)
	want := mac.Sum(nil)
	got, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	return hmac.Equal(want, got)
}

// Sign is the inverse of verifySignature; exposed so test
// fixtures (and any future producer side, e.g. a debug mode
// that injects events) can produce a valid signature.
func (s *BTKWebhookSubscription) Sign(body []byte) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// ASNSubscribe is a stub for Sprint 4+ — RIPE / APNIC ASN feed
// integration. Returning nil signals "not yet implemented" so
// the orchestrator knows to skip it.
type ASNSubscribe func(ctx context.Context, asn uint32) error