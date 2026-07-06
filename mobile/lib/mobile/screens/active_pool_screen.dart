// mobile/lib/mobile/screens/active_pool_screen.dart
//
// PR-10 + PR-22b — Active pool screen (UI skeleton, calls real VPN bridge).
//
// Purpose (per HANDOFF §4.2 PR-10 + BRD §6.1)
// -------------------------------------------
// Lets the user opt in to being a *receiver* for incoming P2P test
// requests. Real matchmaking lives in the backend (PR-6 matching pool
// + WebSocket signalling); this screen just toggles "I am in the pool"
// state and surfaces a placeholder list of nearby sessions.
//
// PR-22b updates: handles `VpnPermissionDeniedError` from `start()`
// when the user has not yet granted VPN consent.
//
// Privacy contract (ADR-0006)
// ---------------------------
// The screen reads `VpnBridge.status()` only. No device identifiers,
// no contact access, no location. The "nearby sessions" list in Phase 2
// will show only the operator name + city (no PII, no MSISDN, no IMEI).
//
// References
// ----------
// - docs/ADR-0003-vpn-layer.md
// - docs/ADR-0004-p2p-echobot.md
// - docs/ADR-0006-anonimlik.md
// - docs/HANDOFF.md §4.2 PR-10

import 'package:flutter/material.dart';

import '../vpn/method_channel.dart';

/// Active pool screen — "be a receiver" toggle.
class ActivePoolScreen extends StatefulWidget {
  const ActivePoolScreen({super.key, VpnBridge? bridge})
      : _bridgeOverride = bridge;

  /// Optional dependency-injection point for unit tests.
  final VpnBridge? _bridgeOverride;

  @override
  State<ActivePoolScreen> createState() => _ActivePoolScreenState();
}

class _ActivePoolScreenState extends State<ActivePoolScreen> {
  bool _enrolled = false;
  VpnLifecycleState _state = VpnLifecycleState.idle;

  VpnBridge get _bridge => widget._bridgeOverride ?? bridge;

  Future<void> _toggleEnrolment(bool value) async {
    if (value) {
      // Joining the pool requires the native sampler to be running so the
      // user can act as a receiver for inbound test requests.
      try {
        final next = await _bridge.start();
        if (!mounted) return;
        setState(() {
          _enrolled = next.state == VpnLifecycleState.sampling;
          _state = next.state;
        });
      } on VpnPermissionDeniedError catch (_) {
        if (!mounted) return;
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('VPN permission is required to join the pool'),
          ),
        );
      }
    } else {
      final next = await _bridge.stop();
      if (!mounted) return;
      setState(() {
        _enrolled = false;
        _state = next.state;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('OpenE2EE — Active pool'),
      ),
      body: Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: <Widget>[
            Text('Bridge state: ${_state.name}'),
            const SizedBox(height: 16),
            SwitchListTile(
              key: const Key('active_pool_screen.enrol_switch'),
              title: const Text('Be a receiver'),
              subtitle: const Text(
                'Allow inbound test requests while the app is in the foreground.',
              ),
              value: _enrolled,
              onChanged: _toggleEnrolment,
            ),
            const SizedBox(height: 24),
            const Padding(
              padding: EdgeInsets.symmetric(horizontal: 24),
              child: Text(
                'Phase 2 will list nearby sessions here (operator name + '
                'city only — no PII).',
                textAlign: TextAlign.center,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
