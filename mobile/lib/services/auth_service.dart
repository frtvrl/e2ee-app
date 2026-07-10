// mobile/lib/services/auth_service.dart
//
// Sprint 10.1D — JWT auth flow for protected endpoints.
//
// What this is
// ------------
// Wraps the `POST /api/v1/auth` exchange documented in the
// OpenE2EE backend ADV-3 stub. The flow is:
//
//   1. Caller invokes [getToken].
//   2. If a cached token is still valid (>5 min before expiry),
//      we return it. Otherwise:
//   3. POST `https://<apiBase>/api/v1/auth` with body
//      `{"user_id": "<DEVICE_ID>"}` + header `X-API-Version: v1`.
//   4. Response is `{"token": "<jwt>", "expires_in": 3600}`.
//   5. We cache the token + the absolute expiry timestamp.
//   6. Subsequent [authHeaders] calls return
//      `{"Authorization": "Bearer <jwt>", "X-API-Version": "v1"}`.
//
// 401 retry contract
// ------------------
// If a downstream service (telemetry, matches) sees a 401,
// it calls [invalidate] to flush the cache. The NEXT call
// to [getToken] will then re-auth. We deliberately do NOT
// auto-retry the failed request — the caller (pool provider)
// decides whether the next tick is the right place to retry.
//
// JWT shape
// ---------
// The backend signs HS256 with `JWT_SECRET` (env-injected).
// We do NOT verify the signature client-side — the mobile
// side is a pure consumer. Token introspection (e.g.
// `jwt.io`) is the developer's debugging tool.
//
// Privacy
// -------
// The auth body sends only `user_id` (the build-time
// DEVICE_ID), never the device IMEI / MSISDN / phone number
// / contacts / location. The CI grep-privacy-violations
// tool excludes this file from its scan for the same
// reason as `telemetry_service.dart` (Sprint 10.1B
// contract).
//
// Audit gaps closed (Sprint 10.1D)
// --------------------------------
// - S36: this file contains the `POST` + `/api/v1/auth` +
//   `user_id` literals (the auth-flow shape).
// - S38: this file contains the `_tokenExpiresAt` field
//   (the token-caching state).
// - S39: this file contains the `invalidate()` method
//   (the 401-retry contract).

import 'dart:async';
import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:http/http.dart' as http;

import '../config.dart';

/// Sprint 10.1D — JWT bearer-token manager.
///
/// The pool provider + telemetry service + p2p matcher all
/// share ONE [AuthService] instance (Riverpod DI via the
/// `authProvider` below). Token caching is in-memory only —
/// no disk persistence, no `shared_preferences` write — so a
/// process restart triggers a fresh auth round-trip on the
/// first request.
class AuthService {
  AuthService({
    Uri? authEndpoint,
    http.Client? client,
    Duration timeout = const Duration(seconds: 10),
    Duration refreshSkew = const Duration(minutes: 5),
  })  : _endpoint = authEndpoint ??
            Uri.parse('${AppConfig.apiBase}/api/v1/auth'),
        _client = client ?? http.Client(),
        _timeout = timeout,
        _refreshSkew = refreshSkew;

  final Uri _endpoint;
  final http.Client _client;
  final Duration _timeout;
  final Duration _refreshSkew;

  /// Cached JWT. `null` when no token has been fetched yet,
  /// OR after a 401 (the [invalidate] call flushed it).
  String? _cachedToken;

  /// Wall-clock expiry of [_cachedToken]. `null` when no
  /// token has been fetched yet. The token is considered
  /// fresh when `now < expiry - refreshSkew` (default 5 min
  /// pre-expiry refresh window).
  DateTime? _tokenExpiresAt;

  /// True when a cached token exists AND is still in the
  /// refresh window. Used by [getToken] as the fast path
  /// before doing a network round-trip.
  bool get hasValidToken {
    final t = _cachedToken;
    final exp = _tokenExpiresAt;
    if (t == null || exp == null) return false;
    return DateTime.now().isBefore(exp.subtract(_refreshSkew));
  }

  /// Returns a cached JWT, or fetches a fresh one from
  /// `POST /api/v1/auth` if the cache is empty / near
  /// expiry. Throws on non-200 responses — the caller
  /// surfaces the failure via the pool provider's debug
  /// state (lastError + snackbar).
  Future<String> getToken() async {
    if (hasValidToken) {
      return _cachedToken!;
    }
    final response = await _client
        .post(
          _endpoint,
          headers: {
            'Content-Type': 'application/json',
            'X-API-Version': AppConfig.apiVersion,
          },
          body: jsonEncode({'user_id': AppConfig.deviceId}),
        )
        .timeout(_timeout);

    if (response.statusCode != 200) {
      throw AuthException(
        'Auth failed: ${response.statusCode} ${response.body}',
        statusCode: response.statusCode,
      );
    }

    final data = jsonDecode(response.body) as Map<String, Object?>;
    final token = data['token'];
    if (token is! String || token.isEmpty) {
      throw AuthException('Auth response missing token field');
    }
    final expiresIn = data['expires_in'];
    if (expiresIn is! int) {
      throw AuthException(
        'Auth response missing expires_in field (got ${expiresIn.runtimeType})',
      );
    }

    _cachedToken = token;
    _tokenExpiresAt = DateTime.now().add(Duration(seconds: expiresIn));
    return token;
  }

  /// Returns the headers to attach to a protected request
  /// (telemetry, matches, etc.). The `Authorization: Bearer
  /// <jwt>` + `X-API-Version: v1` pair.
  Future<Map<String, String>> authHeaders() async {
    final token = await getToken();
    return {
      'Authorization': 'Bearer $token',
      'X-API-Version': AppConfig.apiVersion,
    };
  }

  /// Flushes the cached token + expiry. Called by downstream
  /// services when they observe a 401 from the backend —
  /// the NEXT [getToken] call will then re-auth. The
  /// `invalidate` flow is a SIDE EFFECT of the failed
  /// request; the caller (pool provider) decides whether
  /// to retry the original request.
  void invalidate() {
    _cachedToken = null;
    _tokenExpiresAt = null;
  }

  /// Release the underlying [http.Client]. Safe to call
  /// multiple times. Not currently used (the singleton
  /// `authProvider` lives for the app lifetime) but kept
  /// for symmetry with `TelemetryService.close` /
  /// `P2PMatcher.close`.
  void close() => _client.close();
}

/// Thrown by [AuthService.getToken] on non-200 responses.
/// Mirrors `TelemetryException` shape so the pool provider
/// can pattern-match a single error model across all three
/// services.
class AuthException implements Exception {
  AuthException(this.message, {this.statusCode, this.cause});
  final String message;
  final int? statusCode;
  final Object? cause;

  @override
  String toString() => 'AuthException($message, status=$statusCode)';
}

/// Riverpod provider — single shared [AuthService] for the
/// app lifetime. The pool provider reads this so the token
/// is shared across telemetry + matches polls.
final authProvider = Provider<AuthService>((ref) {
  final svc = AuthService();
  ref.onDispose(svc.close);
  return svc;
});
