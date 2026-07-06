# ADR-0006 — Anonimlik / Veri Minimizasyonu (Privacy & Data Minimisation)

| Field      | Value                                  |
|------------|----------------------------------------|
| **Status** | Accepted (Sprint 1) — Stub             |
| **Date**   | 2026-07-06                             |
| **Owner**  | Architect (mvs_25a7a987f73243899e35a1485c6ba224) |
| **Source** | This ADR is a stub extracted from [`docs/ARCHITECTURE_DECISIONS.md`](ARCHITECTURE_DECISIONS.md) §5 ("Regülasyon Uyumu ve Mağaza Politikaları"). Future contributors should expand it with the full Anonim Cihaz Kimliği / Backend'de Saklanan / KVKK DELETE sections referenced from `backend/internal/storage/interfaces.go` and `mobile/lib/shared/device_identity.dart`. |

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

**Cross-references.**
* Source: [`docs/ARCHITECTURE_DECISIONS.md`](ARCHITECTURE_DECISIONS.md) §5
* VPN layer that enforces the metadata-only invariant on-device:
  [`docs/ADR-0003-vpn-layer.md`](ADR-0003-vpn-layer.md)
* Mobile device identity: `mobile/lib/shared/device_identity.dart`
* Backend storage contract: `backend/internal/storage/interfaces.go`