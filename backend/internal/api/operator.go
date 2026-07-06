package api

// operator.go — GET /api/v1/operator/lookup.
//
// Two query parameters:
//
//	qtype  — "phone_e164" | "phone_national" | "ip_v4" | "ip_v6"
//	q      — the E.164 number or IP string
//
// Example:
//
//	GET /api/v1/operator/lookup?qtype=phone_e164&q=%2B905321234567
//	GET /api/v1/operator/lookup?qtype=ip_v4&q=88.236.78.12
//
// Response shape matches shared/schemas/operator-lookup.schema.json
// (operator.Service already returns OperatorInfo with the right
// JSON tags; we forward the struct directly).
//
// PRIVACY (RISKS §F12):
//   - The query VALUE (phone or IP) is NEVER logged. The
//     access-log middleware emits only the route template
//     (/api/v1/operator/lookup) so the value never lands in
//     the log even by accident.
//   - The response carries the QUERY VALUE back (the client
//     sent it, after all). That is the ONLY PII in the
//     response. The response body itself is not logged.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/opene2ee-com/e2ee-app/backend/internal/operator"
)

// queryTypeFromString maps the API's qtype string to the
// operator package's QueryType enum. Unknown values are
// rejected at parse time so the Service layer never sees a
// bad input.
func queryTypeFromString(s string) (operator.QueryType, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "phone_e164":
		return operator.QueryPhoneE164, nil
	case "phone_national":
		return operator.QueryPhoneNational, nil
	case "ip_v4":
		return operator.QueryIPv4, nil
	case "ip_v6":
		return operator.QueryIPv6, nil
	case "asn":
		return operator.QueryASN, nil
	default:
		return "", errors.New("qtype must be one of: phone_e164, phone_national, ip_v4, ip_v6, asn")
	}
}

// handleOperatorLookup is GET /api/v1/operator/lookup.
func (a *API) handleOperatorLookup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		qtStr := q.Get("qtype")
		val := q.Get("q")
		if qtStr == "" || val == "" {
			writeBadRequest(w, "qtype and q query parameters are required")
			return
		}
		qt, err := queryTypeFromString(qtStr)
		if err != nil {
			writeBadRequest(w, err.Error())
			return
		}
		// Defensive cap on the query string length. The
		// underlying Service applies its own syntax checks
		// (E.164 / IP regex); this is just a safety net so
		// a pathological 1 MB q= doesn't waste cycles.
		if len(val) > 64 {
			writeBadRequest(w, "q value too long (max 64 chars)")
			return
		}
		var (
			info *operator.OperatorInfo
		)
		switch qt {
		case operator.QueryPhoneE164, operator.QueryPhoneNational:
			info, err = a.deps.Cfg.Operator.LookupByPhone(r.Context(), val)
		case operator.QueryIPv4, operator.QueryIPv6:
			info, err = a.deps.Cfg.Operator.LookupByIP(r.Context(), val)
		case operator.QueryASN:
			// operator.Service doesn't expose ASN lookup yet;
			// reject with a stable code so the client knows
			// the feature is planned but not in MVP.
			writeError(w, http.StatusNotImplemented, ErrorBody{
				Code:    CodeNotImplemented,
				Message: "ASN lookup is not available in this build.",
			})
			return
		}
		if err != nil {
			// Service returns "unknown" with 200 for
			// no-match cases; the only error path here is
			// input validation (the operator package's
			// ErrInvalidInput). Anything else is a 5xx.
			if errors.Is(err, operator.ErrInvalidInput) {
				writeBadRequest(w, "Invalid query value.")
				return
			}
			a.deps.Cfg.Logger.Error("operator lookup failed",
				"err_kind", "operator",
				"qtype", qtStr,
			)
			writeInternal(w)
			return
		}
		writeJSON(w, http.StatusOK, info)
	}
}