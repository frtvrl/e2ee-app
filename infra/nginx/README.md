# Nginx — Reverse Proxy + SSL Termination

Nginx runs in **profile-based** mode — only activated with `docker compose --profile nginx up`.

## Activation

```bash
# Dev: Kong-only (no Nginx)
docker compose up -d

# Prod: Kong + Nginx (reverse proxy + SSL termination)
docker compose --profile nginx up -d
```

Port 80 and 443 are exposed only when the Nginx profile is active. Kong's port 8000 (proxy) and 8001 (admin) stay on the internal Docker network.

## SSL / Let's Encrypt

Production uses certbot for Let's Encrypt certificates:
- `certbot_data` volume mounts certs at `/etc/ssl/certs/` and `/etc/ssl/private/` inside the Nginx container
- `certbot_www` volume serves HTTP-01 challenge files

Certificate renewal: `docker compose run --rm certbot renew` (cron-driven, runs every 60 days).

## Logs

- Access log: `/var/log/nginx/access.log`
- Error log: `/var/log/nginx/error.log`
- View: `docker compose logs -f nginx`

## Syntax Check

```bash
docker compose exec nginx nginx -t
```

Output: `nginx: configuration file /etc/nginx/nginx.conf test is successful`

## Graceful Reload

```bash
docker compose exec nginx nginx -s reload
```

Use after editing `nginx.conf` and re-mounting the volume.

## Kong Çakışması

Nginx (port 80/443) ve Kong (port 8000/8001) farklı portlarda çalışır — **aynı anda aktif olmaları çakışma yaratmaz**. Dev ortamında sadece Kong (8000) public-facing; prod'da Nginx (80/443) public-facing, Kong internal.

CORS origin `*` Kong seviyesinde tanımlı; Nginx ekliyor reverse proxy yükünü. Production'da CORS origin'i explicit allowlist'e çevirin.