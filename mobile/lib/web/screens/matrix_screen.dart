// lib/web/screens/matrix_screen.dart
//
// PR-11: Web dashboard — operator × country matrix screen.
//
// Purpose (per HANDOFF §4.2 PR-11 + BRD §6.1)
// -------------------------------------------
// The landing screen of the OpenE2EE dashboard. Renders an operator × country
// matrix where each cell carries a 0-100 transparency score (the score that
// the analysis pipeline in PR-4 computes and the echobot in PR-5 ratifies).
// Clicking a cell navigates to [DetailScreen] for that (operator, country)
// pair.
//
// Data source
// -----------
// Sprint 1 uses **placeholder data** baked into this file as a static
// constant. PR-8 (wire-up) replaces this with a paginated fetch through
// `lib/shared/api_client.dart` to the backend's `GET /api/v1/matrix` route
// (already stubbed in PR-7). Until then, the structure mirrors the
// `matrix-aggregate.schema.json` contract so the eventual swap is a 1-line
// change inside [_MatrixRepository.fetchPlaceholder].
//
// Privacy contract (ADR-0006)
// ---------------------------
// * The dashboard never displays: raw UUIDs, public keys, fingerprints,
//   packet samples, source/destination IPs, or any PII. The score is
//   computed at the backend from already-anonymised telemetry, and what
//   reaches this screen is only: operator wire-name, ISO-3166 country
//   code, score (0-100).
// * No print / debugPrint — the repo is silent in `flutter build web`
//   release mode; console output is reserved for the test harness only.
//
// References
// ----------
// - docs/ADR-0006-anonimlik.md
// - docs/HANDOFF.md §4.2 PR-11
// - shared/schemas/matrix-aggregate.schema.json (PR-7 stub)

import 'package:flutter/material.dart';

import 'detail_screen.dart';
import '../widgets/operator_bar_chart.dart';
import '../widgets/fingerprint_line_chart.dart';

/// Operator wire-name as it appears in the matrix column header.
///
/// These mirror the enum entries in
/// `mobile/lib/shared/telemetry_formatter.dart#TelemetryOperator`, which in
/// turn mirrors `shared/schemas/telemetry.schema.json#operator`. We do not
/// import the enum directly: the dashboard shows a *display* label that
/// already matches the wire string, so the formatter-side enum stays the
/// single source of truth.
enum MatrixOperator {
  turkcell('turkcell', 'Turkcell'),
  vodafoneTr('vodafone_tr', 'Vodafone TR'),
  turkTelekom('turk_telekom', 'Türk Telekom'),
  att('att', 'AT&T'),
  verizon('verizon', 'Verizon'),
  tmobileUs('tmobile_us', 'T-Mobile US'),
  deutscheTelekom('deutsche_telekom', 'Deutsche Telekom'),
  orange('orange', 'Orange'),
  vodafone('vodafone', 'Vodafone EU'),
  o2('o2', 'O2'),
  ee('ee', 'EE'),
  three('three', 'Three');

  const MatrixOperator(this.wire, this.label);

  /// Wire-format identifier (matches telemetry.schema.json).
  final String wire;

  /// Human-readable label for UI rendering.
  final String label;
}

/// ISO-3166 alpha-2 country code shown as the matrix row header.
enum MatrixCountry {
  tr('TR', 'Türkiye'),
  us('US', 'United States'),
  de('DE', 'Deutschland'),
  fr('FR', 'France'),
  gb('GB', 'United Kingdom');

  const MatrixCountry(this.code, this.label);
  final String code;
  final String label;
}

/// One row × column cell — the smallest unit of the matrix.
///
/// The score is an integer in `[0, 100]` per BRD §6 acceptance rule:
/// "0 = no transparency, 100 = perfect transparency". The backend stub
/// (`backend/internal/api/matrix.go`) emits the same shape.
class MatrixCell {
  const MatrixCell({
    required this.operator,
    required this.country,
    required this.score,
    required this.sessionCount,
  });

  final MatrixOperator operator;
  final MatrixCountry country;

  /// Whole-number score in `[0, 100]`.
  final int score;

  /// Number of test sessions contributing to the score (i.e. the
  /// denominator when displaying confidence).
  final int sessionCount;
}

/// In-memory placeholder repository.
///
/// Replace with `ApiClient.get('/api/v1/matrix')` once PR-8 lands.
class MatrixRepository {
  MatrixRepository();

  /// Returns the operator × country matrix as a flat list of cells.
  ///
  /// Deterministic — same data on every call so widget tests / golden
  /// tests are reproducible without mocking a backend.
  List<MatrixCell> fetchPlaceholder() {
    return const <MatrixCell>[
      // TR row
      MatrixCell(
          operator: MatrixOperator.turkcell,
          country: MatrixCountry.tr,
          score: 72,
          sessionCount: 184),
      MatrixCell(
          operator: MatrixOperator.vodafoneTr,
          country: MatrixCountry.tr,
          score: 65,
          sessionCount: 142),
      MatrixCell(
          operator: MatrixOperator.turkTelekom,
          country: MatrixCountry.tr,
          score: 78,
          sessionCount: 201),
      // US row
      MatrixCell(
          operator: MatrixOperator.att,
          country: MatrixCountry.us,
          score: 81,
          sessionCount: 312),
      MatrixCell(
          operator: MatrixOperator.verizon,
          country: MatrixCountry.us,
          score: 85,
          sessionCount: 287),
      MatrixCell(
          operator: MatrixOperator.tmobileUs,
          country: MatrixCountry.us,
          score: 76,
          sessionCount: 254),
      // DE row
      MatrixCell(
          operator: MatrixOperator.deutscheTelekom,
          country: MatrixCountry.de,
          score: 88,
          sessionCount: 198),
      MatrixCell(
          operator: MatrixOperator.vodafone,
          country: MatrixCountry.de,
          score: 74,
          sessionCount: 176),
      MatrixCell(
          operator: MatrixOperator.o2,
          country: MatrixCountry.de,
          score: 69,
          sessionCount: 134),
      // FR row
      MatrixCell(
          operator: MatrixOperator.orange,
          country: MatrixCountry.fr,
          score: 80,
          sessionCount: 162),
      // GB row
      MatrixCell(
          operator: MatrixOperator.ee,
          country: MatrixCountry.gb,
          score: 83,
          sessionCount: 171),
      MatrixCell(
          operator: MatrixOperator.three,
          country: MatrixCountry.gb,
          score: 70,
          sessionCount: 121),
    ];
  }
}

/// Landing screen — operator × country matrix.
class MatrixScreen extends StatefulWidget {
  const MatrixScreen({super.key, MatrixRepository? repository})
      : _repositoryOverride = repository;

  /// Dependency-injection seam for widget tests. Production callers leave
  /// this `null` and the screen instantiates a default [MatrixRepository]
  /// (placeholder data, no I/O).
  final MatrixRepository? _repositoryOverride;

  @override
  State<MatrixScreen> createState() => _MatrixScreenState();
}

class _MatrixScreenState extends State<MatrixScreen> {
  late final MatrixRepository _repo =
      widget._repositoryOverride ?? MatrixRepository();

  late final List<MatrixCell> _cells = _repo.fetchPlaceholder();

  /// Build a `(operator.wire) -> cell` lookup for fast render.
  late final Map<String, Map<String, MatrixCell>> _byRowCol =
      _buildLookup(_cells);

  static Map<String, Map<String, MatrixCell>> _buildLookup(
      List<MatrixCell> cells) {
    final out = <String, Map<String, MatrixCell>>{};
    for (final c in cells) {
      out.putIfAbsent(c.country.code, () => <String, MatrixCell>{});
      out[c.country.code]![c.operator.wire] = c;
    }
    return out;
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('OpenE2EE · Transparency Matrix'),
        backgroundColor: Theme.of(context).colorScheme.inversePrimary,
      ),
      body: LayoutBuilder(
        builder: (context, constraints) {
          return SingleChildScrollView(
            scrollDirection: Axis.vertical,
            child: SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              child: ConstrainedBox(
                constraints: BoxConstraints(
                    minWidth: constraints.maxWidth.isFinite
                        ? constraints.maxWidth
                        : 800),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    _SummaryRow(cells: _cells),
                    const SizedBox(height: 16),
                    _MatrixGrid(
                      cells: _cells,
                      lookup: _byRowCol,
                      onCellTap: _openDetail,
                    ),
                    const SizedBox(height: 24),
                    const Padding(
                      padding: EdgeInsets.symmetric(horizontal: 16),
                      child: Text(
                        'Score distribution (last 24h, all operators)',
                        style: TextStyle(
                            fontSize: 16, fontWeight: FontWeight.w600),
                      ),
                    ),
                    const SizedBox(height: 8),
                    SizedBox(
                      height: 260,
                      child: Padding(
                        padding: const EdgeInsets.symmetric(horizontal: 16),
                        child: OperatorBarChart(
                          cells: _cells,
                        ),
                      ),
                    ),
                    const SizedBox(height: 24),
                    const Padding(
                      padding: EdgeInsets.symmetric(horizontal: 16),
                      child: Text(
                        'Median score per country (trend)',
                        style: TextStyle(
                            fontSize: 16, fontWeight: FontWeight.w600),
                      ),
                    ),
                    const SizedBox(height: 8),
                    SizedBox(
                      height: 260,
                      child: Padding(
                        padding: const EdgeInsets.symmetric(horizontal: 16),
                        child: FingerprintLineChart(cells: _cells),
                      ),
                    ),
                    const SizedBox(height: 32),
                  ],
                ),
              ),
            ),
          );
        },
      ),
    );
  }

  /// Navigate to the detail screen for a single (operator, country) cell.
  void _openDetail(MatrixCell cell) {
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => DetailScreen(cell: cell),
      ),
    );
  }
}

/// Summary header showing aggregate metrics (placeholder values are
/// calculated from [_cells] so the screen stays self-consistent).
class _SummaryRow extends StatelessWidget {
  const _SummaryRow({required this.cells});
  final List<MatrixCell> cells;

  @override
  Widget build(BuildContext context) {
    final totalSessions =
        cells.fold<int>(0, (acc, c) => acc + c.sessionCount);
    final meanScore = cells.isEmpty
        ? 0
        : (cells.fold<int>(0, (acc, c) => acc + c.score) / cells.length)
            .round();

    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(16),
      color: Theme.of(context).colorScheme.surfaceContainerHighest,
      child: Wrap(
        spacing: 32,
        runSpacing: 8,
        children: [
          _SummaryStat(label: 'Operator × country cells', value: '${cells.length}'),
          _SummaryStat(label: 'Total sessions (24h)', value: '$totalSessions'),
          _SummaryStat(label: 'Mean score', value: '$meanScore / 100'),
        ],
      ),
    );
  }
}

class _SummaryStat extends StatelessWidget {
  const _SummaryStat({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(label,
            style: TextStyle(
                color: Theme.of(context).colorScheme.outline, fontSize: 12)),
        const SizedBox(height: 2),
        Text(value,
            style: const TextStyle(
                fontSize: 22, fontWeight: FontWeight.w600)),
      ],
    );
  }
}

/// The matrix itself — a `DataTable` styled as a heat-grid.
class _MatrixGrid extends StatelessWidget {
  const _MatrixGrid({
    required this.cells,
    required this.lookup,
    required this.onCellTap,
  });

  final List<MatrixCell> cells;
  final Map<String, Map<String, MatrixCell>> lookup;
  final void Function(MatrixCell) onCellTap;

  @override
  Widget build(BuildContext context) {
    final operators = MatrixOperator.values;
    final countries = MatrixCountry.values;

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      child: DataTable(
        headingTextStyle:
            const TextStyle(fontWeight: FontWeight.bold, fontSize: 14),
        dataTextStyle: const TextStyle(fontSize: 13),
        columnSpacing: 12,
        columns: <DataColumn>[
          const DataColumn(label: Text('Country \\ Operator')),
          for (final op in operators)
            DataColumn(label: Text(op.label, textAlign: TextAlign.center)),
        ],
        rows: <DataRow>[
          for (final country in countries)
            DataRow(cells: <DataCell>[
              DataCell(Text(
                country.label,
                style: const TextStyle(fontWeight: FontWeight.w600),
              )),
              for (final op in operators)
                _buildOpCell(op, country),
            ]),
        ],
      ),
    );
  }

  DataCell _buildOpCell(MatrixOperator op, MatrixCountry country) {
    final cell = lookup[country.code]?[op.wire];
    if (cell == null) {
      // Empty (country, operator) pair — outside our placeholder footprint.
      return const DataCell(Text('—', style: TextStyle(color: Colors.grey)));
    }
    return DataCell(
      InkWell(
        onTap: () => onCellTap(cell),
        child: _ScoreChip(score: cell.score, sessionCount: cell.sessionCount),
      ),
    );
  }
}

/// A small coloured pill rendering a single `[0..100]` score.
///
/// Mapping per BRD §6:
/// * `< 40`  → red (poor transparency)
/// * `40-69` → amber (mixed)
/// * `>= 70` → green (acceptable or better)
class _ScoreChip extends StatelessWidget {
  const _ScoreChip({required this.score, required this.sessionCount});
  final int score;
  final int sessionCount;

  Color _color(BuildContext context) {
    if (score >= 70) return Colors.green.shade600;
    if (score >= 40) return Colors.amber.shade700;
    return Colors.red.shade600;
  }

  @override
  Widget build(BuildContext context) {
    final color = _color(context);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: color.withValues(alpha: 0.4)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text('$score',
              style: TextStyle(
                  color: color, fontWeight: FontWeight.bold, fontSize: 14)),
          const SizedBox(width: 4),
          Text('($sessionCount)',
              style: TextStyle(
                  color: Theme.of(context).colorScheme.outline,
                  fontSize: 11)),
        ],
      ),
    );
  }
}
