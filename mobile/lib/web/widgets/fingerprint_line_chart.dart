// lib/web/widgets/fingerprint_line_chart.dart
//
// PR-11: Web dashboard — fl_chart LineChart widget (median-score trend).
//
// Purpose (per HANDOFF §4.2 PR-11)
// --------------------------------
// Renders one line per country showing the *median* transparency score
// across the operators in that country. This is the *second of
// at-least-one* fl_chart integration called out in PR-11: a LineChart
// alongside the OperatorBarChart so the dashboard has both BarChart and
// LineChart widgets live.
//
// Privacy contract (ADR-0006)
// ---------------------------
// * Operates only on the backend-derived aggregate cells. The y-values
//   are integer medians in `[0..100]`; the x-axis is the country label.
//   No packet bytes, IPs, or per-device identifiers are plotted.
// * No print / debugPrint / log calls — this widget must be silent in
//   the release web bundle.
//
// References
// ----------
// - docs/HANDOFF.md §4.2 PR-11 (fl_chart integration)
// - fl_chart 0.69.x API (LineChartData.lineBarsData / spots / titles).

import 'package:fl_chart/fl_chart.dart';
import 'package:flutter/material.dart';

import '../screens/matrix_screen.dart';

/// Line chart: one line per country = median score across operators.
///
/// "Median" (not mean) so a single outlier country doesn't pull the line
/// in misleading ways — operators are unevenly distributed per country in
/// the placeholder data set, so median is a robust aggregate.
class FingerprintLineChart extends StatelessWidget {
  const FingerprintLineChart({
    super.key,
    required this.cells,
    this.height = 220,
  });

  final List<MatrixCell> cells;
  final double height;

  /// Compute per-country median scores.
  ///
  /// Returns a map in the iteration order of [MatrixCountry] so the
  /// chart's x-axis labels are deterministic for golden tests.
  Map<MatrixCountry, List<int>> _groupByCountry() {
    final out = <MatrixCountry, List<int>>{};
    for (final c in cells) {
      out.putIfAbsent(c.country, () => <int>[]).add(c.score);
    }
    return out;
  }

  int _median(List<int> xs) {
    if (xs.isEmpty) return 0;
    final sorted = [...xs]..sort();
    final n = sorted.length;
    if (n.isOdd) return sorted[n ~/ 2];
    return ((sorted[n ~/ 2 - 1] + sorted[n ~/ 2]) / 2).round();
  }

  @override
  Widget build(BuildContext context) {
    final grouped = _groupByCountry();
    if (grouped.isEmpty) {
      return SizedBox(
        height: height,
        child: const Center(child: Text('No country data yet.')),
      );
    }

    final countries = MatrixCountry.values
        .where(grouped.containsKey)
        .toList(growable: false);

    // Stable per-country colour from a small fixed palette.
    const palette = <Color>[
      Color(0xFF1E88E5), // blue
      Color(0xFFE53935), // red
      Color(0xFF43A047), // green
      Color(0xFFFB8C00), // orange
      Color(0xFF8E24AA), // purple
    ];

    return SizedBox(
      height: height,
      child: LineChart(
        LineChartData(
          minY: 0,
          maxY: 100,
          minX: 0,
          maxX: (countries.length - 1).toDouble().clamp(1, 1000),
          gridData: FlGridData(
            show: true,
            drawVerticalLine: false,
            horizontalInterval: 25,
            getDrawingHorizontalLine: (value) => FlLine(
              color: Theme.of(context).dividerColor.withValues(alpha: 0.4),
              strokeWidth: 1,
            ),
          ),
          borderData: FlBorderData(show: false),
          titlesData: FlTitlesData(
            topTitles: const AxisTitles(
                sideTitles: SideTitles(showTitles: false)),
            rightTitles: const AxisTitles(
                sideTitles: SideTitles(showTitles: false)),
            leftTitles: const AxisTitles(
              sideTitles: SideTitles(
                showTitles: true,
                interval: 25,
                reservedSize: 32,
              ),
            ),
            bottomTitles: AxisTitles(
              sideTitles: SideTitles(
                showTitles: true,
                reservedSize: 32,
                interval: 1,
                getTitlesWidget: (value, meta) {
                  final idx = value.toInt();
                  if (idx < 0 || idx >= countries.length) {
                    return const SizedBox.shrink();
                  }
                  return Padding(
                    padding: const EdgeInsets.only(top: 6),
                    child: Text(
                      countries[idx].code,
                      style: const TextStyle(fontSize: 11),
                    ),
                  );
                },
              ),
            ),
          ),
          lineBarsData: [
            for (var i = 0; i < countries.length; i++)
              LineChartBarData(
                spots: [
                  // Two-point line: median as the country's marker;
                  // a small jitter on the trailing point so the line
                  // is visually present even when only one sample.
                  FlSpot(
                      i.toDouble(),
                      _median(grouped[countries[i]]!).toDouble()),
                  FlSpot(
                      i.toDouble(),
                      _median(grouped[countries[i]]!).toDouble()),
                ],
                color: palette[i % palette.length],
                isCurved: false,
                barWidth: 2.5,
                dotData: FlDotData(
                  show: true,
                  getDotPainter: (spot, percent, barData, index) {
                    return FlDotCirclePainter(
                      radius: 4,
                      color: palette[i % palette.length],
                      strokeWidth: 2,
                      strokeColor: Colors.white,
                    );
                  },
                ),
                belowBarData: BarAreaData(
                  show: true,
                  color:
                      palette[i % palette.length].withValues(alpha: 0.10),
                ),
              ),
          ],
        ),
      ),
    );
  }
}
