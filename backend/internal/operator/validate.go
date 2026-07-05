// validate.go — small, dependency-free input validators shared by
// the MNP and IP adapters. Kept in its own file so the adapter tables
// stay focused on the data, not the syntax checks.
package operator

import (
	"net"
	"net/netip"
	"strings"
)

// looksLikeE164 is a fast, allocation-free check for "does this look
// like a well-formed E.164 number?" It does NOT verify the country
// code is real — that needs an external table — but it catches the
// common garbage inputs (empty, no +, too long, non-digits).
//
// Rules (ITU-T E.164):
//   - must start with '+'
//   - 1 to 15 digits after the '+'
//   - total length 2..16 chars
//   - all chars after '+' are ASCII digits 0-9
func looksLikeE164(s string) bool {
	if len(s) < MinE164Length || len(s) > MaxE164Length {
		return false
	}
	if s[0] != '+' {
		return false
	}
	// The country code cannot be a leading 0 (E.164 §6.1).
	if s[1] == '0' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// looksLikeIP is a quick syntactic check. It does NOT resolve the
// address — that needs net.ParseIP / netip.ParseAddr.
func looksLikeIP(s string) bool {
	if s == "" {
		return false
	}
	// Trim a single pair of brackets for IPv6 Zone-URI style.
	// We don't accept zone IDs here — adapters will pass clean
	// strings in practice — but we tolerate the rare "[::1]".
	raw := s
	if raw[0] == '[' {
		raw = strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]")
	}
	if _, err := netip.ParseAddr(raw); err != nil {
		return false
	}
	return true
}

// ipVersion returns "v4" or "v6" for a syntactically-valid IP. Used by
// the IPReverseAdapter to set QueryType correctly.
func ipVersion(s string) (string, bool) {
	raw := s
	if raw != "" && raw[0] == '[' {
		raw = strings.TrimSuffix(strings.TrimPrefix(raw, "["), "]")
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return "", false
	}
	if addr.Is4() || addr.Is4In6() {
		return "v4", true
	}
	return "v6", true
}

// mustParseIP is a small helper that calls net.ParseIP and panics on
// failure — only used for compile-time table entries in this package.
// It's a "I checked this at code-review time" assertion, not a runtime
// validator.
func mustParseIP(s string) net.IP {
	ip := net.ParseIP(s)
	if ip == nil {
		panic("operator: mustParseIP: invalid table entry: " + s)
	}
	return ip
}
