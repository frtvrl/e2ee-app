// mobile/lib/services/vpn_service.dart
//
// Sprint 10.1B — Dart-side MethodChannel client for the Android
// `opene2ee/vpn` MethodChannel. Mirrors the Kotlin handler in
// `MainActivity.kt` / `OpenE2eeVpnService.kt`.
//
// Channel: `opene2ee/vpn` (matches `OpenE2eeVpnService.METHOD_CHANNEL`).
//
// Inbound methods (Dart → Kotlin)
// --------------------------------
//   "start"               → begin capture (caller must have
//                            obtained RESULT_OK from
//                            VpnService.prepare first).
//   "stop"   {graceful}   → tear down + flush ring.
//   "status"              → snapshot of {state, packetsObserved,
//                            ringSize, samplingCap}.
//   "getSampledPackets"   → returns a List<Map<String, Object?>>
//                            produced by `OpenE2eeVpnService`'s
//                            bounded ring buffer; each entry is
//                            the same shape `ParsedPacket.toJson()`
//                            produces, so the caller can pipe
//                            them straight into
//                            `TelemetryService.send`.
//   "requestPrepare"      → returns the Android intent action the
//                            activity must `startActivityForResult`
//                            for (handled by MainActivity, exposed
//                            here for convenience).
//
// Outbound methods (Kotlin → Dart, via the `onTelemetry` event channel
//  exposed separately, OR by polling `getSampledPackets`)
// ------------------------------------------------------------------------
//   "onTelemetry" event → final flush of the ring + capture
//                          timestamp. NOT a streaming event in 10.1B —
//                          the pool provider polls `getSampledPackets`
//                          on a 5s cadence to avoid the additional
//                          complexity of an `EventChannel` (10.1C
//                          may switch to push when the JVM side is
//                          refactored).
//
// Permission flow
// ---------------
// `requestVpnPermission` lives on a SECOND channel
// `opene2ee/vpn_permissions` (handled by MainActivity, not the
// service) — see `MainActivity.kt` §PERMISSIONS_CHANNEL.

import 'dart:async';

import 'package:flutter/services.dart';

class VpnService {
  VpnService({MethodChannel? channel})
      : _channel = channel ?? const MethodChannel('opene2ee/vpn');

  final MethodChannel _channel;

  /// Start the local TUN capture. The caller must already hold
  /// RESULT_OK from `VpnService.prepare`; if not, the call
  /// throws `PlatformException` with code `vpn_not_prepared`.
  Future<Map<String, Object?>> start() async {
    final r = await _channel.invokeMethod<Map<Object?, Object?>>('start');
    return (r ?? const {}).cast<String, Object?>();
  }

  /// Stop capture. When [graceful] is true, the service flushes
  /// the ring buffer + finalises the foreground notification
  /// before tearing down; when false, it tears down immediately
  /// (used on user-cancel).
  Future<Map<String, Object?>> stop({bool graceful = true}) async {
    final r = await _channel.invokeMethod<Map<Object?, Object?>>(
      'stop',
      {'graceful': graceful},
    );
    return (r ?? const {}).cast<String, Object?>();
  }

  /// Snapshot the service status.
  Future<Map<String, Object?>> status() async {
    final r = await _channel.invokeMethod<Map<Object?, Object?>>('status');
    return (r ?? const {}).cast<String, Object?>();
  }

  /// Drain the ring buffer of metadata snapshots. Returns a
  /// `List<Map<String, Object?>>` whose shape matches
  /// `ParsedPacket.toJson()` so the result can be fed straight
  /// into `TelemetryService.send`.
  ///
  /// The ring is consumed atomically — the caller's view is
  /// independent of subsequent sampling. The service caps the
  /// ring at `SAMPLING_CAP_PACKETS` (10) so a slow consumer does
  /// not leak memory.
  Future<List<Map<String, Object?>>> getSampledPackets() async {
    final r = await _channel.invokeMethod<List<Object?>>('getSampledPackets');
    if (r == null) return const [];
    return r
        .whereType<Map<Object?, Object?>>()
        .map((m) => m.cast<String, Object?>())
        .toList(growable: false);
  }
}
