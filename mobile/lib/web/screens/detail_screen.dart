// lib/web/screens/detail_screen.dart
//
// PR-11: Web dashboard — detail screen for a single (operator, country) cell.
//
// Purpose (per HANDOFF §4.2 PR-11 + BRD §6.1)
// -------------------------------------------
// Reached by tapping any cell in [MatrixScreen]. Shows the breakdown that
// the aggregated score summarises: the score itself, the contributing
// session count, the operator / country pair in human-readable form, and
// a placeholder "per-protocol" table stub. PR-8 wire-up will replace the
// placeholder breakdown with per-protocol (TLS 1.2 / 1.3 / QUIC) split
// fetched from `GET /api/v1/matrix/{operator}/{country}/detail`.
//
// Privacy contract (ADR-0006)
// ---------------------------
// * The detail screen never reveals: raw UUIDs, Ed25519 public keys,
//   packet bytes, source IPs, or session identifiers. Per-session
//   identifiers are *hashed* on the backend per ADR-0006 — this screen
//   only shows aggregate counts.
// * No print / debugPrint / log calls.
//
// References
// ----------
// - docs/HANDOFF.md §4.2 PR-11
// - docs/ADR-0006-anonimlik.md

import 'package:flutter/material.dart';

import 'matrix_screen.dart';

/// Detail screen — explains one cell of the operator/country matrix.
class DetailScreen extends StatelessWidget {
  const DetailScreen({super.key, required this.cell});

  /// The (operator, country) cell the user drilled into.
  final MatrixCell cell;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return Scaffold(
      appBar: AppBar(
        title: Text(
            '${cell.operator.label} · ${cell.country.label}'),
        backgroundColor: theme.colorScheme.inversePrimary,
      ),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _ScoreHero(score: cell.score, sessionCount: cell.sessionCount),
            const SizedBox(height: 24),
            _BreakdownTable(cell: cell),
            const SizedBox(height: 24),
            _PrivacyNotice(),
          ],
        ),
      ),
    );
  }
}

/// Hero block — the score rendered large, the contributing session count
/// underneath.
class _ScoreHero extends StatelessWidget {
  const _ScoreHero({required this.score, required this.sessionCount});
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
      width: double.infinity,
      padding: const EdgeInsets.all(24),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.10),
        border: Border.all(color: color.withValues(alpha: 0.4)),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Overall transparency score',
            style: TextStyle(
                fontSize: 14,
                color: Theme.of(context).colorScheme.outline),
          ),
          const SizedBox(height: 4),
          Row(
            crossAxisAlignment: CrossAxisAlignment.baseline,
            textBaseline: TextBaseline.alphabetic,
            children: [
              Text('$score',
                  style: TextStyle(
                    color: color,
                    fontSize: 56,
                    fontWeight: FontWeight.bold,
                  )),
              const SizedBox(width: 6),
              const Text('/ 100', style: TextStyle(fontSize: 18)),
              const Spacer(),
              Column(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  const Text('Sessions (24h)',
                      style: TextStyle(fontSize: 12)),
                  Text('$sessionCount',
                      style: const TextStyle(
                          fontSize: 20, fontWeight: FontWeight.w600)),
                ],
              ),
            ],
          ),
          const SizedBox(height: 8),
          const Text(
            'Score range: 0 = no transparency, 100 = perfect transparency '
            '(BRD §6 acceptance rule).',
            style: TextStyle(fontSize: 12),
          ),
        ],
      ),
    );
  }
}

/// Placeholder per-protocol breakdown table. PR-8 will populate the
/// `ScoreComponent` rows with live data fetched from
/// `GET /api/v1/matrix/{operator}/{country}/detail`.
class _BreakdownTable extends StatelessWidget {
  const _BreakdownTable({required this.cell});
  final MatrixCell cell;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text('Score components (placeholder)',
            style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
        const SizedBox(height: 8),
        DataTable(
          headingTextStyle:
              const TextStyle(fontWeight: FontWeight.bold, fontSize: 13),
          dataTextStyle: const TextStyle(fontSize: 13),
          columns: const <DataColumn>[
            DataColumn(label: Text('Component')),
            DataColumn(label: Text('Weight')),
            DataColumn(label: Text('Raw')),
            DataColumn(label: Text('Weighted')),
          ],
          rows: <DataRow>[
            const DataRow(cells: <DataCell>[
              DataCell(Text('TLS handshake visibility')),
              DataCell(Text('40%')),
              DataCell(Text('—')),
              DataCell(Text('—')),
            ]),
            const DataRow(cells: <DataCell>[
              DataCell(Text('Entropy distribution')),
              DataCell(Text('30%')),
              DataCell(Text('—')),
              DataCell(Text('—')),
            ]),
            const DataRow(cells: <DataCell>[
              DataCell(Text('ALPN / protocol mix')),
              DataCell(Text('20%')),
              DataCell(Text('—')),
              DataCell(Text('—')),
            ]),
            const DataRow(cells: <DataCell>[
              DataCell(Text('Echobot coherence')),
              DataCell(Text('10%')),
              DataCell(Text('—')),
              DataCell(Text('—')),
            ]),
            DataRow(cells: <DataCell>[
              DataCell(Text('Aggregate',
                  style: TextStyle(
                      fontWeight: FontWeight.bold,
                      color: Theme.of(context).colorScheme.primary))),
              DataCell(Text('')),
              DataCell(Text('')),
              DataCell(Text('${cell.score} / 100',
                  style: TextStyle(
                      fontWeight: FontWeight.bold,
                      color: Theme.of(context).colorScheme.primary))),
            ]),
          ],
        ),
        const SizedBox(height: 8),
        Text(
          'Operator wire-id: ${cell.operator.wire} · '
          'Country: ${cell.country.code}',
          style: TextStyle(
              fontSize: 11,
              color: Theme.of(context).colorScheme.outline),
        ),
      ],
    );
  }
}

/// Plain-language note that this screen shows aggregate scores only.
///
/// Per ADR-0006, the dashboard must NEVER collect raw packet data, IPs,
/// or device identifiers from the browser. The score is computed from
/// already-anonymised telemetry received from the OpenE2EE mobile app.
class _PrivacyNotice extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: Theme.of(context).colorScheme.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(8),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Icon(Icons.shield_outlined,
              color: Theme.of(context).colorScheme.primary, size: 20),
          const SizedBox(width: 10),
          const Expanded(
            child: Text(
              'This dashboard only displays aggregated, anonymised scores. '
              'No device identifiers, source IPs, or packet payloads are '
              'ever rendered in the browser. See ADR-0006.',
              style: TextStyle(fontSize: 12),
            ),
          ),
        ],
      ),
    );
  }
}
