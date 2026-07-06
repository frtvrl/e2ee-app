// mobile/lib/mobile/vpn/method_channel.dart
//
// PR-10: Mobile-only — Dart-side bridge to the native VPN samplers.
//
// This file is the Dart-side counterpart of:
//
//   - `vpn/vpn_service_android.kt`  (Android `VpnService` subclass)
//   - `vpn/NetworkExtension.swift`  (iOS `NEPacketTunnelProvider` subclass)
//
// It exposes a small, typed API to the rest of the mobile app (the screens
// in `mobile/lib/mobile/screens/`) and hides the `MethodChannel` plumbing.
//
// Architecture (per ADR-0003)
// ---------------------------
// - Flutter ↔ Native transport: `MethodChannel` named `opene2ee/vpn`.
// - Dart → native commands: `start`, `stop`, `status`.
// - Native → Dart callbacks: `onTelemetry`, `onError` (delivered as
//   `MethodCall` invocations on the same channel).
// - The native side samples the **first 10 packets** of a session and
//   forwards a metadata-only summary back. NO raw payload bytes cross
//   the bridge — see ADR-0006 §"Veri Minimizasyonu" and the privacy
//   summary at the bottom of this file.
//
// Platform support
// ----------------
// The native side is platform-specific (Kotlin for Android, Swift for iOS).
// On any other platform (web, desktop) the channel is unavailable; the
// factory in this file falls back to a `NoopVpnBridge` that reports
// `VpnPlatform.unavailable` so callers degrade gracefully. The screens in
// `mobile/lib/mobile/screens/` consume this bridge via the singleton
// `VpnBridge.instance`.
//
// Testability
// -----------
// The interface ([VpnBridge]) is small and side-effect-free in tests —
// `flutter test` exercises `NoopVpnBridge` directly. A real integration
// test that spins up the native side is out of scope for Sprint 1
// (see HANDOFF §4.2 — "Native build (Android/iOS) bu Sprint'te yok").
//
// References
// ----------
// - docs/ADR-0003-vpn-layer.md
// - docs/ADR-0006-anonimlik.md
// - docs/HANDOFF.md §4.2 PR-10

import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

/// Channel name shared with the Android (`vpn_service_android.kt`) and iOS
/// (`NetworkExtension.swift`) implementations.
///
/// MUST stay in sync with:
///   - `OpenE2eeVpnService.METHOD_CHANNEL` (Kotlin)
///   - `OpenE2eeTunnelProvider.methodChannelName` (Swift)
const String kVpnMethodChannel = 'opene2ee/vpn';

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
}

/// A single packet's metadata as extracted on the native side.
///
/// **Privacy invariant (ADR-0006):** the `payload` field is forbidden by
/// contract. If a future implementation tries to add it, the unit test
/// in `mobile/test/mobile/method_channel_test.dart` (Phase 2) will fail.
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

  /// Build from the `Map<String, dynamic>` the native side sends over the
  /// MethodChannel. Tolerates missing optional fields — only `version`,
  /// `protocol`, `packetLength` are required.
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

  /// Build from the raw MethodChannel payload.
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

/// Public contract the UI layer depends on. Hides the `MethodChannel`
/// so screens can be unit-tested with [NoopVpnBridge].
abstract class VpnBridge {
  /// Start the native sampler. Caller is responsible for ensuring the
  /// user has consented and (on iOS) approved the tunnel via the system
  /// NE preferences UI.
  Future<VpnLifecycleState> start();

  /// Stop the native sampler + flush the ring + tear down the tunnel.
  Future<VpnLifecycleState> stop();

  /// Snapshot of the native state machine.
  Future<VpnLifecycleState> status();

  /// Stream of telemetry callbacks from the native side. Multi-subscriber
  /// friendly: every listener gets every event.
  Stream<VpnTelemetry> get telemetry;

  /// Stream of error callbacks (e.g. NE entitlement revoked, TUN error).
  Stream<VpnBridgeError> get errors;
}

/// Error surfaced by the native side.
@immutable
class VpnBridgeError {
  const VpnBridgeError({required this.code, this.message, this.details});

  final String code;
  final String? message;
  final Object? details;
}

/// Production [VpnBridge] backed by a real [MethodChannel].
class MethodChannelVpnBridge implements VpnBridge {
  MethodChannelVpnBridge({MethodChannel? channel})
      : _channel = channel ?? const MethodChannel(kVpnMethodChannel) {
    _channel.setMethodCallHandler(_dispatch);
  }

  final MethodChannel _channel;

  final StreamController<VpnTelemetry> _telemetryCtrl =
      StreamController<VpnTelemetry>.broadcast();
  final StreamController<VpnBridgeError> _errorCtrl =
      StreamController<VpnBridgeError>.broadcast();

  /// Routes native → Dart invocations to the typed stream API.
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
  Future<VpnLifecycleState> start() async {
    final raw = await _channel.invokeMethod<String>('start');
    return _parseState(raw);
  }

  @override
  Future<VpnLifecycleState> stop() async {
    final raw = await _channel.invokeMethod<String>('stop');
    return _parseState(raw);
  }

  @override
  Future<VpnLifecycleState> status() async {
    final raw = await _channel.invokeMapMethod<String, dynamic>('status');
    return _parseState(raw == null ? null : raw['state'] as String?);
  }

  @override
  Stream<VpnTelemetry> get telemetry => _telemetryCtrl.stream;

  @override
  Stream<VpnBridgeError> get errors => _errorCtrl.stream;

  /// Release the MethodChannel handler. Call from a `StatefulWidget`'s
  /// `dispose()` to avoid leaks during hot-restart.
  void dispose() {
    _channel.setMethodCallHandler(null);
    _telemetryCtrl.close();
    _errorCtrl.close();
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
      default:
        return VpnLifecycleState.idle;
    }
  }
}

/// No-op bridge used on platforms without native VPN support (web, desktop)
/// and inside `flutter test`. Never throws — `start` reports
/// [VpnLifecycleState.unavailable] and `telemetry` stays empty.
class NoopVpnBridge implements VpnBridge {
  const NoopVpnBridge();

  @override
  Future<VpnLifecycleState> start() async => VpnLifecycleState.unavailable;

  @override
  Future<VpnLifecycleState> stop() async => VpnLifecycleState.unavailable;

  @override
  Future<VpnLifecycleState> status() async => VpnLifecycleState.unavailable;

  @override
  Stream<VpnTelemetry> get telemetry => const Stream<VpnTelemetry>.empty();

  @override
  Stream<VpnBridgeError> get errors => const Stream<VpnBridgeError>.empty();
}

/// Singleton the screens consume. Wired to the [MethodChannelVpnBridge]
/// in production; tests substitute [NoopVpnBridge].
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