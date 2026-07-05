// mobile/lib/mobile/screens/result_screen.dart
//
// PR-10: Mobile-only — Result screen (UI skeleton).
//
// Purpose (per HANDOFF §4.2 PR-10 + BRD §6.1)
// -------------------------------------------
// Surfaces the outcome of a single network diagnostic session. The
// native side flushes a `VpnTelemetry` payload once 10 packets have
// been observed (or the session is force-stopped); this screen is the
// landing page after the test ends. The actual scoring (entropy,
// TLS fingerprint, 0–100 composite) is computed Dart-side from the
// PR-4 `analysis` package and the backend's score endpoint.
//
// Privacy contract (ADR-0006)
// ---------------------------
// The screen displays metadata-only fields: packet count, protocol mix,
// TLS fingerprint summary, score. NEVER displays source IP, payload
// bytes, MSISDN, IMEI, phoneNumber, or any PII. Source IP shown to the
// user is already /24 (IPv4) or /48 (IPv6) masked at the native layer.
//
// References
// ----------
// - docs/ADR-0003-vpn-layer.md
// - docs/ADR-0006-anonimlik.md
// - docs/HANDOFF.md §4.2 PR-10

import 'dart:async';

import 'package:flutter/material.dart';

import '../vpn/method_channel.dart';

/// Result screen — displays a finished session's metadata summary.
///
/// Skeleton: empty Scaffold + AppBar + title + body placeholder that
/// subscribes to `VpnBridge.telemetry`. Phase 2 work (real scoring
/// visualization, "share to dashboard" CTA) is out of scope for Sprint 1.
class ResultScreen extends StatefulWidget {
  const ResultScreen({super.key, VpnBridge? bridge})
      : _bridgeOverride = bridge;

  /// Optional dependency-injection point for unit tests.
  final VpnBridge? _bridgeOverride;

  @override
  State<ResultScreen> createState() => _ResultScreenState();
}

class _ResultScreenState extends State<ResultScreen> {
  VpnTelemetry? _telemetry;
  StreamSubscription<VpnTelemetry>? _sub;

  VpnBridge get _bridge => widget._bridgeOverride ?? bridge;

  @override
  void initState() {
    super.initState();
    _sub = _bridge.telemetry.listen((event) {
      if (!mounted) return;
      setState(() => _telemetry = event);
    });
  }

  @override
  void dispose() {
    _sub?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final t = _telemetry;
    final count = t?.packets.length ?? 0;

    return Scaffold(
      appBar: AppBar(
        title: const Text('OpenE2EE — Session result'),
      ),
      body: Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: <Widget>[
            Text(
              t == null ? 'No telemetry yet.' : 'Packets captured: $count',
              key: const Key('result_screen.packet_count'),
            ),
            const SizedBox(height: 16),
            if (t != null) ...<Widget>[
              Text(
                'Captured at: ${t.capturedAt.toIso8601String()}',
                key: const Key('result_screen.captured_at'),
              ),
              const SizedBox(height: 8),
              Text(
                'Session: ${t.sessionId ?? '<pending>'}',
                key: const Key('result_screen.session_id'),
              ),
            ],
            const SizedBox(height: 24),
            const Padding(
              padding: EdgeInsets.symmetric(horizontal: 24),
              child: Text(
                'Phase 2 will render the entropy + TLS fingerprint + 0–100 '
                'composite score here.',
                textAlign: TextAlign.center,
              ),
            ),
          ],
        ),
      ),
    );
  }
}