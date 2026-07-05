package api

// sessions_test.go — POST/GET /api/v1/sessions,
// GET /api/v1/sessions/{id}.
//
// Covers:
//   - POST creates a session and stores it (with sender_hash)
//   - POST schema-rejects missing required fields
//   - POST schema-rejects unknown fields
//   - POST with a public_key upserts the device (best-effort)
//   - POST without a public_key does NOT touch the device table
//   - GET /sessions/{id} returns the session
//   - GET /sessions/{id} 404s for unknown ids
//   - GET /sessions/{id} 400s for malformed ids
//   - GET /sessions (list) returns the most recent sessions
//   - GET response does NOT include the receiver hash (privacy)

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validSessionBody() map[string]any {
	return map[string]any{
		"device_id_hash": "abcdef0123456789abcdef0123456789",
		"mode":           "echobot",
		"task_type":      "whatsapp_text",
		"test_text":      "opene2ee-marker-12345",
	}
}

func TestSessions_CreateHappyPath(t *testing.T) {
	ta := newTestAPI(t)
	body, _ := json.Marshal(validSessionBody())
	w := do(t, ta.Handler(), "POST", "/api/v1/sessions", withAPIHeaders(nil), string(body))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	// Storage has exactly one session.
	require.Len(t, ta.Store.Sessions, 1)
	for _, s := range ta.Store.Sessions {
		assert.Equal(t, "echobot", s.Mode)
		assert.Equal(t, "whatsapp_text", s.TaskType)
		assert.Equal(t, "pending", s.Status)
		require.NotNil(t, s.SenderHash)
		assert.Equal(t, "abcdef0123456789abcdef0123456789", *s.SenderHash)
	}

	var resp sessionResponse
	readJSON(t, w.Body, &resp)
	assert.NotEqual(t, uuid.Nil, resp.ID)
	assert.Equal(t, "pending", resp.Status)
}

func TestSessions_CreateSchemaRejectsMissingMode(t *testing.T) {
	ta := newTestAPI(t)
	body := validSessionBody()
	delete(body, "mode")
	raw, _ := json.Marshal(body)
	w := do(t, ta.Handler(), "POST", "/api/v1/sessions", withAPIHeaders(nil), string(raw))
	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, ta.Store.Sessions)
}

func TestSessions_CreateSchemaRejectsInvalidMode(t *testing.T) {
	ta := newTestAPI(t)
	body := validSessionBody()
	body["mode"] = "bogus_mode"
	raw, _ := json.Marshal(body)
	w := do(t, ta.Handler(), "POST", "/api/v1/sessions", withAPIHeaders(nil), string(raw))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSessions_CreateSchemaRejectsInvalidTaskType(t *testing.T) {
	ta := newTestAPI(t)
	body := validSessionBody()
	body["task_type"] = "carrier_pigeon"
	raw, _ := json.Marshal(body)
	w := do(t, ta.Handler(), "POST", "/api/v1/sessions", withAPIHeaders(nil), string(raw))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSessions_CreateSchemaRejectsAdditionalFields(t *testing.T) {
	ta := newTestAPI(t)
	body := validSessionBody()
	body["phone_number"] = "+905321234567" // privacy: must reject
	raw, _ := json.Marshal(body)
	w := do(t, ta.Handler(), "POST", "/api/v1/sessions", withAPIHeaders(nil), string(raw))
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSessions_CreateWithPublicKeyUpsertsDevice(t *testing.T) {
	ta := newTestAPI(t)
	body := validSessionBody()
	body["public_key"] = []byte("0123456789abcdef0123456789abcdef") // 32-byte key (dummy)
	body["public_key_fp"] = "abcdef0123456789abcdef0123456789"
	raw, _ := json.Marshal(body)
	w := do(t, ta.Handler(), "POST", "/api/v1/sessions", withAPIHeaders(nil), string(raw))
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	require.Contains(t, ta.Store.Devices, "abcdef0123456789abcdef0123456789")
}

func TestSessions_CreateWithoutPublicKeyDoesNotUpsertDevice(t *testing.T) {
	ta := newTestAPI(t)
	body, _ := json.Marshal(validSessionBody())
	w := do(t, ta.Handler(), "POST", "/api/v1/sessions", withAPIHeaders(nil), string(body))
	require.Equal(t, http.StatusCreated, w.Code)
	assert.Empty(t, ta.Store.Devices)
}

func TestSessions_GetByID(t *testing.T) {
	ta := newTestAPI(t)
	// Create a session first.
	body, _ := json.Marshal(validSessionBody())
	w := do(t, ta.Handler(), "POST", "/api/v1/sessions", withAPIHeaders(nil), string(body))
	require.Equal(t, http.StatusCreated, w.Code)
	var created sessionResponse
	readJSON(t, w.Body, &created)

	// GET /sessions/{id}
	w = do(t, ta.Handler(), "GET", fmt.Sprintf("/api/v1/sessions/%s", created.ID.String()), withAPIHeaders(nil), "")
	require.Equal(t, http.StatusOK, w.Code)
	var got sessionResponse
	readJSON(t, w.Body, &got)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, "echobot", got.Mode)
	// Privacy: GET response MUST NOT include sender_device_id_hash
	// — the device knows its own hash; we save bandwidth.
	assert.Empty(t, got.DeviceIDHash)
	// Privacy: GET response MUST NOT include receiver_device_id_hash.
	assert.Empty(t, got.ReceiverDeviceIDHash)
}

func TestSessions_GetByID_NotFound(t *testing.T) {
	ta := newTestAPI(t)
	w := do(t, ta.Handler(), "GET", fmt.Sprintf("/api/v1/sessions/%s", uuid.New().String()), withAPIHeaders(nil), "")
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestSessions_GetByID_InvalidID(t *testing.T) {
	ta := newTestAPI(t)
	w := do(t, ta.Handler(), "GET", "/api/v1/sessions/not-a-uuid", withAPIHeaders(nil), "")
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSessions_ListReturns(t *testing.T) {
	ta := newTestAPI(t)
	for i := 0; i < 3; i++ {
		body, _ := json.Marshal(validSessionBody())
		w := do(t, ta.Handler(), "POST", "/api/v1/sessions", withAPIHeaders(nil), string(body))
		require.Equal(t, http.StatusCreated, w.Code)
	}
	w := do(t, ta.Handler(), "GET", "/api/v1/sessions", withAPIHeaders(nil), "")
	require.Equal(t, http.StatusOK, w.Code)
	var list listSessionsResponse
	readJSON(t, w.Body, &list)
	assert.Equal(t, 3, list.Count)
	assert.Len(t, list.Sessions, 3)
}

func TestSessions_ListLimit(t *testing.T) {
	ta := newTestAPI(t)
	for i := 0; i < 5; i++ {
		body, _ := json.Marshal(validSessionBody())
		w := do(t, ta.Handler(), "POST", "/api/v1/sessions", withAPIHeaders(nil), string(body))
		require.Equal(t, http.StatusCreated, w.Code)
	}
	w := do(t, ta.Handler(), "GET", "/api/v1/sessions?limit=2", withAPIHeaders(nil), "")
	require.Equal(t, http.StatusOK, w.Code)
	var list listSessionsResponse
	readJSON(t, w.Body, &list)
	assert.Equal(t, 2, list.Count)
	assert.Len(t, list.Sessions, 2)
}

func TestSessions_StorageErrorOnCreateReturns500(t *testing.T) {
	ta := newTestAPI(t)
	ta.Store.InsertSessionErr = fmt.Errorf("db down")
	body, _ := json.Marshal(validSessionBody())
	w := do(t, ta.Handler(), "POST", "/api/v1/sessions", withAPIHeaders(nil), string(body))
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSessions_PrivacyNoLogOfTestText(t *testing.T) {
	ta := newTestAPI(t)
	body := validSessionBody()
	body["test_text"] = "UNIQUE-MARKER-98765"
	raw, _ := json.Marshal(body)
	w := do(t, ta.Handler(), "POST", "/api/v1/sessions", withAPIHeaders(nil), string(raw))
	require.Equal(t, http.StatusCreated, w.Code)

	for _, e := range ta.Logger.Entries {
		for _, v := range e.Args {
			s, ok := v.(string)
			if !ok {
				continue
			}
			assert.NotEqual(t, "UNIQUE-MARKER-98765", s,
				"test_text must not appear in any log arg (it could be a replay token)")
		}
	}
}