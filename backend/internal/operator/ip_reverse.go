// ip_reverse.go — IP reverse-lookup adapter for the Operator Tespit
// Servisi.
//
// Per HANDOFF §4 PR-3, this adapter answers "which carrier does this
// IP belong to?" by combining two sources:
//
//  1. A local ASN database (CIDR ranges → operator) — always
//     available, fast, no network. The MVP-only source.
//  2. Symbolic RIPE/ARIN whois calls — wired as a no-op closure in
//     this PR; Sprint 2 swaps in real HTTP. The interface is stable
//     so the orchestrator (Service) and the cache layer don't need
//     to change.
//
// SCOPE:
//   - IPv4 only in the local table. IPv6 is accepted at the API
//     surface but every entry currently returns "unknown" — the
//     IPv6 BGP feeds we'd need are out of MVP scope (HANDOFF §9).
//   - Private / loopback addresses (RFC 1918, 127.0.0.0/8, ::1) are
//     NOT in the table and fall through to "unknown" with confidence
//     0.0 — they should never appear on a real telemetry row anyway.
//   - Confidence 0.80: ASN-level mapping is coarse. A real whois
//     response (Sprint 2) will bump this.
//
// POST-MVP (Sprint 2+):
//   - Wire up RIPE REST API for /ip/194.9.0.0/16 style queries.
//   - Add an offline ASN database file (e.g. ip2asn combined snapshot,
//     updated weekly).
//   - Add a 30-day TTL on the whois cache entry (vs 24h for the phone
//     table — whois assignments move much more slowly).
package operator

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"time"
)

// asnEntry is one row in the local ASN table. The CIDR is held as a
// netip.Prefix for fast, allocation-free membership tests.
type asnEntry struct {
	CIDR         netip.Prefix
	Country      string // ISO 3166-1 alpha-2, e.g. "TR", "US"
	Operator     string // enum string, matches telemetry schema
	OperatorName string
	MCC          string // empty for non-mobile
	MNC          string // empty for non-mobile
	Source       Source // typically SourceASNDB for the local table
}

// asnTable is the offline IP-range → operator table used in the MVP.
// Coverage is intentionally limited to a handful of well-known ranges
// to keep the test vectors stable; add more rows as the UseCase
// expands. Sorted by prefix length (most specific first) so a /15
// match beats a /8 match when both apply.
//
// Sources (manual, not auto-fetched):
//   - RIPE STATIC "asn-block" feed snapshot
//   - Turk Telekom / Turkcell / Vodafone TR public IP allocation pages
var asnTable = []asnEntry{
	// ---- TR (country code +90, MCC 286) ----------------------------------
	// Turkcell
	{CIDR: mustPrefix("78.180.0.0/15"), Country: "TR", Operator: "turkcell", OperatorName: "Turkcell", MCC: "286", MNC: "01", Source: SourceASNDB},
	{CIDR: mustPrefix("5.46.0.0/15"), Country: "TR", Operator: "turkcell", OperatorName: "Turkcell", MCC: "286", MNC: "01", Source: SourceASNDB},
	// Vodafone TR
	{CIDR: mustPrefix("31.140.0.0/17"), Country: "TR", Operator: "vodafone_tr", OperatorName: "Vodafone TR", MCC: "286", MNC: "02", Source: SourceASNDB},
	{CIDR: mustPrefix("213.74.0.0/16"), Country: "TR", Operator: "vodafone_tr", OperatorName: "Vodafone TR", MCC: "286", MNC: "02", Source: SourceASNDB},
	// Turk Telekom
	{CIDR: mustPrefix("88.224.0.0/12"), Country: "TR", Operator: "turk_telekom", OperatorName: "Turk Telekom", MCC: "286", MNC: "03", Source: SourceASNDB},
	{CIDR: mustPrefix("85.96.0.0/12"), Country: "TR", Operator: "turk_telekom", OperatorName: "Turk Telekom", MCC: "286", MNC: "03", Source: SourceASNDB},

	// ---- US (country code +1, MCC 310/311/312/313...) --------------------
	// AT&T (approximate; covers a major slice of their consumer block).
	{CIDR: mustPrefix("12.0.0.0/8"), Country: "US", Operator: "att", OperatorName: "AT&T", MCC: "310", MNC: "030", Source: SourceASNDB},
	// Verizon (approximate).
	{CIDR: mustPrefix("71.0.0.0/8"), Country: "US", Operator: "verizon", OperatorName: "Verizon", MCC: "311", MNC: "480", Source: SourceASNDB},
	// T-Mobile US (approximate).
	{CIDR: mustPrefix("172.32.0.0/11"), Country: "US", Operator: "tmobile_us", OperatorName: "T-Mobile US", MCC: "310", MNC: "260", Source: SourceASNDB},

	// ---- DE (country code +49, MCC 262) --------------------------------
	// Deutsche Telekom
	{CIDR: mustPrefix("87.128.0.0/10"), Country: "DE", Operator: "deutsche_telekom", OperatorName: "Deutsche Telekom", MCC: "262", MNC: "01", Source: SourceASNDB},
}

// mustPrefix parses a CIDR string and panics on failure. Only used
// for compile-time table entries — same rationale as mustParseIP.
func mustPrefix(s string) netip.Prefix {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		panic("operator: mustPrefix: invalid table entry: " + s)
	}
	return p
}

// IPReverseAdapter implements OperatorLookup for IP addresses.
//
// whoisLookup is an injected dependency — the default is a no-op
// closure (no network call in MVP). Tests can pass a stub to assert
// the orchestrator wired the whois call correctly when the local
// table misses.
type IPReverseAdapter struct {
	now        func() time.Time
	whoisLookup func(ctx context.Context, ip netip.Addr) (*OperatorInfo, error)
}

// NewIPReverseAdapter returns the default adapter: local table only,
// no whois. This is the production setup for MVP.
func NewIPReverseAdapter() *IPReverseAdapter {
	return &IPReverseAdapter{
		now: time.Now,
		// Default whois is a no-op: it returns ErrUnknownOperator so
		// the orchestrator knows the local table also missed and we
		// have nothing to say. Sprint 2 replaces this body.
		whoisLookup: func(_ context.Context, _ netip.Addr) (*OperatorInfo, error) {
			return nil, ErrUnknownOperator
		},
	}
}

// NewIPReverseAdapterWithWhois lets callers (mainly tests) inject a
// custom whois function. The local table is always consulted first;
// the whois function is only called on a miss.
func NewIPReverseAdapterWithWhois(fn func(ctx context.Context, ip netip.Addr) (*OperatorInfo, error)) *IPReverseAdapter {
	a := NewIPReverseAdapter()
	if fn != nil {
		a.whoisLookup = fn
	}
	return a
}

// Compile-time interface check.
var _ OperatorLookup = (*IPReverseAdapter)(nil)

// sortedBySpecificity returns the ASN table ordered by descending
// prefix length — used so a more specific range wins over a broader
// one. We compute it once on first call.
var asnTableSorted []asnEntry

func init() {
	asnTableSorted = make([]asnEntry, len(asnTable))
	copy(asnTableSorted, asnTable)
	sort.SliceStable(asnTableSorted, func(i, j int) bool {
		// Higher bits = more specific. netip.Prefix.Bits returns
		// the mask length; larger = more specific.
		return asnTableSorted[i].CIDR.Bits() > asnTableSorted[j].CIDR.Bits()
	})
}

// LookupByPhone is a no-op for the IP adapter — it explicitly does
// not resolve phone numbers. Returns ErrUnknownOperator.
func (a *IPReverseAdapter) LookupByPhone(_ context.Context, e164 string) (*OperatorInfo, error) {
	return nil, fmt.Errorf("IP adapter does not resolve phone %q: %w", e164, ErrUnknownOperator)
}

// LookupByIP resolves an IP string to an OperatorInfo. Lookup order:
//   1. Local ASN table (most specific CIDR wins).
//   2. Injected whoisLookup closure (no-op in MVP).
//   3. ErrUnknownOperator → orchestrator will return an "unknown" info.
func (a *IPReverseAdapter) LookupByIP(ctx context.Context, ip string) (*OperatorInfo, error) {
	if !looksLikeIP(ip) {
		return nil, fmt.Errorf("ip %q: %w", ip, ErrInvalidInput)
	}
	// Strip brackets for v6 Zone-URI style.
	raw := ip
	if raw[0] == '[' {
		raw = raw[1 : len(raw)-1]
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return nil, fmt.Errorf("ip %q: %w", ip, ErrInvalidInput)
	}

	// (1) Local ASN table.
	for _, e := range asnTableSorted {
		if e.CIDR.Contains(addr) {
			qt := QueryIPv4
			if addr.Is6() && !addr.Is4In6() {
				qt = QueryIPv6
			}
			info := &OperatorInfo{
				QueryType:    qt,
				QueryValue:   "", // filled below
				Operator:     e.Operator,
				OperatorName: e.OperatorName,
				Country:      e.Country,
				MCC:          e.MCC,
				MNC:          e.MNC,
				Source:       e.Source,
				Confidence:   0.80,
				Timestamp:    a.now().UTC(),
			}
			// PRIVACY (ADR-0006): apply MaskIP before returning. The
			// raw IP MUST NEVER land in cache or response.
			applyIPMask(info, ip)
			return info, nil
		}
	}

	// (2) Injected whois closure.
	if a.whoisLookup != nil {
		info, err := a.whoisLookup(ctx, addr)
		if err == nil && info != nil {
			// Override the query value with the MASKED form (the
			// whois closure may have set the raw IP). Apply masking
			// even if the closure already masked — idempotent.
			if info.QueryValue == "" {
				applyIPMask(info, ip)
			} else {
				applyIPMask(info, info.QueryValue)
			}
			if info.Timestamp.IsZero() {
				info.Timestamp = a.now().UTC()
			}
			return info, nil
		}
		// If whois returns ErrUnknownOperator, fall through to the
		// "unknown" result below. Any other error is propagated.
		if err != nil && !errors.Is(err, ErrUnknownOperator) {
			return nil, err
		}
	}

	// (3) No source could resolve — the orchestrator will turn this
	// into an "unknown" OperatorInfo with source=fallback_unknown.
	return nil, fmt.Errorf("ip %q: %w", ip, ErrUnknownOperator)
}

// ASNTableSize is exported for tests + observability.
func ASNTableSize() int { return len(asnTable) }
