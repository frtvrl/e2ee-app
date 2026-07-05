// Package operator provides the "Operator Tespit Servisi" (Operator
// Detection Service) for OpenE2EE.
//
// Per HANDOFF.md §4 PR-3, the REST layer (PR-7) needs a way to answer
// "which carrier does this phone number / IP belong to?" so that
// telemetry rows can be tagged with an `operator` enum value (one of
// the values in `shared/schemas/telemetry.schema.json`).
//
// This package is intentionally small and dependency-light:
//
//   - OperatorLookup is the single interface the REST handler depends on.
//   - Two adapters implement it: MNPTRAdapter (TR MNP / BTK, offline stub
//     in MVP) and IPReverseAdapter (RIPE/ARIN whois + local ASN table).
//   - A Cache (NoopCache or RedisCache) sits in front of the adapters to
//     avoid hitting the upstream APIs more than once per (phone, ip).
//   - Service orchestrates cache → adapter → cache-write with a 24h TTL
//     per HANDOFF §4 PR-3 ("Redis cache TTL=24h").
//
// SCOPE / NON-GOALS (Sprint 1):
//   - No live HTTP calls. The BTK MNP endpoint is not publicly
//     accessible (HANDOFF §9); RIPE/ARIN whois is wired symbolically
//     in this PR and called for real in Sprint 2. Both adapters return
//     data from a small in-process table — the interface is stable so
//     swapping in a real client is a one-line change.
//   - The cache key is "prefix + sha256(salt || query)" so that even a
//     breached cache can't be reversed to the query value (e.g. the
//     phone number) without the server-side salt.
//   - No goroutines, no background workers — every method is
//     context-aware and synchronous. The cache itself is goroutine-safe
//     (Redis is, and NoopCache is trivially so).
//
// PRIVACY (RISKS §F12): the E.164 phone number is hashed before being
// stored as a cache key, so cache dumps are not enough to recover the
// underlying numbers. The IP address is treated similarly.
package operator

import (
	"context"
	"errors"
	"time"
)

// QueryType identifies how the lookup was triggered. Mirrors
// shared/schemas/operator-lookup.schema.json `query_type` enum.
type QueryType string

const (
	QueryPhoneE164    QueryType = "phone_e164"
	QueryPhoneNational QueryType = "phone_national"
	QueryIPv4         QueryType = "ip_v4"
	QueryIPv6         QueryType = "ip_v6"
	QueryASN          QueryType = "asn"
)

// Source identifies which backend produced an OperatorInfo.
// Mirrors shared/schemas/operator-lookup.schema.json `source` enum.
type Source string

const (
	SourceTRMNPAPI     Source = "tr_mnp_api"
	SourceRIPEWhois    Source = "ripe_whois"
	SourceARINWhois    Source = "arin_whois"
	SourceASNDB        Source = "asn_db"
	SourceFallbackUnknown Source = "fallback_unknown"
)

// OperatorInfo is the in-memory representation of one resolved lookup.
// JSON tags mirror shared/schemas/operator-lookup.schema.json field
// names so it can be marshalled directly to a REST response.
//
// Confidence is a [0,1] number; 0 = no information, 1 = authoritative
// match. "Unknown" responses carry 0.0 confidence; the tr_mnp_api stub
// reports 0.95 (it's a static table, not a real MNP query); the ASN
// table reports 0.80 (ranges are coarse).
type OperatorInfo struct {
	QueryType     QueryType `json:"query_type"`
	QueryValue    string    `json:"query_value"`
	Operator      string    `json:"operator,omitempty"`
	OperatorName  string    `json:"operator_name,omitempty"`
	Country       string    `json:"country,omitempty"`
	MCC           string    `json:"mcc,omitempty"`
	MNC           string    `json:"mnc,omitempty"`
	Source        Source    `json:"source"`
	Confidence    float64   `json:"confidence"`
	Timestamp     time.Time `json:"timestamp"`
	CacheTTLSecs  int       `json:"cache_ttl_seconds,omitempty"`
}

// Sentinel errors. Callers use errors.Is for matching.
var (
	// ErrInvalidInput is returned when a phone or IP argument fails
	// syntactic validation (empty, wrong prefix, bad length, etc.).
	ErrInvalidInput = errors.New("operator: invalid input")

	// ErrUnknownOperator is returned when no adapter can resolve the
	// query to a known operator. Service swallows this and returns
	// an "unknown" OperatorInfo instead — adapters can return it to
	// signal "we're sure there's no answer", vs. an internal error.
	ErrUnknownOperator = errors.New("operator: unknown operator")
)

// DefaultCacheTTL is the cache TTL for resolved lookups.
// Per HANDOFF §4 PR-3: "Redis cache TTL=24h".
const DefaultCacheTTL = 24 * time.Hour

// MaxE164Length is the maximum total length of an E.164 phone number
// (including the leading "+"). Per ITU-T E.164: max 15 digits, plus
// the "+" = 16 chars.
const MaxE164Length = 16

// MinE164Length is the shortest valid E.164 number. Country code "1"
// + a single subscriber digit + "+" = 3 chars.
const MinE164Length = 3

// OperatorLookup is the single dependency the REST layer (PR-7) has
// on this package. The two production adapters — MNPTRAdapter and
// IPReverseAdapter — implement it; the orchestration layer (Service)
// also implements it so callers can use the cache-fronted version.
type OperatorLookup interface {
	// LookupByPhone resolves an E.164 phone number to an OperatorInfo.
	// Implementations may return ErrInvalidInput for bad input and
	// ErrUnknownOperator when the number is in a country the adapter
	// has no data for.
	LookupByPhone(ctx context.Context, e164 string) (*OperatorInfo, error)

	// LookupByIP resolves an IPv4 or IPv6 string to an OperatorInfo.
	// Same error contract as LookupByPhone.
	LookupByIP(ctx context.Context, ip string) (*OperatorInfo, error)
}
