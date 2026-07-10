import 'package:fl_chart/fl_chart.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../state/pool_provider.dart';
import '../theme/app_theme.dart';
import '../widgets/stat_pill.dart';

/// Sprint 10.0 + 10.1A — Aktif Nöbet (Active Pool) screen.
///
/// Orange hero ("Aktif Nöbet Modu") + "Alıcı Ol" toggle card +
/// 3-stat grid (İzlenen Paket / Bağlı Gönüllü / Test Edilenler)
/// + Sprint 10.1A additions: pulse "AKTİF" pill when the toggle
/// is on, a real-time `fl_chart` mini-chart of the last 10 packet
/// deltas, and a "Eşleşme bulundu!" SnackBar + HapticFeedback 5
/// seconds after the user opts in. The toggle gates the periodic
/// mock ticker in [PoolNotifier]; OFF freezes the numbers.
///
/// S25 invariant: no "v-p-n" framing in the UI. See
/// `sprint10-wireframes.html` frame 4.
class ActivePoolScreen extends ConsumerStatefulWidget {
  const ActivePoolScreen({super.key});

  @override
  ConsumerState<ActivePoolScreen> createState() => _ActivePoolScreenState();
}

class _ActivePoolScreenState extends ConsumerState<ActivePoolScreen>
    with SingleTickerProviderStateMixin {
  late final AnimationController _pulseController;
  bool _eslesmeZamanlayiciAktif = false;

  @override
  void initState() {
    super.initState();
    _pulseController = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1200),
    )..repeat(reverse: true);
  }

  @override
  void dispose() {
    _pulseController.dispose();
    super.dispose();
  }

  void _onAliciOlToggle() {
    final eskiAlici = ref.read(poolProvider).isAlici;
    ref.read(poolProvider.notifier).toggleAlici();
    final yeniAlici = !eskiAlici;
    if (yeniAlici && !_eslesmeZamanlayiciAktif) {
      _eslesmeZamanlayiciAktif = true;
      // 5 sn sonra "Eşleşme bulundu!" — Sprint 10.1A eşleşme feedback.
      // The callback re-checks the provider because the user may
      // have toggled back off in the meantime.
      Future.delayed(const Duration(seconds: 5), () {
        if (!mounted) {
          return;
        }
        _eslesmeZamanlayiciAktif = false;
        final halaAlici = ref.read(poolProvider).isAlici;
        if (!halaAlici) {
          return;
        }
        HapticFeedback.lightImpact();
        ScaffoldMessenger.of(context)
          ..hideCurrentSnackBar()
          ..showSnackBar(
            SnackBar(
              content: const Text('Eşleşme bulundu! Test başlıyor...'),
              backgroundColor: AppTheme.primary,
              duration: const Duration(seconds: 3),
              behavior: SnackBarBehavior.floating,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(12),
              ),
            ),
          );
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final pool = ref.watch(poolProvider);
    return Scaffold(
      backgroundColor: AppTheme.bg,
      appBar: AppBar(
        title: const Text('Aktif Nöbet'),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back),
          onPressed: () => context.go('/home/gorevler'),
        ),
      ),
      body: ListView(
        padding: const EdgeInsets.only(bottom: 96),
        children: [
          // Orange hero with pulse "AKTİF" pill.
          _PoolHero(
            pulseController: _pulseController,
            alici: pool.isAlici,
          ),
          // Toggle card.
          Padding(
            padding: const EdgeInsets.all(16),
            child: Container(
              decoration: BoxDecoration(
                color: AppTheme.surface,
                border: Border.all(color: AppTheme.border),
                borderRadius: BorderRadius.circular(20),
              ),
              padding: const EdgeInsets.symmetric(
                horizontal: 16,
                vertical: 20,
              ),
              child: Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: const [
                        Text(
                          'Alıcı Ol',
                          style: TextStyle(
                            fontSize: 14,
                            fontWeight: FontWeight.w600,
                            color: AppTheme.text,
                          ),
                        ),
                        SizedBox(height: 2),
                        Text(
                          'Havuzda 15 dk bekle',
                          style: TextStyle(
                            fontSize: 12,
                            color: AppTheme.muted,
                          ),
                        ),
                      ],
                    ),
                  ),
                  Switch(
                    value: pool.isAlici,
                    onChanged: (_) => _onAliciOlToggle(),
                    activeThumbColor: Colors.white,
                    activeTrackColor: AppTheme.primary,
                  ),
                ],
              ),
            ),
          ),
          // 3-stat grid (2 + 1 full-width).
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Column(
              children: [
                Row(
                  children: [
                    Expanded(
                      child: _StatCard(
                        label: 'İzlenen Paket',
                        value: pool.paketSayisi.toString(),
                      ),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: _StatCard(
                        label: 'Bağlı Gönüllü',
                        value: pool.gonulluSayisi.toString(),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 12),
                _TestEdilenlerCard(
                  available: pool.testEdilenler,
                ),
                const SizedBox(height: 12),
                // Sprint 10.1A: real-time packet delta chart.
                _PaketChartCard(
                  tarihce: pool.paketGecmisi,
                  alici: pool.isAlici,
                ),
                const SizedBox(height: 8),
                _SonGuncellemeCaption(son: pool.sonGuncelleme),
              ],
            ),
          ),
        ],
      ),
      bottomNavigationBar: const _PoolBottomNav(),
    );
  }
}

class _PoolHero extends StatelessWidget {
  const _PoolHero({
    required this.pulseController,
    required this.alici,
  });

  final AnimationController pulseController;
  final bool alici;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.fromLTRB(16, 24, 16, 24),
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [AppTheme.accent, AppTheme.accentDark],
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Text(
                'Aktif Nöbet Modu',
                style: TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
              ),
              const SizedBox(width: 10),
              if (alici)
                _PulseAktifPill(controller: pulseController),
            ],
          ),
          const SizedBox(height: 4),
          const Text(
            'Gönüllü olarak test havuzunda bekle',
            style: TextStyle(
              fontSize: 13,
              color: Colors.white,
            ),
          ),
        ],
      ),
    );
  }
}

class _PulseAktifPill extends StatelessWidget {
  const _PulseAktifPill({required this.controller});

  final AnimationController controller;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        final scale = 1.0 + (controller.value * 0.18);
        final opacity = 0.55 + (controller.value * 0.45);
        return Transform.scale(
          scale: scale,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
            decoration: BoxDecoration(
              color: Colors.white.withValues(alpha: opacity),
              borderRadius: BorderRadius.circular(20),
            ),
            child: const Text(
              'AKTİF',
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w700,
                color: AppTheme.accentDark,
                letterSpacing: 0.6,
              ),
            ),
          ),
        );
      },
    );
  }
}

class _StatCard extends StatelessWidget {
  const _StatCard({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: AppTheme.surface,
        border: Border.all(color: AppTheme.border),
        borderRadius: BorderRadius.circular(16),
      ),
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label.toUpperCase(),
            style: const TextStyle(
              fontSize: 11,
              color: AppTheme.muted,
              letterSpacing: 0.8,
              fontWeight: FontWeight.w500,
            ),
          ),
          const SizedBox(height: 8),
          Text(
            value,
            style: const TextStyle(
              fontSize: 28,
              fontWeight: FontWeight.w700,
              color: AppTheme.primary,
              fontFamily: 'monospace',
              height: 1,
            ),
          ),
        ],
      ),
    );
  }
}

class _TestEdilenlerCard extends StatelessWidget {
  const _TestEdilenlerCard({required this.available});

  final Set<String> available;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      decoration: BoxDecoration(
        color: AppTheme.surface,
        border: Border.all(color: AppTheme.border),
        borderRadius: BorderRadius.circular(16),
      ),
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'TEST EDİLENLER',
            style: TextStyle(
              fontSize: 11,
              color: AppTheme.muted,
              letterSpacing: 0.8,
              fontWeight: FontWeight.w500,
            ),
          ),
          const SizedBox(height: 10),
          Wrap(
            spacing: 8,
            runSpacing: 8,
            children: [
              if (available.contains('rcs'))
                const StatPill(
                  label: 'RCS',
                  colorKey: StatPillColor.accent,
                ),
              if (available.contains('whatsapp'))
                const StatPill(
                  label: 'WhatsApp',
                  colorKey: StatPillColor.whatsapp,
                ),
            ],
          ),
        ],
      ),
    );
  }
}

/// Sprint 10.1A: fl_chart mini-chart of the last 10 per-tick
/// packet deltas. Hidden behind a subtle "Sprint 10.1A — canlı
/// paket akışı" caption so the wireframe origin is traceable.
class _PaketChartCard extends StatelessWidget {
  const _PaketChartCard({required this.tarihce, required this.alici});

  final List<int> tarihce;
  final bool alici;

  @override
  Widget build(BuildContext context) {
    final spots = <FlSpot>[];
    for (var i = 0; i < tarihce.length; i++) {
      spots.add(FlSpot(i.toDouble(), tarihce[i].toDouble()));
    }
    return Container(
      width: double.infinity,
      decoration: BoxDecoration(
        color: AppTheme.surface,
        border: Border.all(color: AppTheme.border),
        borderRadius: BorderRadius.circular(16),
      ),
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              const Text(
                'SON 30 SN PAKET AKIŞI',
                style: TextStyle(
                  fontSize: 11,
                  color: AppTheme.muted,
                  letterSpacing: 0.8,
                  fontWeight: FontWeight.w500,
                ),
              ),
              Text(
                alici ? 'canlı' : 'duraklatıldı',
                style: TextStyle(
                  fontSize: 11,
                  color: alici ? AppTheme.primary : AppTheme.muted,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),
          SizedBox(
            height: 120,
            child: spots.isEmpty
                ? const Center(
                    child: Text(
                      'Veri bekleniyor…',
                      style: TextStyle(
                        fontSize: 12,
                        color: AppTheme.muted,
                      ),
                    ),
                  )
                : LineChart(
                    LineChartData(
                      lineBarsData: [
                        LineChartBarData(
                          spots: spots,
                          isCurved: true,
                          color: AppTheme.primary,
                          barWidth: 2,
                          isStrokeCapRound: true,
                          dotData: const FlDotData(show: false),
                          belowBarData: BarAreaData(
                            show: true,
                            color: AppTheme.primary.withValues(alpha: 0.10),
                          ),
                        ),
                      ],
                      minY: 0,
                      maxY: 4,
                      titlesData: const FlTitlesData(show: false),
                      gridData: const FlGridData(show: false),
                      borderData: FlBorderData(show: false),
                      lineTouchData: const LineTouchData(enabled: false),
                    ),
                  ),
          ),
        ],
      ),
    );
  }
}

class _SonGuncellemeCaption extends StatelessWidget {
  const _SonGuncellemeCaption({required this.son});

  final DateTime? son;

  @override
  Widget build(BuildContext context) {
    final text = son == null
        ? 'Henüz güncelleme yok'
        : 'Son güncelleme: ${_formatRelative(son!)}';
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 4),
      child: Text(
        text,
        style: const TextStyle(
          fontSize: 11,
          color: AppTheme.muted,
        ),
      ),
    );
  }

  String _formatRelative(DateTime ts) {
    final fark = DateTime.now().difference(ts);
    if (fark.inSeconds < 5) {
      return 'şimdi';
    }
    if (fark.inMinutes < 1) {
      return '${fark.inSeconds} sn önce';
    }
    if (fark.inHours < 1) {
      return '${fark.inMinutes} dk önce';
    }
    return '${fark.inHours} sa önce';
  }
}

class _PoolBottomNav extends StatelessWidget {
  const _PoolBottomNav();

  @override
  Widget build(BuildContext context) {
    return BottomNavigationBar(
      currentIndex: 1,
      onTap: (i) {
        switch (i) {
          case 0:
            context.go('/home/gorevler');
            break;
          case 1:
            // already here
            break;
          case 2:
            context.go('/home/skorlar');
            break;
        }
      },
      items: const [
        BottomNavigationBarItem(
          icon: Icon(Icons.task_alt_outlined),
          activeIcon: Icon(Icons.task_alt),
          label: 'Görevler',
        ),
        BottomNavigationBarItem(
          icon: Icon(Icons.people_outline),
          activeIcon: Icon(Icons.people),
          label: 'Aktif Nöbet',
        ),
        BottomNavigationBarItem(
          icon: Icon(Icons.bar_chart_outlined),
          activeIcon: Icon(Icons.bar_chart),
          label: 'Skorlar',
        ),
      ],
    );
  }
}
