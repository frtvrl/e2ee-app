// rdap_test.go — unit tests for the RDAP client.
//
// We redirect the bootstrap URL to an httptest.Server and serve a
// canned RDAP JSON body. The wire protocol is the same in
// production; only the dialer differs.
package operator

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// rdapTestServer wires a single httptest server that responds to
// "/ip/<ip>" with the JSON the test specifies.
type rdapTestServer struct {
	*httptest.Server
	hits int32
}

func newRDAPServer(t *testing.T, body string, status int) *rdapTestServer {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		// Both the bootstrap URL and the IP query share the same
		// test server; we just return the canned body for any
		// path. Production splits these across rdap.org → RIR.
		w.Header().Set("Content-Type", "application/rdap+json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return &rdapTestServer{Server: srv, hits: hits}
}

func TestRDAPClient_Lookup_HappyPath(t *testing.T) {
	body := `{
		"handle": "RIPE-NET-1",
		"startAddress": "78.180.0.0",
		"endAddress": "78.181.255.255",
		"country": "TR",
		"name": "Turkcell-Net",
		"type": "DIRECT ALLOCATION"
	}`
	srv := newRDAPServer(t, body, http.StatusOK)
	c, err := NewRDAPClient(RDAPConfig{
		BootstrapURL: srv.URL + "/",
		HTTPTimeout:  2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRDAPClient: %v", err)
	}
	ip := netip.MustParseAddr("78.180.1.1")
	info, err := c.Lookup(context.Background(), ip)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if info.Operator != "RIPE-NET-1" {
		t.Errorf("Operator = %q, want RIPE-NET-1", info.Operator)
	}
	if info.Country != "TR" {
		t.Errorf("Country = %q, want TR", info.Country)
	}
	if info.Source != SourceRDAP {
		t.Errorf("Source = %q, want %q", info.Source, SourceRDAP)
	}
	if info.Confidence != 0.95 {
		t.Errorf("Confidence = %v, want 0.95", info.Confidence)
	}
}

func TestRDAPClient_Lookup_NotFoundIsUnknown(t *testing.T) {
	srv := newRDAPServer(t, "not found", http.StatusNotFound)
	c, _ := NewRDAPClient(RDAPConfig{BootstrapURL: srv.URL + "/", HTTPTimeout: 2 * time.Second})
	_, err := c.Lookup(context.Background(), netip.MustParseAddr("1.2.3.4"))
	if !errors.Is(err, ErrUnknownOperator) {
		t.Errorf("err = %v, want ErrUnknownOperator", err)
	}
}

func TestRDAPClient_Lookup_EmptyAnswerIsUnknown(t *testing.T) {
	// 200 OK with an empty body must still be classified as
	// unknown — the registry said "no record" without a 404.
	srv := newRDAPServer(t, `{}`, http.StatusOK)
	c, _ := NewRDAPClient(RDAPConfig{BootstrapURL: srv.URL + "/", HTTPTimeout: 2 * time.Second})
	_, err := c.Lookup(context.Background(), netip.MustParseAddr("1.2.3.4"))
	if !errors.Is(err, ErrUnknownOperator) {
		t.Errorf("err = %v, want ErrUnknownOperator (empty body)", err)
	}
}

func TestRDAPClient_Lookup_500IsError(t *testing.T) {
	srv := newRDAPServer(t, "boom", http.StatusInternalServerError)
	c, _ := NewRDAPClient(RDAPConfig{BootstrapURL: srv.URL + "/", HTTPTimeout: 2 * time.Second})
	_, err := c.Lookup(context.Background(), netip.MustParseAddr("1.2.3.4"))
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if errors.Is(err, ErrUnknownOperator) {
		t.Errorf("500 must NOT be ErrUnknownOperator, got %v", err)
	}
}

func TestRDAPClient_Lookup_InvalidIP(t *testing.T) {
	c, _ := NewRDAPClient(RDAPConfig{ BootstrapURL: "https://example.invalid" })
	_, err := c.Lookup(context.Background(), netip.Addr{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestRDAPClient_Lookup_RespectsContextCancellation(t *testing.T) {
	// A server that blocks indefinitely. The client must honour
	// ctx.Done() and abort before the package's default 5s.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	c, _ := NewRDAPClient(RDAPConfig{
		BootstrapURL: srv.URL + "/",
		HTTPTimeout:  500 * time.Millisecond,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Lookup(ctx, netip.MustParseAddr("1.2.3.4"))
	if err == nil {
		t.Fatal("expected timeout / context error")
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("Lookup took %s, expected to abort on ctx deadline", time.Since(start))
	}
}

func TestNewRDAPClient_DefaultBootstrap(t *testing.T) {
	// Empty BootstrapURL → default https://rdap.org/.
	c, err := NewRDAPClient(RDAPConfig{})
	if err != nil {
		t.Fatalf("NewRDAPClient: %v", err)
	}
	if c.cfg.BootstrapURL != defaultRDAPBootstrap {
		t.Errorf("BootstrapURL = %q, want %q", c.cfg.BootstrapURL, defaultRDAPBootstrap)
	}
}

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"", "", "x"}, "x"},
		{[]string{"a", "b", "c"}, "a"},
		{[]string{"", ""}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		got := firstNonEmpty(c.in...)
		if got != c.want {
			t.Errorf("firstNonEmpty(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Smoke test: the full IPReverseAdapter chain with a fake RDAP
// server returns the SourceRDAP answer and masks the IP.
func TestIPReverseAdapter_RDAPHit_MasksIP(t *testing.T) {
	body := `{"handle": "RIPE-NET-1", "country": "TR", "name": "Turkcell-Net"}`
	srv := newRDAPServer(t, body, http.StatusOK)
	rdap, err := NewRDAPClient(RDAPConfig{
		BootstrapURL: srv.URL + "/",
		HTTPTimeout:  2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRDAPClient: %v", err)
	}
	a := NewIPReverseAdapterWithDeps(rdap, nil)
	got, err := a.LookupByIP(context.Background(), "78.180.1.1")
	// 78.180.1.1 is in the local ASN table → that path is hit
	// first and we never reach RDAP. Use an IP NOT in the local
	// table.
	got, err = a.LookupByIP(context.Background(), "203.0.113.5")
	if err != nil {
		t.Fatalf("LookupByIP: %v", err)
	}
	if got.Source != SourceRDAP {
		t.Errorf("Source = %q, want %q", got.Source, SourceRDAP)
	}
	if got.QueryValue != MaskIP("203.0.113.5") {
		t.Errorf("QueryValue = %q, want masked %q",
			got.QueryValue, MaskIP("203.0.113.5"))
	}
	if strings.Contains(got.QueryValue, "203.0.113.5") {
		t.Errorf("QueryValue contains raw IP: %q", got.QueryValue)
	}
}