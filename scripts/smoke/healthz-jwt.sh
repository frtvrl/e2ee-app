#!/usr/bin/env bash
# scripts/smoke/healthz-jwt.sh
# ----------------------------------------------------------------------------
# Sprint 7 AUTHZ-2 — Smoke test: GET /healthz through Kong MUST require a
# valid JWT. Asserts:
#
#   1. unauth request  -> 401 Unauthorized at the gateway, body not JSON
#   2. valid JWT       -> 200 OK + JSON with status="ok" or "degraded"
#   3. expired JWT     -> 401 Unauthorized at the gateway
#
# Why this matters: /healthz leaks Postgres + Redis pool status
# (`pool.count`, `postgres.status`, `redis.status`). Leaving it open
# lets an attacker fingerprint the deployment. This script proves
# the Sprint 7 AUTHZ-2 hardening is in place at the Kong gate.
#
# Usage:
#   KONG_PROXY_URL=http://localhost:8000 \
#     JWT_SECRET=$(grep ^JWT_SECRET infra/.env | cut -d= -f2-) \
#     bash scripts/smoke/healthz-jwt.sh
#
# Required env:
#   KONG_PROXY_URL   Kong proxy URL (default http://localhost:8000)
#   JWT_SECRET       HS256 shared secret (must match Kong + backend)
#
# Optional:
#   HEALTHZ_PATH     Override the path (default /healthz)
#   SKIP_DOCKER_WAIT Don't wait for Kong to accept connections
# ----------------------------------------------------------------------------
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

KONG_PROXY_URL="${KONG_PROXY_URL:-http://localhost:8000}"
HEALTHZ_PATH="${HEALTHZ_PATH:-/healthz}"
SKIP_DOCKER_WAIT="${SKIP_DOCKER_WAIT:-0}"

# ---- preflight -------------------------------------------------------------
if [ -z "${JWT_SECRET:-}" ]; then
    echo "ERROR: JWT_SECRET env var is required (must match infra/kong/kong.yml's {vault://env/jwt-secret})." >&2
    echo "       Set it from infra/.env, e.g.:" >&2
    echo "         export JWT_SECRET=\$(grep ^JWT_SECRET infra/.env | cut -d= -f2-)" >&2
    exit 2
fi

if ! command -v curl >/dev/null 2>&1; then
    echo "ERROR: curl is required (the smoke test uses curl)." >&2
    exit 2
fi

if ! command -v python >/dev/null 2>&1 && ! command -v python3 >/dev/null 2>&1 && ! command -v openssl >/dev/null 2>&1; then
    echo "ERROR: either python or openssl is required to mint the test JWT." >&2
    exit 2
fi

# ---- optional: wait for Kong to be reachable -------------------------------
if [ "${SKIP_DOCKER_WAIT}" != "1" ]; then
    echo "==> Waiting for Kong at ${KONG_PROXY_URL} ..."
    for _ in $(seq 1 30); do
        if curl -fsS -o /dev/null "${KONG_PROXY_URL}${HEALTHZ_PATH}" \
              -H 'Authorization: Bearer x' 2>/dev/null \
              || true; then
            break
        fi
        sleep 1
    done
fi

# ---- mint a minimal HS256 JWT --------------------------------------------
# Header: {"alg":"HS256","typ":"JWT"}
# Payload: {"iss":"opene2ee-monitoring","exp":<now+300>}
# Signature: HMACSHA256(header_b64.payload_b64, secret)
#
# Implementation note: we use Python because it's the most portable
# option across macOS / Linux / Git-Bash on Windows. openssl is also
# acceptable but requires more shell gymnastics.
mint_jwt() {
    local iss="$1"
    local exp_offset="$2"   # seconds from now
    local now exp header payload header_b64 payload_b64 data sig sig_b64 token

    if command -v python3 >/dev/null 2>&1; then
        PY=python3
    else
        PY=python
    fi

    # shellcheck disable=SC2016
    token="$("${PY}" - <<PYEOF
import base64, hashlib, hmac, json, os, time, sys
secret = os.environ['JWT_SECRET'].encode('utf-8')
iss    = "${iss}"
exp    = int(time.time()) + ${exp_offset}
header  = {"alg": "HS256", "typ": "JWT"}
payload = {"iss": iss, "exp": exp}
def b64(d: bytes) -> str:
    return base64.urlsafe_b64encode(d).rstrip(b'=').decode('ascii')
header_b64  = b64(json.dumps(header,  separators=(',', ':')).encode('utf-8'))
payload_b64 = b64(json.dumps(payload, separators=(',', ':')).encode('utf-8'))
data = f"{header_b64}.{payload_b64}".encode('ascii')
sig  = hmac.new(secret, data, hashlib.sha256).digest()
print(f"{header_b64}.{payload_b64}.{b64(sig)}")
PYEOF
)"
    printf '%s' "${token}"
}

# ---- 1) no Authorization header -------------------------------------------
echo
echo "==> 1) GET ${HEALTHZ_PATH} without Authorization — expect 401"
HTTP_NO_AUTH=$(curl -s -o /tmp/healthz-noauth.body -w '%{http_code}' \
    "${KONG_PROXY_URL}${HEALTHZ_PATH}")
BODY_NO_AUTH=$(cat /tmp/healthz-noauth.body 2>/dev/null || true)
echo "    response: HTTP ${HTTP_NO_AUTH}, body length: ${#BODY_NO_AUTH}"
if [ "${HTTP_NO_AUTH}" != "401" ]; then
    echo "    FAIL: expected 401 Unauthorized (Sprint 7 AUTHZ-2 hardening)." >&2
    echo "    body: ${BODY_NO_AUTH}" >&2
    exit 1
fi

# ---- 2) valid JWT ---------------------------------------------------------
JWT_VALID=$(mint_jwt "opene2ee-monitoring" 300)
echo
echo "==> 2) GET ${HEALTHZ_PATH} with valid JWT (iss=opene2ee-monitoring, exp=+300s)"
HTTP_OK=$(curl -s -o /tmp/healthz-ok.body -w '%{http_code}' \
    "${KONG_PROXY_URL}${HEALTHZ_PATH}" \
    -H "Authorization: Bearer ${JWT_VALID}")
BODY_OK=$(cat /tmp/healthz-ok.body 2>/dev/null || true)
echo "    response: HTTP ${HTTP_OK}, body: ${BODY_OK:0:120}$( [ "${#BODY_OK}" -gt 120 ] && echo '...')"
if [ "${HTTP_OK}" != "200" ]; then
    echo "    FAIL: expected 200 OK with valid JWT — got ${HTTP_OK}." >&2
    echo "    body: ${BODY_OK}" >&2
    exit 1
fi
if ! printf '%s' "${BODY_OK}" | grep -qE '"status"\s*:\s*"(ok|degraded)"'; then
    echo "    FAIL: response body does not look like a /healthz JSON payload." >&2
    echo "    body: ${BODY_OK}" >&2
    exit 1
fi

# ---- 3) expired JWT ------------------------------------------------------
JWT_EXPIRED=$(mint_jwt "opene2ee-monitoring" -3600)
echo
echo "==> 3) GET ${HEALTHZ_PATH} with expired JWT — expect 401"
HTTP_EXP=$(curl -s -o /tmp/healthz-exp.body -w '%{http_code}' \
    "${KONG_PROXY_URL}${HEALTHZ_PATH}" \
    -H "Authorization: Bearer ${JWT_EXPIRED}")
BODY_EXP=$(cat /tmp/healthz-exp.body 2>/dev/null || true)
echo "    response: HTTP ${HTTP_EXP}, body length: ${#BODY_EXP}"
if [ "${HTTP_EXP}" != "401" ]; then
    echo "    FAIL: expected 401 Unauthorized for expired JWT — got ${HTTP_EXP}." >&2
    echo "    body: ${BODY_EXP}" >&2
    exit 1
fi

# ---- 4) wrong-iss JWT (opene2ee-mobile used on a /healthz consumer) -------
# Belt-and-braces: even if Sprint 6 PR-37 / PR-39 logic ever loops back
# to enforce iss on a /healthz path, the wrong-iss token should already
# fail Kong's HS256 validation in this config (the consumers' jwt_secrets
# only register `iss=opene2ee-monitoring` for the monitoring consumer).
JWT_WRONG_ISS=$(mint_jwt "totally-unknown-issuer" 300)
echo
echo "==> 4) GET ${HEALTHZ_PATH} with wrong-iss JWT — expect 401"
HTTP_WRONG=$(curl -s -o /tmp/healthz-wrong.body -w '%{http_code}' \
    "${KONG_PROXY_URL}${HEALTHZ_PATH}" \
    -H "Authorization: Bearer ${JWT_WRONG_ISS}")
echo "    response: HTTP ${HTTP_WRONG}, body length: $(wc -c < /tmp/healthz-wrong.body)"
if [ "${HTTP_WRONG}" != "401" ]; then
    echo "    FAIL: expected 401 Unauthorized for wrong-iss JWT — got ${HTTP_WRONG}." >&2
    exit 1
fi

# ---- done -----------------------------------------------------------------
echo
echo "==> OK: Sprint 7 AUTHZ-2 hardening verified at the Kong gate."
echo "    /healthz is JWT-enforced (401 on no-auth, expired, wrong-iss;"
echo "    200 on a valid monitoring JWT)."
