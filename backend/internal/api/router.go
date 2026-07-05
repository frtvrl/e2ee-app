package api

// router.go — chi router setup + route registration.
//
// Public surface:
//
//	api.Handler()    — the full mux (router + middleware chain).
//	api.MountOn(mux) — mount /api/v1 + /healthz onto an existing
//	                   http.Handler (e.g. the http.ServeMux in
//	                   cmd/server/main.go after PR-8 wire-up).
//
// Middleware chain (outermost to innermost):
//
//	RequestID -> DeviceContext -> AccessLog -> CORS -> MaxBytes
//	  -> [on /api/*] APIVersion -> RateLimit -> handler
//
// /healthz sits OUTSIDE the API middleware stack so the load
// balancer doesn't need to send X-API-Version or a device id
// to hit the liveness probe.

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Handler returns the fully-wired chi.Mux. Use this directly in
// tests or pass it to http.Server in main().
func (a *API) Handler() http.Handler {
	return a.buildRouter()
}

// MountOn attaches the API surface onto the given mux under
// the conventional paths:
//
//	/healthz          — liveness probe (no API middleware)
//	/api/v1/...       — full REST surface (API middleware stack)
//	/api/v1           — alias (so /api/v1 without trailing slash
//	                    returns 404 from chi rather than a 200
//	                    from the parent mux)
//
// PR-8 calls MountOn with the bare http.ServeMux from
// cmd/server/main.go so the chi router handles only the
// application paths and the parent can host additional
// (non-API) infrastructure if needed.
func (a *API) MountOn(mux *http.ServeMux) {
	h := a.Handler()
	mux.Handle("/healthz", h)
	mux.Handle("/api/v1/", h)
	mux.Handle("/api/v1", h)
}

// buildRouter constructs the chi.Mux from scratch on every
// call. Cheap (no caching needed — it's just struct setup) and
// keeps the constructor's behavior deterministic for tests.
func (a *API) buildRouter() http.Handler {
	r := chi.NewRouter()

	// Outer middleware — applies to every request that hits
	// the chi router. /healthz sits at the root so it gets
	// these too but skips the API-version + rate-limit checks.
	r.Use(RequestIDMiddleware())
	r.Use(DeviceContextMiddleware())
	r.Use(AccessLogMiddleware(a.cfg.Logger))
	r.Use(CORSMiddleware(a.cfg.CORS))
	r.Use(MaxBytesMiddleware(a.cfg.MaxBodyBytes))

	// /healthz — liveness probe. Sits OUTSIDE the /api/v1
	// subtree so the load balancer doesn't need X-API-Version.
	// Returns a minimal JSON body. PR-8 wire-up extends this
	// to also ping Postgres + Redis.
	r.Get("/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"opene2ee-backend"}`))
	})

	// Root: catch-all for paths that aren't /healthz and aren't
	// under /api/v1/. We return 404 rather than the default
	// "404 page not found" text so clients get a parseable body.
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		writeError(w, http.StatusNotFound, ErrorBody{
			Code:    CodeNotFound,
			Message: "Use /api/v1/sessions, /api/v1/matrix, /api/v1/operator/lookup, or /api/v1/users/{hash}.",
		})
	})

	// /api/v1 subtree: API-version + rate-limit then handlers.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(APIVersionMiddleware(a.cfg.AcceptedAPIVersions...))
		r.Use(RateLimitMiddleware(a.cfg.RateLimit))

		// sessions
		r.Post("/sessions", a.handleCreateSession())
		r.Get("/sessions", a.handleListSessions())
		r.Get("/sessions/{id}", a.handleGetSession())
		r.Post("/sessions/{id}/telemetry", a.handlePostTelemetry())

		// matrix
		r.Get("/matrix", a.handleMatrix())

		// operator lookup
		r.Get("/operator/lookup", a.handleOperatorLookup())

		// users (KVKK delete)
		r.Delete("/users/{device_id_hash}", a.handleDeleteUser())
	})

	return r
}