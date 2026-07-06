# Kong — API Gateway Operations

Kong 3.x running in **DB-less declarative mode** (no Postgres/DB backing).

## Configuration

- Declarative config: `kong.yml` (mounted at `/etc/kong/kong.yml` by docker-compose)
- `KONG_DATABASE=off` is set in `docker-compose.yml`
- Config reload: `docker compose restart kong` (declarative mode requires restart, not SIGHUP)

## Plugins

Three plugins enabled on the `e2ee-backend` service → `/api/v1` route:

1. **rate-limiting** — 60 req/min, 1000 req/hour, `policy: local` (in-memory, DB-less uyumlu)
2. **cors** — origins `*`, methods GET/POST/DELETE, headers Content-Type/Authorization/X-Request-ID
3. **bot-detection** — allowlist: `curl`, `wget`, `OpenE2EE/*`

Disable a plugin: edit `kong.yml` → remove the entry → `docker compose restart kong`.

## Health Check

Kong admin API: `http://localhost:8001/status`

```bash
curl -s http://localhost:8001/status | jq .
```

## Logs

- Access log: `/var/log/kong/access.log` (stdout in docker-compose)
- Error log: `/var/log/kong/error.log`
- View: `docker compose logs -f kong`

## Production Notes

- **SSL termination** lives on the Nginx profile-based layer (port 443), Kong port 8000 (proxy) + 8001 (admin) stay on internal network
- **certbot + Let's Encrypt** integrates via Nginx profile; Kong sees HTTPS traffic after Nginx
- Replace CORS `origins: "*"` with explicit allowlist (e.g. `https://app.opene2ee.com`) in production