package storage

import (
	"context"
	"fmt"
)

// This file holds the TimescaleDB-specific parts of PostgresStore.
//
// The plain schema (devices / sessions / telemetry tables) is created by
// (PostgresStore).Migrate in postgres.go. Timescale hypertables require the
// `timescaledb` extension to be installed on the target database, so they
// are a separate concern and a separate call: EnsureTimescale.
//
// If you don't want Timescale yet (dev without the extension), simply skip
// EnsureTimescale — telemetry still works as a regular Postgres table.

// hypertableSQL is kept as exported constants for tests + reuse.
const (
	hypertableCreateSQL = `SELECT create_hypertable('telemetry', 'timestamp', if_not_exists => TRUE)`
	retentionPolicySQL  = `SELECT add_retention_policy('telemetry', INTERVAL '` + DefaultRetentionInterval + `', if_not_exists => TRUE)`
)

// EnsureTimescale turns `telemetry` into a TimescaleDB hypertable and
// installs the 90-day retention policy (defined in interfaces.go as
// DefaultRetentionInterval; see BRD §10 / RISKS.md E2).
//
// Failure modes:
//   - Extension not installed: psql error 0A000 ("extension not supported")
//     bubbles up unwrapped so the caller (cmd/server PR-8) can decide whether
//     to log-and-continue or refuse to start.
//   - Already a hypertable: no-op (idempotent via if_not_exists).
func (s *PostgresStore) EnsureTimescale(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, hypertableCreateSQL); err != nil {
		return fmt.Errorf("storage: create_hypertable(telemetry): %w", err)
	}
	if _, err := s.pool.Exec(ctx, retentionPolicySQL); err != nil {
		return fmt.Errorf("storage: add_retention_policy(telemetry): %w", err)
	}
	return nil
}
