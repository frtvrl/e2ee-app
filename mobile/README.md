# OpenE2EE — Mobile (Flutter)

Flutter client for the OpenE2EE end-to-end-encryption transparency tool.
Cross-platform (Android + iOS + Web) with shared Dart core under
`mobile/lib/shared/` and per-platform shells under `mobile/lib/mobile/`,
`mobile/lib/ios/`, and `mobile/lib/web/`.

> **Architectural note.** This README documents the Android and iOS native
> shell policies that are enforced at the build-system layer. Per-platform
> implementation details live in the relevant `mobile/<platform>/` source
> tree and ADRs (see `docs/`).

---

## 1. Android — `minSdk` policy

### 1.1 Current floor

| Field            | Value                       | Source                                  |
| ---------------- | --------------------------- | --------------------------------------- |
| `compileSdk`     | **34** (Android 14)         | `mobile/android/app/build.gradle.kts`   |
| `targetSdk`      | **34** (Android 14)         | `mobile/android/app/build.gradle.kts`   |
| `minSdk`         | **23** (Android 6.0 MM)     | `mobile/android/app/build.gradle.kts`   |
| `ndkVersion`     | `flutter.ndkVersion`        | `mobile/android/app/build.gradle.kts`   |

### 1.2 Rationale (Sprint 7 §MOB-5)

`mobile/android/app/build.gradle.kts` pins **`minSdk = 23`** (Android 6.0
Marshmallow). The previous floor of 21 (Android 5.0 Lollipop) was set by
PR-22a/PR-28 for `VpnService.Builder.allowedApplications()`, but Sprint 7
MOB-5 (cyber-security hand-off) found that floor was **below the floor
required by `flutter_secure_storage 9.x`** for hardware-backed Ed25519
private-key-at-rest:

1. **`flutter_secure_storage 9.2.4`** (pinned in `mobile/pubspec.yaml`)
   publishes the requirement as: *"API Level: Android 6.0 (API 23)
   minimum for basic encryption"* (pub.dev Requirements section).
2. **flutter_secure_storage 9.x changelog** explicitly records:
   *"Minimum Android SDK changed from 19 to 23"*.
3. **`android.security.keystore.KeyGenParameterSpec`** — the modern
   AndroidKeyStore key-generation builder that flutter_secure_storage
   uses to create its AES master key — was introduced in **API 23**.
   The pre-API-23 alternative `KeyPairGeneratorSpec` lacks every modern
   control (purpose flags, block modes, padding, user-auth requirements,
   randomized-encryption flag). On API < 23 the master-key generation
   either throws `KeyStoreException` or silently falls back to
   software-only SharedPreferences, breaking the Ed25519 private-key-at-
   rest guarantee from `docs/ADR-0006-anonimlik.md §B1`.
4. **Option B (attestation) and Option C (explicit failure)** from the
   Sprint 7 §MOB-5 spec are not viable here: there is nothing to attest
   to on API < 23 (the API literally does not exist), and an explicit
   runtime failure leaves end-users on Android 5.x with a crash and no
   recourse. **Refusing to install (via `minSdk`)** is the only honest
   option.

### 1.3 Why this is acceptable

| API level | 2026 global active share (Google Play Console public dashboard) |
| --------- | --------------------------------------------------------------- |
| 21-22 (5.0-5.1 Lollipop) | **< 0.5%**                                            |
| 23+ (6.0+ Marshmallow)   | **> 99%**                                             |

For a security-critical end-to-end-encryption app, dropping the bottom
0.5% is the right trade. Users on Android 5.x will see "This app is not
available for your device" in the Play Store — clearer and safer than a
silent insecure storage path.

### 1.4 What is preserved

* `VpnService.Builder.allowedApplications()` / `.disallowedApplications()`
  (the original API 21 floor reference) **still work** on API 23+. The
  per-app VPN features in `OpenE2eeVpnService.kt` are unchanged.
* `targetSdk = 34` (Android 14) — required for
  `foregroundServiceType="specialUse"` on the VPN service.
* `compileSdk = 34` — matches Flutter 3.24+ defaults.

### 1.5 Regression guard

`mobile/test/min_sdk_posture_test.dart` parses
`mobile/android/app/build.gradle.kts` and asserts **`minSdk ≥ 23`**.
The test runs as part of `flutter test`, so a regression that lowers
the floor would be caught in CI before a release build. Update the
test constant `kMinRequiredAndroidSdk` if the floor ever moves.

---

## 2. Android — AndroidKeyStore contract

`mobile/lib/shared/device_identity.dart` stores the Ed25519 private key
in `FlutterSecureStorage`, which on Android is backed by the
**AndroidKeyStore provider** (see ADR-0006 §"Alternatives" for the
threat model).

The contract enforced by the `minSdk = 23` floor:

1. **`flutter_secure_storage` creates an AES-256 master key** under the
   alias `AndroidKeyStore` using `KeyGenParameterSpec` with
   `BLOCK_MODE_GCM` + `ENCRYPTION_PADDING_NONE` + `setRandomizedEncryption
   Required(true)`.
2. **The Ed25519 private key bytes are encrypted under that master key**
   before they ever touch SharedPreferences. Plaintext key bytes never
   leave the secure-storage API boundary.
3. **The master key is non-exportable** — AndroidKeyStore guarantees the
   raw key material cannot be extracted from the device, even by a
   rooted user.
4. **No silent fallback.** If the AndroidKeyStore API cannot honour the
   request, `flutter_secure_storage` raises an exception that bubbles
   up to `DeviceIdentity.loadOrCreate` — there is no code path that
   silently writes the private key in plaintext.

This contract depends on API 23+. API < 23 cannot honour it.

### 2.1 What we explicitly do NOT do

* **No "dev-mode" plaintext fallback** in the secure-storage path.
  Unlike the backend JWT dev-secret (SEC-1 hardening, see
  `docs/policy/`), the mobile secure-storage path never has a fallback
  branch that would persist the private key unencrypted.
* **No IMEI / serial / phoneNumber / MAC** as auxiliary key material
  (per ADR-0006 §"Veri Minimizasyonu"). A grep-based CI check lives in
  `tool/ci_grep_privacy_violations.dart` (Sprint 7 §STRIDE-3-01).

---

## 3. iOS — `IPHONEOS_DEPLOYMENT_TARGET` policy

iOS minimum target is set in `mobile/ios/Podfile` (CocoaPods) and
`mobile/ios/Runner.xcodeproj/project.pbxproj` (Xcode build settings). The
default Flutter scaffold ships iOS 12.0; the project inherits that floor.
Unlike Android, iOS Keychain has been API-stable since iOS 8 and does not
have the pre-/post-API floor split that the AndroidKeyStore contract
requires — so no explicit bump is needed for the MOB-5 hardening.

---

## 4. Web — Flutter Web build target

Web build target uses `dart2wasm` (Flutter 3.24+) and runs entirely in
the browser sandbox. No Keystore / Keychain is consulted on web. The
device identity module is **disabled** on web (it requires a platform
secure storage), and the web dashboard at `mobile/lib/web/` operates
without the Ed25519 signing path.

---

## 5. Build matrix

| Platform | Target            | Min floor | Why                                          |
| -------- | ----------------- | --------- | -------------------------------------------- |
| Android  | API 34 (Android 14) | API 23 (Android 6.0) | AndroidKeyStore + flutter_secure_storage 9.x |
| iOS      | iOS 17 (latest)   | iOS 12    | Flutter scaffold default                     |
| Web      | wasm (browser)    | n/a       | Browser sandbox — no Keystore contract       |
| Windows  | Win 10 1903+      | n/a       | Not currently shipped                        |
| macOS    | macOS 13+         | n/a       | Not currently shipped                        |
| Linux    | glibc 2.31+       | n/a       | Not currently shipped                        |

---

## 6. How to verify the Android floor on a real device

```bash
# Build a debug APK
cd mobile
flutter build apk --debug

# Inspect the manifest minSdkVersion printed into the APK
$ANDROID_HOME/build-tools/34.0.0/aapt dump badging \
    build/app/outputs/flutter-apk/app-debug.apk \
    | grep -E "(sdkVersion|targetSdkVersion|package:)"

# Expected:
#   package: name='com.opene2ee.opene2ee'
#   sdkVersion:'23'
#   targetSdkVersion:'34'
```

If `sdkVersion:'23'` is printed, the floor is correctly enforced.

### 6.1 Manual smoke test

1. Install on an Android 6.0 (API 23) device or emulator.
2. Launch the app. The consent screen should appear.
3. Tap "Allow" — `DeviceIdentity.loadOrCreate` runs, the AndroidKeyStore
   master key is created, the Ed25519 keypair is generated and persisted
   to secure storage.
4. Force-stop and relaunch — `loadOrCreate` reads the persisted key
   without re-issuing.
5. Pull a `logcat` and verify NO log lines contain the private-key
   material (the `DeviceIdentity` module never logs, per ADR-0006).

### 6.2 What NOT to test

We do NOT support Android 5.x. Do not file a "bug" that the app refuses
to install on a Lollipop device — that is the desired behaviour.

---

## 7. Cross-references

* `docs/ADR-0003-vpn-layer.md` — VPN threat model (per-app allowlist)
* `docs/ADR-0006-anonimlik.md` — anonymity contract + key-at-rest
  guarantee
* `docs/SPRINT-7-SCOPE.md` §Item 6 (MOB-5) — original finding
* `mobile/android/app/build.gradle.kts` — source of truth for `minSdk`
* `mobile/test/min_sdk_posture_test.dart` — regression guard
* `mobile/lib/shared/device_identity.dart` — Keystore-backed identity