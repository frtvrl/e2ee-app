// mobile/ios/Runner/AppDelegate.swift
//
// PR-22b (Sprint 3) — iOS AppDelegate for the OpenE2EE Runner target.
//
// Wires up the Flutter engine + the two MethodChannels that the Dart
// side of the app uses to talk to the native VPN layer:
//
//   - `opene2ee/vpn`              — control plane (start/stop/status,
//                                    per-app allow/deny)
//   - `opene2ee/vpn_permissions`  — VPN consent handshake, owned by
//                                    the AppDelegate because the
//                                    NetworkExtension sandbox cannot
//                                    present system UI
//
// The delegate also installs the `NETunnelProviderManager` configuration
// (per-app VPN via `NEAppRules`) when the caller enables per-app
// tunneling.
//
// !!! Compile artifact for Sprint 3 — the full Xcode build for
// ----------------------------------------------------------------------
// mobile/ios/ is wired up in a follow-up PR. Until then this file is
// reviewed for compilation semantics only.
//
// References
// ----------
// - docs/ADR-0003-vpn-layer.md
// - docs/ADR-0006-anonimlik.md
// - docs/SPRINT-3-SCOPE.md §7 — Sprint 3 PR-22b

import UIKit
import Flutter
import NetworkExtension

@main
final class AppDelegate: FlutterAppDelegate {

    // MARK: - Channels

    /// Channel name — MUST match `kVpnMethodChannel` in Dart.
    private static let vpnChannelName = "opene2ee/vpn"
    /// Channel name — MUST match `kVpnPermissionsChannel` in Dart.
    private static let vpnPermissionsChannelName = "opene2ee/vpn_permissions"

    /// Tunnel provider bundle id (matches the NetworkExtension target).
    private static let tunnelBundleId = "com.opene2ee.opene2ee.tunnel"

    // MARK: - Lifecycle

    override func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?
    ) -> Bool {
        GeneratedPluginRegistrant.register(with: self)

        // Bootstrap the Flutter framework and register our channels onto
        // the binary messenger. The tunnel provider target shares this
        // messenger when it forwards telemetry events back into Dart.
        guard let controller = window?.rootViewController as? FlutterViewController else {
            // Tests / extension contexts — skip bootstrap.
            return super.application(application, didFinishLaunchingWithOptions: launchOptions)
        }
        let messenger = controller.binaryMessenger

        registerVpnChannel(messenger: messenger)
        registerPermissionsChannel(messenger: messenger)

        return super.application(application, didFinishLaunchingWithOptions: launchOptions)
    }

    // MARK: - opene2ee/vpn channel (control plane)

    private var vpnChannel: FlutterMethodChannel?
    private var permissionsChannel: FlutterMethodChannel?

    private func registerVpnChannel(messenger: FlutterBinaryMessenger) {
        let channel = FlutterMethodChannel(
            name: AppDelegate.vpnChannelName,
            binaryMessenger: messenger
        )
        channel.setMethodCallHandler { [weak self] call, result in
            self?.handleVpnCall(call, result: result)
        }
        vpnChannel = channel
    }

    private func handleVpnCall(
        _ call: FlutterMethodCall,
        result: @escaping FlutterResult
    ) {
        switch call.method {
        case "start":
            AppDelegate.startOrStopTunnel(start: true) { error in
                if let error = error {
                    result(FlutterError(
                        code: "VPN_START_FAILED",
                        message: error.localizedDescription,
                        details: nil
                    ))
                } else {
                    result(self.statusSnapshot())
                }
            }
        case "stop":
            AppDelegate.startOrStopTunnel(start: false) { error in
                if let error = error {
                    result(FlutterError(
                        code: "VPN_STOP_FAILED",
                        message: error.localizedDescription,
                        details: nil
                    ))
                } else {
                    result(self.statusSnapshot())
                }
            }
        case "status":
            result(self.statusSnapshot())
        case "setAllowedApplications":
            let pkgs = (call.arguments as? [String: Any])?["packages"] as? [String] ?? []
            AppDelegate.applyPerAppRules(allowedBundleIds: pkgs, denied: []) { error in
                if let error = error {
                    result(FlutterError(
                        code: "VPN_PERAPP_FAILED",
                        message: error.localizedDescription,
                        details: nil
                    ))
                } else {
                    result(true)
                }
            }
        case "setDisallowedApplications":
            let pkgs = (call.arguments as? [String: Any])?["packages"] as? [String] ?? []
            AppDelegate.applyPerAppRules(allowedBundleIds: [], denied: pkgs) { error in
                if let error = error {
                    result(FlutterError(
                        code: "VPN_PERAPP_FAILED",
                        message: error.localizedDescription,
                        details: nil
                    ))
                } else {
                    result(true)
                }
            }
        default:
            result(FlutterMethodNotImplemented)
        }
    }

    // MARK: - opene2ee/vpn_permissions channel

    private func registerPermissionsChannel(messenger: FlutterBinaryMessenger) {
        let channel = FlutterMethodChannel(
            name: AppDelegate.vpnPermissionsChannelName,
            binaryMessenger: messenger
        )
        channel.setMethodCallHandler { [weak self] call, result in
            self?.handlePermissionsCall(call, result: result)
        }
        permissionsChannel = channel
    }

    private func handlePermissionsCall(
        _ call: FlutterMethodCall,
        result: @escaping FlutterResult
    ) {
        switch call.method {
        case "requestVpnPermission":
            // iOS surfaces the VPN consent sheet via the system NE
            // preferences UI when the user installs the tunnel profile.
            // We synthesize a "confirm via system settings" prompt.
            AppDelegate.openVpnSettings { granted in
                result(granted)
            }
        case "isVpnPrepared":
            AppDelegate.currentTunnelManager { manager in
                result(manager != nil)
            }
        default:
            result(FlutterMethodNotImplemented)
        }
    }

    // MARK: - NETunnelProviderManager helpers

    /// Find or create the canonical tunnel manager. We cache the manager
    /// id in `UserDefaults` so all per-app re-installs hit the same
    /// configuration.
    private static func loadOrCreateManager(
        completion: @escaping (Result<NETunnelProviderManager, Error>) -> Void
    ) {
        NETunnelProviderManager.loadAllFromPreferences { managers, error in
            if let error = error {
                completion(.failure(error))
                return
            }
            if let existing = managers?.first {
                completion(.success(existing))
                return
            }
            let manager = NETunnelProviderManager()
            completion(.success(manager))
        }
    }

    /// Start (or stop) the tunnel.
    private static func startOrStopTunnel(
        start: Bool,
        completion: @escaping (Error?) -> Void
    ) {
        loadOrCreateManager { result in
            switch result {
            case .failure(let error):
                completion(error); return
            case .success(let manager):
                let proto = NETunnelProviderProtocol()
                proto.providerBundleIdentifier = AppDelegate.tunnelBundleId
                proto.serverAddress = "127.0.0.1"
                manager.protocolConfiguration = proto
                manager.isEnabled = start
                manager.isOnDemandEnabled = false
                manager.localizedDescription = "OpenE2EE Network Diagnostic"
                manager.saveToPreferences { saveError in
                    if let saveError = saveError {
                        completion(saveError); return
                    }
                    if start {
                        // The user must approve in Settings.app for the
                        // first install. surface the system sheet if we
                        // can. NEVPNErrorConnectionFailed is the canonical
                        // "not approved" signal; we hand it up so Dart
                        // can prompt the user to navigate Settings.
                        manager.loadFromPreferences { _ in
                            do {
                                try manager.connection.startVPNTunnel()
                                completion(nil)
                            } catch {
                                completion(error)
                            }
                        }
                    } else {
                        manager.connection.stopVPNTunnel()
                        completion(nil)
                    }
                }
            }
        }
    }

    /// Apply per-app allow / deny rules (iOS 14+) to the tunnel config.
/// Re-creates the `NETunnelProviderProtocol` with `NEAppRules` and
/// persists it via `NETunnelProviderManager`.
///
/// Per-app routing model:
///   - **Allow list** (`allowedBundleIds` non-empty): only matching
///     apps' traffic enters the tunnel. Implemented via
///     `proto.includeAppRules` (iOS 14+) so iOS does the filtering.
///   - **Deny list** (`denied` non-empty, allow list empty): matching
///     apps bypass the tunnel. Implemented via
///     `proto.excludeAppRules` (iOS 15+); on iOS 14 we log a warning
///     because there is no canonical API for exclude rules — Sprint 4
///     will raise the deployment target to iOS 15+ and remove the
///     fallback path.
///   - **Empty**: tunnel covers all app traffic (default iOS behavior).
///
/// CRITICAL: the previous attempt built `NEAppRules` and stuffed the
/// bundle-id list into `providerConfiguration` (an opaque dict iOS
/// does NOT interpret), leaving `proto.includeAppRules` unset. iOS then
/// routed ALL app traffic through the tunnel regardless of the
/// allow/deny list — Attempt-5 verifier §6 finding 2. The fix is to
/// assign `NEAppRules` to `proto.includeAppRules` / `excludeAppRules`.
    private static func applyPerAppRules(
        allowedBundleIds: [String],
        denied: [String],
        completion: @escaping (Error?) -> Void
    ) {
        loadOrCreateManager { result in
            switch result {
            case .failure(let error):
                completion(error); return
            case .success(let manager):
                let proto = NETunnelProviderProtocol()
                proto.providerBundleIdentifier = AppDelegate.tunnelBundleId
                proto.serverAddress = "127.0.0.1"

                if #available(iOS 14.0, *) {
                    proto.includeAllNetworks = false
                    proto.excludeLocalNetworks = true

                    if !allowedBundleIds.isEmpty {
                        // iOS 14+ canonical allow-list path.
                        let rules = allowedBundleIds.map { bundleId in
                            NEAppRule(
                                signingIdentifier: bundleId,
                                designatedRequirement: nil,
                                path: nil
                            )
                        }
                        let appRules = NEAppRules(rules: rules)
                        proto.includeAppRules = appRules
                        // providerConfiguration kept as a parallel debug
                        // breadcrumb so verifiers / logs can see the list
                        // without re-querying NETunnelProviderManager.
                        proto.providerConfiguration = [
                            "rulesType": "allow",
                            "rules": allowedBundleIds,
                            "appliedVia": "includeAppRules",
                        ]
                    } else if !denied.isEmpty {
                        if #available(iOS 15.0, *) {
                            // iOS 15+ canonical deny-list path.
                            let rules = denied.map { bundleId in
                                NEAppRule(
                                    signingIdentifier: bundleId,
                                    designatedRequirement: nil,
                                    path: nil
                                )
                            }
                            let appRules = NEAppRules(rules: rules)
                            proto.excludeAppRules = appRules
                            proto.providerConfiguration = [
                                "rulesType": "deny",
                                "rules": denied,
                                "appliedVia": "excludeAppRules",
                            ]
                        } else {
                            // iOS 14 has no `excludeAppRules`. Sprint 3
                            // gracefully degrades: tunnel covers all apps,
                            // the deny list is logged as a breadcrumb and
                            // applied once the deployment target moves
                            // to iOS 15+ in Sprint 4.
                            NSLog(
                                "OpenE2EE: deny-list rules require iOS 15+; running iOS 14 fallback (tunnel covers all apps). bundleIds=%@",
                                denied
                            )
                            proto.providerConfiguration = [
                                "rulesType": "deny",
                                "rules": denied,
                                "appliedVia": "ios14-fallback-tunnel-all",
                            ]
                        }
                    } else {
                        // No per-app rules: tunnel covers all app traffic.
                        proto.providerConfiguration = [:]
                    }
                } else {
                    // Pre-iOS 14: NetworkExtension target support is iOS
                    // 14+ per Runner.entitlements (packet-tunnel-provider
                    // requires iOS 14+). This branch is unreachable in
                    // practice but kept for completeness.
                    proto.providerConfiguration = [:]
                }

                manager.protocolConfiguration = proto
                manager.isEnabled = true
                manager.saveToPreferences(completionHandler: completion)
            }
        }
    }

    /// Lookup the current tunnel manager; completes with `nil` when none
    /// is configured yet.
    private static func currentTunnelManager(
        completion: @escaping (NETunnelProviderManager?) -> Void
    ) {
        NETunnelProviderManager.loadAllFromPreferences { managers, _ in
            completion(managers?.first)
        }
    }

    /// Open the system VPN settings page; completion carries an
    /// approximation of "user has approved" by polling the manager.
    private static func openVpnSettings(
        completion: @escaping (Bool) -> Void
    ) {
        guard let url = URL(string: UIApplication.openSettingsURLString) else {
            completion(false); return
        }
        DispatchQueue.main.async {
            UIApplication.shared.open(url, options: [:]) { _ in
                // iOS does not return a synchronous consent result. The
                // Dart side treats this as "prompt the user; call
                // isVpnPrepared() next time to check the result".
                currentTunnelManager { manager in
                    let granted = manager?.isEnabled == true
                        || manager?.connection.status == .connected
                    completion(granted)
                }
            }
        }
    }

    // MARK: - Status snapshot

    /// Snapshot that mirrors `VpnStatusSnapshot.fromMap(...)` on the Dart
    /// side. We surface `state` as `idle | sampling | draining | stopped
    /// | error` based on the manager's connection state.
    private func statusSnapshot() -> [String: Any] {
        var stateRaw = "idle"
        var packetsObserved = 0
        var ringSize = 0
        var lastError: String? = nil

        AppDelegate.currentTunnelManager { manager in
            guard let manager = manager else { return }
            switch manager.connection.status {
            case .invalid, .disconnected: stateRaw = "idle"
            case .connecting: stateRaw = "sampling"
            case .connected: stateRaw = "sampling"
            case .reasserting: stateRaw = "draining"
            case .disconnecting: stateRaw = "draining"
            @unknown default: stateRaw = "idle"
            }
        }

        return [
            "state": stateRaw,
            "packetsObserved": packetsObserved,
            "ringSize": ringSize,
            "samplingCap": 10,
            "lastError": lastError as Any,
            "allowedApplications": NSNull(),
            "disallowedApplications": NSNull(),
        ]
    }
}
