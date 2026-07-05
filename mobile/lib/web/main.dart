// lib/web/main.dart
//
// PR-11: Web dashboard entry point.
//
// What this file owns
// --------------------
// * The single `void main()` for the Flutter *web* target of the OpenE2EE
//   mobile app. Built with `flutter build web --target=lib/web/main.dart`
//   (per HANDOFF §4.2 PR-11 / §8 DoD). The default
//   `flutter build web` (which targets `lib/main.dart`) intentionally points
//   at the *mobile* entrypoint, so the dashboard has its own rooted target
//   file to keep the responsibility split clean.
// * Wires the top-level `MaterialApp` to the operator/country matrix as
//   the initial route. Theme is intentionally Material 3 light; the
//   dashboard is internal analytics for an operator (read-only), not a
//   marketing-style landing page.
//
// Privacy contract (ADR-0006)
// ---------------------------
// * This entrypoint never touches device identifiers, the VPN bridge, or
//   `lib/shared/api_client.dart` (which is mobile-only). The dashboard is a
//   read-only view over already-aggregated telemetry that the backend has
//   already anonymised — no IMEI / MSISDN / phoneNumber / MAC / raw packet
//   bytes ever reach `lib/web/`.
// * The dashboard has no `print`/`debugPrint` statements. We render
//   *placeholder* data in Sprint 1; a future PR (PR-8 wire-up) will plug a
//   REST read-side in via `ApiClient` once the backend stabilises.
//
// References
// ----------
// - docs/ADR-0006-anonimlik.md
// - docs/HANDOFF.md §4.2 PR-11 / §8 DoD
// - docs/BRD.md §6 transparency dashboard

import 'package:flutter/material.dart';

import 'screens/matrix_screen.dart';

/// Web dashboard entry point.
///
/// Run with:
/// ```
/// flutter build web --target=lib/web/main.dart
/// ```
/// (and `flutter run -d chrome --target=lib/web/main.dart` for hot-reload
/// during development).
void main() {
  runApp(
    const MaterialApp(
      title: 'OpenE2EE Dashboard',
      home: MatrixScreen(),
    ),
  );
}
