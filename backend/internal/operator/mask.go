// mask.go - privacy-preserving query masking helpers.
//
// Per AGENTS.md / ADR-0006 §Veri Minimizasyonu:
//
//   - The raw counterpart phone number MUST NEVER leave the resolver
//     unmasked. We always store/cache the masked form, and the
//     masked form is what REST responses carry.
//   - The raw IP MUST NEVER be stored or logged. We store/cache
//     /24 (v4) or /48 (v6) subnet forms only.
//
// The masking is deterministic given the input, so the same
// canonical E.164 / IP always yields the same mask - useful for
// downstream joins that don't need to re-resolve the operator.
//
// Verifier §6 report (PR-3 review): raw E.164 / IP MUST be masked
// before any cache write or HTTP response. This file provides
// those helpers.

package operator

import (
	"net/netip"
	"strings"
)

// MaskPhoneE164 returns a partially-redacted form of an E.164 phone
// number suitable for storage in cache / response bodies.
//
// Format:
//
//   - "+" kept verbatim.
//   - Country code (digits after "+" until the first non-digit) is
//     preserved as-is - the caller already normalized the input, so
//     the country code is part of the canonical form.
//   - Subscriber digits: if >= 7, keep the first 3 + "*****" + last 2.
//                     if  <  7, fully mask with one "*" per digit.
//
// Examples (pinned by tests):
//
//   MaskPhoneE164("+905301112233") == "+90530*****33"
//   MaskPhoneE164("+14155550100")  == "+1415*****00"   // NANPA: cc=1, area=415, sub=5550100
//   MaskPhoneE164("+1234567")      == "+1******"       // len 7 falls into <7 branch, cc=1, sub 6 digits masked
func MaskPhoneE164(e164 string) string {
	if e164 == "" {
		return ""
	}
	if e164[0] != '+' {
		// Defensive: caller is supposed to pass normalized E.164.
		// If it doesn't, treat the whole thing as subscriber digits.
		return strings.Repeat("*", len(e164))
	}
	digits := e164[1:]
	ccLen := countryCodeLength(digits)
	sub := digits[ccLen:]
	if len(sub) >= 7 {
		return "+" + digits[:ccLen] + sub[:3] + "*****" + sub[len(sub)-2:]
	}
	return "+" + digits[:ccLen] + strings.Repeat("*", len(sub))
}

// MaskPhone is a short alias for MaskPhoneE164.
func MaskPhone(e164 string) string { return MaskPhoneE164(e164) }

// countryCodeLength heuristically splits an E.164-style digit
// string into a country-code portion (1-3 digits) and a subscriber
// portion. Real phone-number libraries use libphonenumber; we
// keep this minimal because the actual subscriber range is what
// the privacy rule protects.
//
// Mapping (subset):
//
//   - 90 + 5 mobile  -> country code = 2
//   - 1  + 10 NANPA  -> country code = 1
//   - 7  + 10 RU     -> country code = 1
//   - 44 + 9..10 UK  -> country code = 2
//   - 49 + 9..11 DE  -> country code = 2
//   - 33 + 9 FR      -> country code = 2
func countryCodeLength(digits string) int {
	switch {
	case len(digits) >= 11 && strings.HasPrefix(digits, "90"):
		return 2
	case len(digits) >= 11 && len(digits) >= 1 && (digits[0] == '1' || digits[0] == '7'):
		return 1
	case len(digits) >= 10 && strings.HasPrefix(digits, "44"):
		return 2
	case len(digits) >= 11 && (strings.HasPrefix(digits, "49") || strings.HasPrefix(digits, "33")):
		return 2
	}
	if len(digits) >= 7 {
		return 1
	}
	return 0
}

// MaskIP returns a privacy-preserving subnet form of an IP address:
//
//   - IPv4 (including v4-in-v6 like "::ffff:1.2.3.4") -> "/24" subnet
//     (e.g. 88.240.5.12 -> 88.240.5.0/24).
//   - IPv6 -> "/48" subnet (e.g. 2a01:5ec0:1234:5678::1 -> 2a01:5ec0:1234::/48).
//
// Returns empty string for unparseable input so the caller can
// fall back to a "unknown" OperatorInfo rather than leak.
func MaskIP(s string) string {
	return maskAddr(s)
}

// maskAddr is the implementation behind MaskIP, factored out so
// the netip dependency stays scoped here.
//
// IMPORTANT: netip.PrefixFrom(addr, bits) in Go 1.22+ sets the
// prefix length but does NOT zero the host bits. The previous
// implementation returned "88.240.5.12/24" for input
// "88.240.5.12", leaking the raw IP. We therefore call .Masked()
// (or, equivalently, addr.Prefix(bits)) which zeroes the host
// bits before formatting. Empirical probe against net/netip
// on Go 1.26.4 confirms both forms produce the masked output.
func maskAddr(s string) string {
	if s == "" {
		return ""
	}
	// Strip CIDR suffix if present (already-masked values pass through).
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return ""
	}
	// Unmap v4-in-v6 (e.g. "::ffff:1.2.3.4") to its v4 form so
	// the /24 branch produces a v4 mask ("1.2.3.0/24"), not a
	// meaningless v6 mask ("::/24") that exposes zero host bits.
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	if addr.Is4() {
		return netip.PrefixFrom(addr, 24).Masked().String()
	}
	return netip.PrefixFrom(addr, 48).Masked().String()
}

// applyPhoneMask writes the masked form of canonical into
// info.QueryValue. Used by MNPTRAdapter / Service-level finalize so
// the cache and the response always carry the same masked form.
func applyPhoneMask(info *OperatorInfo, canonical string) {
	if info == nil {
		return
	}
	info.QueryValue = MaskPhoneE164(canonical)
}

// applyIPMask writes the masked subnet form into info.QueryValue.
func applyIPMask(info *OperatorInfo, canonical string) {
	if info == nil {
		return
	}
	info.QueryValue = MaskIP(canonical)
}
