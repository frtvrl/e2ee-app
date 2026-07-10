// mobile/test/telemetry_service_test.dart
//
// Sprint 10.1B — TelemetryService tests with a MockClient.
// Verifies the 3 documented response cases:
//   1. 202 Accepted → success (no throw)
//   2. 401 Unauthorized → TelemetryException with statusCode 401
//   3. 429 Too Many Requests → TelemetryException with statusCode 429
//
// Privacy
// -------
// The test body is built from a small fixture list of
// `ParsedPacket` instances — same shape the live code uses.
// We never send raw packet bytes.

import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:opene2ee/services/packet_parser.dart';
import 'package:opene2ee/services/telemetry_service.dart';

void main() {
  group('TelemetryService', () {
    test('202 Accepted → returns normally', () async {
      http.Client client = MockClient((req) async {
        expect(req.method, 'POST');
        expect(
          req.headers['Authorization'],
          startsWith('Bearer '),
        );
        expect(req.headers['Content-Type'], 'application/json');
        final body = jsonDecode(req.body) as Map<String, Object?>;
        expect(body['sessionId'], isA<String>());
        expect(body['sampledAt'], isA<String>());
        expect(body['samplingCap'], 10);
        expect(body['packets'], isA<List<dynamic>>());
        return http.Response('', 202);
      });
      final svc = TelemetryService(
        client: client,
        apiKey: 'test-key',
        sessionId: 'sess-test',
      );
      final packets = [
        ParsedPacket(
          version: 4,
          protocol: Protocol.tcp,
          protocolNumber: 6,
          totalLength: 40,
          srcIpMasked: '10.0.0.0',
          dstIpMasked: '93.184.216.0',
          srcPort: 54321,
          dstPort: 443,
          tcpFlags: 0x12,
        ),
      ];
      await svc.send(packets); // expect no throw
      svc.close();
    });

    test('401 Unauthorized → throws TelemetryException', () async {
      http.Client client = MockClient((req) async {
        return http.Response('unauthorized', 401);
      });
      final svc = TelemetryService(client: client, apiKey: 'bad-key');
      expect(
        () => svc.send([
          ParsedPacket(
            version: 4,
            protocol: Protocol.udp,
            protocolNumber: 17,
            totalLength: 28,
            srcIpMasked: '0.0.0.0',
            dstIpMasked: '0.0.0.0',
            srcPort: 12345,
            dstPort: 53,
          ),
        ]),
        throwsA(
          isA<TelemetryException>()
              .having((e) => e.statusCode, 'statusCode', 401),
        ),
      );
      svc.close();
    });

    test('429 Too Many Requests → throws TelemetryException', () async {
      http.Client client = MockClient((req) async {
        return http.Response('rate-limited', 429);
      });
      final svc = TelemetryService(client: client, apiKey: 'test-key');
      expect(
        () => svc.send([
          ParsedPacket(
            version: 6,
            protocol: Protocol.tcp,
            protocolNumber: 6,
            totalLength: 60,
            srcIpMasked: '2001:db8:0:0:0:0:0:0',
            dstIpMasked: '2001:db8:1:0:0:0:0:0',
            srcPort: 49152,
            dstPort: 443,
            tcpFlags: 0x18,
          ),
        ]),
        throwsA(
          isA<TelemetryException>()
              .having((e) => e.statusCode, 'statusCode', 429),
        ),
      );
      svc.close();
    });

    test('Empty packet list → no HTTP call (no-op)', () async {
      var calls = 0;
      http.Client client = MockClient((req) async {
        calls += 1;
        return http.Response('', 202);
      });
      final svc = TelemetryService(client: client);
      await svc.send(const []);
      expect(calls, 0);
      svc.close();
    });
  });
}
