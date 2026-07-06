// lib/shared/api_client.dart
//
// PR-9 / Mobile shared core — REST client.
//
// Why dio and not `package:http`
// ------------------------------
// We need:
//   * Stable, structured headers on every request (X-API-Version).
//   * Typed exceptions for the two cases we explicitly handle here:
//     401 (token revoked / device deleted per FR-7) and 429 (Kong rate-limit).
//   * A single configuration surface (base URL, timeouts, headers) so PR-10
//     (mobile-only) and PR-11 (web dashboard) share the same network layer.
// `package:http` would force us to hand-roll each of those; `dio` already
// supplies them.
//
// API contract
// ------------
// REST surface per `docs/ADR-0005-api-contracts.md`:
//   POST   /api/v1/sessions
//   GET    /api/v1/sessions/{id}
//   POST   /api/v1/sessions/{id}/telemetry
//   GET    /api/v1/sessions/{id}/result
//   DELETE /api/v1/users/{device_id_hash}
//   GET    /api/v1/matrix
//   GET    /api/v1/operator/lookup
//
// All requests carry `X-API-Version: 1`. All responses carry
// `X-API-Version` which we surface via [ApiResponse.apiVersion] so the
// caller can warn on a downgrade (F8 risk in RISKS.md).
//
// Privacy contract
// ----------------
// * `Authorization` header is the bearer token issued by the backend on
//   device registration. The token is opaque here; nothing in this module
//   logs it.
// * No PII flows through this module. The body shape is whatever the
//   caller provides — we trust the caller's type signature.
// * `ApiClient.dio.options.headers` are NEVER mutated to include
//   request-scoped credentials; use the per-request `options.headers` param
//   instead, to keep the global headers sanitizable at any moment.

import 'dart:async';

import 'package:dio/dio.dart';

/// The current wire-level API version this client speaks.
///
/// Backend's matching rule (`internal/api/middleware.go` in PR-7) returns
/// 400 if `X-API-Version` is missing or unknown. Bump together with
/// `shared/schemas/*.schema.json` revisions.
const int kApiVersion = 1;

/// Thrown by [ApiClient] when the backend replies 401 Unauthorized.
///
/// Per ADR-0005 + RISKS §F1: 401 indicates the device's stored token is
/// invalid or has been revoked (e.g. via DELETE /api/v1/users/{hash}). The
/// UI layer is expected to wipe local cached credentials and re-register.
class UnauthorizedException implements Exception {
  final String? reason;
  final RequestOptions request;
  UnauthorizedException(this.request, {this.reason});

  @override
  String toString() => 'UnauthorizedException(${request.uri}): '
      '${reason ?? "no reason given"}';
}

/// Thrown by [ApiClient] when the backend (or Kong gateway) replies 429
/// Too Many Requests. RISKS §F10 — rate-limit 100 req/min/device.
class RateLimitedException implements Exception {
  /// Seconds until the client may retry. Mirrors the `Retry-After` header
  /// (RFC 7231 §7.1.3) when present; null if the server did not advertise.
  final Duration? retryAfter;
  final RequestOptions request;

  RateLimitedException(this.request, {this.retryAfter});

  @override
  String toString() => 'RateLimitedException(${request.uri}): '
      'retryAfter=${retryAfter ?? "<unspecified>"}';
}

/// Lightweight value-type that captures the subset of an HTTP response we
/// care about. Keeps callers free of `package:dio`'s `Response` (which drags
/// in interceptor state) when they only need the body + a few headers.
class ApiResponse {
  final int statusCode;
  final Map<String, dynamic> body;
  final String? apiVersion;

  const ApiResponse({
    required this.statusCode,
    required this.body,
    this.apiVersion,
  });
}

/// Configuration knobs. Defaults match the dev topology described in
/// `docs/DEPLOYMENT.md` (Kong → backend, all on localhost during tests).
class ApiConfig {
  /// Base URL with NO trailing slash. e.g. `https://api.opene2ee.com`.
  final String baseUrl;

  /// Bearer token issued by the backend on device registration. May be null
  /// for the unauthenticated `/register` round-trip.
  final String? bearerToken;

  /// Connection timeout. Default 10s — long enough for a VPN-tunneled link,
  /// short enough that the UI doesn't freeze.
  final Duration connectTimeout;

  /// Per-request timeout. Default 15s.
  final Duration sendReceiveTimeout;

  const ApiConfig({
    required this.baseUrl,
    this.bearerToken,
    this.connectTimeout = const Duration(seconds: 10),
    this.sendReceiveTimeout = const Duration(seconds: 15),
  });

  ApiConfig copyWith({String? baseUrl, String? bearerToken}) => ApiConfig(
        baseUrl: baseUrl ?? this.baseUrl,
        bearerToken: bearerToken ?? this.bearerToken,
        connectTimeout: connectTimeout,
        sendReceiveTimeout: sendReceiveTimeout,
      );
}

/// Thin wrapper around `Dio` that enforces the OpenE2EE wire contract.
///
/// Construction is cheap; the underlying `Dio` is reused across requests.
/// For tests, pass a custom [Dio] (or wrap one with a `MockAdapter`); see
/// `mobile/test/shared/api_client_test.dart`.
class ApiClient {
  final Dio dio;
  final ApiConfig config;

  ApiClient._(this.dio, this.config);

  /// Build a client. If [dio] is provided (typically for tests), it is used
  /// verbatim; otherwise a fresh `Dio` is constructed with the config.
  factory ApiClient({required ApiConfig config, Dio? dio}) {
    final d = dio ??
        Dio(
          BaseOptions(
            baseUrl: config.baseUrl,
            connectTimeout: config.connectTimeout,
            sendTimeout: config.sendReceiveTimeout,
            receiveTimeout: config.sendReceiveTimeout,
            responseType: ResponseType.json,
          ),
        );

    // Apply / refresh headers and baseUrl regardless of who built the Dio,
    // so a test can pass a pre-configured instance and still get the
    // X-API-Version contract enforced.
    final baseHeaders = <String, String>{
      'Content-Type': 'application/json',
      'Accept': 'application/json',
      'X-API-Version': '$kApiVersion',
    };
    d.options.headers
      ..clear()
      ..addAll(baseHeaders);
    if (d.options.baseUrl.isEmpty) {
      d.options.baseUrl = config.baseUrl;
    }
    if (config.bearerToken != null) {
      d.options.headers['Authorization'] = 'Bearer ${config.bearerToken}';
    }

    final client = ApiClient._(d, config);
    d.interceptors.add(_ErrorMappingInterceptor());
    return client;
  }

  // ---------------------------------------------------------------------------
  // Endpoints — typed wrappers over the raw [request] / [requestJson] methods.
  // ---------------------------------------------------------------------------

  Future<ApiResponse> createSession(Map<String, dynamic> body) =>
      _postJson('/api/v1/sessions', body);

  Future<ApiResponse> getSession(String sessionId) =>
      _get('/api/v1/sessions/$sessionId');

  Future<ApiResponse> postTelemetry(String sessionId, Map<String, dynamic> body) =>
      _postJson('/api/v1/sessions/$sessionId/telemetry', body);

  Future<ApiResponse> getResult(String sessionId) =>
      _get('/api/v1/sessions/$sessionId/result');

  Future<ApiResponse> deleteUser(String deviceIdHash) =>
      _delete('/api/v1/users/$deviceIdHash');

  Future<ApiResponse> getMatrix(Map<String, dynamic> query) =>
      _get('/api/v1/matrix', queryParameters: query);

  Future<ApiResponse> lookupOperator(Map<String, dynamic> query) =>
      _get('/api/v1/operator/lookup', queryParameters: query);

  // ---------------------------------------------------------------------------
  // Raw HTTP surface — generic on purpose so PR-10/11 can add endpoints
  // without touching this class.
  // ---------------------------------------------------------------------------

  Future<ApiResponse> request({
    required String method,
    required String path,
    Map<String, dynamic>? queryParameters,
    Object? body,
    Map<String, String>? headers,
  }) async {
    final opts = Options(
      method: method,
      headers: headers,
      responseType: ResponseType.json,
    );
    try {
      final resp = await dio.request<dynamic>(
        path,
        data: body,
        queryParameters: queryParameters,
        options: opts,
      );
      return _wrap(resp);
    } on DioException catch (e) {
      throw _mapDioException(e);
    }
  }

  /// Convenience: POST a JSON-serializable map. Dio will encode it via its
  /// configured `JsonEncoder`; the `Content-Type: application/json` header is
  /// already on the BaseOptions.
  Future<ApiResponse> requestJson(
    String method,
    String path, {
    Map<String, dynamic>? body,
    Map<String, dynamic>? queryParameters,
  }) =>
      request(
        method: method,
        path: path,
        body: body,
        queryParameters: queryParameters,
      );

  Future<ApiResponse> _postJson(String path, Map<String, dynamic> body) =>
      requestJson('POST', path, body: body);

  Future<ApiResponse> _get(String path, {Map<String, dynamic>? queryParameters}) =>
      requestJson('GET', path, queryParameters: queryParameters);

  Future<ApiResponse> _delete(String path) => requestJson('DELETE', path);

  ApiResponse _wrap(Response<dynamic> resp) {
    final raw = resp.data;
    // Normalize the body into a Map<String, dynamic>:
    //   * JSON object  → use as-is.
    //   * JSON array   → wrap under 'items' so callers can `.body['items']`.
    //   * scalar/null  → wrap under 'value' so the Map type contract holds.
    final Map<String, dynamic> body;
    if (raw is Map) {
      body = raw.cast<String, dynamic>();
    } else if (raw is List) {
      body = <String, dynamic>{'items': List<dynamic>.unmodifiable(raw)};
    } else {
      body = <String, dynamic>{'value': raw};
    }
    final apiVersion = resp.headers.value('X-API-Version');
    return ApiResponse(
      statusCode: resp.statusCode ?? 0,
      body: body,
      apiVersion: apiVersion,
    );
  }

  static Exception _mapDioException(DioException e) {
    final status = e.response?.statusCode ?? 0;
    if (status == 401) {
      return UnauthorizedException(
        e.requestOptions,
        reason: e.response?.statusMessage,
      );
    }
    if (status == 429) {
      final retryAfter = e.response?.headers.value('Retry-After');
      Duration? dur;
      if (retryAfter != null) {
        final secs = int.tryParse(retryAfter);
        dur = secs != null ? Duration(seconds: secs) : null;
      }
      return RateLimitedException(e.requestOptions, retryAfter: dur);
    }
    return e;
  }
}

/// Interceptor that maps Dio-level errors to typed [Exception]s so callers
/// don't need to import `package:dio` just to handle 401/429.
class _ErrorMappingInterceptor extends Interceptor {
  _ErrorMappingInterceptor();

  @override
  void onError(DioException err, ErrorInterceptorHandler handler) {
    final mapped = ApiClient._mapDioException(err);
    if (mapped is! DioException) {
      handler.reject(DioException(
        requestOptions: err.requestOptions,
        response: err.response,
        type: err.type,
        error: mapped,
        stackTrace: err.stackTrace,
      ));
      return;
    }
    handler.next(err);
  }
}