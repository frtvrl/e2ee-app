#!/usr/bin/env bash
# scripts/build-web.sh - Flutter web build wrapper (PR-27)
# ADR-0008 S2.4 - Cross-platform entry point
#
# Why this script exists
# ----------------------
# `flutter build web` defaults to `lib/main.dart`. OpenE2EE deliberately
# keeps a SEPARATE entry point for the web dashboard at `lib/web/main.dart`
# (see mobile/lib/web/main.dart for the rationale, HANDOFF S4.2 PR-11).
# Until the mobile-only `lib/main.dart` exists (future PR), running
# `flutter build web` without `--target` produces a confusing
# "Target of URI doesn't exist" error against `lib/main.dart`.
#
# This wrapper hard-codes the correct invocation:
#     flutter build web --target=lib/web/main.dart
# (run from inside the `mobile/` Flutter package)
#
# Usage:
#   bash scripts/build-web.sh
#   bash scripts/build-web.sh --release
#   bash scripts/build-web.sh --release --web-renderer canvaskit
#   bash scripts/build-web.sh --check          # CI: verify entry exists, don't build
#
# Equivalent to: make build-web  (cross-platform Make target)
#
# Mirrors scripts/build-web.ps1 (PowerShell fallback) - keep in sync.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
MOBILE_DIR="${REPO_ROOT}/mobile"
TARGET_DART="${MOBILE_DIR}/lib/web/main.dart"

# --- Parse wrapper flags; forward remaining args to flutter -----------------
# Usage: scripts/build-web.sh [--check] [--release] [--web-renderer <r>] ...
CHECK_ONLY=0
FORWARD_ARGS=()
for arg in "$@"; do
    case "${arg}" in
        --check)
            CHECK_ONLY=1
            ;;
        --help|-h)
            sed -n '2,26p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *)
            FORWARD_ARGS+=("${arg}")
            ;;
    esac
done

# --- Friendly pre-flight: ensure flutter project + web entry exist ---------
if [ ! -d "${MOBILE_DIR}" ]; then
    echo ""
    echo "ERROR: Flutter project not found at ${MOBILE_DIR}." >&2
    echo "       Did you clone the full repo?" >&2
    exit 1
fi

if [ ! -f "${TARGET_DART}" ]; then
    echo ""
    echo "ERROR: Flutter web entry not found at expected path:" >&2
    echo "       ${TARGET_DART}" >&2
    echo "" >&2
    echo "Why this happens:" >&2
    echo "  OpenE2EE uses a non-standard Flutter web entry point at" >&2
    echo "  mobile/lib/web/main.dart (PR-11 web dashboard). The default" >&2
    echo "  'flutter build web' targets mobile/lib/main.dart (the future" >&2
    echo "  mobile entry point, not yet in this checkout)." >&2
    echo "" >&2
    echo "How to fix (run from repo root):" >&2
    echo "  1. Restore the canonical web entry:" >&2
    echo "       git checkout -- mobile/lib/web/main.dart" >&2
    echo "  2. If the entry is missing because the Flutter package is uninitialised:" >&2
    echo "       cd mobile && flutter create .            # creates web/ + lib/" >&2
    echo "       flutter pub get                          # restore deps" >&2
    echo "       git checkout -- mobile/lib/web/main.dart # restore web entry" >&2
    echo "  3. If you intentionally moved the web entry, update the" >&2
    echo "     --target=<path> flag in this wrapper (see line ~88)." >&2
    echo "" >&2
    exit 1
fi

# --- Resolve flutter --------------------------------------------------------
if ! command -v flutter >/dev/null 2>&1; then
    echo "ERROR: flutter not found on PATH." >&2
    echo "       Install Flutter SDK: https://docs.flutter.dev/get-started/install" >&2
    exit 1
fi

# --- Header: Sprint 4 PR-27 context ----------------------------------------
if [ "${CHECK_ONLY}" -eq 1 ]; then
    echo "==> OpenE2EE Flutter web build (PR-27 wrapper) -- --check (CI preflight)"
    echo "    entry:   mobile/lib/web/main.dart (non-standard; see CONTRIBUTING.md)"
    echo "    OK:      target file exists, flutter on PATH."
    echo ""
    echo "==> Check passed. Skipping build (use without --check to actually build)."
    exit 0
fi

echo "==> OpenE2EE Flutter web build (PR-27 wrapper)"
echo "    entry:   mobile/lib/web/main.dart (non-standard; see CONTRIBUTING.md)"
echo "    args:    flutter build web --target=lib/web/main.dart ${FORWARD_ARGS[*]:-}"
echo "    output:  mobile/build/web"
echo ""

# --- Run --------------------------------------------------------------------
(
    cd "${MOBILE_DIR}"
    flutter build web --target=lib/web/main.dart "${FORWARD_ARGS[@]}"
)

echo ""
echo "==> Done. Output: mobile/build/web"
