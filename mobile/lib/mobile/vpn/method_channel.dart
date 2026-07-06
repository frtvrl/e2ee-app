// mobile/lib/mobile/vpn/method_channel.dart
//
// PR-10 + PR-22a + PR-22b — Dart-side bridge to the native VPN services.
//
// PR-22b (Sprint 3) iOS upgrade:
//   * `MethodChannelVpnBridge` returns [VpnStatusSnapshot] from
//     start/stop/status (matches the Android side from PR-22a).
//   * Permission handshake kept on `opene2ee/vpn_permissions` channel —
//     on iOS the channel is owned by `AppDelegate` rather than the
//     tunnel provider itself.
//
// PR-22a (Sprint 3) upgrade:
//   * Added `setAllowedApplications` / `setDisallowedApplications` for
//     per-app VPN (Android: VpnService.Builder.allowedApplications;
//     iOS 14+: NEAppRules + NETunnelProviderManager).
//   * Added `ensurePermission` / `isPermissionGranted` API backed by the
//     `opene2ee/vpn_permissions` channel.
//   * `start()` no longer attempts to prepare the VPN itself; callers
//     MUST invoke `ensurePermission()` (Android) or the system NE
//     preferences UI (iOS) before `start()`.
//
// Privacy contract (ADR-0006) is preserved: payloads never cross the
// bridge — only metadata snapshots.
//
// References
// ----------
// - docs/ADR-0003-vpn-layer.md
// - docs/ADR-0006-anonimlik.md
// - docs/SPRINT-3-SCOPE.md §7 — Sprint 3 PR-22

import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

/// Channel name shared with the Android (`vpn_service_android.kt`) and iOS
/// (`NetworkExtension.swift` + `AppDelegate.swift`) implementations.
const String kVpnMethodChannel = 'opene2ee/vpn';

/// Companion channel owned by `MainActivity`/`AppDelegate` for the
/// VPN consent handshake. We keep this SEPARATE from [kVpnMethodChannel]
/// because the prepare flow on Android needs an Activity context (only
/// `MainActivity` has one), and on iOS the channel is owned by the
/// `AppDelegate` because `NEPacketTunnelProvider` extensions cannot
/// register MethodChannels directly.
const String kVpnPermissionsChannel = 'opene2ee/vpn_permissions';

/// Current lifecycle state of the native VPN sampler.
enum VpnLifecycleState {
  /// Channel is reachable but no session has started.
  idle,

  /// Native side is sampling packets (cap = 10).
  sampling,

  /// Native side is flushing the ring + tearing down.
  draining,

  /// Session has ended; no in-flight packets.
  stopped,

  /// Native side is not available on this platform (web, desktop tests).
  unavailable,

  /// The native side reported an error (TUN-establish failed, permission
  /// revoked, etc.). Inspect `lastError` on the [VpnStatusSnapshot].
  error,
}

/// Snapshot of the native VPN state — returned by `VpnBridge.status()`.
@immutable
class VpnStatusSnapshot {
  const VpnStatusSnapshot({
    required this.state,
    required this.packetsObserved,
    required this.ringSize,
    required this.samplingCap,
    this.lastError,
    this.allowedApplications,
    this.disallowedApplications,
  });

  /// High-level state — see [VpnLifecycleState].
  final VpnLifecycleState state;

  /// Monotonic counter of packets observed since the last `start`.
  final int packetsObserved;

  /// Current ring buffer fill (0..[samplingCap]).
  final int ringSize;

  /// Cap on the in-memory ring (HANDOFF §6.1 = 10).
  final int samplingCap;

  /// Error message if [state] == [VpnLifecycleState.error], else null.
  final String? lastError;

  /// Per-app VPN allowlist (bundle IDs on iOS, package names on Android)
  /// currently configured. Empty/null = all apps.
  final List<String>? allowedApplications;

  /// Per-app VPN denylist currently configured. Empty/null = no exception.
  final List<String>? disallowedApplications;

  /// Sentinel for platforms without native VPN support.
  static const VpnStatusSnapshot unavailable = VpnStatusSnapshot(
    state: VpnLifecycleState.unavailable,
    packetsObserved: 0,
    ringSize: 0,
    samplingCap: 0,
  );

  factory VpnStatusSnapshot.fromMap(Map<Object?, Object?> json) {
    return VpnStatusSnapshot(
      state: _parseState(json['state'] as String?),
      packetsObserved: ((json['packetsObserved'] as num?) ?? 0).toInt(),
      ringSize: ((json['ringSize'] as num?) ?? 0).toInt(),
      samplingCap: ((json['samplingCap'] as num?) ?? 0).toInt(),
      lastError: json['lastError'] as String?,
      allowedApplications: (json['allowedApplications'] as List<dynamic>?)
          ?.map((e) => e as String)
          .toList(growable: false),
      disallowedApplications: (json['disallowedApplications'] as List<dynamic>?)
          ?.map((e) => e as String)
          .toList(growable: false),
    );
  }

  static VpnLifecycleState _parseState(String? raw) {
    switch (raw) {
      case 'idle':
        return VpnLifecycleState.idle;
      case 'sampling':
        return VpnLifecycleState.sampling;
      case 'draining':
        return VpnLifecycleState.draining;
      case 'stopped':
        return VpnLifecycleState.stopped;
      case 'error':
        return VpnLifecycleState.error;
      default:
        return VpnLifecycleState.idle;
    }
  }
}

/// A single packet's metadata as extracted on the native side.
///
/// **Privacy invariant (ADR-0006):** the `payload` field is forbidden by
/// contract.
@immutable
class VpnPacketMetadata {
  const VpnPacketMetadata({
    required this.version,
    required this.protocol,
    required this.packetLength,
    this.srcIpMasked,
    this.dstIpMasked,
    this.srcPort,
    this.dstPort,
    this.tcpFlags,
    this.tlsClientHelloFingerprint,
  });

  /// IP version: 4 or 6.
  final int version;

  /// IANA IP protocol number (6 = TCP, 17 = UDP, ...).
  final int protocol;

  /// Total length of the captured packet, in bytes (header + payload).
  final int packetLength;

  /// Source IP, masked at /24 (IPv4) or /48 (IPv6) per ADR-0006.
  final String? srcIpMasked;

  /// Destination IP, masked at the same boundary.
  final String? dstIpMasked;

  /// Source transport port (TCP/UDP).
  final int? srcPort;

  /// Destination transport port (TCP/UDP).
  final int? dstPort;

  /// TCP control bits (SYN/ACK/FIN/RST/...).
  final int? tcpFlags;

  /// Opaque TLS Client Hello fingerprint (paired with backend PR-4).
  final String? tlsClientHelloFingerprint;

  factory VpnPacketMetadata.fromMap(Map<Object?, Object?> json) {
    return VpnPacketMetadata(
      version: (json['version'] as num).toInt(),
      protocol: (json['protocol'] as num).toInt(),
      packetLength: (json['packetLength'] as num).toInt(),
      srcIpMasked: json['srcIpMasked'] as String?,
      dstIpMasked: json['dstIpMasked'] as String?,
      srcPort: (json['srcPort'] as num?)?.toInt(),
      dstPort: (json['dstPort'] as num?)?.toInt(),
      tcpFlags: (json['tcpFlags'] as num?)?.toInt(),
      tlsClientHelloFingerprint: json['tlsClientHelloFingerprint'] as String?,
    );
  }
}

/// Telemetry callback payload — the full ring (≤ 10 packets) plus a
/// capture timestamp.
@immutable
class VpnTelemetry {
  const VpnTelemetry({
    required this.sessionId,
    required this.packets,
    required this.capturedAt,
  });

  /// Active session identifier (populated by Dart-side glue from the
  /// matching PR-7 `POST /api/v1/sessions`).
  final String? sessionId;

  /// At most 10 entries per session (HANDOFF §6.1 mobile sampling cap).
  final List<VpnPacketMetadata> packets;

  /// Wall-clock time when the native side flushed the ring.
  final DateTime capturedAt;

  factory VpnTelemetry.fromMap(Map<Object?, Object?> json) {
    final rawPackets = (json['packets'] as List<dynamic>? ?? const [])
        .cast<Map<Object?, Object?>>();
    return VpnTelemetry(
      sessionId: json['sessionId'] as String?,
      packets: rawPackets.map(VpnPacketMetadata.fromMap).toList(growable: false),
      capturedAt: DateTime.fromMillisecondsSinceEpoch(
        (json['capturedAt'] as num).toInt(),
      ),
    );
  }
}

/// Error surfaced by the native side.
@immutable
class VpnBridgeError {
  const VpnBridgeError({required this.code, this.message, this.details});

  final String code;
  final String? message;
  final Object? details;
}

/// Public contract the UI layer depends on. Hides the `MethodChannel`
/// so screens can be unit-tested with [NoopVpnBridge].
abstract class VpnBridge {
  /// Start the native sampler. Throws [VpnPermissionDeniedError] if the
  /// caller has not yet obtained consent via [ensurePermission] (Android)
  /// or the system NE preferences UI (iOS).
  Future<VpnStatusSnapshot> start();

  /// Stop the native sampler + flush the ring + tear down the tunnel.
  Future<VpnStatusSnapshot> stop();

  /// Snapshot of the native state machine.
  Future<VpnStatusSnapshot> status();

  /// Restrict the VPN to a per-app allowlist (Android 5.0+; iOS 14+
  /// via `NEAppRules`). Passing an empty list clears the allowlist.
  /// Mutually exclusive with [setDisallowedApplications].
  Future<void> setAllowedApplications(List<String> bundleOrPackageIds);

  /// Inverse of [setAllowedApplications] — bypass the VPN for these apps.
  Future<void> setDisallowedApplications(List<String> bundleOrPackageIds);

  /// Ensure the user has consented to the system VPN sheet. Returns
  /// true when consent was obtained (or already held). Returns false
  /// if the user declined — in that case, [start] will fail.
  Future<bool> ensurePermission();

  /// Was consent already obtained in this app install? Faster than
  /// [ensurePermission] if the answer is "yes" (no intent / sheet shown).
  Future<bool> isPermissionGranted();

  /// Stream of telemetry callbacks from the native side.
  Stream<VpnTelemetry> get telemetry;

  /// Stream of error callbacks (e.g. NE entitlement revoked, TUN error).
  Stream<VpnBridgeError> get errors;
}

/// Thrown by [VpnBridge.start] when the user has not yet granted the VPN
/// consent. The UI should call [VpnBridge.ensurePermission] first and,
/// on a true result, retry start.
class VpnPermissionDeniedError extends Error {
  VpnPermissionDeniedError(this.message);
  final String message;
  @override
  String toString() => 'VpnPermissionDeniedError: $message';
}

/// Production [VpnBridge] backed by real `MethodChannel`s.
class MethodChannelVpnBridge implements VpnBridge {
  MethodChannelVpnBridge({
    MethodChannel? vpnChannel,
    MethodChannel? permissionsChannel,
  })  : _vpnChannel = vpnChannel ?? const MethodChannel(kVpnMethodChannel),
        _permissionsChannel = permissionsChannel ??
            const MethodChannel(kVpnPermissionsChannel) {
    _vpnChannel.setMethodCallHandler(_dispatch);
  }

  final MethodChannel _vpnChannel;
  final MethodChannel _permissionsChannel;

  final StreamController<VpnTelemetry> _telemetryCtrl =
      StreamController<VpnTelemetry>.broadcast();
  final StreamController<VpnBridgeError> _errorCtrl =
      StreamController<VpnBridgeError>.broadcast();

  Future<dynamic> _dispatch(MethodCall call) async {
    switch (call.method) {
      case 'onTelemetry':
        final args = (call.arguments as Map<Object?, Object?>?) ?? const {};
        _telemetryCtrl.add(VpnTelemetry.fromMap(args));
        return null;
      case 'onError':
        final args = (call.arguments as Map<Object?, Object?>?) ?? const {};
        _errorCtrl.add(VpnBridgeError(
          code: (args['code'] as String?) ?? 'UNKNOWN',
          message: args['message'] as String?,
          details: args['details'],
        ));
        return null;
      default:
        throw MissingPluginException(
          'VpnBridge received unexpected method: ${call.method}',
        );
    }
  }

  @override
  Future<VpnStatusSnapshot> start() async {
    final granted = await ensurePermission();
    if (!granted) {
      throw VpnPermissionDeniedError(
        'VpnBridge.start called before ensurePermission() returned true',
      );
    }
    final raw = await _vpnChannel.invokeMapMethod<String, dynamic>('start')
        ?? const <String, dynamic>{};
    return VpnStatusSnapshot.fromMap(raw);
  }

  @override
  Future<VpnStatusSnapshot> stop() async {
    final raw = await _vpnChannel.invokeMapMethod<String, dynamic>('stop')
        ?? const <String, dynamic>{};
    return VpnStatusSnapshot.fromMap(raw);
  }

  @override
  Future<VpnStatusSnapshot> status() async {
    final raw = await _vpnChannel.invokeMapMethod<String, dynamic>('status')
        ?? const <String, dynamic>{};
    return VpnStatusSnapshot.fromMap(raw);
  }

  @override
  Future<void> setAllowedApplications(List<String> bundleOrPackageIds) async {
    await _vpnChannel.invokeMethod<void>('setAllowedApplications', {
      'packages': bundleOrPackageIds,
    });
  }

  @override
  Future<void> setDisallowedApplications(List<String> bundleOrPackageIds) async {
    await _vpnChannel.invokeMethod<void>('setDisallowedApplications', {
      'packages': bundleOrPackageIds,
    });
  }

  @override
  Future<bool> ensurePermission() async {
    final raw = await _permissionsChannel.invokeMethod<bool>('requestVpnPermission');
    return raw ?? false;
  }

  @override
  Future<bool> isPermissionGranted() async {
    final raw =
        await _permissionsChannel.invokeMethod<bool>('isVpnPrepared');
    return raw ?? false;
  }

  @override
  Stream<VpnTelemetry> get telemetry => _telemetryCtrl.stream;

  @override
  Stream<VpnBridgeError> get errors => _errorCtrl.stream;

  /// Release the MethodChannel handler. Call from a `StatefulWidget`'s
  /// `dispose()` to avoid leaks during hot-restart.
  void dispose() {
    _vpnChannel.setMethodCallHandler(null);
    _telemetryCtrl.close();
    _errorCtrl.close();
  }

  /// Lightweight constructor alias used in unit tests that inject both
  /// channels explicitly.
  static VpnStatusSnapshot parseStatus(Map<Object?, Object?> map) =>
      VpnStatusSnapshot.fromMap(map);
}

/// No-op bridge used on platforms without native VPN support (web, desktop)
/// and inside `flutter test`. Never throws — `start` / `stop` / `status`
/// all return [VpnStatusSnapshot.unavailable]; `setAllowedApplications` /
/// `setDisallowedApplications` silently accept and drop the input.
///
/// This bridge deliberately does NOT mirror the [MethodChannelVpnBridge]
/// permission-check contract: a no-op platform has no native tunnel to
/// gate, so the production-style [VpnPermissionDeniedError] would be
/// misleading. Callers (and `test/mobile/vpn/method_channel_test.dart`'s
/// `NoopVpnBridge all control methods report VpnLifecycleState.unavailable`
/// case) expect start() to return a sentinel snapshot, not throw.
class NoopVpnBridge implements VpnBridge {
  const NoopVpnBridge();

  @override
  Future<VpnStatusSnapshot> start() async => VpnStatusSnapshot.unavailable;

  @override
  Future<VpnStatusSnapshot> stop() async => VpnStatusSnapshot.unavailable;

  @override
  Future<VpnStatusSnapshot> status() async => VpnStatusSnapshot.unavailable;

  @override
  Future<void> setAllowedApplications(List<String> bundleOrPackageIds) async {}

  @override
  Future<void> setDisallowedApplications(List<String> bundleOrPackageIds) async {}

  @override
  Future<bool> ensurePermission() async => false;

  @override
  Future<bool> isPermissionGranted() async => false;

  @override
  Stream<VpnTelemetry> get telemetry => const Stream<VpnTelemetry>.empty();

  @override
  Stream<VpnBridgeError> get errors => const Stream<VpnBridgeError>.empty();
}

/// Singleton the screens consume. Wired to the [MethodChannelVpnBridge]
/// in production; tests substitute any [VpnBridge] via
/// [VpnBridgeInstance.debugSet].
class VpnBridgeInstance {
  VpnBridgeInstance._(this._impl);

  final VpnBridge _impl;
  VpnBridge get impl => _impl;

  static VpnBridge? _overrideForTest;

  /// Replace the singleton — only intended for `flutter test`.
  @visibleForTesting
  static void debugSet(VpnBridge? bridge) {
    _overrideForTest = bridge;
  }

  /// Default factory: real `MethodChannel` on mobile, no-op otherwise.
  factory VpnBridgeInstance.defaultFactory() {
    if (_overrideForTest != null) {
      return VpnBridgeInstance._(_overrideForTest!);
    }
    if (kIsWeb || defaultTargetPlatform != TargetPlatform.android &&
        defaultTargetPlatform != TargetPlatform.iOS) {
      return VpnBridgeInstance._(const NoopVpnBridge());
    }
    return VpnBridgeInstance._(MethodChannelVpnBridge());
  }
}

/// Top-level singleton screens consume as
/// `import 'package:opene2ee/mobile/vpn/method_channel.dart' as vpn; vpn.bridge`.
final VpnBridge bridge = VpnBridgeInstance.defaultFactory().impl;
