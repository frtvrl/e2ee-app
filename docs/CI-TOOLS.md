# docs/CI-TOOLS.md — OpenE2EE CI Tool Version Pinning Policy

**Sprint 7 / STRIDE-8-03 — Coder CI hand-off from cyber-security**

| Field | Value |
|---|---|
| Owner | `coder` (CI gate), `backend` reviewers (PR review) |
| Lifecycle | `tools/PINS.toml` + `tools/install-pinned.{sh,ps1}` + `tools/ci-tools-pin-check.{sh,ps1}` |
| Cross-platform | ADR-0008 — bash on Linux/macOS, PowerShell on Windows |
| First shipped | Sprint 7 (this PR) |
| Cadence | Monthly minor (CVEs), quarterly major (upstream LTS), out-of-band on CVE ≥ 7.0 |
| Enforcement | `ci-tools-pin-check.sh` runs on every PR via `.github/workflows/ci.yml` |

---

## 1. Why this policy exists

The cyber-security review (Sprint 7) flagged that CI scripts in this
repository invoke third-party binaries — `jq`, `curl`, `openssl`,
`base64`, `sha256sum`, `apt-get`, `brew`, `pip`, `npm` — whose
versions are **not pinned**. Two failure modes:

1. **Compromised upstream package.** A malicious version of jq, curl, or
   openssl is uploaded to a package mirror. CI jobs run the malicious
   binary for weeks before anyone notices. There is no PR review
   opportunity because the version drift is automatic.

2. **Runner image bump.** GitHub Actions bumps `ubuntu-latest` and
   `macos-latest` periodically. A new `jq` lands. CI behaviour changes
   silently — JSON output ordering, error messages, regex flags. Flake
   rate climbs; nobody can point to a specific PR that broke things.

The fix is the same in both cases: **make the version a tracked
artefact** so a bump requires a PR.

## 2. The pinning mechanism

### 2.1 Manifest: `tools/PINS.toml`

`tools/PINS.toml` is the authoritative source of truth for which
tool versions are sanctioned. Every entry has:

- `name` — canonical binary name (lowercase)
- `version` — exact upstream version, X.Y.Z
- `source` — how the binary is acquired
  - `github-release` — download a release tarball/binary
  - `apt` — Debian/Ubuntu package, version-pinned
  - `brew` — Homebrew formula, version-pinned
  - `go-install` — `go install <module>@<version>`
  - `os-default` — already pinned by the runner image (recorded for
    the audit trail; no install step)
- `upstream` — where to fetch (URL or package name)
- `sha256` — hex-encoded SHA-256 of the install artefact
- `sha256_url` — URL to the upstream SHA-256SUMS file (proof-of-origin)
- `notes` — free-form caveats

### 2.2 Installer: `tools/install-pinned.{sh,ps1}`

The installer reads PINS.toml and applies each entry:

- For `github-release`: downloads the artefact + SHA-256SUMS, verifies
  against the upstream hashes AND the expected hash in PINS.toml,
  installs to `tools/bin/<name>`.
- For `apt` / `brew` / `go-install`: prints the operator command (this
  script does not sudo by default — see §5 *Rotation procedure*).
- For `os-default`: no-op (recorded for the audit trail).

### 2.3 Validator: `tools/ci-tools-pin-check.{sh,ps1}`

The validator runs on every PR via `.github/workflows/ci.yml`. It
scans `scripts/`, `infra/scripts/`, `tools/`, and
`.github/workflows/` for invocations of the 12 scanned binaries.

For each match, the match is **clean** if the binary is:

- **Allowlisted** — a dist-default OS tool whose integrity is owned by
  the runner image (`bash`, `grep`, `awk`, `sed`, `tar`, `git`, etc.)
- **Pinned** — present in `tools/PINS.toml` (regardless of source)
- **Skipped** — covered by a `# tools-pin: skip` directive at the top
  of the file with documented justification

Otherwise the validator exits 1 and the PR fails the CI gate.

## 3. Pinned tools

| Tool | Version | Source | First pinned | Last rotated |
|---|---|---|---|---|
| jq | 1.7.1 | github-release | 2026-07-07 | — |
| curl | 8.10.1 | github-release | 2026-07-07 | — |
| openssl | 3.3.2 | apt / brew | 2026-07-07 | — |
| sha256sum | coreutils 9.4 | os-default | 2026-07-07 | — |

This table must stay in sync with `tools/PINS.toml` — the validator
checks both for parity.

## 4. Pin cadence

### 4.1 Monthly minor (CVE-driven)

Each month, scan `tools/PINS.toml` against upstream CVE feeds:

- **jq**: <https://github.com/jqlang/jq/security/advisories>
- **curl**: <https://curl.se/docs/security.html>
- **openssl**: <https://www.openssl.org/news/secadv/>
- **GitHub Actions runners**: <https://github.com/actions/runner-images>

If a CVE ≥ 7.0 lands on a pinned version, bump to the next patched
release within 7 days. See §5 *Rotation procedure*.

### 4.2 Quarterly major (LTS-driven)

Each quarter (Mar / Jun / Sep / Dec), bump to the latest upstream
stable release and re-pin. Same procedure as the monthly CVE bump.
Track in git via `git tag -a tools-pin/<iso-date>`.

### 4.3 Out-of-band triggers

A bump MUST ship before the next sprint gate if any of the following:

- **CVE ≥ 7.0** on a pinned version
- **Upstream LTS EOL** (e.g. openssl 3.2.x EOL 2025-11-23)
- **CI regression** caused by the pinned version (test flake,
  parse-output change, etc.)
- **Deprecation** announced by upstream

For out-of-band bumps, the PR description MUST cite the trigger.

## 5. Rotation procedure

The rotation procedure follows the same discipline as the Kong upgrade
policy (`docs/policy/KONG-UPGRADE-POLICY.md`):

### 5.1 Detect (monthly + out-of-band)

The Sprint lead (or whoever picks up the cyber-security baton for
that sprint) opens a tracking issue:

> **Title:** `ci(tools): STRIDE-8-03 monthly rotation <ISO-month>`
>
> **Body:**
>   - CVE list since last rotation
>   - List of pinned versions to bump
>   - Test plan

### 5.2 Update PINS.toml + docs/CI-TOOLS.md (one PR)

In a single PR:

1. Update `tools/PINS.toml`:
   - New `version` for each [[tool]] entry being bumped
   - New `sha256` (compute via `sha256sum` of the downloaded artefact)
   - New `sha256_url` if upstream renamed the SUMS file
2. Update §3 *Pinned tools* table in this file with the new versions
   and the rotation date.
3. Add a `git tag -a tools-pin/<iso-date>` after merge (post-merge
   step, not part of the PR).
4. CI: `ci-tools-pin-check.sh` MUST pass before merge. The diff IS the
   audit trail.

### 5.3 Verify the install

On a clean checkout:

```bash
bash tools/install-pinned.sh --dry-run   # see what would download
bash tools/install-pinned.sh             # actually install
```

Each tool prints `[installed] tools/bin/<name>` on success. If any tool
prints `[error]`, the rotation is incomplete — DO NOT close the
tracking issue.

### 5.4 Cross-platform parity

The rotation MUST verify both bash and PowerShell paths:

```bash
bash tools/install-pinned.sh
pwsh tools/install-pinned.ps1
bash tools/ci-tools-pin-check.sh
pwsh tools/ci-tools-pin-check.ps1
```

All four MUST exit 0. If only one fails, the other is the source of
truth and the failing twin gets a fix-up PR.

### 5.5 Rollback

If the rotation introduces a regression:

1. `git revert` the PR (or `git tag -a tools-pin/<iso-date>` was not
   yet created, just reset).
2. File a follow-up issue with the regression details + the new SHA.
3. The CI gate will revert to the previous pinned versions
   automatically — `tools/PINS.toml` is the single source of truth.

## 6. Adversarial notes

### 6.1 What if the pinned SHA itself becomes compromised?

The validator accepts the SHA-256 from `tools/PINS.toml` as the source
of truth. If an attacker compromises both the GitHub release artefact
AND the SHA-256SUMS file (e.g. by stealing the upstream maintainer's
signing key), the SHA we pinned becomes the attacker's SHA.

Defence in depth:

1. **Cross-check against SLSA / sigstore.** A future Sprint should
   integrate `cosign verify-blob` against an upstream-signed
   attestation. Until then, this PR documents the gap.
2. **Multi-source verification.** For high-impact tools, verify the
   SHA-256 against a second mirror (e.g. curl from GitHub AND
   SourceForge).
3. **CVE-driven bumps.** Even if a SHA is malicious today, the
   monthly CVE-driven rotation (§4.1) limits the exposure window to
   30 days. The attacker must maintain compromise of the upstream
   release channel for the full 30 days to keep the poisoned binary
   in CI.

### 6.2 What about transitive tool dependencies?

The validator covers direct invocations (`jq`, `curl`, etc.). It does
NOT cover transitive deps installed via `pip install -r
requirements.txt`, `npm ci`, or `go mod download`. Those are
covered by separate gates:

- **pip**: `pip install --require-hashes` is the canonical
  approach. For OpenE2EE backend deps, `go.sum` is the equivalent
  (covered by `go mod download` + the `govulncheck` job in
  `.github/workflows/ci.yml`).
- **npm**: `npm ci` (NOT `npm install`) is the canonical approach.
- **Go modules**: `go.sum` is the SHA-256 chain. The
  `backend-govulncheck` job re-verifies on every PR.

This PR does not change those gates — it adds the missing layer
for shell-invoked tools.

### 6.3 What about Windows-specific tooling?

PowerShell-specific tools (e.g. `winget`, `choco`) are out of scope
for this PR. The validator already accepts `# tools-pin: skip` for
files that genuinely cannot be pinned. A future Sprint can extend
PINS.toml with `source = "winget"` / `source = "choco"` entries when
the project starts using them.

## 7. Cross-references

- **ADR-0008-multiplatform-tooling.md** — Cross-platform twin
  pattern (.sh + .ps1). The twin structure is mandatory.
- **docs/policy/KONG-UPGRADE-POLICY.md** — Sprint 7 SCA-19 cadence
  policy. The rotation procedure in §5 follows the same shape.
- **docs/SPRINT-6-PR-39-VERIFICATION.md** — Static-first CI
  precedent. `ci-tools-pin-check.sh` is a static gate (no Docker,
  no network); hermetic + fast.
- **.github/workflows/ci.yml** — Where the validator runs. See
  §8 below.

## 8. CI integration

`tools/ci-tools-pin-check.sh` is wired into `.github/workflows/ci.yml`
as a dedicated `ci-tools-pin-check` job:

```yaml
ci-tools-pin-check:
  name: CI tools — version pinning (STRIDE-8-03)
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v4
    - name: Run validator
      run: bash tools/ci-tools-pin-check.sh
    - name: Run validator (PowerShell twin)
      shell: pwsh
      run: pwsh tools/ci-tools-pin-check.ps1
```

The Linux job runs both the bash and PowerShell twins because
PowerShell is preinstalled on `ubuntu-latest` GitHub Actions runners.
This catches drift between the two implementations.

## 9. Acceptance criteria (Sprint 7 / STRIDE-8-03)

- [x] `tools/PINS.toml` exists with at least 4 entries (jq, curl,
      openssl, sha256sum).
- [x] `tools/ci-tools-pin-check.sh` exists and exits 1 on
      intentional violation.
- [x] `tools/ci-tools-pin-check.ps1` mirrors the bash validator.
- [x] `tools/install-pinned.{sh,ps1}` can install a pinned binary
      with SHA-256 verification.
- [x] `tools/README.md` documents the directory + how to add a new
      tool.
- [x] `tools/ci-tools-pin-check.sh` runs on every PR via
      `.github/workflows/ci.yml`.
- [x] `docs/CI-TOOLS.md` (this file) documents pin cadence,
      rotation procedure, and adversarial notes.
- [x] Bash + PowerShell twins stay in sync (CI runs both).