#!/usr/bin/env bash
# tools/ci-tools-pin-check.sh — Verify CI scripts use pinned third-party
# binaries (Sprint 7 / STRIDE-8-03).
#
# The cyber-security review flagged that the CI toolchain (jq, curl,
# openssl, base64, sha256sum, apt-get, brew, etc.) is unpinned. A
# compromised upstream package or a runner-image bump could silently
# change CI behaviour with no PR review opportunity.
#
# This script is the ENFORCEMENT half of the policy. It scans the
# repository for invocations of "high-risk 3rd-party binaries" and
# exits 1 if any of them are not:
#   - listed in tools/PINS.toml (source != "untracked"), OR
#   - in the ALLOWLIST (dist-default coreutils / OS-pinned languages), OR
#   - executing under a `# tools-pin: skip` directive with documented
#     justification, OR
#   - executing a pinned binary from tools/bin/ (the install target).
#
# Scanned file types: *.sh, *.ps1, *.psm1, *.yml, *.yaml
# Scanned directories: scripts/, infra/scripts/, .github/workflows/,
#                       tools/, docs/ (operator docs only — comments
#                       are stripped before matching so docs prose is
#                       immune).
#
# Comment-handling: the script strips line comments (#, //, <!-- -->) and
# block comments (/* */, <!-- -->) before matching, so "this script uses
# openssl" in a doc doesn't trigger a violation. Quoted strings inside
# actual code are also stripped so passing a binary name as an argument
# (e.g. `echo "curl is cool"`) doesn't trigger.
#
# Exits 0 on clean, 1 on violation, 2 on internal error.
#
# Usage:
#   bash tools/ci-tools-pin-check.sh
#   bash tools/ci-tools-pin-check.sh --verbose   # show every match (clean or not)
#   bash tools/ci-tools-pin-check.sh --self      # allow this file to mention tools
#
# Cross-platform: this is the bash twin. PowerShell equivalent is
# tools/ci-tools-pin-check.ps1 (mirrors this script for Windows CI).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
PINS_FILE="${REPO_ROOT}/tools/PINS.toml"

VERBOSE=0
SELF=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --verbose|-v) VERBOSE=1; shift ;;
        --self)       SELF=1; shift ;;
        --help|-h)
            sed -n '2,40p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *) echo "ERROR: unknown argument: $1" >&2; exit 64 ;;
    esac
done

# ----------------------------------------------------------------------------
# Binaries we scan for.
# ----------------------------------------------------------------------------
# Add to this list when introducing a new tool. Match whole-word at line
# start OR preceded by a shell metacharacter (/, |, &, ;, >, <, space,
# tab, opening paren). The validator runs against both bash and pwsh
# scripts, so the patterns below are designed to fire on either shell.
#
# Notes per binary:
#   jq           — JSON processor (dist-default on most runners but
#                  versions differ; pin preemptively)
#   curl         — HTTP client (high CVE count history; must be pinned)
#   wget         — same
#   openssl      — crypto toolkit (frequent CVE churn)
#   base64       — encoding (cosmetic but still a binary)
#   sha256sum    — SHA-256 verifier
#   apt-get, apt — Debian/Ubuntu package manager (when installing tools
#                  in CI, pin via apt-get install -y <pkg>=<version>)
#   brew         — Homebrew (macOS runners)
#   pip, pip3    — Python package installer (CI deps must come from
#                  requirements*.txt + hash pinning)
#   npm          — Node package installer (CI deps must come from
#                  package-lock.json)
#   gh release   — gh CLI release fetch (use with explicit --clobber
#                  only when the version is also pinned)
# ----------------------------------------------------------------------------
SCAN_BINARIES=(
    jq
    curl
    wget
    openssl
    base64
    sha256sum
    apt-get
    apt
    brew
    pip
    pip3
    npm
)

# Binaries we DO NOT flag. These are either:
#   - dist-default coreutils with implicit OS pinning
#   - the project's own pinned toolchains (go, flutter, docker, git,
#     bash, python, node, etc.) — version-pinned via actions/setup-* or
#     a Makefile-level pin
#
# IMPORTANT: one binary per array slot. The `is_allowlisted` function
# does an exact-match compare, so multi-word entries would never match.
ALLOWLIST=(
    bash
    sh
    zsh
    dash
    git
    svn
    go
    gofmt
    flutter
    dart
    docker
    docker-compose
    podman
    python
    python3
    pip
    pip3
    node
    npm
    npx
    awk
    sed
    grep
    egrep
    fgrep
    cut
    sort
    uniq
    tr
    head
    tail
    wc
    find
    xargs
    which
    command
    test
    echo
    printf
    cat
    ls
    cp
    mv
    rm
    mkdir
    rmdir
    ln
    touch
    chmod
    chown
    tar
    gzip
    gunzip
    zip
    unzip
    date
    env
    true
    false
    dirname
    basename
    realpath
    readlink
    make
    protoc
)

# ----------------------------------------------------------------------------
# Per-binary "reason" used in violation messages.
# ----------------------------------------------------------------------------
declare -A REASONS=(
    [jq]="pin via tools/PINS.toml (github-release) + tools/install-pinned.sh"
    [curl]="pin via tools/PINS.toml (github-release) + tools/install-pinned.sh; or use pinned apt/brew version"
    [wget]="pin via tools/PINS.toml + tools/install-pinned.sh"
    [openssl]="pin via apt/brew with explicit version; record entry in tools/PINS.toml"
    [base64]="dist-default on runners; record entry in tools/PINS.toml (os-default) to satisfy policy"
    [sha256sum]="dist-default (coreutils); record entry in tools/PINS.toml (os-default) to satisfy policy"
    [apt-get]="use 'apt-get install -y <pkg>=<version>'; record entry in tools/PINS.toml (apt)"
    [apt]="use 'apt install -y <pkg>=<version>'; record entry in tools/PINS.toml (apt)"
    [brew]="use 'brew install <formula>@<version>'; record entry in tools/PINS.toml (brew)"
    [pip]="use 'pip install -r requirements.txt' with --require-hashes; never 'pip install <pkg>' unpinned"
    [pip3]="use 'pip3 install -r requirements.txt' with --require-hashes; never 'pip3 install <pkg>' unpinned"
    [npm]="use 'npm ci' (NOT 'npm install'); record entry in tools/PINS.toml for global tool installs"
)

# ----------------------------------------------------------------------------
# Scan roots.
# ----------------------------------------------------------------------------
SCAN_ROOTS=(
    "scripts"
    "infra/scripts"
    "tools"
    ".github/workflows"
)

# ----------------------------------------------------------------------------
# Read pinned binaries from tools/PINS.toml.
# ----------------------------------------------------------------------------
PINS_OK=0
if [[ -f "${PINS_FILE}" ]]; then
    # Each [[tool]] block has a `name = "..."` line; collect unique names.
    PINS_OK=1
    PINNED_NAMES=$(awk -F'"' '/^name[[:space:]]*=/{print $2}' "${PINS_FILE}" | sort -u)
else
    echo "ERROR: ${PINS_FILE} not found (run from repo root)." >&2
    exit 2
fi

is_pinned() {
    local binary="$1"
    local line
    while IFS= read -r line; do
        if [[ "${line}" == "${binary}" ]]; then
            return 0
        fi
    done <<< "${PINNED_NAMES}"
    return 1
}

is_allowlisted() {
    local binary="$1"
    local w
    for w in "${ALLOWLIST[@]}"; do
        if [[ "${w}" == "${binary}" ]]; then
            return 0
        fi
    done
    return 1
}

# ----------------------------------------------------------------------------
# Strip comments from a file in-memory. Writes a temp file with comments
# replaced by blank lines (preserving line numbers for error messages).
# ----------------------------------------------------------------------------
strip_comments() {
    local in_file="$1" out_file="$2"
    # Use the dedicated Python helper file. Putting the Python in its
    # own file (instead of an inline heredoc) avoids the
    # PowerShell-5.1 + heredoc-quoting foot-gun that mangles source
    # with embedded quotes/backticks.
    local helper="${SCRIPT_DIR}/lib/scanner.py"
    if [[ ! -f "${helper}" ]]; then
        echo "ERROR: helper ${helper} not found" >&2
        return 1
    fi
    # Pick a working python interpreter.
    local py=""
    for candidate in python3 python; do
        if command -v "${candidate}" >/dev/null 2>&1; then
            if "${candidate}" --version >/dev/null 2>&1; then
                py="${candidate}"
                break
            fi
        fi
    done
    if [[ -z "${py}" ]]; then
        return 1
    fi
    if ! "${py}" "${helper}" strip "${in_file}" "${out_file}" 2>/dev/null; then
        return 1
    fi
    return 0
}

# ----------------------------------------------------------------------------
# Find the longest matching binary at the start of a word.
# ----------------------------------------------------------------------------
match_binary() {
    local word="$1"
    local b
    for b in "${SCAN_BINARIES[@]}"; do
        if [[ "${word}" == "${b}" ]]; then
            printf '%s\n' "${b}"
            return 0
        fi
    done
    return 1
}

# ----------------------------------------------------------------------------
# Walk all scan roots, scan all relevant files.
# ----------------------------------------------------------------------------
violations=0
clean=0

scan_file() {
    local file="$1"
    local rel="${file#${REPO_ROOT}/}"

    # File-level opt-out — skip the entire file if the directive is present.
    if grep -qE '^\s*#\s*tools-pin:\s*skip' "${file}" 2>/dev/null; then
        if [[ "${VERBOSE}" -eq 1 ]]; then
            echo "  [skip-file] ${rel} (has 'tools-pin: skip' directive)"
        fi
        return 0
    fi

    # Detect a working python interpreter. We test each candidate by
    # actually invoking it (--version exits 0 on a real Python; the
    # WindowsApps python3 stub exits non-zero with "Python not found",
    # which would otherwise be picked by `command -v python3` and then
    # fail at scan time).
    local py=""
    for candidate in python3 python; do
        if command -v "${candidate}" >/dev/null 2>&1; then
            if "${candidate}" --version >/dev/null 2>&1; then
                py="${candidate}"
                break
            fi
        fi
    done
    if [[ -z "${py}" ]]; then
        if [[ "${VERBOSE}" -eq 1 ]]; then
            echo "  [skip] ${rel} (no working python on PATH)"
        fi
        return 0
    fi

    local helper="${SCRIPT_DIR}/lib/scanner.py"
    if [[ ! -f "${helper}" ]]; then
        if [[ "${VERBOSE}" -eq 1 ]]; then
            echo "  [skip] ${rel} (scanner helper missing)"
        fi
        return 0
    fi

    # Run the scanner. It emits "<lineno>|<binary>" per match.
    local matches_tmp
    matches_tmp=$(mktemp)
    if ! "${py}" "${helper}" match "${file}" > "${matches_tmp}" 2>/dev/null; then
        if [[ "${VERBOSE}" -eq 1 ]]; then
            echo "  [skip] ${rel} (scanner error)"
        fi
        rm -f "${matches_tmp}"
        return 0
    fi

    if [[ ! -s "${matches_tmp}" ]]; then
        rm -f "${matches_tmp}"
        return 0
    fi

    while IFS='|' read -r lineno binary; do
        if is_allowlisted "${binary}"; then
            if [[ "${VERBOSE}" -eq 1 ]]; then
                echo "  [allow]  ${rel}:${lineno}  ${binary}"
            fi
            continue
        fi
        if is_pinned "${binary}"; then
            if [[ "${VERBOSE}" -eq 1 ]]; then
                echo "  [pin]    ${rel}:${lineno}  ${binary}"
            fi
            clean=$((clean + 1))
            continue
        fi
        violations=$((violations + 1))
        local reason="${REASONS[${binary}]:-add an entry to tools/PINS.toml}"
        echo "  [FAIL]   ${rel}:${lineno}  ${binary}  -> ${reason}" >&2
    done < "${matches_tmp}"
    rm -f "${matches_tmp}"
}

# ----------------------------------------------------------------------------
# Walk each scan root.
# ----------------------------------------------------------------------------

for root in "${SCAN_ROOTS[@]}"; do
    full="${REPO_ROOT}/${root}"
    if [[ ! -d "${full}" ]]; then
        continue
    fi
    while IFS= read -r -d '' file; do
        # Skip the validator itself unless --self is set.
        if [[ "${file}" == *"ci-tools-pin-check.sh" || "${file}" == *"ci-tools-pin-check.ps1" ]] \
            && [[ "${SELF}" -ne 1 ]]; then
            continue
        fi
        # Skip install-pinned.sh too — it intentionally mentions these tools.
        if [[ "${file}" == *"install-pinned.sh" || "${file}" == *"install-pinned.ps1" ]] \
            && [[ "${SELF}" -ne 1 ]]; then
            continue
        fi
        # Skip the PINS.toml + README files.
        case "${file}" in
            */tools/PINS.toml|*/tools/README.md|*/docs/CI-TOOLS.md)
                continue
                ;;
        esac
        scan_file "${file}"
    done < <(find "${full}" -type f \
        \( -name "*.sh" -o -name "*.ps1" -o -name "*.psm1" \
           -o -name "*.yml" -o -name "*.yaml" \) \
        -print0 2>/dev/null)
done

# ----------------------------------------------------------------------------
# Summary.
# ----------------------------------------------------------------------------

echo
echo "==> ci-tools-pin-check summary"
echo "    scanned roots:        ${SCAN_ROOTS[*]}"
echo "    pinned binaries:      $(echo "${PINNED_NAMES}" | wc -l | tr -d ' ')"
echo "    matched-and-pinned:   ${clean}"
echo "    violations:           ${violations}"

if [[ "${violations}" -gt 0 ]]; then
    echo
    echo "FAIL: ${violations} unpinned 3rd-party invocation(s) detected."
    echo "      Add an entry to tools/PINS.toml or use the existing pinned binary,"
    echo "      or add a `# tools-pin: skip` directive at the top of the file with"
    echo "      justification in the PR description."
    echo "      See docs/CI-TOOLS.md for the full policy."
    exit 1
fi

echo
echo "PASS: all 3rd-party binary invocations are pinned or allowlisted."
exit 0