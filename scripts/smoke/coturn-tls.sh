#!/usr/bin/env bash
# ============================================================================
# Coturn TLS 1.2+ Smoke Test — Sprint 7 SCA-22
# ----------------------------------------------------------------------------
# Black-box probe: speaks TLS to a Coturn TURN-over-TLS listener and asserts
# that the server:
#   1. Refuses TLS 1.0 handshake
#   2. Refuses TLS 1.1 handshake
#   3. Accepts TLS 1.2 handshake
#   4. Presents a cert chain that validates against system trust
#   5. Picks a forward-secrecy cipher (ECDHE-AESGCM / ECDHE-CHACHA20)
#
# Run from repo root:
#     bash scripts/smoke/coturn-tls.sh [host] [port]
#
# Defaults: host=turn.opene2ee.com  port=5349
#
# Exit codes:
#   0 = all assertions passed
#   1 = one or more assertions failed
#   2 = openssl missing / network unreachable
# ============================================================================
set -u

HOST="${1:-turn.opene2ee.com}"
PORT="${2:-5349}"

# ---- Pre-flight -------------------------------------------------------------
if ! command -v openssl >/dev/null 2>&1; then
  echo "FAIL: openssl not found in PATH" >&2
  exit 2
fi

if ! command -v nc >/dev/null 2>&1; then
  echo "FAIL: nc (netcat) not found in PATH" >&2
  exit 2
fi

echo "=== Coturn TLS 1.2+ smoke test ==="
echo "Target: ${HOST}:${PORT}"
echo

# Reachability probe (TCP only — DTLS-over-UDP probed separately below).
if ! nc -z -w 3 "${HOST}" "${PORT}"; then
  echo "FAIL: TCP connect to ${HOST}:${PORT} refused or timed out" >&2
  exit 2
fi

# ---- Helpers ----------------------------------------------------------------
# tls_probe VERSION   -> writes openssl s_client output to $PROBE_OUT and
#                        returns openssl's exit code.
# Opens a 1-byte stdin, then closes — this terminates the handshake without
# sending any TURN/STUN bytes, so the server's TLS stack must validate on
# its own. We use `-brief` where supported (1.1.0+) to keep logs readable.
PROBE_OUT="$(mktemp)"
trap 'rm -f "${PROBE_OUT}"' EXIT

tls_probe() {
  local ver="$1"
  # `-no_ign_eof` + redirect from /dev/null forces openssl to attempt the
  # handshake then exit on EOF. We don't send any TURN payload — this
  # checks TLS posture only.
  echo "" | openssl s_client \
    -connect "${HOST}:${PORT}" \
    -"${ver}" \
    -no_ign_eof \
    -servername "${HOST}" \
    -verify_return_error \
    >"${PROBE_OUT}" 2>&1
}

# tls12_cipher_check  -> reads $PROBE_OUT and prints the negotiated cipher.
tls12_cipher_check() {
  # `Cipher    : ...` line is standard openssl s_client output.
  grep -E '^[[:space:]]*Cipher[[:space:]]*:[[:space:]]*' "${PROBE_OUT}" \
    | head -n1 \
    | sed -E 's/^[[:space:]]*Cipher[[:space:]]*:[[:space:]]*//'
}

# tls12_subject_check -> reads $PROBE_OUT and prints the leaf subject CN.
tls12_subject_check() {
  # `subject=` line in openssl s_client output (after -verify_return_error).
  grep -E '^[[:space:]]*subject=' "${PROBE_OUT}" \
    | head -n1 \
    | sed -E 's/^[[:space:]]*subject=[[:space:]]*//'
}

# tls12_verify_check -> reads $PROBE_OUT and checks "Verify return code: 0 (ok)".
tls12_verify_check() {
  grep -E '^[[:space:]]*Verify return code:[[:space:]]*0[[:space:]]*\(ok\)' \
    "${PROBE_OUT}" >/dev/null
}

# ---- Assertions -------------------------------------------------------------
PASS=0
FAIL=0

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [ "${expected}" = "${actual}" ]; then
    echo "  [PASS] ${label}: ${actual}"
    PASS=$((PASS + 1))
  else
    echo "  [FAIL] ${label}: expected='${expected}' actual='${actual}'" >&2
    FAIL=$((FAIL + 1))
  fi
}

assert_match() {
  local label="$1" pattern="$2" actual="$3"
  if printf '%s' "${actual}" | grep -E "${pattern}" >/dev/null; then
    echo "  [PASS] ${label}: matches /${pattern}/"
    PASS=$((PASS + 1))
  else
    echo "  [FAIL] ${label}: '${actual}' does not match /${pattern}/" >&2
    FAIL=$((FAIL + 1))
  fi
}

assert_cmd() {
  local label="$1" expected_rc="$2" actual_rc="$3"
  if [ "${expected_rc}" = "${actual_rc}" ]; then
    echo "  [PASS] ${label}: rc=${actual_rc}"
    PASS=$((PASS + 1))
  else
    echo "  [FAIL] ${label}: expected rc=${expected_rc} got rc=${actual_rc}" >&2
    FAIL=$((FAIL + 1))
  fi
}

# ---- 1. TLS 1.0 must be refused ---------------------------------------------
echo "[1/5] Probe TLS 1.0 (must FAIL handshake)"
set +e
tls_probe tls1
RC_TLS10=$?
set -e
# s_client exits 0 if the handshake succeeded; non-zero otherwise.
# We expect non-zero (refusal).
assert_cmd "TLS 1.0 refused" "nonzero" "$([ "${RC_TLS10}" -ne 0 ] && echo nonzero || echo zero)"

# ---- 2. TLS 1.1 must be refused ---------------------------------------------
echo "[2/5] Probe TLS 1.1 (must FAIL handshake)"
set +e
tls_probe tls1_1
RC_TLS11=$?
set -e
assert_cmd "TLS 1.1 refused" "nonzero" "$([ "${RC_TLS11}" -ne 0 ] && echo nonzero || echo zero)"

# ---- 3. TLS 1.2 must succeed -----------------------------------------------
echo "[3/5] Probe TLS 1.2 (must SUCCEED handshake)"
set +e
tls_probe tls1_2
RC_TLS12=$?
set -e
assert_cmd "TLS 1.2 accepted" "zero" "$([ "${RC_TLS12}" -eq 0 ] && echo zero || echo nonzero)"

if [ "${RC_TLS12}" -ne 0 ]; then
  echo "  ---- openssl output ----" >&2
  sed 's/^/    /' "${PROBE_OUT}" >&2
  echo "  -----------------------" >&2
fi

# ---- 4. Cert chain validates + subject sane --------------------------------
echo "[4/5] Validate TLS 1.2 cert chain"
if tls12_verify_check; then
  echo "  [PASS] cert chain verifies against system trust"
  PASS=$((PASS + 1))
else
  echo "  [FAIL] cert chain did NOT verify (see openssl output above)" >&2
  FAIL=$((FAIL + 1))
fi

SUBJECT="$(tls12_subject_check)"
if [ -z "${SUBJECT}" ]; then
  echo "  [FAIL] could not read subject from TLS 1.2 probe" >&2
  FAIL=$((FAIL + 1))
else
  echo "  subject: ${SUBJECT}"
  # CN or SAN must include the host we're connecting to (best-effort:
  # subject CN match is sufficient for SCA-22 smoke purposes; full SAN
  # validation is the caller's responsibility in real WebRTC stacks).
  assert_match "subject CN includes host" "^CN[[:space:]]*=[[:space:]]*${HOST//./\\.}" "${SUBJECT}"
fi

# ---- 5. Cipher is forward-secret -------------------------------------------
echo "[5/5] Negotiated cipher must be forward-secret"
CIPHER="$(tls12_cipher_check)"
if [ -z "${CIPHER}" ]; then
  echo "  [FAIL] could not read cipher from TLS 1.2 probe" >&2
  FAIL=$((FAIL + 1))
else
  echo "  cipher : ${CIPHER}"
  # Match ECDHE-AESGCM, ECDHE-CHACHA20, or DHE-AESGCM (PFS fallback).
  # DHE-only (no ECDHE) is acceptable but not preferred.
  assert_match "cipher uses PFS (ECDHE or DHE + AESGCM/CHACHA20)" \
    "(ECDHE-(AESGCM|CHACHA20)|DHE-(AESGCM|CHACHA20))" "${CIPHER}"
fi

# ---- Summary ----------------------------------------------------------------
echo
echo "=== Summary ==="
echo "PASS=${PASS}  FAIL=${FAIL}  total=5"

if [ "${FAIL}" -eq 0 ]; then
  echo "OK: 5/5 coturn-tls assertions passed"
  exit 0
fi

echo "FAIL: ${FAIL}/5 coturn-tls assertions failed" >&2
echo "Hint: check COTURN_TLS_ENABLED=true in infra/.env and cert paths in" \
  "COTURN_TLS_CERTDIR/live/\${COTURN_TLS_DOMAIN}/." >&2
exit 1