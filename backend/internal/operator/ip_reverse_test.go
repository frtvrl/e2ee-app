// ip_reverse_test.go — unit tests for IPReverseAdapter.
package operator

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"
)

func TestIPReverseAdapter_LookupByIP_KnownTR(t *testing.T) {
	// Each of these IPs must resolve to a known TR operator.
	cases := []struct {
		ip      string
		wantOp  string
		wantCC  string
		wantMCC string
		wantMNC string
	}{
		{"78.180.1.1", "turkcell", "TR", "286", "01"},
		{"5.46.0.5", "turkcell", "TR", "286", "01"},
		{"31.140.0.10", "vodafone_tr", "TR", "286", "02"},
		{"213.74.5.5", "vodafone_tr", "TR", "286", "02"},
		{"88.224.1.1", "turk_telekom", "TR", "286", "03"},
		{"85.96.7.7", "turk_telekom", "TR", "286", "03"},
	}
	a := NewIPReverseAdapter()
	for _, c := range cases {
		got, err := a.LookupByIP(context.Background(), c.ip)
		if err != nil {
			t.Errorf("LookupByIP(%q) error: %v", c.ip, err)
			continue
		}
		if got.Operator != c.wantOp {
			t.Errorf("LookupByIP(%q).Operator = %q, want %q",
				c.ip, got.Operator, c.wantOp)
		}
		if got.Country != c.wantCC {
			t.Errorf("LookupByIP(%q).Country = %q, want %q",
				c.ip, got.Country, c.wantCC)
		}
		if got.MCC != c.wantMCC {
			t.Errorf("LookupByIP(%q).MCC = %q, want %q",
				c.ip, got.MCC, c.wantMCC)
		}
		if got.MNC != c.wantMNC {
			t.Errorf("LookupByIP(%q).MNC = %q, want %q",
				c.ip, got.MNC, c.wantMNC)
		}
		if got.QueryValue != MaskIP(c.ip) {
			t.Errorf("LookupByIP(%q).QueryValue = %q, want %q (masked /24 form — ADR-0006 §Veri Minimizasyonu)",
				c.ip, got.QueryValue, MaskIP(c.ip))
		}
		if got.QueryType != QueryIPv4 {
			t.Errorf("LookupByIP(%q).QueryType = %q, want %q",
				c.ip, got.QueryType, QueryIPv4)
		}
	}
}

func TestIPReverseAdapter_LookupByIP_KnownUS(t *testing.T) {
	cases := []struct {
		ip     string
		wantOp string
	}{
		{"12.0.0.1", "att"},
		{"12.255.255.255", "att"},
		{"71.5.5.5", "verizon"},
		{"172.32.0.1", "tmobile_us"},
	}
	a := NewIPReverseAdapter()
	for _, c := range cases {
		got, err := a.LookupByIP(context.Background(), c.ip)
		if err != nil {
			t.Errorf("LookupByIP(%q) error: %v", c.ip, err)
			continue
		}
		if got.Operator != c.wantOp {
			t.Errorf("LookupByIP(%q).Operator = %q, want %q",
				c.ip, got.Operator, c.wantOp)
		}
	}
}

func TestIPReverseAdapter_LookupByIP_Unknown(t *testing.T) {
	a := NewIPReverseAdapter()
	// An IP that's syntactically valid but in no range. 1.2.3.4 is
	// allocated to APNIC but not in our local table; we expect
	// ErrUnknownOperator so the orchestrator can return a "fallback"
	// OperatorInfo.
	_, err := a.LookupByIP(context.Background(), "1.2.3.4")
	if !errors.Is(err, ErrUnknownOperator) {
		t.Errorf("LookupByIP(1.2.3.4) err = %v, want ErrUnknownOperator", err)
	}
}

func TestIPReverseAdapter_LookupByIP_Invalid(t *testing.T) {
	a := NewIPReverseAdapter()
	bad := []string{
		"",
		"not an ip",
		"999.999.999.999",
		"1.2.3",
	}
	for _, s := range bad {
		_, err := a.LookupByIP(context.Background(), s)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("LookupByIP(%q) err = %v, want ErrInvalidInput", s, err)
		}
	}
}

func TestIPReverseAdapter_LookupByIP_IPv6AcceptedButUnknown(t *testing.T) {
	a := NewIPReverseAdapter()
	// IPv6 is accepted at the API surface but the local table is
	// v4-only. The orchestrator turns the resulting error into a
	// "fallback" info, so the test asserts the error is
	// ErrUnknownOperator (not ErrInvalidInput).
	_, err := a.LookupByIP(context.Background(), "2001:db8::1")
	if !errors.Is(err, ErrUnknownOperator) {
		t.Errorf("LookupByIP(2001:db8::1) err = %v, want ErrUnknownOperator", err)
	}
}

func TestIPReverseAdapter_LookupByPhone_ReturnsUnknown(t *testing.T) {
	a := NewIPReverseAdapter()
	_, err := a.LookupByPhone(context.Background(), "+905320000000")
	if !errors.Is(err, ErrUnknownOperator) {
		t.Errorf("IP.LookupByPhone err = %v, want ErrUnknownOperator", err)
	}
}

func TestIPReverseAdapter_WhoisIsCalledOnMiss(t *testing.T) {
	called := false
	a := NewIPReverseAdapterWithWhois(func(_ context.Context, ip netip.Addr) (*OperatorInfo, error) {
		called = true
		if !ip.Is4() {
			t.Errorf("whois got non-v4: %v", ip)
		}
		return &OperatorInfo{
			QueryType:   QueryIPv4,
			Operator:    "verizon",
			Country:     "US",
			Source:      SourceARINWhois,
			Confidence:  0.9,
			Timestamp:   time.Now().UTC(),
		}, nil
	})
	got, err := a.LookupByIP(context.Background(), "1.2.3.4")
	if err != nil {
		t.Fatalf("LookupByIP error: %v", err)
	}
	if !called {
		t.Fatal("whoisLookup was not invoked on table miss")
	}
	if got.Operator != "verizon" {
		t.Errorf("Operator = %q, want verizon", got.Operator)
	}
	if got.Source != SourceARINWhois {
		t.Errorf("Source = %q, want %q", got.Source, SourceARINWhois)
	}
	if got.QueryValue != MaskIP("1.2.3.4") {
		t.Errorf("QueryValue = %q, want %q (masked /24 form — ADR-0006)", got.QueryValue, MaskIP("1.2.3.4"))
	}
}

func TestIPReverseAdapter_WhoisReturningUnknownFallsThrough(t *testing.T) {
	// If the whois closure says "unknown" too, the adapter must
	// surface ErrUnknownOperator (NOT silently invent a result).
	a := NewIPReverseAdapterWithWhois(func(_ context.Context, _ netip.Addr) (*OperatorInfo, error) {
		return nil, ErrUnknownOperator
	})
	_, err := a.LookupByIP(context.Background(), "1.2.3.4")
	if !errors.Is(err, ErrUnknownOperator) {
		t.Errorf("err = %v, want ErrUnknownOperator", err)
	}
}

func TestIPReverseAdapter_WhoisPropagatesOtherErrors(t *testing.T) {
	// A non-ErrUnknownOperator error from the whois closure must be
	// propagated, not swallowed.
	boom := errors.New("whois backend offline")
	a := NewIPReverseAdapterWithWhois(func(_ context.Context, _ netip.Addr) (*OperatorInfo, error) {
		return nil, boom
	})
	_, err := a.LookupByIP(context.Background(), "1.2.3.4")
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}

func TestIPReverseAdapter_WhoisNotCalledOnHit(t *testing.T) {
	// Local-table hit must NOT fall through to whois.
	called := false
	a := NewIPReverseAdapterWithWhois(func(_ context.Context, _ netip.Addr) (*OperatorInfo, error) {
		called = true
		return nil, errors.New("must not be called")
	})
	// 78.180.1.1 hits the Turkcell entry directly.
	_, err := a.LookupByIP(context.Background(), "78.180.1.1")
	if err != nil {
		t.Fatalf("LookupByIP error: %v", err)
	}
	if called {
		t.Error("whois was called even though local table had a hit")
	}
}

func TestIPReverseAdapter_SpecificityOrdering(t *testing.T) {
	// /15 (78.180.0.0/15) must beat any broader range that also
	// contains 78.180.1.1. We don't add an overlapping broader row
	// here, but we DO sanity-check that asnTableSorted is in
	// descending-bits order (which is what makes "most specific
	// wins" work).
	for i := 1; i < len(asnTableSorted); i++ {
		prev := asnTableSorted[i-1].CIDR.Bits()
		cur := asnTableSorted[i].CIDR.Bits()
		if prev < cur {
			t.Errorf("asnTableSorted not in descending-bits order: row %d bits=%d < row %d bits=%d",
				i-1, prev, i, cur)
		}
	}
}

func TestASNTableSize_IsAboveFloor(t *testing.T) {
	if ASNTableSize() < 5 {
		t.Errorf("ASN table shrank: %d rows (want >= 5)", ASNTableSize())
	}
}

func TestMustPrefix_PanicsOnBadInput(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("mustPrefix should panic on invalid input")
		}
	}()
	_ = mustPrefix("not a cidr")
}
