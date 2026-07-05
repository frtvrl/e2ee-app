package storage

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper — every test gets a fresh mock pool.
//
// QueryMatcherRegexp (the default) lets us match SQL fragments like
// `INSERT INTO sessions` instead of forcing exact full SQL strings.
func newMockStore(t *testing.T) (*PostgresStore, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	return &PostgresStore{pool: mock}, mock
}

func TestPostgresStore_Migrate(t *testing.T) {
	s, mock := newMockStore(t)
	// Migrate runs three Exec calls — one per table.
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS devices`).
		WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS sessions`).
		WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec(`CREATE TABLE IF NOT EXISTS telemetry`).
		WillReturnResult(pgxmock.NewResult("CREATE", 0))

	require.NoError(t, s.Migrate(context.Background()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresStore_EnsureTimescale(t *testing.T) {
	s, mock := newMockStore(t)
	// SQL contains parens/comma/=> — match exactly to avoid regex quirks.
	mock, err := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(mock.Close)
	s = &PostgresStore{pool: mock}

	mock.ExpectExec(hypertableCreateSQL).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectExec(retentionPolicySQL).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	require.NoError(t, s.EnsureTimescale(context.Background()))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresStore_UpsertDevice(t *testing.T) {
	s, mock := newMockStore(t)
	pk := make([]byte, 32)
	for i := range pk {
		pk[i] = byte(i)
	}
	mock.ExpectExec(`INSERT INTO devices`).
		WithArgs("hash123", pk, "fp456").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	require.NoError(t, s.UpsertDevice(context.Background(), "hash123", pk, "fp456"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresStore_UpsertDevice_RejectsEmpty(t *testing.T) {
	s, _ := newMockStore(t)
	err := s.UpsertDevice(context.Background(), "", []byte{1, 2, 3}, "fp")
	require.Error(t, err)
}

func TestPostgresStore_InsertTelemetry(t *testing.T) {
	s, mock := newMockStore(t)
	sid := uuid.New()
	ts := time.Now().UTC()

	mock.ExpectExec(`INSERT INTO telemetry`).
		WithArgs("hash", "fp", "turkcell", "whatsapp", "tls", 7.5, &sid, "", ts).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	require.NoError(t, s.InsertTelemetry(context.Background(), Telemetry{
		DeviceIDHash: "hash",
		PublicKeyFP:  "fp",
		Operator:     "turkcell",
		App:          "whatsapp",
		TLSFP:        "tls",
		Entropy:      7.5,
		SessionID:    &sid,
		Timestamp:    ts,
	}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresStore_InsertTelemetry_RejectsMissingFields(t *testing.T) {
	s, _ := newMockStore(t)
	err := s.InsertTelemetry(context.Background(), Telemetry{
		DeviceIDHash: "", // empty — must fail
		PublicKeyFP:  "fp",
		Operator:     "x",
		App:          "y",
		TLSFP:        "z",
		Entropy:      1.0,
	})
	require.Error(t, err)
}

func TestPostgresStore_InsertSession(t *testing.T) {
	s, mock := newMockStore(t)
	sid := uuid.New()

	mock.ExpectExec(`INSERT INTO sessions`).
		WithArgs(sid, "p2p", "whatsapp",
			(*string)(nil), (*string)(nil),
			"active", pgxmock.AnyArg(), (*time.Time)(nil)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	require.NoError(t, s.InsertSession(context.Background(), Session{
		ID:       sid,
		Mode:     "p2p",
		TaskType: "whatsapp",
		Status:   "active",
	}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresStore_InsertSession_RejectsZeroUUID(t *testing.T) {
	s, _ := newMockStore(t)
	err := s.InsertSession(context.Background(), Session{ID: uuid.Nil, Mode: "p2p"})
	require.Error(t, err)
}

func TestPostgresStore_UpdateSessionStatus(t *testing.T) {
	s, mock := newMockStore(t)
	sid := uuid.New()
	now := time.Now().UTC()

	mock.ExpectExec(`UPDATE sessions`).
		WithArgs("completed", &now, sid).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	require.NoError(t, s.UpdateSessionStatus(context.Background(), sid, "completed", &now))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresStore_UpdateSessionStatus_NotFound(t *testing.T) {
	s, mock := newMockStore(t)
	sid := uuid.New()

	mock.ExpectExec(`UPDATE sessions`).
		WithArgs("completed", (*time.Time)(nil), sid).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	err := s.UpdateSessionStatus(context.Background(), sid, "completed", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresStore_GetSession(t *testing.T) {
	s, mock := newMockStore(t)
	sid := uuid.New()
	ts := time.Now().UTC()

	rows := pgxmock.NewRows([]string{
		"id", "mode", "task_type", "sender_hash", "receiver_hash",
		"status", "started_at", "ended_at",
	}).AddRow(sid, "echobot", "rcs", nil, nil, "completed", ts, nil)

	mock.ExpectQuery(`SELECT id, mode`).
		WithArgs(sid).
		WillReturnRows(rows)

	got, err := s.GetSession(context.Background(), sid)
	require.NoError(t, err)
	assert.Equal(t, sid, got.ID)
	assert.Equal(t, "echobot", got.Mode)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresStore_GetSession_NotFound(t *testing.T) {
	s, mock := newMockStore(t)
	sid := uuid.New()

	rows := pgxmock.NewRows([]string{
		"id", "mode", "task_type", "sender_hash", "receiver_hash",
		"status", "started_at", "ended_at",
	})
	mock.ExpectQuery(`SELECT id, mode`).
		WithArgs(sid).
		WillReturnRows(rows)

	got, err := s.GetSession(context.Background(), sid)
	require.Nil(t, got)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestPostgresStore_DeleteUser(t *testing.T) {
	s, mock := newMockStore(t)
	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM telemetry`).
		WithArgs("hash").
		WillReturnResult(pgxmock.NewResult("DELETE", 3))
	mock.ExpectExec(`UPDATE sessions SET sender_hash`).
		WithArgs("hash").
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`UPDATE sessions SET receiver_hash`).
		WithArgs("hash").
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	mock.ExpectExec(`DELETE FROM devices`).
		WithArgs("hash").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectCommit()

	require.NoError(t, s.DeleteUser(context.Background(), "hash"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresStore_DeleteUser_RejectsEmpty(t *testing.T) {
	s, _ := newMockStore(t)
	require.Error(t, s.DeleteUser(context.Background(), ""))
}

// Sanity: the schema constant must include all three tables verbatim.
// Regression-guard: keeps the schema audit easy.
func TestSchemaContainsAllTables(t *testing.T) {
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS devices",
		"CREATE TABLE IF NOT EXISTS sessions",
		"CREATE TABLE IF NOT EXISTS telemetry",
		"device_id_hash",
		"public_key_fp",
		"ip_subnet",
	} {
		assert.Contains(t, schemaSQL, want, "schemaSQL must define %s", want)
	}
}

func TestPostgresStore_ListSessions(t *testing.T) {
	s, mock := newMockStore(t)
	ts := time.Now().UTC()
	sid1 := uuid.New()
	sid2 := uuid.New()

	rows := pgxmock.NewRows([]string{
		"id", "mode", "task_type", "sender_hash", "receiver_hash",
		"status", "started_at", "ended_at",
	}).AddRow(sid1, "p2p", "whatsapp", nil, nil, "completed", ts, nil).
		AddRow(sid2, "echobot", "rcs", nil, nil, "active", ts, nil)

	mock.ExpectQuery(`SELECT id, mode`).
		WithArgs(10).
		WillReturnRows(rows)

	out, err := s.ListSessions(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, out, 2)
	assert.Equal(t, sid1, out[0].ID)
	assert.Equal(t, "p2p", out[0].Mode)
	assert.Equal(t, sid2, out[1].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresStore_ListSessions_DefaultsLimitTo50(t *testing.T) {
	s, mock := newMockStore(t)
	rows := pgxmock.NewRows([]string{
		"id", "mode", "task_type", "sender_hash", "receiver_hash",
		"status", "started_at", "ended_at",
	})
	mock.ExpectQuery(`SELECT id, mode`).
		WithArgs(50). // default
		WillReturnRows(rows)

	out, err := s.ListSessions(context.Background(), 0)
	require.NoError(t, err)
	assert.Empty(t, out)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresStore_Close(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	s := &PostgresStore{pool: mock}

	// Close is registered then exercised by our wrapper; verifying that the
	// call propagates through pgxmock's expectation registry is a good smoke
	// test that no double-close or nil-deref path was taken.
	mock.ExpectClose()
	s.Close()
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgresStore_Pool(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	t.Cleanup(mock.Close)

	s := &PostgresStore{pool: mock}
	got := s.Pool()
	assert.Same(t, mock, got, "Pool() must return the underlying pool")
}
