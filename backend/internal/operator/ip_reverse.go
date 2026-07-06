// ip_reverse.go — IP reverse-lookup adapter for the Operator Tespit
// Servisi.
//
// Per HANDOFF §4 PR-3 + Sprint 3 PR-23, this adapter answers "which
// carrier does this IP belong to?" by combining four sources in
// priority order:
//
//  1. RDAP (Registration Data Access Protocol) — the IETF standard
//     HTTP-based replacement for whois. Authoritative for the
//     registry that allocates the IP block (RIPE for Europe /
//     Middle East, ARIN for North America, APNIC for Asia-Pacific,
//     AFRINIC for Africa, LACNIC for Latin America). Sprint 3
//     wires this up as the PRIMARY live source.
//
//  2. Whois — the legacy port-43 protocol. Still authoritative for
//     some registries (e.g. RIPE for historical allocations) and
//     used as a FALLBACK when RDAP is unavailable.
//
//  3. A local ASN database (CIDR ranges → operator) — always
//     available, fast, no network. Acts as the FALLBACK when both
//     RDAP and whois are unreachable, or for development where the
//     adapter is used offline.
//
//  4. "unknown" — when none of the above know the answer.
//
// SCOPE:
//   - IPv4 first-class in the RDAP + whois paths. IPv6 is accepted
//     at the API surface (RDAP supports it natively; whois via the
//     same registries); entries with no match return "unknown".
//   - Private / loopback addresses (RFC 1918, 127.0.0.0/8, ::1) are
//     rejected by RDAP/whois servers; the local table also has no
//     entry, so they fall through to "unknown" with confidence 0.0.
//     They should never appear on a real telemetry row anyway.
//   - Confidence 0.95 (RDAP/whois), 0.80 (ASN table).
//
// POST-MVP (Sprint 4+):
//   - Add ASN-specific feeds (RIPE STATIC, APNIC delegated stats).
//     ASN lookup is DEFERRED per Sprint 3 scope (PR-23 task).
//   - Add an offline ASN database file (e.g. ip2asn combined
//     snapshot, updated weekly).
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
// Lookup order:
//   1. Local ASN table (most specific CIDR wins).
//   2. RDAP — registry HTTP, authoritative.
//   3. Whois — port-43, fallback for registries that don't expose
//      RDAP for legacy allocations.
//   4. ErrUnknownOperator → orchestrator will return "unknown".
//
// All lookups honour ctx.Done() — a stalled RDAP request or a TCP
// timeout in whois is bounded by the per-call timeout passed to the
// constructor (default 5s).
//
// The HTTP and whois clients are injected so tests can substitute
// httptest.Server-backed clients; in production the defaults are
// used (RDAP bootstraps via https://rdap.org/, whois uses the
// per-RIR servers).
//
// whoisFn is the Sprint-1 backwards-compat path: when set (by
// NewIPReverseAdapterWithWhois), it takes precedence over the
// WhoisClient. This preserves the Sprint-1 test API without
// pulling the closure shape into the new WhoisClient.Lookup path.
type IPReverseAdapter struct {
	now     func() time.Time
	rdap    *RDAPClient
	whois   *WhoisClient
	whoisFn func(ctx context.Context, ip netip.Addr) (*OperatorInfo, error)
	timeout time.Duration
}

// NewIPReverseAdapter returns the default adapter: local ASN table
// only, no network. This matches the Sprint-1 default and is what
// unit tests + offline-development environments use.
//
// For the production live-network behavior (RDAP + whois against
// the public RIRs), call NewIPReverseAdapterLive.
func NewIPReverseAdapter() *IPReverseAdapter {
	return &IPReverseAdapter{
		now:     time.Now,
		rdap:    nil,
		whois:   nil,
		timeout: 5 * time.Second,
	}
}

// NewIPReverseAdapterLive returns an adapter that performs real
// RDAP + whois lookups against the public RIR endpoints, with the
// local ASN table consulted first. Use this in production
// deployments where egress to rdap.org / whois.ripe.net is allowed.
func NewIPReverseAdapterLive() *IPReverseAdapter {
	rdap, _ := NewRDAPClient(RDAPConfig{
		BootstrapURL: defaultRDAPBootstrap,
		HTTPTimeout:  5 * time.Second,
	})
	whois, _ := NewWhoisClient(WhoisConfig{
		Timeout:    5 * time.Second,
		ServerByCC: defaultWhoisServers,
	})
	return &IPReverseAdapter{
		now:     time.Now,
		rdap:    rdap,
		whois:   whois,
		timeout: 5 * time.Second,
	}
}

// NewIPReverseAdapterWithDeps lets callers (mainly tests) inject
// custom RDAP + whois clients. The local table is always consulted
// first; the injected clients are only called on a miss. nil values
// for either client are accepted and disable that lookup path
// (e.g. nil rdap → skip RDAP, whois-only).
func NewIPReverseAdapterWithDeps(rdap *RDAPClient, whois *WhoisClient) *IPReverseAdapter {
	a := NewIPReverseAdapter()
	if rdap != nil {
		a.rdap = rdap
	}
	if whois != nil {
		a.whois = whois
	}
	return a
}

// NewIPReverseAdapterWithWhois is the Sprint-1 backwards-compat
// constructor preserved verbatim. It wires a closure-style whois
// function into the adapter (no RDAP). Existing Sprint-1 tests
// call this constructor and continue to work.
//
// When whoisFn is non-nil, LookupByIP short-circuits to it
// instead of consulting the WhoisClient — see the lookup body.
func NewIPReverseAdapterWithWhois(fn func(ctx context.Context, ip netip.Addr) (*OperatorInfo, error)) *IPReverseAdapter {
	a := NewIPReverseAdapter()
	a.rdap = nil
	if fn != nil {
		a.whoisFn = fn
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
//   2. RDAP HTTP lookup against the appropriate RIR.
//   3. Whois (port 43) fallback against the appropriate RIR.
//   4. ErrUnknownOperator → orchestrator will return an "unknown" info.
//
// Any non-ErrUnknownOperator error from RDAP/whois is propagated,
// NOT masked as "unknown" — a real network outage is operator-
// visible and should not look like a clean "we don't know" answer.
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

	// (1) Local ASN table — fast path, no network.
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
			applyIPMask(info, ip)
			return info, nil
		}
	}

	// Bound the network calls so a stalled registry cannot pin
	// the request goroutine.
	callCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	// (2) RDAP. Authoritative; preferred over whois.
	if a.rdap != nil {
		info, err := a.rdap.Lookup(callCtx, addr)
		if err == nil && info != nil {
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
		if err != nil && !errors.Is(err, ErrUnknownOperator) {
			return nil, err
		}
	}

	// (3) Whois fallback. Same error semantics as RDAP. The
	// whoisFn path (Sprint-1 backwards-compat) takes precedence
	// over the WhoisClient when both are set.
	if a.whoisFn != nil {
		info, err := a.whoisFn(callCtx, addr)
		if err == nil && info != nil {
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
		if err != nil && !errors.Is(err, ErrUnknownOperator) {
			return nil, err
		}
	} else if a.whois != nil {
		info, err := a.whois.Lookup(callCtx, addr)
		if err == nil && info != nil {
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
		if err != nil && !errors.Is(err, ErrUnknownOperator) {
			return nil, err
		}
	}

	// (4) No source could resolve.
	return nil, fmt.Errorf("ip %q: %w", ip, ErrUnknownOperator)
}

// ASNTableSize is exported for tests + observability.
func ASNTableSize() int { return len(asnTable) }
