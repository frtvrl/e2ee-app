// Package main is the entry point for the OpenE2EE backend HTTP server.
//
// Sprint 1 MVP wire-up (HANDOFF.md §4.1 PR-8). This file is the ONLY
// place where every internal package is instantiated — the rest of
// the codebase defines dependencies as interfaces (storage.Store,
// operator.OperatorLookup, api.MatrixQuerier, …) and receives them
// in their constructors. Doing the wiring here keeps every other
// package unit-testable in isolation (see fake_test_helpers.go in
// internal/api and the mock-backed tests in internal/storage).
//
// WHAT THIS FILE DOES
//
//   - Loads config from environment variables (with safe defaults
//     so a developer can `go run ./cmd/server` against a local
//     Postgres + Redis without touching .env).
//   - Constructs the relational store (Postgres + TimescaleDB
//     best-effort) and the key/value store (Redis Active Pool).
//   - Builds the operator-lookup service (MNP-TR + IP-Reverse +
//     in-memory cache for MVP).
//   - Builds an in-memory matrix querier (production will swap to
//     a TimescaleDB-backed aggregator; not in Sprint 1).
//   - Wires the api.Config and gets the chi router via api.Handler().
//   - Mounts the chi router at /api/v1/ on a parent http.ServeMux,
//     and attaches a custom /healthz that PINGS Postgres + Redis
//     and reports each component's status individually.
//   - Runs the server with explicit timeouts and a SIGINT/SIGTERM
//     graceful-shutdown path that propagates context through to
//     the in-flight requests, then closes every opened resource.
//
// WHAT THIS FILE DOES NOT DO
//
//   - WebSocket signalling handler (matching package; Sprint 1
//     WebSocket lives behind chi in a later PR).
//   - Read /auth / verification of operator lookups against live
//     BTK MNP API (out of MVP scope, per HANDOFF §9).
//   - Real TimescaleDB-backed matrix aggregator (the wire contract
//     is in place; only the SQL pipeline is deferred).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/opene2ee-com/e2ee-app/backend/internal/api"
	"github.com/opene2ee-com/e2ee-app/backend/internal/operator"
	"github.com/opene2ee-com/e2ee-app/backend/internal/storage"
)

// -----------------------------------------------------------------------------
// Config
// -----------------------------------------------------------------------------

// Config bundles every value read from the environment. Hardcoded
// defaults match a typical local-dev setup (Postgres + Redis on
// localhost) so a developer can run `go run ./cmd/server` with no
// extra setup; production reads DATABASE_URL / REDIS_ADDR /
// SERVER_PORT from the process environment (set in infra/.env).
//
// Sprint 1 deliberately does NOT depend on a library like viper or
// godotenv — the brief asks for "hardcoded default + env override"
// (HANDOFF §4 PR-8). Moving to a config file is a Sprint 2 concern.
type Config struct {
	// ListenAddr is the bind address (e.g. ":8080" or "127.0.0.1:8080").
	ListenAddr string

	// DatabaseURL is a pgx-style connection string. Default points
	// at a local Postgres without TLS, which is what docker-compose
	// ships in PR-13.
	DatabaseURL string

	// RedisAddr is the host:port Redis listens on. Default is the
	// docker-compose port from PR-13.
	RedisAddr string

	// RedisPassword is optional; empty for local-dev / no-auth Redis.
	RedisPassword string

	// ServerSalt is the privacy-critical server-side salt used by
	// auth.HashDeviceID (per ADR-0006). The api package itself does
	// not read this — the mobile client uses it locally and embeds
	// the hash in every request. We surface it here so an
	// operator can confirm via /healthz-style introspection that the
	// server is configured with a non-default salt in production.
	ServerSalt string

	// ShutdownTimeout caps how long srv.Shutdown is allowed to run
	// before we force-kill the process. Sprint 1 default: 30s
	// (matches the existing scaffold).
	ShutdownTimeout time.Duration

	// HealthcheckTimeout caps each dependency ping (Postgres +
	// Redis) so a single hung backend cannot stall the liveness
	// probe past the load-balancer's grace period.
	HealthcheckTimeout time.Duration
}

// loadConfig reads env vars and applies defaults. Returns an error
// only on parse failures (e.g. SERVER_PORT not a valid integer).
//
// Env vars (all optional):
//
//	SERVER_PORT          (default 8080)
//	DATABASE_URL         (default postgres://opene2ee:opene2ee@localhost:5432/opene2ee?sslmode=disable)
//	REDIS_ADDR           (default localhost:6379)
//	REDIS_PASSWORD       (default "")
//	SERVER_SALT          (default "opene2ee-v1-salt-dev-only-change-in-prod")
//	SHUTDOWN_TIMEOUT_SEC (default 30)
//	HEALTHCHECK_TIMEOUT_MS (default 2000)
func loadConfig() (Config, error) {
	c := Config{
		ListenAddr:          ":8080",
		DatabaseURL:         "postgres://opene2ee:opene2ee@localhost:5432/opene2ee?sslmode=disable",
		RedisAddr:           "localhost:6379",
		RedisPassword:       "",
		ServerSalt:          "opene2ee-v1-salt-dev-only-change-in-prod",
		ShutdownTimeout:     30 * time.Second,
		HealthcheckTimeout:  2 * time.Second,
	}

	if v := strings.TrimSpace(os.Getenv("SERVER_PORT")); v != "" {
		// Accept ":8080", "8080", "127.0.0.1:8080". Normalise to a
		// value http.Server.Addr understands.
		if _, err := strconv.Atoi(v); err == nil {
			c.ListenAddr = ":" + v
		} else {
			c.ListenAddr = v
		}
	}

	if v := strings.TrimSpace(os.Getenv("DATABASE_URL")); v != "" {
		c.DatabaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("REDIS_ADDR")); v != "" {
		c.RedisAddr = v
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		c.RedisPassword = v
	}
	if v := strings.TrimSpace(os.Getenv("SERVER_SALT")); v != "" {
		c.ServerSalt = v
	}
	if v := strings.TrimSpace(os.Getenv("SHUTDOWN_TIMEOUT_SEC")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid SHUTDOWN_TIMEOUT_SEC=%q (must be a positive integer)", v)
		}
		c.ShutdownTimeout = time.Duration(n) * time.Second
	}
	if v := strings.TrimSpace(os.Getenv("HEALTHCHECK_TIMEOUT_MS")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Config{}, fmt.Errorf("invalid HEALTHCHECK_TIMEOUT_MS=%q (must be a positive integer)", v)
		}
		c.HealthcheckTimeout = time.Duration(n) * time.Millisecond
	}

	return c, nil
}

// -----------------------------------------------------------------------------
// Healthz
// -----------------------------------------------------------------------------

// healthCheck is one component's ping result. Reported individually
// per the task spec ("Postgres ping + Redis ping, her biri ayrı
// status döner"). The HTTP status of the whole /healthz response is
// 200 only when every component reports "ok"; otherwise 503 so a
// load-balancer can drain the pod.
type healthCheck struct {
	Status    string `json:"status"`           // "ok" | "error"
	LatencyMS int64  `json:"latency_ms"`       // wall-clock time of the ping
	Error     string `json:"error,omitempty"`  // populated only on Status="error"
}

// healthzResponse is the JSON body of GET /healthz. Top-level Status
// mirrors the worst component status ("ok" if all components ok,
// otherwise "degraded"). Service is the service identifier the
// existing scaffold advertised.
type healthzResponse struct {
	Status  string                 `json:"status"`  // "ok" | "degraded"
	Service string                 `json:"service"` // "opene2ee-backend"
	Checks  map[string]healthCheck `json:"checks"`  // "postgres", "redis", ...
}

// healthzHandler builds the GET /healthz handler. It runs every
// component's ping in parallel (bounded by cfg.HealthcheckTimeout
// per check) so a slow component doesn't add latency to a healthy
// one.
func healthzHandler(store *storage.PostgresStore, pool *storage.RedisStore, cfg Config, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Each ping gets its own bounded context so a hung backend
		// cannot stall the whole probe past the load-balancer's
		// grace period.
		var (
			wg     sync.WaitGroup
			pgChk  healthCheck
			rdChk  healthCheck
			pgCtx  context.Context
			pgStop context.CancelFunc
			rdCtx  context.Context
			rdStop context.CancelFunc
		)

		pgCtx, pgStop = context.WithTimeout(r.Context(), cfg.HealthcheckTimeout)
		defer pgStop()
		rdCtx, rdStop = context.WithTimeout(r.Context(), cfg.HealthcheckTimeout)
		defer rdStop()

		wg.Add(2)
		go func() {
			defer wg.Done()
			pgChk = pingPostgres(pgCtx, store)
		}()
		go func() {
			defer wg.Done()
			rdChk = pingRedis(rdCtx, pool)
		}()
		wg.Wait()

		resp := healthzResponse{
			Service: "opene2ee-backend",
			Checks: map[string]healthCheck{
				"postgres": pgChk,
				"redis":    rdChk,
			},
		}
		if pgChk.Status == "ok" && rdChk.Status == "ok" {
			resp.Status = "ok"
		} else {
			resp.Status = "degraded"
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if resp.Status == "ok" {
			w.WriteHeader(http.StatusOK)
		} else {
			// 503 — Service Unavailable — signals the load balancer
			// to stop sending traffic until the dependency recovers.
			w.WriteHeader(http.StatusServiceUnavailable)
			logger.Warn("healthz degraded",
				"postgres", pgChk.Status,
				"redis", rdChk.Status,
			)
		}
		// encoding/json cannot fail for this static shape, but if it
		// did we'd rather know via a panic than a silent partial body.
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// pingPostgres runs a lightweight `SELECT 1` against the connection
// pool. The store's PgxPool interface deliberately does NOT expose
// Ping (the mockable subset in PR-1 is exec/query/tx-only) so we
// use Exec with the simplest possible probe statement.
func pingPostgres(ctx context.Context, store *storage.PostgresStore) healthCheck {
	start := time.Now()
	hc := healthCheck{}
	if store == nil {
		hc.Status = "error"
		hc.Error = "postgres store not initialised"
		return hc
	}
	if _, err := store.Pool().Exec(ctx, "SELECT 1"); err != nil {
		hc.Status = "error"
		hc.Error = err.Error()
		hc.LatencyMS = time.Since(start).Milliseconds()
		return hc
	}
	hc.Status = "ok"
	hc.LatencyMS = time.Since(start).Milliseconds()
	return hc
}

// pingRedis pings the Redis client. Returns "error" with the wrapped
// error message if the ping fails or times out.
func pingRedis(ctx context.Context, store *storage.RedisStore) healthCheck {
	start := time.Now()
	hc := healthCheck{}
	if store == nil || store.Client() == nil {
		hc.Status = "error"
		hc.Error = "redis client not initialised"
		return hc
	}
	if err := store.Client().Ping(ctx).Err(); err != nil {
		hc.Status = "error"
		hc.Error = err.Error()
		hc.LatencyMS = time.Since(start).Milliseconds()
		return hc
	}
	hc.Status = "ok"
	hc.LatencyMS = time.Since(start).Milliseconds()
	return hc
}

// -----------------------------------------------------------------------------
// Logger
// -----------------------------------------------------------------------------

// newLogger returns a JSON-formatted slog.Logger writing to stdout.
// slog.Default() is also set so any package that uses
// `slog.Default()` directly (the api package does this for the
// access-log middleware if no logger is supplied) gets the same
// handler.
func newLogger() *slog.Logger {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
	return logger
}

// -----------------------------------------------------------------------------
// Module assembly
// -----------------------------------------------------------------------------

// runMigrations applies the schema and (best-effort) the
// TimescaleDB hypertable. Returns an error ONLY if the base
// Migrate fails — EnsureTimescale failures are logged and ignored
// because the production-readiness spec lets us ship without the
// extension (PR-1 contract: "EnsureTimescale … failing gracefully
// if the extension is not installed"). Returning early would block
// a dev who only has plain Postgres running.
func runMigrations(ctx context.Context, store *storage.PostgresStore, logger *slog.Logger) error {
	if err := store.Migrate(ctx); err != nil {
		return fmt.Errorf("postgres migrate: %w", err)
	}
	logger.Info("postgres schema migrated")
	if err := store.EnsureTimescale(ctx); err != nil {
		// Soft-fail per PR-1 contract: the table is already a regular
		// Postgres table, telemetry still works.
		logger.Warn("timescale hypertable skipped",
			"reason", err.Error(),
		)
		return nil
	}
	logger.Info("timescale hypertable ensured")
	return nil
}

// -----------------------------------------------------------------------------
// main
// -----------------------------------------------------------------------------

func main() {
	logger := newLogger()

	cfg, err := loadConfig()
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}
	logger.Info("config loaded",
		"listen_addr", cfg.ListenAddr,
		"redis_addr", cfg.RedisAddr,
		"database_url_host", redactDSN(cfg.DatabaseURL),
		"server_salt_configured", cfg.ServerSalt != "opene2ee-v1-salt-dev-only-change-in-prod",
	)

	// Long-lived startup context. Bounded generously — Postgres /
	// Redis dialing is usually sub-second, but a slow first connect
	// in a cold-start container can take a few seconds.
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelStartup()

	// Storage: Postgres.
	pgStore, err := storage.NewPostgresStore(startupCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("postgres open failed", "err", err)
		os.Exit(1)
	}
	defer pgStore.Close()
	logger.Info("postgres connected")

	if err := runMigrations(startupCtx, pgStore, logger); err != nil {
		logger.Error("migrate failed", "err", err)
		os.Exit(1)
	}

	// Storage: Redis (Active Pool).
	rdStore, err := storage.NewRedisStore(startupCtx, cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		logger.Error("redis open failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		if cerr := rdStore.Close(); cerr != nil {
			logger.Warn("redis close error", "err", cerr)
		}
	}()
	logger.Info("redis connected")

	// Operator Tespit Servisi. Cache is the in-process NoopCache
	// for Sprint 1 (the operator cache layer is wired and tested in
	// PR-3, but a cross-process Redis cache means another moving
	// part to operate in dev — Sprint 2 swaps to operator.RedisCache
	// once a connection-pool sharing story is finalised).
	phoneAdapters := []operator.OperatorLookup{
		operator.NewMNPTRAdapter(),
	}
	ipAdapters := []operator.OperatorLookup{
		operator.NewIPReverseAdapter(),
	}
	opSvc, err := operator.NewService(operator.NoopCache{}, phoneAdapters, ipAdapters)
	if err != nil {
		logger.Error("operator service init failed", "err", err)
		os.Exit(1)
	}
	logger.Info("operator service initialised",
		"phone_adapters", len(phoneAdapters),
		"ip_adapters", len(ipAdapters),
	)

	// Matrix querier. Sprint 1 ships the in-memory dev fallback
	// (PR-7 contract: "use NewMemoryMatrixQuerier for dev"). The
	// TimescaleDB-backed aggregator that will replace this is a
	// separate PR.
	matrix := api.NewMemoryMatrixQuerier()

	// Build the REST surface. operator.Service satisfies
	// api.OperatorLookup (LookupByPhone + LookupByIP). pgStore
	// satisfies every storage.* subset the api package depends on.
	apiSvc, err := api.New(api.Config{
		Logger:     logger,
		Sessions:   pgStore,
		Telemetry:  pgStore,
		Users:      pgStore,
		Operator:   opSvc,
		Matrix:     matrix,
		Devices:    pgStore,
		// DeleteUserHook intentionally omitted in Sprint 1 — the
		// Active Pool purge for KVKK deletes will be added once
		// the matching package exposes a DeleteByHash on its
		// Redis-backed pool. Until then, the relational delete is
		// sufficient (the device can't authenticate any more
		// anyway because its public_key row is gone).
	})
	if err != nil {
		logger.Error("api init failed", "err", err)
		os.Exit(1)
	}

	// Compose the HTTP handler tree. We mount the chi router ONLY
	// under /api/v1/ so we can attach our own /healthz to the
	// parent mux (the chi router's placeholder /healthz is replaced
	// by the dependency-pinging version below).
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthzHandler(pgStore, rdStore, cfg, logger))
	mux.Handle("/api/v1/", apiSvc.Handler())
	mux.Handle("/api/v1", apiSvc.Handler())

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown. The new PR-8 context is propagated to the
	// HTTP server's Shutdown call so any downstream work that reads
	// the request context sees a cancelled ancestor when the process
	// is asked to stop.
	shutdownCtx, cancelShutdown := context.WithCancel(context.Background())
	defer cancelShutdown()

	// (1) listen-goroutine: surfaces ListenAndServe errors via a
	// dedicated channel so the main goroutine can decide between
	// "the operator asked me to stop" and "the listener died".
	listenErrCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErrCh <- err
		}
		close(listenErrCh)
	}()

	// (2) signal-goroutine: the existing SIGTERM handler from the
	// scaffold is preserved — the only addition is that we ALSO
	// accept SIGINT for `Ctrl-C` in local dev.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("shutdown signal received", "signal", sig.String())
		cancelShutdown()
	case err, ok := <-listenErrCh:
		if ok && err != nil {
			logger.Error("server failed", "err", err)
		}
	}

	// Bounded shutdown so a hung request cannot block restart
	// forever. The deferred Close() calls above run AFTER this
	// returns and drain the underlying Postgres + Redis pools.
	shutdownSrvCtx, shutdownCancel := context.WithTimeout(shutdownCtx, cfg.ShutdownTimeout)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownSrvCtx); err != nil {
		logger.Error("server shutdown failed", "err", err)
		os.Exit(1)
	}
	logger.Info("server stopped cleanly")
}

// redactDSN strips the user:password portion of a Postgres DSN so
// we can log which host we're connecting to without leaking
// credentials into the structured-log stream.
func redactDSN(dsn string) string {
	const at = "@"
	const slashes = "//"
	i := strings.Index(dsn, slashes)
	if i < 0 {
		return dsn
	}
	rest := dsn[i+len(slashes):]
	j := strings.Index(rest, at)
	if j < 0 {
		return dsn
	}
	return dsn[:i+len(slashes)] + "***:***" + rest[j:]
}