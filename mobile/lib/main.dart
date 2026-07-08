// mobile/lib/main.dart
//
// Sprint 9.6.8 — Flutter mobile app entry point.
//
// Wires:
//   * `ProviderScope` (flutter_riverpod) for state management.
//   * `MyApp` with `MaterialApp.router` (go_router).
//   * Initial route: `/` → SplashScreen (decides where to go).
//
// Privacy: never logs the UUID v7, the Ed25519 private key, or the
// server salt. See ADR-0006 §"Anonim Cihaz Kimliği".

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'app.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  // Lock to portrait per AndroidManifest config (Sprint 5 MOB-5 — phone-
  // posture only). iOS Info.plist already enforces this.
  await SystemChrome.setPreferredOrientations(<DeviceOrientation>[
    DeviceOrientation.portraitUp,
  ]);
  runApp(const ProviderScope(child: MyApp()));
}
