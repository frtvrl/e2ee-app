// lib/web/widgets/operator_bar_chart.dart
//
// PR-11: Web dashboard — fl_chart BarChart widget (operator distribution).
//
// Purpose (per HANDOFF §4.2 PR-11)
// --------------------------------
// Renders one bar per operator showing the operator's mean transparency
// score (mean of the per-country cells supplied). This is the *first of
// at-least-one* fl_chart integration called out in PR-11 — the matrix view
// shows scores as text chips for fast scanning, and this chart gives the
// pattern-recognition view.
//
// Privacy contract (ADR-0006)
// ---------------------------
// * Consumes only [MatrixCell] aggregates that are already derived on the
//   backend. We never plot: raw UUIDs, packet sizes, packet payloads, or
//   per-user fingerprints. The `score` integer and the `sessionCount` are
//   the only fields used in the chart's `BarChartData`.
// * No print / debugPrint / log calls.
//
// References
// ----------
// - docs/HANDOFF.md §4.2 PR-11 (fl_chart integration)
// - fl_chart 0.69.x API (BarChartData.barGroups, axis titles, bottomTitles)

import 'package:fl_chart/fl_chart.dart';
import 'package:flutter/material.dart';

import '../screens/matrix_screen.dart';

/// Bar chart: one bar per operator = mean score across countries.
class OperatorBarChart extends StatelessWidget {
  const OperatorBarChart({
    super.key,
    required this.cells,
    this.height = 220,
  });

  /// Aggregates supplied from [MatrixScreen]. Must be non-empty; the
  /// caller ([MatrixScreen]) guarantees this by rendering the chart only
  /// after the placeholder repository returns its deterministic row set.
  final List<MatrixCell> cells;

  /// Chart height in logical pixels. Width is supplied by the parent.
  final double height;

  /// Compute per-operator mean score from the cells.
  ///
  /// Returns a list ordered by [MatrixOperator] enum order so the chart
  /// renders in a predictable left-to-right sequence for golden tests.
  List<_OperatorBar> _aggregate() {
    final byOp = <MatrixOperator, List<int>>{};
    for (final c in cells) {
      byOp.putIfAbsent(c.operator, () => <int>[]).add(c.score);
    }
    final out = <_OperatorBar>[];
    for (final op in MatrixOperator.values) {
      final scores = byOp[op];
      if (scores == null || scores.isEmpty) continue;
      final mean = scores.reduce((a, b) => a + b) / scores.length;
      out.add(_OperatorBar(op, mean));
    }
    return out;
  }

  @override
  Widget build(BuildContext context) {
    final bars = _aggregate();
    if (bars.isEmpty) {
      return SizedBox(
        height: height,
        child: const Center(child: Text('No operator data yet.')),
      );
    }

    return SizedBox(
      height: height,
      child: BarChart(
        BarChartData(
          maxY: 100,
          minY: 0,
          alignment: BarChartAlignment.spaceAround,
          borderData: FlBorderData(show: false),
          gridData: FlGridData(
            show: true,
            drawVerticalLine: false,
            horizontalInterval: 25,
            getDrawingHorizontalLine: (value) => FlLine(
              color: Theme.of(context).dividerColor.withValues(alpha: 0.4),
              strokeWidth: 1,
            ),
          ),
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
                reservedSize: 36,
                getTitlesWidget: (value, meta) {
                  final idx = value.toInt();
                  if (idx < 0 || idx >= bars.length) {
                    return const SizedBox.shrink();
                  }
                  return Padding(
                    padding: const EdgeInsets.only(top: 6),
                    child: Text(
                      bars[idx].operator.label,
                      style: const TextStyle(fontSize: 10),
                      overflow: TextOverflow.ellipsis,
                    ),
                  );
                },
              ),
            ),
          ),
          barGroups: [
            for (var i = 0; i < bars.length; i++)
              BarChartGroupData(
                x: i,
                barRods: [
                  BarChartRodData(
                    toY: bars[i].mean,
                    width: 18,
                    color: _barColor(bars[i].mean),
                    borderRadius: const BorderRadius.vertical(
                        top: Radius.circular(4)),
                  ),
                ],
              ),
          ],
        ),
      ),
    );
  }

  /// Colour the bar according to the same thresholds as
  /// [_ScoreChip] in `matrix_screen.dart`. Pulled out so the visual
  /// language is consistent between the matrix chip and the chart bar.
  static Color _barColor(double mean) {
    if (mean >= 70) return Colors.green.shade600;
    if (mean >= 40) return Colors.amber.shade700;
    return Colors.red.shade600;
  }
}

class _OperatorBar {
  const _OperatorBar(this.operator, this.mean);
  final MatrixOperator operator;
  final double mean;
}
