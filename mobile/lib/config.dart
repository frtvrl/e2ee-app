// mobile/lib/config.dart
//
// Sprint 10.1C / 10.1D — build-time injectable config surface.
//
// Why this file exists
// --------------------
// Sprint 10.1B shipped the per-service `<device_api_key_placeholder>`
// literal as an in-source placeholder. Sprint 10.1C promoted the
// device identity and the API key to a single config surface so a
// real build can inject both via `--dart-define`:
//
//   flutter build apk --debug \
//     --dart-define DEVICE_ID=a1b2c3d4e5f60718a1b2c3d4 \
//     --dart-define API_KEY=<real-key> \
//     --dart-define API_BASE=https://api-test.opene2ee.com
//
// Sprint 10.1D adds `apiBase` + `apiVersion` for the JWT auth
// flow (`POST /api/v1/auth`, `/api/v1/telemetry`, `/api/v1/matches`).
//
// The defaults keep an out-of-the-box `flutter build apk --debug`
// working without any extra flags; production / tablet-test
// builds must override DEVICE_ID + API_KEY + API_BASE so the
// backend BFF routes the telemetry + matcher polls to the right
// device record on the right environment.
//
// Sprint 10.1C Owner directive (10.07.2026 22:25)
// -----------------------------------------------
// "test device ID available - a1b2c3d4e5f60718a1b2c3d4 (mock
// device in backend DB). Inject as --dart-define
// DEVICE_ID=a1b2c3d4e5f60718a1b2c3d4 during build."
//
// Sprint 10.1D Owner directive (10.07.2026 22:33)
// -----------------------------------------------
// "POST /api/v1/auth with body user_id=DEVICE_ID returns Bearer
// JWT (HS256, 1h TTL, 3600s). Then protected endpoints
// /api/v1/telemetry and /api/v1/matches need Authorization
// Bearer jwt."
//
// Privacy
// -------
// DEVICE_ID is a backend-side correlation key — it identifies a
// device record in the BFF aggregator's device table so multiple
// telemetry uploads from the same physical device stitch together.
// It is NOT a user identifier (no IMEI / MSISDN / phone number).
// The audit's privacy-grep tool excludes the DEVICE_ID default
// (mock value) from its scan, the same way it excludes the
// `<device_api_key_placeholder>` literal in telemetry_service.dart.

import 'package:flutter/foundation.dart';

/// Sprint 10.1D — `AppConfig` class consolidates all build-time
/// injectable constants. The `kDeviceId` / `kApiKey` aliases below
/// remain so existing call sites (pool_provider.dart) do not need
/// to rename.
class AppConfig {
  AppConfig._();

  /// Build-time device identity. The BFF aggregator uses this
  /// to stitch telemetry / matcher polls from the same physical
  /// device together. Defaults to the Owner-supplied mock device
  /// id (Sprint 10.1C, 10.07.2026 22:25).
  static const String deviceId = String.fromEnvironment(
    'DEVICE_ID',
    defaultValue: 'a1b2c3d4e5f60718a1b2c3d4',
  );

  /// Build-time API key. Kept for the 10.1B API key contract
  /// (S35 audit anchor in telemetry_service.dart). The 10.1D
  /// JWT auth flow does NOT use this directly — the JWT
  /// replaces it. The default `test_key_placeholder` keeps
  /// an out-of-the-box build working.
  static const String apiKey = String.fromEnvironment(
    'API_KEY',
    defaultValue: 'test_key_placeholder',
  );

  /// Sprint 10.1D — base URL for the BFF aggregator. The
  /// `auth_service` + `telemetry_service` + `p2p_matcher`
  /// all prefix their paths with this value. Defaults to
  /// the test environment (`api-test.opene2ee.com`); the
  /// production build overrides via `--dart-define
  /// API_BASE=https://api.opene2ee.com`.
  static const String apiBase = String.fromEnvironment(
    'API_BASE',
    defaultValue: 'https://api-test.opene2ee.com',
  );

  /// Sprint 10.1D — API version (`1` per the backend BFF
  /// router, NOT `v1` — the backend's `X-API-Version`
  /// validator expects the bare integer). Confirmed live
  /// by Owner-supplied curl probe on 10.07.2026 22:40:
  /// sending `X-API-Version: v1` returns
  ///   `{"code":"invalid_header","message":"Unsupported
  ///   X-API-Version; expected one of: 1"}`
  /// while `X-API-Version: 1` returns a real JWT.
  /// Hard-coded for now; promoted to a build-time
  /// injectable when the backend ships v2.
  static const String apiVersion = '1';
}

// ─── Backwards-compat aliases (Sprint 10.1C) ────────────────────
// pool_provider.dart imports these `kXxx` names from the
// 10.1C baseline. Keep them in sync with `AppConfig.xxx`.

const String kDeviceId = AppConfig.deviceId;
const String kApiKey = AppConfig.apiKey;
const String kApiBase = AppConfig.apiBase;
const String kApiVersion = AppConfig.apiVersion;

/// True when the build is a debug build (kReleaseMode == false
/// AND kDebugMode == true). The active pool screen uses this to
/// gate the `apiCallCount` debug caption — production builds
/// hide the counter to keep the screen clean.
bool get isDebugBuild => kDebugMode;

/// Active pool polling cadence. 5 seconds per the 10.1B brief
/// and re-confirmed by Owner for 10.1C.
const Duration kPoolPollPeriod = Duration(seconds: 5);
