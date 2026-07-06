// mnp_tr.go — TR MNP (Mobile Number Portability) adapter for the
// Operator Tespit Servisi.
//
// Per HANDOFF §4 PR-3 + §9, the BTK MNP API is NOT publicly
// accessible in the MVP. This file is the offline stub: it ships a
// small, opinionated table of TR mobile prefixes → operator and
// resolves an E.164 number against it.
//
// SCOPE:
//   - Country code: +90 only (TR). Numbers from other countries
//     return ErrUnknownOperator — they're not a BTK MNP concern.
//   - Prefix-based: the 3-digit prefix after +90 is the "operator
//     code" historically assigned by BTK. This is a STATIC mapping
//     (it does NOT reflect a real-time MNP port). A future PR will
//     swap in a real client (HTTP GET to BTK, or to a paid third-
//     party MNP provider) without changing the OperatorLookup
//     interface or the rest of the package.
//   - Confidence 0.95: we know the operator the number was ORIGINALLY
//     allocated to, but a real MNP check could disagree. Real API
//     will bump this to 1.0.
//
// POST-MVP (Sprint 2+):
//   - Replace the static table with an HTTP client.
//   - Optionally cache the response at the BTK level too (currently
//     the in-package Cache layer handles dedup).
//   - Add retries / circuit-breaker around the BTK call.
package operator

import (
	"context"
	"fmt"
	"time"
)

// trPrefix is one row in the offline MNP table. TR mobile numbers
// are 10 digits long (+90 XXX XXX XXXX) — the first 3 of those
// digits are the operator code.
type trPrefix struct {
	Prefix      string // e.g. "532"
	MNC         string // e.g. "01" — Mobile Network Code
	Operator    string // enum string, matches telemetry schema
	OperatorName string // human-readable
}

// trMNPTable is the offline TR MNP stub. Coverage is a curated subset
// of BTK's current allocations — enough to make the test vectors
// stable, NOT a complete database. Add rows here as you add tests
// or as the in-package UseCase expands.
//
// Sources used (manual, not auto-fetched):
//   - BTK "Mobil İşletmeciler" allocations 2024
//   - turkcell.com.tr / vodafone.com.tr / turktelekom.com.tr official
//     "number range" pages
var trMNPTable = []trPrefix{
	// Turkcell
	{Prefix: "532", MNC: "01", Operator: "turkcell", OperatorName: "Turkcell"},
	{Prefix: "533", MNC: "01", Operator: "turkcell", OperatorName: "Turkcell"},
	{Prefix: "534", MNC: "01", Operator: "turkcell", OperatorName: "Turkcell"},
	{Prefix: "535", MNC: "01", Operator: "turkcell", OperatorName: "Turkcell"},
	{Prefix: "536", MNC: "01", Operator: "turkcell", OperatorName: "Turkcell"},
	{Prefix: "537", MNC: "01", Operator: "turkcell", OperatorName: "Turkcell"},
	{Prefix: "538", MNC: "01", Operator: "turkcell", OperatorName: "Turkcell"},
	{Prefix: "539", MNC: "01", Operator: "turkcell", OperatorName: "Turkcell"},

	// Vodafone TR
	{Prefix: "540", MNC: "02", Operator: "vodafone_tr", OperatorName: "Vodafone TR"},
	{Prefix: "541", MNC: "02", Operator: "vodafone_tr", OperatorName: "Vodafone TR"},
	{Prefix: "542", MNC: "02", Operator: "vodafone_tr", OperatorName: "Vodafone TR"},
	{Prefix: "543", MNC: "02", Operator: "vodafone_tr", OperatorName: "Vodafone TR"},
	{Prefix: "544", MNC: "02", Operator: "vodafone_tr", OperatorName: "Vodafone TR"},
	{Prefix: "545", MNC: "02", Operator: "vodafone_tr", OperatorName: "Vodafone TR"},
	{Prefix: "546", MNC: "02", Operator: "vodafone_tr", OperatorName: "Vodafone TR"},
	{Prefix: "547", MNC: "02", Operator: "vodafone_tr", OperatorName: "Vodafone TR"},
	{Prefix: "548", MNC: "02", Operator: "vodafone_tr", OperatorName: "Vodafone TR"},
	{Prefix: "549", MNC: "02", Operator: "vodafone_tr", OperatorName: "Vodafone TR"},

	// Turk Telekom (AVEA / TT Mobil)
	{Prefix: "500", MNC: "03", Operator: "turk_telekom", OperatorName: "Turk Telekom"},
	{Prefix: "501", MNC: "03", Operator: "turk_telekom", OperatorName: "Turk Telekom"},
	{Prefix: "505", MNC: "03", Operator: "turk_telekom", OperatorName: "Turk Telekom"},
	{Prefix: "506", MNC: "03", Operator: "turk_telekom", OperatorName: "Turk Telekom"},
	{Prefix: "507", MNC: "03", Operator: "turk_telekom", OperatorName: "Turk Telekom"},
	{Prefix: "530", MNC: "03", Operator: "turk_telekom", OperatorName: "Turk Telekom"},
	{Prefix: "531", MNC: "03", Operator: "turk_telekom", OperatorName: "Turk Telekom"},
	{Prefix: "550", MNC: "03", Operator: "turk_telekom", OperatorName: "Turk Telekom"},
	{Prefix: "551", MNC: "03", Operator: "turk_telekom", OperatorName: "Turk Telekom"},
}

// trMCC is the Mobile Country Code for Turkey (286). Single constant
// — every row in trMNPTable shares it.
const trMCC = "286"

// trCountryISO is the ISO 3166-1 alpha-2 code for Turkey.
const trCountryISO = "TR"

// MNPTRAdapter implements OperatorLookup for TR mobile numbers.
//
// All fields are read-only after construction; the zero value is NOT
// usable — call NewMNPTRAdapter.
type MNPTRAdapter struct {
	// now is overridable in tests for deterministic timestamps.
	now func() time.Time
}

// NewMNPTRAdapter returns a ready-to-use adapter with the offline
// table. There is no configuration — the table is hardcoded in this
// file by design. To extend coverage, add a row to trMNPTable.
func NewMNPTRAdapter() *MNPTRAdapter {
	return &MNPTRAdapter{now: time.Now}
}

// Compile-time interface check.
var _ OperatorLookup = (*MNPTRAdapter)(nil)

// LookupByPhone resolves an E.164 number to a TR operator. Errors:
//   - ErrInvalidInput: not E.164 or not a +90 number
//   - ErrUnknownOperator: +90 but the 3-digit prefix is not in the table
//
// PRIVACY (ADR-0006): info.QueryValue is set to the MASKED form
// (MaskPhoneE164) — never the raw E.164. The Service layer will
// not re-mask; the masked value is what lands in cache + REST.
func (a *MNPTRAdapter) LookupByPhone(_ context.Context, e164 string) (*OperatorInfo, error) {
	if !looksLikeE164(e164) {
		return nil, fmt.Errorf("phone %q: %w", e164, ErrInvalidInput)
	}
	// We only know about +90 here. Other country codes are explicitly
	// out-of-scope (a different adapter would handle them).
	const trCC = "+90"
	if len(e164) < len(trCC)+1 || e164[:len(trCC)] != trCC {
		return nil, ErrUnknownOperator
	}
	// Extract the operator code: chars after "+90", 3 digits.
	const opCodeLen = 3
	if len(e164) < len(trCC)+opCodeLen {
		return nil, fmt.Errorf("phone %q: too short after country code: %w",
			e164, ErrInvalidInput)
	}
	opCode := e164[len(trCC) : len(trCC)+opCodeLen]

	// Linear scan is fine for ~30 rows. If this ever grows past a few
	// hundred, switch to a map[string]trPrefix keyed by opCode.
	for _, row := range trMNPTable {
		if row.Prefix == opCode {
			info := &OperatorInfo{
				QueryType:    QueryPhoneE164,
				QueryValue:   "", // filled in below
				Operator:     row.Operator,
				OperatorName: row.OperatorName,
				Country:      trCountryISO,
				MCC:          trMCC,
				MNC:          row.MNC,
				Source:       SourceTRMNPAPI,
				Confidence:   0.95,
				Timestamp:    a.now().UTC(),
			}
			// Apply the privacy mask before returning. NEVER return the
			// raw E.164.
			applyPhoneMask(info, e164)
			return info, nil
		}
	}
	return nil, fmt.Errorf("phone %q (op-code %s): %w", e164, opCode, ErrUnknownOperator)
}

// LookupByIP is a no-op for the MNP adapter — it explicitly does not
// resolve IP addresses. Returns ErrUnknownOperator so the orchestrator
// (Service) can move on to the next adapter in its chain.
func (a *MNPTRAdapter) LookupByIP(_ context.Context, ip string) (*OperatorInfo, error) {
	return nil, fmt.Errorf("MNP adapter does not resolve IP %q: %w", ip, ErrUnknownOperator)
}

// MNPTRTableSize is exported for tests + observability. It lets a
// caller assert that the offline table has a minimum expected size
// without exposing the table itself.
func MNPTRTableSize() int { return len(trMNPTable) }

// Helper for tests: a trPrefix row can be looked up by raw prefix
// (not the full E.164) to assert the table shape directly.
func lookupTRPrefixByCode(code string) (trPrefix, bool) {
	for _, r := range trMNPTable {
		if r.Prefix == code {
			return r, true
		}
	}
	return trPrefix{}, false
}

// end of MNPTRAdapter
