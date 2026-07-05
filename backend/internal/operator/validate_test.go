// validate_test.go — unit tests for the input validators in validate.go.
package operator

import "testing"

func TestLooksLikeE164_Boundaries(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		// shortest legal: country code "1" + 1 digit
		{"+12", true},
		// longest legal: 15 digits after +
		{"+123456789012345", true},
		// one over
		{"+1234567890123456", false},
		// two short
		{"+1", false},
		// missing +
		{"12", false},
		// leading zero country code is invalid per ITU-T E.164
		{"+0123456789", false},
		// letters
		{"+90abc00000", false},
		// spaces
		{"+90 532 000 00 00", false},
	}
	for _, c := range cases {
		if got := looksLikeE164(c.in); got != c.want {
			t.Errorf("looksLikeE164(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestLooksLikeIP_Common(t *testing.T) {
	valid := []string{
		"8.8.8.8",
		"127.0.0.1",
		"::1",
		"2001:db8::1",
		"0.0.0.0",
	}
	for _, s := range valid {
		if !looksLikeIP(s) {
			t.Errorf("looksLikeIP(%q) = false, want true", s)
		}
	}
	invalid := []string{
		"",
		"not an ip",
		"999.999.999.999",
		"1.2.3",     // too few octets
		"1.2.3.4.5", // too many
	}
	for _, s := range invalid {
		if looksLikeIP(s) {
			t.Errorf("looksLikeIP(%q) = true, want false", s)
		}
	}
}

func TestIPVersion(t *testing.T) {
	cases := []struct {
		in     string
		wantV  string
		wantOk bool
	}{
		{"8.8.8.8", "v4", true},
		{"::1", "v6", true},
		{"2001:db8::1", "v6", true},
		{"not an ip", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		v, ok := ipVersion(c.in)
		if v != c.wantV || ok != c.wantOk {
			t.Errorf("ipVersion(%q) = (%q,%v), want (%q,%v)",
				c.in, v, ok, c.wantV, c.wantOk)
		}
	}
}

func TestMustParseIP_PanicsOnBadInput(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("mustParseIP should panic on invalid input")
		}
	}()
	_ = mustParseIP("not an ip")
}
