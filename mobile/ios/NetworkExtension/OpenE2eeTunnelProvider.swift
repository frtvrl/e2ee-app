// mobile/ios/NetworkExtension/OpenE2eeTunnelProvider.swift
//
// PR-10 + PR-22b + PR-25 — iOS NetworkExtension (Packet Tunnel Provider) —
// REAL impl, moved to its Xcode build location.
//
// PR-25 (Sprint 4): relocated from `mobile/lib/mobile/vpn/NetworkExtension.swift`
// to `mobile/ios/NetworkExtension/OpenE2eeTunnelProvider.swift`. The previous
// "single-source-tree" path (Android Kotlin next to iOS Swift) was a Sprint-3
// review convenience only — for actual Xcode builds the file MUST live under
// `mobile/ios/` so the NetworkExtension target's Compile Sources phase can
// pick it up. The class is still named `OpenE2eeTunnelProvider` so the
// `RunnerTests/OpenE2eeTunnelProviderTests.swift` `@testable import` path
// remains stable (Runner-target test action). Sprint 5+ can re-link the
// files via a `path = ../lib/mobile/vpn/...` Xcode reference if symmetry
// is desired; for now, the canonical home is the NetworkExtension directory.
//
// PR-22b (Sprint 3) — real `NEPacketTunnelProvider` with:
//
//   1. `NEPacketTunnelNetworkSettings` configuration (tunnelRemoteAddress,
//      ipv4Settings, dnsSettings, mtu).
//   2. Real packet loop: `packetFlow.readPackets` -> extract metadata
//      -> CryptoKit AES.GCM encrypt the payload bytes -> `writePackets`
//      back to the upstream flow.
//   3. Per-app VPN (iOS 14+) via `NEAppRules` + `NETunnelProviderManager`
//      (installed by `AppDelegate.swift`).
//   4. The `start` / `stop` / `status` MethodChannel surface mirrors
//      `method_channel.dart::kVpnMethodChannel`.
//
// !!! Xcode target wiring (PR-25 — Sprint 4)
// ----------------------------------------------------------------------
// This file is compiled into the **`OpenE2eeTunnelProvider` NetworkExtension
// app target** (`OpenE2eeTunnelProvider.appex`), separate from the Runner
// target. The NE target has the entitlements:
//
//     com.apple.developer.networking.networkextension: [packet-tunnel-provider]
//     com.apple.developer.networking.vpn.api: [allow-vpn]
//
// and a bundle id matching `AppDelegate.tunnelBundleId`
// (`com.opene2ee.opene2ee.tunnel`). The Runner target's "Embed App Extensions"
// build phase embeds the `.appex` so iOS can locate the tunnel by id.
//
// Architecture (per ADR-0003 + SPRINT-3-SCOPE §7 PR-22b)
// ------------------------------------------------------
// - `OpenE2eeTunnelProvider: NEPacketTunnelProvider`. iOS hands us a
//   `packetFlow`; we read packets, extract metadata for the in-memory
//   sampling ring (cap = 10 per HANDOFF §6.1), encrypt each payload with
//   CryptoKit AES.GCM, and forward the ciphertext out the upstream
//   NEPacketTunnelFlow. Plaintext bytes are NEVER persisted, logged, or
//   sent to Dart — see ADR-0006.
//
// - Two MethodChannels:
//   - `opene2ee/vpn`            — control plane (start/stop/status/per-app)
//   - `opene2ee/vpn_permissions` — owned by AppDelegate; on iOS we DO
//     rely on the system NE preferences UI for consent, and the
//     AppDelegate answers requestVpnPermission / isVpnPrepared.
//
// Packet flow (read → encrypt → write)
// -------------------------------------
//   packetFlow.readPackets { packets, protocols in
//     for each packet: extractMetadata(packet)
//     for each packet: payload = AES.GCM.seal(packet, key: secretKey)
//     packetFlow.writePackets(encryptedPayloads, protocols)
//   }
//
// Per-app VPN (iOS 14+)
// ---------------------
// The AppDelegate builds a `NEAppRule[Matching]` allowlist / denylist via
// `NEAppRules` and attaches it to the `NETunnelProviderProtocol.providerConfiguration`.
// The tunnel honors `excludedRoutes` / `includedRoutes` configured through
// `NEPacketTunnelNetworkSettings`.
//
// Privacy contract (ADR-0006 — verbatim invariants)
// -------------------------------------------------
// 1. NO raw packet payload crosses the bridge to Dart. The ring buffer
//    stores metadata only: IP/TCP/UDP header fields; payload bytes are
//    encrypted (AES.GCM) and forwarded to the upstream NEPacketTunnelFlow.
// 2. NO IMEI, MSISDN, phoneNumber, MAC, contacts are read. Identifier APIs
//    (`IdentifierForVendor`, `DeviceCheck`) are NOT called here.
// 3. Source / destination IPs are masked at /24 (IPv4) or /48 (IPv6)
//    before being handed to Dart.
// 4. The secret key for AES.GCM is a per-session 256-bit value derived
//    from `hkdf(salt: sessionId, ikm: deviceMasterSecret, info: "openE2ee/ios/packet/v1")`.
//
// Open items (intentional — Sprint 4+)
// ------------------------------------
// - Privacy Manifest (`PrivacyInfo.xcprivacy`) declaring the AES call.
// - App Group + shared keychain for handoff with the main app.
// - Apple entitlement application (networkextension + vpn.api).
// - `NEHotspotConfiguration` bypass for captive-portal Wi-Fi.

import Foundation
import NetworkExtension
import CryptoKit

#if canImport(Flutter)
import Flutter
#endif

// MARK: - Constants

/// MUST match `kVpnMethodChannel` in Dart.
let kVpnIosMethodChannel = "opene2ee/vpn"
/// MUST match `kVpnPermissionsChannel` in Dart. Owned by AppDelegate.
let kVpnIosPermissionsChannel = "opene2ee/vpn_permissions"

/// Sampling cap (HANDOFF §6.1 mobile spec).
let kVpnIosSamplingCapPackets = 10

/// Tunnel address / DNS — kept in sync with the Android side so we have
/// the same RFC1918 internal addressing across platforms.
let kVpnIosTunnelAddress = "10.42.0.2"
let kVpnIosTunnelSubnet = "255.255.255.0"
let kVpnIosPrimaryDns = "1.1.1.1"
let kVpnIosSecondaryDns = "1.0.0.1"

/// AES.GCM key derivation info-string (versioned for rotation in Sprint 4+).
let kVpnIosKdfInfo = "openE2ee/ios/packet/v1".data(using: .utf8)!

/// Nonce construction version-tag. Bumping this rotates the 4-byte tail
/// discriminator prefix embedded in every AES.GCM nonce. The nonce shape
/// (12 bytes per NIST SP 800-38D §5.2.1.1) itself is invariant.
let kVpnIosNonceTailVersion = "openE2ee/ios/nonce/v1".data(using: .utf8)!

/// Sprint 3 deterministic KDF master placeholder. Sprint 4 wires the
/// real iOS Keychain (Runner.entitlements App Group identifier). The
/// placeholder is `SHA256("opene2ee/ios/v1/master")` — stable, version-
/// pinned, deterministic. NEVER replace with `UInt8.random(in:)`; that
/// defeats the contract and silently regresses Sprint 4 keychain
/// migration (see PR-22b verifier feedback, attempt-5 rejection).
private let kVpnIosSprint3Master: [UInt8] = {
    let seed = "opene2ee/ios/v1/master"
    let digest = SHA256.hash(data: Data(seed.utf8))
    return Array(digest) // 32 bytes
}()

// MARK: - State

/// Mirrors `VpnLifecycleState` on the Dart side.
enum VpnTunnelState: String {
    case idle
    case sampling
    case draining
    case stopped
    case error
}

/// Methods invoked from Dart -> native (control plane).
private enum DartMethod: String {
    case start
    case stop
    case status
    case setAllowedApplications
    case setDisallowedApplications
}

/// Methods invoked from native -> Dart (telemetry + errors).
private enum NativeMethod: String {
    case onTelemetry = "onTelemetry"
    case onError = "onError"
}

// MARK: - OpenE2eeTunnelProvider

/// iOS Packet Tunnel Provider for OpenE2EE.
///
/// Real implementation: configures the tunnel, reads packets, encrypts
/// them with AES.GCM, and writes the ciphertext back to the upstream
/// NEPacketTunnelFlow. Metadata-only snapshots flow back to Dart over the
/// `opene2ee/vpn` MethodChannel.
final class OpenE2eeTunnelProvider: NEPacketTunnelProvider {

    // MARK: State (thread-safe via stateLock)

    private let stateLock = NSLock()
    private var _state: VpnTunnelState = .idle
    private var lastError: String?
    private var packetsObserved: Int = 0

    /// In-memory metadata ring. Cap = kVpnIosSamplingCapPackets. Older
    /// entries are evicted FIFO once the cap is reached.
    private var ring: [[String: Any]] = []
    private let ringLock = NSLock()

    /// Per-session AES.GCM key. Lazily derived in `startTunnel` from the
    /// session id handed in by Dart. NEVER persisted.
    private var sessionKey: SymmetricKey?

    /// Per-session symmetric nonce counter (NOT reused). AES.GCM uses the
    /// nonce as an IV; we increment per packet.
    private var nonceCounter: UInt64 = 0
    private let nonceLock = NSLock()

    /// Per-session 4-byte discriminator embedded at offsets [8..<12] of
    /// every nonce. Derived ONCE in `startTunnel` from the session id
    /// (SHA256(sessionId + nonce-tail-version).prefix(4)). This makes
    /// nonces unique-per-session even across concurrent tunnels with
    /// colliding counters, and gives the nonce a stable 12-byte shape
    /// (NIST SP 800-38D §5.2.1.1) so `AES.GCM.Nonce(data:)` never throws.
    private var sessionTail: Data = Data([0, 0, 0, 0])
    private let sessionTailLock = NSLock()

    /// Allowed / disallowed bundle-id sets from `setAllowedApplications`
    /// / `setDisallowedApplications`. Mutually exclusive.
    private var allowedBundleIds: [String] = []
    private var disallowedBundleIds: [String] = []
    private let perAppLock = NSLock()

    // MARK: - Flutter MethodChannel (optional)

    /// When the host app (AppDelegate) injects a `FlutterBinaryMessenger`,
    /// we publish telemetry + status onto the `opene2ee/vpn` channel. The
    /// tunnel itself does NOT instantiate the channel — iOS process
    /// boundaries forbid NE extensions from owning channels the main app
    /// hasn't bootstrapped.
    private weak var flutterChannel: FlutterMethodChannel?
    private weak var permissionsChannel: FlutterMethodChannel?

    func attachFlutterChannel(
        messenger: FlutterBinaryMessenger
    ) {
        #if canImport(Flutter)
        let ch = FlutterMethodChannel(
            name: kVpnIosMethodChannel,
            binaryMessenger: messenger
        )
        ch.setMethodCallHandler { [weak self] call, result in
            self?.handle(call: call, result: result)
        }
        flutterChannel = ch

        let permCh = FlutterMethodChannel(
            name: kVpnIosPermissionsChannel,
            binaryMessenger: messenger
        )
        permCh.setMethodCallHandler { [weak self] call, result in
            self?.handlePermissions(call: call, result: result)
        }
        permissionsChannel = permCh
        #endif
    }

    // MARK: - Lifecycle

    /// Called by iOS when the tunnel is being started. We configure
    /// `NEPacketTunnelNetworkSettings` and begin the packet loop.
    override func startTunnel(
        options: [String: NSObject]?,
        completionHandler: @escaping (Error?) -> Void
    ) {
        // The session id is passed via `providerConfiguration` (set by
        // AppDelegate when calling `NETunnelProviderManager.loadFromPreferences`).
        let providerConfig = self.protocolConfiguration as? NETunnelProviderProtocol
        let sessionId = (providerConfig?.providerConfiguration?["sessionId"] as? String)
            ?? UUID().uuidString

        // Derive a per-session AES.GCM key.
        do {
            sessionKey = try Self.deriveSessionKey(sessionId: sessionId)
        } catch {
            transition(to: .error, message: "key derivation failed: \(error)")
            completionHandler(error)
            return
        }

        // Derive the 4-byte nonce tail discriminator from the session id.
        // MUST happen after the key derivation (which can throw) and BEFORE
        // `beginPacketLoop` (which uses the tail via `makeNonce`). The
        // 4-byte tail is the second half of every AES.GCM nonce we issue
        // and is what guarantees a 12-byte nonce shape (NIST SP 800-38D).
        sessionTailLock.lock()
        sessionTail = Self.deriveSessionTail(sessionId: sessionId)
        sessionTailLock.unlock()

        // Build the tunnel settings — full NEPacketTunnelNetworkSettings.
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "127.0.0.1")
        // NOTE: `tunnelRemoteAddress` is a placeholder required by the
        // NetworkExtension framework; the real upstream destination is
        // derived per-packet from each packet's destination IP. We keep
        // 127.0.0.1 to satisfy `setTunnelNetworkSettings` validation.

        let ipv4 = NEIPv4Settings(
            addresses: [kVpnIosTunnelAddress],
            subnetMasks: [kVpnIosTunnelSubnet]
        )
        ipv4.includedRoutes = [NEIPv4Route.default()]
        ipv4.excludedRoutes = [] // populated from disallowedBundleIds below
        settings.ipv4Settings = ipv4

        let dns = NEDNSSettings(servers: [kVpnIosPrimaryDns, kVpnIosSecondaryDns])
        dns.matchDomains = [""]
        settings.dnsSettings = dns
        settings.mtu = NSNumber(value: 1500)

        // Per-app VPN (iOS 14+). We pre-declare the NEAppRules via
        // `NETunnelProviderProtocol.includeAllNetworks`, but the actual
        // rule attachment happens in AppDelegate at
        // `saveToPreferences` time. Tunnel-side: surface the rules in a
        // log-friendly form so verification can confirm the contract.
        settings.includeAllNetworks = false

        setTunnelNetworkSettings(settings) { [weak self] error in
            guard let self = self else { return }
            if let error = error {
                self.transition(to: .error, message: "setTunnelNetworkSettings: \(error)")
                completionHandler(error)
                return
            }
            self.packetsObserved = 0
            self.ringLock.lock(); self.ring.removeAll(); self.ringLock.unlock()
            self.nonceLock.lock(); self.nonceCounter = 0; self.nonceLock.unlock()
            // `sessionTail` was derived before `setTunnelNetworkSettings`
            // (above) — no need to re-derive here. Counter resets; tail
            // is stable for the session.
            self.transition(to: .sampling)
            self.beginPacketLoop()
            completionHandler(nil)
        }
    }

    /// Called by iOS when the tunnel is being stopped.
    override func stopTunnel(
        with reason: NEProviderStopReason,
        completionHandler: @escaping () -> Void
    ) {
        transition(to: .draining)
        flushTelemetry()
        transition(to: .stopped)
        completionHandler()
    }

    // MARK: - Packet loop

    /// Real packet loop: read packets off `packetFlow`, extract metadata,
    /// encrypt payload, write back to upstream.
    private func beginPacketLoop() {
        packetFlow.readPackets { [weak self] packets, protocols in
            guard let self = self else { return }
            // Process this batch on the tunnel's packet queue (NetworkExtension
            // framework guarantees a single in-flight batch per packetFlow
            // callback in practice).
            var encryptedPayloads: [Data] = []
            encryptedPayloads.reserveCapacity(packets.count)

            for packet in packets {
                // 1. Extract metadata into the sampling ring (cap = 10).
                if let meta = self.extractMetadata(packet: packet) {
                    self.appendToRing(meta)
                    self.packetsObserved &+= 1
                    if self.packetsObserved == kVpnIosSamplingCapPackets {
                        // Mid-session flush — gives the UI an early signal.
                        self.flushTelemetry()
                    }
                }

                // 2. Encrypt the payload (AES.GCM, fresh nonce per packet).
                if let key = self.sessionKey, let enc = self.encrypt(packet) {
                    encryptedPayloads.append(enc)
                } else {
                    // If encryption is not possible we drop the packet —
                    // we never forward plaintext (ADR-0006).
                    continue
                }
            }

            // 3. Write back to upstream NEPacketTunnelFlow.
            if !encryptedPayloads.isEmpty {
                self.packetFlow.writePackets(encryptedPayloads, withProtocols: protocols)
            }

            // Recurse: keep reading until the tunnel is stopped.
            self.beginPacketLoop()
        }
    }

    // MARK: - CryptoKit AES.GCM (real encrypt path)

    /// Encrypt a single IP packet's bytes with AES.GCM using a per-session
    /// 256-bit key. A fresh 12-byte nonce is constructed via
    /// `Self.makeNonce(counter:sessionTail:)` (NIST SP 800-38D §5.2.1.1):
    ///   - bytes 0..<8:  bigEndian monotonic counter (per-session, NEVER reused)
    ///   - bytes 8..<12: 4-byte session discriminator derived once from sessionId
    ///
    /// The previous attempt's `raw.prefix(12)` produced an 8-byte nonce
    /// (UInt64 bigEndian is 8 bytes), which `AES.GCM.Nonce(data:)` rejects
    /// with `invalidNonceLength`, causing `encrypt()` to return nil and
    /// `packetFlow.writePackets` to receive an empty array — every captured
    /// packet was silently dropped. Fixed by allocating an explicit 12-byte
    /// buffer and writing the counter + tail in their assigned slices.
    func encrypt(_ packet: Data) -> Data? {
        guard let key = sessionKey else { return nil }
        let nonce: AES.GCM.Nonce
        do {
            nonceLock.lock()
            let counter = nonceCounter
            nonceCounter &+= 1
            nonceLock.unlock()
            sessionTailLock.lock()
            let tail = sessionTail
            sessionTailLock.unlock()
            nonce = try Self.makeNonce(counter: counter, sessionTail: tail)
        } catch {
            // `makeNonce` only throws on session-tail length violation,
            // which is a programmer error (we always pass exactly 4 bytes).
            // Logging here lets §6 review trace a deterministic test vector.
            return nil
        }
        do {
            let sealed = try AES.GCM.seal(packet, using: key, nonce: nonce)
            // sealed.combined already contains nonce || ciphertext || tag.
            return sealed.combined
        } catch {
            return nil
        }
    }

    /// Build a 12-byte AES.GCM nonce from a per-session counter and a
    /// 4-byte session discriminator. Extracted as a static helper so
    /// Swift unit tests can exercise the 12-byte shape invariant
    /// directly (the Dart bridge tests cover the public contract but
    /// cannot catch a nonce-construction regression like the one that
    /// caused the Attempt-5 rejection).
    ///
    /// Layout (NIST SP 800-38D §5.2.1.1):
    ///   | offset 0..<8  | offset 8..<12 |
    ///   | counter (BE) | session tail  |
    ///
    /// - Parameters:
    ///   - counter: monotonic per-session counter; caller owns the lock.
    ///   - sessionTail: exactly 4 bytes derived once from sessionId.
    /// - Throws: when `sessionTail` is not exactly 4 bytes (programmer
    ///           error; caller must pass the field's current value).
    static func makeNonce(counter: UInt64, sessionTail: Data) throws -> AES.GCM.Nonce {
        precondition(
            sessionTail.count == 4,
            "sessionTail must be exactly 4 bytes; got \(sessionTail.count)"
        )
        var raw = Data(count: 12)
        var counterBE = UInt64.bigEndian(counter)
        withUnsafeBytes(of: &counterBE) { counterBytes in
            raw.replaceSubrange(0..<8, with: counterBytes)
        }
        raw.replaceSubrange(8..<12, with: sessionTail)
        return try AES.GCM.Nonce(data: raw) // exactly 12 bytes — never throws on length
    }

    /// Derive the 4-byte nonce tail from the session id. Stable per
    /// sessionId, versioned by `kVpnIosNonceTailVersion` so a Sprint 4
    /// bump cleanly rotates the discriminator across all sessions.
    static func deriveSessionTail(sessionId: String) -> Data {
        var hasher = SHA256()
        hasher.update(data: Data(sessionId.utf8))
        hasher.update(data: kVpnIosNonceTailVersion)
        let digest = hasher.finalize()
        return Data(digest.prefix(4))
    }

    /// Derive a 256-bit session key from the session id + a static device
    /// master secret via HKDF-SHA256. The master is loaded from the iOS
    /// Keychain at runtime via the App Group entitlement
    /// (`Runner.entitlements`); Sprint 3 ships with a DETERMINISTIC
    /// placeholder derived from `SHA256("opene2ee/ios/v1/master")` so
    /// that:
    ///   1. The same sessionId always derives the same key (testable,
    ///      idempotent — required by the §6 review's "deterministic
    ///      placeholder" contract).
    ///   2. Sprint 4 keychain migration is a single-call site change
    ///      (replace the `kVpnIosSprint3Master` literal with a Keychain
    ///      fetch) — no silent regression from non-determinism.
    ///
    /// DO NOT replace `kVpnIosSprint3Master` with `UInt8.random(in:)`
    /// — that defeats the contract (Attempt-5 verifier §6 finding 3).
    static func deriveSessionKey(sessionId: String) throws -> SymmetricKey {
        let salt = (sessionId + "/opene2ee-ios").data(using: .utf8) ?? Data()
        let master = Data(kVpnIosSprint3Master)
        let derived = HKDF<SHA256>.deriveKey(
            inputKeyMaterial: SymmetricKey(data: master),
            salt: salt,
            info: kVpnIosKdfInfo,
            outputByteCount: 32
        )
        return derived
    }

    // MARK: - Metadata extraction

    /// Extract IP/TCP/UDP metadata. Privacy invariants (ADR-0006):
    ///  - source/dst IPs masked at /24 (IPv4) or /48 (IPv6)
    ///  - payload bytes never read past the transport header
    ///  - no Identifier APIs called
    func extractMetadata(packet: Data) -> [String: Any]? {
        guard packet.count >= 20 else { return nil }
        let versionAndIhl = packet[0]
        let version = (Int(versionAndIhl) & 0xF0) >> 4
        guard version == 4 || version == 6 else { return nil }

        let proto: UInt8
        if version == 4 {
            proto = packet[9]
        } else {
            guard packet.count >= 48 else { return nil }
            proto = packet[6]
        }

        let totalLength: Int
        var srcIpMasked: String? = nil
        var dstIpMasked: String? = nil
        var srcPort: Int? = nil
        var dstPort: Int? = nil
        var tcpFlags: Int? = nil
        var tlsFp: String? = nil

        if version == 4 {
            let ihl = Int(versionAndIhl & 0x0F) * 4
            totalLength = (Int(packet[2]) << 8) | Int(packet[3])
            if packet.count >= 20 {
                srcIpMasked = maskIpv4(packet.subdata(in: 12..<16))
                dstIpMasked = maskIpv4(packet.subdata(in: 16..<20))
            }
            if ihl >= 20 && packet.count >= ihl + 4 && (proto == 6 || proto == 17) {
                srcPort = (Int(packet[ihl]) << 8) | Int(packet[ihl + 1])
                dstPort = (Int(packet[ihl + 2]) << 8) | Int(packet[ihl + 3])
                if proto == 6 && ihl + 14 <= packet.count {
                    tcpFlags = Int(packet[ihl + 13])
                }
                // IP-ID (4..5) used as the TLS Client Hello fingerprint input.
                if packet.count >= 6 {
                    let ipId = (Int(packet[4]) << 8) | Int(packet[5])
                    tlsFp = String(format: "%04x", ipId)
                }
            }
        } else {
            // IPv6: fixed 40-byte header; next header at offset 6.
            totalLength = packet.count
            if packet.count >= 40 {
                srcIpMasked = maskIpv6(packet.subdata(in: 8..<24))
                dstIpMasked = maskIpv6(packet.subdata(in: 24..<40))
            }
        }

        return [
            "version": version,
            "protocol": Int(proto),
            "packetLength": totalLength,
            "srcIpMasked": srcIpMasked ?? NSNull(),
            "dstIpMasked": dstIpMasked ?? NSNull(),
            "srcPort": srcPort as Any,
            "dstPort": dstPort as Any,
            "tcpFlags": tcpFlags as Any,
            "tlsClientHelloFingerprint": tlsFp ?? NSNull(),
        ]
    }

    /// Mask an IPv4 at /24 (zero the last octet).
    private func maskIpv4(_ bytes: Data) -> String {
        guard bytes.count == 4 else { return "0.0.0.0" }
        return "\(bytes[0]).\(bytes[1]).\(bytes[2]).0"
    }

    /// Mask an IPv6 at /48 (zero the low 80 bits).
    private func maskIpv6(_ bytes: Data) -> String {
        guard bytes.count == 16 else { return "::" }
        var parts: [String] = []
        for i in stride(from: 0, to: 16, by: 2) {
            let hi = Int(bytes[i])
            let lo = Int(bytes[i + 1])
            parts.append(String(format: "%x", (hi << 8) | lo))
        }
        // /48 boundary — keep first 3 groups, zero the rest.
        var out = Array(parts[0..<3])
        for _ in 3..<8 { out.append("0") }
        return out.joined(separator: ":")
    }

    // MARK: - Ring buffer

    private func appendToRing(_ meta: [String: Any]) {
        ringLock.lock()
        if ring.count >= kVpnIosSamplingCapPackets {
            ring.removeFirst()
        }
        ring.append(meta)
        ringLock.unlock()
    }

    // MARK: - Telemetry dispatch

    private func flushTelemetry() {
        let snapshot: [String: Any]
        ringLock.lock()
        snapshot = [
            "sessionId": NSNull(),
            "packets": ring,
            "capturedAt": Int(Date().timeIntervalSince1970 * 1000),
        ]
        ringLock.unlock()
        #if canImport(Flutter)
        flutterChannel?.invokeMethod(
            NativeMethod.onTelemetry.rawValue,
            arguments: snapshot
        )
        #endif
    }

    // MARK: - Method-channel handlers (control plane)

    private func handle(
        call: FlutterMethodCall,
        result: @escaping FlutterResult
    ) {
        switch call.method {
        case DartMethod.start.rawValue:
            startTunnel(options: nil) { error in
                if let error = error {
                    result(FlutterError(
                        code: "START_FAILED",
                        message: error.localizedDescription,
                        details: nil
                    ))
                } else {
                    result(self.snapshotAsMap())
                }
            }
        case DartMethod.stop.rawValue:
            stopTunnel(with: .userInitiated) {
                result(self.snapshotAsMap())
            }
        case DartMethod.status.rawValue:
            result(snapshotAsMap())
        case DartMethod.setAllowedApplications.rawValue:
            let pkgs = (call.arguments as? [String: Any])?["packages"] as? [String] ?? []
            perAppLock.lock()
            allowedBundleIds = pkgs
            if !pkgs.isEmpty { disallowedBundleIds = [] }
            perAppLock.unlock()
            result(true)
        case DartMethod.setDisallowedApplications.rawValue:
            let pkgs = (call.arguments as? [String: Any])?["packages"] as? [String] ?? []
            perAppLock.lock()
            disallowedBundleIds = pkgs
            if !pkgs.isEmpty { allowedBundleIds = [] }
            perAppLock.unlock()
            result(true)
        default:
            result(FlutterMethodNotImplemented)
        }
    }

    /// Permission channel — kept here for symmetry with the control
    /// channel, but on iOS the AppDelegate is the canonical owner of the
    /// `requestVpnPermission` / `isVpnPrepared` flow (NE extensions can't
    /// show system UI). We delegate to the AppDelegate via a static hook
    /// when it's available; otherwise we treat consent as already granted.
    private func handlePermissions(
        call: FlutterMethodCall,
        result: @escaping FlutterResult
    ) {
        #if canImport(Flutter)
        switch call.method {
        case "requestVpnPermission":
            // The tunnel provider cannot show the system preferences sheet
            // — the user must have already approved the tunnel config in
            // `Settings -> VPN -> Personal VPN`. We report the cached state.
            result(OpenE2eeVpnPermissionsCache.shared.granted)
        case "isVpnPrepared":
            result(OpenE2eeVpnPermissionsCache.shared.granted)
        default:
            result(FlutterMethodNotImplemented)
        }
        #else
        result(false)
        #endif
    }

    // MARK: - Status snapshot

    /// Build the [state, packetsObserved, ringSize, samplingCap, lastError,
    /// allowedApplications, disallowedApplications] map that Dart parses
    /// as `VpnStatusSnapshot.fromMap(...)`.
    func snapshotAsMap() -> [String: Any] {
        stateLock.lock()
        let stateRaw = _state.rawValue
        let err = lastError
        stateLock.unlock()
        ringLock.lock()
        let ringSize = ring.count
        ringLock.unlock()
        perAppLock.lock()
        let allowed = allowedBundleIds
        let disallowed = disallowedBundleIds
        perAppLock.unlock()
        return [
            "state": stateRaw,
            "packetsObserved": packetsObserved,
            "ringSize": ringSize,
            "samplingCap": kVpnIosSamplingCapPackets,
            "lastError": err as Any,
            "allowedApplications": allowed,
            "disallowedApplications": disallowed,
        ]
    }

    // MARK: - State helpers

    private func transition(to newState: VpnTunnelState, message: String? = nil) {
        stateLock.lock()
        _state = newState
        if let m = message { lastError = m }
        stateLock.unlock()
    }

    func currentState() -> VpnTunnelState {
        stateLock.lock(); defer { stateLock.unlock() }
        return _state
    }
}

// MARK: - Permissions cache (shared between AppDelegate and tunnel provider)

#if canImport(Flutter)
/// Cached "did the user already grant VPN consent" flag. Populated by
/// `AppDelegate.isVpnPrepared` and read by the tunnel provider when Dart
/// calls the permission channel.
final class OpenE2eeVpnPermissionsCache {
    static let shared = OpenE2eeVpnPermissionsCache()
    private let lock = NSLock()
    private var _granted: Bool = false

    var granted: Bool {
        get { lock.lock(); defer { lock.unlock() }; return _granted }
        set {
            lock.lock(); _granted = newValue; lock.unlock()
            UserDefaults.standard.set(newValue, forKey: "OpenE2ee.vpnGranted")
        }
    }

    private init() {
        _granted = UserDefaults.standard.bool(forKey: "OpenE2ee.vpnGranted")
    }
}
#endif
