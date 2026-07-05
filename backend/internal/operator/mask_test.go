// mask_test.go - regression tests for the privacy-preserving masking
// helpers in mask.go.
//
// PR-3 Verifier §6 follow-up review at c7b3704 caught a critical
// bug: netip.PrefixFrom(addr, bits) sets the prefix length but
// does NOT zero the host bits. The previous maskAddr returned
// "88.240.5.12/24" for input "88.240.5.12" - leaking the raw IP
// into cache and REST responses. These tests pin the corrected
// behavior so the regression cannot recur.
package operator

import "testing"

func TestMaskPhoneE164_KnownAnswers(t *testing.T) {
	// Pinned by the docstring above and the PR-3 Verifier
	// follow-up. NANPA keeps cc=1 + area=415 + subscriber=5550100,
	// so the masked form preserves 4 chars + "*****" + 2 chars.
	cases := []struct {
		in, want string
	}{
		{"+905301112233", "+90530*****33"}, // TR: cc=90, area=530, sub=1112233
		{"+14155550100", "+1415*****00"},   // NANPA: cc=1, area=415, sub=5550100
		{"+1234567", "+1******"},           // short - cc=1, sub 6 digits -> 6 stars
		{"", ""},
		{"905301112233", "************"},   // no "+" - fully masked (defensive)
	}
	for _, tc := range cases {
		got := MaskPhoneE164(tc.in)
		if got != tc.want {
			t.Errorf("MaskPhoneE164(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMaskPhone_AliasMatchesMaskPhoneE164(t *testing.T) {
	in := "+905301112233"
	if MaskPhone(in) != MaskPhoneE164(in) {
		t.Errorf("MaskPhone alias disagrees with MaskPhoneE164: %q vs %q",
			MaskPhone(in), MaskPhoneE164(in))
	}
}

// TestMaskIP_ActuallyMasks is the KAT regression pin for the
// privacy-critical bug at c7b3704. The PR-3 Verifier follow-up
// showed netip.PrefixFrom(addr, bits).String() returned the raw
// IP with a /24 suffix instead of zeroing the host bits. The new
// .Masked() call collapses every host in a subnet onto the same
// masked form. v4 and v6 are exercised as subtests so a single
// `go test -run TestMaskIP_ActuallyMasks` is enough to verify both.
func TestMaskIP_ActuallyMasks(t *testing.T) {
	t.Run("v4/24", func(t *testing.T) {
		cases := []struct {
			in, want string
		}{
			{"88.240.5.12", "88.240.5.0/24"},
			{"88.240.5.255", "88.240.5.0/24"},
			{"88.240.5.0", "88.240.5.0/24"},
			{"1.2.3.4", "1.2.3.0/24"},
			{"192.168.1.1", "192.168.1.0/24"},
		}
		for _, tc := range cases {
			got := MaskIP(tc.in)
			if got != tc.want {
				t.Errorf("MaskIP(%q) = %q, want %q", tc.in, got, tc.want)
			}
		}
	})
	t.Run("v6/48", func(t *testing.T) {
		cases := []struct {
			in, want string
		}{
			{"2a01:5ec0:1234:5678::1", "2a01:5ec0:1234::/48"},
			{"2a01:5ec0:1234:5678:9abc:def0:1234:5678", "2a01:5ec0:1234::/48"},
			{"2001:db8::1", "2001:db8::/48"},
		}
		for _, tc := range cases {
			got := MaskIP(tc.in)
			if got != tc.want {
				t.Errorf("MaskIP(%q) = %q, want %q", tc.in, got, tc.want)
			}
		}
	})
}

// TestMaskPhoneE164_DocstringExamplesMatch is the KAT pin for
// the corrected MaskPhoneE164 docstring at L40-42 of mask.go.
// The previous KAT values ("+14155*****00" and "+*******") did
// not match the actual countryCodeLength heuristic on Go 1.26.4.
// Pinning the observed output prevents the next reader from
// being misled by stale examples.
func TestMaskPhoneE164_DocstringExamplesMatch(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"+905301112233", "+90530*****33"},
		{"+14155550100", "+1415*****00"},
		{"+1234567", "+1******"},
	}
	for _, tc := range cases {
		got := MaskPhoneE164(tc.in)
		if got != tc.want {
			t.Errorf("MaskPhoneE164(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMaskIP_V4In6_UnmapsToV4(t *testing.T) {
	// "::ffff:1.2.3.4" is an IPv4-mapped IPv6. If we treat it
	// as plain v6 with /24 we'd get "::/24" - which masks
	// nothing useful. Unmap to the embedded v4 first.
	cases := []struct {
		in, want string
	}{
		{"::ffff:1.2.3.4", "1.2.3.0/24"},
		{"::ffff:88.240.5.12", "88.240.5.0/24"},
	}
	for _, tc := range cases {
		got := MaskIP(tc.in)
		if got != tc.want {
			t.Errorf("MaskIP(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMaskIP_EmptyAndUnparseable(t *testing.T) {
	cases := []string{
		"",
		"not-an-ip",
		"999.999.999.999",
		"::::",
	}
	for _, in := range cases {
		got := MaskIP(in)
		if got != "" {
			t.Errorf("MaskIP(%q) = %q, want \"\"", in, got)
		}
	}
}

func TestMaskIP_AlreadyMaskedPassesThrough(t *testing.T) {
	// Calling MaskIP on an already-masked CIDR should be a fixed
	// point. Strip the "/" suffix, re-mask, get the same value.
	cases := []struct {
		in, want string
	}{
		{"88.240.5.0/24", "88.240.5.0/24"},
		{"2a01:5ec0:1234::/48", "2a01:5ec0:1234::/48"},
		// Already-masked but with non-zero host should still
		// re-mask down to the canonical /24 or /48.
		{"88.240.5.42/24", "88.240.5.0/24"},
	}
	for _, tc := range cases {
		got := MaskIP(tc.in)
		if got != tc.want {
			t.Errorf("MaskIP(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestApplyPhoneMask_WritesMaskedQueryValue(t *testing.T) {
	info := &OperatorInfo{QueryValue: "raw"}
	applyPhoneMask(info, "+905301112233")
	if info.QueryValue != "+90530*****33" {
		t.Errorf("applyPhoneMask: QueryValue = %q, want masked form",
			info.QueryValue)
	}
}

func TestApplyPhoneMask_NilInfoIsNoOp(t *testing.T) {
	applyPhoneMask(nil, "+905301112233") // must not panic
}

func TestApplyIPMask_WritesMaskedQueryValue(t *testing.T) {
	info := &OperatorInfo{QueryValue: "raw"}
	applyIPMask(info, "88.240.5.12")
	// Regression: previously returned "88.240.5.12/24" - the raw
	// IP. Must now be the masked form.
	if info.QueryValue != "88.240.5.0/24" {
		t.Errorf("applyIPMask: QueryValue = %q, want %q (privacy critical)",
			info.QueryValue, "88.240.5.0/24")
	}
}

func TestApplyIPMask_NilInfoIsNoOp(t *testing.T) {
	applyIPMask(nil, "88.240.5.12") // must not panic
}
