# OpenE2EE — Review Checklist (Long-Lived)

| Field      | Value                                                |
|------------|------------------------------------------------------|
| **ID**     | ADV-3 follow-up (Sprint 8 Item 3)                    |
| **Owner**  | Coder (mvs_922d854f06024acb813931d46323a2fc)        |
| **Source** | Sprint 2 hotfix integration gate §6 adversarial findings (`docs/SPRINT-2-HOTFIX-INTEGRATION-GATE.md`) + Sprint 7 verifier §6 carry-overs |
| **Status** | Living document — updated at the end of each Sprint |
| **Date**   | 2026-07-07                                           |
| **Base**   | `origin/main @ 0b669cc` (Sprint 7 merged)            |

> **Scope notice.** This is the **Sprint 8 ADV-3 follow-up** — the
> single source of truth for *"what review findings are still open
> vs closed"* across Sprints 2 → 7 → 8. It does NOT duplicate
> per-PR §6 review reports (those live in `~/.mavis/agents/verifier/workspace/reports/`)
> and does NOT replace the `RISK_MITIGATION_REPORT.md` (which is the
> high-level architectural risk register). This checklist is the
> *operational follow-up ledger* — every finding has a concrete Sprint
> or PR reference and a concrete close-out plan.

---

## 1. Source: Sprint 2 Hotfix Review Findings

The 4 adversarial findings raised by the Sprint 2 hotfix integration
gate verifier (mvs_e6984c48a20b4a4fb75da462ef3876d3) on 2026-07-06 are
the original "review ihlalleri" this checklist tracks. Verbatim from
`docs/SPRINT-2-HOTFIX-INTEGRATION-GATE.md` §6:

| ID   | Finding                                                                                          | Source PR | Severity (as raised) |
|------|--------------------------------------------------------------------------------------------------|-----------|----------------------|
| ADV-1 | Certbot path mismatch between `docker-compose.yml`, `nginx.conf`, `nginx/README.md`              | PR-13 (pre-existing) | Advisory (non-blocking) |
| ADV-2 | CORS `origins: "*"` is dev-friendly but must be tightened to explicit allowlist in prod         | PR-13     | Advisory (non-blocking) |
| ADV-3 | `kong.yml` ships only 3 plugins (rate-limit, CORS, bot-detection); JWT auth, request-transformer, prometheus missing | PR-13 | Advisory (non-blocking) |
| ADV-4 | `nginx.conf` HTTP→HTTPS redirect missing on port 80                                              | PR-13     | Advisory (non-blocking) |

---

## 2. Closure Status (post-Sprint 7)

| ID   | Status     | Closed by                                                                                       | Commit / PR                                          | Notes |
|------|------------|-------------------------------------------------------------------------------------------------|------------------------------------------------------|-------|
| ADV-1 | **CLOSED** | Sprint 3 PR-13 certbot path reconciliation                                                    | `2cbc7b3` (Sprint 3 ADV-1 fix) → `7076376` (merge)   | Docker-compose, nginx.conf, nginx/README.md now consistent on `/etc/letsencrypt/live/<domain>/` (nginx.conf lines 22–27 confirm `certbot_data:/etc/letsencrypt:ro` mount + `opene2ee.local/fullchain.pem` path). |
| ADV-2 | **CLOSED** | Sprint 3 Kong CORS hardening (separate from PR-32 JWT plugin and AUTHZ-2 /healthz JWT extension) | `51a4f6f` → `588676a` → `11ee73e` (ADV-2 commit chain) | Wildcard `"*"` replaced with explicit allowlist `https://app.opene2ee.com`, `https://staging.opene2ee.com`, `http://localhost:3000`, `http://localhost:8080`. `infra/kong/kong.yml` lines 110–114 still hold the Sprint 3 values on main. Note: dev-mode `localhost` is intentionally retained for `flutter run`. |
| ADV-3 | **PARTIAL** | JWT half closed across Sprint 5 PR-32 + Sprint 7 AUTHZ-2; `request-transformer` + `prometheus` plugins still missing (S7-OPEN-3 carry-over) | `fcfa107` (Sprint 5 JWT plugin) → `42f22be` (PR-32 merge) → `2c09635` (Sprint 7 AUTHZ-2 healthz fix) → `cd92c37` (AUTHZ-2 merge) | `kong.yml` now carries: (a) `jwt` plugin per-route for the protected subtree (lines 167–284), (b) HS256 `jwt_secrets` for `opene2ee-mobile` + `opene2ee-monitoring` consumers (lines 62–91), (c) `/healthz` JWT-protected route (lines 274–284). The two remaining gaps from the original ADV-3 finding — `request-transformer` and `prometheus` plugins — remain advisory and are tracked as S7-OPEN-3 in §3.2. |
| ADV-4 | **CLOSED** | Sprint 3 Nginx HTTPS redirect                                                                  | `da796b8` (Sprint 3 ADV-4 fix) → `971e219` (merge)   | Port 80 now serves `return 301 https://$host$request_uri;` (lines 14–18 in nginx.conf) — backend proxy is no longer reachable over plaintext HTTP. |

**Net effect on Sprint 2 hotfix findings (post-Sprint 7):**
- 3 of 4 fully closed (ADV-1 certbot path, ADV-2 CORS, ADV-4 HTTPS redirect) — all closed in **Sprint 3** (the original remediation Sprint), not Sprint 7.
- 1 of 4 partially closed (ADV-3 kong.yml plugins) — JWT half closed via Sprint 5 PR-32 + Sprint 7 AUTHZ-2; `request-transformer` + `prometheus` still missing → S7-OPEN-3 carry-over in §3.2.
- 0 carry-over from Sprint 2 itself (the Sprint 2 hotfix review's own gate already flagged ADV-1..ADV-4 as advisory non-blocking, and Sprint 3 closed all four within its own scope).

---

## 3. Open Items — Carry-Over to Sprint 9+

These items are **NOT** part of Sprint 8 scope. They are tracked here so
the Sprint 9 planner has a concrete starting list rather than re-deriving
it from `RETROSPECTIVE.md` (which is no longer maintained as a single
file — see §5 follow-ups).

### 3.1 Sprint 2 carry-over (0 items)

All Sprint 2 hotfix review findings (ADV-1..ADV-4) were closed in **Sprint 3**
within the original remediation Sprint's own scope. None carry over to
Sprint 9+ from the Sprint 2 hotfix review itself. (See §2 for closure
evidence.)

### 3.2 Sprint 7 verifier §6 carry-over (3 items)

These came out of Sprint 7 verification but were not closed by any Sprint
7 PR. They are non-blocking but worth tracking.

| ID            | Item                                                                                                | Source                                                                                  | Severity |
|---------------|-----------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------|----------|
| S7-OPEN-1     | Kong 3.8 EOL grandfather clause — bump PR to `kong:3.10-alpine` (LTS) within 14 days of KONG-UPGRADE-POLICY acceptance | `docs/policy/KONG-UPGRADE-POLICY.md` Grandfather Clause §                                | High     |
| S7-OPEN-2     | Coturn TLS `:!ECDSA` cipher-list consistency — verifier noted inconsistency vs declared TLS 1.2+ minimums in `infra/coturn/turnserver.conf` | Sprint 7 Item 3 SCA-22 verifier minor finding                                            | Low      |
| S7-OPEN-3     | ADV-3 PARTIAL — JWT closed (`fcfa107` / `42f22be` / `2c09635` / `cd92c37`); request-transformer + prometheus plugins still missing | ADV-3 partial close (§2 above)                                                            | Low      |

### 3.3 Cross-platform CI carry-over (1 item)

| ID            | Item                                                                                                | Source                                                                                  | Severity |
|---------------|-----------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------|----------|
| MP-CI-OPEN-1  | Multi-OS matrix `flutter build ios` not yet exercised on `macos-latest` for release config (only simulator build per Sprint 7 Item 7 STRIDE-8-01) | `docs/ADR-0008-multiplatform-tooling.md` §"Per-OS Matrix" + Sprint 7 Item 7               | Med      |

---

## 4. Cross-Links to Sprint 8 ADR Extensions

The Sprint 8 scope (`docs/SPRINT-8-SCOPE.md` Items 4–6) is **not** a
review-checklist follow-up — it is an architectural extension. The
cross-links below show how the open items in §3 relate to the ADR
extension work, so a verifier reading the Sprint 8 closure can trace
the dependency chain end-to-end.

| Open item          | ADR extension in Sprint 8                                       | Why the link matters                                                                                              |
|--------------------|-----------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------|
| S7-OPEN-1 (Kong EOL) | `docs/ADR-0003-vpn-layer.md` extension (Item 5)                                              | VPN layer upstream cipher posture (TLS 1.2+ minimum) sets the floor; Kong bump must clear same posture via SCA-22 coturn TLS precedent. |
| S7-OPEN-3 (Kong plugins) | `docs/ADR-0006-anonimlik.md` extension (Item 4, Anonymized Device Identity + Risk Register A1-G1 + KVKK + MaskIP) | The `prometheus` plugin enables per-`device_id_hash` request-rate metrics without breaking Anonim Cihaz Kimliği privacy contract (hashed only). |
| MP-CI-OPEN-1 (iOS release matrix) | `docs/ADR-0008-multiplatform-tooling.md` extension (Item 6)            | The per-OS matrix update is the canonical source for "what runs where" — the iOS release-build gap closes in the same PR. |

---

## 5. Sprint 8 Closure Mapping

When Sprint 8 closes, the verifier §6 should update this checklist as
follows:

1. **Sprint 2 carry-over (§3.1) is empty** — all ADV-1..ADV-4 already
   closed in Sprint 3 (full closure) or Sprint 5+7 (partial JWT for
   ADV-3). No §3.1 movement is expected unless Sprint 8 re-opens one
   of the ADV-* rows.
2. For each S7-OPEN-* row in §3.2: same logic as before — close in §2
   with the closing PR, or carry-over to §3.1 (re-tagged as `S8-OPEN-*`)
   with bumped target. Note S7-OPEN-3 (ADV-3 partial close for
   `request-transformer` + `prometheus`) is the highest-likelihood
   carry-over — Kong plugin additions usually need their own sprint.
3. For MP-CI-OPEN-1 in §3.3: tied to ADR-0008 extension; close in
   the same Sprint 8 PR if Sprint 8 Item 1 PR-MP-CI adds iOS release
   to the matrix, otherwise carry-over.
4. Update the header `Base` SHA to the post-Sprint 8 merge commit.

**Document discipline.** This checklist is the single source of
truth for "what review findings are still open". The following
documents are **out of scope** for this checklist:

* `docs/RISK_MITIGATION_REPORT.md` — architectural risk register
  (likelihood × impact × owner), not operational follow-ups.
* Per-PR §6 reports — `~/.mavis/agents/verifier/workspace/reports/<pr>-review-attempt-N.md`
  — granular per-PR evidence, not cross-sprint rollup.
* `docs/RETROSPECTIVE.md` — not maintained as a single file; per-Sprint
  retrospectives live in `docs/SPRINT-N-RETROSPECTIVE.md` if authored.

---

## 6. References

* `docs/SPRINT-2-HOTFIX-INTEGRATION-GATE.md` §6 — original 4 findings
  (ADV-1..ADV-4), non-blocking advisory register.
* `docs/SPRINT-2-HOTFIX-INFRA-CONFIG.md` — Architect spec for the
  4-file PR-13 follow-up.
* `docs/SPRINT-7-MERGE-XCHECK.md` — Sprint 7 merge cross-check
  (35 commits ahead of Sprint 6 base).
* `docs/SPRINT-8-SCOPE.md` Item 3 — this checklist's task brief.
* `docs/ADR-0006-anonimlik.md` — anonimlik / veri minimizasyonu ADR
  (Sprint 8 Item 4 extension).
* `docs/ADR-0003-vpn-layer.md` — VPN layer ADR (Sprint 8 Item 5
  extension).
* `docs/ADR-0008-multiplatform-tooling.md` — multiplatform tooling ADR
  (Sprint 8 Item 6 extension).
* `infra/kong/kong.yml` — current Kong JWT-enabled config (PR-32 +
  AUTHZ-2 /healthz extension).
* `infra/nginx/nginx.conf` — current nginx config (Sprint 3 ADV-4
  HTTPS redirect at lines 14-18).
* `docs/policy/KONG-UPGRADE-POLICY.md` — Kong cadence policy
  (S7-OPEN-1 grandfather clause).

---

**File path:** `C:\repos\e2ee-app\docs\REVIEW-CHECKLIST.md`
**Author:** Coder (mvs_922d854f06024acb813931d46323a2fc)
**Pattern:** Sprint 2 §6 register + Sprint 7 §6 carry-over + Sprint 8 ADR cross-links
**Push kararı:** §8 user/Architect onayı sonrası
**Amaç:** Sprint 9+ planner için somut carry-over listesi (review
ihlalleri tek dosyada toplanmış; ADV-1/ADV-2/ADV-4 Sprint 3'te
kapalı, ADV-3 partial; Sprint 7 carry-over S7-OPEN-1..3 + MP-CI-OPEN-1
Sprint 9+'a taşınır)