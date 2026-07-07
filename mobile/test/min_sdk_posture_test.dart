// test/min_sdk_posture_test.dart
//
// Sprint 7 §MOB-5 — Android minSdk posture regression guard.
//
// Reads `mobile/android/app/build.gradle.kts` and asserts that the
// `defaultConfig.minSdk` value is **>= 23** (Android 6.0 Marshmallow).
//
// Why: `flutter_secure_storage 9.x` (pinned in pubspec.yaml at ^9.2.4)
// requires API 23+ for hardware-backed AndroidKeyStore AES master-key
// generation. Pre-API-23 either throws KeyStoreException or silently
// falls back to software-only SharedPreferences, breaking the Ed25519
// private-key-at-rest guarantee from docs/ADR-0006-anonimlik.md §B1.
//
// This is a posture test — pure file-parsing, no device required.
// Runs as part of `flutter test` so a regression that lowers minSdk
// below the required floor is caught in CI before a release build.
//
// If the floor ever needs to move, update `kMinRequiredAndroidSdk`
// below AND document the rationale in mobile/README.md §1.2.

import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

/// The minimum acceptable Android API level.
///
/// Tracked separately from the `mobile/android/app/build.gradle.kts`
/// value so a regression that lowers the floor is caught immediately.
const int kMinRequiredAndroidSdk = 23;

/// Absolute or repo-relative path to the Gradle build script.
///
/// `flutter test` runs with the package's working directory at the
/// `mobile/` directory, so the relative path is stable.
const String kBuildGradleRelativePath = 'android/app/build.gradle.kts';

void main() {
  group('Sprint 7 §MOB-5 — Android minSdk posture', () {
    late File buildGradle;
    late String buildGradleContent;

    setUpAll(() {
      // Try a few candidate paths so the test works whether it is run
      // from `mobile/` (flutter test) or from the repo root (dart test).
      final candidates = <String>[
        kBuildGradleRelativePath,
        'mobile/$kBuildGradleRelativePath',
        '../$kBuildGradleRelativePath',
      ];

      File? resolved;
      for (final candidate in candidates) {
        final f = File(candidate);
        if (f.existsSync()) {
          resolved = f;
          break;
        }
      }

      if (resolved == null) {
        throw StateError(
          'Could not locate $kBuildGradleRelativePath from '
          '${Directory.current.path}. Tried: $candidates',
        );
      }
      buildGradle = resolved;
      buildGradleContent = buildGradle.readAsStringSync();
    });

    test('build.gradle.kts exists and is non-empty', () {
      expect(buildGradle.existsSync(), isTrue,
          reason: 'mobile/android/app/build.gradle.kts must exist');
      expect(buildGradleContent.length, greaterThan(100),
          reason: 'build.gradle.kts looks suspiciously short');
    });

    test('declares minSdk = $kMinRequiredAndroidSdk (Android 6.0+)', () {
      // The Kotlin DSL syntax is `minSdk = 23` on its own line within
      // the `defaultConfig { ... }` block. We match the literal line.
      // This is intentionally strict — a future engineer who writes
      // `minSdk = 22` (e.g. to support one more legacy device) will
      // see this test fail and have to update the constant deliberately.
      final pattern = RegExp(
        r'^\s*minSdk\s*=\s*(\d+)\s*$',
        multiLine: true,
      );
      final matches = pattern.allMatches(buildGradleContent).toList();
      expect(matches, isNotEmpty,
          reason:
              'No `minSdk = N` line found in build.gradle.kts. '
              'Sprint 7 §MOB-5 requires minSdk = $kMinRequiredAndroidSdk.');
      expect(matches.length, 1,
          reason:
              'Expected exactly one `minSdk = N` line. Found ${matches.length}. '
              'Move the duplicate or split into a comment.');
      final value = int.parse(matches.first.group(1)!);
      expect(value, greaterThanOrEqualTo(kMinRequiredAndroidSdk),
          reason:
              'minSdk = $value is below the MOB-5-required floor '
              '$kMinRequiredAndroidSdk. Bumping minSdk is the MOB-5 fix; '
              'lowering it would silently break flutter_secure_storage '
              '9.x AndroidKeyStore AES master-key generation on the '
              'affected devices.');
    });

    test('rationale comment block mentions MOB-5 + AndroidKeyStore', () {
      // The rationale comment block at the top of build.gradle.kts is
      // a key part of the MOB-5 record. A future engineer who removes
      // or rewrites it without understanding the contract should see
      // this test fail.
      expect(
        buildGradleContent.contains('MOB-5'),
        isTrue,
        reason:
            'build.gradle.kts must reference "MOB-5" so the MOB-5 '
            'rationale is preserved alongside the floor value. '
            'See mobile/README.md §1.2 for the full rationale.',
      );
      expect(
        buildGradleContent.contains('AndroidKeyStore'),
        isTrue,
        reason:
            'build.gradle.kts rationale block must mention '
            'AndroidKeyStore (the source of the API 23 floor).',
      );
      expect(
        buildGradleContent.contains('flutter_secure_storage'),
        isTrue,
        reason:
            'build.gradle.kts rationale block must mention '
            'flutter_secure_storage (the consumer plugin).',
      );
    });

    test('defaultConfig block is in the expected position', () {
      // Defensive: if someone restructures the file (e.g. moves the
      // minSdk line out of `defaultConfig`), the lint-level guard
      // above still passes but the real AGP build might misbehave.
      // This test asserts the structural invariant.
      //
      // We match `defaultConfig {` (with the opening brace) so that
      // free-form mentions of "defaultConfig" inside rationale
      // comments (e.g. "see the defaultConfig block below") do not
      // fool the offset check. Only the actual Kotlin DSL block
      // opener has the `{` immediately after.
      final blockOpenerPattern = RegExp(r'defaultConfig\s*\{');
      final blockOpenerMatches =
          blockOpenerPattern.allMatches(buildGradleContent).toList();
      expect(blockOpenerMatches, isNotEmpty,
          reason:
              'build.gradle.kts must contain a `defaultConfig {` block '
              'opener. AGP requires this block to declare the module\'s '
              'default configuration values (minSdk, targetSdk, etc.).');
      final minSdkMatches = RegExp(r'^\s*minSdk\s*=\s*\d+\s*$',
              multiLine: true)
          .allMatches(buildGradleContent)
          .toList();
      expect(minSdkMatches, isNotEmpty,
          reason: 'No `minSdk = N` line found in build.gradle.kts.');
      // The minSdk declaration must appear AFTER the `defaultConfig {`
      // block opener — otherwise AGP would parse it in an unexpected
      // context.
      final defaultConfigOffset = blockOpenerMatches.first.start;
      final minSdkOffset = minSdkMatches.first.start;
      expect(minSdkOffset, greaterThan(defaultConfigOffset),
          reason:
              'minSdk declaration (offset $minSdkOffset) must appear '
              'inside the defaultConfig block (block opener at '
              '$defaultConfigOffset). If minSdk is declared before '
              'defaultConfig, AGP may parse it in the wrong scope.');
    });

    test('targetSdk is set to 34 (Android 14) for foregroundServiceType',
        () {
      // Independent posture pin — `targetSdk = 34` is required for
      // `foregroundServiceType="specialUse"` on the VPN service
      // (see AndroidManifest.xml and OpenE2eeVpnService.kt
      //  startForegroundCompat).
      // If this fails, AGP's `MissingForegroundServiceTypeException`
      // will surface at runtime when the VPN service starts on API 34+.
      final pattern = RegExp(r'^\s*targetSdk\s*=\s*(\d+)\s*$', multiLine: true);
      final matches = pattern.allMatches(buildGradleContent).toList();
      expect(matches, isNotEmpty,
          reason: 'No `targetSdk = N` line found in build.gradle.kts');
      expect(matches.length, 1);
      final value = int.parse(matches.first.group(1)!);
      expect(value, greaterThanOrEqualTo(34),
          reason:
              'targetSdk = $value is below 34 (Android 14). The VPN '
              'service requires API 34+ for foregroundServiceType='
              '"specialUse" (see AndroidManifest.xml and '
              'OpenE2eeVpnService.kt startForegroundCompat).');
    });
  });
}