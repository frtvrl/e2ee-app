# ADR-0003 — VPN Layer (Android VpnService + iOS NetworkExtension)

| Field      | Value                                  |
|------------|----------------------------------------|
| **Status** | Accepted (Sprint 3) — Stub             |
| **Date**   | 2026-07-06                             |
| **Owner**  | Architect (mvs_25a7a987f73243899e35a1485c6ba224) |
| **Source** | This ADR is a stub extracted from [`docs/ARCHITECTURE_DECISIONS.md`](ARCHITECTURE_DECISIONS.md). Future contributors should expand it with the full Risk Register (A1–G1) and per-platform implementation details. |

> **Scope notice.** This is a Sprint 4 PR-26 placeholder created to resolve
> the broken `docs/ADR-0003-vpn-layer.md` cross-references flagged by the
> Sprint 3 Integration Gate. The stub captures the architectural *Context*,
> *Decision* and *Consequences* already documented in
> `docs/ARCHITECTURE_DECISIONS.md` §4 (MVP Kapsamı) and §5 (Regülasyon
> Uyumu). It is **not** a substitute for the full ADR-0003 design record —
> the VPN Risk Register (risks A1–G1: Apple Store Review, B2 Android 14+
> foregroundServiceType, G1 TLS 1.3 0-RTT, etc.) remains to be authored in
> a follow-up.

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

**Cross-references.**
* Source: [`docs/ARCHITECTURE_DECISIONS.md`](ARCHITECTURE_DECISIONS.md) §4, §5, §6
* Privacy contract: [`docs/ADR-0006-anonimlik.md`](ADR-0006-anonimlik.md)
* iOS counterpart: [`docs/SPRINT-3-SCOPE.md`](SPRINT-3-PLAN-TEMPLATE.md) §7 PR-22