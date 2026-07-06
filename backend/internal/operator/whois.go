// whois.go — port-43 whois client.
//
// Whois is the legacy line-oriented protocol used by all five RIRs
// (RIPE, ARIN, APNIC, AFRINIC, LACNIC) plus many local registries.
// Sprint 3 (PR-23) keeps whois as the FALLBACK after RDAP — some
// legacy allocations are still whois-only.
//
// The protocol is trivial: open a TCP connection to the registry's
// whois port (43), send "<query>\r\n", read the response lines
// until the connection is closed by the peer (most registries
// close on EOF after the response is complete) or until our
// timeout fires.
//
// PRIVACY (ADR-0006): we parse only the fields we need for
// operator identification (netname, descr, country, origin AS).
// Registrant / admin / abuse emails are deliberately skipped.
package operator

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// defaultWhoisServers is the per-RIR whois server used when the
// caller doesn't override. Keyed by the RIR's ISO 3166 country
// code (the registry that allocates IP blocks in that country).
// Falls back to "whois.ripe.net" for European / Middle Eastern
// addresses — RIPE serves all of Europe / Middle East.
var defaultWhoisServers = map[string]string{
	// RIPE NCC: Europe, Middle East, parts of Central Asia.
	"":       "whois.ripe.net",
	"TR":     "whois.ripe.net",
	"DE":     "whois.ripe.net",
	"FR":     "whois.ripe.net",
	"UK":     "whois.ripe.net",
	"GB":     "whois.ripe.net",
	"NL":     "whois.ripe.net",
	"RU":     "whois.ripe.net",
	"SA":     "whois.ripe.net",
	"AE":     "whois.ripe.net",
	// ARIN: US, Canada, parts of Caribbean.
	"US":     "whois.arin.net",
	"CA":     "whois.arin.net",
	// APNIC: Asia-Pacific.
	"CN":     "whois.apnic.net",
	"JP":     "whois.apnic.net",
	"KR":     "whois.apnic.net",
	"IN":     "whois.apnic.net",
	"AU":     "whois.apnic.net",
	// AFRINIC: Africa.
	"ZA":     "whois.afrinic.net",
	"EG":     "whois.afrinic.net",
	// LACNIC: Latin America.
	"BR":     "whois.lacnic.net",
	"MX":     "whois.lacnic.net",
	"AR":     "whois.lacnic.net",
}

// WhoisConfig configures a WhoisClient.
type WhoisConfig struct {
	// Timeout caps a single query (connect + read). Default 5s.
	Timeout time.Duration

	// ServerByCC maps an ISO 3166-1 alpha-2 country code to a
	// whois server hostname. When the country is not in the map
	// the empty-key entry is used (default: whois.ripe.net).
	ServerByCC map[string]string

	// Dialer is overridable for tests (net.Pipe or a custom
	// in-process server). The default is a TCP dialer with the
	// configured Timeout.
	Dialer func(ctx context.Context, network, addr string) (net.Conn, error)
}

// WhoisClient performs port-43 whois lookups against the configured
// registry servers.
//
// The zero value is NOT usable — call NewWhoisClient.
type WhoisClient struct {
	cfg    WhoisConfig
	now    func() time.Time
}

// NewWhoisClient validates cfg and returns a usable client.
func NewWhoisClient(cfg WhoisConfig) (*WhoisClient, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.ServerByCC == nil {
		cfg.ServerByCC = defaultWhoisServers
	}
	if cfg.Dialer == nil {
		d := &net.Dialer{Timeout: cfg.Timeout}
		cfg.Dialer = d.DialContext
	}
	return &WhoisClient{cfg: cfg, now: time.Now}, nil
}

// Lookup resolves a single IP through the whois protocol.
//
// Returns:
//   - (*OperatorInfo, nil) on success
//   - (nil, ErrUnknownOperator) when the registry returned no
//     matching record (the response is a "no entries found"
//     notice — e.g. ARIN's "No match found for query")
//   - (nil, err) on any other failure (network, decode, timeout)
//
// Source is set to SourceRIPEWhois (or SourceARINWhois when the
// configured server is whois.arin.net — Sprint 3 picks servers
// from the ServerByCC map keyed on the empty string. A Sprint 4
// PR may add IP→country geolocation to switch RIRs
// automatically; that work is out of scope here).
func (c *WhoisClient) Lookup(ctx context.Context, ip netip.Addr) (*OperatorInfo, error) {
	if !ip.IsValid() {
		return nil, fmt.Errorf("whois: invalid IP: %w", ErrInvalidInput)
	}
	// Decide which server to ask. We don't have an IP→country map
	// here (that would need GeoIP), so we default to the empty-key
	// entry (RIPE) and let the caller override via WithDialer /
	// ServerByCC. For tests the dialer is fully injected.
	server := c.serverForCC("")
	resp, err := c.query(ctx, server, ip.String())
	if err != nil {
		return nil, err
	}
	parsed := parseWhoisResponse(resp)
	if parsed.netName == "" && parsed.descr == "" && parsed.country == "" {
		return nil, ErrUnknownOperator
	}
	// Use country to decide which Source enum value to record.
	src := SourceRIPEWhois
	if server == "whois.arin.net" {
		src = SourceARINWhois
	}
	qt := QueryIPv4
	if ip.Is6() && !ip.Is4In6() {
		qt = QueryIPv6
	}
	opName := parsed.netName
	if opName == "" {
		opName = parsed.descr
	}
	info := &OperatorInfo{
		QueryType:    qt,
		QueryValue:   "", // masked by adapter
		Operator:     firstNonEmpty(parsed.netName, parsed.descr, parsed.originAS),
		OperatorName: opName,
		Country:      parsed.country,
		Source:       src,
		Confidence:   0.90,
		Timestamp:    c.now().UTC(),
	}
	return info, nil
}

// serverForCC picks a server from the map; empty key is the
// default fallback.
func (c *WhoisClient) serverForCC(cc string) string {
	if s, ok := c.cfg.ServerByCC[cc]; ok {
		return s
	}
	if s, ok := c.cfg.ServerByCC[""]; ok {
		return s
	}
	return "whois.ripe.net"
}

// query opens a TCP connection, sends "<query>\r\n", reads the
// response (until EOF or timeout) and returns the bytes.
func (c *WhoisClient) query(ctx context.Context, server, query string) (string, error) {
	dctx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	conn, err := c.cfg.Dialer(dctx, "tcp", net.JoinHostPort(server, "43"))
	if err != nil {
		return "", fmt.Errorf("whois: dial %s: %w", server, err)
	}
	defer conn.Close()
	// Set deadline on the connection itself (covers reads that
	// race with context cancellation).
	if dl, ok := dctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	if _, err := conn.Write([]byte(query + "\r\n")); err != nil {
		return "", fmt.Errorf("whois: write: %w", err)
	}
	var sb strings.Builder
	br := bufio.NewReader(conn)
	for {
		line, err := br.ReadString('\n')
		if line != "" {
			sb.WriteString(line)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return sb.String(), fmt.Errorf("whois: read: %w", err)
		}
	}
	return sb.String(), nil
}

// whoisParsed holds the subset of whois fields we care about.
type whoisParsed struct {
	netName string
	descr   string
	country string
	originAS string
}

// parseWhoisResponse extracts the standard fields we use. Each
// RIR uses slightly different field names ("netname" vs "NetName"
// vs "name"), so we match case-insensitively and accept several
// variants.
//
// Only fields relevant to operator identification are extracted.
// Registrant / admin / abuse contacts are deliberately ignored.
func parseWhoisResponse(body string) whoisParsed {
	var p whoisParsed
	for _, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%") || strings.HasPrefix(line, "#") {
			continue
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(line[:colon]))
		val := strings.TrimSpace(line[colon+1:])
		if val == "" {
			continue
		}
		switch key {
		case "netname", "name", "netname-arin", "netname-ripencc":
			p.netName = firstNonEmpty(p.netName, val)
		case "descr", "description", "owner", "organization", "org-name", "organisation":
			p.descr = firstNonEmpty(p.descr, val)
		case "country", "country-code":
			p.country = strings.ToUpper(val)
		case "origin", "origin-as", "source-as":
			// Strip the leading "AS" prefix if present.
			num := strings.TrimPrefix(strings.ToUpper(val), "AS")
			if _, err := strconv.ParseUint(num, 10, 32); err == nil {
				p.originAS = "AS" + num
			} else {
				p.originAS = val
			}
		}
	}
	return p
}