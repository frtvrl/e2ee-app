// btk_feed_test.go — unit tests for the BTK MNP feed client.
//
// We exercise the wire protocol against an httptest.Server that
// speaks the BTK pull endpoint contract; the production code path
// is the same — only the dialer differs.
//
// Webhook subscription tests verify the HMAC-SHA256 signature
// scheme, the timestamp skew window, and the replay-protection
// dedupe.
package operator

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// insecureTLS returns a *tls.Config that accepts any server
// certificate. Only for use in unit tests against httptest
// self-signed servers.
func insecureTLS() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true}
}

func newTestBTKServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *BTKFeedConfig) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	// httptest TLS gives us a self-signed server cert; configure
	// the client to trust it via InsecureSkipVerify only for the
	// duration of this test.
	cfg := BTKFeedConfig{
		Endpoint:    srv.URL + "/v1/lookup",
		HTTPTimeout: 2 * time.Second,
		UserAgent:   "opene2ee-operator/test",
	}
	return srv, &cfg
}

func TestBTKFeedClient_Lookup_HappyPath(t *testing.T) {
	srv, cfg := newTestBTKServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		var req BTKLookupRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decode req: %v", err)
		}
		if req.MSISDN != "+905320000000" {
			t.Errorf("req.MSISDN = %q, want +905320000000", req.MSISDN)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(BTKLookupResponse{
			MSISDN:       "+905320000000",
			Operator:     "vodafone_tr",
			OperatorName: "Vodafone TR",
			MCC:          "286",
			MNC:          "02",
			Confidence:   0.99,
		})
	})
	cfg.Endpoint = srv.URL + "/v1/lookup"

	// Trust the httptest cert by overriding the transport with
	// the test server's client. Easier: use the cfg HTTPClient
	// field. We don't have one — instead, build the client with
	// an http.Client that skips verification for this test.
	c, err := newTestBTKClientInsecure(*cfg)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	resp, err := c.Lookup(context.Background(), "+905320000000")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if resp.Operator != "vodafone_tr" {
		t.Errorf("Operator = %q, want vodafone_tr", resp.Operator)
	}
	if resp.Confidence != 0.99 {
		t.Errorf("Confidence = %v, want 0.99", resp.Confidence)
	}
}

// newTestBTKClientInsecure constructs a BTKFeedClient that trusts
// the httptest TLS server's self-signed cert. InsecureSkipVerify
// is acceptable only in unit tests.
func newTestBTKClientInsecure(cfg BTKFeedConfig) (*BTKFeedClient, error) {
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 2 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "opene2ee-operator/test"
	}
	tr := &http.Transport{
		TLSClientConfig: insecureTLS(),
	}
	httpClient := &http.Client{
		Timeout:   cfg.HTTPTimeout,
		Transport: tr,
	}
	c, err := NewBTKFeedClient(cfg)
	if err != nil {
		return nil, err
	}
	c.http = httpClient
	return c, nil
}

func TestBTKFeedClient_Lookup_NotFoundIsUnknown(t *testing.T) {
	srv, cfg := newTestBTKServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	cfg.Endpoint = srv.URL + "/v1/lookup"
	c, _ := newTestBTKClientInsecure(*cfg)
	_, err := c.Lookup(context.Background(), "+905320000000")
	if !errors.Is(err, ErrUnknownOperator) {
		t.Errorf("err = %v, want ErrUnknownOperator", err)
	}
}

func TestBTKFeedClient_Lookup_500IsError(t *testing.T) {
	srv, cfg := newTestBTKServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("btk backend down"))
	})
	cfg.Endpoint = srv.URL + "/v1/lookup"
	c, _ := newTestBTKClientInsecure(*cfg)
	_, err := c.Lookup(context.Background(), "+905320000000")
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
	if errors.Is(err, ErrUnknownOperator) {
		t.Errorf("500 must NOT be classified as ErrUnknownOperator, got %v", err)
	}
}

func TestBTKFeedClient_Lookup_InvalidInput(t *testing.T) {
	c, _ := newTestBTKClientInsecure(BTKFeedConfig{Endpoint: "https://example.invalid"})
	_, err := c.Lookup(context.Background(), "not a phone")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestNewBTKFeedClient_RequiresEndpoint(t *testing.T) {
	if _, err := NewBTKFeedClient(BTKFeedConfig{}); err == nil {
		t.Error("empty Endpoint accepted")
	}
}

func TestNewBTKFeedAdapter_RequiresClient(t *testing.T) {
	if _, err := NewBTKFeedAdapter(nil); err == nil {
		t.Error("nil client accepted")
	}
}

func TestBTKFeedAdapter_LookupByPhone_MasksMSISDN(t *testing.T) {
	srv, cfg := newTestBTKServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(BTKLookupResponse{
			MSISDN: "+905320000000", Operator: "turkcell", OperatorName: "Turkcell",
			MCC: "286", MNC: "01", Confidence: 1,
		})
	})
	cfg.Endpoint = srv.URL + "/v1/lookup"
	c, _ := newTestBTKClientInsecure(*cfg)
	a, err := NewBTKFeedAdapter(c)
	if err != nil {
		t.Fatalf("NewBTKFeedAdapter: %v", err)
	}
	info, err := a.LookupByPhone(context.Background(), "+905320000000")
	if err != nil {
		t.Fatalf("LookupByPhone: %v", err)
	}
	// Privacy invariant: the masked MSISDN (not the raw one)
	// must be in QueryValue. ADR-0006.
	if info.QueryValue != MaskPhoneE164("+905320000000") {
		t.Errorf("QueryValue = %q, want masked form %q",
			info.QueryValue, MaskPhoneE164("+905320000000"))
	}
	if strings.Contains(info.QueryValue, "905320000000") {
		t.Errorf("QueryValue still contains raw subscriber digits: %q", info.QueryValue)
	}
	if info.Source != SourceBTKFeed {
		t.Errorf("Source = %q, want %q", info.Source, SourceBTKFeed)
	}
}

func TestBTKFeedAdapter_LookupByIP_ReturnsUnknown(t *testing.T) {
	c, _ := newTestBTKClientInsecure(BTKFeedConfig{Endpoint: "https://example.invalid"})
	a, _ := NewBTKFeedAdapter(c)
	_, err := a.LookupByIP(context.Background(), "8.8.8.8")
	if !errors.Is(err, ErrUnknownOperator) {
		t.Errorf("err = %v, want ErrUnknownOperator", err)
	}
}

func TestClampConfidence(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{-0.5, 0},
		{0, 0},
		{0.5, 0.5},
		{1, 1},
		{1.5, 1},
	}
	for _, c := range cases {
		got := clampConfidence(c.in)
		if got != c.want {
			t.Errorf("clampConfidence(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Webhook subscription
// ---------------------------------------------------------------------------

func TestBTKWebhook_Verify_HappyPath(t *testing.T) {
	var called int32
	handler := func(_ context.Context, ev BTKWebhookEvent) error {
		atomic.AddInt32(&called, 1)
		if ev.MSISDN != "+905320000000" {
			t.Errorf("ev.MSISDN = %q, want +905320000000", ev.MSISDN)
		}
		return nil
	}
	sub, err := NewBTKWebhookSubscription([]byte("s3cr3t"), handler)
	if err != nil {
		t.Fatalf("NewBTKWebhookSubscription: %v", err)
	}
	body, _ := json.Marshal(map[string]string{"msisdn": "+905320000000", "ported_to": "vodafone_tr"})
	sig := sub.Sign(body)
	ts := time.Now().UTC().Format(time.RFC3339)
	if err := sub.Verify(context.Background(), body, sig, ts); err != nil {
		t.Errorf("Verify: %v", err)
	}
	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("handler called %d times, want 1", called)
	}
	// Replay of the same event MUST be deduped.
	if err := sub.Verify(context.Background(), body, sig, ts); err != nil {
		t.Errorf("Verify replay: %v", err)
	}
	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("handler called %d times after replay, want 1 (dedupe failed)", called)
	}
}

func TestBTKWebhook_Verify_BadSignature(t *testing.T) {
	sub, _ := NewBTKWebhookSubscription([]byte("s3cr3t"), func(_ context.Context, _ BTKWebhookEvent) error { return nil })
	body := []byte(`{"msisdn":"+905320000000"}`)
	if err := sub.Verify(context.Background(), body, "deadbeef", time.Now().UTC().Format(time.RFC3339)); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestBTKWebhook_Verify_StaleTimestamp(t *testing.T) {
	sub, _ := NewBTKWebhookSubscription([]byte("s3cr3t"), func(_ context.Context, _ BTKWebhookEvent) error { return nil })
	body := []byte(`{}`)
	sig := sub.Sign(body)
	// 10 minutes ago — well outside the 5-minute skew window.
	stale := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	if err := sub.Verify(context.Background(), body, sig, stale); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput (stale)", err)
	}
}

func TestBTKWebhook_Verify_BadTimestamp(t *testing.T) {
	sub, _ := NewBTKWebhookSubscription([]byte("s3cr3t"), func(_ context.Context, _ BTKWebhookEvent) error { return nil })
	body := []byte(`{}`)
	sig := sub.Sign(body)
	if err := sub.Verify(context.Background(), body, sig, "not-a-timestamp"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput (bad ts)", err)
	}
}

func TestBTKWebhook_RequiresSecretAndHandler(t *testing.T) {
	if _, err := NewBTKWebhookSubscription(nil, func(_ context.Context, _ BTKWebhookEvent) error { return nil }); err == nil {
		t.Error("nil secret accepted")
	}
	if _, err := NewBTKWebhookSubscription([]byte("x"), nil); err == nil {
		t.Error("nil handler accepted")
	}
}