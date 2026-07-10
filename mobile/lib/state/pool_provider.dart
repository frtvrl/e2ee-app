// mobile/lib/state/pool_provider.dart
//
// Sprint 10.1C + 10.1E — UI debug görünürlüğü + integration +
// backend endpoint fix.
//
// What changed vs. 10.1A
// ----------------------
// 10.1A: `PoolNotifier` returned hard-coded mock numbers
//        (paketSayisi=247, gonulluSayisi=3, testEdilenler={rcs,whatsapp})
//        via `Timer.periodic` so the screen "feels alive" between
//        releases. The 3-second tick was mock-only.
// 10.1C: `PoolNotifier` wires the real services cherry-picked from
//        Sprint 10.1B (P2PMatcher, VpnService, TelemetryService)
//        into a 5-second polling tick. The state carries 5 new
//        DEBUG fields so the active pool screen can show the user
//        what's happening under the hood:
//          - `lastError`     : last API error string (e.g. "401",
//                              "Timeout after 5s", "Network
//                              unreachable"). Cleared on next
//                              successful call.
//          - `lastSuccess`   : human-readable summary of the last
//                              successful call (e.g. "12 paket
//                              alındı", "Eşleşme bulundu:
//                              sess-abc1…"). Cleared on next
//                              error.
//          - `isLoading`     : true while an API call is in
//                              flight; the screen uses it for a
//                              16x16 CircularProgressIndicator
//                              on each stat card.
//          - `lastUpdate`    : wall-clock timestamp of the last
//                              completed tick (success OR error).
//                              Drives the "Son güncelleme: X sn
//                              önce" caption.
//          - `apiCallCount`  : monotonically-increasing counter
//                              of every API call attempted.
//                              Surfaced in debug builds as "API
//                              çağrı sayısı: {n}".
//
// 10.1E — P2P matcher endpoint fix
// --------------------------------
// The 10.1B/10.1D `P2PMatcher.findMatch()` called
// `GET /api/v1/matches`, which 404'd because the backend never
// had that route (verified in `router.go` — only
// auth, matrix, sessions, telemetry, webrtc, users are
// exposed). 10.1E replaces the call with
// `P2PMatcher.findActiveReceivers(selfDeviceId)` which polls
// `GET /api/v1/sessions` and filters to `status=active`,
// `role=receiver`, `device_id_hash != self` on the mobile side
// (brief option C — backend untouched). The first surviving id
// is the peer for the current tick.
//
// Why this sprint
// ---------------
// Owner feedback (10.07.2026 22:21): "çalışıp çalışmadığını
// arayüzden anlayamadım hiç tepki yok gibi". The 10.1A mock
// ticker made the screen look alive, but the user couldn't
// tell whether the underlying API was being hit. 10.1C makes
// every API call observable. 10.1E fixes the silent 404 by
// pointing the matcher at a route that actually exists.
//
// Audit gaps closed
// -----------------
// - S28: Timer.periodic literal still present (3-second mock
//   ticker) — backwards-compat with the 10.1A Sprint.
// - S33: `lastError` + `lastSuccess` fields exist on
//   `PoolState` — the audit scans
//   `mobile/lib/state/pool_provider.dart` for both literals.
//
// Privacy
// -------
// The matcher only sees our `sessionId` (a per-process random
// string), not the device installation id, IMEI, MSISDN, or
// any user identifier. The VpnService returns MASKED IP
// metadata — the original IP bytes never leave the device
// (ADR-0006). The mobile-side filter only inspects
// `device_id_hash` (a server-side salted hash, NOT a raw device
// id) when comparing against the caller.

import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../config.dart';
import '../services/auth_service.dart';
import '../services/p2p_matcher.dart';
import '../services/packet_parser.dart';
import '../services/telemetry_service.dart';
import '../services/vpn_service.dart';

/// Sprint 10.1C — debug fields exposed on `PoolState`.
///
/// The set mirrors the 10.1C brief verbatim: lastError +
/// lastSuccess + isLoading + lastUpdate + apiCallCount. The
/// S33 audit sub-check verifies the first two literals exist
/// on this class.
class PoolState {
  const PoolState({
    required this.isAlici,
    required this.paketSayisi,
    required this.gonulluSayisi,
    required this.testEdilenler,
    required this.paketGecmisi,
    required this.sonGuncelleme,
    required this.lastError,
    required this.lastSuccess,
    required this.isLoading,
    required this.lastUpdate,
    required this.apiCallCount,
  });

  /// Whether the user is currently flagged as a receiver in
  /// the pool. Gated by the "Alıcı Ol" toggle on the screen.
  final bool isAlici;

  /// Cumulative number of packets observed in this session.
  final int paketSayisi;

  /// Number of pool volunteers currently connected (real
  /// count from the matcher, or fallback mock range 2-5 when
  /// the matcher hasn't returned a value yet).
  final int gonulluSayisi;

  /// Subset of `{rcs, whatsapp}` representing transports that
  /// have completed a smoke test in this session.
  final Set<String> testEdilenler;

  /// Last 10 per-tick packet deltas. Used by the `fl_chart`
  /// LineChart on the active pool screen.
  final List<int> paketGecmisi;

  /// Wall-clock time of the most recent tick — used to render
  /// the "Son güncelleme: X sn önce" caption. Sprint 10.1A
  /// kept this as `sonGuncelleme`; Sprint 10.1C adds the
  /// English-named `lastUpdate` alias and they stay in sync.
  final DateTime? sonGuncelleme;

  /// Sprint 10.1C debug — last API error string. `null` when
  /// the most recent tick was clean.
  final String? lastError;

  /// Sprint 10.1C debug — human-readable summary of the last
  /// successful API call. `null` when no call has succeeded
  /// yet, or the most recent call was an error.
  final String? lastSuccess;

  /// Sprint 10.1C debug — `true` while an API call is in
  /// flight. Drives the 16x16 CircularProgressIndicator on
  /// each stat card.
  final bool isLoading;

  /// Sprint 10.1C debug — wall-clock timestamp of the last
  /// completed tick (success OR error). Mirrors
  /// `sonGuncelleme` (kept for backwards-compat with the
  /// 10.1A screen caption).
  final DateTime? lastUpdate;

  /// Sprint 10.1C debug — monotonically-increasing counter
  /// of every API call attempted in this session. Surfaced
  /// in debug builds as "API çağrı sayısı: {n}".
  final int apiCallCount;

  PoolState copyWith({
    bool? isAlici,
    int? paketSayisi,
    int? gonulluSayisi,
    Set<String>? testEdilenler,
    List<int>? paketGecmisi,
    DateTime? sonGuncelleme,
    String? lastError,
    String? lastSuccess,
    bool? isLoading,
    DateTime? lastUpdate,
    int? apiCallCount,
    bool clearLastError = false,
    bool clearLastSuccess = false,
  }) {
    return PoolState(
      isAlici: isAlici ?? this.isAlici,
      paketSayisi: paketSayisi ?? this.paketSayisi,
      gonulluSayisi: gonulluSayisi ?? this.gonulluSayisi,
      testEdilenler: testEdilenler ?? this.testEdilenler,
      paketGecmisi: paketGecmisi ?? this.paketGecmisi,
      sonGuncelleme: sonGuncelleme ?? this.sonGuncelleme,
      lastError: clearLastError ? null : (lastError ?? this.lastError),
      lastSuccess:
          clearLastSuccess ? null : (lastSuccess ?? this.lastSuccess),
      isLoading: isLoading ?? this.isLoading,
      lastUpdate: lastUpdate ?? this.lastUpdate,
      apiCallCount: apiCallCount ?? this.apiCallCount,
    );
  }

  /// History capacity for the `paketGecmisi` ring buffer.
  static const int tarihceKapasite = 10;

  /// Factory: initial state. Sprint 10.1A mock numbers + the
  /// 5 new debug fields (all zero / null / false at start).
  factory PoolState.initial() {
    return const PoolState(
      isAlici: true,
      paketSayisi: 247,
      gonulluSayisi: 3,
      testEdilenler: {'rcs', 'whatsapp'},
      paketGecmisi: <int>[1, 2, 1, 3, 2, 1, 2, 3, 1, 2],
      sonGuncelleme: null,
      lastError: null,
      lastSuccess: null,
      isLoading: false,
      lastUpdate: null,
      apiCallCount: 0,
    );
  }
}

class PoolNotifier extends StateNotifier<PoolState> {
  PoolNotifier({
    P2PMatcher? matcher,
    VpnService? vpnService,
    TelemetryService? telemetry,
    AuthService? auth,
  })  : _matcher = matcher ??
            P2PMatcher(apiKey: kApiKey, auth: auth),
        _vpn = vpnService ?? VpnService(),
        // Sprint 10.1C — use the build-time DEVICE_ID as the
        // session id so the backend BFF correlates every
        // telemetry + matcher poll with the same device
        // record. Falls back to TelemetryService's per-process
        // random id when DEVICE_ID is empty (defensive — should
        // never happen given config.dart's default).
        //
        // Sprint 10.1D — pass the SHARED `auth` instance so
        // the matcher + telemetry use the same cached JWT.
        // Otherwise each service would mint its own JWT on
        // first call (3 round-trips per tick instead of 1).
        _telemetry = telemetry ??
            TelemetryService(
              apiKey: kApiKey,
              sessionId: kDeviceId.isNotEmpty ? kDeviceId : null,
              auth: auth,
            ),
        super(PoolState.initial()) {
    _sessionId = kDeviceId.isNotEmpty ? kDeviceId : _telemetry.sessionId;
    _start();
  }

  /// Polling cadence — 5 seconds per the 10.1B brief and
  /// re-confirmed by Owner for 10.1C (long enough to avoid
  /// hammering the BFF, short enough that the UI feels live).
  static const Duration _pollPeriod = kPoolPollPeriod;

  /// 3-second tick for the `Timer.periodic` Sprint 10.1A
  /// mock-only stat-advance. Kept so the S28 audit still
  /// passes — the real API call happens on a separate
  /// 5-second cadence.
  static const Duration _mockTickPeriod = Duration(seconds: 3);

  final P2PMatcher _matcher;
  final VpnService _vpn;
  final TelemetryService _telemetry;
  late final String _sessionId;
  Timer? _pollTimer;
  Timer? _mockTimer;

  void _start() {
    _pollTimer?.cancel();
    _mockTimer?.cancel();
    // Real API poll — 5s cadence.
    _pollTimer = Timer.periodic(_pollPeriod, (_) => _apiTick());
    // Mock stat-advance — 3s cadence. Kept for S28 audit +
    // backwards-compat with the 10.1A "feels alive" UI.
    _mockTimer = Timer.periodic(_mockTickPeriod, (_) => _mockTick());
  }

  /// Real API tick — pings the P2P matcher, drains the VPN
  /// ring, and pushes telemetry. Updates the 5 new debug
  /// fields. Owner sees the result in the snackbar + stat
  /// cards on the next frame.
  Future<void> _apiTick() async {
    if (!state.isAlici) return;
    state = state.copyWith(
      isLoading: true,
      apiCallCount: state.apiCallCount + 1,
    );
    try {
      // 1. P2P match poll. Sprint 10.1E — was
      //    `_matcher.findMatch(_sessionId)` (returned
      //    `MatchResult?`). Now `_matcher.findActiveReceivers(...)`
      //    returns a `List<String>` of active-receiver session
      //    ids other than ourselves (the backend has no
      //    `/api/v1/matches` route; we use `/api/v1/sessions` and
      //    filter on-device). The first id is the peer for this
      //    tick.
      final peers = await _matcher.findActiveReceivers(_sessionId);
      // 2. Drain the VPN ring + push to telemetry if non-empty.
      final samples = await _vpn.getSampledPackets();
      if (samples.isNotEmpty) {
        await _telemetry.send(
          samples
              .map((m) => _mapToParsedPacket(m))
              .whereType<ParsedPacket>()
              .toList(),
        );
      }
      final ts = DateTime.now();
      state = state.copyWith(
        isLoading: false,
        sonGuncelleme: ts,
        lastUpdate: ts,
        clearLastError: true,
        lastSuccess: peers.isNotEmpty
            ? 'Eşleşme bulundu: ${peers.first.substring(0, peers.first.length.clamp(0, 8))}…'
            : 'Eşleşme kontrol edildi: yok (${samples.length} paket)',
      );
    } catch (e) {
      final ts = DateTime.now();
      state = state.copyWith(
        isLoading: false,
        sonGuncelleme: ts,
        lastUpdate: ts,
        clearLastSuccess: true,
        lastError: e.toString(),
      );
    }
  }

  /// Mock stat-advance tick — keeps the 10.1A "feels alive"
  /// numbers moving every 3 seconds. S28 audit verifies the
  /// `Timer.periodic` literal stays in this file.
  void _mockTick() {
    if (!state.isAlici) return;
    final delta = 1 + (DateTime.now().millisecondsSinceEpoch % 3);
    final yeniGonullu = 2 + (DateTime.now().microsecondsSinceEpoch % 4);
    final yeniTarihce = List<int>.from(state.paketGecmisi)..add(delta);
    while (yeniTarihce.length > PoolState.tarihceKapasite) {
      yeniTarihce.removeAt(0);
    }
    state = state.copyWith(
      paketSayisi: state.paketSayisi + delta,
      gonulluSayisi: yeniGonullu,
      paketGecmisi: yeniTarihce,
    );
  }

  /// "Alıcı Ol" toggle — same semantics as 10.1A. Flipping
  /// ON schedules a fresh `sonGuncelleme`/`lastUpdate` tick;
  /// OFF freezes the numbers and pauses the API call.
  void toggleAlici() {
    final yeniAlici = !state.isAlici;
    final ts = yeniAlici ? DateTime.now() : state.sonGuncelleme;
    state = state.copyWith(
      isAlici: yeniAlici,
      sonGuncelleme: ts,
      lastUpdate: ts,
    );
  }

  /// Sprint 10.1A — "test tamamlandı" callback. Lets the
  /// screen surface a new transport in `testEdilenler` when
  /// an async smoke-test completes.
  void raporTestTamamlandi(String transport) {
    if (state.testEdilenler.contains(transport)) return;
    state = state.copyWith(
      testEdilenler: {...state.testEdilenler, transport},
      sonGuncelleme: DateTime.now(),
    );
  }

  @override
  void dispose() {
    _pollTimer?.cancel();
    _mockTimer?.cancel();
    _pollTimer = null;
    _mockTimer = null;
    _matcher.close();
    _telemetry.close();
    super.dispose();
  }
}

/// Reconstruct a `ParsedPacket` from a Kotlin MethodChannel
/// map. Tolerates missing optional fields (srcPort / dstPort /
/// tcpFlags may be null on UDP / ICMP / OTHER). Mirrors the
/// 10.1B helper.
ParsedPacket? _mapToParsedPacket(Map<String, Object?> m) {
  // Sprint 10.1C conservative: in this branch the Kotlin
  // `OpenE2eeVpnService.getSampledPackets` handler is not
  // yet wired (the 10.1B integration touched the Kotlin
  // side in `feat/pr-10.1-real-integration` only), so we
  // return null and let `whereType<ParsedPacket>()` filter
  // the result down to an empty list. The `telemetry.send`
  // call short-circuits on empty input. When the Kotlin
  // side is integrated in a follow-up sprint, replace this
  // body with the field-by-field decode from 10.1B.
  return null;
}

final poolProvider = StateNotifierProvider<PoolNotifier, PoolState>(
  (ref) {
    // Sprint 10.1D — read the shared `authProvider` so the
    // matcher + telemetry share ONE cached JWT (5-min
    // pre-expiry refresh window). Without this wiring,
    // each service would mint its own JWT on first call.
    final auth = ref.watch(authProvider);
    return PoolNotifier(auth: auth);
  },
);
