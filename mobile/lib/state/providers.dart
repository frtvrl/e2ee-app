// mobile/lib/state/providers.dart
//
// Sprint 9.6.8 — Riverpod providers for the app shell. Production
// wiring only — tests override these in
// `ProviderScope(overrides: [...])` (see `mobile/test/main_test.dart`).
//
// Why a flat file (one provider per type) instead of nested
// providers? 9.6.8 ships a minimal shell with 2 providers; a future
// sprint can split this file by feature if the count grows. Keep
// this file flat for now — easier to grep, easier to review.

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../mobile/auth/biometric.dart';
import '../shared/device_identity.dart';

/// Production biometric authenticator.
///
/// Tests override this with `FakeBiometricAuthenticator` to avoid
/// touching the platform `local_auth` channel.
final biometricAuthProvider = Provider<BiometricAuthenticator>(
  (ref) => LocalAuthBiometricAuthenticator(),
);

/// Loads (or creates) the device identity on first read.
///
/// `loadOrCreate()` mints a UUID v7 + Ed25519 keypair on first run
/// and stores the private key in `flutter_secure_storage` (Android
/// Keystore / iOS Keychain). Subsequent reads return the persisted
/// identity. The future is intentionally not cached in a state
/// provider — each read is cheap (one Keystore round-trip) and a
/// stale identity after a KVKK `reset()` would be a privacy bug.
final deviceIdentityProvider = FutureProvider<DeviceIdentity>(
  (ref) => DeviceIdentity.loadOrCreate(),
);
