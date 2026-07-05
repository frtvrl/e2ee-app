// mobile/lib/mobile/vpn/NetworkExtension.swift
//
// PR-10: Mobile-only — iOS NetworkExtension skeleton (Swift).
//
// !!! Code-review artifact for Sprint 1 — no native build in this PR !!!
// ----------------------------------------------------------------------
// File location: this file lives under `mobile/lib/mobile/vpn/` because
// the task brief asked for it there (single source tree, easy code review).
// It is NOT compiled by Xcode in this PR.
//
// When the iOS target is wired up, this file MUST be moved to:
//     mobile/ios/NetworkExtension/OpenE2eeTunnelProvider.swift
// inside a NetworkExtension app target with the entitlements:
//     com.apple.developer.networking.networkextension: [packet-tunnel-provider]
//     com.apple.developer.networking.vpn.api: [allow-vpn]
// and a Privacy Manifest declaring the data-flow reasons (Apple requires
// NSPrivacyAccessedAPITypes declarations for FILE_TIMESTAMP + USER_DEFAULTS
// if used).
//
// Architecture (per ADR-0003 + HANDOFF §4.2 PR-10)
// ------------------------------------------------
// - Subclass of `NEPacketTunnelProvider`. iOS hands us a `NEPacketTunnelFlow`
//   (read + write); we forward packets to the real network, and BEFORE
//   forwarding we copy a *metadata-only* fingerprint of each packet into
//   an in-memory ring buffer (cap = 10 packets).
// - Channel name: "opene2ee/vpn" (must match the Dart-side constant in
//   `mobile/lib/mobile/vpn/method_channel.dart::kVpnMethodChannel`).
// - Methods exposed to Dart (FlutterMethodChannel):
//       "start"  → begin packet capture (after user consent in NE prefs UI)
//       "stop"   → flush ring + tear down tunnel
//       "status" → returns current state (idle | sampling | draining | stopped)
// - Methods invoked from native → Dart (telemetry callback):
//       "onTelemetry" → invoked with metadata summary once 10 packets are
//                       observed, or when the session is force-stopped.
//
// Info.plist (deployed, not the file on disk)
// ------------------------------------------
// Add to the Runner target's Info.plist (and the NetworkExtension target's
// Info.plist — both display the same usage description to the user):
//
//   <key>NSVPNUsageDescription</key>
//   <string>OpenE2EE performs network diagnostics to evaluate encryption quality on your device. No content is read.</string>
//
// Apple requires NSVPNUsageDescription for any app that uses Personal VPN
// or NetworkExtension APIs. The wording is fixed by this brief; any future
// change requires an ADR amendment (see ADR-0003 risk A1 — Apple
// entitlement is sensitive to misleading usage descriptions).
//
// Privacy contract (ADR-0006 — verbatim)
// --------------------------------------
// 1. NO raw packet payload is copied off-device. The ring buffer stores
//    metadata only (IP/TCP/UDP header fields). Payload bytes flow through
//    the tunnel untouched but are never read past the metadata length.
// 2. NO IMEI, MSISDN, phoneNumber, MAC, contacts, advertisingIdentifier.
//    `IdentifierForVendor` and `DeviceCheck` are explicitly NOT called
//    here — same rule as the Android side.
// 3. Source IP is masked at /24 (IPv4) or /48 (IPv6) before leaving the
//    device. Matches backend storage rule `device_ip_masked`.
// 4. Sampling cap = 10 packets (HANDOFF §6.1). Adaptive sampling on
//    TLS 1.3 0-RTT is a Sprint 2 follow-up (ADR-0003 risk G1).
//
// Open items (intentional — Sprint 2+)
// ------------------------------------
// - NetworkExtension entitlement application to Apple (A1 risk).
// - `NEAppRules` / `matchDomains` exclusion of system traffic.
// - Tunnel provider configuration UI (NETunnelProviderManager.saveToPreferences).
// - Privacy Manifest (`PrivacyInfo.xcprivacy`) with NSPrivacyTracking=false
//   + the `NSPrivacyCollectedAPITypes` declarations required by WWDC 2023.
//
// References
// ----------
// - docs/ADR-0003-vpn-layer.md (Flutter native extension; MethodChannel bridge)
// - docs/ADR-0006-anonimlik.md (veri minimizasyonu)
// - docs/HANDOFF.md §4.2 PR-10
// - docs/RISKS.md A1 (entitlement rejection), A4 (background VPN limits)

import Foundation
import NetworkExtension
import Flutter

/// iOS NetworkExtension packet tunnel provider for OpenE2EE.
///
/// Subclass of `NEPacketTunnelProvider`. iOS hands us a `packetFlow`
/// from which we read IP packets, extract metadata, and forward the bytes
/// untouched to the upstream network.
final class OpenE2eeTunnelProvider: NEPacketTunnelProvider {

    // MARK: - Constants

    /// MUST match the Dart-side `kVpnMethodChannel` in `method_channel.dart`.
    static let methodChannelName = "opene2ee/vpn"

    /// Sampling cap per HANDOFF BRD §6.1 mobile spec — first 10 packets.
    static let samplingCapPackets = 10

    /// Methods invoked from native → Dart.
    private enum NativeMethod: String {
        case onTelemetry = "onTelemetry"
        case onError = "onError"
    }

    /// Methods invoked from Dart → native.
    private enum DartMethod: String {
        case start
        case stop
        case status
    }

    // MARK: - State

    enum State: String {
        case idle, sampling, draining, stopped
    }

    private let stateLock = NSLock()
    private var _state: State = .idle
    private var packetsObserved: Int = 0

    /// In-memory ring of metadata-only entries. Cap = samplingCapPackets.
    /// Older entries are evicted FIFO once the cap is reached.
    private var ring: [[String: Any]] = []

    /// MethodChannel into Flutter. Wired by `setUpFlutterChannel(_:)`.
    private weak var flutterChannel: FlutterMethodChannel?

    // MARK: - Tunnel lifecycle

    /// Called by iOS when the tunnel is being started. We configure a
    /// tunnel that captures all traffic but only samples the first 10
    /// packets (metadata only).
    override func startTunnel(
        options: [String: NSObject]?,
        completionHandler: @escaping (Error?) -> Void
    ) {
        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: "127.0.0.1")
        // Phase 2: real tunnelRemoteAddress derived from the user's egress.
        // The local loopback placeholder lets us pass settings validation
        // without granting any real upstream connectivity for the skeleton.

        let ipv4 = NEIPv4Settings(addresses: ["10.42.0.2"], subnetMasks: ["255.255.255.0"])
        ipv4.includedRoutes = [NEIPv4Route.default()]
        settings.ipv4Settings = ipv4

        settings.dnsSettings = NEDNSSettings(servers: ["1.1.1.1", "1.0.0.1"])
        settings.mtu = NSNumber(value: 1500)

        setTunnelNetworkSettings(settings) { [weak self] error in
            guard let self = self else { return }
            if let error = error {
                completionHandler(error)
                return
            }
            self.transition(to: .sampling)
            self.beginPacketLoop()
            completionHandler(nil)
        }
    }

    /// Called by iOS when the tunnel is being stopped (user, OS, or app).
    override func stopTunnel(
        with reason: NEProviderStopReason,
        completionHandler: @escaping () -> Void
    ) {
        transition(to: .draining)
        flushTelemetry()
        transition(to: .stopped)
        completionHandler()
    }

    // MARK: - Flutter ↔ Native bridge

    /// Wire the Flutter MethodChannel. Called from
    /// `AppDelegate.application(_:didFinishLaunchingWithOptions:)`
    /// (or the SwiftUI SceneDelegate equivalent) after the FlutterEngine
    /// has been instantiated.
    func setUpFlutterChannel(messenger: FlutterBinaryMessenger) {
        let channel = FlutterMethodChannel(
            name: OpenE2eeTunnelProvider.methodChannelName,
            binaryMessenger: messenger
        )
        channel.setMethodCallHandler { [weak self] call, result in
            self?.handle(call: call, result: result)
        }
        flutterChannel = channel
    }

    private func handle(call: FlutterMethodCall, result: @escaping FlutterResult) {
        switch call.method {
        case DartMethod.start.rawValue:
            // iOS requires the user to have approved the tunnel via the
            // system NE preferences UI; the Dart side is responsible for
            // navigating the user there before calling this.
            startTunnel(options: nil) { error in
                if let error = error {
                    result(FlutterError(
                        code: "START_FAILED",
                        message: error.localizedDescription,
                        details: nil
                    ))
                } else {
                    result(self.currentState().rawValue)
                }
            }
        case DartMethod.stop.rawValue:
            stopTunnel(with: .userInitiated) {
                result(self.currentState().rawValue)
            }
        case DartMethod.status.rawValue:
            result([
                "state": currentState().rawValue,
                "packetsObserved": packetsObserved,
                "ringSize": ring.count,
                "samplingCap": OpenE2eeTunnelProvider.samplingCapPackets,
            ])
        default:
            result(FlutterMethodNotImplemented)
        }
    }

    // MARK: - Packet loop (metadata-only capture)

    private func beginPacketLoop() {
        // Phase 2: real implementation reads from `packetFlow.readPackets`
        // on a high-priority DispatchQueue and feeds each packet into
        // `extractMetadata(_:length:)`. The skeleton leaves the loop body
        // empty because the file is not yet compiled into a target.
    }

    /**
     * Extract IP/TCP/UDP metadata from a packet buffer.
     *
     * Privacy invariants (ADR-0006) enforced here:
     * - Source / destination IPs are masked at /24 (IPv4) or /48 (IPv6).
     * - Payload bytes are NEVER read past the metadata length fields.
     * - No identifier APIs are touched (IdentifierForVendor, DeviceCheck, etc.).
     */
    private func extractMetadata(packet: Data, length: Int) -> [String: Any]? {
        guard length >= 20 else { return nil } // too short for an IPv4 header
        let versionAndIhl = packet[0]
        let version = (Int(versionAndIhl) & 0xF0) >> 4
        guard version == 4 || version == 6 else { return nil }

        let protocolByte: UInt8
        if version == 4 {
            protocolByte = packet[9]
        } else {
            // IPv6 next-header at offset 6
            guard length >= 48 else { return nil }
            protocolByte = packet[6]
        }

        return [
            "version": version,
            "protocol": Int(protocolByte),            // 6 = TCP, 17 = UDP, ...
            "packetLength": length,
            "srcIpMasked": NSNull(),                  // Phase 2: extract + mask
            "dstIpMasked": NSNull(),                  // Phase 2: extract + mask
            "srcPort": NSNull(),                      // Phase 2
            "dstPort": NSNull(),                      // Phase 2
            "tcpFlags": NSNull(),                     // Phase 2
            "tlsClientHelloFingerprint": NSNull(),    // Phase 2 — paired with PR-4 analysis
        ]
    }

    // MARK: - Telemetry dispatch

    private func flushTelemetry() {
        let payload: [String: Any] = [
            "sessionId": NSNull(),                 // populated by Dart-side glue (PR-10 Phase 2)
            "packets": ring,
            "capturedAt": Int(Date().timeIntervalSince1970 * 1000),
        ]
        flutterChannel?.invokeMethod(
            NativeMethod.onTelemetry.rawValue,
            arguments: payload
        )
    }

    // MARK: - State helpers

    private func transition(to newState: State) {
        stateLock.lock()
        _state = newState
        stateLock.unlock()
    }

    private func currentState() -> State {
        stateLock.lock()
        defer { stateLock.unlock() }
        return _state
    }
}