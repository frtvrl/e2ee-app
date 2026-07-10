import 'dart:async';
import 'dart:math';

import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Sprint 10.0/10.1A — Aktif Nöbet (Active Pool) mock state.
///
/// Three stats are surfaced on the active pool screen (see
/// `sprint10-wireframes.html` frame 4):
///   - `paketSayisi`   : number of packets observed in the local sample
///   - `gonulluSayisi` : number of connected volunteers
///   - `testEdilenler` : which transports have been verified in this
///                       session (subset of `{rcs, whatsapp}`)
///
/// Sprint 10.1A adds real-time-feel mock updates so the screen
/// "feels alive" between releases. The numbers are still mock —
/// Sprint 10.1+ wires these to the local Go aggregator. The pool
/// toggle (`isAlici`) gates whether the periodic timer keeps
/// running; flipping it OFF pauses the mock ticker.
///
/// The history list `paketGecmisi` keeps the last 10 deltas (one
/// per 3-second tick) so the `fl_chart` mini-chart on the screen
/// can render a real-time line graph. `sonGuncelleme` powers the
/// "Son güncelleme: X sn önce" caption.
class PoolState {
  const PoolState({
    required this.isAlici,
    required this.paketSayisi,
    required this.gonulluSayisi,
    required this.testEdilenler,
    required this.paketGecmisi,
    required this.sonGuncelleme,
  });

  /// Whether the user is currently flagged as a receiver in the
  /// mock pool. Gated by the "Alıcı Ol" toggle on the screen.
  final bool isAlici;

  /// Cumulative number of packets observed in this session.
  final int paketSayisi;

  /// Number of mock volunteers currently in the pool (2-5 range).
  final int gonulluSayisi;

  /// Subset of `{rcs, whatsapp}` representing transports that
  /// have completed a smoke test in this session.
  final Set<String> testEdilenler;

  /// Last 10 per-tick packet deltas. Used by the `fl_chart`
  /// LineChart on the active pool screen. Oldest at index 0,
  /// newest at the last index.
  final List<int> paketGecmisi;

  /// Wall-clock time of the most recent tick — used to render
  /// the "Son güncelleme: X sn önce" caption.
  final DateTime? sonGuncelleme;

  PoolState copyWith({
    bool? isAlici,
    int? paketSayisi,
    int? gonulluSayisi,
    Set<String>? testEdilenler,
    List<int>? paketGecmisi,
    DateTime? sonGuncelleme,
  }) {
    return PoolState(
      isAlici: isAlici ?? this.isAlici,
      paketSayisi: paketSayisi ?? this.paketSayisi,
      gonulluSayisi: gonulluSayisi ?? this.gonulluSayisi,
      testEdilenler: testEdilenler ?? this.testEdilenler,
      paketGecmisi: paketGecmisi ?? this.paketGecmisi,
      sonGuncelleme: sonGuncelleme ?? this.sonGuncelleme,
    );
  }

  /// History capacity for the `paketGecmisi` ring buffer — used
  /// by both `PoolState.initial()` and `PoolNotifier._tick` to
  /// keep the same window size (10 data points = ~30 seconds at
  /// the 3-second tick interval).
  static const int tarihceKapasite = 10;

  /// Factory: initial mock state (Sprint 10.0 baseline +
  /// Sprint 10.1A history buffer).
  factory PoolState.initial() {
    return const PoolState(
      isAlici: true,
      paketSayisi: 247,
      gonulluSayisi: 3,
      testEdilenler: {'rcs', 'whatsapp'},
      paketGecmisi: <int>[1, 2, 1, 3, 2, 1, 2, 3, 1, 2],
      sonGuncelleme: null,
    );
  }
}

class PoolNotifier extends StateNotifier<PoolState> {
  PoolNotifier() : super(PoolState.initial()) {
    _start();
  }

  Timer? _timer;
  final Random _rng = Random();

  void _start() {
    _timer?.cancel();
    _timer = Timer.periodic(const Duration(seconds: 3), (_) => _tick());
  }

  /// Periodic update — runs every 3 seconds while the pool is
  /// `isAlici`. OFF state freezes the numbers; flipping back ON
  /// resumes from the current values (no jump).
  void _tick() {
    if (!state.isAlici) {
      return;
    }
    final delta = _rng.nextInt(3) + 1; // 1..3
    final yeniGonullu = 2 + _rng.nextInt(4); // 2..5
    final yeniTarihce = List<int>.from(state.paketGecmisi)..add(delta);
    while (yeniTarihce.length > PoolState.tarihceKapasite) {
      yeniTarihce.removeAt(0);
    }
    state = state.copyWith(
      paketSayisi: state.paketSayisi + delta,
      gonulluSayisi: yeniGonullu,
      paketGecmisi: yeniTarihce,
      sonGuncelleme: DateTime.now(),
    );
  }

  void toggleAlici() {
    final yeniAlici = !state.isAlici;
    state = state.copyWith(
      isAlici: yeniAlici,
      // Mark a "fresh" timestamp on every transition so the caption
      // resets to "şimdi" when the user re-enables.
      sonGuncelleme: yeniAlici ? DateTime.now() : state.sonGuncelleme,
    );
  }

  /// Sprint 10.1A — "test tamamlandı" callback. Lets the screen
  /// surface a new transport in `testEdilenler` when an async
  /// smoke-test completes (mock — wired in Sprint 10.1B).
  void raporTestTamamlandi(String transport) {
    if (state.testEdilenler.contains(transport)) {
      return;
    }
    state = state.copyWith(
      testEdilenler: {...state.testEdilenler, transport},
      sonGuncelleme: DateTime.now(),
    );
  }

  @override
  void dispose() {
    _timer?.cancel();
    _timer = null;
    super.dispose();
  }
}

final poolProvider = StateNotifierProvider<PoolNotifier, PoolState>(
  (ref) => PoolNotifier(),
);
