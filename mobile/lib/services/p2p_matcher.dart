// mobile/lib/services/p2p_matcher.dart
//
// Sprint 10.1B + 10.1D + 10.1E — peer-to-peer matcher via JWT-auth.
//
// What this is
// ------------
// Polls the backend for an active receiver session, so the
// "Aktif Nöbet" pool screen can show the user a peer match.
//
// Sprint 10.1B — sessionId-keyed match lookup (broken: 404'd
//                because the backend never had that route)
// Sprint 10.1D — JWT auth (Bearer <jwt> + X-API-Version: 1)
// Sprint 10.1E — Endpoint fix
// ----------------------------
// The 10.1B endpoint (a sessionId-keyed match lookup) does NOT
// exist on the OpenE2EE backend (verified in `router.go` — the
// public + protected routes are auth, matrix, operator/lookup,
// sessions, telemetry, webrtc, users; there is no legacy
// matches route). Every poll returned 404, and the pool
// provider surfaced the 404 via `lastError`.
//
// Sprint 10.1E replaces the broken 10.1B endpoint with
// `GET /api/v1/sessions` (the existing session-list endpoint)
// and does the active-receiver filter on the mobile side per
// the brief's option C:
//
//   - `GET <apiBase>/api/v1/sessions` (list all sessions)
//   - mobile filter: `status == "active"` AND `role == "receiver"`
//     AND `device_id_hash != selfDeviceId`
//   - the first surviving id is the peer for the current tick
//
// Why option C and not A/B?
// -------------------------
// Option A (add a `status=` + `role=` server-side filter) and
// Option B (add a dedicated peer-creation endpoint) both
// require a backend change. The Owner directive for 10.1E
// ("backend dokunmadan, sadece P2PMatcher değişir") picks
// option C — mobile-side filter against the existing list
// endpoint, no backend round-trip needed.
//
// Privacy
// -------
// We send our own `sessionId` (a per-process random string
// from `TelemetryService._generateSessionId`) — not the device
// installation id, not the IMEI/MSISDN, and not the masked IP.
// The backend returns a list of session metadata records; the
// peer filter keys on `device_id_hash` so we never see raw
// device ids.
//
// Error handling
// --------------
// 200 -> parse body, filter, return `List<String>` of peer session
//   ids (may be empty when no active receiver is available yet).
// 401 / 403 -> invalidate cached JWT, return `[]`. The pool
//   provider's lastError surfaces the failure; the next tick
//   re-auths automatically.
// 5xx / network error -> throw; pool provider logs + retries
//   on the next tick.

import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

import '../config.dart';
import 'auth_service.dart';

class P2PMatcher {
  P2PMatcher({
    Uri? endpoint,
    String? apiKey,
    AuthService? auth,
    http.Client? client,
    Duration timeout = const Duration(seconds: 5),
  })  : _endpoint = endpoint ??
            // Sprint 10.1E — sessions list path (replaces the
            // 10.1B/10.1D legacy matches path that 404'd because
            // the backend never had that route).
            Uri.parse('${AppConfig.apiBase}/api/v1/sessions'),
        _apiKey = apiKey ?? kApiKey,
        _auth = auth ?? AuthService(),
        _client = client ?? http.Client(),
        _timeout = timeout;

  final Uri _endpoint;
  // Retained for 10.1B backwards-compat — the 10.1E primary
  // path uses _auth.authHeaders(). NOT removed so a test that
  // constructs P2PMatcher without an AuthService can still
  // call findActiveReceivers.
  final String _apiKey;
  final AuthService _auth;
  final http.Client _client;
  final Duration _timeout;

  /// List sessions and filter to active receivers other than
  /// ourselves. Returns the matching session ids in the order
  /// the backend returned them (typically newest-first by
  /// creation timestamp). Returns an empty list when:
  ///   - the backend returned no sessions,
  ///   - all sessions are filtered out (none active, none
  ///     receivers, or only the caller's own session),
  ///   - the JWT was rejected (401/403) and the cache is
  ///     flushed for the next tick.
  /// Throws on any other status / transport error.
  Future<List<String>> findActiveReceivers(String selfDeviceId) async {
    try {
      // Sprint 10.1D — pull a JWT via auth_service, then
      // GET. The `authHeaders()` call also re-auths if the
      // cached token is near expiry.
      final headers = await _auth.authHeaders();
      headers['Accept'] = 'application/json';
      final resp = await _client
          .get(
            _endpoint,
            headers: headers,
          )
          .timeout(_timeout);
      if (resp.statusCode == 200) {
        final body = jsonDecode(resp.body);
        if (body is! Map<String, Object?>) {
          throw const FormatException(
            'response body is not a JSON object',
          );
        }
        final rawSessions = body['sessions'];
        if (rawSessions is! List) {
          // Tolerate missing / non-list field — treat as empty
          // pool rather than throwing (the previous contract
          // was a 204 "no peer" and the pool provider handles
          // that case the same way).
          return <String>[];
        }
        final peers = <String>[];
        for (final s in rawSessions) {
          if (s is! Map) continue;
          final status = s['status'];
          final role = s['role'];
          final deviceIdHash = s['device_id_hash'];
          final id = s['id'];
          if (id is! String || id.isEmpty) continue;
          if (status != 'active') continue;
          if (role != 'receiver') continue;
          if (deviceIdHash is String && deviceIdHash == selfDeviceId) {
            continue;
          }
          peers.add(id);
        }
        return peers;
      }
      if (resp.statusCode == 204) return <String>[];
      if (resp.statusCode == 401 || resp.statusCode == 403) {
        // Flush the cached JWT — next call will re-auth.
        // Return empty (not throw) so the pool provider
        // treats this as "no peer" and retries on the
        // next tick with a fresh JWT.
        _auth.invalidate();
        return <String>[];
      }
      throw http.ClientException(
        'unexpected status ${resp.statusCode}',
        _endpoint,
      );
    } on TimeoutException {
      rethrow;
    } catch (e) {
      if (e is http.ClientException) rethrow;
      throw http.ClientException('transport error: $e', _endpoint);
    }
  }

  void close() => _client.close();
}
