# CI Matrix — Runner → Step Mapping

| Field      | Value                                                                       |
|------------|-----------------------------------------------------------------------------|
| **Status** | Accepted (Sprint 7 Item 5 — STRIDE-8-02)                                    |
| **Date**   | 2026-07-07                                                                  |
| **Owner**  | Architect (mvs_25a7a987f73243899e35a1485c6ba224)                            |
| **Source** | Hand-off from cyber-security sprint review. Anchored to current pin matrix in `.github/workflows/ci.yml` + `.github/workflows/ios.yml`. Cross-references: [`ADR-0008-multiplatform-tooling.md`](ADR-0008-multiplatform-tooling.md) §"Per-OS Matrix", [`MULTIPLATFORM.md`](MULTIPLATFORM.md) §"Windows", `SPRINT-3-PLAN-TEMPLATE.md` §"Integration Gate". |

> **Purpose.** Single source of truth for "what runs on which GitHub-hosted
> runner, and why every matrix leg — or its deliberate omission — is the way
> it is." If you add a new runner or change a matrix, update this doc in the
> same PR. The CI workflow comments reference this file via
> `docs/CI-MATRIX.md`.

---

## 1. Authority and precedence

This doc is **canonical for the CI runner/step mapping**. The matrix
comments inside `.github/workflows/*.yml` are the implementation; this
doc is the rationale. If they ever drift, the workflow is the source of
truth for *what runs today*, and this doc is the source of truth for
*why*.

The two must stay in lock-step; the matrix comments explicitly
cross-reference §"Windows docker skip rationale" below.

---

## 2. Runner matrix at a glance

| Runner           | Image                                                | Role                                                  |
|------------------|------------------------------------------------------|-------------------------------------------------------|
| `ubuntu-latest`  | GitHub-hosted Ubuntu 22.04 (Linux x86_64)            | **Canonical CI runner** — every PR must pass here    |
| `macos-latest`   | Apple silicon (M1) — macOS 14/15                     | Cross-OS Go + Flutter regression; iOS Xcode build     |
| `windows-latest` | GitHub-hosted Windows Server 2022                    | Cross-OS Go + Flutter regression; **no docker**      |

`ubuntu-latest` is the merge gate. A green build on Linux is necessary
and sufficient; the other two runners exist for early cross-OS
regression catching (the matrix's job is to confirm the suite PASSES on
all 3 runners with the same source — see ci.yml §"go-build-test" header).

---

## 3. Workflow → Runner → Step matrix

The canonical workflow files are:

- `.github/workflows/ci.yml` (multi-OS matrix + Linux-only docker + Flutter + schemas + docs)
- `.github/workflows/ios.yml` (iOS Xcode build, macOS-only — Sprint 7 Item 7 STRIDE-8-01)

The table below is exhaustive. Every row is a job; every cell explains
whether that job runs on a given runner, and if not, **why** with a
pointer to the inline `NOTICE — Sprint 7 STRIDE-8-02` comment.

Legend:

- ✓✓ = primary canonical CI runner (must-pass for merge)
- ✓ = runs in CI
- ✗ = intentionally skipped (rationale in §4)

### 3.1 `.github/workflows/ci.yml`

| Job ID                  | Job name                                | ubuntu-latest | macos-latest | windows-latest | Linux-only rationale?           |
|-------------------------|-----------------------------------------|:-------------:|:------------:|:--------------:|---------------------------------|
| `go-build-test`         | Go — vet + build + test                 | ✓✓            | ✓            | ✓              | No                              |
| `flutter-analyze-test`  | Flutter — analyze + test                | ✓✓            | ✓            | ✓              | No                              |
| `docker-compose-config` | Docker compose config                   | ✓             | ✓            | ✗              | **Yes — STRIDE-8-02 §4.1**      |
| `privacy-check`         | Privacy — KVKK DELETE spot-check        | ✓✓            | ✓            | ✓              | No                              |
| `backend-lint`          | Backend — go vet (legacy single-OS)     | ✓✓            | ✗            | ✗              | Not matrix-scoped — legacy      |
| `backend-test`          | Backend — go test (race + cover)        | ✓✓            | ✗            | ✗              | `-race` Linux-only (PR-31)      |
| `backend-govulncheck`   | Backend — govulncheck (PR-38)           | ✓✓            | ✗            | ✗              | govulncheck CLI on Linux only   |
| `go-test-race`          | Backend — race detection (PR-31)        | ✓✓            | ✗            | ✗              | `-race` Linux-only              |
| `backend-build`         | Backend — docker build                  | ✓✓            | ✗            | ✗              | **Yes — STRIDE-8-02 §4.2**      |
| `mobile-analyze`        | Mobile — flutter analyze (legacy)       | ✓✓            | ✗            | ✗              | Not matrix-scoped — legacy      |
| `mobile-test`           | Mobile — flutter test (legacy)          | ✓✓            | ✗            | ✗              | Not matrix-scoped — legacy      |
| `mobile-build-web`      | Mobile — flutter build web (legacy)     | ✓✓            | ✗            | ✗              | Not matrix-scoped — legacy      |
| `schemas-validate`      | Schemas — validate (JSON Schema)        | ✓✓            | ✗            | ✗              | Not matrix-scoped — Linux tools |
| `docs-lint`             | Docs — markdown lint                    | ✓✓            | ✗            | ✗              | Not matrix-scoped — Linux tools |

Total matrix fan-out: 11 legs (3+3+2+3) per workflow run, all under the
multi-OS matrix banner. The legacy single-OS jobs sit alongside the
matrix by design (Sprint 3 PR-MP-CI rationale: matrix is cross-OS
parity; legacy per-step gates are SHA-stable gates that pre-date the
matrix).

### 3.2 `.github/workflows/ios.yml` (Sprint 7 Item 7 — STRIDE-8-01)

| Job ID            | Job name                                       | ubuntu-latest | macos-latest | windows-latest |
|-------------------|------------------------------------------------|:-------------:|:------------:|:--------------:|
| `ios-build`       | iOS — Xcode + CocoaPods + xcodebuild test      | ✓ (static)    | ✓ (full)     | ✓ (static)     |

This workflow exists primarily for the `macos-latest` leg; the
`ubuntu-latest` and `windows-latest` legs are static-only (Podfile Ruby
syntax + Info.plist XML validation + project.pbxproj structural sanity).
All macOS-specific steps (xcodebuild) carry an
`if: matrix.os == 'macos-latest'` guard. Full matrix details live in
`.github/workflows/ios.yml` and the Item 7 PR description.

---

## 4. Windows docker skip rationale (STRIDE-8-02)

There are exactly **two** docker-touching jobs in the CI workflows.
Both are explicitly Windows-skipped. The skip is enforced at two layers:

1. **Workflow level** — `runs-on:` is pinned (no matrix) or the matrix
   omits `windows-latest` entirely.
2. **Comment level** — every skip is marked with an inline
   `NOTICE — Sprint 7 STRIDE-8-02 (Windows docker skip)` block that
   spells out (a) why, (b) where the equivalent coverage is, and
   (c) the manual verify path for Windows developers.

The two jobs are below.

### 4.1 `docker-compose-config` (ci.yml)

| Field                  | Value                                                     |
|------------------------|-----------------------------------------------------------|
| Matrix                 | `[ubuntu-latest, macos-latest]` (windows-latest omitted)  |
| Validates              | `infra/docker-compose.yml` syntax via `infra/scripts/validate_compose.py` (pure-Python, OS-agnostic) |
| Windows coverage?      | No — explicitly skipped                                   |
| Manual verify on Win?  | Yes — see §5.1                                            |

**Why Windows is skipped:**

1. **POSIX secret file paths.** `infra/docker-compose.yml` declares four
   secret `file:` references that are POSIX-style relative paths
   (e.g. `file: ../.secrets/postgres_password.txt` at L104-L110).
   These resolve correctly on Linux and macOS Docker runtimes but
   fail on Windows bash without explicit path translation. The
   production deployment target is Linux containers behind Kong/Nginx
   (see `infra/docker-compose.yml` services), so Windows-Docker parity
   has no operational value.
2. **YAML anchor semantics.** The compose file uses 12 YAML anchor
   references (`<<: *default-restart`, `logging: *default-logging`).
   These are well-formed and validated by PyYAML, but the canonical
   `docker compose config` runtime dereferences them inside a
   Linux-style filesystem layout — again, no Windows translation
   guarantees.
3. **Operational coverage is already complete.** `ubuntu-latest` +
   `macos-latest` cover both Linux and Unix-y container runtimes
   (macOS Docker Desktop uses the same Docker Compose v2 / BuildKit
   version as the production Linux host once you account for the
   xhyve VM, which is invisible to the YAML level).

### 4.2 `backend-build` (ci.yml)

| Field                  | Value                                                     |
|------------------------|-----------------------------------------------------------|
| `runs-on:`             | `ubuntu-latest` (single-OS, no matrix)                    |
| Validates              | `backend/Dockerfile` build via `docker/build-push-action@v6` with `cache-from: type=gha` |
| Windows coverage?      | No — single-OS Linux by design                            |
| Manual verify on Win?  | Yes — see §5.2                                            |

**Why this job is Linux-only (not matrix-scoped):**

1. **Production target is Linux.** The backend image is `debian-slim`
   only (no Windows- or macOS-specific stages). Building only on
   `ubuntu-latest` matches the production deployment target with no
   coverage loss.
2. **Docker Desktop differences are noise.** GitHub-hosted Windows
   runners run Docker Desktop inside a WSL2-backed VM with a
   different BuildKit cache and overlay-driver profile; macOS runners
   use Docker Desktop's xhyve-based VM with a separate cache scope.
   Building on either adds BuildKit-environment signal, not
   application signal.
3. **GHA cache determinism.** `cache-from: type=gha` is keyed by
   ref + runner. A multi-OS build matrix would produce 3× the cache
   storage cost without proportional signal.

### 4.3 What is NOT a Windows docker skip

To make the boundary explicit, the following are deliberately
**NOT** a Windows docker skip and run on all 3 runners:

- `go-build-test` — uses GitHub Actions `services:` to spin up
  `timescale/timescaledb-ha:pg16` and `redis:7-alpine`. These
  containers run inside GitHub's own runner VMs (Linux VM on macOS
  and Windows runners via the service container abstraction), not on
  the host's Docker Desktop — so Windows-Skip does not apply.
- `flutter-analyze-test` — no docker.
- `privacy-check` — no docker (pure Go test run).

If a future PR introduces a docker-touching step that needs to run on
Windows, **do not** add `windows-latest` to the existing
`docker-compose-config` or `backend-build` matrix without first
updating this document and verifying the Docker Desktop WSL2 / xhyve
limitations are no longer blockers.

---

## 5. Manual docker verify on Windows (developer escape hatch)

Per the §4 inline NOTICEs, Windows developers are NOT covered by CI
docker validation. They must run docker-compose validation locally
before opening a PR that touches `infra/docker-compose.yml`,
`infra/coturn/`, or any compose-volume path.

### 5.1 `docker-compose-config` equivalent

From a Windows host, run via **WSL 2** (NOT Git Bash, NOT cmd.exe):

```powershell
# In a Windows PowerShell with WSL 2 + Ubuntu installed
wsl --status                        # confirm WSL 2 default
wsl -d Ubuntu                       # enter the distro
cd /mnt/c/repos/e2ee-app            # or your repo path
python infra/scripts/validate_compose.py
# Cross-check: docker compose config --quiet (requires Docker Desktop WSL2 integration)
docker compose -f infra/docker-compose.yml config --quiet
```

Both invocations should exit 0. The `validate_compose.py` is the
hermetic floor (no Docker daemon needed); the `docker compose config`
is the canonical live check. See
[`MULTIPLATFORM.md`](MULTIPLATFORM.md) §2.2 for WSL 2 setup.

### 5.2 `backend-build` equivalent

```powershell
# WSL 2
cd /mnt/c/repos/e2ee-app/backend
docker buildx build \
  --tag opene2ee-backend:local \
  --cache-from type=gha \
  --cache-to type=gha,mode=max \
  --load .
docker run --rm opene2ee-backend:local /bin/echo OK
```

This mirrors what CI does on `ubuntu-latest` and gives a
Linux-equivalent verification path.

### 5.3 If you cannot install WSL 2

You are out of luck for docker validation. Options:

- **Pair with a Linux/macOS developer** for compose + build reviews
  (the reviewer runs `docker compose config --quiet` and reports).
- **Use a CI dry-run.** Open a draft PR; the CI matrix runs the
  validation for you. This is the cheapest path — recommended for
  first-time contributors.
- **Install PowerShell-7 + run the `pwsh` shim.** Does NOT help with
  docker; the docker daemon is a separate constraint.

---

## 6. Update protocol

When modifying the runner matrix, follow these rules. The Verifier §6
review will check them.

1. **Adding a runner.** Edit `.github/workflows/*.yml` matrix, then
   add a row to §3 and (if the new runner skips a job) document the
   skip in §4. Both files in the same PR.
2. **Removing a runner.** Same as adding — document the removal
   (e.g. "dropped windows-latest from `mobile-analyze-test` because
   ..."). NEVER remove without rationale.
3. **Adding a docker step.** Default to `ubuntu-latest` single-OS
   unless you can defend a multi-OS matrix in §4. If the step needs
   Windows, you MUST first update this doc and verify Docker Desktop
   WSL2/xhyve coverage.
4. **NOTICE comments are mandatory.** Every Windows skip must have an
   inline `NOTICE — Sprint 7 STRIDE-8-02 (Windows docker skip)` block
   that points to this doc. The block must explain (a) why, (b) where
   the equivalent coverage is, (c) the manual verify path.
5. **YAML validity.** `python -c "import yaml; yaml.safe_load(open(...))"`
   must exit 0 before commit. The 2-validator cross-check (Read tool
   + Python re-parse) is the project standard for any config or
   workflow file — see Sprint 7 Item 1 / Item 2 lessons.

---

## 7. References

- [`ADR-0008-multiplatform-tooling.md`](ADR-0008-multiplatform-tooling.md) §"Per-OS Matrix (Sprint 3 §5)" — older, broader trade-off matrix; this doc supersedes it for the **runner → step** mapping.
- [`MULTIPLATFORM.md`](MULTIPLATFORM.md) — contributor setup guide (Windows / macOS / Linux dev envs).
- [`NATIVE-DEV-SETUP.md`](NATIVE-DEV-SETUP.md) — Android Studio + Xcode dev tooling.
- [`SPRINT-3-PLAN-TEMPLATE.md`](SPRINT-3-PLAN-TEMPLATE.md) — Sprint 3+ plan + integration gate contract.
- `.github/workflows/ci.yml` — multi-OS matrix + Linux-only docker.
- `.github/workflows/ios.yml` — iOS Xcode build (Sprint 7 Item 7).
- `infra/scripts/validate_compose.py` — pure-Python compose validator used by `docker-compose-config` CI job.
- Docker Desktop on Windows limitations: <https://docs.docker.com/desktop/why/#limitations-of-docker-for-windows>

---

**History.**

- Sprint 3 PR-MP-CI: 11-leg multi-OS matrix introduced; Windows skipped
  in `docker-compose-config` (bash anchors + POSIX secret paths rationale
  inline).
- Sprint 5 PR-31: `-race` job added; Linux-only by design.
- Sprint 6 PR-38: `backend-govulncheck` job added; Linux-only by design.
- Sprint 7 Item 7 STRIDE-8-01: `.github/workflows/ios.yml` added;
  macos-latest runs full xcodebuild, ubuntu/windows run static checks.
- **Sprint 7 Item 5 STRIDE-8-02 (this doc):** explicit `NOTICE`
  comments added at each Windows-skip; `docs/CI-MATRIX.md` created as
  the authoritative runner → step mapping.