// mobile/lib/theme/app_theme.dart
//
// Sprint 9.6.8 — Material 3 theme (light + dark) for the OpenE2EE
// mobile app shell. Single seed color (opene2ee brand teal).
//
// Reference:
//   * Material 3 design system: https://m3.material.io/styles/color/
//   * Flutter `ColorScheme.fromSeed`:
//     https://api.flutter.dev/flutter/material/ColorScheme/fromSeed.html
//
// Why a single seed (not hand-picked primary/secondary)? M3 derives
// the entire palette (primary, secondary, tertiary, error, surface,
// etc.) from the seed, so we get a coherent, accessible color set
// for free. Brand color is teal (0xFF00897B) to match the public
// docs site.

import 'package:flutter/material.dart';

class AppTheme {
  AppTheme._();

  /// OpenE2EE brand teal — matches docs site header.
  static const Color _seed = Color(0xFF00897B);

  /// Light Material 3 theme.
  static ThemeData light() {
    return ThemeData(
      useMaterial3: true,
      colorScheme: ColorScheme.fromSeed(
        seedColor: _seed,
        brightness: Brightness.light,
      ),
    );
  }

  /// Dark Material 3 theme.
  static ThemeData dark() {
    return ThemeData(
      useMaterial3: true,
      colorScheme: ColorScheme.fromSeed(
        seedColor: _seed,
        brightness: Brightness.dark,
      ),
    );
  }
}
