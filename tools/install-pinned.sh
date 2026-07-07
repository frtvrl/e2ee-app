#!/usr/bin/env bash
# tools/install-pinned.sh — Install (or refresh) the pinned third-party
# binaries declared in tools/PINS.toml.
#
# Sprint 7 / STRIDE-8-03 — Coder CI hand-off from cyber-security:
#
#   The cyber-security review flagged that the CI toolchain (jq, curl,
#   openssl, base64, sha256sum, etc.) is unpinned. A compromised
#   upstream package or a runner-image bump could silently change CI
#   behaviour with no PR review opportunity.
#
#   This script is the INSTALL half of the policy. It:
#     1. Reads tools/PINS.toml.
#     2. For each [[tool]] entry whose source = "github-release":
#        - downloads the artefact
#        - downloads the SHA256SUMS file
#        - verifies the SHA-256 matches the pinned value
#        - installs to tools/bin/<name>-<version>
#     3. For source = "apt" / "brew" / "go-install": emits the install
#        command (operator must run it; this script does NOT sudo by
#        default — see docs/CI-TOOLS.md §"Rotation procedure").
#     4. For source = "os-default": no-op (recorded for the audit trail).
#
# Usage:
#   bash tools/install-pinned.sh                      # install everything
#   bash tools/install-pinned.sh --only jq            # install one tool
#   bash tools/install-pinned.sh --dry-run            # print plan, no downloads
#
# Cross-platform: this is the bash twin. PowerShell equivalent is
# tools/install-pinned.ps1 (mirrors this script).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PINS_FILE="${REPO_ROOT}/tools/PINS.toml"
BIN_DIR="${REPO_ROOT}/tools/bin"

ONLY=""
DRY_RUN=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --only)
            ONLY="${2:-}"
            shift 2
            ;;
        --only=*)
            ONLY="${1#--only=}"
            shift
            ;;
        --dry-run)
            DRY_RUN=1
            shift
            ;;
        --help|-h)
            sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *)
            echo "ERROR: unknown argument: $1" >&2
            exit 64
            ;;
    esac
done

if [[ ! -f "${PINS_FILE}" ]]; then
    echo "ERROR: ${PINS_FILE} not found (run from repo root)." >&2
    exit 1
fi

mkdir -p "${BIN_DIR}"

# ----------------------------------------------------------------------------
# Tiny TOML parser — emits shell-sourceable variable assignments per block.
# ----------------------------------------------------------------------------
# We avoid a TOML library so this script runs in CI before any tooling is
# installed. Our schema is fixed ([[tool]] blocks, simple key = "value"
# lines), so awk is sufficient.
#
# Output format:
#   ---TOOL---
#   NAME='jq'
#   VERSION='1.7.1'
#   SOURCE='github-release'
#   ...
#   ---END---
# ----------------------------------------------------------------------------

PARSED=$(mktemp)
trap 'rm -f "${PARSED}" "${BIN_DIR}/.artefact.$$" "${BIN_DIR}/.sums.$$"' EXIT

awk '
    BEGIN { in_block = 0 }
    /^[[:space:]]*#/ { next }
    /^[[:space:]]*\[\[tool\]\]/ {
        if (in_block) print "---END---"
        in_block = 1
        print "---TOOL---"
        next
    }
    in_block && /^[[:space:]]*$/ { next }
    in_block && /^[[:space:]]*[a-z_]+[[:space:]]*=/ {
        # Extract key=value, strip quotes.
        k = $0
        sub(/[[:space:]]*=.*/, "", k)
        v = $0
        sub(/^[[:space:]]*[a-z_]+[[:space:]]*=[[:space:]]*"?/, "", v)
        sub(/"[[:space:]]*$/, "", v)
        # Uppercase key for shell variable convention.
        K = toupper(k)
        printf "%s=%c%s%c\n", K, 39, v, 39
        next
    }
    END {
        if (in_block) print "---END---"
    }
' "${PINS_FILE}" > "${PARSED}"

# ----------------------------------------------------------------------------
# Apply per-block.
# ----------------------------------------------------------------------------

install_github_release() {
    local name="$1" version="$2" upstream="$3" expected_sha="$4" sums_url="$5"
    local artefact="${BIN_DIR}/.${name}-${version}.artefact"
    local sums_file="${BIN_DIR}/.${name}-${version}.SHA256SUMS"

    if [[ "${DRY_RUN}" -eq 1 ]]; then
        echo "    [dry-run] would download ${upstream}"
        echo "    [dry-run] would verify SHA-256 = ${expected_sha:0:16}..."
        echo "    [dry-run] would install to ${BIN_DIR}/${name}"
        return 0
    fi

    if [[ -z "${expected_sha}" ]]; then
        echo "    [error] PINS.toml: ${name}@${version} has empty sha256 for source=github-release" >&2
        return 1
    fi

    if ! command -v curl >/dev/null 2>&1; then
        echo "    [error] curl not on PATH; cannot fetch ${upstream}" >&2
        return 1
    fi

    echo "    -> downloading $(basename "${upstream}")"
    if ! curl -fsSL "${upstream}" -o "${artefact}"; then
        echo "    [error] download failed for ${upstream}" >&2
        rm -f "${artefact}"
        return 1
    fi

    echo "    -> downloading SHA256SUMS"
    if ! curl -fsSL "${sums_url}" -o "${sums_file}"; then
        echo "    [warn] SHA256SUMS download failed (${sums_url}); relying on PINS.toml expected_sha only"
        rm -f "${sums_file}"
    fi

    echo "    -> verifying SHA-256"
    if [[ -f "${sums_file}" ]]; then
        local line upstream_sha
        line=$(grep -E "$(basename "${upstream}")|${version}" "${sums_file}" | head -n 1 || true)
        if [[ -n "${line}" ]]; then
            upstream_sha=$(echo "${line}" | awk '{print $1}')
            if [[ "${upstream_sha}" != "${expected_sha}" ]]; then
                echo "    [error] upstream SHA mismatch: expected ${expected_sha:0:16}..., got ${upstream_sha:0:16}..." >&2
                rm -f "${artefact}" "${sums_file}"
                return 1
            fi
            echo "    [OK] upstream SHA256SUMS verified"
        else
            echo "    [warn] no matching line in SHA256SUMS for $(basename "${upstream}")"
        fi
    fi

    local actual_sha
    actual_sha=$(sha256sum "${artefact}" | awk '{print $1}')
    if [[ "${actual_sha}" != "${expected_sha}" ]]; then
        echo "    [error] SHA-256 mismatch:" >&2
        echo "             expected: ${expected_sha}" >&2
        echo "             actual:   ${actual_sha}" >&2
        rm -f "${artefact}" "${sums_file}"
        return 1
    fi
    echo "    [OK] SHA-256 verified: ${actual_sha:0:16}..."

    local final="${BIN_DIR}/${name}"
    case "${upstream}" in
        *.tar.gz|*.tgz)
            local extract_dir="${BIN_DIR}/.extract-${name}-${version}"
            mkdir -p "${extract_dir}"
            tar -xzf "${artefact}" -C "${extract_dir}"
            local extracted_bin
            extracted_bin=$(find "${extract_dir}" -type f -name "${name}" -executable 2>/dev/null | head -n 1 || true)
            if [[ -z "${extracted_bin}" ]]; then
                echo "    [error] could not locate ${name} inside the tarball" >&2
                rm -rf "${artefact}" "${sums_file}" "${extract_dir}"
                return 1
            fi
            mv "${extracted_bin}" "${final}"
            chmod 0755 "${final}"
            rm -rf "${extract_dir}" "${sums_file}"
            ;;
        *)
            # Raw binary (e.g. jq-linux-amd64 from GitHub releases).
            mv "${artefact}" "${final}"
            chmod 0755 "${final}"
            rm -f "${sums_file}"
            ;;
    esac

    echo "    [installed] ${final}"
}

current_name=""
in_block=0

# Per-block variables (set by the source loop below).
NAME=""
VERSION=""
SOURCE=""
UPSTREAM=""
SHA256=""
SHA256_URL=""
NOTES=""

while IFS= read -r line; do
    if [[ "${line}" == "---TOOL---" ]]; then
        in_block=1
        NAME=""; VERSION=""; SOURCE=""; UPSTREAM=""; SHA256=""; SHA256_URL=""; NOTES=""
        continue
    fi
    if [[ "${line}" == "---END---" ]]; then
        in_block=0
        current_name="${NAME}"
        if [[ -z "${current_name}" ]]; then
            continue
        fi
        if [[ -n "${ONLY}" && "${ONLY}" != "${current_name}" ]]; then
            continue
        fi
        echo "==> ${current_name} ${VERSION} (source=${SOURCE})"
        case "${SOURCE}" in
            github-release)
                install_github_release "${current_name}" "${VERSION}" "${UPSTREAM}" "${SHA256}" "${SHA256_URL}"
                ;;
            apt)
                echo "    [manual] operator: sudo apt-get install -y ${current_name}=${VERSION}"
                echo "             (see docs/CI-TOOLS.md §Rotation procedure)"
                ;;
            brew)
                echo "    [manual] operator: brew install ${UPSTREAM}"
                echo "             (see docs/CI-TOOLS.md §Rotation procedure)"
                ;;
            go-install)
                echo "    [manual] operator: go install ${UPSTREAM}@${VERSION}"
                echo "             (see docs/CI-TOOLS.md §Rotation procedure)"
                ;;
            os-default)
                echo "    [skip] OS-default; integrity owned by the runner image"
                ;;
            *)
                echo "    [warn] unknown source '${SOURCE}' — skipping"
                ;;
        esac
        continue
    fi
    if [[ "${in_block}" -eq 1 ]]; then
        # Parse KEY='value' and assign.
        key="${line%%=*}"
        val="${line#*=}"
        # Strip surrounding single quotes.
        val="${val#\'}"
        val="${val%\'}"
        case "${key}" in
            NAME)      NAME="${val}" ;;
            VERSION)   VERSION="${val}" ;;
            SOURCE)    SOURCE="${val}" ;;
            UPSTREAM)  UPSTREAM="${val}" ;;
            SHA256)    SHA256="${val}" ;;
            SHA256_URL) SHA256_URL="${val}" ;;
            NOTES)     NOTES="${val}" ;;
        esac
    fi
done < "${PARSED}"

echo
echo "Done. Pinned binaries (if any) are under ${BIN_DIR}/."
echo "Run tools/ci-tools-pin-check.sh to verify everything still complies."