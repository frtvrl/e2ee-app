// rdap.go — RDAP (Registration Data Access Protocol) client.
//
// RDAP is the IETF-standard HTTP-based replacement for whois,
// defined in RFCs 7480-7485. Each Regional Internet Registry
// (RIR) publishes its own RDAP endpoint; the IANA bootstrap
// file at https://rdap.org/ maps an IP block to the right
// RIR's RDAP server.
//
// Sprint 3 (PR-23) wires RDAP as the PRIMARY live source for IP
// reverse DNS, ahead of whois and ahead of the local ASN table.
//
// PRIVACY (ADR-0006 §Veri Minimizasyonu): RDAP responses may carry
// abuse / registrant emails. We DO NOT propagate those fields
// to OperatorInfo — only the operator / country / ASN fields. The
// raw IP is masked (mask.go) before being placed in cache or
// returned to the REST handler.
package operator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

// defaultRDAPBootstrap is the IANA bootstrap URL that maps CIDR
// ranges to the authoritative RDAP server for the registry that
// allocated the block. It is exposed as a var (not a const) so
// tests can redirect it to httptest.
var defaultRDAPBootstrap = "https://rdap.org/"

// RDAPConfig configures an RDAPClient. Only BootstrapURL is
// required for the default behaviour; the other fields are for
// tests and advanced deployments.
type RDAPConfig struct {
	// BootstrapURL is the IANA RDAP bootstrap URL. Defaults to
	// https://rdap.org/.
	BootstrapURL string

	// HTTPTimeout caps a single request. Default 5s.
	HTTPTimeout time.Duration

	// HTTPClient overrides the default *http.Client. Tests inject
	// one backed by httptest.Server.
	HTTPClient *http.Client

	// UserAgent sent on every request. Default "opene2ee-operator/1.0".
	UserAgent string
}

// RDAPClient performs RDAP IP-network lookups.
//
// The zero value is NOT usable — call NewRDAPClient.
type RDAPClient struct {
	cfg     RDAPConfig
	http    *http.Client
	now     func() time.Time
}

// NewRDAPClient validates cfg and returns a usable client.
// Returns an error when BootstrapURL is empty.
func NewRDAPClient(cfg RDAPConfig) (*RDAPClient, error) {
	if cfg.BootstrapURL == "" {
		cfg.BootstrapURL = defaultRDAPBootstrap
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 5 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "opene2ee-operator/1.0"
	}
	var httpClient *http.Client
	if cfg.HTTPClient != nil {
		httpClient = cfg.HTTPClient
	} else {
		httpClient = &http.Client{Timeout: cfg.HTTPTimeout}
	}
	return &RDAPClient{cfg: cfg, http: httpClient, now: time.Now}, nil
}

// rdapBootstrap is the IANA bootstrap response shape — a map
// from service tag to the list of RDAP base URLs. We only need
// the "ip" entry.
type rdapBootstrap struct {
	IP  []string `json:"ip"`
}

// rdapIPResponse is the subset of RFC 7483 we parse. Fields we
// don't use (entities, events, notices, etc.) are ignored.
type rdapIPResponse struct {
	Handle     string `json:"handle"`     // RIR-specific handle
	StartAddress string `json:"startAddress"` // RFC 7483 §5
	EndAddress   string `json:"endAddress"`
	Country     string `json:"country"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	// The "entities" array can carry abuse / registrant contacts.
	// We deliberately do NOT extract it (ADR-0006 privacy).
}

// Lookup resolves a single IP through the bootstrap → RIR chain.
//
// Returns:
//   - (*OperatorInfo, nil) on success
//   - (nil, ErrUnknownOperator) when no RIR has a record for this IP
//     (the bootstrap has no matching service entry, OR every RDAP
//     server returned 404)
//   - (nil, err) for any other error (network, decode, bad IP)
//
// Source is set to SourceRDAP.
func (c *RDAPClient) Lookup(ctx context.Context, ip netip.Addr) (*OperatorInfo, error) {
	if !ip.IsValid() {
		return nil, fmt.Errorf("rdap: invalid IP: %w", ErrInvalidInput)
	}
	// Discover the RDAP server via the bootstrap. We pass the IP
	// itself as the "prefix" — IANA returns the matching service
	// entry.
	base, err := c.bootstrapServer(ctx, ip)
	if err != nil {
		if errors.Is(err, ErrUnknownOperator) {
			return nil, ErrUnknownOperator
		}
		return nil, fmt.Errorf("rdap: bootstrap: %w", err)
	}
	url := strings.TrimRight(base, "/") + "/ip/" + ip.String()
	body, err := c.doGET(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("rdap: %s: %w", url, err)
	}
	var resp rdapIPResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("rdap: decode: %w", err)
	}
	if resp.Name == "" && resp.Handle == "" {
		// Empty answer → treat as unknown.
		return nil, ErrUnknownOperator
	}
	qt := QueryIPv4
	if ip.Is6() && !ip.Is4In6() {
		qt = QueryIPv6
	}
	info := &OperatorInfo{
		QueryType:    qt,
		QueryValue:   "", // filled by the adapter (masked)
		Operator:     firstNonEmpty(resp.Handle, resp.Name),
		OperatorName: resp.Name,
		Country:      resp.Country,
		Source:       SourceRDAP,
		Confidence:   0.95,
		Timestamp:    c.now().UTC(),
	}
	if info.OperatorName == "" {
		info.OperatorName = info.Operator
	}
	return info, nil
}

// bootstrapServer issues an HTTP GET to the IANA bootstrap URL
// with the IP as a query parameter. The response is a JSON object
// mapping service tag → list of base URLs. We always pass the IP
// and rely on the bootstrap service to return only the matching
// registries.
//
// For the public rdap.org endpoint, a simpler "rdap.org/ip/<ip>"
// redirects to the correct RIR. We use that path because it
// avoids the bootstrap-file parsing when one isn't strictly
// needed.
func (c *RDAPClient) bootstrapServer(ctx context.Context, ip netip.Addr) (string, error) {
	// Fast path: rdap.org/ip/<ip> redirects to the right RIR.
	// Follow the redirect once via a HEAD; if it succeeds, return
	// the resolved Location header.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.cfg.BootstrapURL+"ip/"+ip.String(), nil)
	if err != nil {
		return "", fmt.Errorf("rdap: bootstrap request: %w", err)
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/rdap+json, application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("rdap: bootstrap http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", ErrUnknownOperator
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("rdap: bootstrap http %d", resp.StatusCode)
	}
	// On success, the redirect target's host is the RIR we want.
	// Strip the path portion and return just the base.
	final := resp.Request.URL
	if final == nil {
		return "", errors.New("rdap: bootstrap: no final URL")
	}
	base := final.Scheme + "://" + final.Host
	// Strip any path suffix beyond "/".
	if i := strings.Index(final.Path, "/ip/"); i >= 0 {
		base += final.Path[:i]
	} else if final.Path != "" {
		base += "/"
	}
	return base, nil
}

// doGET performs an HTTP GET against url and returns the body
// bytes (capped at 1 MiB — RDAP IP responses are tiny).
func (c *RDAPClient) doGET(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/rdap+json, application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrUnknownOperator
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("rdap: http %d: %s", resp.StatusCode, string(raw))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// firstNonEmpty returns the first non-empty string among the
// arguments. Used to pick the best "operator" label from a
// RDAP response (which may have Name, Handle, or neither).
func firstNonEmpty(s ...string) string {
	for _, x := range s {
		if x != "" {
			return x
		}
	}
	return ""
}