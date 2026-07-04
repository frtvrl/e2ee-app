package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements Store against PostgreSQL + TimescaleDB.
type PostgresStore struct {
	pool PgxPool
}

// PgxPool is the subset of *pgxpool.Pool we use. Declaring it as an
// interface here (rather than depending on pgxpool directly) lets test
// code inject pgxmock.PgxPoolIface without spinning up a real DB.
type PgxPool interface {
	Close()
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Compile-time checks.
var (
	_ Store         = (*PostgresStore)(nil)
	_ PgxPool       = (*pgxpool.Pool)(nil)
)

// schemaSQL creates the three core tables. Idempotent (CREATE IF NOT EXISTS).
// TimescaleDB hypertable is set up in EnsureTimescale (separate concern).
//
// The schema is split into three statements, executed as separate Exec
// calls. Bundling them into one multi-statement Exec makes pgx's extended
// protocol happy but complicates mocking (pgxmock can't easily assert
// against multi-statement SQL), so we keep them flat.
const (
	schemaDevicesSQL = `
CREATE TABLE IF NOT EXISTS devices (
    device_id_hash  TEXT PRIMARY KEY,
    public_key      BYTEA NOT NULL,
    public_key_fp   TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_devices_public_key_fp ON devices(public_key_fp);
`
	schemaSessionsSQL = `
CREATE TABLE IF NOT EXISTS sessions (
    id              UUID PRIMARY KEY,
    mode            TEXT NOT NULL CHECK (mode IN ('p2p','echobot','single')),
    task_type       TEXT NOT NULL,
    sender_hash     TEXT,
    receiver_hash   TEXT,
    status          TEXT NOT NULL CHECK (status IN ('pending','active','completed','incomplete')),
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ended_at        TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
CREATE INDEX IF NOT EXISTS idx_sessions_started_at ON sessions(started_at DESC);
`
	schemaTelemetrySQL = `
CREATE TABLE IF NOT EXISTS telemetry (
    id              BIGSERIAL,
    device_id_hash  TEXT NOT NULL,
    public_key_fp   TEXT NOT NULL,
    operator        TEXT NOT NULL,
    app             TEXT NOT NULL,
    tls_fp          TEXT NOT NULL,
    entropy         DOUBLE PRECISION NOT NULL,
    session_id      UUID,
    ip_subnet       INET,
    timestamp       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`
)

// schemaSQL is the legacy single-string concatenation kept for backward
// compat with tests that just assert table names exist. Not used in Migrate.
const schemaSQL = schemaDevicesSQL + schemaSessionsSQL + schemaTelemetrySQL

// NewPostgresStore opens a pooled connection to PostgreSQL.
//
// connString follows pgx's standard format:
//   postgres://user:pass@host:5432/dbname?sslmode=disable
func NewPostgresStore(ctx context.Context, connString string) (*PostgresStore, error) {
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("storage: parse pgxpool config: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("storage: open pgx pool: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

// Pool exposes the underlying connection pool for advanced callers (health
// pings, transactions). Returns the interface so callers can also receive
// a mock in tests.
func (s *PostgresStore) Pool() PgxPool { return s.pool }

// Close releases the connection pool.
func (s *PostgresStore) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// Migrate applies the schema. Tables are created if not exists; safe to call
// on every startup. Each table is created in its own Exec so callers can
// decide per-step error handling if needed.
func (s *PostgresStore) Migrate(ctx context.Context) error {
	for _, stmt := range []string{schemaDevicesSQL, schemaSessionsSQL, schemaTelemetrySQL} {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("storage: migrate schema: %w", err)
		}
	}
	return nil
}

// UpsertDevice inserts (or refreshes last_seen on) an anonymized device record.
//
// Inputs:
//   - hash       — server-side SHA-256(uuid_v7 + SERVER_SALT)[:16] hex.
//   - publicKey  — 32-byte Ed25519 public key (raw, will be bytea).
//   - fp         — SHA-256(public_key)[:16] hex.
func (s *PostgresStore) UpsertDevice(ctx context.Context, hash string, publicKey []byte, fp string) error {
	if hash == "" || len(publicKey) == 0 || fp == "" {
		return fmt.Errorf("storage: UpsertDevice: empty arg (hash=%q fp=%q pk_len=%d)", hash, fp, len(publicKey))
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO devices (device_id_hash, public_key, public_key_fp)
		VALUES ($1, $2, $3)
		ON CONFLICT (device_id_hash)
		DO UPDATE SET last_seen = NOW(), public_key = EXCLUDED.public_key, public_key_fp = EXCLUDED.public_key_fp
	`, hash, publicKey, fp)
	if err != nil {
		return fmt.Errorf("storage: upsert device: %w", err)
	}
	return nil
}

// InsertTelemetry appends one anonymized telemetry row.
//
// PRIVACY: This MUST be called only with already-anonymized data. The caller
// (HTTP handler in PR-7) is responsible for:
//   - hashing the device UUID with SERVER_SALT before sending;
//   - masking the IP to /24 (IPv4) or /48 (IPv6) before sending;
//   - never sending the public key (only its fingerprint).
func (s *PostgresStore) InsertTelemetry(ctx context.Context, t Telemetry) error {
	if t.DeviceIDHash == "" || t.PublicKeyFP == "" {
		return fmt.Errorf("storage: InsertTelemetry: missing required device_id_hash / public_key_fp")
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO telemetry
		    (device_id_hash, public_key_fp, operator, app, tls_fp, entropy, session_id, ip_subnet, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::inet, $9)
	`, t.DeviceIDHash, t.PublicKeyFP, t.Operator, t.App, t.TLSFP, t.Entropy, t.SessionID, t.IPSubnet, t.Timestamp); err != nil {
		return fmt.Errorf("storage: insert telemetry: %w", err)
	}
	return nil
}

// InsertSession stores a new test session row.
func (s *PostgresStore) InsertSession(ctx context.Context, sess Session) error {
	if sess.ID == uuid.Nil {
		return fmt.Errorf("storage: InsertSession: zero uuid")
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (id, mode, task_type, sender_hash, receiver_hash, status, started_at, ended_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, sess.ID, sess.Mode, sess.TaskType, sess.SenderHash, sess.ReceiverHash, sess.Status, sess.StartedAt, sess.EndedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("storage: session %s already exists: %w", sess.ID, err)
		}
		return fmt.Errorf("storage: insert session: %w", err)
	}
	return nil
}

// UpdateSessionStatus moves a session to a new state (e.g. pending → active
// → completed) and optionally sets ended_at.
func (s *PostgresStore) UpdateSessionStatus(ctx context.Context, id uuid.UUID, status string, endedAt *time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE sessions
		   SET status = $1, ended_at = COALESCE($2, ended_at)
		 WHERE id = $3
	`, status, endedAt, id)
	if err != nil {
		return fmt.Errorf("storage: update session status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("storage: session %s: %w", id, ErrNotFound)
	}
	return nil
}

// GetSession loads a session by ID.
func (s *PostgresStore) GetSession(ctx context.Context, id uuid.UUID) (*Session, error) {
	var sess Session
	err := s.pool.QueryRow(ctx, `
		SELECT id, mode, task_type, sender_hash, receiver_hash, status, started_at, ended_at
		  FROM sessions
		 WHERE id = $1
	`, id).Scan(
		&sess.ID, &sess.Mode, &sess.TaskType, &sess.SenderHash, &sess.ReceiverHash,
		&sess.Status, &sess.StartedAt, &sess.EndedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("storage: get session %s: %w", id, err)
	}
	return &sess, nil
}

// ListSessions returns the most recent sessions (newest first), capped by limit.
// limit <= 0 means default (50).
func (s *PostgresStore) ListSessions(ctx context.Context, limit int) ([]Session, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, mode, task_type, sender_hash, receiver_hash, status, started_at, ended_at
		  FROM sessions
		 ORDER BY started_at DESC
		 LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("storage: list sessions: %w", err)
	}
	defer rows.Close()

	out := make([]Session, 0, limit)
	for rows.Next() {
		var sess Session
		if err := rows.Scan(
			&sess.ID, &sess.Mode, &sess.TaskType, &sess.SenderHash, &sess.ReceiverHash,
			&sess.Status, &sess.StartedAt, &sess.EndedAt,
		); err != nil {
			return nil, fmt.Errorf("storage: scan session row: %w", err)
		}
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("storage: iterate sessions: %w", err)
	}
	return out, nil
}

// DeleteUser hard-deletes every row belonging to a device.
//
// KVKK (BRD §8 FR-7) + GDPR Art. 17 — right to erasure. The 7-day SLA is
// a process concern (a periodic background sweeper would catch edge cases);
// this method gives the synchronous user-facing path.
func (s *PostgresStore) DeleteUser(ctx context.Context, deviceIDHash string) error {
	if deviceIDHash == "" {
		return fmt.Errorf("storage: DeleteUser: empty hash")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("storage: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM telemetry WHERE device_id_hash = $1`, deviceIDHash); err != nil {
		return fmt.Errorf("storage: delete telemetry for %s: %w", deviceIDHash, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET sender_hash = NULL WHERE sender_hash = $1`, deviceIDHash); err != nil {
		return fmt.Errorf("storage: null-out sessions.sender_hash for %s: %w", deviceIDHash, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE sessions SET receiver_hash = NULL WHERE receiver_hash = $1`, deviceIDHash); err != nil {
		return fmt.Errorf("storage: null-out sessions.receiver_hash for %s: %w", deviceIDHash, err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM devices WHERE device_id_hash = $1`, deviceIDHash); err != nil {
		return fmt.Errorf("storage: delete device %s: %w", deviceIDHash, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("storage: commit delete-user tx: %w", err)
	}
	return nil
}
