# Production Setup — Coturn TLS (Sprint 7 SCA-22)

This document is the **operator runbook** for bringing Coturn (TURN/STUN
server) into a production-grade TLS posture. It is the hand-off companion
to `infra/coturn/turnserver.conf`, `infra/coturn/entrypoint.sh`, and the
`coturn` service block in `infra/docker-compose.yml`.

> **Scope (SCA-22, cyber-security hand-off):**
> - Use a real certificate (Let's Encrypt or internal CA). **No self-signed.**
> - Enforce TLS 1.2+ (no TLS 1.0 / 1.1).
> - Disable plain `turn:` (UDP/TCP) relay — only `turns:` (TLS) or
>   `turn+dtls://` (DTLS) is accepted in production.
> - Document cert rotation.

---

## 1. Cert Provisioning

### 1.1 Option A — Let's Encrypt (recommended for public TURN)

Let's Encrypt issues short-lived (90-day) certs that auto-renew via
`certbot`. The `certbot_data` volume in `docker-compose.yml` is already
declared for Nginx (Sprint 1) and is mounted into the Coturn container at
`/etc/coturn/certs` (RO) so the same certs can serve both surfaces.

**One-time bootstrap (HTTP-01 challenge via Nginx):**

```bash
# 1. Bring up Nginx profile first so port 80 is reachable for ACME
docker compose -f infra/docker-compose.yml \
  --profile nginx --env-file infra/.env up -d nginx

# 2. Issue cert (replace opene2ee.com with your domain)
docker compose -f infra/docker-compose.yml \
  --profile nginx run --rm certbot certonly --webroot \
  --webroot-path=/var/www/certbot \
  --email admin@opene2ee.com --agree-tos --no-eff-email \
  -d opene2ee.com -d turn.opene2ee.com

# 3. Confirm cert layout
docker compose -f infra/docker-compose.yml \
  --profile nginx run --rm certbot certificates
# Expected:
#   Certificate Name: opene2ee.com
#       Domains: opene2ee.com turn.opene2ee.com
#       Expiry Date: ... (90 days from issue)
#       Certificate Path: /etc/letsencrypt/live/opene2ee.com/fullchain.pem
#       Private Key Path: /etc/letsencrypt/live/opene2ee.com/privkey.pem
```

The `certbot_data` Docker volume now holds:

```
/var/lib/docker/volumes/opene2ee-certbot-data/_data/
└── live/
    └── opene2ee.com/
        ├── fullchain.pem     # certificate + chain
        ├── privkey.pem       # private key (mode 0600 in container)
        ├── cert.pem          # leaf only (not used by Coturn)
        └── chain.pem         # chain only (not used by Coturn)
```

### 1.2 Option B — Internal / Corporate CA

If your infra sits behind a private CA (e.g. HashiCorp Vault PKI,
Active Directory CS, Smallstep), drop the issued cert + chain into the
certbot volume directly:

```bash
# On the host:
DOMAIN=opene2ee.com
VOLUME_ROOT=$(docker volume inspect opene2ee-certbot-data \
  --format '{{ .Mountpoint }}')

sudo mkdir -p "${VOLUME_ROOT}/live/${DOMAIN}"
sudo cp /secure/path/fullchain.pem "${VOLUME_ROOT}/live/${DOMAIN}/"
sudo cp /secure/path/privkey.pem   "${VOLUME_ROOT}/live/${DOMAIN}/"
sudo chmod 0644 "${VOLUME_ROOT}/live/${DOMAIN}/fullchain.pem"
sudo chmod 0600 "${VOLUME_ROOT}/live/${DOMAIN}/privkey.pem"
```

Coturn reads them at the same canonical path, so the entrypoint script
needs no change.

### 1.3 Anti-Pattern — Self-Signed

**Do not** self-sign a cert for production Coturn. Mobile clients
(Chrome, Firefox, Safari, native WebRTC stacks) will reject the cert at
the `RTCPeerConnection.setRemoteDescription` step with a generic
`DTLS handshake failure`. End users see a black video screen with no
useful diagnostic.

If you need a temporary cert for staging only, prefer a one-off Let's
Encrypt `--staging` issuance (rate-limit friendly) over self-signed.

---

## 2. Enabling TLS in Production

Edit `infra/.env` (NOT `.env.example` — that file is committed):

```ini
# Coturn TLS / DTLS — production
COTURN_TLS_ENABLED=true
COTURN_TLS_DOMAIN=opene2ee.com       # must match certbot live dir
COTURN_TLS_PORT=5349                 # TURNS (TCP) + DTLS (UDP)
COTURN_TLS_CERTDIR=/etc/coturn/certs # mounted from certbot_data
```

Then restart Coturn:

```bash
docker compose -f infra/docker-compose.yml up -d coturn
```

The entrypoint script (`infra/coturn/entrypoint.sh`) inspects
`COTURN_TLS_ENABLED`. When `true`:

1. Verifies both `fullchain.pem` and `privkey.pem` exist (fail-closed
   otherwise — exits 1 instead of silently falling back to plain mode).
2. Starts `turnserver` with `--tls --dtls`, `cipher-list` (TLS 1.2+),
   `--no-tlsv1 --no-tlsv1_1`, and the cert/pkey paths.
3. Logs the active posture to stderr (`COTURN_TLS_ENABLED=true ...`).

Confirm with:

```bash
docker compose -f infra/docker-compose.yml logs coturn | head -40
docker compose -f infra/docker-compose.yml exec coturn \
  sh -c 'ps -ef | grep turnserver | grep -v grep'
```

---

## 3. Mobile / WebRTC Client Configuration

In `mobile/lib/shared/turn_config.dart` (Sprint 2 will introduce this
file; it is the same place the existing `turn_credentials.dart` lives),
the production ICE config must use TLS/DTLS — **never** plain `turn:`:

```dart
// Production
final iceServers = [
  {
    'urls': [
      'turns:turn.opene2ee.com:5349?transport=tcp',  // TURNS (RFC 6062)
      'turns:turn.opene2ee.com:5349?transport=udp',  // TURNS-over-DTLS
      'stun:turn.opene2ee.com:3478',                  // STUN, plain UDP ok
    ],
    'username': '<ephemeral-username>',
    'credential': '<ephemeral-credential>',
  },
];
```

**Forbidden in production:**
- `turn:turn.opene2ee.com:3478?transport=udp` — credentials leaked.
- `turn:turn.opene2ee.com:3478?transport=tcp` — credentials leaked + no
  confidentiality on relayed data.

**Why STUN can stay plain:** STUN binding requests carry no credentials
and no user payload — they are connectivity probes. DTLS-SRTP then
protects the media channel end-to-end (RFC 5764). Allowing STUN over
plain UDP is the WebRTC norm and is *not* a SCA-22 violation.

---

## 4. Cert Rotation

Certbot renews certs at ~60 days to leave a 30-day safety buffer.
The renewal flow:

```bash
# Cron entry on the host (every 12h, well under the 90-day window):
0 */12 * * * /usr/local/bin/opene2ee-certbot-renew.sh
```

`/usr/local/bin/opene2ee-certbot-renew.sh`:

```bash
#!/bin/bash
set -euo pipefail
cd /opt/opene2ee

docker compose -f infra/docker-compose.yml --profile nginx \
  run --rm certbot renew --quiet

# Coturn picks up the new cert via SIGHUP — active sessions preserved.
docker compose -f infra/docker-compose.yml kill -s HUP coturn
```

Why SIGHUP and not `restart`? Coturn's SIGHUP handler reloads the TLS
context in place. Active TURN allocations are not torn down — only new
handshakes use the fresh cert. Restarting would drop every WebRTC call
currently in progress.

Manual renewal (e.g. after a CA migration):

```bash
docker compose -f infra/docker-compose.yml --profile nginx \
  run --rm certbot renew --force-renewal
docker compose -f infra/docker-compose.yml kill -s HUP coturn
```

Verify the new cert landed:

```bash
bash scripts/smoke/coturn-tls.sh turn.opene2ee.com 5349
# Expected: TLS 1.2 accept, TLS 1.0/1.1 reject, cert subject
#           CN=opene2ee.com (or SAN include) issuer=Let's Encrypt ...
```

---

## 5. Smoke Test

`scripts/smoke/coturn-tls.sh` and `scripts/smoke/coturn-tls.ps1` run a
black-box check from the operator's shell:

```bash
# From repo root
bash scripts/smoke/coturn-tls.sh turn.opene2ee.com 5349
# or on Windows
powershell -ExecutionPolicy Bypass -File scripts/smoke/coturn-tls.ps1 `
  -Host turn.opene2ee.com -Port 5349
```

The script asserts:

1. **TLS 1.0 handshake rejected** — `openssl s_client -tls1` exits non-zero.
2. **TLS 1.1 handshake rejected** — `openssl s_client -tls1_1` exits non-zero.
3. **TLS 1.2 handshake accepted** — `openssl s_client -tls1_2` exits 0 and
   completes the handshake.
4. **Cert chain validates** — `verify_return_error` is empty; subject
   matches the configured `COTURN_TLS_DOMAIN`.
5. **Cipher in TLS 1.2 group** — server picks an ECDHE-AESGCM or
   ECDHE-CHACHA20 cipher (forward secrecy).

A passing run prints `OK: 5/5 coturn-tls assertions passed`. A failure
prints `FAIL: <N>/5 ...` and the offending `openssl s_client` output.

Run after every cert renewal or Coturn config change. Wire it into CI
once Coturn staging is online.

---

## 6. TLS Posture Summary

| Layer                          | Dev (`COTURN_TLS_ENABLED=false`) | Prod (`COTURN_TLS_ENABLED=true`) |
|--------------------------------|----------------------------------|----------------------------------|
| Listening port 3478/udp+TCP    | STUN + plain TURN (relay off)    | STUN only (DTLS gated to 5349)   |
| Listening port 5349/tcp        | (closed)                         | TURNS (RFC 6062)                 |
| Listening port 5349/udp        | (closed)                         | TURN-over-DTLS (RFC 6347)        |
| Cert                           | none                             | Let's Encrypt or internal CA     |
| Cipher suite                   | n/a                              | TLS 1.2+ only, ECDHE-AESGCM preferred |
| TLS versions                   | n/a                              | TLS 1.2, TLS 1.3 (TLS 1.0/1.1 refused) |
| Plain `turn://` (UDP/TCP)      | Allowed in dev (no creds in test) | **Refused** — only `turns:` or DTLS |
| Cert rotation                  | n/a                              | certbot auto-renew + SIGHUP      |

---

## 7. References

- [RFC 5766 — Traversal Using Relays around NAT (TURN)](https://www.rfc-editor.org/rfc/rfc5766)
- [RFC 6062 — TURN Over TCP](https://www.rfc-editor.org/rfc/rfc6062)
- [RFC 6347 — Datagram Transport Layer Security Version 1.2](https://www.rfc-editor.org/rfc/rfc6347)
- [RFC 8656 — TURN (obsoletes 5766)](https://www.rfc-editor.org/rfc/rfc8656)
- [Mozilla SSL Configuration Generator — Intermediate](https://ssl-config.mozilla.org/#server=nginx&config=intermediate)
- [EFF Certbot docs](https://eff-certbot.readthedocs.io/en/latest/)
- `infra/coturn/turnserver.conf` — Coturn runtime config
- `infra/coturn/entrypoint.sh` — TLS gate logic
- `infra/nginx/README.md` — Nginx + Let's Encrypt convention
- `scripts/smoke/coturn-tls.sh` / `.ps1` — TLS 1.2+ smoke test