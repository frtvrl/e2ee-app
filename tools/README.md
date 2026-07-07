# tools/ — OpenE2EE CI tool pinning (Sprint 7 / STRIDE-8-03)
#
# Hand-off from the cyber-security review: CI shell scripts invoked
# third-party binaries (jq, curl, openssl, base64, sha256sum, etc.)
# without version pinning. A compromised upstream package or a runner
# image bump could silently change CI behaviour with no PR review
# opportunity.
#
# This directory ships the policy + tooling that closes that gap.
#
# ----------------------------------------------------------------------------
# Files
# ----------------------------------------------------------------------------
#
#   PINS.toml               Central manifest of pinned tools: name,
#                           version, source (github-release / apt / brew
#                           / go-install / os-default), SHA-256, and the
#                           upstream SHA-256SUMS URL. This is the
#                           authoritative source of truth.
#
#   install-pinned.sh       Linux/macOS installer. Reads PINS.toml,
#                           downloads each github-release artefact,
#                           verifies the SHA-256 against both the
#                           upstream SHA256SUMS and the expected value
#                           in PINS.toml, and installs to bin/.
#
#   install-pinned.ps1      Windows twin (ADR-0008 cross-platform).
#
#   ci-tools-pin-check.sh   The CI gate. Scans scripts/, infra/scripts/,
#                           tools/, .github/workflows/ for invocations
#                           of jq/curl/openssl/base64/sha256sum/apt-get/
#                           brew/pip/pip3/npm. Exits 1 if any of them
#                           are not:
#                             - allowlisted (dist-default OS tool)
#                             - pinned in PINS.toml
#                             - covered by a `# tools-pin: skip`
#                               directive with documented justification
#
#   ci-tools-pin-check.ps1  Windows twin (ADR-0008 cross-platform).
#
#   bin/                    Install target for github-release artefacts.
#                           Created empty; populated by install-pinned.
#
# ----------------------------------------------------------------------------
# Adding a new tool to the policy
# ----------------------------------------------------------------------------
#
# 1. Open tools/PINS.toml. Append a new [[tool]] block:
#
#      [[tool]]
#      name = "yourtool"
#      version = "X.Y.Z"
#      source = "github-release"        # or apt / brew / go-install / os-default
#      upstream = "https://..."
#      sha256 = "<hex>"
#      sha256_url = "https://.../SHA256SUMS"
#      notes = "..."
#
# 2. Update docs/CI-TOOLS.md §"Pinned tools" with the same row.
#
# 3. Run `bash tools/ci-tools-pin-check.sh --verbose` to confirm.
#
# 4. Run `bash tools/install-pinned.sh --only yourtool` to verify the
#    installer downloads + verifies correctly.
#
# 5. Commit + open PR. The diff IS the audit trail.
#
# ----------------------------------------------------------------------------
# Bypassing the gate (rare)
# ----------------------------------------------------------------------------
#
# If a tool genuinely cannot be pinned (e.g. an OS-only binary that
# varies by distro), put `# tools-pin: skip` at the top of the file
# and document the rationale in the PR description. The validator will
# skip the file and the PR review becomes the audit trail.
#
# ----------------------------------------------------------------------------
# See also
# ----------------------------------------------------------------------------
#
#   docs/CI-TOOLS.md        Full policy: pin cadence, rotation procedure,
#                           adversarial notes, cross-references to
#                           SPRINT-6-PR-39-VERIFICATION (static-first CI
#                           precedent) and KONG-UPGRADE-POLICY.md
#                           (SCA-19 cadence policy).
#
#   docs/ADR-0008-multiplatform-tooling.md  Why the bash + PowerShell
#                           twin pattern is mandatory.