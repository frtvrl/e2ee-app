// mnp_tr_test.go — unit tests for MNPTRAdapter.
package operator

import (
	"context"
	"errors"
	"testing"
)

func TestMNPTRAdapter_LookupByPhone_KnownPrefixes(t *testing.T) {
	// Spot-check several prefixes that MUST exist (the table is
	// small, but these are the well-known carriers).
	cases := []struct {
		phone       string
		wantOp      string
		wantOpName  string
		wantMNC     string
	}{
		{"+905320000000", "turkcell", "Turkcell", "01"},
		{"+905390000000", "turkcell", "Turkcell", "01"},
		{"+905400000000", "vodafone_tr", "Vodafone TR", "02"},
		{"+905490000000", "vodafone_tr", "Vodafone TR", "02"},
		{"+905050000000", "turk_telekom", "Turk Telekom", "03"},
		{"+905310000000", "turk_telekom", "Turk Telekom", "03"},
		{"+905510000000", "turk_telekom", "Turk Telekom", "03"},
	}
	a := NewMNPTRAdapter()
	for _, c := range cases {
		got, err := a.LookupByPhone(context.Background(), c.phone)
		if err != nil {
			t.Errorf("LookupByPhone(%q) error: %v", c.phone, err)
			continue
		}
		if got.Operator != c.wantOp {
			t.Errorf("LookupByPhone(%q).Operator = %q, want %q",
				c.phone, got.Operator, c.wantOp)
		}
		if got.OperatorName != c.wantOpName {
			t.Errorf("LookupByPhone(%q).OperatorName = %q, want %q",
				c.phone, got.OperatorName, c.wantOpName)
		}
		if got.MNC != c.wantMNC {
			t.Errorf("LookupByPhone(%q).MNC = %q, want %q",
				c.phone, got.MNC, c.wantMNC)
		}
		if got.Country != "TR" {
			t.Errorf("LookupByPhone(%q).Country = %q, want TR", c.phone, got.Country)
		}
		if got.MCC != "286" {
			t.Errorf("LookupByPhone(%q).MCC = %q, want 286", c.phone, got.MCC)
		}
		if got.Source != SourceTRMNPAPI {
			t.Errorf("LookupByPhone(%q).Source = %q, want %q",
				c.phone, got.Source, SourceTRMNPAPI)
		}
		if got.Confidence <= 0 || got.Confidence > 1 {
			t.Errorf("LookupByPhone(%q).Confidence = %v, want in (0,1]",
				c.phone, got.Confidence)
		}
		if got.QueryType != QueryPhoneE164 {
			t.Errorf("LookupByPhone(%q).QueryType = %q, want %q",
				c.phone, got.QueryType, QueryPhoneE164)
		}
		if got.QueryValue != c.phone {
			t.Errorf("LookupByPhone(%q).QueryValue = %q, want %q",
				c.phone, got.QueryValue, c.phone)
		}
		if got.Timestamp.IsZero() {
			t.Errorf("LookupByPhone(%q).Timestamp is zero", c.phone)
		}
	}
}

func TestMNPTRAdapter_LookupByPhone_InvalidInput(t *testing.T) {
	a := NewMNPTRAdapter()
	bad := []string{
		"",
		"905320000000",      // missing +
		"+",                 // too short
		"+1",                // too short
		"+90 532 000 0000",  // spaces
		"+abc",              // not a number
		"+0123456789012345", // leading 0 country code
	}
	for _, s := range bad {
		_, err := a.LookupByPhone(context.Background(), s)
		if err == nil {
			t.Errorf("LookupByPhone(%q) returned nil error, want ErrInvalidInput", s)
			continue
		}
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("LookupByPhone(%q) err = %v, want errors.Is(ErrInvalidInput)", s, err)
		}
	}
}

func TestMNPTRAdapter_LookupByPhone_NotTR(t *testing.T) {
	a := NewMNPTRAdapter()
	// Well-formed E.164, but not +90 → adapter must report unknown
	// (this is the contract: MNP is TR-only).
	notTR := []string{
		"+12025550143",   // US
		"+442071838750",  // UK
		"+8613800000000", // CN
	}
	for _, s := range notTR {
		_, err := a.LookupByPhone(context.Background(), s)
		if !errors.Is(err, ErrUnknownOperator) {
			t.Errorf("LookupByPhone(%q) err = %v, want ErrUnknownOperator", s, err)
		}
	}
}

func TestMNPTRAdapter_LookupByPhone_UnknownTRPrefix(t *testing.T) {
	a := NewMNPTRAdapter()
	// +90 but a prefix not in the table (we have 500-559, so 560 is
	// safely unmapped).
	_, err := a.LookupByPhone(context.Background(), "+905600000000")
	if !errors.Is(err, ErrUnknownOperator) {
		t.Errorf("unknown prefix err = %v, want ErrUnknownOperator", err)
	}
}

func TestMNPTRAdapter_LookupByIP_ReturnsUnknown(t *testing.T) {
	a := NewMNPTRAdapter()
	// The MNP adapter is phone-only; IP lookups must return
	// ErrUnknownOperator so the orchestrator can move on to the
	// IP reverse adapter.
	_, err := a.LookupByIP(context.Background(), "8.8.8.8")
	if !errors.Is(err, ErrUnknownOperator) {
		t.Errorf("MNP.LookupByIP err = %v, want ErrUnknownOperator", err)
	}
}

func TestMNPTRTableSize_IsAboveFloor(t *testing.T) {
	// We expect at least the 3 carriers × several prefixes each.
	// If this drops below 20, somebody accidentally deleted rows.
	if MNPTRTableSize() < 20 {
		t.Errorf("TR MNP table shrank: %d rows (want >= 20)", MNPTRTableSize())
	}
}

func TestLookupTRPrefixByCode(t *testing.T) {
	row, ok := lookupTRPrefixByCode("532")
	if !ok || row.Operator != "turkcell" {
		t.Errorf("lookupTRPrefixByCode(532) = (%+v, %v), want (turkcell, true)", row, ok)
	}
	_, ok = lookupTRPrefixByCode("999")
	if ok {
		t.Errorf("lookupTRPrefixByCode(999) = present, want absent")
	}
}

func TestMNPTRAdapter_NowIsCallable(t *testing.T) {
	// Smoke test: NewMNPTRAdapter returns a usable adapter.
	a := NewMNPTRAdapter()
	if a == nil {
		t.Fatal("NewMNPTRAdapter returned nil")
	}
	if a.now == nil {
		t.Fatal("adapter.now not initialized")
	}
	if _, err := a.LookupByPhone(context.Background(), "+905320000000"); err != nil {
		t.Errorf("first call failed: %v", err)
	}
}
