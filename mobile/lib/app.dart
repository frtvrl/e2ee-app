// mobile/lib/app.dart
//
// Sprint 9.6.8 — `MyApp` root widget, wires Material 3 theme + go_router.
//
// The router itself lives in `lib/router/app_router.dart` so it can be
// tested in isolation (see `mobile/test/router_test.dart`).
//
// Theme: Material 3, light + dark, single seed color (opene2ee brand).
// The seed can be any 32-bit ARGB; we use a teal that matches the
// public-facing docs site.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'router/app_router.dart';
import 'theme/app_theme.dart';

class MyApp extends ConsumerWidget {
  const MyApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(appRouterProvider);
    return MaterialApp.router(
      title: 'OpenE2EE',
      debugShowCheckedModeBanner: false,
      theme: AppTheme.light(),
      darkTheme: AppTheme.dark(),
      themeMode: ThemeMode.system,
      routerConfig: router,
    );
  }
}
