package api

// telemetry_test.go — POST /api/v1/sessions/{id}/telemetry.
//
// This is the MANDATORY schema-validation handler
// (HANDOFF §4.1 PR-7). Tests cover:
//   - happy path: valid telemetry round-trips through the
//     validator AND lands in the storage fake with all fields
//     preserved (modulo the privacy-preserving filter — never
//     accept raw UUID, raw IP, or raw public key)
//   - schema rejection: missing required fields → 400 with
//     schema_validation code and per-field details
//   - schema rejection: additional properties → 400
//   - schema rejection: out-of-range entropy → 400
//   - session id mismatch (body vs URL) → 400
//   - the privacy grep guard: no log entry contains the raw
//     payload bytes, no response body echoes them back

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validTelemetryBody returns a JSON body that passes the
// telemetry schema. Tests start from this baseline and mutate
// individual fields to exercise rejection paths.
func validTelemetryBody() map[string]any {
	return map[string]any{
		"device_id_hash":  "abcdef0123456789abcdef0123456789",     // 32 hex chars
		"public_key_fp":   "0123456789abcdef0123456789abcdef",    // 32 hex chars
		"operator":        "turkcell",
		"app":             "whatsapp",
		"tls_fp":          "deadbeef0123456789abcdef01234567",    // 32 hex
		"entropy":         7.42,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	}
}

func TestTelemetry_HappyPath(t *testing.T) {
	ta := newTestAPI(t)
	sessionID := uuid.New()
	body, _ := json.Marshal(validTelemetryBody())
	url := fmt.Sprintf("/api/v1/sessions/%s/telemetry", sessionID.String())

	w := do(t, ta.Handler(), "POST", url, withAPIHeaders(nil), string(body))
	require.Equal(t, http.StatusAccepted, w.Code, "valid payload must be accepted; body=%s", w.Body.String())

	// Storage must have received exactly one row.
	require.Len(t, ta.Store.TelemetryRows, 1)
	got := ta.Store.TelemetryRows[0]
	assert.Equal(t, "abcdef0123456789abcdef0123456789", got.DeviceIDHash)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", got.PublicKeyFP)
	assert.Equal(t, "turkcell", got.Operator)
	assert.Equal(t, "whatsapp", got.App)
	assert.Equal(t, 7.42, got.Entropy)
	require.NotNil(t, got.SessionID)
	assert.Equal(t, sessionID, *got.SessionID)

	// Response carries the row id.
	var resp telemetryResponse
	readJSON(t, w.Body, &resp)
	assert.Greater(t, resp.ID, int64(0))
	assert.True(t, resp.Accepted)
	assert.Equal(t, sessionID.String(), resp.SessionID)
}

func TestTelemetry_SchemaRejectsMissingRequired(t *testing.T) {
	ta := newTestAPI(t)
	sessionID := uuid.New()
	url := fmt.Sprintf("/api/v1/sessions/%s/telemetry", sessionID.String())

	// Drop device_id_hash.
	body := validTelemetryBody()
	delete(body, "device_id_hash")
	raw, _ := json.Marshal(body)
	w := do(t, ta.Handler(), "POST", url, withAPIHeaders(nil), string(raw))
	require.Equal(t, http.StatusBadRequest, w.Code)

	var errBody ErrorBody
	readJSON(t, w.Body, &errBody)
	assert.Equal(t, CodeSchemaValidation, errBody.Code)
	require.NotEmpty(t, errBody.Details)
	foundDeviceField := false
	for _, d := range errBody.Details {
		if d.Field == "(root)" || d.Field == "/device_id_hash" {
			foundDeviceField = true
		}
	}
	assert.True(t, foundDeviceField, "validation error should mention device_id_hash field; got %+v", errBody.Details)
}

func TestTelemetry_SchemaRejectsAdditionalProperties(t *testing.T) {
	ta := newTestAPI(t)
	sessionID := uuid.New()
	url := fmt.Sprintf("/api/v1/sessions/%s/telemetry", sessionID.String())

	body := validTelemetryBody()
	// Smuggle a forbidden field through. The schema's
	// additionalProperties:false MUST reject it.
	body["phone_number"] = "+905321234567"
	body["raw_uuid"] = "00000000-0000-0000-0000-000000000000"
	raw, _ := json.Marshal(body)
	w := do(t, ta.Handler(), "POST", url, withAPIHeaders(nil), string(raw))
	require.Equal(t, http.StatusBadRequest, w.Code)
	var errBody ErrorBody
	readJSON(t, w.Body, &errBody)
	assert.Equal(t, CodeSchemaValidation, errBody.Code)

	// Storage must NOT have received the row.
	assert.Empty(t, ta.Store.TelemetryRows)
}

func TestTelemetry_SchemaRejectsEntropyOutOfRange(t *testing.T) {
	ta := newTestAPI(t)
	sessionID := uuid.New()
	url := fmt.Sprintf("/api/v1/sessions/%s/telemetry", sessionID.String())

	body := validTelemetryBody()
	body["entropy"] = 10.0 // out of [0,8] range per schema
	raw, _ := json.Marshal(body)
	w := do(t, ta.Handler(), "POST", url, withAPIHeaders(nil), string(raw))
	require.Equal(t, http.StatusBadRequest, w.Code)

	body["entropy"] = -1.0
	raw, _ = json.Marshal(body)
	w = do(t, ta.Handler(), "POST", url, withAPIHeaders(nil), string(raw))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTelemetry_SchemaRejectsInvalidOperator(t *testing.T) {
	ta := newTestAPI(t)
	sessionID := uuid.New()
	url := fmt.Sprintf("/api/v1/sessions/%s/telemetry", sessionID.String())

	body := validTelemetryBody()
	body["operator"] = "not_a_real_operator"
	raw, _ := json.Marshal(body)
	w := do(t, ta.Handler(), "POST", url, withAPIHeaders(nil), string(raw))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTelemetry_SchemaRejectsBadDeviceHashShape(t *testing.T) {
	ta := newTestAPI(t)
	sessionID := uuid.New()
	url := fmt.Sprintf("/api/v1/sessions/%s/telemetry", sessionID.String())

	body := validTelemetryBody()
	body["device_id_hash"] = "raw-uuid-00000000-0000-0000-0000-000000000000"
	raw, _ := json.Marshal(body)
	w := do(t, ta.Handler(), "POST", url, withAPIHeaders(nil), string(raw))
	require.Equal(t, http.StatusBadRequest, w.Code,
		"raw UUID-shaped device_id_hash must be rejected (privacy invariant)")
}

func TestTelemetry_SessionIDInBodyMustMatchURL(t *testing.T) {
	ta := newTestAPI(t)
	urlSession := uuid.New()
	bodySession := uuid.New() // different
	url := fmt.Sprintf("/api/v1/sessions/%s/telemetry", urlSession.String())

	body := validTelemetryBody()
	body["session_id"] = bodySession.String()
	raw, _ := json.Marshal(body)
	w := do(t, ta.Handler(), "POST", url, withAPIHeaders(nil), string(raw))
	require.Equal(t, http.StatusBadRequest, w.Code)

	// No row persisted.
	assert.Empty(t, ta.Store.TelemetryRows)
}

func TestTelemetry_RejectsInvalidSessionIDInURL(t *testing.T) {
	ta := newTestAPI(t)
	url := "/api/v1/sessions/not-a-uuid/telemetry"
	body, _ := json.Marshal(validTelemetryBody())
	w := do(t, ta.Handler(), "POST", url, withAPIHeaders(nil), string(body))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTelemetry_RejectsEmptyBody(t *testing.T) {
	ta := newTestAPI(t)
	url := fmt.Sprintf("/api/v1/sessions/%s/telemetry", uuid.New().String())
	w := do(t, ta.Handler(), "POST", url, withAPIHeaders(nil), "")
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestTelemetry_StorageErrorReturns500(t *testing.T) {
	ta := newTestAPI(t)
	ta.Store.InsertTelemetryErr = fmt.Errorf("db down")
	sessionID := uuid.New()
	url := fmt.Sprintf("/api/v1/sessions/%s/telemetry", sessionID.String())
	body, _ := json.Marshal(validTelemetryBody())
	w := do(t, ta.Handler(), "POST", url, withAPIHeaders(nil), string(body))
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestTelemetry_DoesNotEchoPayloadInResponse(t *testing.T) {
	ta := newTestAPI(t)
	sessionID := uuid.New()
	url := fmt.Sprintf("/api/v1/sessions/%s/telemetry", sessionID.String())
	body, _ := json.Marshal(validTelemetryBody())
	w := do(t, ta.Handler(), "POST", url, withAPIHeaders(nil), string(body))
	// Response body must NOT echo the request payload.
	// (The response is telemetryResponse with just id/accepted/session_id.)
	assert.NotContains(t, w.Body.String(), "turkcell")
	assert.NotContains(t, w.Body.String(), "whatsapp")
	assert.NotContains(t, w.Body.String(), "7.42")
}

func TestTelemetry_DoesNotLogPayload(t *testing.T) {
	ta := newTestAPI(t)
	sessionID := uuid.New()
	url := fmt.Sprintf("/api/v1/sessions/%s/telemetry", sessionID.String())
	body, _ := json.Marshal(validTelemetryBody())
	w := do(t, ta.Handler(), "POST", url, withAPIHeaders(nil), string(body))
	require.Equal(t, http.StatusAccepted, w.Code)
	// Grep the access log for the payload field values.
	for _, e := range ta.Logger.Entries {
		for _, v := range e.Args {
			s, ok := v.(string)
			if !ok {
				continue
			}
			assert.NotEqual(t, "turkcell", s, "operator value must not appear in any log arg")
			assert.NotEqual(t, "whatsapp", s, "app value must not appear in any log arg")
		}
	}
}