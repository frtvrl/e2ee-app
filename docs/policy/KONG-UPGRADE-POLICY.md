# Kong Gateway Upgrade Cadence Policy

| Field      | Value                                                  |
|------------|--------------------------------------------------------|
| **ID**     | SCA-19                                                 |
| **Owner**  | Architect (mvs_25a7a987f73243899e35a1485c6ba224)      |
| **Source** | Sprint 7 carry-over from cyber-security focused mini-plan (Item 2 / SCA-19) |
| **Status** | Accepted (Sprint 7)                                    |
| **Date**   | 2026-07-07                                             |
| **Applies to** | `infra/docker-compose.yml` `kong` service        |

---

## Context

OpenE2EE uses **Kong Gateway 3.x (DB-less, declarative)** as the public-facing
reverse proxy / API gateway for the Go backend. The image pin lives in
`infra/docker-compose.yml` and the declarative config lives in
`infra/kong/kong.yml` (GitOps — every config change is a PR).

Kong is a security-critical surface: it terminates TLS, enforces JWT auth
(Sprint 5 PR-32), rate-limits, and exposes the admin API. Unlike long-lived
infrastructure components where "if it works, don't touch it" is acceptable,
a public-facing gateway must track upstream security patches on a
**predictable cadence** while still reacting fast to **out-of-band CVEs**.

This policy fixes the cadence, the triggers for unscheduled upgrades, the
rollback procedure, and the smoke-test gate that every Kong image bump must
clear before merge.

> **Cross-link.** This policy assumes the contributor runs on the normalised
> toolchain described in [`docs/ADR-0008-multiplatform-tooling.md`](ADR-0008-multiplatform-tooling.md)
> (`make setup`, `make test`, `docker compose config` on Linux / macOS;
> PowerShell-7 + Git Bash on Windows). Docker compose validation is
> Linux + macOS only per the ADR-0008 runner matrix — Windows contributors
> must rely on `make test` + CI's `docker-compose-config` leg.

---

## Current Pin (as of 2026-07-07)

```yaml
# infra/docker-compose.yml — kong service (line 285)
image: kong:3.8-alpine
```

* **Track:** Kong Gateway 3.x (DB-less mode).
* **Variant:** `alpine` (smaller surface, musl libc; matches our `redis:7-alpine` and `nginx:alpine` normalisation).
* **Source of truth:** the `image:` line in `infra/docker-compose.yml`. There
  is **no** Kong image in any other file — `infra/Dockerfile` does not exist
  for Kong, and CI does not reference Kong images directly.

Whenever this pin moves, this section must be updated as part of the same PR.

---

## Cadence

Kong Gateway releases minor versions roughly every 4–6 weeks. To keep us on
a supported security track without thrashing dev environments, we adopt a
**two-tier cadence**:

| Tier            | Cadence     | Target bump                                  |
|-----------------|-------------|----------------------------------------------|
| **Minor**       | **Monthly** | Next `kong:3.x.y-alpine` within 4–6 wk of upstream tag |
| **Major**       | **Quarterly** | Review `3.x` → `3.(x+1)` line at the start of each calendar quarter |

### Monthly minor

* Triggered by the upstream `kong/kong` GitHub release webhook (or a manual
  check on the first Monday of each month).
* Scope: **patch + minor bump inside the current major track** (e.g. `3.8.1` → `3.9.0`).
* No config changes assumed; a config diff is still required as evidence.
* SLA: PR opened within 5 business days of upstream tag.

### Quarterly major

* Triggered by the first Monday of January / April / July / October.
* Scope: review the **next major line** (e.g. `3.x` → `4.x` when available).
* Mandatory actions:
  1. Read upstream migration guide end-to-end.
  2. Diff `infra/kong/kong.yml` against the new major's plugin-schema changes
     (`/schemas` endpoint of the new Kong image — see test plan §3).
  3. Bump `image:` and run the full smoke-test gate (§3).
  4. Update this policy's "Current Pin" section.
* SLA: PR opened within 10 business days of the cadence date.

---

## Out-of-band Triggers (skip the cadence)

The following triggers authorise an **unscheduled** Kong upgrade. They are
ranked by urgency.

| # | Trigger                                                                 | SLA      | Required artefact                            |
|---|--------------------------------------------------------------------------|----------|----------------------------------------------|
| 1 | **CVE published** with CVSS ≥ 7.0 affecting our Kong minor line           | 48 h     | CVE-ID in commit message + `govulncheck` run |
| 2 | **Upstream LTS reached EOL** for our pinned minor line                    | 14 days  | Upstream EOL announcement URL in PR body    |
| 3 | **Breaking feature required** by a backend PR (e.g. new auth plugin)     | Same sprint as the feature | Plugin-name + docs link in PR body  |
| 4 | **Kong admin API / TLS regression** discovered in prod                   | 24 h     | Incident report reference in PR body        |

Triggers 1, 2, 4 override the cadence: do not wait for the next monthly /
quarterly window. Trigger 3 is normally folded into the feature sprint — it
does not authorise skipping the smoke-test gate.

---

## Rollback Procedure

Rollback is the **default response** to any smoke-test failure post-bump.
The compose file + GitOps declarative config together give us a sub-minute
recovery path.

1. **Revert the image pin** in `infra/docker-compose.yml`:
   ```bash
   git revert --no-edit <bump-commit-sha>
   # or, if the bump is the only unmerged PR:
   git checkout origin/main -- infra/docker-compose.yml infra/kong/kong.yml
   ```
2. **Re-pull and re-up**:
   ```bash
   docker compose -f infra/docker-compose.yml --env-file infra/.env pull kong
   docker compose -f infra/docker-compose.yml --env-file infra/.env up -d kong
   ```
3. **Confirm health**:
   ```bash
   docker compose -f infra/docker-compose.yml ps kong
   docker compose -f infra/docker-compose.yml exec kong kong health
   curl -fsS http://localhost:8100/status | jq .
   ```
4. **If the bump also touched `infra/kong/kong.yml`**: the revert must
   restore **both** files in lock-step. A partial revert leaves the gateway
   in a state where `kong reload` (admin API call) would silently drop the
   new plugin config — see §"Common rollback mistakes" below.
5. **Post-mortem** within 48 h of any rollback triggered by Triggers 1 or 4.

### Common rollback mistakes

* **Reverting only the compose file.** Kong declarative config is reloaded
  from `/etc/kong/kong.yml` on every container start (KONG_DECLARATIVE_CONFIG),
  so reverting only `infra/docker-compose.yml` without `infra/kong/kong.yml`
  keeps the new config in place once the old image comes up. **Always revert
  both files together.**
* **`kong reload` after partial rollback.** DB-less Kong ignores admin API
  reload — declarative config is read once at start. A reload call leaves
  you thinking you rolled back when you did not.
* **Skipping the healthcheck.** `kong health` is the only signal that
  confirms the running image matches the declared image. Never trust
  `docker ps` alone.

---

## Test Plan (per upgrade PR)

Every Kong image bump — cadence-driven or trigger-driven — must clear the
following gate before merge. The gate is **static-first** (matches Sprint 5
PR-32 precedent: docker not always available on the contributor's host;
CI is the canonical runner).

### 1. Compose config validation

```bash
docker compose -f infra/docker-compose.yml config --quiet
```

Expected: exit 0, no output. This catches YAML syntax errors and broken
`${VAR:?required}` expansions without starting any container.

### 2. Compose syntax (Linux / macOS only — per ADR-0008 runner matrix)

```bash
make test-compose
# or equivalently:
docker compose -f infra/docker-compose.yml config
```

Expected: parsed YAML printed; no anchor / `<<: *default-restart` errors.
**Windows contributors skip this step locally** — the CI
`docker-compose-config` job (Linux + macOS) is the canonical validation.

### 3. Schema sanity (Kong declarative config)

```bash
docker run --rm -v "$PWD/infra/kong:/etc/kong:ro" kong:<new-pin>-alpine \
  kong config -c /etc/kong/kong.yml parse
```

Expected: parses without schema-violation warnings. Catches plugin-name
typos and removed fields before we ever boot the gateway.

### 4. Smoke tests (compose up + curl)

```bash
docker compose -f infra/docker-compose.yml --env-file infra/.env up -d kong
docker compose -f infra/docker-compose.yml ps kong        # expect (healthy)
docker compose -f infra/docker-compose.yml exec kong kong health   # expect 200

# 4.1 — admin API reachable
curl -fsS http://localhost:8100/status | jq .

# 4.2 — proxy rejects unauthenticated /api/v1 request (Sprint 5 PR-32)
code=$(curl -s -o /dev/null -w '%{http_code}' \
  http://localhost:8000/api/v1/auth/whoami)
[ "$code" = "401" ] || { echo "FAIL: expected 401, got $code"; exit 1; }

# 4.3 — proxy allows /healthz route (Sprint 7 Item 1)
code=$(curl -s -o /dev/null -w '%{http_code}' http://localhost:8000/healthz)
[ "$code" = "200" ] || { echo "FAIL: expected 200, got $code"; exit 1; }

# 4.4 — JWT round-trip (HS256, matches infra/kong/kong.yml jwt plugin)
#        Use a short Python helper — see infra/kong/smoke-jwt.py
python3 infra/kong/smoke-jwt.py
```

Expected: 4.1 → 200; 4.2 → 401; 4.3 → 200; 4.4 → all assertions pass.
Any deviation = **FAIL** = trigger the rollback procedure.

### 5. CI gates

The PR must be green on:

* `docker-compose-config` (Linux + macOS matrix leg)
* `privacy-check` (KVKK DELETE smoke test — confirms Kong proxy still
  forwards DELETE correctly)
* `go-build-test` (backend healthcheck exercise via Kong)

If any of these turn red after the Kong bump, the PR is blocked until the
smoke test (step 4) is reproduced locally + a fix or revert lands.

---

## Cross-references

* [`docs/ADR-0008-multiplatform-tooling.md`](ADR-0008-multiplatform-tooling.md)
  — toolchain normalisation; explains why the test plan above is
  Linux / macOS for Docker validation and Windows + Linux + macOS for
  `go test` / `flutter test`.
* [`docs/ARCHITECTURE_DECISIONS.md`](ARCHITECTURE_DECISIONS.md) — Kong is
  the §"Kong Konfigürasyonu" decision (DB-less, declarative, GitOps).
* `infra/docker-compose.yml` — `kong` service, line ~285 (`image: kong:3.8-alpine`).
* `infra/kong/kong.yml` — declarative config; **must** be reverted in lock-step
  with the compose file on any rollback (§"Common rollback mistakes").
* [`docs/SPRINT-6-PR-39-VERIFICATION.md`](SPRINT-6-PR-39-VERIFICATION.md)
  — the precedent for static-first verification when Docker is unavailable
  on the contributor host.

---

## Change log

| Date       | Bump (old → new)              | Type  | Author / PR          |
|------------|--------------------------------|-------|----------------------|
| 2026-07-07 | (policy authored — no bump)    | docs  | Architect / SCA-19    |