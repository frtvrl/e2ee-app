// mobile/test/main_test.dart
//
// Sprint 9.6.8 — smoke test for the app shell boot.
//
// Pins the minimum-viable behavior: the app boots, the splash
// screen renders with the progress indicator, and the test
// key (`Key('splash_screen.progress')`) is findable on the first
// frame.
//
// We do NOT need to override `deviceIdentityProvider` for the
// first-frame check — the splash renders the `CircularProgressIndicator`
// synchronously and the future is only `ref.read` after the first
// frame. If the test later needs to verify navigation behaviour,
// the override pattern below can be uncommented.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:opene2ee/app.dart';

void main() {
  testWidgets('app boots and shows splash progress', (tester) async {
    await tester.pumpWidget(
      const ProviderScope(
        child: MyApp(),
      ),
    );
    // First frame: SplashScreen with progress indicator.
    // The future behind `deviceIdentityProvider` is only read
    // post-frame (see SplashScreen.initState) so the first pump
    // shows the CircularProgressIndicator without waiting for
    // the Keystore round-trip.
    expect(find.byKey(const Key('splash_screen.progress')), findsOneWidget);
  });
}
