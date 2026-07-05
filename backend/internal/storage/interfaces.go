// Package storage provides persistent storage adapters for OpenE2EE backend.
//
// It defines two cohesive interfaces:
//
//   - Store      — relational storage (PostgreSQL + TimescaleDB) for devices,
//                  sessions, and time-series telemetry.
//   - ReceiverPool — key/value storage (Redis) for the P2P Active Pool (set
//                    of "nöbet" receivers waiting to be matched).
//
// ADR references:
//   - ADR-0005 §JSON Şemaları (telemetry / session / operator-lookup shapes)
//   - ADR-0006 §Anonim Cihaz Kimliği (device_id_hash, public_key_fp,
//                  masked IP, no payload storage)
package storage

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when a row is missing.
var ErrNotFound = errors.New("storage: not found")

// Session represents an E2EE transparency test session.
//
// Fields correspond to `shared/schemas/session.schema.json` (ADR-0005).
type Session struct {
	ID           uuid.UUID  `json:"id"`
	Mode         string     `json:"mode"` // "p2p" | "echobot" | "single"
	TaskType     string     `json:"task_type"`
	SenderHash   *string    `json:"sender_hash,omitempty"`
	ReceiverHash *string    `json:"receiver_hash,omitempty"`
	Status       string     `json:"status"` // "pending" | "active" | "completed" | "incomplete"
	StartedAt    time.Time  `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
}

// Telemetry represents an anonymized telemetry sample.
//
// Fields map 1:1 to `shared/schemas/telemetry.schema.json` (ADR-0005 §Örnek).
//
// PRIVACY (ADR-0006): DeviceIDHash is the server-side hash, NEVER the raw
// UUID v7. PublicKeyFP is the SHA-256 fingerprint, NEVER the raw public key
// (the public key itself lives in the `devices` table; private keys never
// leave the device). IPSubnet is masked (/24 IPv4, /48 IPv6). Payload bytes
// are never stored — only the derived score (Entropy) and TLS fingerprint.
type Telemetry struct {
	DeviceIDHash string     `json:"device_id_hash"`
	PublicKeyFP  string     `json:"public_key_fp"`
	Operator     string     `json:"operator"`
	App          string     `json:"app"`
	TLSFP        string     `json:"tls_fp"`
	Entropy      float64    `json:"entropy"`
	SessionID    *uuid.UUID `json:"session_id,omitempty"`
	IPSubnet     string     `json:"ip_subnet,omitempty"`
	Timestamp    time.Time  `json:"timestamp"`
}

// Store is the persistent (relational) storage interface.
type Store interface {
	// Close releases the underlying connection pool.
	Close()

	// Migrate applies the schema (idempotent). Must be called once at startup.
	// Includes devices, sessions, telemetry tables + TimescaleDB hypertable
	// (call EnsureTimescale separately, since it requires the extension).
	Migrate(ctx context.Context) error

	// EnsureTimescale creates the TimescaleDB hypertable on `telemetry` and
	// installs the retention policy. Failing gracefully if the extension is
	// not installed — the table itself still works as a regular one.
	EnsureTimescale(ctx context.Context) error

	// Devices.
	UpsertDevice(ctx context.Context, hash string, publicKey []byte, fp string) error

	// Telemetry.
	InsertTelemetry(ctx context.Context, t Telemetry) error

	// Sessions.
	InsertSession(ctx context.Context, s Session) error
	UpdateSessionStatus(ctx context.Context, id uuid.UUID, status string, endedAt *time.Time) error
	GetSession(ctx context.Context, id uuid.UUID) (*Session, error)
	ListSessions(ctx context.Context, limit int) ([]Session, error)

	// KVKK / GDPR: hard-delete all data belonging to a device.
	// Per RISKS.md E3 + BRD §8 FR-7: 7-day SLA for user-initiated delete.
	DeleteUser(ctx context.Context, deviceIDHash string) error
}

// ReceiverPool is the Active Pool (P2P receivers waiting to be matched).
// Implementations must use TTL-based expiry so crashed clients auto-cleanup.
type ReceiverPool interface {
	// Add registers a device hash as an active receiver for the given TTL.
	Add(ctx context.Context, deviceHash string, ttl time.Duration) error

	// PopMatching atomically removes and returns one receiver (any, FIFO-ish).
	// Returns ErrNotFound if the pool is empty.
	PopMatching(ctx context.Context) (string, error)

	// Count returns the current pool size (for /healthz + debugging).
	Count(ctx context.Context) (int64, error)

	// Close releases the underlying client.
	Close() error
}

// DefaultRetentionInterval is the TimescaleDB retention for telemetry.
// Per BRD §E2 (RISKS.md E2): 90 days hot, 1 year cold aggregate — Phase 1
// implements only the hot window.
const DefaultRetentionInterval = "90 days"

// DefaultPoolTTL is how long a receiver stays in the active pool without
// re-registration. Per ADR-0004 §1 "Nöbet 15 dk" → use slightly longer to
// absorb network jitter.
const DefaultPoolTTL = 15 * time.Minute
