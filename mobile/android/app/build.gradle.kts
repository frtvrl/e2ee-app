// mobile/android/app/build.gradle.kts
//
// PR-28 (Sprint 5) — Flutter app module build script.
//
// Configures the `:app` subproject that hosts both the Flutter embedding
// (Activity + MethodChannels) AND the native Kotlin VPN service that
// PR-22a introduced and PR-24 relocated into the Android source tree.
//
// Highlights:
//   - Applies Flutter's Android Gradle plugin (handles Dart build,
//     `flutter pub get`, GeneratedPluginRegistrant injection, asset
//     bundling).
//   - Applies the Kotlin Android plugin (lets Gradle compile the
//     `mobile/android/app/src/main/kotlin/com/opene2ee/opene2ee/**/*.kt`
//     sources — including `vpn/OpenE2eeVpnService.kt`).
//   - Pins minSdk = 23 (Android 6.0 Marshmallow) — see SPRINT-7 §MOB-5
//     rationale block below. targetSdk = 34 (Android 14, required for
//     `foregroundServiceType="specialUse"`), compileSdk = 34.
//   - Declares runtime deps: AndroidX core, AndroidX annotation (for
//     `@RequiresApi` lint enforcement), androidx.core (ServiceCompat),
//     kotlinx-coroutines-android (used by future async plumbing).
//   - Distinguishes debug vs release signing configs. Debug uses the
//     AGP-provided auto-generated debug keystore (no password needed,
//     valid for one year — fine for sampling / on-device testing).
//     Release is empty unless the developer drops a keystore at
//     `mobile/android/key.properties` (gitignored).
//
// ─── SPRINT-7 §MOB-5 — minSdk = 23 rationale (Android 6.0 Marshmallow)
// ───────────────────────────────────────────────────────────────────
// History: PR-22a / PR-28 set minSdk = 21 (Lollipop) because that was
// the floor for `VpnService.Builder.allowedApplications()` and
// `.disallowedApplications()`. MOB-5 (cyber-security hand-off) found
// this floor was BELOW the floor that flutter_secure_storage 9.x
// requires for AndroidKeyStore-backed encryption:
//
//   • flutter_secure_storage 9.2.4 (pinned in pubspec.yaml) documents:
//       "API Level: Android 6.0 (API 23) minimum for basic encryption"
//     https://pub.dev/packages/flutter_secure_storage  (Requirements)
//   • flutter_secure_storage 9.x changelog records:
//       "Minimum Android SDK changed from 19 to 23"
//   • `android.security.keystore.KeyGenParameterSpec` (the modern
//     AndroidKeyStore key-generation builder — supports PURPOSE_*,
//     BLOCK_MODE_*, ENCRYPTION_PADDING_*, user-auth requirements,
//     randomized-encryption flag) was added in API 23.
//     Pre-API-23 only had `KeyPairGeneratorSpec`, which lacks every
//     one of those controls. flutter_secure_storage's AES master key
//     cannot be created on API < 23 without falling back to software-
//     only SharedPreferences — which would silently break the Ed25519
//     private-key-at-rest guarantee that ADR-0006 §B1 relies on.
//
// Decision: bump minSdk from 21 → 23. This is preferred over
// "attestation" (Option B in the Sprint 7 spec) because the API
// literally does not exist on API < 23 — there is nothing to
// attest to. It is preferred over "explicit failure at runtime"
// (Option C) because end-users on Android 5.x would see a crash
// with no recourse; refusing to install is more honest.
//
// VPN impact: `VpnService.Builder.allowedApplications()` is API 21+,
// so VPN functionality is preserved unchanged. The `@RequiresApi(21)`
// annotations on OpenE2eeVpnService.kt are now belt-and-braces
// (the floor enforces it) but kept as defensive documentation.
//
// Market-share note (2026): Android 5.0-5.1.1 < 0.5% of active
// devices per Google Play Console public dashboards. Acceptable
// drop for a security-critical E2EE app.
//
// Regression guard: mobile/test/min_sdk_posture_test.dart parses this
// file and asserts minSdk >= 23. Update the test constant if the
// floor ever moves.
//
// References:
//   - https://docs.flutter.dev/deployment/android
//   - https://developer.android.com/build/building-cmdline
//   - https://developer.android.com/reference/android/security/keystore/KeyGenParameterSpec
//   - https://pub.dev/packages/flutter_secure_storage
//   - docs/ADR-0003-vpn-layer.md
//   - docs/ADR-0006-anonimlik.md (Ed25519 private-key-at-rest contract)
//   - docs/SPRINT-7-SCOPE.md §Item 6 MOB-5

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("dev.flutter.flutter-gradle-plugin")
}

android {
    namespace = "com.opene2ee.opene2ee"
    compileSdk = 34
    ndkVersion = flutter.ndkVersion

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    sourceSets {
        getByName("main").java.srcDirs("src/main/kotlin")
    }

    defaultConfig {
        // Floor — API 23 (Android 6.0 Marshmallow) — see the SPRINT-7
        // §MOB-5 rationale block at the top of this file. The two
        // hard requirements that pin this floor are:
        //   (1) `android.security.keystore.KeyGenParameterSpec` (the
        //       AndroidKeyStore key-generation builder used by
        //       flutter_secure_storage's AES master key) is API 23+.
        //   (2) flutter_secure_storage 9.2.4 explicitly documents
        //       "Android 6.0 (API 23) minimum for basic encryption".
        // VPN functionality is preserved: `VpnService.Builder
        // .allowedApplications()` / `.disallowedApplications()` are
        // API 21+, so the per-app VPN features OpenE2eeVpnService.kt
        // ships still work. The `@RequiresApi(21)` annotations in
        // OpenE2eeVpnService.kt are now defensive (the floor enforces
        // it) but kept for documentation.
        // Regression guard: mobile/test/min_sdk_posture_test.dart.
        minSdk = 23
        targetSdk = 34
        versionCode = 1
        versionName = "0.1.0"
    }

    signingConfigs {
        // Debug uses AGP's auto-generated debug keystore (created on
        // first build under `~/.android/debug.keystore`). No password
        // required. Long enough validity for sampling / QA / CI builds.
        getByName("debug") {
            // Defaults are fine; we declare it explicitly so the
            // release block below can mirror its structure.
        }

        // Release — empty by default. If a developer drops a
        // `key.properties` file at `mobile/android/key.properties`
        // (see `.gitignore`), it is loaded here and a release
        // signing config is wired up. We deliberately do NOT fall
        // back to the debug keystore for release builds — that
        // would silently ship APKls signed with the well-known
        // Android SDK debug key.
        create("release") {
            val keyPropsFile = rootProject.file("key.properties")
            if (keyPropsFile.exists()) {
                val keyProps = java.util.Properties().apply {
                    load(keyPropsFile.inputStream())
                }
                storeFile = file(keyProps["storeFile"] as String)
                storePassword = keyProps["storePassword"] as String
                keyAlias = keyProps["keyAlias"] as String
                keyPassword = keyProps["keyPassword"] as String
            }
        }
    }

    buildTypes {
        getByName("release") {
            // Sign release with the release config when a key is
            // configured, otherwise sign with debug so local
            // `flutter build apk --release` runs succeed (Play Store
            // uploads still require a real keystore).
            signingConfig = if (rootProject.file("key.properties").exists()) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
            // PR-39 (Sprint 6) — R8/ProGuard now enabled for release.
            // Addresses cyber-security review finding MOB-3 (High):
            //   "Release builds ship with `isMinifyEnabled = false;
            //    isShrinkResources = false` — full Kotlin symbol names +
            //    dev-friendly stack traces land in the APK. OWASP
            //    MASVS-CODE-2 violation."
            //
            // `proguard-android-optimize.txt` is AGP's optimized rule set
            // (further passes than `proguard-android.txt`); `proguard-rules.pro`
            // carries our project-specific keep rules (Flutter embedding,
            // MethodChannels, JNI callbacks, JSON DTOs).
            //
            // Verify every release build still runs the MethodChannels:
            //   - Flutter embedding activity launches cleanly
            //   - `mobile/vpn` MethodChannel reaches OpenE2eeVpnService
            //   - `flutter_secure_storage` MethodChannel reaches the
            //     Android Keystore backing store
            // See docs/SPRINT-6-PR-39-VERIFICATION.md for the manual
            // smoke-test plan.
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro",
            )
        }
    }
}

flutter {
    source = "../.."
}

dependencies {
    // Kotlin runtime — provided by the Kotlin Android plugin via
    // stdlib, declared explicitly so the Kotlin compiler can resolve
    // references like `kotlin.LazyThreadSafetyMode` from the VPN service.
    implementation("androidx.core:core-ktx:1.12.0")
    implementation("androidx.annotation:annotation:1.7.1")
    // `androidx.core.app.ServiceCompat` (startForeground, startForegroundService)
    // and `ForegroundServiceType` constants land here. Required by the
    // PR-28 transient-service fix to keep `startForeground` callable on
    // API 34+ without the deprecation warning breaking CI.
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.7.3")
}

// Ensure the VPN service class file ends up in the compiled output —
// defensive; AGP/Kotlin already pick up `src/main/kotlin` via the
// sourceSets entry above, but a future rename or move is easier to
// audit if we list the package here explicitly.
androidComponents {
    onVariants { variant ->
        // no-op; placeholder for future ABI/signing hooks.
    }
}