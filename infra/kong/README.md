# Kong — API Gateway Operations

Kong 3.x running in **DB-less declarative mode** (no Postgres/DB backing).

## Configuration

- Declarative config: `kong.yml` (mounted at `/etc/kong/kong.yml` by docker-compose)
- `KONG_DATABASE=off` is set in `docker-compose.yml`
- Config reload: `docker compose restart kong` (declarative mode requires restart, not SIGHUP)

## Plugins

### Service-level (every route)

Three plugins are wired at the **service level** so they apply to every
route on `e2ee-backend` — public and protected alike:

1. **rate-limiting** — 60 req/min, 1000 req/hour, `policy: local` (in-memory, DB-less uyumlu)
2. **cors** — explicit allowlist: `https://app.opene2ee.com` (prod), `https://staging.opene2ee.com` (staging), `http://localhost:3000` + `http://localhost:8080` (dev); methods GET/POST/DELETE; headers Content-Type/Authorization/X-Request-ID
3. **bot-detection** — allowlist: `curl`, `wget`, `OpenE2EE/*`

Disable a service-level plugin: edit `kong.yml` → remove the entry under
`services[].plugins` → `docker compose restart kong`.

### Route-level — JWT (Sprint 5 PR-32, ADV-3; Sprint 7 AUTHZ-2 hardening)

The **JWT plugin** is wired on a per-route basis, NOT service-level, so it
fires ONLY on the **protected subtree** (see "Routes" below). The plugin
config is the same on every protected route, including the new `healthz`
route added in Sprint 7 to close the AUTHZ-2 gap (an unauthenticated
attacker probing Postgres / Redis pool status via `/healthz`):

```yaml
plugins:
  - name: jwt
    config:
      secret_is_base64: false
      claims_to_verify:
        - exp
```

Plus **two Consumers** at the top of the yaml hold the shared HS256
secret — one for end-user mobile clients, one for `/healthz` monitoring:

```yaml
consumers:
  - username: opene2ee-mobile
    jwt_secrets:
      - key: opene2ee-backend
        algorithm: HS256
        secret: "{vault://env/jwt-secret}"
  - username: opene2ee-monitoring
    jwt_secrets:
      - key: opene2ee-monitoring
        algorithm: HS256
        secret: "{vault://env/jwt-secret}"
```

#### Why HS256?

- The Go backend (`backend/internal/auth/jwt.go`) signs with HS256
  using `golang-jwt/jwt/v5`.
- The same shared secret verifies on both sides. Kong does the
  gateway check; the backend's `IsAuthorized` middleware re-verifies
  as defence-in-depth.
- The `key` field is the credential identifier; it matches the
  backend's `auth.Issuer = "opene2ee-backend"` claim convention.

#### Why `{vault://env/jwt-secret}`?

Kong's bundled **env vault** resolves env-var references at startup,
so the HS256 secret does NOT appear in `kong.yml` on disk. The env
var is `JWT_SECRET`, set by `infra/docker-compose.yml` for both the
Kong and backend containers (they MUST match). The backend's
`backend/cmd/server/main.go` reads the same env var and passes it to
`api.Config.JWTSecret`.

To rotate the secret (ADV-3 follow-up): change `JWT_SECRET` in
`infra/.env` → `docker compose up -d kong backend`. Both services
restart with the new secret and all existing tokens become invalid
(intentional — rotation invalidates outstanding tokens).

#### `claims_to_verify: [exp]`

Kong validates the `exp` (expiry) claim at the gateway. The full
HS256 signature check + `iss` validation is performed by Kong's
JWT plugin automatically (signature is verified because the secret
is configured; `iss` is verified against the consumer's `key`).

The backend's `IsAuthorized` middleware re-runs the same checks
on the request side — that's the defence-in-depth layer for the
case where Kong is bypassed (local dev, integration tests, future
internal services).

## Routes

Routes are split by auth posture so the JWT plugin fires ONLY on
protected endpoints:

### Public (no JWT)

| Route name                  | Path                       | Methods |
|-----------------------------|----------------------------|---------|
| `api-v1-matrix`             | `/api/v1/matrix`           | GET     |
| `api-v1-operator-lookup`    | `/api/v1/operator/lookup`  | GET     |
| `api-v1-auth`               | `/api/v1/auth`             | POST    |

`api-v1-auth` (login) MUST be public — a login endpoint that
required a token would be chicken-and-egg.

### Protected (JWT required)

| Route name                       | Path                                       | Methods      |
|----------------------------------|--------------------------------------------|--------------|
| `api-v1-sessions`                | `/api/v1/sessions`                         | POST, GET    |
| `api-v1-sessions-by-id`          | `/api/v1/sessions/{id}`                    | GET          |
| `api-v1-sessions-telemetry`      | `/api/v1/sessions/{id}/telemetry`          | POST         |
| `api-v1-users-by-hash`           | `/api/v1/users/{device_id_hash}`           | DELETE       |
| `api-v1-webrtc-config`           | `/api/v1/webrtc/config`                    | GET          |
| `api-v1-webrtc-offer`            | `/api/v1/webrtc/offer`                     | POST         |
| `api-v1-webrtc-answer`           | `/api/v1/webrtc/answer`                    | POST         |
| `api-v1-webrtc-ice`              | `/api/v1/webrtc/ice`                       | POST         |
| `healthz`                        | `/healthz`                                 | GET          |

The `healthz` route is the **Sprint 7 AUTHZ-2 hardening**: the Go
backend's `/healthz` handler returns a JSON body containing
`postgres.status` / `redis.status` + the live Active-Pool size, so
leaving it unauthenticated lets an attacker fingerprint the
deployment and time probes to backend-disruption events. Kong now
requires a valid JWT on `GET /healthz` before forwarding to the
backend.

Path matching:

- Plain prefix strings for exact paths (e.g. `/api/v1/sessions`).
- PCRE regex (prefix `~`) for dynamic captures (e.g.
  `~/api/v1/sessions/[^/]+$` matches `/api/v1/sessions/<id>`
  without matching `/api/v1/sessions/<id>/telemetry`).
- The full path is forwarded to the backend (default `strip_path`
  is false) so the Go chi router sees the original URL.

### Adding a new protected route

When you add a new route to the backend that should require a
JWT, follow this pattern in `kong.yml`:

```yaml
  - name: api-v1-<your-route>
    paths:
      - /api/v1/<your-path>     # or "~/api/v1/<your-path>/[^/]+$" for dynamic
    methods:
      - <METHOD>
    plugins:
      - name: jwt
        config:
          secret_is_base64: false
          claims_to_verify:
            - exp
```

Then:

1. Register the route in `backend/internal/api/router.go` inside
   the JWT-protected `r.Group(...)` subtree (where
   `r.Use(a.IsAuthorized())` is applied).
2. Mirror the route in this README's "Protected" table.

### Adding a new public route

Just add a route block WITHOUT a `plugins:` entry:

```yaml
  - name: api-v1-<your-public-route>
    paths:
      - /api/v1/<your-path>
    methods:
      - <METHOD>
```

Service-level plugins (rate-limit, CORS, bot-detection) still apply.

## End-to-end JWT flow

```
client                    Kong (JWT plugin)              Go backend (IsAuthorized)
  |                              |                              |
  | POST /api/v1/auth            |                              |
  |----------------------------->|                              |
  |                              | (public route — no JWT)     |
  |                              |----------------------------->|
  |                              |                              | IssueJWT(user_id, ttl, JWT_SECRET)
  |                              |<-----------------------------|
  |<------ 200 + token ----------|                              |
  |                              |                              |
  | GET /api/v1/sessions         |                              |
  |  Authorization: Bearer ...   |                              |
  |----------------------------->|                              |
  |                              | verify HS256 + exp           |
  |                              | (forward with X-Consumer-*)  |
  |                              |----------------------------->|
  |                              |                              | VerifyJWT(token, JWT_SECRET)
  |                              |                              | ctx.Value("user_id") = claims.Subject
  |                              |<------ 200 ------------------|
  |<------ 200 ------------------|                              |
```

Both Kong and the backend use the SAME `JWT_SECRET` env var, so a
token valid at the gateway is also valid at the backend. The
backend's `IsAuthorized` is the **defence-in-depth** check for
direct-backend calls (local dev, integration tests, future
internal services).

## Health Check

Kong admin API: `http://localhost:8001/status`

```bash
curl -s http://localhost:8001/status | jq .
```

### Application `/healthz` (Sprint 7 AUTHZ-2)

The Go backend exposes `GET /healthz` for liveness probes. The
response body includes `postgres.status`, `redis.status`, and the
Active Pool size (`pool.count`) — sensitive post-Sprint-3 deployment
metadata that **must not leak to unauthenticated callers**.

Two consumer credentials exist in `kong.yml`:

| Consumer                  | `iss` claim (`jwt_secrets.key`) | Audience                       |
|---------------------------|---------------------------------|--------------------------------|
| `opene2ee-mobile`         | `opene2ee-backend`              | End-user mobile clients        |
| `opene2ee-monitoring`     | `opene2ee-monitoring`           | External uptime / Prometheus   |

#### External probe (Prometheus blackbox, status page)

Mint a JWT signed with `JWT_SECRET` and `iss=opene2ee-monitoring`
(any standard HS256 library), then:

```bash
curl -i http://localhost:8000/healthz \
  -H "Authorization: Bearer $TOKEN"
# → 200 OK + JSON body
```

No token (or expired / wrong-iss token) returns **401 Unauthorized**
at the gateway and never reaches the backend. There is no anonymous
fallback by design (Sprint 7 AUTHZ-2).

#### Container healthcheck (Docker internal network)

`backend`'s Docker healthcheck already pings `localhost:8080/healthz`
directly via the internal network, **bypassing Kong** (no token
needed). `nginx`'s healthcheck (Sprint 7 follow-up edit) now also
pings `http://backend:8080/healthz` directly for the same reason.

```bash
# From INSIDE the backend container
wget -qO- http://localhost:8080/healthz

# From INSIDE the nginx container (shares the `external` Docker
# network with backend)
wget -qO- http://backend:8080/healthz
```

If you change the backend listen port (default `8080`), update
both healthcheck blocks in `infra/docker-compose.yml`.

A runnable smoke test lives at `scripts/smoke/healthz-jwt.sh`; it
expects the Kong proxy to be reachable at `$KONG_PROXY_URL`
(default `http://localhost:8000`) and asserts that a request
**without** `Authorization` returns 401, while a request **with** a
freshly-minted HS256 JWT (using `iss=opene2ee-monitoring`) returns
200 + JSON.

## Logs

- Access log: `/var/log/kong/access.log` (stdout in docker-compose)
- Error log: `/var/log/kong/error.log`
- View: `docker compose logs -f kong`

## Production Notes

- **SSL termination** lives on the Nginx profile-based layer (port 443), Kong port 8000 (proxy) + 8001 (admin) stay on internal network
- **certbot + Let's Encrypt** integrates via Nginx profile; Kong sees HTTPS traffic after Nginx
- CORS `origins` is an explicit allowlist (production + staging + dev). Wildcard `"*"` must NOT be used — see `kong.yml` for the canonical list.
- **JWT_SECRET** MUST be ≥32 bytes. Generate with `openssl rand -hex 32` and store in `infra/.env` (NEVER commit). Rotate quarterly; rotation invalidates outstanding tokens (intentional, see above).