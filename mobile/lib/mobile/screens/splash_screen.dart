// mobile/lib/mobile/screens/splash_screen.dart
//
// Sprint 9.6.8 — Splash / decision screen. Boots the app, resolves
// `deviceIdentityProvider`, and navigates to the next route.
//
// Navigation logic (deliberately kept inside the screen, not in
// `app_router.dart`'s `redirect:` callback, per the 9.6.8 brief):
//
//   * `deviceIdentityProvider` resolves successfully → `/test`
//     (the user's identity exists; jump straight to the action).
//   * `deviceIdentityProvider` throws (e.g. biometric unavailable,
//     Keystore corruption) → `/auth` (let the user recover via the
//     biometric gate).
//   * While `deviceIdentityProvider` is loading → stay on `/` and
//     show a centered `CircularProgressIndicator`.
//
// Riverpod constraint: `ref.listen` can only be called inside
// `build()` of a `ConsumerWidget` / `ConsumerStatefulWidget` — it
// throws at runtime if invoked from `initState` or any other
// callback. We use `ref.listen` in `build()` and trigger the
// initial `ref.read` from `initState` so the future starts
// resolving on mount.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../state/providers.dart';

class SplashScreen extends ConsumerStatefulWidget {
  const SplashScreen({super.key});

  @override
  ConsumerState<SplashScreen> createState() => _SplashScreenState();
}

class _SplashScreenState extends ConsumerState<SplashScreen> {
  @override
  void initState() {
    super.initState();
    // Trigger the future (ref.listen does not eagerly resolve).
    // ignore: unawaited_futures
    ref.read(deviceIdentityProvider.future);
  }

  @override
  Widget build(BuildContext context) {
    // Listen for state changes (Riverpod constraint: only inside build).
    ref.listen<AsyncValue<Object?>>(
      deviceIdentityProvider,
      (previous, next) {
        next.whenOrNull(
          data: (_) {
            if (!mounted) return;
            context.go('/test');
          },
          error: (_, __) {
            if (!mounted) return;
            context.go('/auth');
          },
        );
      },
    );
    return Scaffold(
      body: Center(
        child: CircularProgressIndicator(
          key: const Key('splash_screen.progress'),
        ),
      ),
    );
  }
}
