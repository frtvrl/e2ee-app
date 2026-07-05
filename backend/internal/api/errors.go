package api

// errors.go — uniform JSON error responses for every handler.
//
// All non-2xx responses use one of the shapes defined here. The
// mobile/web clients can switch on `code` to drive UI copy.
//
// PRIVACY: error messages are deliberately generic. We never echo
// back the offending field value (it might be a phone number),
// never include the request body, and never include the path that
// the client requested beyond a coarse category. Validation errors
// do include the field PATH (because the mobile UI needs to
// highlight the bad field) but never the field VALUE.

import (
	"encoding/json"
	"errors"
	"net/http"
)

// ErrorCode is the machine-readable discriminator in error
// responses. Add new codes here as new failure modes are wired in;
// the client switches on this string, never on the human message.
type ErrorCode string

const (
	CodeBadRequest          ErrorCode = "bad_request"             // generic 400 (malformed JSON, missing header)
	CodeMissingHeader       ErrorCode = "missing_header"          // 400 — required header absent (e.g. X-API-Version)
	CodeInvalidHeader       ErrorCode = "invalid_header"          // 400 — header present but wrong value
	CodeSchemaValidation    ErrorCode = "schema_validation"       // 400 — body failed JSON-Schema
	CodeMethodNotAllowed    ErrorCode = "method_not_allowed"      // 405
	CodeNotFound            ErrorCode = "not_found"               // 404
	CodeRateLimited         ErrorCode = "rate_limited"            // 429
	CodePayloadTooLarge     ErrorCode = "payload_too_large"       // 413
	CodeInternal            ErrorCode = "internal_error"          // 500 — never leak details
	CodeNotImplemented      ErrorCode = "not_implemented"         // 501
)

// ErrorBody is the canonical JSON error envelope.
//
// JSON shape (stable contract — clients parse this):
//
//	{
//	  "code":    "schema_validation",
//	  "message": "Request body failed validation.",
//	  "details": [ { "field": "/operator", "message": "value must be one of: ..." } ]
//	}
type ErrorBody struct {
	Code    ErrorCode         `json:"code"`
	Message string            `json:"message"`
	Details []ValidationIssue `json:"details,omitempty"`
}

// writeError serializes an ErrorBody with the given HTTP status.
// On marshal failure (which should not happen for these tiny
// structs) we fall back to a hard-coded 500 JSON string. We never
// surface the marshal error itself — that would risk logging
// arbitrary caller data.
func writeError(w http.ResponseWriter, status int, body ErrorBody) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false) // privacy: don't HTML-escape phone-number-shaped strings into &#43; form
	if err := enc.Encode(body); err != nil {
		// Last-resort fallback. Writing the raw status line is safe —
		// no caller-controlled data ends up in this string.
		http.Error(w, `{"code":"internal_error","message":"response marshal failed"}`, status)
	}
}

// writeBadRequest is the most common helper — every handler that
// rejects a malformed body funnels through here.
func writeBadRequest(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusBadRequest, ErrorBody{
		Code:    CodeBadRequest,
		Message: msg,
	})
}

// writeValidation funnels *ValidationError through to a 400 with
// the per-field details. The handler's only job is to wrap the
// underlying error from schemaSet.Validate.
func writeValidation(w http.ResponseWriter, err *ValidationError) {
	if err == nil {
		writeBadRequest(w, "invalid request")
		return
	}
	writeError(w, http.StatusBadRequest, ErrorBody{
		Code:    CodeSchemaValidation,
		Message: "Request body failed schema validation.",
		Details: err.Details,
	})
}

// writeNotFound is used by GET /sessions/{id}, DELETE /users/{hash},
// and the matrix endpoint when a filter set matches nothing.
func writeNotFound(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusNotFound, ErrorBody{
		Code:    CodeNotFound,
		Message: msg,
	})
}

// writeInternal swallows the underlying error (which might contain
// query values, IP addresses, or stack-frame paths) and emits a
// generic 500. The handler MUST log the underlying error itself
// via the access-log middleware before returning.
func writeInternal(w http.ResponseWriter) {
	writeError(w, http.StatusInternalServerError, ErrorBody{
		Code:    CodeInternal,
		Message: "Internal server error.",
	})
}

// writeRateLimited is the 429 response. The Retry-After header
// carries the seconds-until-the-bucket-refills value so well-
// behaved clients can back off correctly.
func writeRateLimited(w http.ResponseWriter, retryAfterSeconds int) {
	if retryAfterSeconds < 1 {
		retryAfterSeconds = 1
	}
	w.Header().Set("Retry-After", itoa(retryAfterSeconds))
	writeError(w, http.StatusTooManyRequests, ErrorBody{
		Code:    CodeRateLimited,
		Message: "Rate limit exceeded.",
	})
}

// itoa is a stdlib-free integer-to-string for the Retry-After
// header. Avoids pulling in strconv for one call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// isValidationError lets handlers route errors through the right
// write helper without an errors.As per call site. It's the
// canonical "what kind of API error is this" predicate.
func isValidationError(err error) (*ValidationError, bool) {
	if err == nil {
		return nil, false
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return nil, false
}