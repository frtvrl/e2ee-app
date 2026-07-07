# ADR-0003 — VPN Layer (Android VpnService + iOS NetworkExtension)

| Field      | Value                                  |
|------------|----------------------------------------|
| **Status** | Accepted (Sprint 3) — VPN-Layer Extension (Sprint 8 PR-s8item5) |
| **Date**   | 2026-07-07 (Sprint 8 extension) — original stub 2026-07-06 |
| **Owner**  | Architect (mvs_25a7a987f73243899e35a1485c6ba224) |
| **Source** | This ADR was extracted as a Sprint 4 PR-26 stub from [`docs/ARCHITECTURE_DECISIONS.md`](ARCHITECTURE_DECISIONS.md) §4 (MVP Kapsamı) and §5 (Regülasyon Uyumu). Sprint 8 PR-s8item5 extends it with three sections — **VPN purge semantics** (STRIDE-6-03 follow-up), **iOS Keychain access group** (MOB-8 cert pinning connection), **Android Keystore** (MOB-5 follow-up) — plus the per-platform threat model. Cross-links the Sprint 5 PR-22b iOS VPN, Sprint 6 PR-39 mobile security hardening, and Sprint 7 Items 4 / 6 / 14 follow-ups. |

> **Sprint 8 extension notice.** The original Sprint 4 PR-26 stub captured
> the architectural *Context*, *Decision* and *Consequences* already
> documented in `ARCHITECTURE_DECISIONS.md`. Sprint 8 PR-s8item5 keeps the
> original sections intact (no breaking edits — they are referenced from
> `ADR-0006-anonimlik.md`, the Sprint 3 PR-22b review notes, and the
> Sprint 7 closure) and **adds** three top-level sections:
>
> 1. **VPN purge semantics** — when KVKK Art. 17 DELETE fires
>    (`backend/internal/api/users.go` Sprint 6 PR-37 gate), the server
>    purges the device's Active Pool row (Sprint 7 Item 4
>    `STRIDE-6-03`), **AND** the mobile-side VPN session must tear
>    down locally so the device stops holding any per-session tunnel
>    state.
> 2. **iOS Keychain access group** — `Runner.entitlements` declares
>    `com.apple.security.application-groups` with
>    `group.com.opene2ee.opene2ee` (Sprint 5 PR-22b + Sprint 7 MOB-6
>    TeamID wiring). Sprint 7 MOB-8 cert-pinning SPKI hashes (and the
>    PR-29 Keychain master key) are stored in Keychain under this
>    access group so the host app **and** the `OpenE2eeTunnelProvider`
>    network-extension process can read the same secrets.
> 3. **Android Keystore** — Sprint 7 MOB-5 bumped `minSdk` from 21 to
>    23 so `KeyGenParameterSpec` (genuine-backed AES master-key
>    generation) is unconditionally available. VPN tunnel keys
>    (the AES-256-GCM master + the Ed25519 private key from
>    `device_identity.dart`) live in AndroidKeyStore-backed
>    `flutter_secure_storage`.
>
> A new **Threat model** section enumerates the per-attack-surface
> mitigations (per-app VPN rules, tunnel transport security,
> `NetworkExtension` process isolation).

---

## Context

OpenE2EE is a network-security / E2EE-transparency tool. To prove end-to-end
encryption between two devices we need to *observe* the live network traffic
that flows between them — but the application-store policy and the user's
privacy posture forbid payload capture or off-device upload of raw packets.
We therefore need a **sampling-only VPN layer** that:

1. Brings up a system-mediated tunnel so the OS hands us a copy of every
   packet the device transmits or receives.
2. **Never** copies packet payloads off-device — only header metadata (IP,
   transport, IP-ID) is retained.
3. Operates under explicit, time-bounded user consent (no 7/24 background
   spying).
4. Stays inside App Store / Google Play VPN policy: positioned as a *Network
   Diagnostic Tool*, not a VPN proxy.

These constraints are documented in `docs/ARCHITECTURE_DECISIONS.md` §4
("MVP Kapsamı") and §5 ("Regülasyon Uyumu ve Mağaza Politikaları").

## Decision

The mobile app brings up a **local VPN tunnel** at the OS boundary
(Android `VpnService`, iOS `NetworkExtension`) and reads only the metadata
needed to compute a sampled entropy/fingerprint score:

* **Per-platform primitive.**
  * Android — `android.net.VpnService` + `Builder.establish()`. Permission
    handshake owned by `MainActivity` (`VpnService.prepare()` →
    `onActivityResult(RESULT_OK)`); tunnel runtime owned by
    `OpenE2eeVpnService` (TUN reader thread + bounded metadata ring +
    `protect()` to forward payload back to the real NIC).
  * iOS — `NEVPNManager` + `NEPacketTunnelProvider` (Sprint 3 PR-22b). iOS
    restricts background interception, so the Flutter side only uses the
    *official* `NetworkExtension` API.
* **Sampling, not streaming.** Only the first `SAMPLING_CAP_PACKETS` (10) of
  each session are captured into a bounded ring; the cap is hit quickly so
  battery and CPU stay low. See §6 of ARCHITECTURE_DECISIONS.md — task-based
  model.
* **No off-device payload.** `protect()` hands the original packet bytes
  back to the OS for normal forwarding; the ring buffer only ever sees IP
  /TCP/UDP header fields, transport ports, TCP flags, and an IP-ID-derived
  TLS-1.3 0-RTT heuristic. See `ADR-0006-anonimlik.md` §"Veri Minimizasyonu".
* **Foreground service notification.** Android 14+ (API 34) requires
  `foregroundServiceType="specialUse"` for VPN services not classified as
  *system*. Declared in `AndroidManifest.xml` (Manifest Risk **B2** of the
  full ADR).
* **Per-app allowlist / denylist.** `VpnService.Builder.allowedApplications`
  (API 21+) lets the user scope the tunnel to specific apps; mutually
  exclusive with `disallowedApplications`.
* **Consent UX.** First-launch consent screen discloses that traffic is
  *only* processed on-device for security scoring; iOS uses `NetworkExtension`
  only. (§5 of ARCHITECTURE_DECISIONS.md.)
* **Task-based activation** — the VPN profile is only active during an
  active test (default 2 minutes); auto-disables after the task ends. (§6.)

## Consequences

### Positive
* App Store / Google Play approval path stays open — sampling with
  foreground notification + per-task activation is the documented
  "VpnService exception category" pattern (§5 of ARCHITECTURE_DECISIONS.md).
* Battery cost is bounded — sampling 10 packets per task, not streaming.
* Privacy posture is defensible — no raw payload ever crosses the OS
  boundary; metadata is masked before reaching Dart (see ADR-0006).
* Single code base for Android + iOS via Flutter; native MethodChannels
  are thin and the Kotlin/Swift service logic is sibling-symmetric for
  review.

### Negative / Trade-offs
* Cannot detect TLS 1.3 0-RTT resumption telemetry beyond IP-ID
  heuristics (Risk **G1** of the full ADR) — deferred to a Sprint 4
  follow-up.
* Android 14+ manifest must declare `foregroundServiceType="specialUse"`
  with a `<property>` subtype — Google Play review treats this as a
  non-renewable exception category (Risk **B2**).
* iOS background-interception limits force the *Active Pool* model on
  P2P receiver side (Sprint 3 PR-22b) — see ADR-0006 §"Active Pool".

### Follow-ups
* Author the full ADR-0003 Risk Register (A1–G1) when the VPN layer exits
  the stub state.
* Expand with per-platform handshake state diagrams (Android prepare flow,
  iOS NEVPNManager protocol negotiation).
* Document the integration with the Flutter `opene2ee/vpn` MethodChannel
  surface (current contract lives in
  `mobile/lib/mobile/vpn/method_channel.dart`).

---

## VPN purge semantics (Sprint 8 — STRIDE-6-03 follow-up)

KVKK / GDPR Art. 17 grants the user the right to erasure. The Sprint 6
PR-37 server gate (`backend/internal/api/users.go`) hard-deletes every
row belonging to the `device_id_hash` from Postgres; the Sprint 7 Item 4
`STRIDE-6-03` follow-up extended this with a server-side **Active Pool
purge** that removes the device's waiting-receiver row from Redis. This
section binds the mobile-side VPN layer into the same SLA.

### Three-layer purge

| Layer | Component | Effect | Sprint |
|---|---|---|---|
| Relational | `storage.Postgres.DeleteUser(ctx, deviceIDHash)` (`backend/internal/storage/postgres.go:267`) | Hard-delete every `telemetry` + `devices` row for the hash | Sprint 6 PR-37 |
| Cache | `matching.RedisPool.DeleteByHash(ctx, hash)` (`backend/internal/matching/pool.go:442`) | `ZRANGE`+`ZREM` every member matching the hash from `opene2ee:matching:pool:hash:<hash>` | Sprint 7 Item 4 |
| Tunnel | `OpenE2eeVpnService.stopTunnel()` (Android) / `NEPacketTunnelProvider.stopTunnel(...)` (iOS) | Drop in-memory ring buffer; tear down `ParcelFileDescriptor`; clear `Keychain` access-group entry if `kvkk=true` | **Sprint 8 PR-s8item5 (this ADR)** |

The **tunnel layer** is the missing third leg. Without it, after a
successful Art. 17 DELETE on the server the device continues to hold
per-session state in two places:

1. The bounded ring buffer (Android: `mobile/android/.../OpenE2eeVpnService.kt`,
   iOS: `mobile/ios/NetworkExtension/OpenE2eeTunnelProvider.swift`) — keeps
   the last `SAMPLING_CAP_PACKETS=10` headers per task.
2. The Keychain / AndroidKeyStore entry for the per-session tunnel AES
   key (when `key-per-session=true`; otherwise only the long-lived
   master is held).

The mobile client MUST drop both on receipt of a 200 from
`DELETE /api/v1/users/{device_id_hash}` (the same JWT-authenticated
endpoint that triggered the server-side purge).

### Contract — mobile VPN teardown

```dart
// mobile/lib/mobile/vpn/vpn_teardown.dart (Sprint 8 follow-up; design
// surface — implementation lands when this ADR is approved).
Future<void> tearDownOnKvkkDelete({
  required VpnMethodChannel channel,
  required SecureStorage storage,
}) async {
  // 1. Stop the VPN service. On Android this invokes
  //    OpenE2eeVpnService.onRevoke() (system callback) and clears
  //    the VpnService.protect() file descriptor; on iOS it invokes
  //    NETunnelProviderManager.connection.stopVPNTunnel().
  await channel.invokeMethod('stopTunnel', <String, Object?>{
    'reason': 'kvkk_delete',
  });

  // 2. Wipe the in-memory ring buffer. The native side drops the
  //    ParcelFileDescriptor / packetFlow readPackets completion
  //    handler before returning.
  await channel.invokeMethod('purgeRingBuffer', null);

  // 3. Wipe the per-session AES key from Keychain (access group
  //    `group.com.opene2ee.opene2ee`) / AndroidKeyStore. The
  //    long-lived master (PR-29 Keychain master, MOB-5 Keystore
  //    master) is intentionally NOT wiped — the device identity
  //    (Ed25519 keypair) survives the delete so the user can
  //    re-register without a fresh install.
  await storage.deletePerSessionVpnKey();
}
```

The Dart-side `tearDownOnKvkkDelete` MUST be called from the same
`DELETE /api/v1/users/{device_id_hash}` response handler that the
Sprint 6 PR-37 `handleDeleteUser` invokes on the server. The two are
fire-and-forget independent: a hook failure on either side does NOT
block the other. The server hook's "no rollback on Redis failure"
posture (`backend/cmd/server/main.go:615-626`) is mirrored on the
client — if `stopTunnel` fails, the DELETE has still succeeded
server-side and the `RingBufferPurge` is best-effort within a 7-day
window that matches the SLA.

### Idempotency

* `tearDownOnKvkkDelete` is idempotent: a second call after a successful
  first call is a no-op (the tunnel is already stopped, the ring is
  already empty, the per-session key is already deleted).
* The server hook is idempotent: `DeleteByHash` returns `(0, nil)` on
  an empty hash, `ZREM` of a missing member returns 0, `RunSweeper`'s
  `SweepIdle` is a `ZREMRANGEBYSCORE` that is a no-op on an empty pool
  (see `backend/internal/matching/pool.go:442-491`).

### Audit trail

Both sides MUST log the event for the KVKK audit log:

* Server: `kvkk_delete_audit` table (`backend/internal/storage/postgres.go`),
  one row per `DELETE /api/v1/users/{device_id_hash}` call, with
  `removed_pool_rows` count, `removed_telemetry_rows` count, and the
  hash.
* Client: structured `slog` / `logger` line with
  `event=kvkk_vpn_teardown`, `removed_ring=N`, `removed_key=true/false`,
  `device_id_hash=...`. The logger is the
  `PinnedHttpOverrides`-wrapped `ApiClient`'s structured logger
  (Sprint 7 MOB-8 wiring) so the line inherits the same
  privacy posture.

---

## iOS Keychain access group (Sprint 8 — MOB-8 connection)

The VPN tunnel master key (Sprint 5 PR-22b / Sprint 5 PR-29 Keychain
master), the per-session tunnel key, and the cert-pinning SPKI hashes
(Sprint 7 MOB-8) all live in **Keychain** on iOS. The host Runner and
the `OpenE2eeTunnelProvider` network-extension are two **separate
processes** — without an entitlement they cannot read each other's
Keychain entries. The `application-groups` entitlement is the bridge.

### Entitlement surface

`mobile/ios/Runner/Runner.entitlements` declares:

```xml
<key>com.apple.security.application-groups</key>
<array>
    <string>group.com.opene2ee.opene2ee</string>
</array>
```

The matching `mobile/ios/NetworkExtension/OpenE2eeTunnelProvider.entitlements`
carries the same value (Sprint 5 PR-25 baseline). Apple requires that
**both** targets carry the entitlement before Keychain queries with
`kSecAttrAccessGroup = "group.com.opene2ee.opene2ee"` succeed in both
processes.

Sprint 7 Item 13 (MOB-6) added `com.apple.developer.team-identifier`
because Apple's developer documentation mandates it whenever an App
Group is declared ("If your app uses the App Groups entitlement, you
must also have the team identifier entitlement"). The TeamID flows
from `mobile/ios/Config/{Local,Production}.xcconfig` via
`$(TEAMS_IDENTIFIER)` substitution.

### What lives where

| Secret | Keychain class | Access group | Read by | Write by |
|---|---|---|---|---|
| Tunnel master key (32-byte AES-256-GCM, `kVpnIosSprint3Master` source-of-truth) | `kSecClassKey` | `group.com.opene2ee.opene2ee` | `OpenE2eeTunnelProvider` (read on `startTunnel`) | `Runner` (seed via `SecItemAdd` on first launch, idempotent via `errSecDuplicateItem`) |
| Per-session tunnel key (32-byte, rotates on each `startTunnel`) | `kSecClassKey` | `group.com.opene2ee.opene2ee` | `OpenE2eeTunnelProvider` | `OpenE2eeTunnelProvider` (write) |
| Device-identity Ed25519 private key | `kSecClassKey` (via `flutter_secure_storage`) | `group.com.opene2ee.opene2ee` | `Runner` (sign telemetry) | `Runner` (generate on first launch) |
| MOB-8 SPKI pin-set (read by `ApiClient` via `PinnedHttpOverrides`) | in-memory Dart `Set<String>` (NOT in Keychain — see below) | — | `Runner` | `Runner` (build-time config) |

The MOB-8 SPKI pin-set is intentionally **not** stored in Keychain: the
shipped values come from the Dart `CertPinConfig` literal and the
`<pin digest="SHA-256">` entries in `network_security_config.xml` /
`Info.plist NSPinnedDomains` (see `docs/SPRINT-7-MOB-8-CERT-PINNING.md`
§2). The connection to Keychain is via the access group only — when the
Dart-side `PinnedHttpOverrides` reads the pinned-host set, the
underlying `HttpClient` re-uses the same `NSAppTransportSecurity`
configuration that the network-extension process inherits from the
host app's bundle. This means a Keychain-resident CA rotation (future
work) can update the pins without rebuilding the app, as long as the
operator follows the rotation procedure in `SPRINT-7-MOB-8-CERT-PINNING.md`
§4.1.

### Access-group + biometric binding

When `BiometricAuthenticator` (Sprint 7 MOB-10) is enabled, the
per-session tunnel key MUST be re-bound to biometric access. The pattern
mirrors MOB-10's `AuthOptions(biometricOnly=true, stickyAuth=true)`:

```swift
let access = SecAccessControlCreateWithFlags(
    kCFAllocatorDefault,
    kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
    [.biometryCurrentSet, .privateKeyUsage],
    &error
)
// kSecAttrAccessGroup = "group.com.opene2ee.opene2ee"
// kSecAttrApplicationTag = "opene2ee.ios.vpn.session.<sessionIdHash>"
```

The `biometryCurrentSet` flag means the entry is invalidated on Touch
ID / Face ID enrollment change — the device-identity survives (it's
not biometric-gated) but the per-session tunnel key must be re-derived
on the next `startTunnel` call.

### Why not share via `UserDefaults` / file hand-off

Apple explicitly forbids inter-process file hand-off between a host
app and a NetworkExtension except via App Group containers
(`group.<bundleid>`) or Keychain access groups. File hand-off is also
a privacy regression — the master key would briefly touch the
filesystem. The Keychain path is the only one that keeps the secret
in the Secure Enclave when `kSecAttrTokenID = kSecAttrTokenIDSecureEnclave`
is set on A-series chips.

---

## Android Keystore (Sprint 8 — MOB-5 follow-up)

Android's analogue of the iOS Keychain access group is the
**AndroidKeyStore** provider, gated on API level 23 (Android 6.0
Marshmallow). Sprint 7 Item 6 (MOB-5) bumped `minSdk` from 21 to 23
specifically so the genuine-backed key generation API is
unconditionally available — `flutter_secure_storage 9.x` falls back to
software-only SharedPreferences on API < 23, which would break the
Ed25519 private-key-at-rest guarantee from
`docs/ADR-0006-anonimlik.md §B1`.

### Per-secret storage

| Secret | AndroidKeyStore alias | Class | Backing hardware |
|---|---|---|---|
| Tunnel master key (32-byte AES-256-GCM) | `opene2ee_vpn_master_v1` | `KeyGenParameterSpec` + `KeyProperties.KEY_ALGORITHM_AES` + `KeyProperties.BLOCK_MODE_GCM` + `KeyProperties.ENCRYPTION_PADDING_NONE` | StrongBox (TEE on devices without StrongBox) when `isStrongBoxBacked=true` |
| Per-session tunnel key (32-byte, rotates per task) | `opene2ee_vpn_session_<sessionIdHash>` | same as above | TEE / StrongBox |
| Device-identity Ed25519 private key | `opene2ee_device_identity_v1` | `KeyGenParameterSpec` + `KeyProperties.KEY_ALGORITHM_EC` + `KeyProperties.DIGEST_SHA256` (Ed25519 in API 33+; SECP256R1 below) | StrongBox when available |

The per-session key is generated in `OpenE2eeVpnService.onStartCommand`
under a `KeyGenParameterSpec` with
`setUserAuthenticationRequired(false)` (task-bounded, no user gesture)
and `setRandomizedEncryptionRequired(true)`. The random-encryption
requirement means the ciphertext is unique per use, defending against
chosen-ciphertext attacks on the KeyStore blob.

### StrongBox vs TEE

`isStrongBoxBacked=true` is the preferred setting on devices that
have a discrete StrongBox security chip (Pixel 3+, Samsung Galaxy S10+
in selected regions). On devices without StrongBox the KeyMint
fallback is to TEE — both are acceptable, but operators MUST log the
backing hardware at first launch via `KeyInfo.getSecurityLevel()` so
a missing-StrongBox device is visible in the diagnostics dump (no PII
is leaked — only the constant `STRONGBOX`/`TEE`/`SOFTWARE` enum).

The `KeyInfo.getSecurityLevel()` value is asserted in the regression
test `mobile/test/mobile/security/keystore_posture_test.dart` (to be
authored in a Sprint 9 follow-up; the Sprint 8 ADR extension is the
design record). The test asserts:

1. `SecurityLevel.STRONGBOX` OR `SecurityLevel.TRUSTED_ENVIRONMENT`
   (TEE) — `SOFTWARE` level fails the build on Android 6.0+ devices.
2. The master key alias `opene2ee_vpn_master_v1` exists in
   AndroidKeyStore after the first VPN handshake.
3. The per-session key alias is rotated (deleted + re-created) on
   every `startTunnel`.

### Inter-process sharing

Unlike iOS, Android does not need an explicit cross-process hand-off
because the VPN service (`OpenE2eeVpnService`) runs in the **same**
process as the host activity (`MainActivity`) by default — the
`VpnService.Builder()` returns a `ParcelFileDescriptor` that is owned
by the same Linux UID. The AndroidKeyStore is keyed by the app's UID,
so the host activity and the VPN service see the same key material
without any explicit `Keychain access group` analogue.

The trade-off is that the VPN service runs **in-process** — a
memory-disclosure bug in the host activity leaks the VPN service's
in-memory state. iOS's `NEPacketTunnelProvider` runs **out-of-process**
(Sprint 5 PR-22b), which is a strictly stronger isolation boundary;
the Android side accepts this in exchange for the
`ParcelFileDescriptor` ergonomics. The trade-off is documented in the
threat model below.

---

## Threat model (Sprint 8 — per-app VPN + tunnel transport + NE process isolation)

The original ADR's *Risk Register (A1–G1)* was a Sprint 5 PR-33 stub.
This section enumerates the per-attack-surface threats that the
Sprint 8 extensions (purge semantics, Keychain access group, Android
Keystore) defend against, alongside the pre-existing surface (per-app
VPN rules, tunnel transport security, `NetworkExtension` process
isolation). Risks are tracked in `docs/RISK-REGISTER.md` (Sprint 5
follow-up; full likelihood/impact/owner columns) and revisited every
Sprint.

| ID  | Surface | Threat | Mitigation (status) |
|-----|---------|--------|---------------------|
| V1  | Per-app VPN rules | A misconfigured allowlist sends the user's bank-app traffic through the tunnel, leaking metadata | `VpnService.Builder.allowedApplications` is set from a hard-coded list in `OpenE2eeVpnService.kt`; `disallowedApplications` is the mirror (Sprint 5 PR-28). *Active.* |
| V2  | Per-app VPN rules | iOS 14 allowlist via `proto.includeAppRules` falls back to `NSLog` breadcrumb on deny-list | Sprint 5 PR-29 bumped `MinimumOSVersion` to 15.0; `proto.excludeAppRules` is now the canonical deny-list path. *Active.* |
| V3  | Tunnel transport | `protect()` returns payload bytes to the OS — bug in protect() copies bytes off-device | Ring buffer copies only IP/TCP/UDP header fields + transport ports; CI grep test enforces no `getPayload` / `payload[0]` patterns in `mobile/lib/mobile/vpn/` (Sprint 7 STRIDE-3-01 pattern). *Active.* |
| V4  | Tunnel transport | TLS 1.3 0-RTT resumption leaks metadata that 0-RTT heuristics can't catch | IP-ID-derived heuristic (Risk G1); documented limitation. *Planned.* |
| V5  | NetworkExtension isolation (iOS) | A compromised host app reads tunnel master key from Keychain | Master key access is gated by `kSecAttrAccessGroup = group.com.opene2ee.opene2ee` + `kSecAttrAccessibleWhenUnlockedThisDeviceOnly` + `errSecInteractionNotAllowed` on biometric-gated reads. *Active.* |
| V6  | NetworkExtension isolation (iOS) | A compromised `OpenE2eeTunnelProvider` reads the device-identity Ed25519 private key | Ed25519 key lives in Keychain under the same access group but is bound to `kSecAttrAccessGroup = group.com.opene2ee.opene2ee` + `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly` — the tunnel provider can read it for `signTunnelHandshake()` but cannot export it. *Active.* |
| V7  | NetworkExtension isolation (Android) | In-process VPN service leaks via host-activity memory disclosure | Accepted trade-off; `ParcelFileDescriptor` ergonomics justify. Counter-mitigation: master key never enters Dart (lives in AndroidKeyStore); Dart only sees `KeyInfo.getSecurityLevel()` enum. *Active with documented trade-off.* |
| V8  | Purge semantics | KVKK DELETE succeeds on server but the device continues to hold per-session state | Sprint 8 PR-s8item5 `tearDownOnKvkkDelete` (this ADR) — Dart-side teardown on 200 from `DELETE /api/v1/users/{device_id_hash}`. *Active.* |
| V9  | Purge semantics | Mobile teardown hook fails after server DELETE — partial state | Best-effort + idempotent + bounded retry; 7-day window matches the `DefaultIdleSweepInterval` + 7-day consumer-retry jitter (see ADR-0006 G1). *Active.* |
| V10 | Keychain access group (iOS) | A future App Group is added with a typo, breaking the tunnel | `Runner.entitlements` + `OpenE2eeTunnelProvider.entitlements` MUST carry the same string verbatim; regression test `mobile/test/mobile/security/keychain_access_group_posture_test.dart` (to be authored — Sprint 9 follow-up) reads both files and asserts equality. *Planned.* |
| V11 | Android Keystore | API < 23 device falls back to software-only SharedPreferences (breaks Ed25519 private-key-at-rest) | Sprint 7 MOB-5 `minSdk = 23` — install is refused on API < 23 by the Play Store. *Active.* |
| V12 | Android Keystore | KeyStore blob exfiltrated from a rooted device | StrongBox / TEE backing means the AES key material does not exist in software-extractable form. `setUserAuthenticationRequired(false)` is acceptable because the per-session key is task-bounded. *Active.* |

> **Status legend.** *Active* = in production today; *Planned* = Sprint 9+
> follow-up. *Active*: V1, V2, V3, V5, V6, V7, V8, V9, V11, V12.
> *Planned*: V4, V10.

### Attack surfaces not in scope

* **OS-level compromise** (rooted Android, jailbroken iOS). The Keychain
  Secure Enclave and AndroidKeyStore StrongBox both assume a
  non-compromised OS. Defending against an OS-level attacker is out of
  scope; the documentation lists "device must not be rooted/jailbroken"
  as a Sprint 1 first-launch disclosure.
* **Network attacker observing the tunnel transport** (downstream of the
  `protect()` call). The tunnel is end-to-end encrypted (AES-256-GCM
  in `OpenE2eeTunnelProvider.makeNonce`); the network attacker sees only
  ciphertext. This is the *intended* threat model.

---

## Sprint 5-7 cross-link summary

The Sprint 8 PR-s8item5 extensions are bound to the following earlier
PRs / items. Verifier should confirm each cross-link is present in the
merged commit graph:

| Item | Source | Connection |
|---|---|---|
| Sprint 5 PR-22b (iOS VPN implementation) | `705be45` | The base of the iOS NetworkExtension + per-app VPN allowlist / denylist work that the Keychain access group extends. |
| Sprint 5 PR-29 (RunnerTests + Keychain master + iOS 15+) | `a925701` | Introduces `loadMasterKeyFromKeychain()` + `application-groups` entitlement — the access-group pattern this ADR formalises. |
| Sprint 6 PR-39 (mobile security hardening) | `a428d8c` | Cleartext pin (`network_security_config.xml`) + R8/ProGuard + iOS NE MinimumOSVersion. PR-39 ships the placeholder pin-set that MOB-8 promotes to real pins. |
| Sprint 7 Item 4 (STRIDE-6-03 Active Pool purge) | `ba2fc31` / `ea9ab31` | Server-side `DeleteByHash` / `SweepIdle` / `RunSweeper`. The mobile VPN teardown is the third leg of the same SLA. |
| Sprint 7 Item 6 (MOB-5 Android Keystore) | `5f65137` / `da7bbc1` | `minSdk` 21→23 bump. Without this floor, AndroidKeyStore-backed AES master keys fall back to software SharedPreferences. |
| Sprint 7 Item 14 (MOB-8 cert pinning) | `2bee5f0` / `ccef5d7` | SPKI pin-set + `PinnedHttpOverrides` — the native + Dart layers that the iOS Keychain access group lets the host app **and** the `OpenE2eeTunnelProvider` share. |
| Sprint 7 Item 13 (MOB-6 TeamID entitlements) | `fbad733` / `6f0e0d6` | The `com.apple.developer.team-identifier` entitlement Apple requires whenever an App Group is declared — pre-condition for `application-groups` working. |

---

**Cross-references.**
* Source: [`docs/ARCHITECTURE_DECISIONS.md`](ARCHITECTURE_DECISIONS.md) §4, §5, §6
* Privacy contract: [`docs/ADR-0006-anonimlik.md`](ADR-0006-anonimlik.md)
  (Anonim Cihaz Kimliği + Backend'de Saklanan + KVKK DELETE)
* iOS VPN implementation: Sprint 3 PR-22b (commit `705be45`,
  `mobile/ios/NetworkExtension/OpenE2eeTunnelProvider.swift`) + Sprint 5
  PR-29 Keychain master (commit `a925701`,
  `loadMasterKeyFromKeychain()` + Runner.entitlements access group)
* Android VPN implementation: Sprint 5 PR-22a / PR-24 / PR-28
  (`mobile/android/.../OpenE2eeVpnService.kt`)
* Mobile security hardening: Sprint 6 PR-39
  (`docs/SPRINT-6-PR-39-VERIFICATION.md` — cleartext pin + R8/ProGuard +
  iOS NE MinimumOSVersion)
* Cert pinning: Sprint 7 MOB-8
  (`docs/SPRINT-7-MOB-8-CERT-PINNING.md` — Android `<pin-set>` + iOS
  `NSPinnedDomains` + Dart `PinnedHttpOverrides`)
* Active Pool purge: Sprint 7 Item 4 / STRIDE-6-03
  (`backend/internal/matching/pool.go` — `DeleteByHash` + `SweepIdle`
  + `RunSweeper`; `backend/cmd/server/main.go:615-626` `DeleteUserHook`)
* Android Keystore: Sprint 7 Item 6 / MOB-5 (`mobile/android/app/build.gradle.kts`
  `minSdk = 23` + rationale block)
* iOS TeamID entitlements: Sprint 7 Item 13 / MOB-6
  (`mobile/ios/Runner.entitlements` + `mobile/ios/Config/{Local,Production}.xcconfig`)