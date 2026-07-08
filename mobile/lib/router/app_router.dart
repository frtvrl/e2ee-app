// mobile/lib/router/app_router.dart
//
// Sprint 9.6.8 — `go_router` 5-route table for the OpenE2EE mobile
// app shell.
//
// Route table (all paths start with `/`):
//
//   /              SplashScreen  — decides redirect target
//   /auth          AuthGateScreen — biometric prompt gate
//   /test          TestScreen — main test launch
//   /active-pool   ActivePoolScreen — sampling in progress
//   /result        ResultScreen — last completed run
//
// Why no `redirect:` callback? 9.6.8 simplification: navigation
// happens inside `SplashScreen.build` using `ref.listen` on the
// `deviceIdentityProvider` future, so the router is a pure route
// table. A future sprint can promote it to a `redirect:` callback
// once the routing logic stabilises.
//
// `appRouterProvider` is a `Provider<GoRouter>` so widget tests can
// override it with a stub router (see `mobile/test/main_test.dart`).

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../mobile/screens/active_pool_screen.dart';
import '../mobile/screens/auth_gate_screen.dart' show AuthGateScreen;
import '../mobile/screens/result_screen.dart';
import '../mobile/screens/splash_screen.dart' show SplashScreen;
import '../mobile/screens/test_screen.dart';

/// `go_router` 5-route table.
///
/// `appRouterProvider` is a `Provider<GoRouter>` so widget tests can
/// override it with a stub.
final appRouterProvider = Provider<GoRouter>((ref) {
  return GoRouter(
    initialLocation: '/',
    routes: <RouteBase>[
      GoRoute(
        path: '/',
        builder: (context, state) => const SplashScreen(),
      ),
      GoRoute(
        path: '/auth',
        builder: (context, state) => const AuthGateScreen(),
      ),
      GoRoute(
        path: '/test',
        builder: (context, state) => const TestScreen(),
      ),
      GoRoute(
        path: '/active-pool',
        builder: (context, state) => const ActivePoolScreen(),
      ),
      GoRoute(
        path: '/result',
        builder: (context, state) => const ResultScreen(),
      ),
    ],
  );
});
