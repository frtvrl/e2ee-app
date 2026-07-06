// whois_test.go — unit tests for the port-43 whois client.
//
// We use net.Pipe (in-process TCP) to exercise the wire protocol
// without requiring a real whois server. The Dialer hook on
// WhoisConfig lets us redirect every connect to the in-process
// server.
package operator

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

// whoisPipeServer returns a Dialer that hands out connections
// already pre-connected to a fake whois server speaking the
// response in `body`.
type whoisPipeServer struct {
	mu     sync.Mutex
	body   string
	closed bool
}

func (w *whoisPipeServer) dialer(_ context.Context, network, _ string) (net.Conn, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, errors.New("test: server closed")
	}
	c1, c2 := net.Pipe()
	go func() {
		defer c1.Close()
		br := bufio.NewReader(c1)
		// Read the query line.
		q, err := br.ReadString('\n')
		if err != nil && err != io.EOF {
			return
		}
		_ = q // we don't introspect the query in tests
		_, _ = c1.Write([]byte(w.body))
	}()
	_ = network
	return c2, nil
}

func TestWhoisClient_Lookup_HappyPath(t *testing.T) {
	srv := &whoisPipeServer{
		body: `inetnum:        78.180.0.0 - 78.181.255.255
netname:        Turkcell-Net
descr:          Turkcell
country:        TR
admin-c:        TT1234-RIPE
% This is the RIPE Database query server.
`,
	}
	c, err := NewWhoisClient(WhoisConfig{
		Timeout:    1 * time.Second,
		ServerByCC: map[string]string{"": "whois.test.invalid"},
		Dialer:     srv.dialer,
	})
	if err != nil {
		t.Fatalf("NewWhoisClient: %v", err)
	}
	info, err := c.Lookup(context.Background(), netip.MustParseAddr("78.180.1.1"))
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if info.Operator != "Turkcell-Net" {
		t.Errorf("Operator = %q, want Turkcell-Net", info.Operator)
	}
	if info.Country != "TR" {
		t.Errorf("Country = %q, want TR", info.Country)
	}
	if info.Source != SourceRIPEWhois {
		t.Errorf("Source = %q, want %q", info.Source, SourceRIPEWhois)
	}
}

func TestWhoisClient_Lookup_ARINServerOutput(t *testing.T) {
	// Sprint 3 does NOT do IP→country geolocation — Lookup
	// always uses the empty-key server (RIPE in production).
	// This test verifies that ARIN-style output ("NetRange:",
	// "NetName:", "Country:" at top level) is parsed correctly
	// when it arrives. Sprint 4+ will add a GeoIP-driven server
	// picker that switches Source to SourceARINWhois.
	srv := &whoisPipeServer{
		body: `NetRange:       71.0.0.0 - 71.255.255.255
NetName:        VERIZON
Country:        US
RegDate:        2000-01-01
`,
	}
	c, _ := NewWhoisClient(WhoisConfig{
		Timeout:    1 * time.Second,
		ServerByCC: map[string]string{"": "whois.test.invalid"},
		Dialer:     srv.dialer,
	})
	info, err := c.Lookup(context.Background(), netip.MustParseAddr("71.5.5.5"))
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if info.Operator != "VERIZON" {
		t.Errorf("Operator = %q, want VERIZON (parsed from NetName)", info.Operator)
	}
	if info.Country != "US" {
		t.Errorf("Country = %q, want US", info.Country)
	}
}

func TestWhoisClient_Lookup_NoMatchIsUnknown(t *testing.T) {
	srv := &whoisPipeServer{
		body: `% No entries found in the selected source(s).
`,
	}
	c, _ := NewWhoisClient(WhoisConfig{
		Timeout:    1 * time.Second,
		ServerByCC: map[string]string{"": "whois.test.invalid"},
		Dialer:     srv.dialer,
	})
	_, err := c.Lookup(context.Background(), netip.MustParseAddr("1.2.3.4"))
	if !errors.Is(err, ErrUnknownOperator) {
		t.Errorf("err = %v, want ErrUnknownOperator", err)
	}
}

func TestWhoisClient_Lookup_InvalidIP(t *testing.T) {
	c, _ := NewWhoisClient(WhoisConfig{})
	_, err := c.Lookup(context.Background(), netip.Addr{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestWhoisClient_Lookup_DialErrorIsPropagated(t *testing.T) {
	c, _ := NewWhoisClient(WhoisConfig{
		Dialer: func(_ context.Context, _, _ string) (net.Conn, error) {
			return nil, errors.New("dial refused")
		},
	})
	_, err := c.Lookup(context.Background(), netip.MustParseAddr("1.2.3.4"))
	if err == nil {
		t.Fatal("expected dial error, got nil")
	}
}

func TestParseWhoisResponse_FieldVariants(t *testing.T) {
	body := `
netname:        A
descr:          descr-value
country:        tr
organisation:   ORG-B
origin:         AS12345
`
	p := parseWhoisResponse(body)
	if p.netName != "A" {
		t.Errorf("netName = %q, want A", p.netName)
	}
	if p.descr != "descr-value" {
		t.Errorf("descr = %q, want descr-value", p.descr)
	}
	if p.country != "TR" {
		t.Errorf("country = %q, want TR (upper-cased)", p.country)
	}
	if p.originAS != "AS12345" {
		t.Errorf("originAS = %q, want AS12345", p.originAS)
	}
}

func TestParseWhoisResponse_IgnoresCommentLines(t *testing.T) {
	body := `% This is a comment
# Another comment
netname: B
`
	p := parseWhoisResponse(body)
	if p.netName != "B" {
		t.Errorf("netName = %q, want B", p.netName)
	}
}

func TestParseWhoisResponse_SkipsEmptyValues(t *testing.T) {
	body := `netname:
descr: real
`
	p := parseWhoisResponse(body)
	if p.netName != "" {
		t.Errorf("netName = %q, want empty", p.netName)
	}
	if p.descr != "real" {
		t.Errorf("descr = %q, want real", p.descr)
	}
}

func TestWhoisClient_ServerByCC_FallsBackToDefault(t *testing.T) {
	c, _ := NewWhoisClient(WhoisConfig{
		ServerByCC: map[string]string{
			"":   "whois.ripe.net",
			"TR": "whois.ripe.net",
		},
	})
	// Unmapped country → empty key.
	if got := c.serverForCC("ZZ"); got != "whois.ripe.net" {
		t.Errorf("serverForCC(ZZ) = %q, want whois.ripe.net", got)
	}
	if got := c.serverForCC("TR"); got != "whois.ripe.net" {
		t.Errorf("serverForCC(TR) = %q, want whois.ripe.net", got)
	}
}

// Smoke test: IPReverseAdapter chain with a fake whois client.
func TestIPReverseAdapter_WhoisHit_MasksIP(t *testing.T) {
	srv := &whoisPipeServer{
		body: `netname: Verizon-Net
country: US
`,
	}
	whois, _ := NewWhoisClient(WhoisConfig{
		Timeout:    1 * time.Second,
		ServerByCC: map[string]string{"": "whois.test.invalid"},
		Dialer:     srv.dialer,
	})
	a := NewIPReverseAdapterWithDeps(nil, whois)
	// 203.0.113.5 is NOT in the local ASN table → falls through
	// to RDAP (nil → skip) → whois.
	got, err := a.LookupByIP(context.Background(), "203.0.113.5")
	if err != nil {
		t.Fatalf("LookupByIP: %v", err)
	}
	if got.Operator != "Verizon-Net" {
		t.Errorf("Operator = %q, want Verizon-Net", got.Operator)
	}
	if got.Source != SourceRIPEWhois {
		t.Errorf("Source = %q, want %q", got.Source, SourceRIPEWhois)
	}
	if !strings.HasSuffix(got.QueryValue, "/24") {
		t.Errorf("QueryValue = %q, want masked /24 form", got.QueryValue)
	}
}