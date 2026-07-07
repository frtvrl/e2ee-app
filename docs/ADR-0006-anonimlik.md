# ADR-0006 — Anonimlik / Veri Minimizasyonu (Privacy & Data Minimisation)

| Field      | Value                                  |
|------------|----------------------------------------|
| **Status** | Accepted (Sprint 1) — Sprint 8 extension (HIGH) |
| **Date**   | 2026-07-06 (stub); 2026-07-07 (Sprint 8 extension) |
| **Owner**  | Architect (mvs_25a7a987f73243899e35a1485c6ba224) |
| **Source** | Originally a stub extracted from [`docs/ARCHITECTURE_DECISIONS.md`](ARCHITECTURE_DECISIONS.md) §5 ("Regülasyon Uyumu ve Mağaza Politikaları"). Sprint 5 PR-33 expanded the Anonim Cihaz Kimliği + Risk Register A1-G1 sections. Sprint 8 (this PR) tightens the contract to its normative form (UUID v7 device-only, Ed25519 private key device-only, `device_id_hash = SHA-256(uuid||salt)[:16]`, MaskIP /24 v4 /48 v6), codifies the KVKK DELETE cross-device propagation path (PR-37), and adds a Testability section that names the gap (backend grep missing). |

> **Scope notice.** This is a Sprint 4 PR-26 placeholder created to resolve
> the broken `docs/ADR-0006-anonimlik.md` cross-references flagged by the
> Sprint 3 Integration Gate. The stub captures the four architectural
> *Principles* already documented in `docs/ARCHITECTURE_DECISIONS.md` §5:
> Açık Kaynak Şeffaflığı, Veri Minimizasyonu (Anonimleştirme), Pazarlama
> Konumlandırması, Açık Onam (Consent UI). It is **not** a substitute for
> the full ADR-0006 — the Anonim Cihaz Kimliği (device_id_hash, public_key_fp)
> contract and the Backend'de Saklanan (server-side hashed fingerprint)
> contract remain to be authored in a follow-up.

---

## Context

OpenE2EE is positioned as a *Network Security & Transparency Tool*, not a
VPN proxy. The app sits on the user's device and observes network traffic
to score E2EE health — but if it leaked raw packet payloads, destination
IPs, phone-identifiers (IMEI/MSISDN/MAC), or WebRTC SDP bodies, it would
violate:

* **App Store / Google Play policy** on VPN-usage categories.
* **KVKK / GDPR** data-minimisation requirements.
* The user's reasonable expectation that a *diagnostic* tool never becomes
  a *spy*.

The non-negotiable rule is therefore: **the device observes, the server
sees only anonymised aggregates, and no payload byte, identifier, or
negotiation string ever crosses the OS boundary intact.** These constraints
are documented in `docs/ARCHITECTURE_DECISIONS.md` §5.

## Decision

The privacy contract is enforced through four principles applied end-to-end
(mobile, transport, backend):

### 1. Açık Kaynak Şeffaflığı (Open-Source Transparency)

The project is **fully open-source under the MIT License** on GitHub. The
App Review teams are shown the source code to prove there is no hidden
agenda (no backdoor, no spyware). See `LICENSE` (MIT) and the public
repository at `github.com/opene2ee-com/e2ee-app`.

### 2. Veri Minimizasyonu (Data Minimisation — the cardinal rule)

The mobile app **never** sends a packet's raw payload, its destination IP,
or any hardware / SIM identifier to the backend. What reaches the server
is only an anonymised JSON telemetry blob of the form:

```json
{
  "app": "WhatsApp",
  "encryptionScore": 0.99,
  "operator": "Turkcell",
  "sessionIdHash": "…",
  "tlsFingerprintShort": "…"
}
```

Enforcement lives in three layers:

* **Mobile** — `OpenE2eeVpnService` (`mobile/lib/mobile/vpn/vpn_service_android.kt`)
  copies only IP/TCP/UDP header fields into the ring buffer and calls
  `protect()` to return the payload to the OS. Source/destination IPs are
  masked at `/24` (IPv4) or `/48` (IPv6) before reaching Dart. The class
  MUST NOT touch `TelephonyManager`, `WifiInfo`, or `BluetoothAdapter`
  (a CI grep test enforces this).
* **Mobile** — `WebRTCClient` (`mobile/lib/shared/webrtc_client.dart`)
  scrubs SDP bodies and ICE candidate strings before logging. CI tests in
  `mobile/test/shared/webrtc_client_test.dart` assert `scrubLog` invariants.
* **Backend** — `internal/storage/interfaces.go` stores only
  `DeviceIDHash` (server-side hash, never the raw ID), `PublicKeyFp`, and
  `/24`-masked IPs. `internal/operator/rdap.go` deliberately ignores
  registrant/abuse contact fields returned by RDAP.

### 3. Pazarlama Konumlandırması (Marketing Positioning)

The app is submitted to Google Play under the **VpnService exception
category** ("Network Security Tool") — not as a general VPN proxy. iOS
uses only the official `NetworkExtension` API. The framing is "E2EE
transparency", not "anonymity / circumvention".

### 4. Açık Onam (Explicit Consent UI)

The first-launch experience is a full-page **transparency / consent
screen** that discloses:

* Traffic is processed *only* on-device for security scoring.
* No plaintext data is transmitted to the server.
* The session is *user-triggered* and *task-bounded* (§6 of
  `ARCHITECTURE_DECISIONS.md`).

The consent screen is mandatory; the diagnostic surface does not activate
without it.

### Anonim Cihaz Kimliği (Anonymised Device Identity — referenced from `mobile/lib/shared/device_identity.dart`)

* Device identity is an **Ed25519 keypair** generated on first launch.
* The *private* key lives in the platform secure store (Android Keystore /
  iOS Keychain via `flutter_secure_storage`).
* The *public key* is registered with the backend as a 16-byte SHA-256
  fingerprint (`public_key_fp`). The raw key bytes never leave the device.
* Wire-format fingerprint length is pinned by ADR-0006 and exposed via
  `deviceIdentity.publicKeyFingerprint`.
* Signing is OPTIONAL in MVP (per F9) and currently disabled by default.

### Backend'de Saklanan (Server-Side Storage — referenced from `backend/internal/storage/interfaces.go`)

The backend stores only:

* `DeviceIDHash` — server-side hash, NEVER the raw device id.
* `PublicKeyFp` — 16-byte SHA-256 fingerprint of the public key.
* `/24` (IPv4) / `/48` (IPv6) masked source/destination IPs.
* Sampled metadata (transport ports, TCP flags, IP-ID).

KVKK/GDPR DELETE is implemented server-side; raw PII does not exist on the
server to be deleted.

## Consequences

### Positive
* App Store / Google Play approval path stays open — sampling with
  foreground notification + per-task activation is the documented
  "VpnService exception category" pattern (§5 of ARCHITECTURE_DECISIONS.md).
* The privacy posture is **defensible** — there is no raw payload or
  identifier on the wire to leak.
* Backwards-compatible — existing CI grep tests (`mobile/test/...` and
  `backend/internal/...`) already encode the contract.

### Negative / Trade-offs
* Some telemetry is coarser than a full-payload capture would allow (e.g.
  TLS 1.3 0-RTT resumption can only be inferred from IP-ID — see
  ADR-0003 risk G1).
* Server-side analytics cannot join sessions by raw device id; it can only
  join by `DeviceIDHash`. This is a feature, not a bug, but downstream
  anti-abuse has to operate on the hashed signal.
* First-launch consent screen adds a small UX friction vs. an
  auto-accepted EULA — acceptable trade for legal posture.

### Follow-ups
* Author the full ADR-0006 sections (Anonim Cihaz Kimliği, Backend'de
  Saklanan, KVKK DELETE) when the privacy contract exits the stub state.
* Document the threat model (insider, ISP, app-store-review, malicious
  recipient of telemetry JSON) and the per-attack-surface mitigations.
* Cross-link from the `SECURITY.md` (to be authored) and from the App
  Store / Google Play privacy-questionnaire responses.

---

## Risk Register (A1–G1, plus E3)

The privacy contract in §Decision is exposed to seven named risks
(A1–G1) plus the E3 AUTHZ-1 long-term follow-up. Each is enumerated
with the affected §Decision principle, the threat, and the documented
mitigation. The register currently lives in this ADR (Risk Register
section below); the full likelihood / impact / owner / review-cadence
columns are a Sprint 5+ follow-up that will land in
`docs/RISK-REGISTER.md` when the register graduates beyond this
ADR's scope.

| ID  | Affected principle      | Risk                                                                                  | Mitigation (status)                                                                                                       |
|-----|-------------------------|---------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------|
| A1  | Veri Minimizasyonu (§5) | UUID v7 timestamp bits are predictable → server can re-derive install time from `device_id_hash` | Server-side timestamp mask: hash includes `server_salt` (§Backend'de Saklanan); raw UUID never crosses the device boundary. |
| B1  | Anonim Cihaz Kimliği    | Ed25519 private key extraction from app sandbox (rooted device, memory dump)         | Private key lives in Android Keystore / iOS Keychain via `flutter_secure_storage`; access gated on `Keychain accessGroup` + Secure Enclave on A-series chips (PR-29 §B). |
| C1  | Backend'de Saklanan     | `server_salt` rotation → all `device_id_hash` values change → session join breakage  | Dual-salt transition window: new + old salt accepted for N days; slog warn-log records both salts; re-hash job runs async.       |
| D1  | Anonim Cihaz Kimliği    | SHA-256(publicKey)[:16] fingerprint collision across distinct devices                  | Truncation `[:16]` (32 hex chars) is a 64-bit space — acceptable uniqueness verified by `TestPublicKeyFingerprint_UniquePerKey` (asserts distinct keys → distinct fingerprints); full 32-byte fp logged for backend join. |
| E1  | KVKK / GDPR (§5)        | Data retention beyond legally permitted window                                        | Default 6-month retention; configurable via `STORAGE_RETENTION_DAYS`; KVKK DELETE handler wipes `devices` + `telemetry` rows. |
| E3  | KVKK / GDPR (§5)        | AUTHZ-1 cross-device gate alone is necessary-but-not-sufficient once a real `users` table ships — the handler must additionally confirm the device row is still active. | Sprint 8+ follow-up: when the `users` table lands, extend `handleDeleteUser` (`backend/internal/api/users.go` L81-98) with a row-active check before `DeleteUser`, and add a regression test (`TestDeleteUser_DeviceRowInactive_403`) to `backend/internal/api/users_test.go`. Tracked here so the scope of the follow-up PR is explicit. |
| F1  | Anonim Cihaz Kimliği    | Cross-platform UUID byte order (mobile Go-side vs backend) yields divergent hashes    | Explicit big-endian (`uuid.UUID` in `google/uuid` is big-endian by spec); `TestHashDeviceID_KnownAnswer` pins the input order `uuid || salt`. |
| G1  | Right to erasure (§5)   | Async propagation of KVKK DELETE to Redis cache + downstream consumers                | Best-effort DELETE with slog warn-log (`users.go` L92-95 logs `delete-user cross-device attempt blocked` etc.); periodic Redis-pool sweep via `RunSweeper` + `SweepIdle` (`backend/internal/matching/pool.go` L482-491, L508-522) bounds Redis staleness at 15m max (`DefaultPoolTTL = 15 * time.Minute`); KVKK 7-day SLA upheld by sweeper. |

> **Status legend.** Mitigations marked *(active)* are in production
> today. Mitigations marked *(planned)* are Sprint 5+ follow-ups.
> *Active*: A1, B1, D1, E1, F1. *Planned*: C1, G1, E3.
> E3 numbering follows the legacy `RISKS.md` external reference
> (cited from `backend/internal/storage/interfaces.go` L90: "Per
> RISKS.md E3 + BRD §8 FR-7: 7-day SLA for user-initiated delete");
> the local A1–G1 sequence is preserved untouched.

### Follow-ups
* Lift this register into `docs/RISK-REGISTER.md` with full Risk
  Register columns (likelihood, impact, owner, review cadence) when
  it grows beyond seven risks.
* Wire each mitigation to a CI test that fails the build if the
  invariant regresses (e.g. `TestHashDeviceID_KnownAnswer` already
  pins F1; `TestHashDeviceID_SaltRotationChangesHash` already pins
  the C1 precondition; `TestPublicKeyFingerprint_UniquePerKey` pins
  D1; the AUTHZ-1 cluster in `users_test.go` pins the G1 surface
  — see §Testability below).

---

## Anonim Cihaz Kimliği (DeviceIdentity)

This section expands the in-§Decision "Anonim Cihaz Kimliği" stub into a
top-level contract. The DeviceIdentity is the *only* identifier the
backend ever sees from a device — it is the unit of privacy, telemetry
attribution, and KVKK DELETE.

### Identity construction

A DeviceIdentity is the tuple:

```
device_id       := uuid v7                 (generated on first launch, 128 bits)
server_salt     := 32 random bytes        (server-side, per-tenant secret)
device_id_hash  := SHA-256(device_id || server_salt)[:16]
public_key      := Ed25519 public key      (32 bytes, generated with private)
public_key_fp   := SHA-256(public_key)[:16]
fingerprint     := lowercase hex of public_key_fp (32 chars)
```

The input order for `HashDeviceID` is **uuid || salt** (bytes) — the
order is pinned by `TestHashDeviceID_KnownAnswer` and documented in
`backend/internal/auth/keys.go`. Reversing the order produces a
different hash and breaks the contract.

### Cross-platform implementation

| Layer   | Path                                                | Role                                                                |
|---------|-----------------------------------------------------|---------------------------------------------------------------------|
| Mobile  | `mobile/lib/shared/device_identity.dart`            | Generates UUID v7 + Ed25519 keypair; exposes `publicKeyFingerprint` |
| Mobile  | `mobile/lib/shared/telemetry_formatter.dart`        | Serialises `device_id_hash` + `public_key_fp` into telemetry JSON   |
| Backend | `backend/internal/auth/keys.go`                    | `HashDeviceID(uuid, salt)` → `device_id_hash` (server-side only)    |
| Backend | `backend/internal/storage/interfaces.go`            | Storage contract: `DeviceIDHash` + `PublicKeyFP` only              |
| Backend | `backend/internal/storage/postgres.go`             | Schema: `devices(device_id_hash PK, public_key, public_key_fp, …)` |

> **Path note.** The task instruction referenced
> `mobile/lib/mobile/identity/` as the canonical mobile-identity path.
> In the current codebase the identity primitives live in
> `mobile/lib/shared/` (not `mobile/lib/mobile/identity/`); the
> cross-references above use the actual paths on disk.

### Threat model

| Threat                                                | Defence                                                                                  |
|-------------------------------------------------------|------------------------------------------------------------------------------------------|
| App sandbox breach exposes private key                | `flutter_secure_storage` → Android Keystore / iOS Keychain (PR-29 §B for Keychain)       |
| Server compromise exposes `server_salt`               | Salt is per-tenant; rotated on incident; `device_id_hash` is one-way SHA-256             |
| Fingerprint collision across distinct devices         | 64-bit `[:16]` space — acceptable per `TestPublicKeyFingerprint_UniquePerKey` (asserts distinct keys → distinct fingerprints) |
| Client clock skew biases UUID v7 timestamp bits       | UUID v7 timestamp is *device-side*; server masks with its own `server_salt` (Risk A1)    |

### Follow-ups

* Move the cross-platform table into `docs/IDENTITY-CONTRACT.md` with
  per-platform edge cases (Secure Enclave gating, Keystore
  `setUserAuthenticationRequired`).
* Promote `publicKeyFingerprint` from `device_identity.dart` to a
  top-level export in `mobile/lib/shared/identity.dart` (Sprint 5
  follow-up).
* Wire a cross-language KAT (Go `HashDeviceID` ↔ Dart equivalent) so a
  backend known-vector drift is caught in mobile CI.

---

## Sprint 8 Extension — Contract Pin (HIGH)

Sprint 5 PR-33 expanded the "Anonim Cihaz Kimliği (DeviceIdentity)"
section above from the Sprint 4 PR-26 stub into the canonical
form. Sprint 8 tightens the contract by pinning three explicit,
testable invariants that the stub and Sprint 5 prose state but do
not surface as normative rules. Verifier §6 will use this section
as the regression target.

### Anonim Cihaz Kimliği — normative contract

The DeviceIdentity is the *only* identifier the backend ever sees.
The contract pins three rules; each is enforced by a named test or
a CI guard.

| # | Invariant (normative) | Source-of-truth (code) | Test / guard pinning it |
|---|---|---|---|
| C-1 | **The raw UUID v7 lives ONLY on the device.** It is never serialized to the backend over the wire, never logged, never written to cache, and never echoed in REST responses or URL paths. | `mobile/lib/shared/device_identity.dart` L80-81 (`uuidV7` is non-serializable); `mobile/lib/shared/telemetry_formatter.dart` L135 (`deviceIdHash` is the only device field on `Telemetry`) | `mobile/test/shared/device_identity_test.dart::forbiddenHardwareIdsNeverAppear` (F1 anonimlik); `tool/ci_grep_privacy_violations.dart` (STRIDE-3-01 mobile grep) |
| C-2 | **The Ed25519 private key lives ONLY on the device**, in the platform secure store (Android Keystore / iOS Keychain via `flutter_secure_storage`). It is not a field on `DeviceIdentity`; the `sign()` method reads it lazily from secure storage and never returns it. | `mobile/lib/shared/device_identity.dart` L73-77 (private key is *not* a class field; comment explicitly calls it a "privacy foot-gun"); L134-150 (`sign()` reads from `_secure` and returns only the 64-byte signature) | Lint rule + `device_identity_test.dart`; Secure Enclave / StrongBox gating lives in `flutter_secure_storage` defaults — see ADR-0003 §B for Keychain access-group story |
| C-3 | **The wire-format device identifier is `device_id_hash = SHA-256(uuid_v7 \|\| server_salt)[:16]`** — 16 bytes, hex-encoded to 32 lowercase chars. The input order is `uuid` first, then `salt`, and the salt is server-issued (per-tenant, never shared with the device's UUID in plaintext). | `backend/internal/auth/keys.go` L46-58 (`HashDeviceID`); `mobile/lib/shared/device_identity.dart` L100-125 (`deviceIdHash`) | `TestHashDeviceID_KnownAnswer` (pinned vector at keys.go L42-45); `TestPublicKeyFingerprint_KnownAnswer` (pinned at keys.go L64-66); `TestHashDeviceID_SaltRotationChangesHash` (Risk C1 precondition) |

> **Why the order matters (C-3).** SHA-256("uuid || salt") and
> SHA-256("salt || uuid") produce different digests for the same
> (uuid, salt) pair. If mobile and backend disagreed on the order,
> every `device_id_hash` the device sends at registration would
> silently fail to join the server-side `devices` row — the user
> would appear as a brand-new device on every telemetry POST, and
> any KVKK DELETE would target a row that no telemetry row ever
> pointed to. The cross-language KAT
> (`TestHashDeviceID_KnownAnswer` on both sides) is the only way to
> catch this drift — it was caught and fixed by **Sprint 1 PR-15**
> ("HashDeviceID uuid-first per ADR-0006 §Backend'de Saklanan",
> commit 353cb2a; promoted via PR-15 merge a8ab7d4). Reversing the
> order today would be a regression of F1 anonimlik.

> **Why the `[:16]` truncation (C-3).** The full 32-byte SHA-256
> digest is 64 hex chars; the wire format truncates to 16 bytes / 32
> hex chars. This is a 64-bit space — collision probability across
> the full device population of OpenE2EE is negligible for the
> foreseeable future (see Risk Register **D1** below). The full
> 32-byte digest is *not* used anywhere on the wire, but the
> backend keeps `public_key` and `public_key_fp` as separate columns
> so a future lengthening is a non-breaking change.

---

## Sprint 8 Extension — KVKK DELETE (Cross-Device Propagation)

The KVKK / GDPR right-to-erasure path is the *only* destructive
endpoint in the system, and it is the path most likely to be
abused for cross-device deletion if authorization is sloppy. This
section codifies the propagation chain that Sprint 5/6/7 wired up.

### Endpoint

`DELETE /api/v1/users/{device_id_hash}` — the path parameter is the
*server-side* `device_id_hash` (the same 32-hex-char value the
device sends in telemetry), NOT the raw UUID v7. Putting the raw
UUID in the URL would mean it gets logged at every proxy layer
(Kong access log, backend request log, time-series ingest) and
could be correlated across services — exactly the cross-device
re-identification vector the privacy posture is designed to avoid.

Implementation: `backend/internal/api/users.go` (`handleDeleteUser`).

### Authorization gate (Sprint 6 PR-37 — AUTHZ-1)

The endpoint sits inside the `IsAuthorized` middleware subtree, so
a bearer JWT is required. PR-37 (commit 91c6102, hand-off from
cyber-security review of AUTHZ-1 / STRIDE-6-04) added the second
gate: the JWT `sub` claim MUST equal the `device_id_hash` path
parameter. The check is implemented at `users.go` L81-98:

```go
subject := UserIDFromContext(r.Context())
if subject == ""      { writeUnauthorized(w, ...); return }
if subject != hash    { writeForbidden(w, ...);    return }
```

A mismatch returns 403 with a generic message — we deliberately do
not distinguish "no such device" from "wrong owner" so an attacker
cannot enumerate `device_id_hash` values by probing the endpoint.
The mismatch is warn-logged with `sub_matches_path: false` for
post-hoc audit.

Tests pinning the AUTHZ-1 contract (see `backend/internal/api/users_test.go`):

| Test | Asserts |
|---|---|
| `TestDeleteUser_HappyPath_SubMatchesHash` | JWT sub == path hash → `DeleteUser` called exactly once with the path hash |
| `TestDeleteUser_CrossDeviceAttempt_Forbidden` | JWT sub ≠ path hash → `DeleteUser` NEVER called; response is 403 |
| `TestDeleteUser_BadHashShape` | non-hex / too-short / too-long path → 400, `DeleteUser` NEVER called |
| `TestDeleteUser_MissingBearer_Returns401` | no Authorization header → 401, `DeleteUser` NEVER called |
| `TestDeleteUser_ExpiredToken_Returns401` | expired JWT → 401, `DeleteUser` NEVER called |
| `TestDeleteUser_StoreError_Returns500` | DB failure → 500, no hook fired |
| `TestDeleteUser_HookFailure_Still200` | relational delete succeeds, hook fails → 200 returned; user-facing right-to-erasure is not rolled back |
| `TestDeleteUser_HashBoundary` | exact 16-char and 64-char hashes accepted; one off → 400 |
| `TestDeleteUser_NoCrossDeviceSideEffects` | cross-device attempt → no Redis-side purge, no telemetry delete, no `sessions.sender_hash`/`receiver_hash` null-out |

### Propagation chain (Sprint 6 PR-37 + Sprint 7 STRIDE-6-03)

On a successful `DeleteUser(ctx, hash)` the handler fires a
`DeleteUserHook` callback wired in `cmd/server/main.go`:

1. **Relational storage (`storage.PostgresStore.DeleteUser`)** —
   single transaction: `DELETE FROM telemetry WHERE device_id_hash = $1`,
   `UPDATE sessions SET sender_hash = NULL WHERE sender_hash = $1`,
   `UPDATE sessions SET receiver_hash = NULL WHERE receiver_hash = $1`,
   `DELETE FROM devices WHERE device_id_hash = $1`, then `COMMIT`.
   See `backend/internal/storage/postgres.go` L267-298.
2. **Redis Active Pool (`matching.RedisPool.DeleteByHash`)** — fired
   from `DeleteUserHook` to remove the user's waiting-receiver row
   immediately rather than waiting for the 15-minute TTL. Wired in
   Sprint 7 PR-39 (commit ba2fc31, STRIDE-6-03 follow-up). O(N) over
   the pool; acceptable for steady-state pool sizes (tens).
3. **Best-effort hook failure** — if step 2 fails (Redis outage,
   network blip), the response is still 200 and the user-facing
   right-to-erasure is upheld. The 7-day KVKK SLA is guaranteed by
   the periodic background sweeper (`RunSweeper` → `SweepIdle`, every
   `DefaultIdleSweepInterval = 60s`), which keeps Redis staleness
   bounded at 15m max (`DefaultPoolTTL = 15 * time.Minute`). Audit
   trail: slog warn-log on hook failure (`users.go` L117-119) +
   cross-device attempts (`users.go` L92-95 with
   `sub_matches_path: false`). No dedicated `kvkk_delete_audit`
   table is needed because the user-facing right-to-erasure is
   upheld by the synchronous step (1) regardless of step (2)
   outcome.

### Defence-in-depth notes

* The JWT secret in production is Kong-minted; the dev-fallback JWT
  secret (SEC-1 follow-up in `cmd/server/main.go`) is logged as a
  no-go in production environments.
* `isValidDeviceHash` enforces the 16-64 lowercase-hex shape so a
  phone-number-shaped injection can't sneak into the path
  parameter (`users.go` L135-146).
* When a real `users` table lands (Sprint 6+ follow-up), the same
  handler must additionally confirm the device row is still active
  (see Risk Register **E3** below) — the AUTHZ-1 gate alone is
  necessary but not sufficient long-term.

---

## Sprint 8 Extension — MaskIP (/24 IPv4 /48 IPv6)

The IP-masking helper lives at
`backend/internal/operator/mask.go` and is the second privacy
primitive (after `device_id_hash`) that the wire format relies on.
The contract is **deterministic and one-way**: given an IP, the
output is a /24 (v4) or /48 (v6) subnet string with the host bits
zeroed; given the subnet, the original IP cannot be recovered.

### Contract

```text
MaskIPv4(s) -> "<a>.<b>.<c>.0/24"        (host bits zeroed)
MaskIPv6(s) -> "<a>:<b>:<c>::/48"       (host bits zeroed)
MaskIPv4_in_v6("::ffff:1.2.3.4") -> "1.2.3.0/24"  (unmapped first)
```

The implementation uses Go's `net/netip` package and applies
`.Masked()` to the prefix so the host bits are zeroed before
formatting. **This was a critical bug fix in Sprint 1 PR-20**
(commit 2e4d492, "MaskIP — actually mask via .Masked()"; merged via
PR-20 → 8fcbeda). Before PR-20, `netip.PrefixFrom(addr, 24).String()`
returned `"88.240.5.12/24"` — the raw IP with a `/24` suffix
appended — leaking the original address into cache writes and REST
responses. The PR-20 fix is now pinned by `TestMaskIP_ActuallyMasks`
in `backend/internal/operator/mask_test.go`.

### Tests pinning the contract

| Test | Asserts |
|---|---|
| `TestMaskIP_ActuallyMasks` (v4 subtest) | `88.240.5.12` → `88.240.5.0/24`; full v4 KAT table |
| `TestMaskIP_ActuallyMasks` (v6 subtest) | `2a01:5ec0:1234:5678::1` → `2a01:5ec0:1234::/48`; full v6 KAT table |
| `TestMaskIP_V4In6_UnmapsToV4` | `::ffff:1.2.3.4` → `1.2.3.0/24` (v4-mapped-in-v6 unmap, not `::/24`) |
| `TestMaskIP_EmptyAndUnparseable` | empty / unparseable → `""` (no panic, no leak) |
| `TestMaskIP_AlreadyMaskedPassesThrough` | `88.240.5.0/24` is a fixed point; `88.240.5.42/24` re-masks to `88.240.5.0/24` |
| `TestApplyIPMask_WritesMaskedQueryValue` | `applyIPMask(info, "88.240.5.12")` writes the masked form to `info.QueryValue` (regression pin for PR-20) |
| `TestApplyIPMask_NilInfoIsNoOp` | defensive nil-check |

### Where MaskIP is called

* `applyIPMask(info, canonical)` — used by MNPTRAdapter / Service-level
  finalize so the cache and the response always carry the same
  masked form. See `mask.go` L151-157.
* Direct callers (search: `grep -rn "operator.MaskIP\|operator.MaskPhone" backend/`):
  any future operator-resolver call site that previously would have
  logged the raw IP MUST go through MaskIP first.

### Threat model

| Threat | Defence |
|---|---|
| Raw IP leaks into cache (operator resolver) | `applyIPMask` writes the masked form into `info.QueryValue` before cache write |
| Raw IP leaks into REST response | `MaskIP` is the only entry point for IP → response serialization |
| v4-in-v6 maps to a meaningless `::/24` | `addr.Unmap()` on `Is4In6()` (mask.go L132-134) |
| netip API drift (Go 1.22 → 1.26) | `.Masked()` is the documented zeroing primitive; the regression test pins the KAT |

---

## Sprint 8 Extension — Testability (CI guard coverage)

ADR-0006 invariants are only as strong as the tests that pin them.
This section names every test/guard that currently pins an
ADR-0006 invariant, and the **gaps** the Sprint 8 audit found that
need follow-up work.

### Existing guards (pinning ADR-0006 today)

| Guard / test | Path | Pins |
|---|---|---|
| `TestHashDeviceID_KnownAnswer` | `backend/internal/auth/keys_test.go` | C-3 (input order `uuid \|\| salt`); pins reference vector at keys.go L42-45 |
| `TestHashDeviceID_SaltRotationChangesHash` | `backend/internal/auth/keys_test.go` | Risk C1 precondition (salt rotation must change hash) |
| `TestPublicKeyFingerprint_KnownAnswer` | `backend/internal/auth/keys_test.go` | C-3 (fingerprint algorithm + length) |
| `TestPublicKeyFingerprint_UniquePerKey` | `backend/internal/auth/keys_test.go` | Risk D1 (distinct keys → distinct fingerprints at /16 truncation) |
| `TestMaskIP_ActuallyMasks` (v4/v6) | `backend/internal/operator/mask_test.go` | MaskIP /24 v4 /48 v6 contract |
| `TestMaskIP_V4In6_UnmapsToV4` | `backend/internal/operator/mask_test.go` | v4-in-v6 unmapping |
| `TestApplyIPMask_WritesMaskedQueryValue` | `backend/internal/operator/mask_test.go` | Regression pin for Sprint 1 PR-20 |
| `TestDeleteUser_HappyPath_SubMatchesHash` | `backend/internal/api/users_test.go` | AUTHZ-1 happy path |
| `TestDeleteUser_CrossDeviceAttempt_Forbidden` | `backend/internal/api/users_test.go` | AUTHZ-1 cross-device rejection (PR-37 contract) |
| `TestDeleteUser_NoCrossDeviceSideEffects` | `backend/internal/api/users_test.go` | AUTHZ-1 side-effect isolation |
| `forbiddenHardwareIdsNeverAppear` | `mobile/test/shared/device_identity_test.dart` L245-249 | C-1 / F1 anonimlik (mobile) |
| `tool/ci_grep_privacy_violations.dart` | `tool/ci_grep_privacy_violations.dart` | F1 anonimlik — Sprint 7 STRIDE-3-01 mobile grep; scans `mobile/lib` + `mobile/test` for `imei`, `telephonymanager`, `getdeviceid`, `androidid` in code position |

### Known gaps (Sprint 8 audit)

| Gap | Risk | Recommended follow-up |
|---|---|---|
| **No backend CI grep for raw-IP leaks.** A new Go commit that writes a raw IP into a cache / response without going through `MaskIP` would not be caught by an automated guard — only by code review. The Sprint 7 mobile grep (`tool/ci_grep_privacy_violations.dart`) is the right shape but covers only Dart / Kotlin / Swift, not Go. | High — the PR-20 leak regressed once (Sprint 1 §6 D/E retry), and the only thing standing between us and a recurrence is reviewer vigilance. | Sprint 8+ follow-up: extend `tool/ci_grep_privacy_violations.dart` (or add a sibling `tool/ci_grep_backend_privacy.go`) to scan `backend/cmd/`, `backend/internal/` for raw IP literals (`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`) outside the masking helpers. The existing Dart script's comment-stripping state machine is portable to Go with minor adjustments. |
| **No cross-language KAT for `HashDeviceID`.** The Go KAT pins the backend-side reference vector; a Dart equivalent at `mobile/test/shared/device_identity_test.dart` does not pin the same vector (the Dart implementation is tested for shape, not byte-equality with the Go output). | Medium — a future Dart-side change to `_hexLower`, `_uuidToBytes`, or `pc.SHA256Digest` would not be caught until production telemetry mismatches the server's `devices` row. | Sprint 8+ follow-up: add `TestDeviceIdHash_KnownAnswer_MatchesGoBackend` in `mobile/test/shared/device_identity_test.dart` with the same `(uuid, salt) → hash` reference vector as `TestHashDeviceID_KnownAnswer`. |
| **AUTHZ-1 covers destructive endpoints; the login endpoint stays open per its ADV-3 contract.** Once a real `users` table lands, the AUTHZ-1 gate alone is necessary-but-not-sufficient — the handler must additionally confirm the device row is still active. | Low (Phase 1) → High (when `users` table ships) | Track under Risk E3; the AUTHZ-1 test suite must be extended in the same PR that ships the `users` table. |
| **No automated test for `server_salt` rotation → all `device_id_hash` change → session join breakage** (Risk C1 mitigation: dual-salt transition window). | Medium — the dual-salt transition code path has not been authored yet; when it is, it needs a regression test that pins "old + new salt both accepted for N days". | Sprint 8+ follow-up when C1 mitigation ships. |

### Testability posture summary

* **Mobile side**: F1 anonimlik is enforced both by a Dart-level
  test (`forbiddenHardwareIdsNeverAppear`) and a CI guard
  (`tool/ci_grep_privacy_violations.dart`). This is the gold
  standard — every Sprint that touches `mobile/lib/` runs both.
* **Backend side**: ADR-0006 invariants are pinned by named Go
  tests (KAT for `HashDeviceID`, `PublicKeyFingerprint`, `MaskIP`,
  AUTHZ-1), but there is **no equivalent of the mobile CI grep**
  for the backend tree. This is the most visible gap in the
  current Testability posture and the recommended Sprint 8+
  follow-up. Until it ships, reviewer vigilance is the only
  backstop for a regression of the PR-20 MaskIP bug.

---

## Revision History

| Sprint | PR | Change |
|---|---|---|
| Sprint 1 | (stub at `docs/ARCHITECTURE_DECISIONS.md` §5) | Privacy contract drafted as 4 principles |
| Sprint 1 | **PR-15** (`353cb2a` → `a8ab7d4`) | HashDeviceID uuid-first fix — input order `uuid \|\| salt` pinned by KAT. **Closes F1 anonimlik input-order regression.** |
| Sprint 1 | **PR-20** (`2e4d492` → `8fcbeda`) | MaskIP `.Masked()` fix — host bits now zeroed before format. **Closes Sprint 1 §6 D/E retry; pins /24 v4 /48 v6 KAT.** |
| Sprint 4 | PR-26 (`eec3c3c`) | Stub ADR files extracted (`ADR-0003`, `ADR-0006`, `ADR-0008`) to resolve Sprint 3 Integration Gate cross-refs |
| Sprint 5 | PR-33 | Anonim Cihaz Kimliği (DeviceIdentity) + Risk Register A1-G1 expansion; cross-platform implementation table; threat model |
| Sprint 6 | **PR-37** (`91c6102` → `67a8c29`) | AUTHZ-1 — JWT `sub` MUST equal `device_id_hash` path on KVKK DELETE. **Closes cross-device deletion regression (AUTHZ-1 / STRIDE-6-04).** |
| Sprint 6 | **PR-38** (`81854d0` → `730da02`) | pgx/v5 CVE chain guardrail: CI `govulncheck` + hermetic pin test. Defense-in-depth for the storage layer that holds the privacy contract. |
| Sprint 7 | PR-39 / STRIDE-3-01 | `tool/ci_grep_privacy_violations.dart` — mobile CI guard for IMEI / TelephonyManager / getDeviceId / androidId (Sprint 7 Item 12, commit `a9fed70`, merged via `7ff3efd`). Closes F1 anonimlik mobile grep gap. |
| Sprint 7 | PR-39 / STRIDE-6-03 | Active Pool `DeleteByHash` + `SweepIdle` + `RunSweeper` — Redis-side KVKK DELETE propagation wired into `DeleteUserHook` (commit `ba2fc31` = Sprint 7 Item 4 merge). |
| Sprint 8 | **This PR** (`feat/pr-s8-adr-0006-ext`) | Normative contract pin (C-1, C-2, C-3); KVKK DELETE cross-device section; MaskIP section; Testability section with named gaps. |

---

## Cross-references

* Source: [`docs/ARCHITECTURE_DECISIONS.md`](ARCHITECTURE_DECISIONS.md) §5
* VPN layer that enforces the metadata-only invariant on-device:
  [`docs/ADR-0003-vpn-layer.md`](ADR-0003-vpn-layer.md)
* Mobile device identity: `mobile/lib/shared/device_identity.dart`
* Backend storage contract: `backend/internal/storage/interfaces.go`
* Backend hash + fingerprint primitives: `backend/internal/auth/keys.go`
* Backend IP masking primitives: `backend/internal/operator/mask.go`
* KVKK DELETE handler (AUTHZ-1 gate): `backend/internal/api/users.go`
* KVKK DELETE relational store: `backend/internal/storage/postgres.go` (`DeleteUser`)
* KVKK DELETE Redis hook: `backend/internal/matching/pool.go` (`DeleteByHash`, `SweepIdle`, `RunSweeper`)
* Mobile telemetry wire format (anonymised fields only):
  `mobile/lib/shared/telemetry_formatter.dart`
* Mobile CI grep (F1 anonimlik): `tool/ci_grep_privacy_violations.dart`
* Sprint 1 PR-15 (HashDeviceID uuid-first): commit `353cb2a` → merge `a8ab7d4`
* Sprint 1 PR-20 (MaskIP `.Masked()` fix): commit `2e4d492` → merge `8fcbeda`
* Sprint 6 PR-37 (AUTHZ-1 JWT sub check): commit `91c6102` → merge `67a8c29`
* Sprint 6 PR-38 (pgx/v5 CVE chain guardrail): commit `81854d0` → merge `730da02`
* Sprint 7 STRIDE-3-01 (mobile CI grep): commit `a9fed70` → merge `7ff3efd` (Sprint 7 Item 12)
* Sprint 7 STRIDE-6-03 (Active Pool KVKK purge): commit `ba2fc31` = Sprint 7 Item 4 merge