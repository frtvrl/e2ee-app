// test/shared/api_client_test.dart
//
// PR-9 / Mobile shared core — ApiClient unit tests.
//
// We exercise the client against an in-process Dio MockAdapter (via
// mocktail) so no network is touched. The contract under test:
//   * X-API-Version: 1 header on every outbound request.
//   * Authorization: Bearer ... when configured.
//   * 401 → UnauthorizedException, 429 → RateLimitedException (with
//     Retry-After parsed), 2xx → ApiResponse with parsed JSON body.
//   * Typed endpoint helpers hit the right paths/methods.

import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:opene2ee/shared/api_client.dart';

/// Captures the request and returns a canned response. Acts as both
/// `onRequest` and `onResponse` so we can inspect what the Dio pipeline
/// produced after headers were applied.
class _CapturingAdapter implements HttpClientAdapter {
  final List<RequestOptions> seen = <RequestOptions>[];

  @override
  void close({bool force = false}) {}

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<Uint8List>? requestStream,
    Future<dynamic>? cancelFuture,
  ) async {
    seen.add(options);
    final body = utf8.encode(jsonEncode({
      'ok': true,
      'path': options.path,
      'method': options.method,
      'echo_headers': {
        for (final e in options.headers.entries) e.key: e.value,
      },
    }));
    return ResponseBody.fromBytes(
      body,
      200,
      headers: <String, List<String>>{
        'Content-Type': <String>['application/json'],
        'X-API-Version': <String>['1'],
      },
    );
  }
}

/// Same as _CapturingAdapter but lets the test drive the response code +
/// headers per call (e.g. for 401 / 429 paths).
class _StubAdapter implements HttpClientAdapter {
  int statusCode;
  Map<String, String> headers;
  Object? body;

  _StubAdapter({
    this.statusCode = 200,
    this.headers = const <String, String>{},
    this.body,
  });

  @override
  void close({bool force = false}) {}

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<Uint8List>? requestStream,
    Future<dynamic>? cancelFuture,
  ) async {
    Object payload;
    if (body != null) {
      payload = body!;
    } else {
      payload = <String, dynamic>{'echo_path': options.path};
    }
    // Always advertise JSON so dio's default transformer parses the bytes
    // into a Map. Without this, Response.data stays as the raw String even
    // when responseType=json (the transformer gates on Content-Type).
    final merged = <String, String>{
      'Content-Type': 'application/json',
      ...headers,
    };
    return ResponseBody.fromBytes(
      utf8.encode(payload is String ? payload : jsonEncode(payload)),
      statusCode,
      headers: <String, List<String>>{
        for (final e in merged.entries) e.key: <String>[e.value],
      },
    );
  }
}

void main() {
  setUpAll(() {
    // mocktail needs a registered fallback for any() on Options / Map args.
    registerFallbackValue(Options());
  });

  group('ApiClient.headers', () {
    test('every outbound request carries X-API-Version: 1', () async {
      final adapter = _CapturingAdapter();
      final dio = Dio(BaseOptions())
        ..httpClientAdapter = adapter;
      final client = ApiClient(
        config: const ApiConfig(baseUrl: 'https://api.test'),
        dio: dio,
      );

      await client.getMatrix(<String, dynamic>{'op': 'turkcell'});
      await client.createSession(<String, dynamic>{'mode': 'echobot'});
      await client.deleteUser('a' * 32);

      expect(adapter.seen, hasLength(3));
      for (final req in adapter.seen) {
        expect(req.headers['X-API-Version'], '1',
            reason: 'request to ${req.path} missing X-API-Version');
        expect(req.headers['Content-Type'], 'application/json');
      }
    });

    test('Authorization header is set when bearerToken is configured',
        () async {
      final adapter = _CapturingAdapter();
      final dio = Dio(BaseOptions())
        ..httpClientAdapter = adapter;
      final client = ApiClient(
        config: const ApiConfig(
          baseUrl: 'https://api.test',
          bearerToken: 'token-abc-123',
        ),
        dio: dio,
      );

      await client.getMatrix(<String, dynamic>{});
      expect(adapter.seen.single.headers['Authorization'],
          'Bearer token-abc-123');
    });

    test('no Authorization header when bearerToken is null', () async {
      final adapter = _CapturingAdapter();
      final dio = Dio(BaseOptions())
        ..httpClientAdapter = adapter;
      final client = ApiClient(
        config: const ApiConfig(baseUrl: 'https://api.test'),
        dio: dio,
      );

      await client.getMatrix(<String, dynamic>{});
      expect(adapter.seen.single.headers.containsKey('Authorization'), isFalse);
    });
  });

  group('ApiClient error mapping', () {
    test('401 throws UnauthorizedException', () async {
      final adapter = _StubAdapter(statusCode: 401);
      final dio = Dio(BaseOptions())
        ..httpClientAdapter = adapter;
      final client = ApiClient(
        config: const ApiConfig(baseUrl: 'https://api.test'),
        dio: dio,
      );

      expect(
        () => client.getMatrix(<String, dynamic>{}),
        throwsA(isA<UnauthorizedException>()),
      );
    });

    test('429 throws RateLimitedException with parsed Retry-After',
        () async {
      final adapter = _StubAdapter(
        statusCode: 429,
        headers: <String, String>{'Retry-After': '7'},
      );
      final dio = Dio(BaseOptions())
        ..httpClientAdapter = adapter;
      final client = ApiClient(
        config: const ApiConfig(baseUrl: 'https://api.test'),
        dio: dio,
      );

      Object? thrown;
      try {
        await client.getMatrix(<String, dynamic>{});
      } catch (e) {
        thrown = e;
      }
      expect(thrown, isA<RateLimitedException>());
      expect((thrown as RateLimitedException).retryAfter,
          const Duration(seconds: 7));
    });

    test('429 without Retry-After still throws RateLimitedException',
        () async {
      final adapter = _StubAdapter(statusCode: 429);
      final dio = Dio(BaseOptions())
        ..httpClientAdapter = adapter;
      final client = ApiClient(
        config: const ApiConfig(baseUrl: 'https://api.test'),
        dio: dio,
      );
      expect(
        () => client.getMatrix(<String, dynamic>{}),
        throwsA(isA<RateLimitedException>()),
      );
    });

    test('200 returns ApiResponse with parsed body + X-API-Version header',
        () async {
      final adapter = _StubAdapter(
        statusCode: 200,
        headers: <String, String>{'X-API-Version': '1'},
        body: <String, dynamic>{'id': 'abc-123', 'status': 'open'},
      );
      final dio = Dio(BaseOptions())
        ..httpClientAdapter = adapter;
      final client = ApiClient(
        config: const ApiConfig(baseUrl: 'https://api.test'),
        dio: dio,
      );

      final resp = await client.createSession(<String, dynamic>{'mode': 'p2p'});
      expect(resp.statusCode, 200);
      expect(resp.body['id'], 'abc-123');
      expect(resp.body['status'], 'open');
      expect(resp.apiVersion, '1');
    });
  });

  group('ApiClient typed endpoints', () {
    late _CapturingAdapter adapter;
    late ApiClient client;

    setUp(() {
      adapter = _CapturingAdapter();
      final dio = Dio(BaseOptions())
        ..httpClientAdapter = adapter;
      client = ApiClient(
        config: const ApiConfig(baseUrl: 'https://api.test'),
        dio: dio,
      );
    });

    test('createSession → POST /api/v1/sessions', () async {
      await client.createSession(<String, dynamic>{});
      expect(adapter.seen.single.method, 'POST');
      expect(adapter.seen.single.path, '/api/v1/sessions');
    });

    test('getSession → GET /api/v1/sessions/{id}', () async {
      await client.getSession('019234d4-7c8a-7def-8ace-1234567890ab');
      expect(adapter.seen.single.method, 'GET');
      expect(adapter.seen.single.path,
          '/api/v1/sessions/019234d4-7c8a-7def-8ace-1234567890ab');
    });

    test('postTelemetry → POST /api/v1/sessions/{id}/telemetry', () async {
      await client.postTelemetry(
        'sess-1',
        <String, dynamic>{'device_id_hash': 'a' * 32},
      );
      expect(adapter.seen.single.method, 'POST');
      expect(adapter.seen.single.path, '/api/v1/sessions/sess-1/telemetry');
    });

    test('getResult → GET /api/v1/sessions/{id}/result', () async {
      await client.getResult('sess-1');
      expect(adapter.seen.single.method, 'GET');
      expect(adapter.seen.single.path, '/api/v1/sessions/sess-1/result');
    });

    test('deleteUser → DELETE /api/v1/users/{hash}', () async {
      await client.deleteUser('a' * 32);
      expect(adapter.seen.single.method, 'DELETE');
      expect(adapter.seen.single.path, '/api/v1/users/${'a' * 32}');
    });

    test('getMatrix → GET /api/v1/matrix with query params', () async {
      await client.getMatrix(<String, dynamic>{
        'operator': 'turkcell',
        'app': 'whatsapp',
      });
      final req = adapter.seen.single;
      expect(req.method, 'GET');
      expect(req.path, '/api/v1/matrix');
      expect(req.queryParameters['operator'], 'turkcell');
      expect(req.queryParameters['app'], 'whatsapp');
    });

    test('lookupOperator → GET /api/v1/operator/lookup with query',
        () async {
      await client.lookupOperator(<String, dynamic>{'phone': '+905551234567'});
      final req = adapter.seen.single;
      expect(req.method, 'GET');
      expect(req.path, '/api/v1/operator/lookup');
      expect(req.queryParameters['phone'], '+905551234567');
    });
  });

  group('ApiConfig.copyWith', () {
    test('preserves timeouts when only updating bearerToken', () {
      const original = ApiConfig(baseUrl: 'https://x.test');
      final updated = original.copyWith(bearerToken: 'tok');
      expect(updated.baseUrl, 'https://x.test');
      expect(updated.bearerToken, 'tok');
      expect(updated.connectTimeout, original.connectTimeout);
      expect(updated.sendReceiveTimeout, original.sendReceiveTimeout);
    });
  });
}