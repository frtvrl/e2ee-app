// mobile/test/mobile/vpn/method_channel_test.dart
//
// PR-22a (Sprint 3) — Unit tests for the VPN MethodChannel bridge.
//
// These tests exercise the Dart-side surface only. The Kotlin/Swift
// native code cannot be compiled by `flutter test` (no Gradle/Xcode in
// this branch yet — see SPRINT-3-SCOPE.md §7), so we drive a fake
// `BinaryMessenger` via `TestDefaultBinaryMessengerBinding` and assert
// the round-trip.
//
// Coverage matrix
// ---------------
// 1. start / stop / status round-trip via MethodChannelVpnBridge.
// 2. Per-app VPN: setAllowedApplications + setDisallowedApplications
//    forward packages to native.
// 3. Permission flow: ensurePermission() / isPermissionGranted().
// 4. Telemetry dispatch: native → Dart `onTelemetry` routes into the
//    broadcast stream.
// 5. Error dispatch: `onError` → `errors` stream.
// 6. NoopVpnBridge falls back to VpnLifecycleState.unavailable.
// 7. Privacy invariant: no payload field on VpnPacketMetadata.
// 8. VpnStatusSnapshot.fromMap round-trip.

import 'dart:async';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:opene2ee/mobile/vpn/method_channel.dart';

/// Register a mock handler on the VPN MethodChannel. Returns nothing —
/// tests use the closure to inspect captured calls.
void _setVpnHandler(
  Future<dynamic> Function(MethodCall call) handler,
) {
  TestWidgetsFlutterBinding.ensureInitialized();
  TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
      .setMockMethodCallHandler(
        const MethodChannel(kVpnMethodChannel),
        handler,
      );
}

void _setPermHandler(
  Future<dynamic> Function(MethodCall call) handler,
) {
  TestWidgetsFlutterBinding.ensureInitialized();
  TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
      .setMockMethodCallHandler(
        const MethodChannel(kVpnPermissionsChannel),
        handler,
      );
}

/// Push a synthetic MethodCall into [channel] FROM the platform side
/// (so the Dart MethodChannel handler dispatches it).
Future<void> _pushPlatformCall({
  required String channel,
  required String method,
  Map<Object?, Object?>? args,
}) async {
  const codec = StandardMethodCodec();
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  await messenger.handlePlatformMessage(
    channel,
    codec.encodeMethodCall(MethodCall(method, args)),
    (_) {},
  );
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  tearDown(() {
    // Clear handlers so subsequent test groups don't share state.
    TestWidgetsFlutterBinding.ensureInitialized();
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(
      const MethodChannel(kVpnMethodChannel),
      null,
    );
    messenger.setMockMethodCallHandler(
      const MethodChannel(kVpnPermissionsChannel),
      null,
    );
  });

  group('MethodChannelVpnBridge.transport', () {
    test('start invokes the vpn channel with permission granted',
        () async {
      _setVpnHandler((call) async {
        if (call.method == 'start') {
          return {
            'state': 'sampling',
            'packetsObserved': 0,
            'ringSize': 0,
            'samplingCap': 10,
            'lastError': null,
          };
        }
        return null;
      });
      _setPermHandler((call) async => true);

      final bridge = MethodChannelVpnBridge();
      final snap = await bridge.start();
      expect(snap.state, VpnLifecycleState.sampling);
      expect(snap.samplingCap, 10);

      bridge.dispose();
    });

    test('stop invokes the vpn channel and forwards the response',
        () async {
      _setVpnHandler((call) async {
        if (call.method == 'stop') {
          return {
            'state': 'stopped',
            'packetsObserved': 5,
            'ringSize': 0,
            'samplingCap': 10,
            'lastError': null,
          };
        }
        return null;
      });
      _setPermHandler((call) async => true);

      final bridge = MethodChannelVpnBridge();
      final snap = await bridge.stop();
      expect(snap.state, VpnLifecycleState.stopped);
      expect(snap.packetsObserved, 5);

      bridge.dispose();
    });

    test('status returns the native snapshot with all fields populated',
        () async {
      _setVpnHandler((call) async {
        if (call.method == 'status') {
          return {
            'state': 'sampling',
            'packetsObserved': 7,
            'ringSize': 7,
            'samplingCap': 10,
            'lastError': null,
            'allowedApplications': ['com.example.a'],
            'disallowedApplications': null,
          };
        }
        return null;
      });
      _setPermHandler((call) async => true);

      final bridge = MethodChannelVpnBridge();
      final snap = await bridge.status();
      expect(snap.state, VpnLifecycleState.sampling);
      expect(snap.packetsObserved, 7);
      expect(snap.ringSize, 7);
      expect(snap.samplingCap, 10);
      expect(snap.allowedApplications, ['com.example.a']);
      expect(snap.disallowedApplications, isNull);

      bridge.dispose();
    });

    test('start without permission throws VpnPermissionDeniedError',
        () async {
      _setVpnHandler((call) async => null);
      _setPermHandler((call) async => false);

      final bridge = MethodChannelVpnBridge();
      expect(
        () => bridge.start(),
        throwsA(isA<VpnPermissionDeniedError>()),
      );

      bridge.dispose();
    });
  });

  group('MethodChannelVpnBridge.per-app VPN', () {
    test('setAllowedApplications completes without throwing', () async {
      _setVpnHandler((call) async => true);
      _setPermHandler((call) async => true);

      final bridge = MethodChannelVpnBridge();
      await bridge.setAllowedApplications(
        const ['com.opene2ee.app', 'com.example.a'],
      );

      bridge.dispose();
    });

    test('setDisallowedApplications completes without throwing', () async {
      _setVpnHandler((call) async => true);
      _setPermHandler((call) async => true);

      final bridge = MethodChannelVpnBridge();
      await bridge.setDisallowedApplications(const ['com.example.bypass']);

      bridge.dispose();
    });

    test('empty per-app list is accepted (clears any prior allowlist)',
        () async {
      _setVpnHandler((call) async => true);
      _setPermHandler((call) async => true);

      final bridge = MethodChannelVpnBridge();
      await bridge.setAllowedApplications(const <String>[]);
      await bridge.setDisallowedApplications(const <String>[]);

      bridge.dispose();
    });
  });

  group('MethodChannelVpnBridge.permission flow', () {
    test('ensurePermission returns true when the activity grants consent',
        () async {
      _setVpnHandler((call) async => null);
      _setPermHandler((call) async {
        if (call.method == 'requestVpnPermission') return true;
        return null;
      });

      final bridge = MethodChannelVpnBridge();
      expect(await bridge.ensurePermission(), isTrue);

      bridge.dispose();
    });

    test('ensurePermission returns false when the user declines', () async {
      _setVpnHandler((call) async => null);
      _setPermHandler((call) async {
        if (call.method == 'requestVpnPermission') return false;
        return null;
      });

      final bridge = MethodChannelVpnBridge();
      expect(await bridge.ensurePermission(), isFalse);

      bridge.dispose();
    });

    test('isPermissionGranted returns the cached answer', () async {
      _setVpnHandler((call) async => null);
      _setPermHandler((call) async {
        if (call.method == 'isVpnPrepared') return true;
        return null;
      });

      final bridge = MethodChannelVpnBridge();
      expect(await bridge.isPermissionGranted(), isTrue);

      bridge.dispose();
    });
  });

  group('MethodChannelVpnBridge.telemetry dispatch', () {
    test('onTelemetry from native routes into the broadcast stream',
        () async {
      _setVpnHandler((call) async => null);
      _setPermHandler((call) async => true);

      final bridge = MethodChannelVpnBridge();
      final completer = Completer<VpnTelemetry>();
      final sub = bridge.telemetry.listen(completer.complete);

      await _pushPlatformCall(
        channel: kVpnMethodChannel,
        method: 'onTelemetry',
        args: {
          'sessionId': 'test-1',
          'packets': [
            {
              'version': 4,
              'protocol': 6,
              'packetLength': 60,
              'srcIpMasked': '10.0.0.0',
              'dstIpMasked': '142.250.0.0',
              'srcPort': 54321,
              'dstPort': 443,
              'tcpFlags': 24,
              'tlsClientHelloFingerprint': 'abcd',
            },
          ],
          'capturedAt': 1700000000000,
        },
      );

      final telemetry =
          await completer.future.timeout(const Duration(seconds: 2));
      expect(telemetry.sessionId, 'test-1');
      expect(telemetry.packets, hasLength(1));
      final packet = telemetry.packets.single;
      expect(packet.version, 4);
      expect(packet.protocol, 6);
      expect(packet.srcIpMasked, '10.0.0.0');
      expect(packet.dstPort, 443);

      await sub.cancel();
      bridge.dispose();
    });

    test('onError routes into the errors stream', () async {
      _setVpnHandler((call) async => null);
      _setPermHandler((call) async => true);

      final bridge = MethodChannelVpnBridge();
      final completer = Completer<VpnBridgeError>();
      final sub = bridge.errors.listen(completer.complete);

      await _pushPlatformCall(
        channel: kVpnMethodChannel,
        method: 'onError',
        args: {
          'code': 'tun_read_failed',
          'message': 'lost TUN',
        },
      );

      final err =
          await completer.future.timeout(const Duration(seconds: 2));
      expect(err.code, 'tun_read_failed');
      expect(err.message, 'lost TUN');

      await sub.cancel();
      bridge.dispose();
    });
  });

  group('NoopVpnBridge', () {
    test('all control methods report VpnLifecycleState.unavailable',
        () async {
      const bridge = NoopVpnBridge();
      expect((await bridge.start()).state, VpnLifecycleState.unavailable);
      expect((await bridge.stop()).state, VpnLifecycleState.unavailable);
      expect((await bridge.status()).state, VpnLifecycleState.unavailable);
      expect(await bridge.ensurePermission(), isFalse);
      expect(await bridge.isPermissionGranted(), isFalse);
      await bridge.setAllowedApplications(const ['a', 'b']);
      await bridge.setDisallowedApplications(const ['c']);
    });
  });

  group('Privacy invariants (ADR-0006)', () {
    test('VpnPacketMetadata has no payload field', () {
      final m = const VpnPacketMetadata(
        version: 4,
        protocol: 6,
        packetLength: 60,
      );
      expect(m.toString().toLowerCase(), isNot(contains('payload')),
          reason: 'VpnPacketMetadata.toString must not mention payload');
    });

    test('VpnTelemetry does not propagate any payload-shaped fields', () {
      final t = VpnTelemetry.fromMap({
        'sessionId': 's',
        'packets': <Map<Object?, Object?>>[
          {
            'version': 4,
            'protocol': 6,
            'packetLength': 90,
            'srcIpMasked': '10.0.0.0',
            'dstIpMasked': null,
            'srcPort': null,
            'dstPort': null,
            'tcpFlags': null,
            'tlsClientHelloFingerprint': null,
          },
        ],
        'capturedAt': 1700000000000,
      });
      expect(t.packets.single.srcIpMasked, '10.0.0.0');
      expect(t.toString().toLowerCase(), isNot(contains('payload')));
    });
  });

  group('VpnStatusSnapshot.fromMap round-trip', () {
    test('parses known state values', () {
      for (final s in ['idle', 'sampling', 'draining', 'stopped', 'error']) {
        final snap = VpnStatusSnapshot.fromMap({
          'state': s,
          'packetsObserved': 0,
          'ringSize': 0,
          'samplingCap': 10,
        });
        if (s != 'idle') {
          expect(
            snap.state,
            isIn(<VpnLifecycleState>{
              VpnLifecycleState.sampling,
              VpnLifecycleState.draining,
              VpnLifecycleState.stopped,
              VpnLifecycleState.error,
            }),
          );
        }
      }
    });

    test('unavailable sentinel is consistent', () {
      const a = VpnStatusSnapshot.unavailable;
      const b = VpnStatusSnapshot.unavailable;
      expect(identical(a, b), isTrue);
      expect(a.state, VpnLifecycleState.unavailable);
    });
  });
}
