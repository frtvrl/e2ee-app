// test/shared/telemetry_formatter_test.dart
//
// PR-9 / Mobile shared core — telemetry serializer unit tests.
//
// We pin:
//   * Required fields always emitted with the exact key names the Go backend
//     validates against (shared/schemas/telemetry.schema.json).
//   * Optional fields omitted when null — never emitted as `"x": null`.
//   * Enum wire names — `vodafone_tr`, `TLSv1.3`, `ip_reverse`, etc.
//   * Numeric range checks — entropy/peerScore/confidence bounds.

import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:opene2ee/shared/telemetry_formatter.dart';

Telemetry _base() => Telemetry(
      deviceIdHash: 'a' * 32,
      publicKeyFingerprint: 'b' * 32,
      operator: TelemetryOperator.turkcell,
      app: TelemetryApp.whatsapp,
      tlsFingerprint: 'c' * 64,
      entropy: 7.95,
      timestamp: '2026-07-05T04:13:01Z',
    );

void main() {
  group('telemetryToJson — required fields', () {
    test('emits all required keys with stable order', () {
      final json = telemetryToJson(_base());
      // Top-level keys (order-independent on the wire, but we lock it for
      // reviewer ergonomics):
      expect(json.keys.toList(), <String>[
        'device_id_hash',
        'public_key_fp',
        'operator',
        'app',
        'tls_fp',
        'entropy',
        'timestamp',
      ]);
    });

    test('round-trips through jsonEncode / jsonDecode', () {
      final t = _base();
      final encoded = telemetryToJsonString(t);
      final decoded = jsonDecode(encoded) as Map<String, dynamic>;
      expect(decoded['device_id_hash'], t.deviceIdHash);
      expect(decoded['public_key_fp'], t.publicKeyFingerprint);
      expect(decoded['operator'], 'turkcell');
      expect(decoded['app'], 'whatsapp');
      expect(decoded['entropy'], closeTo(7.95, 1e-9));
      expect(decoded['timestamp'], '2026-07-05T04:13:01Z');
    });
  });

  group('telemetryToJson — enum wire names', () {
    test('TelemetryOperator uses underscored names from the schema', () {
      final cases = <TelemetryOperator, String>{
        TelemetryOperator.turkcell: 'turkcell',
        TelemetryOperator.vodafoneTr: 'vodafone_tr',
        TelemetryOperator.turkTelekom: 'turk_telekom',
        TelemetryOperator.tmobileUs: 'tmobile_us',
        TelemetryOperator.deutscheTelekom: 'deutsche_telekom',
        TelemetryOperator.unknown: 'unknown',
      };
      for (final entry in cases.entries) {
        final json = telemetryToJson(_base().copyWith(operator: entry.key));
        expect(json['operator'], entry.value,
            reason: 'operator ${entry.key} should serialize as ${entry.value}');
      }
    });

    test('TelemetryApp uses names as-is from the schema', () {
      final cases = <TelemetryApp, String>{
        TelemetryApp.whatsapp: 'whatsapp',
        TelemetryApp.rcs: 'rcs',
        TelemetryApp.telegram: 'telegram',
        TelemetryApp.signal: 'signal',
      };
      for (final entry in cases.entries) {
        final json = telemetryToJson(_base().copyWith(app: entry.key));
        expect(json['app'], entry.value);
      }
    });

    test('TlsVersion uses the TLSv1.x wire forms', () {
      final cases = <TlsVersion, String>{
        TlsVersion.tlsv10: 'TLSv1.0',
        TlsVersion.tlsv11: 'TLSv1.1',
        TlsVersion.tlsv12: 'TLSv1.2',
        TlsVersion.tlsv13: 'TLSv1.3',
      };
      for (final entry in cases.entries) {
        final json = telemetryToJson(_base().copyWith(tlsVersion: entry.key));
        expect(json['tls_version'], entry.value);
      }
    });

    test('TelemetryOperatorSource uses underscored wire names', () {
      final cases = <TelemetryOperatorSource, String>{
        TelemetryOperatorSource.mnp: 'mnp',
        TelemetryOperatorSource.ipReverse: 'ip_reverse',
        TelemetryOperatorSource.asnDb: 'asn_db',
        TelemetryOperatorSource.unknown: 'unknown',
      };
      for (final entry in cases.entries) {
        final json = telemetryToJson(
          _base().copyWith(operatorSource: entry.key),
        );
        expect(json['operator_source'], entry.value);
      }
    });

    test('fromWire round-trips for every enum', () {
      // fromWire is what the backend uses to validate payloads; if any enum
      // forgets a wire form, the backend rejects the POST.
      for (final v in TelemetryOperator.values) {
        expect(TelemetryOperator.fromWire(v.wire), v,
            reason: 'TelemetryOperator ${v.name} wire=${v.wire}');
      }
      for (final v in TelemetryApp.values) {
        expect(TelemetryApp.fromWire(v.wire), v);
      }
      for (final v in TlsVersion.values) {
        expect(TlsVersion.fromWire(v.wire), v);
      }
      for (final v in TelemetryOperatorSource.values) {
        expect(TelemetryOperatorSource.fromWire(v.wire), v);
      }
      for (final v in MatchMode.values) {
        expect(MatchMode.fromWire(v.wire), v);
      }
    });

    test('fromWire returns null for unknown strings', () {
      expect(TelemetryOperator.fromWire('nope'), isNull);
      expect(TelemetryApp.fromWire(''), isNull);
      expect(TlsVersion.fromWire('SSLv3'), isNull);
    });
  });

  group('telemetryToJson — optional fields', () {
    test('omits optional fields when null', () {
      final json = telemetryToJson(_base());
      // None of these should appear at all (vs. being null).
      expect(json.containsKey('operator_source'), isFalse);
      expect(json.containsKey('tls_version'), isFalse);
      expect(json.containsKey('cipher_suites'), isFalse);
      expect(json.containsKey('sni'), isFalse);
      expect(json.containsKey('session_id'), isFalse);
      expect(json.containsKey('match_mode'), isFalse);
      expect(json.containsKey('peer_score'), isFalse);
      expect(json.containsKey('confidence'), isFalse);
      expect(json.containsKey('signature'), isFalse);
    });

    test('includes optional fields when set', () {
      final t = _base().copyWith(
        operatorSource: TelemetryOperatorSource.mnp,
        tlsVersion: TlsVersion.tlsv13,
        cipherSuites: <String>['TLS_AES_256_GCM_SHA384'],
        sni: 'web.whatsapp.com',
        sessionId: '019234d4-7c8a-7def-8ace-1234567890ab',
        matchMode: MatchMode.echobot,
        peerScore: 87.5,
        confidence: 0.92,
        signature: 'd' * 64,
      );
      final json = telemetryToJson(t);
      expect(json['operator_source'], 'mnp');
      expect(json['tls_version'], 'TLSv1.3');
      expect(json['cipher_suites'], <String>['TLS_AES_256_GCM_SHA384']);
      expect(json['sni'], 'web.whatsapp.com');
      expect(json['session_id'], '019234d4-7c8a-7def-8ace-1234567890ab');
      expect(json['match_mode'], 'echobot');
      expect(json['peer_score'], closeTo(87.5, 1e-9));
      expect(json['confidence'], closeTo(0.92, 1e-9));
      expect(json['signature'], 'd' * 64);
    });
  });

  group('telemetryToJson — range validation', () {
    test('entropy out of [0,8] throws', () {
      expect(
        () => telemetryToJson(_base().copyWith(entropy: -0.1)),
        throwsArgumentError,
      );
      expect(
        () => telemetryToJson(_base().copyWith(entropy: 8.1)),
        throwsArgumentError,
      );
    });

    test('peerScore out of [0,100] throws', () {
      expect(
        () => telemetryToJson(_base().copyWith(peerScore: -1.0)),
        throwsArgumentError,
      );
      expect(
        () => telemetryToJson(_base().copyWith(peerScore: 101.0)),
        throwsArgumentError,
      );
    });

    test('confidence out of [0,1] throws', () {
      expect(
        () => telemetryToJson(_base().copyWith(confidence: -0.01)),
        throwsArgumentError,
      );
      expect(
        () => telemetryToJson(_base().copyWith(confidence: 1.01)),
        throwsArgumentError,
      );
    });

    test('boundary values are accepted', () {
      expect(
        () => telemetryToJson(_base().copyWith(entropy: 0.0)),
        returnsNormally,
      );
      expect(
        () => telemetryToJson(_base().copyWith(entropy: 8.0)),
        returnsNormally,
      );
      expect(
        () => telemetryToJson(_base().copyWith(peerScore: 0.0, confidence: 0.0)),
        returnsNormally,
      );
      expect(
        () => telemetryToJson(_base().copyWith(peerScore: 100.0, confidence: 1.0)),
        returnsNormally,
      );
    });
  });
}

// Tiny test-local helper to keep tests readable. Production callers must
// construct via the canonical named constructor.
extension on Telemetry {
  Telemetry copyWith({
    String? deviceIdHash,
    String? publicKeyFingerprint,
    TelemetryOperator? operator,
    TelemetryApp? app,
    String? tlsFingerprint,
    double? entropy,
    String? timestamp,
    Object? operatorSource = _sentinel,
    Object? tlsVersion = _sentinel,
    Object? cipherSuites = _sentinel,
    Object? sni = _sentinel,
    Object? sessionId = _sentinel,
    Object? matchMode = _sentinel,
    Object? peerScore = _sentinel,
    Object? confidence = _sentinel,
    Object? signature = _sentinel,
  }) {
    return Telemetry(
      deviceIdHash: deviceIdHash ?? this.deviceIdHash,
      publicKeyFingerprint: publicKeyFingerprint ?? this.publicKeyFingerprint,
      operator: operator ?? this.operator,
      app: app ?? this.app,
      tlsFingerprint: tlsFingerprint ?? this.tlsFingerprint,
      entropy: entropy ?? this.entropy,
      timestamp: timestamp ?? this.timestamp,
      operatorSource: identical(operatorSource, _sentinel)
          ? this.operatorSource
          : operatorSource as TelemetryOperatorSource?,
      tlsVersion: identical(tlsVersion, _sentinel)
          ? this.tlsVersion
          : tlsVersion as TlsVersion?,
      cipherSuites: identical(cipherSuites, _sentinel)
          ? this.cipherSuites
          : cipherSuites as List<String>?,
      sni: identical(sni, _sentinel) ? this.sni : sni as String?,
      sessionId: identical(sessionId, _sentinel)
          ? this.sessionId
          : sessionId as String?,
      matchMode: identical(matchMode, _sentinel)
          ? this.matchMode
          : matchMode as MatchMode?,
      peerScore: identical(peerScore, _sentinel)
          ? this.peerScore
          : peerScore as double?,
      confidence: identical(confidence, _sentinel)
          ? this.confidence
          : confidence as double?,
      signature: identical(signature, _sentinel)
          ? this.signature
          : signature as String?,
    );
  }
}

const Object _sentinel = Object();