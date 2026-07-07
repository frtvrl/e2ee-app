#!/bin/sh
# ============================================================================
# Coturn entrypoint — Sprint 7 SCA-22 (TLS/DTLS gate)
# ----------------------------------------------------------------------------
# Bu script coturn/coturn:4.6 image'in default entrypoint'ini override eder.
# COTURN_TLS_ENABLED env değişkenine göre dev/prod komut ayarlarını kurar:
#
#   COTURN_TLS_ENABLED=false (default, dev):
#     -n --no-tls --no-dtls --simple-log
#     + tüm sertifikasız dev parametreleri
#
#   COTURN_TLS_ENABLED=true (prod):
#     -n --tls --dtls
#     --cert=/etc/coturn/certs/live/${COTURN_TLS_DOMAIN}/fullchain.pem
#     --pkey=/etc/coturn/certs/live/${COTURN_TLS_DOMAIN}/privkey.pem
#     --tls-listening-port=${COTURN_TLS_PORT:-5349}
#     --dtls-listening-port=${COTURN_TLS_PORT:-5349}
#     --cipher-list=<TLS 1.2+ only>
#     --no-tlsv1 --no-tlsv1_1 --no-sslv2 --no-sslv3
#     + sertifika mevcut değilse HATA ile exit 1 (fail-closed)
#
# Üretim sertifika yolu: /etc/coturn/certs/live/<DOMAIN>/
#   (certbot_data volume mount tarafından sağlanır; bkz docker-compose.yml)
#
# Cert rotation: certbot renew sonrası `docker kill -s HUP coturn`
# (Coturn SIGHUP ile TLS context reload yapar; mevcut session korunur).
# ============================================================================

set -eu

# ---- Ortak (dev + prod) parametreler ----
COMMON_ARGS="-n
  --log-file=stdout
  --realm=${COTURN_REALM:-opene2ee.local}
  --static-auth-secret=${COTURN_STATIC_SECRET:?COTURN_STATIC_SECRET gerekli}
  --use-auth-secret
  --listening-port=${COTURN_LISTEN_PORT:-3478}
  --min-port=${COTURN_MIN_PORT:-49152}
  --max-port=${COTURN_MAX_PORT:-65535}
  --fingerprint
  --no-multicast-peers
  --no-cli
  --no-tcp-relay
  --stale-nonce=600
  --no-loopback-peers
  --no-multicast-peers
  --allowed-peer-ip=${COTURN_PUBLIC_IP:?COTURN_PUBLIC_IP gerekli (host'un dış IP'si)}
  --simple-log"

# ---- TLS 1.2+ cipher list (Mozilla intermediate, RSA-leaning) ----
CIPHER_LIST="ECDHE+AESGCM:ECDHE+CHACHA20:DHE+AESGCM:DHE+CHACHA20:!aNULL:!MD5:!3DES:!RC4:!DES:!EXPORT:!LOW:!TLSv1:!TLSv1.1:!ECDSA"

if [ "${COTURN_TLS_ENABLED:-false}" = "true" ]; then
  # ------------------------------------------------------------------
  # PRODUCTION — TLS + DTLS aktif (SCA-22)
  # ------------------------------------------------------------------
  TLS_PORT="${COTURN_TLS_PORT:-5349}"
  CERT_DIR="${COTURN_TLS_CERTDIR:-/etc/coturn/certs}"
  DOMAIN="${COTURN_TLS_DOMAIN:-${DOMAIN:-opene2ee.local}}"

  CERT_FILE="${CERT_DIR}/live/${DOMAIN}/fullchain.pem"
  KEY_FILE="${CERT_DIR}/live/${DOMAIN}/privkey.pem"

  # Fail-closed: cert yoksa başlama, logla ve exit 1.
  if [ ! -f "${CERT_FILE}" ] || [ ! -f "${KEY_FILE}" ]; then
    echo "[coturn-entrypoint] FATAL: COTURN_TLS_ENABLED=true ama cert bulunamadı:" >&2
    echo "  CERT_FILE=${CERT_FILE} (exists: $([ -f "${CERT_FILE}" ] && echo yes || echo no))" >&2
    echo "  KEY_FILE=${KEY_FILE}  (exists: $([ -f "${KEY_FILE}" ] && echo yes || echo no))" >&2
    echo "  Certbot provisioning için: docs/SETUP.md §Cert Provisioning" >&2
    exit 1
  fi

  echo "[coturn-entrypoint] COTURN_TLS_ENABLED=true → TLS+DTLS açılıyor" >&2
  echo "[coturn-entrypoint]   CERT: ${CERT_FILE}" >&2
  echo "[coturn-entrypoint]   KEY : ${KEY_FILE}" >&2
  echo "[coturn-entrypoint]   TLS_PORT: ${TLS_PORT}/tcp + ${TLS_PORT}/udp (DTLS)" >&2

  exec turnserver \
    ${COMMON_ARGS} \
    --tls \
    --dtls \
    --tls-listening-port="${TLS_PORT}" \
    --dtls-listening-port="${TLS_PORT}" \
    --cert="${CERT_FILE}" \
    --pkey="${KEY_FILE}" \
    --cipher-list="${CIPHER_LIST}" \
    --no-tlsv1 \
    --no-tlsv1_1 \
    --no-sslv2 \
    --no-sslv3
else
  # ------------------------------------------------------------------
  # DEV — TLS/DTLS kapalı (SCA-22 dev fallback)
  # ------------------------------------------------------------------
  echo "[coturn-entrypoint] COTURN_TLS_ENABLED=false → --no-tls --no-dtls (dev mode)" >&2
  exec turnserver \
    ${COMMON_ARGS} \
    --no-tls \
    --no-dtls
fi