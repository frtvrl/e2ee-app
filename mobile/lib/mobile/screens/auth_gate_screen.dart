// mobile/lib/mobile/screens/auth_gate_screen.dart
//
// Sprint 9.6.8 — Biometric auth gate. Wraps
// `BiometricAuthenticator.authenticate(...)` in a Riverpod-driven
// flow that gates the sensitive screens (/test, /active-pool,
// /result) behind a FaceID / TouchID / fingerprint prompt.
//
// Privacy contract (mirrors `lib/mobile/auth/biometric.dart`):
//
//   * No PII in the prompt text. The reason string is a hand-
//     written constant, scrubbed of any identifier (UUID hex,
//     fingerprint, e-mail, MSISDN) per ADR-0006.
//   * Fail-closed on `BiometricUnavailableError`. We surface a
//     `SnackBar` with the cause + a Retry button — we do NOT
//     silently fall back to a non-biometric path. The test
//     `biometric_test.dart::wrappersFailClosedWhenHardwareMissing`
//     pins this contract.
//   * `biometricOnly: true` (in `kHardenedAuthOptions`) prevents
//     the OS from offering the system passcode / PIN / pattern as
//     an alternative auth factor.
//
// Test pin: `Key('auth_gate.prompt_button')` on the prompt button
// so widget tests can `find.byKey` it without depending on text or
// color.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../auth/biometric.dart';
import '../../state/providers.dart';

class AuthGateScreen extends ConsumerStatefulWidget {
  const AuthGateScreen({super.key});

  @override
  ConsumerState<AuthGateScreen> createState() => _AuthGateScreenState();
}

class _AuthGateScreenState extends ConsumerState<AuthGateScreen> {
  bool _busy = false;

  Future<void> _prompt() async {
    if (_busy) return;
    setState(() => _busy = true);
    final auth = ref.read(biometricAuthProvider);
    try {
      final ok = await auth.authenticate(
        localizedReason: kBiometricPromptReason,
      );
      if (!mounted) return;
      if (ok) {
        context.go('/test');
      } else {
        _showSnack('Authentication denied');
      }
    } on BiometricUnavailableError catch (e) {
      if (!mounted) return;
      _showSnack('Biometric unavailable: ${e.cause.name}'
          '${e.detail == null ? '' : ' (${e.detail})'}');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  void _showSnack(String message) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(message),
        action: SnackBarAction(
          label: 'Retry',
          onPressed: _prompt,
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Authenticate')),
      body: Center(
        child: FilledButton(
          key: const Key('auth_gate.prompt_button'),
          onPressed: _busy ? null : _prompt,
          child: const Text('Authenticate to continue'),
        ),
      ),
    );
  }
}
