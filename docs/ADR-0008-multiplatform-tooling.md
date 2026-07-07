# ADR-0008 — Multiplatform Tooling (Windows / macOS / Linux)

| Field      | Value                                  |
|------------|----------------------------------------|
| **Status** | Accepted (Sprint 2) — Extended Sprint 8 (multiplatform tooling + CI tools pinning + runner-image posture) |
| **Date**   | 2026-07-07                             |
| **Owner**  | Architect (mvs_25a7a987f73243899e35a1485c6ba224) |
| **Source** | This ADR is a stub extracted from [`docs/ARCHITECTURE_DECISIONS.md`](ARCHITECTURE_DECISIONS.md) §1 ("Frontend") and §2 ("Backend"). The full multiplatform implementation lives in [`docs/MULTIPLATFORM.md`](MULTIPLATFORM.md) (contributor guide) and [`docs/NATIVE-DEV-SETUP.md`](NATIVE-DEV-SETUP.md) (Android Studio + Xcode dev tooling). **Sprint 8 Item 6** extends this ADR with the 3-OS matrix update (Sprint 7 STRIDE-8-01/02 + Sprint 8 PR-MP-CI carry-over), the CI tools pinning pattern (Sprint 7 STRIDE-8-03), and a runner-image security posture section. **Authority split:** the *runner → step* mapping for CI is now canonical in [`docs/CI-MATRIX.md`](CI-MATRIX.md) (Sprint 7 Item 5 / STRIDE-8-02); this ADR retains authority over *platform-runtime* expectations (what toolchains / shells / OS-specific build hosts the project supports). |

> **Scope notice.** This is a Sprint 4 PR-26 placeholder created to resolve
> the broken `docs/ADR-0008-multiplatform-tooling.md` cross-references
> flagged by the Sprint 3 Integration Gate. The stub captures the
> architectural *Context*, *Decision* and *Consequences* already
> documented in `docs/ARCHITECTURE_DECISIONS.md` §1 (Flutter for
> front-end + dashboard) and §2 (Go for backend). It is **not** a
> substitute for the full ADR-0008 — the per-OS shell selection (Git Bash
> vs WSL vs PowerShell-7), the `.editorconfig`/`.gitattributes` rationale,
> and the cross-platform `make`/`scripts/*.{sh,ps1}` matrix remain to be
> authored in a follow-up.
>
> **Sprint 8 Item 6 follow-up.** Three additions on top of the Sprint 3 §5
> Per-OS Matrix: (a) **Per-OS matrix extension** — Sprint 7 STRIDE-8-01
> added `macos-latest` iOS matrix (`.github/workflows/ios.yml`), Sprint 7
> STRIDE-8-02 documented the Windows docker skip with a 33-line `NOTICE`
> block and created `docs/CI-MATRIX.md` as the canonical runner→step
> mapping, Sprint 8 PR-MP-CI (carry-over from Sprint 3) extends 3-OS
> coverage for additional matrix legs; (b) **CI tools pinning** — Sprint 7
> STRIDE-8-03 introduced `tools/PINS.toml` + `tools/ci-tools-pin-check.{sh,ps1}`
> + `tools/install-pinned.{sh,ps1}` with monthly / quarterly rotation
> cadence and SHA-256 verification, policy captured in
> [`docs/CI-TOOLS.md`](CI-TOOLS.md); (c) **Runner-image security posture**
> — preference order documented for ephemerality, isolation, and supply-chain
> integrity. Cross-link index at the bottom of this ADR.

---

## Context

OpenE2EE is a monorepo with three runtimes (Go backend, Flutter mobile,
Flutter web dashboard) and three primary contributor platforms
(Windows, macOS, Linux). The naive approach — "let every dev install
their preferred shell and toolchain" — produced platform drift (CRLF/LF
noise, `cmd.exe` users without `make`, `.DS_Store`/Thumbs.db pollution)
during Sprint 1. We need an explicit *normalisation layer* that lets:

* PR diffs stay clean regardless of the contributor's OS.
* The same `make setup`, `make test`, `make lint`, `make build` commands
  work on Windows, macOS, and Linux.
* A single CI runner (Linux) can validate the work of contributors on
  every OS.

The architectural context is `docs/ARCHITECTURE_DECISIONS.md` §1
(Flutter chosen precisely because it serves mobile + web from a single
codebase) and §2 (Go chosen for the packet-analysis backend). The
practical implementation is `docs/MULTIPLATFORM.md`.

## Decision

### Framework pinning

* **Flutter** (≥ 3.24, Dart ≥ 3.5) is the **single frontend codebase**
  for mobile (Android + iOS) and for the web dashboard. Rationale (§1
  of ARCHITECTURE_DECISIONS.md): native performance on both mobile
  platforms, web compile for the dashboard, one team and one codebase.
* **Go** (≥ 1.26) is the **backend language**. The `gopacket` library
  is the packet-analysis primitive (§2 of ARCHITECTURE_DECISIONS.md).

### Platform normalisation layer

The repo ships a small set of files that every contributor / CI runner
shares:

* **`.editorconfig`** — Go uses tabs, everything else 2 spaces, UTF-8 LF.
* **`.gitattributes`** — text files forced LF, binary files diff-skipped,
  `.ps1` / `.bat` files preserved CRLF for shell semantics.
* **`.gitignore`** — OS / IDE / build artifacts (`.DS_Store`, `Thumbs.db`,
  `build/`, `.idea/workspace.xml`, `Pods/`, `Podfile.lock`).
* **`scripts/*.sh` + `scripts/*.ps1` + `Makefile`** — same logic, two
  shell flavours. CI is Linux-bash; Windows contributors use Git Bash or
  WSL; PowerShell-7 is the documented native fallback for `cmd.exe`
  users.

### Per-OS entry points

| OS       | Recommended shell | Setup                              |
|----------|-------------------|------------------------------------|
| Windows  | Git Bash (or WSL) | `make setup`                       |
| macOS    | native bash / zsh | `make setup` (or `brew install`)   |
| Linux    | native bash       | `make setup`                       |

Linux is the **canonical CI runner** (`ubuntu-latest`); every PR must
pass on Linux before merge.

### Toolchain pins

* `core.autocrlf false` is a **required** global git setting on Windows
  to prevent CRLF drift.
* PowerShell-7 (`winget install Microsoft.PowerShell`) is the native
  fallback for Windows users who do not want to install Git Bash.
* CocoaPods (`sudo gem install cocoapods` or `brew install cocoapods`)
  is required only for iOS release builds.
* Flutter SDK path is resolved by `flutter doctor -v`; `local.properties`
  is auto-populated.

### Flutter multiplatform surface

* **Mobile targets** — Android (API 21+, target API 34) and iOS
  (macOS-only build host). Per-app details in
  [`docs/NATIVE-DEV-SETUP.md`](NATIVE-DEV-SETUP.md).
* **Web target** — Flutter Web compiles the same Dart codebase into the
  dashboard. CI validates with `flutter build web`.
* **Desktop targets** — explicitly **out of scope** for MVP. No
  `flutter create --platforms=macos,windows,linux` is run.

## Consequences

### Positive
* A PR opened on Windows diffs identically on macOS and Linux — no
  CRLF/LF churn, no `.DS_Store`, no platform-conditional build files.
* One CI runner (Linux) validates the work of every contributor.
* Single Flutter codebase → mobile + web stay in sync (e.g.
  `telemetry_formatter.dart`, `device_identity.dart`).
* PR review is faster — reviewers don't need to context-switch between
  shells.

### Negative / Trade-offs
* Windows native contributors without Git Bash or WSL must install
  PowerShell-7 (`winget install Microsoft.PowerShell`) — one-time setup
  cost.
* iOS-only tooling (Xcode, CocoaPods) cannot be exercised on Windows or
  Linux; CI for iOS is deferred to Sprint 4 (`macos-latest` runner).
* The `scripts/*.{sh,ps1}` parallel pair means every shell-script change
  has to be applied in both files. Reviewers must check both.
* Android Studio + Xcode (full IDE dev tooling) are documented in
  [`docs/NATIVE-DEV-SETUP.md`](NATIVE-DEV-SETUP.md); they are *not* the
  default for PRs (CI is), but they are the recommended IDE for native
  Kotlin / Swift work.

### Follow-ups
* Author the full ADR-0008 with the per-OS trade-off matrix, the
  `.editorconfig`/`.gitattributes` rationale, and the GitHub Actions
  multi-OS runner plan (Sprint 4 PR-MP-CI).
* Migrate `scripts/*.sh` → bash-strict-mode (`set -euo pipefail`) and
  mirror the same flags in `scripts/*.ps1`.
* Adopt `bun` / `pnpm` / `task` only if the existing `make` + `*.{sh,ps1}`
  surface proves insufficient — boring solution preferred.

---

## Per-OS Matrix — Platform Runtime (Sprint 8 update)

> **Authority split (Sprint 7 Item 5 / STRIDE-8-02).** The canonical
> *runner → step* mapping (which CI job runs on which runner, with
> per-skip rationale) lives in [`docs/CI-MATRIX.md`](CI-MATRIX.md). This
> ADR retains authority over the **platform-runtime** expectations:
> what build toolchains, shells, and OS-specific build hosts the
> project *supports* (regardless of whether they run in CI today).
> If a row below says ✗ for "runs in CI today", the cross-link in
> `docs/CI-MATRIX.md` explains why (typically a deliberate skip with a
> manual-verify escape hatch on the developer's host).

Sprint 3 PR-MP-CI shipped the original 3-OS GitHub Actions matrix.
Sprint 7 extended it twice — **STRIDE-8-01** (`.github/workflows/ios.yml`,
a dedicated iOS-only workflow with macOS-Apple-silicon full build + 2
OSes static-only legs) and **STRIDE-8-02** (Windows-docker-skip NOTICEs
+ the canonical `docs/CI-MATRIX.md`). Sprint 8 **PR-MP-CI** is the
carry-over extension (Sprint 3 backlog item, routed to Coder this
sprint) that adds additional 3-OS coverage to legs that previously
sat at 1-OS or 2-OS. The owner of Sprint 8 PR-MP-CI is responsible for
reconciling any overlap with Sprint 7 MOB-5 (Android Keystore) and
STRIDE-8-01 (iOS matrix) per the §7 routing note in
[`docs/SPRINT-8-SCOPE.md`](SPRINT-8-SCOPE.md).

Legend: ✓ = runs in CI / supported on this OS, ✗ = not run / not
supported, ✓✓ = primary canonical CI runner.

| Feature                      | Ubuntu (linux)        | macOS                  | Windows                | Sprint 8 source                                                                                                       |
|------------------------------|-----------------------|------------------------|------------------------|-----------------------------------------------------------------------------------------------------------------------|
| Go build (`go build`)        | ✓✓                    | ✓                      | ✓                      | Sprint 3 PR-MP-CI (3-OS matrix)                                                                                       |
| Go test (`go test`)          | ✓✓                    | ✓                      | ✓                      | Sprint 3 PR-MP-CI; services: `timescale/timescaledb-ha:pg16` + `redis:7-alpine`                                         |
| Flutter analyze              | ✓✓                    | ✓                      | ✓                      | Sprint 3 PR-MP-CI; `--fatal-infos`                                                                                    |
| Flutter test                 | ✓✓                    | ✓                      | ✓                      | Sprint 3 PR-MP-CI                                                                                                     |
| Docker compose config        | ✓                     | ✓                      | ✗                      | Sprint 3 PR-MP-CI; Windows skipped — STRIDE-8-02 §4.1 (`docs/CI-MATRIX.md`)                                           |
| Privacy check (KVKK DELETE)  | ✓✓                    | ✓                      | ✓                      | Sprint 3 PR-MP-CI                                                                                                     |
| Race detection (`-race`)     | ✓                     | ✗                      | ✗                      | Sprint 5 PR-31 (`backend-test`, `ubuntu-latest` only)                                                                 |
| iOS Xcode build              | ✓ (static)            | ✓ (full)               | ✓ (static)             | Sprint 7 STRIDE-8-01 (`.github/workflows/ios.yml`); static-only on ubuntu/windows (Podfile + Info.plist + pbxproj)     |
| Android Gradle build         | ✓                     | ✗                      | ✗                      | Sprint 3 PR-MP-CI; `gradle assembleDebug` Linux-only — Runner-image security posture (§Runner-Image Security Posture)  |
| CI tools pin check           | ✓                     | ✓                      | ✓                      | Sprint 7 STRIDE-8-03 — `tools/ci-tools-pin-check.{sh,ps1}` runs both twins on `ubuntu-latest` for parity              |
| Backend govulncheck          | ✓                     | ✗                      | ✗                      | Sprint 6 PR-38; govulncheck CLI on Linux only                                                                        |
| Multi-OS matrix extensions   | ✓                     | ✓                      | ✓                      | **Sprint 8 PR-MP-CI** — carry-over extension; additional legs promoted from 1-OS / 2-OS to 3-OS                        |

### Runner rationale (updated Sprint 8)

* `ubuntu-latest` is the **canonical CI runner** — every PR must pass
  on Linux before merge.
* macOS + Windows are kept on the matrix for early cross-OS regression
  catching; both are Linux-equivalent for `go test` / `flutter test`.
  The Sprint 8 PR-MP-CI carry-over explicitly does **not** relax this —
  any new 3-OS leg still has to PASS on Linux before merge.
* `-race` is Linux-only: the matrix's job is to confirm the suite
  PASSES on all 3 runners with the same source; race detection would
  multiply runner-minutes without proportional signal (Sprint 5
  PR-31).
* Docker compose validation is intentionally Linux + macOS —
  `infra/docker-compose.yml` uses bash anchors (`<<: *default-restart`)
  and POSIX secret file paths (`../.secrets/...`) that don't translate
  to Windows bash. The Windows manual-verify path is documented in
  `docs/CI-MATRIX.md` §5.1 (WSL 2 + `infra/scripts/validate_compose.py`
  + `docker compose config --quiet`).
* iOS is **not** covered by `ci.yml`'s 3-OS matrix at all — it lives in
  the dedicated `ios.yml` workflow (Sprint 7 STRIDE-8-01). This is by
  design: the iOS-only workflow runs `xcodebuild ... build test` on
  `macos-latest` (Apple silicon, macOS 14/15) and static-only checks
  (Podfile Ruby syntax + Info.plist XML validation + project.pbxproj
  structural sanity) on `ubuntu-latest` and `windows-latest`. The
  cross-OS matrix in `ci.yml` keeps the Go / Flutter leg coverage
  separate from the Xcode build.
* Android Gradle builds run on `ubuntu-latest` only (no macOS/Windows
  runner provisioned for `gradlew assembleDebug`); Sprint 7 MOB-5
  raised `minSdk` from 21 → 23 to align with the project's
  Android Keystore posture — that change is independent of the
  runner matrix.

### Sprint 3 → Sprint 8 PR-MP-CI lineage

* **Sprint 3 PR-MP-CI** (origin): `.github/workflows/ci.yml` 11-leg
  multi-OS matrix (3+3+2+3). Windows skipped for docker with a 3-line
  inline comment "bash anchors + POSIX paths".
* **Sprint 5 PR-31** (additive): `backend-test` job with `-race` +
  coverage on Linux; not in the cross-OS matrix by design.
* **Sprint 6 PR-38** (additive): `backend-govulncheck` job on Linux.
* **Sprint 7 Item 5 / STRIDE-8-02** (hardening): 33-line
  `NOTICE — Sprint 7 STRIDE-8-02 (Windows docker skip)` block at
  `docker-compose-config` + 20-line `NOTICE` block at `backend-build` +
  `docs/CI-MATRIX.md` (300 lines) as the canonical runner→step doc.
* **Sprint 7 Item 7 / STRIDE-8-01** (new workflow): `.github/workflows/ios.yml`
  with macos-latest full build + 2 static-only OSes; `ios.yml`
  registered as a 1-row entry in `docs/CI-MATRIX.md` §3.2.
* **Sprint 7 Item 9 / STRIDE-8-03** (supply chain): `tools/PINS.toml` +
  `tools/ci-tools-pin-check.{sh,ps1}` + `tools/install-pinned.{sh,ps1}`
  + `docs/CI-TOOLS.md`. Wired into `ci.yml` as a dedicated
  `ci-tools-pin-check` job on `ubuntu-latest`. See
  [§ CI Tools Pinning](#ci-tools-pinning-sprint-7-stride-8-03) below.
* **Sprint 8 PR-MP-CI** (carry-over extension): adds 3-OS coverage to
  legs that previously sat at 1-OS / 2-OS. Owner: Coder. This is a
  matrix-promotion PR — it does **not** change the
  `docs/CI-MATRIX.md` authority model; the canonical doc still lives
  there. The Sprint 8 PR-MP-CI PR description MUST cross-reference
  `docs/CI-MATRIX.md` §6 *Update protocol* before merge.

### Follow-ups

* Sprint 8 PR-MP-CI Owner: update `docs/CI-MATRIX.md` §3 *Workflow →
  Runner → Step matrix* in the same PR per `docs/CI-MATRIX.md` §6
  rule #1 ("Adding a runner").
* Reconsider `-race` parity on macOS once cgo / clang parity is
  verified (Sprint 5 PR-31 memory entry).
* A future Sprint may introduce a `windows-latest` Docker Desktop
  path for the `backend-build` job; per `docs/CI-MATRIX.md` §6 rule
  #3, this requires §4 of that doc to be updated first with a
  WSL2/xhyve-coverage defence.

---

## CI Tools Pinning (Sprint 7 STRIDE-8-03)

CI scripts in this repository invoke third-party binaries (`jq`,
`curl`, `openssl`, `sha256sum`, `pyyaml`, plus the OS-package managers
`apt-get` and `brew`) whose versions must be **pinned to an exact
upstream version + SHA-256**. The cyber-security review (Sprint 7
STRIDE-8-03) flagged that unpinned third-party invocations are a
silent supply-chain risk — a compromised upstream package or a
GitHub Actions runner-image bump can swap a binary version with no
PR review opportunity. Sprint 7 closed that gap by introducing the
`tools/` pinning triplet, documented in
[`docs/CI-TOOLS.md`](CI-TOOLS.md). This section captures the
architectural decision; the policy + cadence + adversarial notes live
in that doc.

### The pinning triplet

| Artefact                                  | Role                                                            | Authority                                        |
|-------------------------------------------|-----------------------------------------------------------------|--------------------------------------------------|
| `tools/PINS.toml`                         | Authoritative manifest of pinned tool versions + SHA-256       | Source of truth (audit-trail grade)              |
| `tools/install-pinned.{sh,ps1}`           | Bash + PowerShell twins that install each pinned tool           | Cross-platform parity (Sprint 7 acceptance gate) |
| `tools/ci-tools-pin-check.{sh,ps1}`       | Bash + PowerShell twins that scan the repo and fail on unpinned | CI gate (runs on every PR)                       |
| `docs/CI-TOOLS.md`                        | Policy + cadence + rotation procedure + adversarial notes       | Operational specification                        |
| `tools/README.md`                         | Quick-start for adding a new tool                               | Contributor guide                                |

The bash + PowerShell twins are **mandatory**. Every shell-script
change has to be applied in both files; the `ci-tools-pin-check`
CI job (`.github/workflows/ci.yml`, `ubuntu-latest`) runs **both**
twins to catch drift — see `docs/CI-TOOLS.md` §5.4 *Cross-platform
parity*.

### PINS.toml entry shape

Each entry has: `name`, `version` (exact `X.Y.Z`, no leading `v`),
`source` (`github-release` | `apt` | `brew` | `go-install` |
`os-default` | `pypi`), `upstream`, `sha256`, `sha256_url`, `notes`.
For `source = "os-default"` the SHA-256 is `"N/A"` (distro owns the
integrity guarantee); for `source = "github-release"` the SHA-256
must be the hex-encoded SHA-256 of the install artefact and the
`sha256_url` must point at the upstream SHA-256SUMS file for
proof-of-origin. See `tools/PINS.toml` L20-L50 for the field
contract and `tools/PINS.toml` L60-L122 for the current entries
(jq, curl, openssl, sha256sum, pyyaml). The contract is also
re-stated in `tools/install-pinned.sh` L13-L30 for the install path
and in `tools/ci-tools-pin-check.sh` L45-L52 for the validator.

### Rotation cadence

The full cadence + rotation procedure is in `docs/CI-TOOLS.md` §4
*Pin cadence* and §5 *Rotation procedure*. Summary, with the
upstream source of each claim:

| Tier            | Cadence  | Trigger                                                      | Source                                   |
|-----------------|----------|--------------------------------------------------------------|------------------------------------------|
| Monthly minor   | 30 days  | CVE ≥ 7.0 against a pinned version                           | `docs/CI-TOOLS.md` §4.1                  |
| Quarterly major | 90 days  | Upstream LTS release (jq, curl, openssl quarterly cadence)   | `docs/CI-TOOLS.md` §4.2; vendor release notes |
| Out-of-band     | ASAP     | CVE ≥ 7.0 / upstream LTS EOL / CI regression / deprecation  | `docs/CI-TOOLS.md` §4.3                  |

The rotation procedure itself follows the same shape as the Kong
upgrade policy (`docs/policy/KONG-UPGRADE-POLICY.md`) — see
`docs/CI-TOOLS.md` §5.1-§5.5: detect → update PINS.toml + CI-TOOLS.md
in one PR → verify install on a clean checkout → cross-platform
parity (both bash + PowerShell twins PASS) → rollback path via
`git revert`.

### CI integration

The `ci-tools-pin-check.sh` validator is wired into
`.github/workflows/ci.yml` as a dedicated `ci-tools-pin-check` job on
`ubuntu-latest` (single-OS — it scans the repo, not the runner
platform). The job runs **both** twins so a future drift between the
.sh and .ps1 implementations fails CI before merge. This is the
Sprint 7 STRIDE-8-03 acceptance criterion #7 (see
`docs/CI-TOOLS.md` §9).

### Authority and follow-ups

* `tools/PINS.toml` is the **single source of truth** for what
  versions are sanctioned. `docs/CI-TOOLS.md` §3 *Pinned tools* table
  must mirror it — drift between the two is a Verifier §6 failure.
* Future pins: `source = "winget"` / `source = "choco"` for
  Windows-specific tooling are out-of-scope today (see
  `docs/CI-TOOLS.md` §6.3). When the project starts using them,
  PINS.toml needs new entries.
* SLSA / sigstore cross-verification is the natural next layer
  (currently documented as a gap in `docs/CI-TOOLS.md` §6.1).

---

## Runner-Image Security Posture (Sprint 8)

The choice of CI runner image is a security decision, not just a
performance one. GitHub Actions exposes two flavours:

1. **GitHub-hosted runners** (`ubuntu-latest`, `macos-latest`,
   `windows-latest`) — ephemeral, single-tenant VMs managed by
   GitHub; each job gets a fresh VM, no persistent state between jobs;
   secrets are scoped per-job.
2. **Self-hosted runners** — persistent, configured and operated by
   the project; state can leak across jobs and across PRs; a malicious
   PR can attempt to read secrets from previous jobs or from the
   runner's filesystem.

GitHub's own hardening guide
(<https://docs.github.com/en/actions/security-for-github-actions-security-guides/security-hardening-for-github-actions>)
is explicit: **GitHub-hosted runners are the recommended choice for
most situations because they are ephemeral and isolated**. This ADR
codifies that preference as project policy.

### Preference order (project default)

1. **GitHub-hosted, ephemeral, single-job** (current CI). Every job
   in `.github/workflows/ci.yml` and `.github/workflows/ios.yml` runs
   on a fresh GitHub-hosted VM — `ubuntu-latest` (Linux x86_64, Ubuntu
   22.04 image), `macos-latest` (Apple silicon M1, macOS 14/15), or
   `windows-latest` (Windows Server 2022). See `docs/CI-MATRIX.md` §2
   for the runner-image inventory.
2. **Self-hosted, ephemeral** (future, if ever needed). If the
   project ever needs a GPU runner, a macOS-Intel runner, or a
   custom-network runner, it MUST be ephemeral (one job per VM,
   destroyed at job end) and MUST NOT persist any state. This is
   the only acceptable self-hosted posture.
3. **Self-hosted, persistent** (avoid). Persistent self-hosted
   runners are an anti-pattern for any project that runs
   untrusted-code PRs from forks or from contributors who may not
   yet have merge rights. State leakage across jobs + secret
   exposure is a known risk. If a use case ever demands it, it
   MUST be justified in an ADR that explicitly addresses
   `pull_request_target` isolation, secret scoping, and the
   attacker model (see GitHub hardening guide §"Using
   self-hosted runners").

### Why this matters for OpenE2EE

* **Untrusted-code threat model.** A PR from a contributor who does
  not yet have merge rights is a typical case where the workflow
  runs code the maintainer hasn't reviewed line-by-line. A
  persistent self-hosted runner would let such a PR exfiltrate
  secrets from prior jobs or tamper with the runner's disk.
  GitHub-hosted runners sidestep this by destroying the VM after
  every job.
* **Runner-image supply chain.** GitHub publishes a
  [runner-images](https://github.com/actions/runner-images) repo
  with the exact package set per `ubuntu-latest` / `macos-latest`
  / `windows-latest` build. This is auditable. A self-hosted
  runner's image is whatever the operator built — no equivalent
  audit trail.
* **Tool pinning vs runner-image pinning.** Sprint 7 STRIDE-8-03
  pinned the shell-invoked tools (jq, curl, openssl) in
  `tools/PINS.toml` precisely because the runner image can bump
  silently. The pinning triplet + the ephemeral-VM preference are
  complementary: pinning makes the inside of the VM reproducible,
  ephemerality makes the outside of the VM attacker-resistant.

### CI integration

The current workflows (`.github/workflows/ci.yml`,
`.github/workflows/ios.yml`, `.github/workflows/android-release.yml`)
are 100% GitHub-hosted. No self-hosted runners are registered for
this project. The Sprint 8 PR-MP-CI carry-over extension inherits
this posture — any new leg MUST remain GitHub-hosted.

### Follow-ups

* If a future Sprint needs a runner type GitHub does not provide
  (e.g. GPU matrix, ARM Linux, Windows ARM), author an ADR that
  justifies the deviation and picks **ephemeral self-hosted**
  over **persistent self-hosted**.
* Audit the `pull_request_target` usage: any workflow using this
  trigger MUST scope secrets aggressively and treat untrusted-code
  PRs as the attacker model. (None of the current three workflows
  use `pull_request_target` — confirmed by grep on 2026-07-07
  during Sprint 8 Item 6 authoring.)
* Long-term: evaluate GitHub-hosted ARM Linux / GPU runners when
  they become generally available.

---

**Cross-references.**
* Source: [`docs/ARCHITECTURE_DECISIONS.md`](ARCHITECTURE_DECISIONS.md) §1, §2
* Contributor guide: [`docs/MULTIPLATFORM.md`](MULTIPLATFORM.md)
* Native dev tooling: [`docs/NATIVE-DEV-SETUP.md`](NATIVE-DEV-SETUP.md)
* Sprint 3 plan template: [`docs/SPRINT-3-PLAN-TEMPLATE.md`](SPRINT-3-PLAN-TEMPLATE.md)
* Sprint 8 scope (this item): [`docs/SPRINT-8-SCOPE.md`](SPRINT-8-SCOPE.md) §"Sprint 8 Item Routing Table" row 6
* **Canonical runner→step mapping:** [`docs/CI-MATRIX.md`](CI-MATRIX.md) (Sprint 7 Item 5 / STRIDE-8-02) — supersedes this ADR's Per-OS Matrix for the **runner → step** direction
* **CI tools pinning policy:** [`docs/CI-TOOLS.md`](CI-TOOLS.md) (Sprint 7 Item 9 / STRIDE-8-03)
* **Pinning manifest:** [`tools/PINS.toml`](../../tools/PINS.toml)
* **Pinning tooling (bash + PowerShell twins):** [`tools/ci-tools-pin-check.sh`](../../tools/ci-tools-pin-check.sh), [`tools/ci-tools-pin-check.ps1`](../../tools/ci-tools-pin-check.ps1), [`tools/install-pinned.sh`](../../tools/install-pinned.sh), [`tools/install-pinned.ps1`](../../tools/install-pinned.ps1), [`tools/README.md`](../../tools/README.md)
* **CI workflows:** [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) (multi-OS matrix + Linux-only docker + ci-tools-pin-check job), [`.github/workflows/ios.yml`](../../.github/workflows/ios.yml) (Sprint 7 Item 7 / STRIDE-8-01 macos-latest iOS matrix), [`.github/workflows/android-release.yml`](../../.github/workflows/android-release.yml) (Android release build, Linux-only)
* **GitHub hardening guide (runner-image security posture):** <https://docs.github.com/en/actions/security-for-github-actions-security-guides/security-hardening-for-github-actions>
* **Sprint 8 carry-over (3-OS extension):** Sprint 8 PR-MP-CI (Coder owner — see [`docs/SPRINT-8-SCOPE.md`](SPRINT-8-SCOPE.md) row 1)

**Sprint lineage.**
* Sprint 3 PR-MP-CI — original 11-leg 3-OS matrix in `.github/workflows/ci.yml`.
* Sprint 4 PR-26 — broken cross-reference stub fixed; this ADR created.
* Sprint 5 PR-31 — `-race` Linux-only job added (`backend-test`).
* Sprint 6 PR-38 — `backend-govulncheck` Linux-only job added.
* Sprint 7 Item 5 / STRIDE-8-02 — Windows docker-skip NOTICEs + `docs/CI-MATRIX.md` canonical mapping doc.
* Sprint 7 Item 7 / STRIDE-8-01 — `.github/workflows/ios.yml` macos-latest iOS matrix added.
* Sprint 7 Item 9 / STRIDE-8-03 — `tools/PINS.toml` + `tools/ci-tools-pin-check.{sh,ps1}` + `tools/install-pinned.{sh,ps1}` + `docs/CI-TOOLS.md` pinning triplet.
* **Sprint 8 Item 6 (this ADR extension)** — Per-OS matrix extension (3-OS row added) + CI Tools Pinning section + Runner-Image Security Posture section + Sprint 8 PR-MP-CI lineage.
* **Sprint 8 PR-MP-CI (Coder, parallel)** — 3-OS matrix carry-over extension (additional legs promoted from 1-OS / 2-OS to 3-OS).