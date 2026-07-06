package api

// api.go — the API struct that holds every dependency the REST
// handlers need, and the constructor New(...) that wires them
// together.
//
// PR-7 ships the constructor + every handler. The PR-8 wire-up
// instantiates the storage / operator / matrix services and
// passes them in. Tests inject fakes (see router_test.go,
// sessions_test.go, ...).
//
// All dependencies are interfaces — the api package never
// imports storage/postgres.go or operator/redis.go directly.
// This keeps the test surface tiny and the dependency graph
// acyclic.
//
// PRIVACY (ADR-0006): the api package is the last line of
// defense before user data hits storage. See doc.go for the
// full invariant list. The grep test
// TestPackagePrivacyInvariants re-verifies the contract at
// every `go test` run.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/opene2ee-com/e2ee-app/backend/internal/matching"
	"github.com/opene2ee-com/e2ee-app/backend/internal/operator"
	"github.com/opene2ee-com/e2ee-app/backend/internal/storage"
)

// SessionWriter is the subset of storage.Store that the sessions
// handler writes through.
type SessionWriter interface {
	InsertSession(ctx context.Context, s storage.Session) error
	UpdateSessionStatus(ctx context.Context, id uuid.UUID, status string, endedAt *time.Time) error
	GetSession(ctx context.Context, id uuid.UUID) (*storage.Session, error)
	ListSessions(ctx context.Context, limit int) ([]storage.Session, error)
}

// TelemetryWriter is the subset of storage.Store used by the
// telemetry handler.
type TelemetryWriter interface {
	InsertTelemetry(ctx context.Context, t storage.Telemetry) (int64, error)
}

// UserPurger is the subset of storage.Store used by the
// DELETE /users/{device_id_hash} handler (KVKK / GDPR Art. 17).
//
// The Redis-side purge (e.g. removing the device from the
// Active Pool) is layered on by the DeleteUserHook callback in
// Config — storage doesn't know about Redis keys for the
// Active Pool, so the API layer coordinates the multi-store
// delete.
type UserPurger interface {
	DeleteUser(ctx context.Context, deviceIDHash string) error
}

// OperatorLookup is the interface the operator handler depends
// on. operator.Service satisfies it.
type OperatorLookup interface {
	LookupByPhone(ctx context.Context, e164 string) (*operator.OperatorInfo, error)
	LookupByIP(ctx context.Context, ip string) (*operator.OperatorInfo, error)
}

// MatrixQuerier is the read-only aggregation interface used by
// the matrix handler. The production implementation will be
// backed by TimescaleDB (deferred — not in PR-7). The
// MemoryMatrixQuerier in matrix.go is the dev/test fallback.
type MatrixQuerier interface {
	Aggregate(ctx context.Context, f MatrixFilter) ([]MatrixRow, error)
}

// DeviceRegistrar is used by the sessions handler when the
// client supplies a public_key + public_key_fp at session
// creation time. May be nil if the caller already registered
// out-of-band (e.g. the mobile app's first-boot flow).
type DeviceRegistrar interface {
	UpsertDevice(ctx context.Context, hash string, publicKey []byte, fp string) error
}

// Compile-time interface assertions — fail at build time if
// storage.Store drifts away from what the API layer needs.
var (
	_ SessionWriter   = (storage.Store)(nil)
	_ TelemetryWriter = (storage.Store)(nil)
	_ UserPurger      = (storage.Store)(nil)
	_ DeviceRegistrar = (storage.Store)(nil)
)

// Config bundles every dependency the API layer needs. Wire-up
// (PR-8) instantiates one Config and passes it to New.
type Config struct {
	// Logger is the slog (or compatible) logger the access-log
	// middleware writes through. Must not be nil; New will
	// substitute slog.Default() if nil.
	Logger Logger
	// Sessions is the SessionWriter backing the sessions handler.
	Sessions SessionWriter
	// Telemetry is the TelemetryWriter backing the telemetry handler.
	Telemetry TelemetryWriter
	// Users is the UserPurger backing the DELETE /users handler.
	Users UserPurger
	// Operator is the OperatorLookup backing the operator handler.
	Operator OperatorLookup
	// Matrix is the MatrixQuerier backing the matrix handler.
	Matrix MatrixQuerier
	// Devices is the optional DeviceRegistrar called when a
	// session is created with a fresh public_key. May be nil if
	// the caller already registered out-of-band.
	Devices DeviceRegistrar
	// RateLimit is the rate-limiter middleware. If nil,
	// NewRateLimiter(DefaultRateLimitConfig) is used.
	RateLimit *RateLimiter
	// CORS drives the CORS middleware. If the zero value,
	// DefaultCORSConfig is used.
	CORS CORSConfig
	// AcceptedAPIVersions is the list of X-API-Version values
	// the middleware accepts. If empty, {"1"} is used.
	AcceptedAPIVersions []string
	// MaxBodyBytes is the request-body cap. If <= 0, MaxBodyBytes
	// (64 KB) is used.
	MaxBodyBytes int64
	// DeleteUserHook is called by the DELETE /users handler
	// after the relational delete succeeds. The hook is the
	// right place to purge Redis-side state (Active Pool
	// membership, operator cache entries for this hash, etc.).
	// Failures of the hook are LOGGED but do NOT roll back the
	// relational delete — KVKK demands hard-deletion within the
	// 7-day SLA, and a transient Redis hiccup must not undo the
	// right-to-erasure the user just exercised.
	DeleteUserHook func(ctx context.Context, deviceIDHash string) error
	// WebRTC is the matching.WebRTCManagerIface powering the
	// /api/v1/webrtc/* endpoints (Sprint 3 PR-21a). If nil,
	// the four endpoints 500 with "internal_error".
	WebRTC matching.WebRTCManagerIface
}

// API is the wired handler. Hold a reference in main(), call
// Handler() to get the http.Handler for chi.Mux...Mount.
type API struct {
	cfg    Config
	deps   *routerDeps
}

// New validates the config, compiles the embedded JSON-Schemas,
// and returns a ready-to-mount API.
//
// New returns an error if any required dependency is nil
// (Sessions, Telemetry, Users, Operator, Matrix). Optional deps
// (Logger, RateLimit, CORS, Devices, DeleteUserHook) get safe
// defaults.
func New(cfg Config) (*API, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Sessions == nil {
		return nil, errors.New("api: Config.Sessions is required")
	}
	if cfg.Telemetry == nil {
		return nil, errors.New("api: Config.Telemetry is required")
	}
	if cfg.Users == nil {
		return nil, errors.New("api: Config.Users is required")
	}
	if cfg.Operator == nil {
		return nil, errors.New("api: Config.Operator is required")
	}
	if cfg.Matrix == nil {
		return nil, errors.New("api: Config.Matrix is required (use NewMemoryMatrixQuerier for dev)")
	}
	if cfg.RateLimit == nil {
		cfg.RateLimit = NewRateLimiter(DefaultRateLimitConfig)
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = MaxBodyBytes
	}
	schemas, err := loadSchemas()
	if err != nil {
		return nil, fmt.Errorf("api: load schemas: %w", err)
	}
	deps := &routerDeps{
		Cfg:     cfg,
		Schemas: schemas,
	}
	return &API{cfg: cfg, deps: deps}, nil
}

// routerDeps bundles the fields every handler closure captures.
// Splitting it from API keeps the Handler() method short and the
// handler closures easy to test in isolation.
type routerDeps struct {
	Cfg     Config
	Schemas *schemaSet
}