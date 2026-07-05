// mobile/lib/mobile/screens/test_screen.dart
//
// PR-10: Mobile-only — Test launch screen (UI skeleton).
//
// Purpose (per HANDOFF §4.2 PR-10 + BRD §6.1)
// -------------------------------------------
// Lets the user kick off a fresh network diagnostic session. The actual
// consent gate (KVKK/GDPR) is a PR-12 follow-up; this screen just owns
// the "I want to start a test" button. The button hands off to the
// native VPN sampler via `VpnBridge.start()` and waits for the first
// telemetry callback before navigating to the result screen.
//
// Privacy contract (ADR-0006)
// ---------------------------
// No identifier collection on this screen. The only state is the
// `VpnLifecycleState` returned by the bridge. No IMEI, MSISDN,
// phoneNumber, contacts, or device fingerprint is read.
//
// References
// ----------
// - docs/ADR-0003-vpn-layer.md
// - docs/ADR-0006-anonimlik.md
// - docs/HANDOFF.md §4.2 PR-10

import 'package:flutter/material.dart';

import '../vpn/method_channel.dart';

/// Test launch screen — entry point for a network diagnostic session.
///
/// This is a skeleton: empty Scaffold + AppBar + title + body placeholder.
/// Phase 2 work (consent gate, task catalog: WhatsApp / RCS) is out of
/// scope for Sprint 1.
class TestScreen extends StatefulWidget {
  const TestScreen({super.key, VpnBridge? bridge})
      : _bridgeOverride = bridge;

  /// Optional dependency-injection point for unit tests. `null` in
  /// production — defaults to the [bridge] singleton.
  final VpnBridge? _bridgeOverride;

  @override
  State<TestScreen> createState() => _TestScreenState();
}

class _TestScreenState extends State<TestScreen> {
  VpnLifecycleState _state = VpnLifecycleState.idle;
  bool _busy = false;

  VpnBridge get _bridge => widget._bridgeOverride ?? bridge;

  Future<void> _startSession() async {
    if (_busy) return;
    setState(() => _busy = true);
    try {
      final next = await _bridge.start();
      if (!mounted) return;
      setState(() => _state = next);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _stopSession() async {
    if (_busy) return;
    setState(() => _busy = true);
    try {
      final next = await _bridge.stop();
      if (!mounted) return;
      setState(() => _state = next);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final isSampling = _state == VpnLifecycleState.sampling;
    final isUnavailable = _state == VpnLifecycleState.unavailable;

    return Scaffold(
      appBar: AppBar(
        title: const Text('OpenE2EE — Start a diagnostic session'),
      ),
      body: Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: <Widget>[
            Text(
              'Current state: ${_state.name}',
              key: const Key('test_screen.state'),
            ),
            const SizedBox(height: 16),
            if (isUnavailable)
              const Padding(
                padding: EdgeInsets.symmetric(horizontal: 24),
                child: Text(
                  'Native VPN sampler is not available on this platform. '
                  'Run on a physical Android or iOS device.',
                  textAlign: TextAlign.center,
                ),
              ),
            const SizedBox(height: 16),
            FilledButton(
              key: const Key('test_screen.start_button'),
              onPressed: (isUnavailable || _busy) ? null : _startSession,
              child: const Text('Start a test'),
            ),
            const SizedBox(height: 8),
            OutlinedButton(
              key: const Key('test_screen.stop_button'),
              onPressed: (!isSampling || _busy) ? null : _stopSession,
              child: const Text('Stop'),
            ),
          ],
        ),
      ),
    );
  }
}