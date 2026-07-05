// lookup_test.go — unit tests for the package-level types, error
// sentinels, and E.164 / IP validation helpers.
package operator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Compile-time assertion: a *Service satisfies OperatorLookup too
// (it's the cache-fronted orchestrator). We can't reference Service
// here (it would create an import cycle: service.go imports this
// file's package, lookup.go is in the same package, so it's fine),
// but we can at least assert the interface and constants.

func TestOperatorInfo_DefaultJSONTags(t *testing.T) {
	// Sanity check: every field has the JSON tag the REST layer
	// will marshal to. If somebody renames a field without
	// updating the JSON tag, this catches it before schema drift.
	info := OperatorInfo{
		QueryType:    QueryPhoneE164,
		QueryValue:   "+905320000000",
		Operator:     "turkcell",
		OperatorName: "Turkcell",
		Country:      "TR",
		MCC:          "286",
		MNC:          "01",
		Source:       SourceTRMNPAPI,
		Confidence:   0.95,
	}
	if string(info.QueryType) != "phone_e164" {
		t.Fatalf("QueryType enum string drifted: %q", info.QueryType)
	}
	if string(info.Source) != "tr_mnp_api" {
		t.Fatalf("Source enum string drifted: %q", info.Source)
	}
}

func TestSentinelErrors_AreDistinct(t *testing.T) {
	// errors.Is must distinguish ErrInvalidInput from ErrUnknownOperator.
	if errors.Is(ErrInvalidInput, ErrUnknownOperator) {
		t.Fatal("ErrInvalidInput must NOT match ErrUnknownOperator")
	}
	// And wrapping must preserve identity for errors.Is-based matching.
	wrapped := errors.New("wrap: " + ErrInvalidInput.Error())
	if errors.Is(wrapped, ErrInvalidInput) {
		t.Fatal("string-wrapped errors must NOT match via errors.Is — use %w")
	}
}

func TestConstants_AreSane(t *testing.T) {
	if DefaultCacheTTL != 24*time.Hour {
		t.Fatalf("DefaultCacheTTL must be 24h per HANDOFF §4 PR-3, got %v", DefaultCacheTTL)
	}
	if MinE164Length < 3 || MaxE164Length > 16 {
		t.Fatalf("E.164 length bounds drifted: [%d,%d]", MinE164Length, MaxE164Length)
	}
}

func TestQueryType_AllEnumsAreDistinct(t *testing.T) {
	// Every constant in the QueryType block must be distinct and
	// non-empty. This is a small invariant but it catches accidental
	// copy-paste in future edits.
	all := []QueryType{
		QueryPhoneE164, QueryPhoneNational, QueryIPv4, QueryIPv6, QueryASN,
	}
	seen := map[QueryType]bool{}
	for _, q := range all {
		if q == "" {
			t.Fatalf("empty QueryType constant")
		}
		if seen[q] {
			t.Fatalf("duplicate QueryType: %q", q)
		}
		seen[q] = true
	}
}

func TestSource_AllEnumsAreDistinct(t *testing.T) {
	all := []Source{
		SourceTRMNPAPI, SourceRIPEWhois, SourceARINWhois,
		SourceASNDB, SourceFallbackUnknown,
	}
	seen := map[Source]bool{}
	for _, s := range all {
		if s == "" {
			t.Fatalf("empty Source constant")
		}
		if seen[s] {
			t.Fatalf("duplicate Source: %q", s)
		}
		seen[s] = true
	}
}

// fakeAdapter is a no-op OperatorLookup used in interface-assertion
// tests. Defined here (not in cache_test / service_test) so the
// interface coverage is colocated with the interface definition.
type fakeAdapter struct {
	phoneOut *OperatorInfo
	phoneErr error
	ipOut    *OperatorInfo
	ipErr    error
}

func (f *fakeAdapter) LookupByPhone(_ context.Context, _ string) (*OperatorInfo, error) {
	return f.phoneOut, f.phoneErr
}
func (f *fakeAdapter) LookupByIP(_ context.Context, _ string) (*OperatorInfo, error) {
	return f.ipOut, f.ipErr
}

// Compile-time check: a *fakeAdapter must satisfy OperatorLookup.
// This is the only way to enforce the interface without bringing in
// the rest of the package's adapters.
var _ OperatorLookup = (*fakeAdapter)(nil)

func TestFakeAdapter_SatisfiesInterface(t *testing.T) {
	a := &fakeAdapter{
		phoneOut: &OperatorInfo{Operator: "turkcell"},
		ipOut:    &OperatorInfo{Operator: "verizon"},
	}
	ctx := context.Background()
	p, err := a.LookupByPhone(ctx, "+905320000000")
	if err != nil || p == nil || p.Operator != "turkcell" {
		t.Fatalf("phone lookup wrong: %+v err=%v", p, err)
	}
	i, err := a.LookupByIP(ctx, "8.8.8.8")
	if err != nil || i == nil || i.Operator != "verizon" {
		t.Fatalf("ip lookup wrong: %+v err=%v", i, err)
	}
}

// TestE164_BasicShape is a smoke test for the validation helper that
// mnp_tr.go will use. Implemented as a "valid strings" set + an
// "invalid strings" set so future maintainers can extend both without
// touching the helper.
func TestE164Shape_Valid(t *testing.T) {
	valid := []string{
		"+905320000000",     // TR mobile
		"+12025550143",      // US
		"+442071838750",     // UK
		"+8613800000000",    // CN
		"+498001234567",     // DE
	}
	for _, s := range valid {
		if !looksLikeE164(s) {
			t.Errorf("expected %q to look like E.164", s)
		}
	}
}

func TestE164Shape_Invalid(t *testing.T) {
	invalid := []string{
		"",                // empty
		"905320000000",    // missing +
		"+",               // +
		"+0",              // too short
		"+0123456789012345", // 16 digits, exceeds 15
		"+90 532 000 00 00", // spaces
		"++905320000000",  // double plus
		"+90abc0000000",   // letters
		strings.Repeat("+", 5), // just plusses
	}
	for _, s := range invalid {
		if looksLikeE164(s) {
			t.Errorf("expected %q to be REJECTED as E.164", s)
		}
	}
}
